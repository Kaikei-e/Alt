#!/usr/bin/env python3
"""Tests for scripts/publish-pact-bridge.py and the Pact bridge playbooks.

Starts an in-process HTTP stub broker on 127.0.0.1 (ephemeral port) and
exercises the manual Pact verification bridge helper across happy-path,
origin-normalisation, credential-handling, and failure modes, plus static
invariants on playbooks/publish-manual-verifications.yml and
playbooks/_publish-bridge.yml.

Run:
    python3 scripts/tests/test-publish-pact-bridge.py
"""

from __future__ import annotations

import base64
import http.server
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import threading
from typing import Any

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "publish-pact-bridge.py"
PLAYBOOK = ROOT / "playbooks" / "_publish-bridge.yml"
MAIN_PLAYBOOK = ROOT / "playbooks" / "publish-manual-verifications.yml"

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
    def __init__(self, base_url: str, port: int) -> None:
        self.base_url = base_url
        self.port = port
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
                "client_port": self.client_address[1],
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
        self.port = int(port)
        self.base_url = f"http://{host}:{port}"
        self.state = BrokerState(self.base_url, self.port)
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
    password: str | None = BROKER_PASS,
) -> subprocess.CompletedProcess[str]:
    env = dict(os.environ)
    if password is None:
        env.pop("PACT_BROKER_PASSWORD", None)
    else:
        env["PACT_BROKER_PASSWORD"] = password
    return subprocess.run(
        [sys.executable, str(SCRIPT)],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )


def assert_no_password_leak(
    label: str,
    proc: subprocess.CompletedProcess[str],
    extra_secrets: tuple[str, ...] = (),
) -> None:
    combined = (proc.stdout or "") + (proc.stderr or "")
    b64_token = EXPECTED_AUTH.split(" ", 1)[1]
    secrets = (BROKER_PASS, b64_token, *extra_secrets)
    check(
        f"{label}: password never appears in stdout/stderr",
        all(s not in combined for s in secrets if s),
        combined,
    )


def find_env_password_nodes(node: Any, path: str = "") -> list[str]:
    """Recursively collect node paths where an `environment` block references PACT_BROKER_PASSWORD."""
    hits: list[str] = []
    if isinstance(node, dict):
        label = str(node.get("name") or path or "<unnamed>")
        if "environment" in node:
            env_val = node["environment"]
            if "PACT_BROKER_PASSWORD" in json.dumps(env_val):
                hits.append(label)
        for k, v in node.items():
            if k != "environment":
                hits.extend(find_env_password_nodes(v, f"{path}.{k}"))
    elif isinstance(node, list):
        for idx, item in enumerate(node):
            hits.extend(find_env_password_nodes(item, f"{path}[{idx}]"))
    return hits


