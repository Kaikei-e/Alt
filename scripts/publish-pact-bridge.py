#!/usr/bin/env python3
"""Publish manual Pact verification records for one (provider, consumer) bridge.

Replaces the per-version and per-publish-target Ansible `uri` loops in
playbooks/_publish-bridge.yml with a single serial Python process that opens a
fresh HTTP connection per request (matching `uri` connection semantics without
Ansible per-item task overhead).

Input is read as a JSON document from stdin:
  {
    "broker_base_url": "http(s)://host[:port]",
    "broker_username": "<user>",
    "bridge": {
      "provider": "<provider>",
      "consumer": "<consumer>",
      "implementation": "<verifier-label>",
      "provider_version": "<sha>",
      "evidence": {
        "result": "passed",
        "implementation": "<verifier-label>",
        "verifier_version": "<version>"
      }
    }
  }

The basic-auth password is read exclusively from `PACT_BROKER_PASSWORD` in the
inherited process environment so it never appears on argv, and all error paths
redact both the raw password and its base64 token before writing to stderr.
"""

from __future__ import annotations

import base64
import http.client
import json
import os
import sys
import urllib.parse
from typing import Any, NamedTuple

REQUEST_TIMEOUT_SECONDS = 30
HTTP_OK = 200
HTTP_NOT_FOUND = 404
_DEFAULT_PORTS = {"http": 80, "https": 443}


class BridgePublishError(Exception):
    """Raised when a broker request or response invariant fails."""


def _redact(text: str, secrets: tuple[str, ...]) -> str:
    out = text
    for secret in secrets:
        if secret:
            out = out.replace(secret, "<redacted>")
    return out


def _origin(parsed: urllib.parse.SplitResult, *, pair: str) -> tuple[str, str, int]:
    scheme = parsed.scheme.lower()
    if scheme not in _DEFAULT_PORTS:
        msg = f"Unsupported scheme in broker URL for {pair}: {parsed.scheme!r}"
        raise BridgePublishError(msg)
    if parsed.username is not None or parsed.password is not None:
        msg = f"Refusing broker URL containing userinfo for {pair}"
        raise BridgePublishError(msg)
    host = (parsed.hostname or "").lower()
    if not host:
        msg = f"Invalid broker URL (missing host) for {pair}: {parsed.geturl()}"
        raise BridgePublishError(msg)
    try:
        port = parsed.port if parsed.port is not None else _DEFAULT_PORTS[scheme]
    except ValueError as exc:
        msg = f"Invalid port in broker URL for {pair}: {parsed.geturl()}"
        raise BridgePublishError(msg) from exc
    return (scheme, host, port)


def _open_connection(origin: tuple[str, str, int], timeout: int) -> http.client.HTTPConnection:
    scheme, host, port = origin
    if scheme == "http":
        return http.client.HTTPConnection(host, port, timeout=timeout)
    return http.client.HTTPSConnection(host, port, timeout=timeout)


class _BrokerEndpoint(NamedTuple):
    base_origin: tuple[str, str, int]
    headers: dict[str, str]
    pair: str
    timeout: int = REQUEST_TIMEOUT_SECONDS


class _BridgePayload(NamedTuple):
    endpoint: _BrokerEndpoint
    versions_url: str
    provider_version: str
    implementation: str
    verifier_version: str
    success: bool


def _request(
    endpoint: _BrokerEndpoint,
    method: str,
    url: str,
    body: bytes | None = None,
    *,
    extra_headers: dict[str, str] | None = None,
) -> tuple[int, bytes]:
    target = urllib.parse.urlsplit(url)
    # Refuse to send basic-auth credentials if a broker link points at a
    # different origin or carries userinfo (audit F-004).
    target_origin = _origin(target, pair=endpoint.pair)
    if target_origin != endpoint.base_origin:
        msg = f"Refusing to follow cross-origin broker URL for {endpoint.pair}: {url}"
        raise BridgePublishError(msg)
    path = target.path or "/"
    if target.query:
        path = f"{path}?{target.query}"

    headers = {**endpoint.headers, **(extra_headers or {}), "Connection": "close"}
    conn = _open_connection(endpoint.base_origin, endpoint.timeout)
    try:
        conn.request(method, path, body=body, headers=headers)
        resp = conn.getresponse()
        data = resp.read()
    except (http.client.HTTPException, OSError) as exc:
        msg = f"HTTP {method} failed for {endpoint.pair} at {url}: {exc}"
        raise BridgePublishError(msg) from exc
    else:
        return resp.status, data
    finally:
        conn.close()


