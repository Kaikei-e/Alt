#!/usr/bin/env python3
"""Tests for scripts/compose-port-audit.py port binding and overlay audit logic.

Run:
    python3 scripts/tests/test-compose-port-audit.py
    python3 -m pytest scripts/tests/test-compose-port-audit.py -q
"""

from __future__ import annotations

import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

spec = importlib.util.spec_from_file_location("port_audit", SCRIPTS / "compose-port-audit.py")
assert spec is not None
assert spec.loader is not None
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()
WILDCARD_IPV4 = "0.0.0.0"  # noqa: S104 - test fixture value


def check(name: str, condition: bool) -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        sys.stdout.write(f"  FAIL  {name}\n")
        RESULTS.failed += 1


sys.stdout.write("is_loopback\n")
check("127.0.0.1 is loopback", audit.is_loopback("127.0.0.1"))
check("127.0.0.2 is loopback", audit.is_loopback("127.0.0.2"))
check("::1 is loopback", audit.is_loopback("::1"))
check("0.0.0.0 is not loopback", not audit.is_loopback(WILDCARD_IPV4))
check(":: is not loopback", not audit.is_loopback("::"))
check("public IP is not loopback", not audit.is_loopback("192.168.1.1"))
check("invalid IP is not loopback", not audit.is_loopback("invalid-host"))


sys.stdout.write("violations (base stack port binding rules)\n")
check(
    "loopback dict port is accepted",
    audit.violations(
        {
            "services": {
                "web": {"ports": [{"host_ip": "127.0.0.1", "target": 8080, "published": "8080"}]}
            }
        }
    )
    == [],
)
check(
    "0.0.0.0 dict port is flagged",
    len(
        audit.violations(
            {
                "services": {
                    "web": {
                        "ports": [{"host_ip": WILDCARD_IPV4, "target": 8080, "published": "8080"}]
                    }
                }
            }
        )
    )
    == 1,
)
check(
    "missing host_ip defaults to 0.0.0.0 and is flagged",
    len(audit.violations({"services": {"web": {"ports": [{"target": 8080, "published": "8080"}]}}}))
    == 1,
)
check(
    "edge allowlist (plecto-proxy:8443) on 0.0.0.0 is permitted",
    audit.violations(
        {
            "services": {
                "plecto-proxy": {
                    "ports": [{"host_ip": WILDCARD_IPV4, "target": 8443, "published": "80"}]
                }
            }
        }
    )
    == [],
)
check(
    "pact-broker allowlist (pact-broker:9292) on non-loopback IP is permitted",
    audit.violations(
        {
            "services": {
                "pact-broker": {
                    "ports": [{"host_ip": "100.64.0.1", "target": 9292, "published": "9292"}]
                }
            }
        }
    )
    == [],
)
check(
    "short syntax loopback string is accepted",
    audit.violations({"services": {"web": {"ports": ["127.0.0.1:9000:9000"]}}}) == [],
)
check(
    "short syntax 0.0.0.0 string is flagged",
    len(audit.violations({"services": {"web": {"ports": ["9000:9000"]}}})) == 1,
)
check(
    "grafana loopback IPv4 (127.0.0.1) is accepted",
    audit.violations({"services": {"grafana": {"ports": ["127.0.0.1:3001:3000"]}}}) == [],
)
check(
    "grafana loopback IPv6 (::1) is accepted",
    audit.violations(
        {
            "services": {
                "grafana": {"ports": [{"host_ip": "::1", "target": 3000, "published": "3001"}]}
            }
        }
    )
    == [],
)
check(
    "grafana published on 0.0.0.0 is flagged",
    len(
        audit.violations(
            {
                "services": {
                    "grafana": {
                        "ports": [{"host_ip": WILDCARD_IPV4, "target": 3000, "published": "3001"}]
                    }
                }
            }
        )
    )
    == 1,
)
check(
    "grafana published on IPv6 wildcard :: is flagged",
    len(
        audit.violations(
            {
                "services": {
                    "grafana": {"ports": [{"host_ip": "::", "target": 3000, "published": "3001"}]}
                }
            }
        )
    )
    == 1,
)
check(
    "grafana short syntax 0.0.0.0 is flagged",
    len(audit.violations({"services": {"grafana": {"ports": ["3001:3000"]}}})) == 1,
)


sys.stdout.write("overlay allowlist policy\n")
EXPECTED_AUGUR_VIOLATIONS = 2
augur_cfg = {
    "services": {
        "knowledge-augur": {
            "ports": [{"host_ip": WILDCARD_IPV4, "target": 11434, "published": "11435"}]
        },
        "knowledge-embedder": {
            "ports": [{"host_ip": WILDCARD_IPV4, "target": 11434, "published": "11436"}]
        },
    }
}
check(
    "compose.augur.yaml ports are permitted under overlay allowlist",
    audit.violations(augur_cfg, overlay_path=pathlib.Path("compose.augur.yaml")) == [],
)
check(
    "compose.augur.yaml ports without overlay context are flagged as violations",
    len(audit.violations(augur_cfg, overlay_path=None)) == EXPECTED_AUGUR_VIOLATIONS,
)

augur_unallowlisted_cfg = {
    "services": {
        "knowledge-augur": {
            "ports": [{"host_ip": WILDCARD_IPV4, "target": 11434, "published": "11435"}]
        },
        "unauthorized-worker": {
            "ports": [{"host_ip": WILDCARD_IPV4, "target": 8888, "published": "8888"}]
        },
    }
}
augur_unallowlisted_violations = audit.violations(
    augur_unallowlisted_cfg, overlay_path=pathlib.Path("compose.augur.yaml")
)
check(
    "non-allowlisted service in compose.augur.yaml is flagged",
    len(augur_unallowlisted_violations) == 1
    and "unauthorized-worker" in augur_unallowlisted_violations[0],
)