def load_helper_module() -> Any:
    spec = importlib.util.spec_from_file_location("publish_pact_bridge", SCRIPT)
    assert spec is not None and spec.loader is not None
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def main() -> int:
    print("1. Helper script and playbook static checks")
    check("scripts/publish-pact-bridge.py exists", SCRIPT.is_file(), str(SCRIPT))
    check("playbooks/_publish-bridge.yml exists", PLAYBOOK.is_file(), str(PLAYBOOK))
    check(
        "playbooks/publish-manual-verifications.yml exists",
        MAIN_PLAYBOOK.is_file(),
        str(MAIN_PLAYBOOK),
    )

    plays = yaml.safe_load(MAIN_PLAYBOOK.read_text(encoding="utf-8"))
    check(
        "publish-manual-verifications.yml declares connection: local on the play",
        isinstance(plays, list)
        and len(plays) >= 1
        and all(
            isinstance(p, dict) and p.get("connection") == "local"
            for p in plays
        ),
        repr(
            [
                p.get("connection") if isinstance(p, dict) else p
                for p in (plays or [])
            ]
        ),
    )

    tasks = yaml.safe_load(PLAYBOOK.read_text(encoding="utf-8"))
    check(
        "_publish-bridge.yml parses as a non-empty task list",
        isinstance(tasks, list) and len(tasks) > 0,
        repr(tasks),
    )

    env_with_pw = find_env_password_nodes(
        plays, "publish-manual-verifications.yml"
    ) + find_env_password_nodes(tasks, "_publish-bridge.yml")
    check(
        "no play, block, or task in either playbook puts PACT_BROKER_PASSWORD in environment:",
        env_with_pw == [],
        f"nodes with PACT_BROKER_PASSWORD in environment: {env_with_pw}",
    )

    helper_tasks = [
        t
        for t in (tasks or [])
        if isinstance(t, dict)
        and isinstance(t.get("ansible.builtin.command"), dict)
        and any(
            "publish-pact-bridge.py" in str(arg)
            for arg in (t["ansible.builtin.command"].get("argv") or [])
        )
    ]
    check(
        "helper command task is present and has no no_log so redacted stderr is visible on failure",
        len(helper_tasks) == 1 and "no_log" not in helper_tasks[0],
        repr(helper_tasks),
    )

    versions_path = "/pacts/provider/recap-worker/consumer/alt-backend/versions"
    v1_path = "/pacts/provider/recap-worker/consumer/alt-backend/version/1.0.0"
    v2_path = "/pacts/provider/recap-worker/consumer/alt-backend/version/2.0.0"
    v3_path = "/pacts/provider/recap-worker/consumer/alt-backend/version/3.0.0"
    pub1_path = "/pacts/provider/recap-worker/consumer/alt-backend/pact-version/sha-1/verification-results"
    pub3_path = "/pacts/provider/recap-worker/consumer/alt-backend/pact-version/sha-3/verification-results"

    print("2. Happy path: request order, fresh connections, headers, POST bodies, 200 and 201 accepted")
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
        client_ports = {r["client_port"] for r in broker.state.requests}
        check(
            "opens a fresh connection per request instead of reusing keep-alive",
            len(broker.state.requests) == 6 and len(client_ports) == 6,
            f"distinct client ports={len(client_ports)} for {len(broker.state.requests)} requests",
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

    print("3. Default verifier_version fallback and missing PACT_BROKER_PASSWORD")
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
        proc = run_helper(make_payload(b, verifier_version=None))
        check(
            "missing verifier_version exits 0",
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
        assert_no_password_leak("verifier_version default", proc)

        proc_no_pw = run_helper(make_payload(b), password=None)
        check(
            "unset PACT_BROKER_PASSWORD exits non-zero",
            proc_no_pw.returncode != 0,
            f"rc={proc_no_pw.returncode}",
        )
        check(
            "unset PACT_BROKER_PASSWORD prints a clear message naming the variable",
            "PACT_BROKER_PASSWORD" in proc_no_pw.stderr,
            proc_no_pw.stderr.strip(),
        )
        assert_no_password_leak("unset password", proc_no_pw)

    print("4. Origin normalisation: case-insensitive scheme/host, default ports, reject cross-host and userinfo")
    with StubBroker() as broker:
        port = broker.port
        # Base URL uses lower-case localhost; broker links use upper-case LOCALHOST
        lower_base = f"http://localhost:{port}"
        upper_base = f"HTTP://LOCALHOST:{port}"
        broker.state.routes = {
            ("GET", versions_path): (
                200,
                {
                    "_embedded": {
                        "pacts": [
                            {
                                "_links": {
                                    "self": {"href": f"{upper_base}{v1_path}"}
                                }
                            },
                        ]
                    }
                },
            ),
            ("GET", v1_path): (
                200,
                {
                    "_links": {
                        "pb:publish-verification-results": {
                            "href": f"{upper_base}{pub1_path}"
                        }
                    }
                },
            ),
            ("POST", pub1_path): (200, {"ok": True}),
        }
        proc_case = run_helper(make_payload(lower_base))
        check(
            "accepts upper-case scheme and host in broker links against lower-case broker_base_url",
            proc_case.returncode == 0,
            proc_case.stderr.strip(),
        )
        assert_no_password_leak("upper-case host link", proc_case)

        # Reject cross-host link
        broker.state.routes[("GET", versions_path)] = (
            200,
            {
                "_embedded": {
                    "pacts": [
                        {
                            "_links": {
                                "self": {
                                    "href": f"http://other.invalid:{port}{v1_path}"
                                }
                            }
                        }
                    ]
                }
            },
        )
        proc_cross = run_helper(make_payload(lower_base))
        check(
            "rejects broker link pointing to a different host",
            proc_cross.returncode != 0
            and f"http://other.invalid:{port}{v1_path}" in proc_cross.stderr,
            proc_cross.stderr.strip(),
        )
        assert_no_password_leak("cross-host link", proc_cross)

        # Reject userinfo in broker link even when host and port match
        userinfo_secret = "embedded-url-secret-value"
        broker.state.routes[("GET", versions_path)] = (
            200,
            {
                "_embedded": {
                    "pacts": [
                        {
                            "_links": {
                                "self": {
                                    "href": f"http://attacker:{userinfo_secret}@localhost:{port}{v1_path}"
                                }
                            }
                        }
                    ]
                }
            },
        )
        proc_userinfo = run_helper(make_payload(lower_base))
        check(
            "rejects broker link containing userinfo with error naming the pair",
            proc_userinfo.returncode != 0
            and "Refusing broker URL containing userinfo" in proc_userinfo.stderr
            and "alt-backend" in proc_userinfo.stderr
            and "recap-worker" in proc_userinfo.stderr,
            proc_userinfo.stderr.strip(),
        )
        assert_no_password_leak(
            "userinfo in link", proc_userinfo, extra_secrets=(userinfo_secret,)
        )

    mod = load_helper_module()
    origin_fn = getattr(mod, "_origin", None)
    if callable(origin_fn):
        import urllib.parse

        http_default = origin_fn(
            urllib.parse.urlsplit("http://Broker.Example.COM/pacts"),
            pair="c → p",
        )
        http_explicit = origin_fn(
            urllib.parse.urlsplit("HTTP://broker.example.com:80/pacts"),
            pair="c → p",
        )
        https_default = origin_fn(
            urllib.parse.urlsplit("https://Broker.Example.COM/pacts"),
            pair="c → p",
        )
        https_explicit = origin_fn(
            urllib.parse.urlsplit("HTTPS://broker.example.com:443/pacts"),
            pair="c → p",
        )
        check(
            "origin normalisation equates explicit default ports (:80 for http, :443 for https) and case differences",
            http_default == http_explicit == ("http", "broker.example.com", 80)
            and https_default
            == https_explicit
            == ("https", "broker.example.com", 443),
            f"http={http_default!r}/{http_explicit!r} https={https_default!r}/{https_explicit!r}",
        )
    else:
        check(
            "origin normalisation equates explicit default ports (:80 for http, :443 for https) and case differences",
            False,
            "_origin helper not defined on publish-pact-bridge.py",
        )

    print("5. Failure modes: 500 on POST, empty publish list, 404 list, no publish URL resolved, detail GET error")
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