def _load_stdin() -> dict[str, Any]:
    try:
        doc = json.loads(sys.stdin.read())
    except (OSError, json.JSONDecodeError) as exc:
        msg = f"Invalid bridge input JSON: {exc}"
        raise BridgePublishError(msg) from exc
    if not isinstance(doc, dict):
        msg = "Bridge input JSON must be an object"
        raise BridgePublishError(msg)
    return doc


def _parse_bridge_payload(doc: dict[str, Any], password: str, *, timeout: int) -> _BridgePayload:
    broker_base_url = str(doc.get("broker_base_url") or "").rstrip("/")
    broker_username = str(doc.get("broker_username") or "")
    bridge = doc.get("bridge")
    if not isinstance(bridge, dict):
        msg = "Bridge input JSON requires a 'bridge' object"
        raise BridgePublishError(msg)

    provider = str(bridge.get("provider") or "")
    consumer = str(bridge.get("consumer") or "")
    provider_version = str(bridge.get("provider_version") or "")
    evidence = bridge.get("evidence")
    if not isinstance(evidence, dict):
        msg = "Bridge input JSON requires a 'bridge.evidence' object"
        raise BridgePublishError(msg)

    implementation = str(evidence.get("implementation") or "")
    verifier_version = str(evidence.get("verifier_version") or "unknown")
    success = evidence.get("result") == "passed"

    pair = f"{consumer} → {provider}"
    if not all(
        (broker_base_url, broker_username, provider, consumer, provider_version, implementation)
    ):
        msg = f"Missing required bridge fields for {pair}"
        raise BridgePublishError(msg)

    base_origin = _origin(urllib.parse.urlsplit(broker_base_url), pair=pair)
    b64_auth = base64.b64encode(f"{broker_username}:{password}".encode()).decode("ascii")
    base_headers = {
        "Authorization": f"Basic {b64_auth}",
        "Accept": "application/hal+json, application/json",
    }
    endpoint = _BrokerEndpoint(
        base_origin=base_origin, headers=base_headers, pair=pair, timeout=timeout
    )
    versions_url = f"{broker_base_url}/pacts/provider/{provider}/consumer/{consumer}/versions"
    return _BridgePayload(
        endpoint=endpoint,
        versions_url=versions_url,
        provider_version=provider_version,
        implementation=implementation,
        verifier_version=verifier_version,
        success=success,
    )


