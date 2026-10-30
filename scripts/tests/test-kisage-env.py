#!/usr/bin/env python3
"""TDD & Dogfooding suite for kisage v0.1.1 integration (Domains A & B).

Verifies:
  1. Lockstep invariants between deploy/desired-env.lkal and deploy/host-prereqs.yaml.
  2. Domain A: Host environment LKAL reconciliation (init, plan, apply, sibling
     /local/* & comment preservation, prune of retired Created leaves, dotenv
     projection passing scripts/check-env-overrides.py).
  3. Domain B: AI Agent MCP guardrail over stdio JSON-RPC (`kisage mcp`),
     including 6-tool catalog validation, untrusted actor claim rejection,
     K203 external drift detection + kisage_explain / kisage_adopt resolution,
     K401 Scope enforcement on /local/*, and .claude/settings.json PreToolUse
     Layer 3 boundary protection.

Run:
    python3 scripts/tests/test-kisage-env.py
"""

from __future__ import annotations

import importlib.util
import json
import os
import pathlib
import shutil
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "kisage-env.py"
DECL_PATH = ROOT / "deploy" / "desired-env.lkal"
MANIFEST_PATH = ROOT / "deploy" / "host-prereqs.yaml"
CHECK_OVERRIDES = ROOT / "scripts" / "check-env-overrides.py"
CLAUDE_SETTINGS = ROOT / ".claude" / "settings.json"
MCP_CONFIG = ROOT / ".mcp.json"

PASS = 0
FAIL = 0


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}" + (f" ({detail})" if detail else ""))
        FAIL += 1


def find_kisage_bin() -> str:
    env_bin = os.environ.get("KISAGE_BIN")
    if env_bin and pathlib.Path(env_bin).is_file():
        return env_bin
    which_bin = shutil.which("kisage")
    if which_bin:
        return which_bin
    local_bin = pathlib.Path.home() / ".local" / "bin" / "kisage"
    if local_bin.is_file():
        return str(local_bin)
    raise FileNotFoundError("kisage binary not found in PATH or ~/.local/bin/kisage")


def mcp_call(
    kisage_bin: str, host_config: pathlib.Path, requests: list[dict]
) -> list[dict]:
    payload = "\n".join(json.dumps(req) for req in requests) + "\n"
    env = {**os.environ, "KISAGE_HOST_CONFIG": str(host_config)}
    proc = subprocess.run(
        [kisage_bin, "mcp"],
        input=payload,
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"kisage mcp exited {proc.returncode}: {proc.stderr}")
    responses = []
    for line in proc.stdout.splitlines():
        if line.strip():
            responses.append(json.loads(line))
    return responses


