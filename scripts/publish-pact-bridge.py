#!/usr/bin/env python3
"""Publish manual Pact verification records for one (provider, consumer) bridge.

Replaces the per-version and per-publish-target Ansible `uri` loops in
playbooks/_publish-bridge.yml with a single serial HTTP session against the
Pact Broker so that large pact version histories do not pay per-item task
overhead.

Input is read as a JSON document from stdin (or `--input <path>`):
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
environment so it never appears on argv, and all error paths redact it before
writing to stderr.
"""

from __future__ import annotations

import argparse
import base64
import http.client
import json
import os
import pathlib
import sys
import urllib.parse
from typing import Any

REQUEST_TIMEOUT_SECONDS = 30


class BridgePublishError(Exception):
    """Raised when a broker request or response invariant fails."""


def _redact(text: str, secrets: tuple[str, ...]) -> str:
    out = text
    for secret in secrets:
        if secret:
            out = out.replace(secret, "<redacted>")
    return out


def _open_connection(
    parsed_base: urllib.parse.SplitResult, timeout: int
) -> http.client.HTTPConnection:
    host = parsed_base.hostname
    if not host:
        raise BridgePublishError(
            f"Invalid broker_base_url (missing host): {parsed_base.geturl()}"
        )
    if parsed_base.scheme == "http":
        return http.client.HTTPConnection(
            host, parsed_base.port, timeout=timeout
        )
    if parsed_base.scheme == "https":
        return http.client.HTTPSConnection(
            host, parsed_base.port, timeout=timeout
        )
    raise BridgePublishError(
        f"Unsupported scheme in broker_base_url: {parsed_base.scheme!r}"
    )


def _request(
    conn: http.client.HTTPConnection,
    parsed_base: urllib.parse.SplitResult,
    method: str,
    url: str,
    headers: dict[str, str],
    body: bytes | None = None,
    *,
    pair: str,
) -> tuple[int, bytes]:
    target = urllib.parse.urlsplit(url)
    # Refuse to send basic-auth credentials if a broker link points at a
    # different origin (audit F-004).
    if (target.scheme, target.netloc) != (
        parsed_base.scheme,
        parsed_base.netloc,
    ):
        raise BridgePublishError(
            f"Refusing to follow cross-origin broker URL for {pair}: {url}"
        )
    path = target.path or "/"
    if target.query:
        path = f"{path}?{target.query}"
    try:
        conn.request(method, path, body=body, headers=headers)
        resp = conn.getresponse()
        data = resp.read()
        return resp.status, data
    except (http.client.HTTPException, OSError) as exc:
        raise BridgePublishError(
            f"HTTP {method} failed for {pair} at {url}: {exc}"
        ) from exc


def _load_input(input_arg: str | None) -> dict[str, Any]:
    try:
        if input_arg and input_arg != "-":
            raw = pathlib.Path(input_arg).read_text(encoding="utf-8")
        else:
            raw = sys.stdin.read()
        doc = json.loads(raw)
    except (OSError, json.JSONDecodeError) as exc:
        raise BridgePublishError(f"Invalid bridge input JSON: {exc}") from exc
    if not isinstance(doc, dict):
        raise BridgePublishError("Bridge input JSON must be an object")
    return doc