def _fetch_pact_version_urls(endpoint: _BrokerEndpoint, versions_url: str) -> list[str]:
    status, raw_versions = _request(endpoint, "GET", versions_url)
    if status == HTTP_NOT_FOUND:
        msg = (
            f"No pact on the broker for {endpoint.pair} ({versions_url}), "
            "but bridge_registry declares one. Either the consumer has "
            "never published this pact, or the row outlived the contract "
            "and should be deleted from bridge_registry."
        )
        raise BridgePublishError(msg)
    if status != HTTP_OK:
        msg = f"Failed to list pact versions for {endpoint.pair} at {versions_url}: HTTP {status}"
        raise BridgePublishError(msg)

    try:
        versions_doc = json.loads(raw_versions.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        msg = f"Malformed JSON from {versions_url} for {endpoint.pair}: {exc}"
        raise BridgePublishError(msg) from exc

    pacts = ((versions_doc.get("_embedded") or {}).get("pacts")) or []
    if not isinstance(pacts, list) or len(pacts) == 0:
        msg = (
            f"No pact on the broker for {endpoint.pair} ({versions_url}), "
            "but bridge_registry declares one. Either the consumer has "
            "never published this pact, or the row outlived the contract "
            "and should be deleted from bridge_registry."
        )
        raise BridgePublishError(msg)

    pact_version_urls: list[str] = []
    for pact in pacts:
        href = (
            ((pact.get("_links") if isinstance(pact, dict) else None) or {})
            .get("self", {})
            .get("href")
        )
        if not isinstance(href, str) or not href:
            msg = (
                f"Missing _links.self.href in pact version entry for "
                f"{endpoint.pair} at {versions_url}"
            )
            raise BridgePublishError(msg)
        pact_version_urls.append(href)
    return pact_version_urls


def _collect_publish_targets(
    endpoint: _BrokerEndpoint, pact_version_urls: list[str], versions_url: str
) -> list[str]:
    publish_targets: list[str] = []
    for detail_url in pact_version_urls:
        detail_status, raw_detail = _request(endpoint, "GET", detail_url)
        if detail_status != HTTP_OK:
            msg = (
                f"Failed to fetch pact version details for {endpoint.pair} at "
                f"{detail_url}: HTTP {detail_status}"
            )
            raise BridgePublishError(msg)
        try:
            detail_doc = json.loads(raw_detail.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            msg = f"Malformed JSON from {detail_url} for {endpoint.pair}: {exc}"
            raise BridgePublishError(msg) from exc

        links = (detail_doc.get("_links") if isinstance(detail_doc, dict) else None) or {}
        pub_link = links.get("pb:publish-verification-results")
        if isinstance(pub_link, dict):
            pub_href = pub_link.get("href")
            if isinstance(pub_href, str) and pub_href:
                publish_targets.append(pub_href)

    if len(publish_targets) == 0:
        msg = (
            f"No publish-verification-results URL resolved for {endpoint.pair} "
            f"({versions_url}). Broker response schema may have changed "
            "or consumer has never been published."
        )
        raise BridgePublishError(msg)
    return publish_targets


def publish_bridge(
    doc: dict[str, Any],
    password: str,
    *,
    timeout: int = REQUEST_TIMEOUT_SECONDS,
) -> dict[str, int]:
    payload = _parse_bridge_payload(doc, password, timeout=timeout)
    pact_version_urls = _fetch_pact_version_urls(payload.endpoint, payload.versions_url)
    publish_targets = _collect_publish_targets(
        payload.endpoint, pact_version_urls, payload.versions_url
    )

    post_body = json.dumps(
        {
            "success": payload.success,
            "providerApplicationVersion": payload.provider_version,
            "verifiedBy": {
                "implementation": payload.implementation,
                "version": payload.verifier_version,
            },
        }
    ).encode("utf-8")

    for publish_url in publish_targets:
        post_status, _ = _request(
            payload.endpoint,
            "POST",
            publish_url,
            body=post_body,
            extra_headers={"Content-Type": "application/json"},
        )
        if post_status not in (200, 201):
            msg = (
                f"Failed to publish verification record for {payload.endpoint.pair} at "
                f"{publish_url}: HTTP {post_status}"
            )
            raise BridgePublishError(msg)

    return {
        "pact_versions": len(pact_version_urls),
        "verifications_posted": len(publish_targets),
    }


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    if args:
        sys.stderr.write(
            "publish-pact-bridge: takes no positional or flag arguments; pass JSON via stdin\n"
        )
        return 2

    password = os.environ.get("PACT_BROKER_PASSWORD", "")
    if not password:
        sys.stderr.write(
            "publish-pact-bridge: PACT_BROKER_PASSWORD environment variable is required\n"
        )
        return 1

    try:
        doc = _load_stdin()
        username = str(doc.get("broker_username") or "")
        b64_secret = base64.b64encode(f"{username}:{password}".encode()).decode("ascii")
        secrets = (password, b64_secret)
    except BridgePublishError as exc:
        sys.stderr.write(f"{_redact(str(exc), (password,))}\n")
        return 1

    try:
        result = publish_bridge(doc, password)
    except BridgePublishError as exc:
        sys.stderr.write(f"{_redact(str(exc), secrets)}\n")
        return 1

    sys.stdout.write(json.dumps(result) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