dev_cfg = {
    "services": {
        "mock-auth": {
            "ports": [
                {"host_ip": WILDCARD_IPV4, "target": 4001, "published": "4001"},
                {"host_ip": WILDCARD_IPV4, "target": 4002, "published": "4002"},
            ]
        },
        "alt-backend": {"ports": [{"host_ip": WILDCARD_IPV4, "target": 9000, "published": "9000"}]},
    }
}
check(
    "compose/dev.yaml allowlisted services are accepted",
    audit.violations(dev_cfg, overlay_path=pathlib.Path("compose/dev.yaml")) == [],
)
check(
    "strict repo-relative path lookup: compose/dev.yaml matches",
    len(audit.get_overlay_allowlist(pathlib.Path("compose/dev.yaml"))) > 0,
)
check(
    "strict repo-relative path lookup: dev.yaml does not match (no basename fallback)",
    audit.get_overlay_allowlist(pathlib.Path("dev.yaml")) == {},
)
check(
    "dead allowlist entries removed: compose.dev.yaml has no allowlist entry",
    audit.get_overlay_allowlist(pathlib.Path("compose.dev.yaml")) == {},
)
check(
    "dead allowlist entries removed: compose/compose.dev.yaml has no allowlist entry",
    audit.get_overlay_allowlist(pathlib.Path("compose/compose.dev.yaml")) == {},
)


sys.stdout.write("zero_publish_violations\n")
check(
    "alt-data-hub without ports is accepted in base stack",
    audit.zero_publish_violations({"services": {"alt-data-hub": {"ports": []}}}, is_overlay=False)
    == [],
)
check(
    "alt-data-hub publishing loopback port is flagged",
    len(
        audit.zero_publish_violations(
            {
                "services": {
                    "alt-data-hub": {
                        "ports": [{"host_ip": "127.0.0.1", "target": 9443, "published": "9443"}]
                    }
                }
            },
            is_overlay=False,
        )
    )
    == 1,
)
check(
    "missing alt-data-hub in base stack is flagged (prevents silent rename)",
    len(audit.zero_publish_violations({"services": {"other-service": {}}}, is_overlay=False)) == 1,
)
check(
    "missing alt-data-hub in overlay stack is permitted",
    audit.zero_publish_violations({"services": {"other-service": {}}}, is_overlay=True) == [],
)
check(
    "alt-data-hub defined in overlay with published port is flagged",
    len(
        audit.zero_publish_violations(
            {"services": {"alt-data-hub": {"ports": ["127.0.0.1:9443:9443"]}}}, is_overlay=True
        )
    )
    == 1,
)


sys.stdout.write("non-standalone overlay skip handling\n")
orig_resolved_config = audit.resolved_config
try:

    def fake_resolved_config(compose_file: pathlib.Path) -> dict:
        if "failing-overlay.yaml" in str(compose_file):
            err_msg = (
                "service depends on undefined network: alt-network\nsecond line of docker error"
            )
            raise audit.ComposeConfigError(err_msg, 1)
        return {"services": {}}

    audit.resolved_config = fake_resolved_config

    failed, ports = audit.audit_file(pathlib.Path("compose/failing-overlay.yaml"), is_overlay=True)
    check("audit_file skips non-standalone overlay without failing", not failed and ports == 0)

    base_raised_system_exit = False
    try:
        audit.audit_file(pathlib.Path("compose/failing-overlay.yaml"), is_overlay=False)
    except SystemExit:
        base_raised_system_exit = True
    check(
        "audit_file raises SystemExit when base stack config resolution fails",
        base_raised_system_exit,
    )
finally:
    audit.resolved_config = orig_resolved_config


sys.stdout.write("real repository overlay discovery\n")
repo_overlays = audit.find_overlay_files(ROOT)
repo_overlay_names = {p.name for p in repo_overlays}
check(
    "real repo discovers compose.augur.yaml",
    "compose.augur.yaml" in repo_overlay_names,
)
check(
    "real repo does not list compose.yaml as overlay",
    "compose.yaml" not in repo_overlay_names,
)
check(
    "real repo does not list base.yaml as overlay",
    "base.yaml" not in repo_overlay_names,
)


sys.stdout.write("repo-root staging support\n")

with tempfile.TemporaryDirectory() as _stage_dir:
    _stage = pathlib.Path(_stage_dir)
    (_stage / ".env").write_text("DOCKER_GROUP_ID=0\n", encoding="utf-8")
    fake_proc = subprocess.CompletedProcess(
        args=[], returncode=0, stdout=json.dumps({"services": {}}), stderr=""
    )
    with mock.patch.object(audit.subprocess, "run", return_value=fake_proc) as mocked_run:
        audit.resolved_config(_stage / "compose" / "compose.yaml", repo_root=_stage)
    argv = mocked_run.call_args.args[0]
    check(
        "resolved_config uses repo_root/.env and cwd=repo_root without --no-env-resolution",
        argv[:4] == ["docker", "compose", "--env-file", str(_stage / ".env")]
        and "--no-env-resolution" not in argv
        and mocked_run.call_args.kwargs.get("cwd") == _stage,
    )
    check(
        "relative_path_str preserves staged symlink path relative to repo_root",
        audit.relative_path_str(_stage / "compose.augur.yaml", repo_root=_stage)
        == "compose.augur.yaml",
    )


sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
sys.exit(1 if RESULTS.failed else 0)
