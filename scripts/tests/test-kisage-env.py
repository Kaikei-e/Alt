#!/usr/bin/env python3
"""TDD & Dogfooding suite for kisage v0.1.2 & official LKAL CLI integration.

Verifies the 7 acceptance cases:
  1. Official LKAL CLI JSON normalization (`lkal json -plain`), block/labeled
     equivalence, true Quantity literals (`7.5s`, `250ms`), malformed/duplicate
     field rejection, and fail-closed binary resolution (no regex fallback).
  2. Domain A: Isolated sandbox initialization, Plan (exit 2), Apply (Converged),
     second Plan (exit 0 NoOp), deterministic dotenv output & unrepresentable
     value rejection, with zero writes to repo `.kisage/`.
  3. External managed drift fail-closed (`K203`) + explicit `explain`/`adopt`/`apply`.
  4. Sibling `/local/*` & `###` comment preservation + `K206`/`K401` Scope boundary.
  5. Conservative deletion boundary (`apply` reports `K604`; `prune` deletes).
  6. Alt semantic/schema checks (`RATE_LIMIT_EXTERNAL_API_INTERVAL = 1s` rejected).
  7. Read-only `preflight` command comparing repository-managed keys against an
     actual dotenv file (preserving unmanaged keys, catching hand-edited drift,
     missing keys, duplicate keys, and malformed lines while emitting KEY names
     only and never leaking values/secrets).

Run:
    python3 scripts/tests/test-kisage-env.py
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import pathlib
import re
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
REPO_KISAGE_DIR = ROOT / ".kisage"
COMPOSE_AUDIT_WF = ROOT / ".github" / "workflows" / "compose-audit.yaml"

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
    for candidate in (
        os.environ.get("KISAGE_BIN"),
        shutil.which("kisage"),
        str(pathlib.Path.home() / ".local" / "bin" / "kisage"),
        "/tmp/kisage",
    ):
        if candidate and pathlib.Path(candidate).is_file():
            return candidate
    raise FileNotFoundError(
        "kisage binary not found in KISAGE_BIN, PATH, ~/.local/bin/kisage, or /tmp/kisage"
    )


def find_lkal_bin() -> str:
    for candidate in (
        os.environ.get("LKAL_BIN"),
        shutil.which("lkal"),
        str(pathlib.Path.home() / ".local" / "bin" / "lkal"),
        "/tmp/lkal",
    ):
        if candidate and pathlib.Path(candidate).is_file():
            return candidate
    raise FileNotFoundError(
        "lkal binary not found in LKAL_BIN, PATH, ~/.local/bin/lkal, or /tmp/lkal"
    )


def snapshot_tree_state(path: pathlib.Path) -> dict[str, tuple[int, int]] | None:
    if not path.exists():
        return None
    state: dict[str, tuple[int, int]] = {}
    for p in sorted([path, *path.rglob("*")]):
        st = p.lstat()
        state[str(p.relative_to(path))] = (st.st_mtime_ns, st.st_size)
    return state


def run_cli_capture(kisage_env_mod, argv: list[str]) -> tuple[int, str, str]:
    out_buf = io.StringIO()
    err_buf = io.StringIO()
    with contextlib.redirect_stdout(out_buf), contextlib.redirect_stderr(err_buf):
        try:
            rc = kisage_env_mod.main(argv)
        except SystemExit as exc:
            rc = int(exc.code) if exc.code is not None else 1
    return rc, out_buf.getvalue(), err_buf.getvalue()


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
    kisage_before = snapshot_tree_state(REPO_KISAGE_DIR)

    print("1. Binary & files presence")
    check("deploy/desired-env.lkal exists", DECL_PATH.is_file(), str(DECL_PATH))
    check("scripts/kisage-env.py exists", SCRIPT.is_file(), str(SCRIPT))
    if not SCRIPT.is_file() or not DECL_PATH.is_file():
        print(f"\n{PASS} passed, {FAIL} failed (RED: missing implementation files)")
        return 1

    kisage_bin = find_kisage_bin()
    lkal_bin = find_lkal_bin()
    expected_kisage_ver = os.environ.get("KISAGE_EXPECTED_VERSION", "0.1.2")
    ver_proc = subprocess.run(
        [kisage_bin, "--version"], capture_output=True, text=True, check=False
    )
    check(
        f"kisage binary is version {expected_kisage_ver}",
        ver_proc.returncode == 0
        and ver_proc.stdout.strip() == f"kisage version {expected_kisage_ver}",
        ver_proc.stdout.strip(),
    )

    lkal_probe = subprocess.run(
        [lkal_bin, "json", "-"],
        input='@lkal "0.1"\n',
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "official lkal binary responds to `lkal json -` with $lkal 0.1",
        lkal_probe.returncode == 0
        and json.loads(lkal_probe.stdout or "{}").get("$lkal") == "0.1",
        lkal_probe.stderr.strip() or lkal_probe.stdout.strip(),
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

    print("2. Case 1: Official LKAL CLI JSON normalization, Quantity & fail-closed parser")
    # Missing lkal binary must fail closed (no handwritten fallback)
    missing_bin_failed = False
    try:
        kisage_env.parse_lkal_sections(
            '@lkal "0.1"\nupstreams {\n  AUTH_HUB_INTERNAL_URL = "https://auth-hub:8443"\n}\n',
            lkal_bin="/tmp/nonexistent-lkal-binary-for-test",
        )
    except (ValueError, FileNotFoundError):
        missing_bin_failed = True
    check(
        "parse_lkal_sections fails closed when lkal binary is missing",
        missing_bin_failed,
    )

    # Arbitrary executable in /tmp outside the explicit allow-list must be rejected
    untrusted_tmp_bin = pathlib.Path(
        f"/tmp/untrusted-lkal-probe-{os.getpid()}"
    )
    try:
        shutil.copyfile("/bin/true", untrusted_tmp_bin)
        os.chmod(untrusted_tmp_bin, 0o700)
        untrusted_rejected = False
        try:
            kisage_env._validate_lkal_binary(str(untrusted_tmp_bin))
        except ValueError:
            untrusted_rejected = True
        check(
            "_validate_lkal_binary rejects arbitrary executable in /tmp outside explicit allow-list",
            untrusted_rejected,
        )
    finally:
        untrusted_tmp_bin.unlink(missing_ok=True)

    # Block vs labeled normalization + true Quantity literal normalization (7.50s -> "7.5s")
    block_lkal = (
        '@lkal "0.1"\n'
        "rate_limits {\n"
        "  RATE_LIMIT_EXTERNAL_API_INTERVAL = 7.50s\n"
        "  RATE_LIMIT_EXTERNAL_API_BURST = 3\n"
        "  RATE_LIMIT_PREFETCH_SLOT_WAIT = 250ms\n"
        "}\n"
    )
    labeled_lkal = (
        '@lkal "0.1"\n'
        'rate_limits "RATE_LIMIT_EXTERNAL_API_INTERVAL" = 7.5s\n'
        'rate_limits "RATE_LIMIT_EXTERNAL_API_BURST" = 3\n'
        'rate_limits "RATE_LIMIT_PREFETCH_SLOT_WAIT" = 250ms\n'
    )
    try:
        parsed_block = kisage_env.parse_lkal_sections(block_lkal)
        parsed_labeled = kisage_env.parse_lkal_sections(labeled_lkal)
    except Exception as exc:
        parsed_block = {"error": str(exc)}
        parsed_labeled = {"error_labeled": str(exc)}
    check(
        "block and labeled LKAL syntax with true Quantity normalize to identical sections",
        parsed_block == parsed_labeled
        and parsed_block.get("rate_limits")
        == {
            "RATE_LIMIT_EXTERNAL_API_INTERVAL": "7.5s",
            "RATE_LIMIT_EXTERNAL_API_BURST": 3,
            "RATE_LIMIT_PREFETCH_SLOT_WAIT": "250ms",
        },
        repr(parsed_block),
    )

    # Duplicate key across block and labeled syntax must be rejected (L201)
    dup_lkal = (
        '@lkal "0.1"\n'
        "upstreams {\n"
        '  AUTH_HUB_INTERNAL_URL = "https://auth-hub:8443"\n'
        "}\n"
        'upstreams "AUTH_HUB_INTERNAL_URL" = "https://auth-hub:9443"\n'
    )
    dup_rejected = False
    try:
        kisage_env.parse_lkal_sections(dup_lkal)
    except ValueError:
        dup_rejected = True
    check(
        "duplicate key across block and labeled entries is rejected",
        dup_rejected,
    )

    # Malformed LKAL input rejected
    malformed_rejected = False
    try:
        kisage_env.parse_lkal_sections('@lkal "0.1"\nupstreams {\n  BROKEN = unquoted_bare_word\n}\n')
    except ValueError:
        malformed_rejected = True
    check("malformed LKAL bare word is rejected", malformed_rejected)

    decl_raw_text = DECL_PATH.read_text(encoding="utf-8")

    print(
        "3. Case 6: Lockstep verification & Alt semantic/schema checks (<5s floor rejected)"
    )
    verify_errors = kisage_env.verify_declaration(DECL_PATH, MANIFEST_PATH)
    check(
        "deploy/desired-env.lkal passes lockstep verification with deploy/host-prereqs.yaml",
        verify_errors == [],
        repr(verify_errors),
    )

    with tempfile.TemporaryDirectory() as tmp_check_dir:
        os.chmod(tmp_check_dir, 0o700)
        bad_decl = pathlib.Path(tmp_check_dir) / "bad-1s.lkal"
        bad_text = re.sub(
            r"RATE_LIMIT_EXTERNAL_API_INTERVAL\s*=\s*[^\n]+",
            "RATE_LIMIT_EXTERNAL_API_INTERVAL = 1s",
            decl_raw_text,
        )
        bad_decl.write_text(bad_text, encoding="utf-8")
        os.chmod(bad_decl, 0o600)
        bad_errs = kisage_env.verify_declaration(bad_decl, MANIFEST_PATH)
        check(
            "verify_declaration rejects RATE_LIMIT_EXTERNAL_API_INTERVAL = 1s (< 5s floor)",
            any("5.0s floor" in e or "RATE_LIMIT_EXTERNAL_API_INTERVAL" in e for e in bad_errs),
            repr(bad_errs),
        )

        # Unrepresentable dotenv value (embedded newline / control char) must be rejected
        newline_lkal = pathlib.Path(tmp_check_dir) / "newline.lkal"
        newline_lkal.write_text(
            '@lkal "0.1"\nlocal {\n  BAD_MULTILINE = "line1\\nINJECTED=evil"\n}\n',
            encoding="utf-8",
        )
        os.chmod(newline_lkal, 0o600)
        newline_rejected = False
        try:
            kisage_env.render_dotenv(newline_lkal)
        except ValueError:
            newline_rejected = True
        check(
            "render_dotenv rejects unrepresentable multiline/newline value against dotenv injection",
            newline_rejected,
        )

        # Labeled LKAL entry with trailing newline in key ("BAD_KEY\n") must be rejected
        newline_key_lkal = pathlib.Path(tmp_check_dir) / "newline-key.lkal"
        newline_key_lkal.write_text(
            '@lkal "0.1"\nlocal "BAD_KEY\\n" = "x"\n',
            encoding="utf-8",
        )
        os.chmod(newline_key_lkal, 0o600)
        newline_key_rejected = False
        try:
            kisage_env.render_dotenv(newline_key_lkal)
        except ValueError:
            newline_key_rejected = True
        check(
            "render_dotenv rejects labeled LKAL key with trailing newline",
            newline_key_rejected,
        )

        # Duplicate env key across sections must be rejected
        cross_dup_lkal = pathlib.Path(tmp_check_dir) / "cross-dup.lkal"
        cross_dup_lkal.write_text(
            '@lkal "0.1"\nupstreams {\n  SHARED_KEY = "https://a:8443"\n}\nlocal {\n  SHARED_KEY = "https://b:8443"\n}\n',
            encoding="utf-8",
        )
        os.chmod(cross_dup_lkal, 0o600)
        cross_dup_rejected = False
        try:
            kisage_env.render_dotenv(cross_dup_lkal)
        except ValueError:
            cross_dup_rejected = True
        check(
            "render_dotenv rejects duplicate environment key across different sections",
            cross_dup_rejected,
        )

        # Safe quoting of values containing spaces / # comment characters
        quoted_lkal = pathlib.Path(tmp_check_dir) / "quoted.lkal"
        quoted_lkal.write_text(
            '@lkal "0.1"\nlocal {\n  SPACE_VAL = "hello world #not-a-comment"\n}\n',
            encoding="utf-8",
        )
        os.chmod(quoted_lkal, 0o600)
        try:
            rendered_quoted = kisage_env.render_dotenv(quoted_lkal)
            roundtrip_env = overrides.parse_env_file(rendered_quoted)
        except Exception as exc:
            roundtrip_env = {"error": str(exc)}
        check(
            "render_dotenv quotes strings with spaces/# so parse_env_file round-trips losslessly",
            roundtrip_env.get("SPACE_VAL") == "hello world #not-a-comment",
            repr(roundtrip_env),
        )

    print(
        "4. Cases 2–5: Sandbox initialization, Plan, Apply, Sibling Preservation, Prune, Drift & Scope"
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
            "rendered dotenv contains both managed upstreams/allowlists/quantities and preserved local settings",
            parsed_env.get("AUTH_HUB_INTERNAL_URL") == "https://auth-hub:8443"
            and parsed_env.get("RATE_LIMIT_EXTERNAL_API_INTERVAL") == "7.5s"
            and parsed_env.get("RATE_LIMIT_PREFETCH_SLOT_WAIT") == "250ms"
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

        print("5. Case 7: Read-only dotenv preflight (`kisage-env.py preflight`)")
        actual_env_path = pathlib.Path(tmp_parent) / "actual.env"
        # Include unmanaged local secrets alongside converged managed keys
        secret_sentinel = "SUPER_SECRET_LIVE_VALUE_DO_NOT_LEAK_998877"
        converged_with_local_secrets = (
            dotenv_text
            + f"\n# Unmanaged host secrets\nPOSTGRES_PASSWORD={secret_sentinel}\n"
        )
        actual_env_path.write_text(converged_with_local_secrets, encoding="utf-8")
        os.chmod(actual_env_path, 0o600)

        rc_ok, out_ok, err_ok = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(decl_copy),
            ],
        )
        check(
            "preflight exits 0 when all managed keys match and unmanaged/local keys are present",
            rc_ok == 0 and secret_sentinel not in out_ok and secret_sentinel not in err_ok,
            f"rc={rc_ok} err={err_ok!r}",
        )

        # Hand-edit a managed key in the generated dotenv
        tampered_sentinel = "http://tampered-secret-upstream:8888"
        tampered_env_text = converged_with_local_secrets.replace(
            "AUTH_HUB_INTERNAL_URL=https://auth-hub:8443",
            f"AUTH_HUB_INTERNAL_URL={tampered_sentinel}",
        )
        actual_env_path.write_text(tampered_env_text, encoding="utf-8")
        rc_drift, out_drift, err_drift = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(decl_copy),
            ],
        )
        combined_drift_diag = out_drift + err_drift
        check(
            "preflight catches hand-edited managed key in dotenv and exits nonzero",
            rc_drift != 0 and "AUTH_HUB_INTERNAL_URL" in combined_drift_diag,
            f"rc={rc_drift} diag={combined_drift_diag!r}",
        )
        check(
            "preflight diagnostics report KEY names only and never leak actual or secret values",
            tampered_sentinel not in combined_drift_diag
            and "https://auth-hub:8443" not in combined_drift_diag
            and secret_sentinel not in combined_drift_diag,
            repr(combined_drift_diag),
        )

        # Missing managed key
        missing_key_text = "\n".join(
            line
            for line in converged_with_local_secrets.splitlines()
            if not line.startswith("SEARCH_INDEXER_CONNECT_URL=")
        ) + "\n"
        actual_env_path.write_text(missing_key_text, encoding="utf-8")
        rc_miss, out_miss, err_miss = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(decl_copy),
            ],
        )
        check(
            "preflight fails nonzero on missing managed key and names the key",
            rc_miss != 0 and "SEARCH_INDEXER_CONNECT_URL" in (out_miss + err_miss),
            f"rc={rc_miss}",
        )

        # Duplicate key in actual dotenv
        dup_env_text = (
            converged_with_local_secrets
            + f"POSTGRES_PASSWORD={secret_sentinel}_dup\n"
        )
        actual_env_path.write_text(dup_env_text, encoding="utf-8")
        rc_dup, out_dup, err_dup = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(decl_copy),
            ],
        )
        check(
            "preflight fails safely on duplicate dotenv key without leaking secret value",
            rc_dup != 0
            and "POSTGRES_PASSWORD" in (out_dup + err_dup)
            and secret_sentinel not in (out_dup + err_dup),
            f"rc={rc_dup}",
        )

        # Malformed line in actual dotenv
        malformed_secret = "RAW_PASTED_CREDENTIAL_WITHOUT_EQUALS_12345"
        actual_env_path.write_text(
            converged_with_local_secrets + f"{malformed_secret}\n",
            encoding="utf-8",
        )
        rc_mal, out_mal, err_mal = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(decl_copy),
            ],
        )
        check(
            "preflight fails safely on malformed dotenv line without echoing raw line content",
            rc_mal == 1
            and "malformed" in (out_mal + err_mal).lower()
            and malformed_secret not in (out_mal + err_mal),
            f"rc={rc_mal} diag={out_mal + err_mal!r}",
        )

        # Unquoted apostrophes and double-quoted backslash escapes in unmanaged secrets (no false stops)
        apostrophe_and_escaped_env = (
            converged_with_local_secrets
            + "LOCAL_APOSTROPHE=operator's-valid-secret # inline comment\n"
            + 'LOCAL_ESCAPED="val\\"with\\"quotes\\\\and\\$dollar"\n'
            + "LOCAL_SINGLE_QUOTED='literal $NOT_EXPANDED and \"quotes\"'\n"
        )
        actual_env_path.write_text(apostrophe_and_escaped_env, encoding="utf-8")
        rc_esc, out_esc, err_esc = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(decl_copy),
            ],
        )
        parsed_strict_map, parsed_strict_errs = kisage_env._parse_actual_dotenv_strict(
            apostrophe_and_escaped_env
        )
        check(
            "preflight accepts valid Compose unquoted apostrophes and escaped quoted values without false stops",
            rc_esc == 0
            and parsed_strict_errs == []
            and parsed_strict_map.get("LOCAL_APOSTROPHE") == "operator's-valid-secret"
            and parsed_strict_map.get("LOCAL_ESCAPED") == 'val"with"quotes\\and$dollar'
            and parsed_strict_map.get("LOCAL_SINGLE_QUOTED")
            == 'literal $NOT_EXPANDED and "quotes"',
            f"rc={rc_esc} errs={parsed_strict_errs!r} err_out={err_esc!r}",
        )

        # Valid Compose variable references in unmanaged keys are permitted without evaluation or false stops
        unmanaged_interp_sentinel = "UNMANAGED_REF_TARGET_SECRET_112233"
        unmanaged_interp_env = (
            converged_with_local_secrets
            + "APP_CLICKHOUSE_USER=${CLICKHOUSE_USER}\n"
            + "APP_CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}\n"
            + "APP_CLICKHOUSE_DATABASE=${CLICKHOUSE_DB:-rask_logs}\n"
            + f'RECAP_DB_DSN="postgres://recap:${{{unmanaged_interp_sentinel}}}@recap-db:5432/recap"\n'
            + "TAG_GENERATOR_SERVICE_TOKEN=${SERVICE_SECRET}\n"
        )
        actual_env_path.write_text(unmanaged_interp_env, encoding="utf-8")
        rc_unm_int, out_unm_int, err_unm_int = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(decl_copy),
            ],
        )
        check(
            "preflight permits valid Compose variable references in unmanaged keys without evaluation or leaking values",
            rc_unm_int == 0
            and err_unm_int == ""
            and unmanaged_interp_sentinel not in (out_unm_int + err_unm_int)
            and "CLICKHOUSE_USER" not in (out_unm_int + err_unm_int),
            f"rc={rc_unm_int} err={err_unm_int!r}",
        )

        # Unescaped $ interpolation in repository-managed keys is explicitly rejected (no silent misparse)
        interp_sentinel = "INTERP_SECRET_SENTINEL_554433"
        managed_interp_env = converged_with_local_secrets.replace(
            "AUTH_HUB_INTERNAL_URL=https://auth-hub:8443",
            f'AUTH_HUB_INTERNAL_URL="https://${{{interp_sentinel}}}:8443"',
        ).replace(
            "RATE_LIMIT_EXTERNAL_API_INTERVAL=7.5s",
            f"RATE_LIMIT_EXTERNAL_API_INTERVAL=${{{interp_sentinel}}}",
        )
        actual_env_path.write_text(managed_interp_env, encoding="utf-8")
        rc_int, out_int, err_int = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(decl_copy),
            ],
        )
        check(
            "preflight rejects unescaped '$' variable interpolation in managed keys with key-only diagnostics",
            rc_int == 1
            and "AUTH_HUB_INTERNAL_URL" in (out_int + err_int)
            and "RATE_LIMIT_EXTERNAL_API_INTERVAL" in (out_int + err_int)
            and interp_sentinel not in (out_int + err_int),
            f"rc={rc_int} diag={out_int + err_int!r}",
        )

        # Invalid declaration containing secret sentinels must not leak raw LKAL CLI stderr or declaration values
        actual_env_path.write_text(converged_with_local_secrets, encoding="utf-8")
        decl_sentinel_syntax = "DECL_SYNTAX_SECRET_SENTINEL_445566"
        decl_sentinel_value = "DECL_VALUE_SECRET_SENTINEL_778899"
        bad_syntax_decl = pathlib.Path(tmp_parent) / "bad-syntax-decl.lkal"
        bad_syntax_decl.write_text(
            f'@lkal "0.1"\nupstreams {{\n  BROKEN_KEY = {decl_sentinel_syntax}\n}}\n',
            encoding="utf-8",
        )
        os.chmod(bad_syntax_decl, 0o600)
        rc_badsyn, out_badsyn, err_badsyn = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(bad_syntax_decl),
            ],
        )
        bad_val_decl = pathlib.Path(tmp_parent) / "bad-val-decl.lkal"
        bad_val_decl.write_text(
            decl_copy.read_text(encoding="utf-8").replace(
                "RATE_LIMIT_EXTERNAL_API_INTERVAL = 7.5s",
                f'RATE_LIMIT_EXTERNAL_API_INTERVAL = "{decl_sentinel_value}"',
            ),
            encoding="utf-8",
        )
        os.chmod(bad_val_decl, 0o600)
        rc_badval, out_badval, err_badval = run_cli_capture(
            kisage_env,
            [
                "preflight",
                "--env-file",
                str(actual_env_path),
                "--declaration",
                str(bad_val_decl),
            ],
        )
        combined_decl_diag = out_badsyn + err_badsyn + out_badval + err_badval
        check(
            "preflight sanitizes invalid declaration failures and never leaks raw LKAL stderr or declaration values",
            rc_badsyn == 1
            and rc_badval == 1
            and decl_sentinel_syntax not in combined_decl_diag
            and decl_sentinel_value not in combined_decl_diag,
            repr(combined_decl_diag),
        )

        print(
            "6. Domain B: MCP stdio JSON-RPC dogfooding (Plan, Apply, Prune, Drift K203 + Adopt, Scope K401)"
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
        drifted_target = re.sub(
            r'(RATE_LIMIT_EXTERNAL_API_INTERVAL\s*=\s*)(?:"7\.5s"|7\.5s)',
            r"\g<1>9s",
            target_lkal.read_text(encoding="utf-8"),
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
        converged_target_text = target_lkal.read_text(encoding="utf-8")
        check(
            "MCP kisage_adopt + kisage_apply resolves K203 drift and converges back to 7.5s",
            not adopt_resps[0]["result"]["isError"]
            and not adopt_resps[1]["result"]["isError"]
            and adopt_resps[1]["result"]["structuredContent"]["summary"]["convergence"]
            == "Converged"
            and re.search(
                r"RATE_LIMIT_EXTERNAL_API_INTERVAL\s*=\s*7\.5s\b",
                converged_target_text,
            )
            is not None,
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

    print("7. Layer 3 Execution Boundary (.claude/settings.json, .mcp.json & repo .kisage isolation)")
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

    kisage_after = snapshot_tree_state(REPO_KISAGE_DIR)
    check(
        "test suite performs zero writes to repository .kisage/ state",
        kisage_before == kisage_after,
    )

    print("8. Tooling Provisioning & CI Gate Contracts")
    with tempfile.TemporaryDirectory() as tmp_make_dir:
        os.chmod(tmp_make_dir, 0o700)
        tmp_make_path = pathlib.Path(tmp_make_dir)
        sentinel_bin_dir = tmp_make_path / "bin"
        sentinel_bin_dir.mkdir(mode=0o700)
        lkal_sentinel = sentinel_bin_dir / "lkal"
        kisage_sentinel = sentinel_bin_dir / "kisage"
        lkal_sentinel.write_text("SENTINEL_LKAL_DO_NOT_OVERWRITE\n", encoding="utf-8")
        kisage_sentinel.write_text("SENTINEL_KISAGE_DO_NOT_OVERWRITE\n", encoding="utf-8")
        bad_env_for_make = tmp_make_path / "broken-for-make.env"
        bad_env_for_make.write_text(
            "*** MAKE_SYNTAX_ERROR_IF_INCLUDED ***\n", encoding="utf-8"
        )
        bad_src_dir = tmp_make_path / "nonexistent-src"

        bad_lkal_proc = subprocess.run(
            [
                "make",
                "-C",
                str(ROOT),
                "install-lkal",
                f"ENV_FILE={bad_env_for_make}",
                f"LKAL_SRC_DIR={bad_src_dir}",
                f"LKAL_INSTALL_DIR={sentinel_bin_dir}",
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        bad_kisage_proc = subprocess.run(
            [
                "make",
                "-C",
                str(ROOT),
                "install-kisage",
                f"ENV_FILE={bad_env_for_make}",
                f"LKAL_SRC_DIR={bad_src_dir}",
                f"KISAGE_SRC_DIR={bad_src_dir}",
                f"KISAGE_INSTALL_DIR={sentinel_bin_dir}",
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            "Make installers skip .env include on pure goals, fail nonzero on bad local source, and preserve existing staged binaries",
            bad_lkal_proc.returncode != 0
            and bad_kisage_proc.returncode != 0
            and "MAKE_SYNTAX_ERROR_IF_INCLUDED"
            not in (bad_lkal_proc.stderr + bad_kisage_proc.stderr)
            and lkal_sentinel.read_text(encoding="utf-8")
            == "SENTINEL_LKAL_DO_NOT_OVERWRITE\n"
            and kisage_sentinel.read_text(encoding="utf-8")
            == "SENTINEL_KISAGE_DO_NOT_OVERWRITE\n",
            f"lkal_rc={bad_lkal_proc.returncode} kisage_rc={bad_kisage_proc.returncode}",
        )

    wf_text = COMPOSE_AUDIT_WF.read_text(encoding="utf-8")
    import yaml

    wf_parsed = yaml.safe_load(wf_text)
    required_wf_paths = (
        "deploy/desired-env.lkal",
        "scripts/kisage-env.py",
        "scripts/tests/test-kisage-env.py",
        "docs/runbooks/kisage-env.md",
        "Makefile",
    )
    check(
        "compose-audit.yaml parses as valid YAML and triggers on all kisage/LKAL pilot paths (push & pull_request)",
        isinstance(wf_parsed, dict)
        and "kisage-env-invariant" in (wf_parsed.get("jobs") or {})
        and all(wf_text.count(p) >= 2 for p in required_wf_paths),
        repr({p: wf_text.count(p) for p in required_wf_paths}),
    )
    check(
        "compose-audit.yaml defines kisage-env-invariant job running verify and test-kisage-env.py with setup-go",
        "kisage-env-invariant:" in wf_text
        and "actions/setup-go@" in wf_text
        and "python3 scripts/kisage-env.py verify" in wf_text
        and "python3 scripts/tests/test-kisage-env.py" in wf_text,
    )

    print(f"\n{PASS} passed, {FAIL} failed")
    return 1 if FAIL else 0


if __name__ == "__main__":
    raise SystemExit(main())
