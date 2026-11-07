#!/usr/bin/env python3
"""Tests for scripts/publish-pact-bridge.py.

Starts an in-process HTTP stub broker on 127.0.0.1 (ephemeral port) and
exercises the manual Pact verification bridge helper across happy-path and
failure modes.

Run:
    python3 scripts/tests/test-publish-pact-bridge.py
"""

from __future__ import annotations

import base64
import http.server
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import threading
from typing import Any

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "publish-pact-bridge.py"

PASS = 0
FAIL = 0

BROKER_USER = "pact-user"
BROKER_PASS = "super-secret-broker-password-do-not-leak"
EXPECTED_AUTH = "Basic " + base64.b64encode(
    f"{BROKER_USER}:{BROKER_PASS}".encode("utf-8")
).decode("ascii")
PROVIDER_SHA = "0123456789abcdef0123456789abcdef01234567"


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}" + (f" ({detail})" if detail else ""))
        FAIL += 1


class BrokerState:
    def __init__(self, base_url: str) -> None:
        self.base_url = base_url
        self.requests: list[dict[str, Any]] = []
        self.routes: dict[tuple[str, str], tuple[int, Any]] = {}


class StubBrokerHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    state: BrokerState

    def log_message(self, format: str, *args: Any) -> None:
        return

    def _handle(self, method: str) -> None:
        length = int(self.headers.get("Content-Length", "0"))
        raw_body = self.rfile.read(length) if length > 0 else b""
        parsed_body = None
        if raw_body:
            try:
                parsed_body = json.loads(raw_body.decode("utf-8"))
            except json.JSONDecodeError:
                parsed_body = raw_body.decode("utf-8", errors="replace")

        self.state.requests.append(
            {
                "method": method,
                "path": self.path,
                "auth": self.headers.get("Authorization"),
                "content_type": self.headers.get("Content-Type"),
                "body": parsed_body,
            }
        )

        status, payload = self.state.routes.get(
            (method, self.path), (404, {"error": "not found"})
        )
        body_bytes = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/hal+json")
        self.send_header("Content-Length", str(len(body_bytes)))
        self.end_headers()
        self.wfile.write(body_bytes)

    def do_GET(self) -> None:
        self._handle("GET")

    def do_POST(self) -> None:
        self._handle("POST")


class StubBroker:
    def __init__(self) -> None:
        self._server = http.server.ThreadingHTTPServer(
            ("127.0.0.1", 0), StubBrokerHandler
        )
        host, port = self._server.server_address[:2]
        self.base_url = f"http://{host}:{port}"
        self.state = BrokerState(self.base_url)
        StubBrokerHandler.state = self.state
        self._thread = threading.Thread(
            target=self._server.serve_forever, daemon=True
        )

    def __enter__(self) -> StubBroker:
        self._thread.start()
        return self

    def __exit__(self, *exc: Any) -> None:
        self._server.shutdown()
        self._server.server_close()
        self._thread.join(timeout=5)


def make_payload(
    base_url: str,
    *,
    verifier_version: str | None = "0.1.0",
    result: str = "passed",
) -> dict[str, Any]:
    evidence: dict[str, Any] = {
        "schema": "alt.pact.evidence.v1",
        "run_id": "test-run",
        "provider": "recap-worker",
        "consumer": "alt-backend",
        "implementation": "rust-stub-replay",
        "provider_version": PROVIDER_SHA,
        "verified_at": "2026-01-01T00:00:00Z",
        "interactions_verified": 4,
        "interactions_skipped": 0,
        "result": result,
    }
    if verifier_version is not None:
        evidence["verifier_version"] = verifier_version

    return {
        "broker_base_url": base_url,
        "broker_username": BROKER_USER,
        "bridge": {
            "provider": "recap-worker",
            "consumer": "alt-backend",
            "implementation": "rust-stub-replay",
            "provider_version": PROVIDER_SHA,
            "evidence": evidence,
        },
    }


def run_helper(
    payload: dict[str, Any],
    *,
    password: str = BROKER_PASS,
    via_file: bool = False,
) -> subprocess.CompletedProcess[str]:
    env = {**os.environ, "PACT_BROKER_PASSWORD": password}
    encoded = json.dumps(payload)
    if via_file:
        with tempfile.TemporaryDirectory() as tmp:
            input_path = pathlib.Path(tmp) / "bridge-input.json"
            input_path.write_text(encoded, encoding="utf-8")
            os.chmod(input_path, 0o600)
            return subprocess.run(
                [sys.executable, str(SCRIPT), "--input", str(input_path)],
                capture_output=True,
                text=True,
                env=env,
                check=False,
            )
    return subprocess.run(
        [sys.executable, str(SCRIPT)],
        input=encoded,
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )


def assert_no_password_leak(label: str, proc: subprocess.CompletedProcess[str]) -> None:
    combined = (proc.stdout or "") + (proc.stderr or "")
    b64_token = EXPECTED_AUTH.split(" ", 1)[1]
    check(
        f"{label}: password never appears in stdout/stderr",
        BROKER_PASS not in combined and b64_token not in combined,
        combined,
    )