def main() -> int:
    print("1. Binary & files presence")
    check("deploy/desired-env.lkal exists", DECL_PATH.is_file(), str(DECL_PATH))
    check("scripts/kisage-env.py exists", SCRIPT.is_file(), str(SCRIPT))
    if not SCRIPT.is_file() or not DECL_PATH.is_file():
        print(f"\n{PASS} passed, {FAIL} failed (RED: missing implementation files)")
        return 1

    kisage_bin = find_kisage_bin()
    ver_proc = subprocess.run(
        [kisage_bin, "--version"], capture_output=True, text=True, check=False
    )
    check(
        "kisage binary is version 0.1.1",
        ver_proc.returncode == 0 and ver_proc.stdout.strip() == "kisage version 0.1.1",
        ver_proc.stdout.strip(),
    )

    spec = importlib.util.spec_from_file_location("kisage_env", SCRIPT)
    assert spec is not None and spec.loader is not None
    kisage_env = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(kisage_env)

    ov_spec = importlib.util.spec_from_file_location(
        "check_env_overrides", CHECK_OVERRIDES
    )
    assert ov_spec is not None and ov_spec.loader is not None
    overrides = importlib.util.module_from_spec(ov_spec)
    ov_spec.loader.exec_module(overrides)
    rules = overrides.load_rules(MANIFEST_PATH)

    print(
        "2. Lockstep verification (deploy/desired-env.lkal <-> deploy/host-prereqs.yaml)"
    )
    verify_errors = kisage_env.verify_declaration(DECL_PATH, MANIFEST_PATH)
    check(
        "deploy/desired-env.lkal passes lockstep verification with deploy/host-prereqs.yaml",
        verify_errors == [],
        repr(verify_errors),
    )

    print(
        "3. Domain A: Sandbox initialization, Plan, Apply, Sibling Preservation & Dotenv Projection"
    )
    with tempfile.TemporaryDirectory() as tmp_parent:
        os.chmod(tmp_parent, 0o700)
        sandbox = pathlib.Path(tmp_parent) / "kisage-sandbox"
        decl_copy = pathlib.Path(tmp_parent) / "desired-env.lkal"
        shutil.copyfile(DECL_PATH, decl_copy)
        os.chmod(decl_copy, 0o600)

        rc = kisage_env.main(
            [
                "init",
                "--sandbox-dir",
                str(sandbox),
                "--declaration",
                str(decl_copy),
                "--kisage-bin",
                kisage_bin,
            ]
        )
        check("kisage-env.py init exits 0", rc == 0, f"rc={rc}")

        host_json = sandbox / "host.json"
        target_lkal = sandbox / "host-env.lkal"
        check(
            "host.json and host-env.lkal have 0600 permissions",
            (host_json.stat().st_mode & 0o777) == 0o600
            and (target_lkal.stat().st_mode & 0o777) == 0o600,
        )

        # Customize local operator leaves and comment before apply
        initial_target = target_lkal.read_text(encoding="utf-8")
        custom_target = initial_target.replace(
            'KRATOS_COOKIE_DOMAIN = ""',
            '### Operator custom domain\n  KRATOS_COOKIE_DOMAIN = ".alt.local.example"',
        )
        target_lkal.write_text(custom_target, encoding="utf-8")

        env = {**os.environ, "KISAGE_HOST_CONFIG": str(host_json)}
        plan_pre = subprocess.run(
            [kisage_bin, "plan", str(decl_copy), "-detailed-exitcode", "-json"],
            capture_output=True,
            text=True,
            env=env,
            check=False,
        )
        check(
            "kisage plan -detailed-exitcode returns 2 (diff present) before apply",
            plan_pre.returncode == 2,
        )

        apply_res = subprocess.run(
            [kisage_bin, "apply", str(decl_copy), "-json"],
            capture_output=True,
            text=True,
            env=env,
            check=False,
        )
        apply_doc = json.loads(apply_res.stdout)
        check(
            "kisage apply exits 0 with Completed execution and Converged state",
            apply_res.returncode == 0
            and apply_doc["summary"]["execution"] == "Completed"
            and apply_doc["summary"]["convergence"] == "Converged",
            repr(apply_doc.get("summary")),
        )

        after_apply_text = target_lkal.read_text(encoding="utf-8")
        check(
            "preserves undeclared /local/KRATOS_COOKIE_DOMAIN value and ### comment across apply",
            'KRATOS_COOKIE_DOMAIN = ".alt.local.example"' in after_apply_text
            and "### Operator custom domain" in after_apply_text,
        )

        plan_post = subprocess.run(
            [kisage_bin, "plan", str(decl_copy), "-detailed-exitcode", "-json"],
            capture_output=True,
            text=True,
            env=env,
            check=False,
        )
        check(
            "kisage plan -detailed-exitcode returns 0 (NoOp) immediately after apply",
            plan_post.returncode == 0,
        )

        dotenv_text = kisage_env.render_dotenv(target_lkal)
        parsed_env = overrides.parse_env_file(dotenv_text)
        check(
            "rendered dotenv contains both managed upstreams/allowlists and preserved local settings",
            parsed_env.get("AUTH_HUB_INTERNAL_URL") == "https://auth-hub:8443"
            and parsed_env.get("KRATOS_COOKIE_DOMAIN") == ".alt.local.example"
            and "alt-backend"
            in parsed_env.get("AUTH_HUB_MTLS_ALLOWED_PEERS", "").split(","),
            repr(parsed_env),
        )
        check(
            "rendered dotenv trips zero errors or warnings in scripts/check-env-overrides.py",
            overrides.evaluate(parsed_env, rules) == [],
            repr(overrides.evaluate(parsed_env, rules)),
        )

        print(
            "4. Domain B: MCP stdio JSON-RPC dogfooding (Plan, Apply, Prune, Drift K203 + Adopt, Scope K401)"
        )
        mcp_resps = mcp_call(
            kisage_bin,
            host_json,
            [
                {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}},
                {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}},
                {
                    "jsonrpc": "2.0",
                    "id": 3,
                    "method": "tools/call",
                    "params": {
                        "name": "kisage_plan",
                        "arguments": {
                            "file": str(decl_copy),
                            "detailedExitCode": True,
                            "actor": "spoofed-untrusted-actor",
                        },
                    },
                },
            ],
        )
        tools_list = [t["name"] for t in mcp_resps[1]["result"]["tools"]]
        check(
            "MCP tools/list exposes all 6 normative tools",
            tools_list
            == [
                "kisage_plan",
                "kisage_explain",
                "kisage_apply",
                "kisage_prune",
                "kisage_adopt",
                "kisage_release",
            ],
            repr(tools_list),
        )
        plan_call_res = mcp_resps[2]["result"]
        check(
            "MCP kisage_plan ignores untrusted actor claim and reports Converged",
            not plan_call_res["isError"]
            and plan_call_res["structuredContent"]["summary"]["convergence"]
            == "Converged",
            repr(plan_call_res),
        )

        # Test conservative deletion: remove KNOWLEDGE_HOME_ENABLE_SUPERSEDE_UX from desired declaration
        decl_text = decl_copy.read_text(encoding="utf-8")
        retired_decl_text = (
            "\n".join(
                line
                for line in decl_text.splitlines()
                if "KNOWLEDGE_HOME_ENABLE_SUPERSEDE_UX" not in line
            )
            + "\n"
        )
        decl_copy.write_text(retired_decl_text, encoding="utf-8")

        retire_resps = mcp_call(
            kisage_bin,
            host_json,
            [
                {
                    "jsonrpc": "2.0",
                    "id": 10,
                    "method": "tools/call",
                    "params": {
                        "name": "kisage_apply",
                        "arguments": {"file": str(decl_copy)},
                    },
                },
                {
                    "jsonrpc": "2.0",
                    "id": 11,
                    "method": "tools/call",
                    "params": {
                        "name": "kisage_prune",
                        "arguments": {"file": str(decl_copy)},
                    },
                },
            ],
        )
        apply_retire_doc = retire_resps[0]["result"]["structuredContent"]
        prune_retire_doc = retire_resps[1]["result"]["structuredContent"]
        check(
            "MCP kisage_apply never deletes retired leaf (reports NotConverged / K604)",
            apply_retire_doc["summary"]["convergence"] == "NotConverged"
            and any(
                d["code"] == "K604" for d in apply_retire_doc["summary"]["diagnostics"]
            ),
            repr(apply_retire_doc["summary"]),
        )
        check(
            "MCP kisage_prune deletes retired Origin=Created leaf and reaches Converged",
            prune_retire_doc["summary"]["execution"] == "Completed"
            and prune_retire_doc["summary"]["convergence"] == "Converged"
            and "KNOWLEDGE_HOME_ENABLE_SUPERSEDE_UX"
            not in target_lkal.read_text(encoding="utf-8"),
            repr(prune_retire_doc["summary"]),
        )

        # Simulate external drift on /rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL
        drifted_target = target_lkal.read_text(encoding="utf-8").replace(
            '"7.5s"',
            '"9s"',
        )
        target_lkal.write_text(drifted_target, encoding="utf-8")

        drift_resps = mcp_call(
            kisage_bin,
            host_json,
            [
                {
                    "jsonrpc": "2.0",
                    "id": 20,
                    "method": "tools/call",
                    "params": {
                        "name": "kisage_apply",
                        "arguments": {"file": str(decl_copy)},
                    },
                },
                {
                    "jsonrpc": "2.0",
                    "id": 21,
                    "method": "tools/call",
                    "params": {
                        "name": "kisage_explain",
                        "arguments": {
                            "file": str(decl_copy),
                            "path": "/rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL",
                        },
                    },
                },
            ],
        )
        drift_apply_doc = drift_resps[0]["result"]["structuredContent"]
        explain_doc = drift_resps[1]["result"]["structuredContent"]
        check(
            "MCP kisage_apply halts fail-closed with K203 on external drift",
            drift_resps[0]["result"]["isError"]
            and drift_apply_doc["summary"]["execution"] == "Blocked"
            and any(
                d["code"] == "K203" for d in drift_apply_doc["summary"]["diagnostics"]
            ),
            repr(drift_apply_doc["summary"]),
        )

        path_explain = explain_doc["explanation"]
        token = path_explain["conditionToken"]
        rev = path_explain["ownerRevision"]
        adopt_resps = mcp_call(
            kisage_bin,
            host_json,
            [
                {
                    "jsonrpc": "2.0",
                    "id": 22,
                    "method": "tools/call",
                    "params": {
                        "name": "kisage_adopt",
                        "arguments": {
                            "file": str(decl_copy),
                            "path": "/rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL",
                            "expect": token,
                            "ownerRevision": rev,
                        },
                    },
                },
                {
                    "jsonrpc": "2.0",
                    "id": 23,
                    "method": "tools/call",
                    "params": {
                        "name": "kisage_apply",
                        "arguments": {"file": str(decl_copy)},
                    },
                },
            ],
        )
        check(
            "MCP kisage_adopt + kisage_apply resolves K203 drift and converges back to 7.5s",
            not adopt_resps[0]["result"]["isError"]
            and not adopt_resps[1]["result"]["isError"]
            and adopt_resps[1]["result"]["structuredContent"]["summary"]["convergence"]
            == "Converged"
            and '"7.5s"' in target_lkal.read_text(encoding="utf-8"),
        )

        # Test UnownedConflict (K206) + Scope boundary (K401 PolicyDenied) on /local/*
        forbidden_decl = (
            retired_decl_text
            + '\nlocal {\n  KRATOS_COOKIE_DOMAIN = ".hijacked.example"\n  INJECTED_LOCAL_KEY = "blocked"\n}\n'
        )
        decl_copy.write_text(forbidden_decl, encoding="utf-8")
        scope_resps = mcp_call(
            kisage_bin,
            host_json,
            [
                {
                    "jsonrpc": "2.0",
                    "id": 30,
                    "method": "tools/call",
                    "params": {
                        "name": "kisage_apply",
                        "arguments": {"file": str(decl_copy)},
                    },
                }
            ],
        )
        scope_doc = scope_resps[0]["result"]["structuredContent"]
        diag_codes = {d["code"] for d in scope_doc["summary"]["diagnostics"]}
        check(
            "MCP kisage_apply blocks existing /local/* overwrite (K206) and out-of-scope /local/* creation (K401)",
            scope_resps[0]["result"]["isError"]
            and scope_doc["summary"]["execution"] == "Blocked"
            and {"K206", "K401"} <= diag_codes
            and 'KRATOS_COOKIE_DOMAIN = ".alt.local.example"'
            in target_lkal.read_text(encoding="utf-8")
            and "INJECTED_LOCAL_KEY" not in target_lkal.read_text(encoding="utf-8"),
            repr(scope_doc["summary"]),
        )

    print("5. Layer 3 Execution Boundary (.claude/settings.json & .mcp.json)")
    settings_text = CLAUDE_SETTINGS.read_text(encoding="utf-8")
    check(
        ".claude/settings.json PreToolUse blocks direct Edit/Write to .kisage/*",
        ".kisage/*" in settings_text,
    )
    if MCP_CONFIG.is_file():
        mcp_cfg = json.loads(MCP_CONFIG.read_text(encoding="utf-8"))
        kisage_srv = (mcp_cfg.get("mcpServers") or {}).get("kisage") or {}
        check(
            ".mcp.json registers kisage MCP server with KISAGE_HOST_CONFIG",
            kisage_srv.get("args") == ["mcp"]
            and "KISAGE_HOST_CONFIG" in (kisage_srv.get("env") or {}),
            repr(kisage_srv),
        )

    print(f"\n{PASS} passed, {FAIL} failed")
    return 1 if FAIL else 0


if __name__ == "__main__":
    raise SystemExit(main())