def publish_bridge(
    doc: dict[str, Any],
    password: str,
    *,
    timeout: int = REQUEST_TIMEOUT_SECONDS,
) -> dict[str, int]:
    broker_base_url = str(doc.get("broker_base_url") or "").rstrip("/")
    broker_username = str(doc.get("broker_username") or "")
    bridge = doc.get("bridge") if isinstance(doc.get("bridge"), dict) else doc

    provider = str(bridge.get("provider") or "")
    consumer = str(bridge.get("consumer") or "")
    provider_version = str(bridge.get("provider_version") or "")
    evidence = (
        bridge.get("evidence")
        if isinstance(bridge.get("evidence"), dict)
        else {}
    )
    implementation = str(
        evidence.get("implementation") or bridge.get("implementation") or ""
    )
    verifier_version = str(evidence.get("verifier_version") or "unknown")
    success = evidence.get("result") == "passed"

    pair = f"{consumer} → {provider}"
    if (
        not broker_base_url
        or not broker_username
        or not provider
        or not consumer
        or not provider_version
        or not implementation
    ):
        raise BridgePublishError(
            f"Missing required bridge fields for {pair}"
        )

    parsed_base = urllib.parse.urlsplit(broker_base_url)
    b64_auth = base64.b64encode(
        f"{broker_username}:{password}".encode("utf-8")
    ).decode("ascii")
    base_headers = {
        "Authorization": f"Basic {b64_auth}",
        "Accept": "application/hal+json, application/json",
    }

    versions_url = (
        f"{broker_base_url}/pacts/provider/{provider}"
        f"/consumer/{consumer}/versions"
    )

    conn = _open_connection(parsed_base, timeout)
    try:
        status, raw_versions = _request(
            conn,
            parsed_base,
            "GET",
            versions_url,
            base_headers,
            pair=pair,
        )
        if status == 404:
            raise BridgePublishError(
                f"No pact on the broker for {pair} ({versions_url}), "
                "but bridge_registry declares one. Either the consumer has "
                "never published this pact, or the row outlived the contract "
                "and should be deleted from bridge_registry."
            )
        if status != 200:
            raise BridgePublishError(
                f"Failed to list pact versions for {pair} at {versions_url}: "
                f"HTTP {status}"
            )

        try:
            versions_doc = json.loads(raw_versions.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise BridgePublishError(
                f"Malformed JSON from {versions_url} for {pair}: {exc}"
            ) from exc

        pacts = ((versions_doc.get("_embedded") or {}).get("pacts")) or []
        if not isinstance(pacts, list) or len(pacts) == 0:
            raise BridgePublishError(
                f"No pact on the broker for {pair} ({versions_url}), "
                "but bridge_registry declares one. Either the consumer has "
                "never published this pact, or the row outlived the contract "
                "and should be deleted from bridge_registry."
            )

        pact_version_urls: list[str] = []
        for pact in pacts:
            href = (
                ((pact.get("_links") if isinstance(pact, dict) else None) or {})
                .get("self", {})
                .get("href")
            )
            if not isinstance(href, str) or not href:
                raise BridgePublishError(
                    f"Missing _links.self.href in pact version entry for "
                    f"{pair} at {versions_url}"
                )
            pact_version_urls.append(href)

        publish_targets: list[str] = []
        for detail_url in pact_version_urls:
            detail_status, raw_detail = _request(
                conn,
                parsed_base,
                "GET",
                detail_url,
                base_headers,
                pair=pair,
            )
            if detail_status != 200:
                raise BridgePublishError(
                    f"Failed to fetch pact version details for {pair} at "
                    f"{detail_url}: HTTP {detail_status}"
                )
            try:
                detail_doc = json.loads(raw_detail.decode("utf-8"))
            except (UnicodeDecodeError, json.JSONDecodeError) as exc:
                raise BridgePublishError(
                    f"Malformed JSON from {detail_url} for {pair}: {exc}"
                ) from exc

            links = (
                detail_doc.get("_links")
                if isinstance(detail_doc, dict)
                else None
            ) or {}
            pub_link = links.get("pb:publish-verification-results")
            if isinstance(pub_link, dict):
                pub_href = pub_link.get("href")
                if isinstance(pub_href, str) and pub_href:
                    publish_targets.append(pub_href)

        if len(publish_targets) == 0:
            raise BridgePublishError(
                f"No publish-verification-results URL resolved for {pair} "
                f"({versions_url}). Broker response schema may have changed "
                "or consumer has never been published."
            )

        post_body = json.dumps(
            {
                "success": success,
                "providerApplicationVersion": provider_version,
                "verifiedBy": {
                    "implementation": implementation,
                    "version": verifier_version,
                },
            }
        ).encode("utf-8")
        post_headers = {
            **base_headers,
            "Content-Type": "application/json",
        }

        for publish_url in publish_targets:
            post_status, _ = _request(
                conn,
                parsed_base,
                "POST",
                publish_url,
                post_headers,
                body=post_body,
                pair=pair,
            )
            if post_status not in (200, 201):
                raise BridgePublishError(
                    f"Failed to publish verification record for {pair} at "
                    f"{publish_url}: HTTP {post_status}"
                )

        return {
            "pact_versions": len(pact_version_urls),
            "verifications_posted": len(publish_targets),
        }
    finally:
        conn.close()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--input",
        default="-",
        help="Path to JSON input file, or '-' for stdin (default)",
    )
    args = parser.parse_args(argv)

    password = os.environ.get("PACT_BROKER_PASSWORD", "")
    if not password:
        sys.stderr.write(
            "publish-pact-bridge: PACT_BROKER_PASSWORD environment variable is required\n"
        )
        return 1

    try:
        doc = _load_input(args.input)
        username = str(doc.get("broker_username") or "")
        b64_secret = base64.b64encode(
            f"{username}:{password}".encode("utf-8")
        ).decode("ascii")
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