def main() -> int:
    print("1. Helper script presence")
    check("scripts/publish-pact-bridge.py exists", SCRIPT.is_file(), str(SCRIPT))

    versions_path = "/pacts/provider/recap-worker/consumer/alt-backend/versions"
    v1_path = "/pacts/provider/recap-worker/consumer/alt-backend/version/1.0.0"
    v2_path = "/pacts/provider/recap-worker/consumer/alt-backend/version/2.0.0"
    v3_path = "/pacts/provider/recap-worker/consumer/alt-backend/version/3.0.0"
    pub1_path = "/pacts/provider/recap-worker/consumer/alt-backend/pact-version/sha-1/verification-results"
    pub3_path = "/pacts/provider/recap-worker/consumer/alt-backend/pact-version/sha-3/verification-results"

    print("2. Happy path: request order, headers, POST bodies, 200 and 201 accepted")
    with StubBroker() as broker:
        b = broker.base_url
        broker.state.routes = {
            ("GET", versions_path): (
                200,
                {
                    "_embedded": {
                        "pacts": [
                            {"_links": {"self": {"href": f"{b}{v1_path}"}}},
                            {"_links": {"self": {"href": f"{b}{v2_path}"}}},
                            {"_links": {"self": {"href": f"{b}{v3_path}"}}},
                        ]
                    }
                },
            ),
            ("GET", v1_path): (
                200,
                {
                    "_links": {
                        "pb:publish-verification-results": {
                            "href": f"{b}{pub1_path}"
                        }
                    }
                },
            ),
            # v2 intentionally omits pb:publish-verification-results and must be skipped
            ("GET", v2_path): (200, {"_links": {}}),
            ("GET", v3_path): (
                200,
                {
                    "_links": {
                        "pb:publish-verification-results": {
                            "href": f"{b}{pub3_path}"
                        }
                    }
                },
            ),
            ("POST", pub1_path): (200, {"ok": True}),
            ("POST", pub3_path): (201, {"ok": True}),
        }

        proc = run_helper(make_payload(b))
        check(
            "exits 0 when 200 and 201 are returned by POSTs",
            proc.returncode == 0,
            proc.stderr.strip(),
        )
        assert_no_password_leak("happy path", proc)

        observed_order = [
            (r["method"], r["path"]) for r in broker.state.requests
        ]
        expected_order = [
            ("GET", versions_path),
            ("GET", v1_path),
            ("GET", v2_path),
            ("GET", v3_path),
            ("POST", pub1_path),
            ("POST", pub3_path),
        ]
        check(
            "requests are issued in exact order (list GET, all detail GETs, all publish POSTs)",
            observed_order == expected_order,
            f"got {observed_order}",
        )
        check(
            "every request carries the Basic Authorization header",
            len(broker.state.requests) == 6
            and all(
                r["auth"] == EXPECTED_AUTH for r in broker.state.requests
            ),
            repr([r["auth"] for r in broker.state.requests]),
        )

        post_requests = [
            r for r in broker.state.requests if r["method"] == "POST"
        ]
        expected_body = {
            "success": True,
            "providerApplicationVersion": PROVIDER_SHA,
            "verifiedBy": {
                "implementation": "rust-stub-replay",
                "version": "0.1.0",
            },
        }
        check(
            "POST requests set Content-Type application/json and exact verification body",
            len(post_requests) == 2
            and all(
                r["content_type"] == "application/json"
                and r["body"] == expected_body
                for r in post_requests
            ),
            repr(post_requests),
        )

        try:
            summary = json.loads(proc.stdout or "{}")
        except json.JSONDecodeError:
            summary = {}
        check(
            "stdout reports verifications_posted count as JSON",
            summary.get("verifications_posted") == 2,
            repr(proc.stdout),
        )

    print("3. Default verifier_version fallback and --input file mode")
    with StubBroker() as broker:
        b = broker.base_url
        broker.state.routes = {
            ("GET", versions_path): (
                200,
                {
                    "_embedded": {
                        "pacts": [
                            {"_links": {"self": {"href": f"{b}{v1_path}"}}},
                        ]
                    }
                },
            ),
            ("GET", v1_path): (
                200,
                {
                    "_links": {
                        "pb:publish-verification-results": {
                            "href": f"{b}{pub1_path}"
                        }
                    }
                },
            ),
            ("POST", pub1_path): (200, {"ok": True}),
        }
        proc = run_helper(
            make_payload(b, verifier_version=None), via_file=True
        )
        check(
            "--input temp file mode exits 0",
            proc.returncode == 0,
            proc.stderr.strip(),
        )
        post_requests = [
            r for r in broker.state.requests if r["method"] == "POST"
        ]
        check(
            "missing verifier_version defaults to 'unknown'",
            len(post_requests) == 1
            and post_requests[0]["body"]
            == {
                "success": True,
                "providerApplicationVersion": PROVIDER_SHA,
                "verifiedBy": {
                    "implementation": "rust-stub-replay",
                    "version": "unknown",
                },
            },
            repr(post_requests),
        )
        assert_no_password_leak("input file mode", proc)

    print("4. Failure modes: 500 on POST, empty publish list, 404 list, no publish URL resolved, detail GET error")
    with StubBroker() as broker:
        b = broker.base_url
        failing_post_url = f"{b}{pub3_path}"
        broker.state.routes = {
            ("GET", versions_path): (
                200,
                {
                    "_embedded": {
                        "pacts": [
                            {"_links": {"self": {"href": f"{b}{v1_path}"}}},
                            {"_links": {"self": {"href": f"{b}{v3_path}"}}},
                        ]
                    }
                },
            ),
            ("GET", v1_path): (
                200,
                {
                    "_links": {
                        "pb:publish-verification-results": {
                            "href": f"{b}{pub1_path}"
                        }
                    }
                },
            ),
            ("GET", v3_path): (
                200,
                {
                    "_links": {
                        "pb:publish-verification-results": {
                            "href": failing_post_url
                        }
                    }
                },
            ),
            ("POST", pub1_path): (200, {"ok": True}),
            ("POST", pub3_path): (500, {"error": "broker internal error"}),
        }
        proc = run_helper(make_payload(b))
        check(
            "500 on POST exits non-zero",
            proc.returncode != 0,
            f"rc={proc.returncode}",
        )
        check(
            "500 on POST names the failing URL and pair in stderr",
            failing_post_url in proc.stderr
            and "alt-backend" in proc.stderr
            and "recap-worker" in proc.stderr,
            proc.stderr.strip(),
        )
        assert_no_password_leak("500 on POST", proc)

    with StubBroker() as broker:
        b = broker.base_url
        broker.state.routes = {
            ("GET", versions_path): (200, {"_embedded": {"pacts": []}}),
        }
        proc = run_helper(make_payload(b))
        check(
            "empty pact versions list exits non-zero",
            proc.returncode != 0,
            f"rc={proc.returncode}",
        )
        check(
            "empty pact versions list emits 'No pact on the broker for' with pair and URL",
            "No pact on the broker for" in proc.stderr
            and "alt-backend" in proc.stderr
            and "recap-worker" in proc.stderr
            and f"{b}{versions_path}" in proc.stderr,
            proc.stderr.strip(),
        )
        assert_no_password_leak("empty list", proc)

    with StubBroker() as broker:
        b = broker.base_url
        broker.state.routes = {
            ("GET", versions_path): (404, {"error": "not found"}),
        }
        proc = run_helper(make_payload(b))
        check(
            "404 on pact versions list exits non-zero",
            proc.returncode != 0,
            f"rc={proc.returncode}",
        )
        check(
            "404 on pact versions list emits 'No pact on the broker for' with pair and URL",
            "No pact on the broker for" in proc.stderr
            and "alt-backend" in proc.stderr
            and "recap-worker" in proc.stderr
            and f"{b}{versions_path}" in proc.stderr,
            proc.stderr.strip(),
        )
        assert_no_password_leak("404 list", proc)

    with StubBroker() as broker:
        b = broker.base_url
        broker.state.routes = {
            ("GET", versions_path): (
                200,
                {
                    "_embedded": {
                        "pacts": [
                            {"_links": {"self": {"href": f"{b}{v2_path}"}}},
                        ]
                    }
                },
            ),
            ("GET", v2_path): (200, {"_links": {}}),
        }
        proc = run_helper(make_payload(b))
        check(
            "no publish-verification-results URL resolved exits non-zero",
            proc.returncode != 0,
            f"rc={proc.returncode}",
        )
        check(
            "no publish URL resolved emits 'No publish-verification-results URL resolved' with pair",
            "No publish-verification-results URL resolved" in proc.stderr
            and "alt-backend" in proc.stderr
            and "recap-worker" in proc.stderr,
            proc.stderr.strip(),
        )
        assert_no_password_leak("no publish URL", proc)

    with StubBroker() as broker:
        b = broker.base_url
        failing_detail_url = f"{b}{v1_path}"
        broker.state.routes = {
            ("GET", versions_path): (
                200,
                {
                    "_embedded": {
                        "pacts": [
                            {"_links": {"self": {"href": failing_detail_url}}},
                        ]
                    }
                },
            ),
            ("GET", v1_path): (502, {"error": "bad gateway"}),
        }
        proc = run_helper(make_payload(b))
        check(
            "non-200 on detail GET exits non-zero",
            proc.returncode != 0,
            f"rc={proc.returncode}",
        )
        check(
            "non-200 on detail GET names the failing detail URL and pair in stderr",
            failing_detail_url in proc.stderr
            and "alt-backend" in proc.stderr
            and "recap-worker" in proc.stderr,
            proc.stderr.strip(),
        )
        assert_no_password_leak("detail GET 502", proc)

    print(f"\n{PASS} passed, {FAIL} failed")
    return 1 if FAIL else 0


if __name__ == "__main__":
    raise SystemExit(main())
