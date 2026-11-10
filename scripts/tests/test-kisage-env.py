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
    python3 scripts/tests/test-kisage-env.py --audit-host-config
"""

from __future__ import annotations

import argparse
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


def check_lkal_cli_gates(kisage_env) -> None:
    from unittest import mock

    check(
        "_sanitize_lkal_cli_error and stderr regex scrapers are removed",
        not hasattr(kisage_env, "_sanitize_lkal_cli_error")
        and not hasattr(kisage_env, "_LKAL_DIAG_CODE_RE")
        and not hasattr(kisage_env, "_LKAL_DIAG_LOC_RE"),
    )

    secret_token = "SECRET_BARE_TOKEN_887766"
    broken_lkal = (
        '@lkal "0.1"\n'
        "upstreams {\n"
        f"  BROKEN_KEY = {secret_token}\n"
        "}\n"
    )
    broken_err = ""
    try:
        kisage_env.parse_lkal_sections(broken_lkal)
    except ValueError as exc:
        broken_err = str(exc)
    check(
        "parse_lkal_sections extracts L102 and 3:16 from --json diagnostics without leaking token or message/hint",
        "--json" in broken_err
        and "L102" in broken_err
        and "3:16" in broken_err
        and secret_token not in broken_err
        and "bare word" not in broken_err.lower()
        and "enum" not in broken_err.lower(),
        repr(broken_err),
    )

    raw_stderr_sentinel = "RAW_STDERR_SECRET_SENTINEL_334455"
    unparseable_cases = [
        f"-:3:16: L102 bare word {raw_stderr_sentinel}\n",
        "",
        "not-json",
        "[]",
        '{"code": "L102", "line": 3, "col": 16}',
        '[{"code": "BAD", "line": 3, "col": 16}]',
        '[{"code": "L102", "line": "3", "col": 16}]',
    ]
    unparseable_ok = True
    unparseable_detail = ""
    for bad_stderr in unparseable_cases:
        fake_proc = subprocess.CompletedProcess(
            args=["lkal", "json", "-plain", "--json", "-"],
            returncode=1,
            stdout="",
            stderr=bad_stderr,
        )
        with mock.patch.object(
            kisage_env.subprocess, "run", return_value=fake_proc
        ) as run_mock:
            got_err = ""
            try:
                kisage_env.parse_lkal_sections('@lkal "0.1"\n')
            except ValueError as exc:
                got_err = str(exc)
            called_args = run_mock.call_args[0][0] if run_mock.call_args else []
            if (
                "--json" not in called_args
                or "-plain" not in called_args
                or "unparseable diagnostic" not in got_err
                or "L102" in got_err
                or "3:16" in got_err
                or raw_stderr_sentinel in got_err
            ):
                unparseable_ok = False
                unparseable_detail = f"args={called_args!r} err={got_err!r}"
                break
    check(
        "parse_lkal_sections invokes lkal with --json and -plain and fails closed without regex fallback on non-JSON or malformed stderr",
        unparseable_ok,
        unparseable_detail,
    )

    fmt_fn = getattr(kisage_env, "check_lkal_format", None)
    decl_fmt_errs = fmt_fn(DECL_PATH) if callable(fmt_fn) else ["missing check_lkal_format"]
    check(
        "deploy/desired-env.lkal is in canonical lkal fmt format",
        callable(fmt_fn) and decl_fmt_errs == [],
        repr(decl_fmt_errs),
    )

    with tempfile.TemporaryDirectory() as tmp_fmt_dir:
        os.chmod(tmp_fmt_dir, 0o700)
        tmp_fmt_path = pathlib.Path(tmp_fmt_dir)

        lkal_bin = find_lkal_bin()
        fmt_proc = subprocess.run(
            [lkal_bin, "fmt", str(DECL_PATH)],
            capture_output=True,
            text=True,
            check=True,
        )
        canonical_text = fmt_proc.stdout
        canonical_file = tmp_fmt_path / "canonical-desired-env.lkal"
        canonical_file.write_text(canonical_text, encoding="utf-8")
        os.chmod(canonical_file, 0o600)

        non_canonical_text = canonical_text.replace(
            '    "acolyte-orchestrator",\n',
            '    "acolyte-orchestrator"\n',
            1,
        )
        non_canonical_file = tmp_fmt_path / "non-canonical-desired-env.lkal"
        non_canonical_file.write_text(non_canonical_text, encoding="utf-8")
        os.chmod(non_canonical_file, 0o600)

        canon_fmt_errs = (
            fmt_fn(canonical_file) if callable(fmt_fn) else ["missing check_lkal_format"]
        )
        canon_rc, _canon_out, canon_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(canonical_file)]
        )
        check(
            "canonical declaration copy passes format gate and kisage-env.py verify",
            callable(fmt_fn) and canon_fmt_errs == [] and canon_rc == 0,
            f"fmt_errs={canon_fmt_errs!r} rc={canon_rc} err={canon_err!r}",
        )

        non_canon_fmt_errs = (
            fmt_fn(non_canonical_file) if callable(fmt_fn) else []
        )
        non_canon_rc, _non_canon_out, non_canon_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(non_canonical_file)]
        )
        check(
            "non-canonical declaration copy fails format gate and kisage-env.py verify naming the file",
            callable(fmt_fn)
            and len(non_canon_fmt_errs) == 1
            and non_canonical_file.name in non_canon_fmt_errs[0]
            and non_canon_rc == 1
            and non_canonical_file.name in non_canon_err,
            f"fmt_errs={non_canon_fmt_errs!r} rc={non_canon_rc} err={non_canon_err!r}",
        )

        broken_decl_file = tmp_fmt_path / "broken-desired-env.lkal"
        broken_decl_file.write_text(broken_lkal, encoding="utf-8")
        os.chmod(broken_decl_file, 0o600)
        broken_fmt_errs = fmt_fn(broken_decl_file) if callable(fmt_fn) else []
        unexpected_fmt_proc = subprocess.CompletedProcess(
            args=["lkal", "fmt", "-l", str(canonical_file)],
            returncode=0,
            stdout=f"unexpected-output-{raw_stderr_sentinel}\n",
            stderr="",
        )
        with mock.patch.object(
            kisage_env.subprocess, "run", return_value=unexpected_fmt_proc
        ):
            unexpected_fmt_errs = fmt_fn(canonical_file) if callable(fmt_fn) else []
        broken_rc, broken_out, broken_verify_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(broken_decl_file)]
        )
        broken_combined = broken_out + broken_verify_err
        check(
            "syntactically broken declaration temp file and unexpected lkal fmt -l output fail closed without leaking tokens",
            callable(fmt_fn)
            and len(broken_fmt_errs) == 1
            and broken_decl_file.name in broken_fmt_errs[0]
            and secret_token not in broken_fmt_errs[0]
            and len(unexpected_fmt_errs) == 1
            and canonical_file.name in unexpected_fmt_errs[0]
            and raw_stderr_sentinel not in unexpected_fmt_errs[0]
            and broken_rc == 1
            and "--json" in broken_verify_err
            and "L102" in broken_verify_err
            and "3:16" in broken_verify_err
            and secret_token not in broken_combined,
            repr((broken_fmt_errs, unexpected_fmt_errs, broken_combined)),
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


def run_compose_lockstep_and_preflight_checks(kisage_env) -> None:
    print("9. Compose Lockstep & Absent-Default Preflight Contracts")

    def _verify(*args, **kwargs):
        try:
            return kisage_env.verify_declaration(*args, **kwargs)
        except TypeError as exc:
            return [f"__type_error__:{exc}"]

    def _preflight(*args, **kwargs):
        try:
            return kisage_env.preflight_dotenv(*args, **kwargs)
        except TypeError as exc:
            return [f"__type_error__:{exc}"]

    with tempfile.TemporaryDirectory() as tmp_dir:
        tmp = pathlib.Path(tmp_dir)
        fx_manifest = tmp / "host-prereqs.yaml"
        fx_manifest.write_text(
            MANIFEST_PATH.read_text(encoding="utf-8"), encoding="utf-8"
        )
        sections = kisage_env.parse_lkal_sections(
            DECL_PATH.read_text(encoding="utf-8")
        )
        allowlists = sections["mtls_allowlists"]
        allowlist_lkal_lines = [
            f"  {k} = [{', '.join(json.dumps(p) for p in peers)}]"
            for k, peers in sorted(allowlists.items())
        ]
        allowlist_compose_lines = [
            f"      - {k}=${{{k}:-{','.join(peers)}}}"
            for k, peers in sorted(allowlists.items())
        ]

        fx_compose_dir = tmp / "compose"
        fx_compose_dir.mkdir()
        fx_compose_root = fx_compose_dir / "compose.yaml"
        fx_compose_root.write_text(
            "include:\n  - sub.yaml\n",
            encoding="utf-8",
        )
        fx_sub_yaml = fx_compose_dir / "sub.yaml"

        def write_sub_compose(
            auth_default: str = "https://auth-hub:8443",
            second_auth_default: str | None = None,
            second_auth_raw: str | None = None,
            pinned_search: str = "https://search-indexer:9443",
            extra_app_env_lines: list[str] | None = None,
        ) -> None:
            extra_svc = ""
            if second_auth_raw is not None:
                extra_svc = (
                    "  second-svc:\n"
                    "    image: example/second:latest\n"
                    "    environment:\n"
                    f"      - {second_auth_raw}\n"
                )
            elif second_auth_default is not None:
                extra_svc = (
                    "  second-svc:\n"
                    "    image: example/second:latest\n"
                    "    environment:\n"
                    f"      - AUTH_HUB_INTERNAL_URL=${{AUTH_HUB_INTERNAL_URL:-{second_auth_default}}}\n"
                )
            extra_app = (
                "".join(f"      - {line}\n" for line in extra_app_env_lines)
                if extra_app_env_lines
                else ""
            )
            fx_sub_yaml.write_text(
                "services:\n"
                "  app:\n"
                "    image: example/app:latest\n"
                "    environment:\n"
                f"      - AUTH_HUB_INTERNAL_URL=${{AUTH_HUB_INTERNAL_URL:-{auth_default}}}\n"
                f"      - SEARCH_INDEXER_CONNECT_URL={pinned_search}\n"
                "      - RATE_LIMIT_EXTERNAL_API_INTERVAL=${RATE_LIMIT_EXTERNAL_API_INTERVAL:-7.5s}\n"
                + extra_app
                + "\n".join(allowlist_compose_lines)
                + "\n"
                + extra_svc,
                encoding="utf-8",
            )

        fx_decl = tmp / "desired-env.lkal"
        fx_decl.write_text(
            '@lkal "0.1"\n'
            "upstreams {\n"
            '  AUTH_HUB_INTERNAL_URL = "https://auth-hub:8443"\n'
            '  SEARCH_INDEXER_CONNECT_URL = "https://search-indexer:9443"\n'
            "}\n"
            "mtls_allowlists {\n"
            + "\n".join(allowlist_lkal_lines)
            + "\n}\n"
            "rate_limits {\n"
            "  RATE_LIMIT_EXTERNAL_API_INTERVAL = 7.5s\n"
            "}\n"
            "feature_flags {\n"
            "  KNOWLEDGE_HOME_ENABLE_PAGE = true\n"
            "}\n",
            encoding="utf-8",
        )
        fx_template = tmp / ".env.template"
        fx_example = tmp / ".env.example"
        fx_example.write_text("", encoding="utf-8")

        # 1. Defaulted key with equal default passes; differing default fails naming the key
        write_sub_compose()
        fx_template.write_text("KNOWLEDGE_HOME_ENABLE_PAGE=true\n", encoding="utf-8")
        errs_ok = _verify(
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        check(
            "fixture verify_declaration passes when defaulted, pinned, and passthrough keys match",
            errs_ok == [],
            repr(errs_ok),
        )

        diff_default_sentinel = "https://auth-hub:9999/DIFF_DEFAULT_SENTINEL"
        write_sub_compose(auth_default=diff_default_sentinel)
        errs_diff_default = _verify(
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_diff_default = "\n".join(errs_diff_default)
        check(
            "verify_declaration fails on differing compose default naming the key without leaking values",
            bool(errs_diff_default)
            and "AUTH_HUB_INTERNAL_URL" in joined_diff_default
            and diff_default_sentinel not in joined_diff_default
            and "https://auth-hub:8443" not in joined_diff_default,
            repr(errs_diff_default),
        )

        # 2. Two interpolations of one key with different defaults fail
        second_default_sentinel = "https://auth-hub:7777/SECOND_DEFAULT_SENTINEL"
        write_sub_compose(
            auth_default="https://auth-hub:8443",
            second_auth_default=second_default_sentinel,
        )
        errs_two_defaults = _verify(
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_two_defaults = "\n".join(errs_two_defaults)
        check(
            "verify_declaration fails when two interpolations of one key have different defaults",
            bool(errs_two_defaults)
            and "AUTH_HUB_INTERNAL_URL" in joined_two_defaults
            and second_default_sentinel not in joined_two_defaults
            and "https://auth-hub:8443" not in joined_two_defaults,
            repr(errs_two_defaults),
        )

        # 3. Pinned literal equal passes (covered above); pinned literal mismatch fails
        pinned_mismatch_sentinel = "https://search-indexer:9999/PINNED_SENTINEL"
        write_sub_compose(pinned_search=pinned_mismatch_sentinel)
        errs_pinned_mismatch = _verify(
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_pinned_mismatch = "\n".join(errs_pinned_mismatch)
        check(
            "verify_declaration fails on pinned literal mismatch naming the key without leaking values",
            bool(errs_pinned_mismatch)
            and "SEARCH_INDEXER_CONNECT_URL" in joined_pinned_mismatch
            and pinned_mismatch_sentinel not in joined_pinned_mismatch
            and "https://search-indexer:9443" not in joined_pinned_mismatch,
            repr(errs_pinned_mismatch),
        )

        # 4. Passthrough key missing from template fails; template value mismatch fails
        write_sub_compose()
        fx_template.write_text("# KNOWLEDGE_HOME_ENABLE_PAGE=true\n", encoding="utf-8")
        errs_pt_missing = _verify(
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        check(
            "verify_declaration fails when passthrough key is missing or commented out in .env.template",
            bool(errs_pt_missing)
            and "KNOWLEDGE_HOME_ENABLE_PAGE" in "\n".join(errs_pt_missing),
            repr(errs_pt_missing),
        )

        pt_mismatch_sentinel = "PT_TEMPLATE_MISMATCH_SENTINEL"
        fx_template.write_text(
            f"KNOWLEDGE_HOME_ENABLE_PAGE={pt_mismatch_sentinel}\n", encoding="utf-8"
        )
        errs_pt_mismatch = _verify(
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_pt_mismatch = "\n".join(errs_pt_mismatch)
        check(
            "verify_declaration fails when passthrough key in .env.template mismatches declaration",
            bool(errs_pt_mismatch)
            and "KNOWLEDGE_HOME_ENABLE_PAGE" in joined_pt_mismatch
            and pt_mismatch_sentinel not in joined_pt_mismatch,
            repr(errs_pt_mismatch),
        )

        # 5. Defaulted key that the template sets to a different value fails
        tpl_defaulted_mismatch = "https://auth-hub:6666/TPL_MISMATCH_SENTINEL"
        fx_template.write_text(
            "KNOWLEDGE_HOME_ENABLE_PAGE=true\n"
            f"AUTH_HUB_INTERNAL_URL={tpl_defaulted_mismatch}\n",
            encoding="utf-8",
        )
        errs_tpl_defaulted = _verify(
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_tpl_defaulted = "\n".join(errs_tpl_defaulted)
        check(
            "verify_declaration fails when .env.template sets a defaulted key to a different value",
            bool(errs_tpl_defaulted)
            and "AUTH_HUB_INTERNAL_URL" in joined_tpl_defaulted
            and tpl_defaulted_mismatch not in joined_tpl_defaulted
            and "https://auth-hub:8443" not in joined_tpl_defaulted,
            repr(errs_tpl_defaulted),
        )

        # 6. Preflight: absent defaulted accepted, absent pinned accepted, absent passthrough rejected,
        # present-but-different managed key rejected, no declared or dotenv VALUE in any error string
        fx_template.write_text("KNOWLEDGE_HOME_ENABLE_PAGE=true\n", encoding="utf-8")
        fx_dotenv = tmp / ".env"
        fx_dotenv.write_text("KNOWLEDGE_HOME_ENABLE_PAGE=true\n", encoding="utf-8")
        pf_ok = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        check(
            "preflight_dotenv accepts absent defaulted and absent pinned keys when passthrough keys match",
            pf_ok == [],
            repr(pf_ok),
        )

        fx_dotenv.write_text("", encoding="utf-8")
        pf_missing_pt = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_pf_missing_pt = "\n".join(pf_missing_pt)
        check(
            "preflight_dotenv rejects absent passthrough key and names only the key",
            bool(pf_missing_pt)
            and "KNOWLEDGE_HOME_ENABLE_PAGE" in joined_pf_missing_pt
            and "AUTH_HUB_INTERNAL_URL" not in joined_pf_missing_pt
            and "SEARCH_INDEXER_CONNECT_URL" not in joined_pf_missing_pt,
            repr(pf_missing_pt),
        )

        dotenv_bad_val = "https://auth-hub:5555/DOTENV_DRIFT_SENTINEL"
        fx_dotenv.write_text(
            "KNOWLEDGE_HOME_ENABLE_PAGE=true\n"
            f"AUTH_HUB_INTERNAL_URL={dotenv_bad_val}\n",
            encoding="utf-8",
        )
        pf_drift = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_pf_drift = "\n".join(pf_drift)
        check(
            "preflight_dotenv rejects present-but-different managed key without leaking declared or dotenv value",
            bool(pf_drift)
            and "AUTH_HUB_INTERNAL_URL" in joined_pf_drift
            and dotenv_bad_val not in joined_pf_drift
            and "https://auth-hub:8443" not in joined_pf_drift,
            repr(pf_drift),
        )

        # 6b. Required class bindings: brace ${KEY}, ${KEY:?...}, bare $KEY, and mixed default+required precedence
        write_sub_compose(
            auth_default="https://auth-hub:8443",
            second_auth_raw="AUTH_HUB_INTERNAL_URL=${AUTH_HUB_INTERNAL_URL:?required in second-svc}",
            extra_app_env_lines=[
                "REQ_BRACE_KEY=${REQ_BRACE_KEY}",
                "REQ_COLON_Q_KEY=${REQ_COLON_Q_KEY:?must be set}",
                "REQ_BARE_KEY=$REQ_BARE_KEY",
            ],
        )
        req_bindings, req_binding_errs = kisage_env.resolve_compose_bindings(
            {
                "REQ_BRACE_KEY",
                "REQ_COLON_Q_KEY",
                "REQ_BARE_KEY",
                "AUTH_HUB_INTERNAL_URL",
            },
            compose_root=fx_compose_root,
        )
        check(
            "resolve_compose_bindings classifies ${KEY}, ${KEY:?...}, bare $KEY, and mixed ${KEY:-default}+required as required",
            req_binding_errs == []
            and req_bindings is not None
            and req_bindings.get("REQ_BRACE_KEY", {}).get("class") == "required"
            and req_bindings.get("REQ_COLON_Q_KEY", {}).get("class") == "required"
            and req_bindings.get("REQ_BARE_KEY", {}).get("class") == "required"
            and req_bindings.get("AUTH_HUB_INTERNAL_URL", {}).get("class")
            == "required",
            repr((req_bindings, req_binding_errs)),
        )

        fx_dotenv.write_text("KNOWLEDGE_HOME_ENABLE_PAGE=true\n", encoding="utf-8")
        pf_missing_req = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_pf_missing_req = "\n".join(pf_missing_req)
        fx_dotenv.write_text(
            "KNOWLEDGE_HOME_ENABLE_PAGE=true\n"
            "AUTH_HUB_INTERNAL_URL=https://auth-hub:8443\n",
            encoding="utf-8",
        )
        pf_present_req = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        check(
            "preflight_dotenv rejects absent required key (including mixed default+required) and accepts when present",
            bool(pf_missing_req)
            and "AUTH_HUB_INTERNAL_URL" in joined_pf_missing_req
            and "https://auth-hub:8443" not in joined_pf_missing_req
            and pf_present_req == [],
            repr((pf_missing_req, pf_present_req)),
        )

        # 6c. Allowlist set comparison in preflight_dotenv: reordered peers accepted; missing, extra, and duplicate peers rejected
        write_sub_compose()
        fx_dotenv.write_text(
            "KNOWLEDGE_HOME_ENABLE_PAGE=true\n"
            "SEARCH_INDEXER_MTLS_ALLOWED_PEERS=acolyte-orchestrator,rag-orchestrator,alt-backend\n",
            encoding="utf-8",
        )
        pf_allowlist_reordered = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        check(
            "preflight_dotenv accepts mtls_allowlists key with same peers in a different order",
            pf_allowlist_reordered == [],
            repr(pf_allowlist_reordered),
        )

        fx_dotenv.write_text(
            "KNOWLEDGE_HOME_ENABLE_PAGE=true\n"
            "SEARCH_INDEXER_MTLS_ALLOWED_PEERS=alt-backend,rag-orchestrator\n",
            encoding="utf-8",
        )
        pf_allowlist_missing = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        extra_allowlist_sentinel = "rogue-dotenv-peer-sentinel-7788"
        fx_dotenv.write_text(
            "KNOWLEDGE_HOME_ENABLE_PAGE=true\n"
            f"SEARCH_INDEXER_MTLS_ALLOWED_PEERS=alt-backend,rag-orchestrator,acolyte-orchestrator,{extra_allowlist_sentinel}\n",
            encoding="utf-8",
        )
        pf_allowlist_extra = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        fx_dotenv.write_text(
            "KNOWLEDGE_HOME_ENABLE_PAGE=true\n"
            "SEARCH_INDEXER_MTLS_ALLOWED_PEERS=alt-backend,rag-orchestrator,acolyte-orchestrator,alt-backend\n",
            encoding="utf-8",
        )
        pf_allowlist_dup = _preflight(
            fx_dotenv,
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_allowlist_neg = "\n".join(
            [*pf_allowlist_missing, *pf_allowlist_extra, *pf_allowlist_dup]
        )
        check(
            "preflight_dotenv rejects mtls_allowlists missing peer, extra peer, and duplicate peer without leaking values",
            bool(pf_allowlist_missing)
            and "SEARCH_INDEXER_MTLS_ALLOWED_PEERS"
            in "\n".join(pf_allowlist_missing)
            and bool(pf_allowlist_extra)
            and "SEARCH_INDEXER_MTLS_ALLOWED_PEERS"
            in "\n".join(pf_allowlist_extra)
            and bool(pf_allowlist_dup)
            and "SEARCH_INDEXER_MTLS_ALLOWED_PEERS" in "\n".join(pf_allowlist_dup)
            and extra_allowlist_sentinel not in joined_allowlist_neg
            and "alt-backend" not in joined_allowlist_neg,
            repr((pf_allowlist_missing, pf_allowlist_extra, pf_allowlist_dup)),
        )

        # 6d. CLI positive control: changed-but-consistent declaration, compose fixture, and template exit 0 via verify CLI
        consistent_url = "https://auth-hub:8444"
        write_sub_compose(auth_default=consistent_url)
        fx_template.write_text("KNOWLEDGE_HOME_ENABLE_PAGE=false\n", encoding="utf-8")
        raw_changed_decl = (
            fx_decl.read_text(encoding="utf-8")
            .replace(
                '  AUTH_HUB_INTERNAL_URL = "https://auth-hub:8443"',
                f'  AUTH_HUB_INTERNAL_URL = "{consistent_url}"',
            )
            .replace(
                "  KNOWLEDGE_HOME_ENABLE_PAGE = true",
                "  KNOWLEDGE_HOME_ENABLE_PAGE = false",
            )
        )
        fmt_changed_proc = subprocess.run(
            [find_lkal_bin(), "fmt", "-"],
            input=raw_changed_decl,
            capture_output=True,
            text=True,
            check=True,
        )
        fx_decl_changed = tmp / "changed-consistent.lkal"
        fx_decl_changed.write_text(fmt_changed_proc.stdout, encoding="utf-8")
        os.chmod(fx_decl_changed, 0o600)
        cli_cons_rc, cli_cons_out, cli_cons_err = run_cli_capture(
            kisage_env,
            [
                "verify",
                "--declaration",
                str(fx_decl_changed),
                "--manifest",
                str(fx_manifest),
                "--compose-root",
                str(fx_compose_root),
                "--env-template",
                str(fx_template),
                "--env-example",
                str(fx_example),
            ],
        )
        check(
            "kisage-env.py verify CLI exits 0 when declaration, compose fixture, and .env.template change consistently",
            cli_cons_rc == 0
            and cli_cons_err == ""
            and "canonical lkal fmt" in cli_cons_out
            and "compose" in cli_cons_out.lower()
            and ".env.template" in cli_cons_out,
            f"rc={cli_cons_rc} out={cli_cons_out!r} err={cli_cons_err!r}",
        )

    # 7. Real repository verify_declaration() returns no errors
    repo_verify_errs = kisage_env.verify_declaration(DECL_PATH, MANIFEST_PATH)
    check(
        "real repository verify_declaration() returns no compose or template lockstep errors",
        repo_verify_errs == [],
        repr(repo_verify_errs),
    )


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


def run_mtls_policy_and_llm_budgets_checks(
    kisage_env, overrides, kisage_bin: str
) -> None:
    print("10. mTLS Policy & LLM Budget Managed Sections")

    expected_mtls_policy: dict[str, object] = {
        "ACOLYTE_ORCHESTRATOR_MTLS_ENFORCE": True,
        "ACOLYTE_ORCHESTRATOR_PEER_IDENTITY_STRICT": False,
        "ALT_BUTTERFLY_FACADE_MTLS_ENFORCE": False,
        "ALT_BUTTERFLY_FACADE_MTLS_LISTEN": False,
        "AUTH_HUB_MTLS_CLIENT_AUTH": "require_and_verify",
        "NEWS_CREATOR_PEER_IDENTITY_STRICT": True,
        "PRE_PROCESSOR_MTLS_CLIENT_AUTH": "require_and_verify",
        "PRE_PROCESSOR_MTLS_ENFORCE": True,
        "RAG_ORCHESTRATOR_PEER_IDENTITY_MODE": "mtls",
        "RECAP_SUBWORKER_PEER_IDENTITY_STRICT": True,
        "RECAP_WORKER_MTLS_ENFORCE": True,
        "SEARCH_INDEXER_MTLS_CLIENT_AUTH": "require_and_verify",
        "SEARCH_INDEXER_MTLS_ENFORCE": True,
        "TAG_GENERATOR_MTLS_ENFORCE": True,
        "TAG_GENERATOR_PEER_IDENTITY_STRICT": True,
    }
    expected_llm_budgets: dict[str, object] = {
        "LLM_NUM_CTX": 12288,
        "LLM_NUM_PREDICT": 1200,
        "SUMMARY_NUM_PREDICT": 1200,
    }
    expected_str_map = {
        k: ("true" if v else "false") if isinstance(v, bool) else str(v)
        for k, v in {**expected_mtls_policy, **expected_llm_budgets}.items()
    }

    go_backend_cfg = ROOT / "alt-backend" / "app" / "config" / "binary_backend.go"
    go_text = (
        go_backend_cfg.read_text(encoding="utf-8")
        if go_backend_cfg.is_file()
        else ""
    )
    slice_match = re.search(
        r"\bvar\s+backendRejectedMTLSEnv\s*=\s*\[\]string\s*\{([^}]*)\}",
        go_text,
        re.S,
    )
    go_rejected_keys = (
        re.findall(r'"([^"]+)"', slice_match.group(1))
        if slice_match is not None
        else []
    )
    forbidden_const = getattr(kisage_env, "DOTENV_FORBIDDEN_KEYS", None)
    check(
        "DOTENV_FORBIDDEN_KEYS is in lockstep with backendRejectedMTLSEnv in alt-backend/app/config/binary_backend.go",
        go_backend_cfg.is_file()
        and slice_match is not None
        and bool(go_rejected_keys)
        and isinstance(forbidden_const, tuple)
        and set(forbidden_const) == set(go_rejected_keys)
        and len(forbidden_const) == len(go_rejected_keys),
        f"file={go_backend_cfg.is_file()} slice={slice_match is not None} go={go_rejected_keys!r} const={forbidden_const!r}",
    )

    sections = kisage_env.parse_lkal_sections(
        DECL_PATH.read_text(encoding="utf-8")
    )
    mtls_policy_sec = sections.get("mtls_policy") or {}
    llm_budgets_sec = sections.get("llm_budgets") or {}
    verify_errs = kisage_env.verify_declaration(DECL_PATH, MANIFEST_PATH)
    check(
        "repository declaration contains mtls_policy (15 keys) and llm_budgets (3 keys) and verify_declaration() returns no errors",
        mtls_policy_sec == expected_mtls_policy
        and len(mtls_policy_sec) == 15
        and llm_budgets_sec == expected_llm_budgets
        and len(llm_budgets_sec) == 3
        and verify_errs == [],
        f"mtls_policy={len(mtls_policy_sec)} llm_budgets={len(llm_budgets_sec)} errs={verify_errs!r}",
    )

    declared_new_keys = set(mtls_policy_sec) | set(llm_budgets_sec)
    bindings, binding_errs = kisage_env.resolve_compose_bindings(declared_new_keys)
    check(
        "every declared key of mtls_policy and llm_budgets is classified defaulted by resolve_compose_bindings on the real repository",
        len(declared_new_keys) == 18
        and binding_errs == []
        and bindings is not None
        and all(
            bindings.get(k, {}).get("class") == "defaulted"
            for k in declared_new_keys
        ),
        f"count={len(declared_new_keys)} errs={binding_errs!r}",
    )

    with tempfile.TemporaryDirectory() as tmp_dir:
        tmp = pathlib.Path(tmp_dir)
        fx_manifest = tmp / "host-prereqs.yaml"
        fx_manifest.write_text(
            MANIFEST_PATH.read_text(encoding="utf-8"), encoding="utf-8"
        )
        allowlists = sections["mtls_allowlists"]
        allowlist_lkal_lines = [
            f"  {k} = [{', '.join(json.dumps(p) for p in peers)}]"
            for k, peers in sorted(allowlists.items())
        ]
        allowlist_compose_lines = [
            f"      - {k}=${{{k}:-{','.join(peers)}}}"
            for k, peers in sorted(allowlists.items())
        ]
        fx_compose_dir = tmp / "compose"
        fx_compose_dir.mkdir()
        fx_compose_root = fx_compose_dir / "compose.yaml"
        fx_compose_root.write_text("include:\n  - sub.yaml\n", encoding="utf-8")
        fx_sub_yaml = fx_compose_dir / "sub.yaml"
        mtls_policy_sentinel = "DIFF_MTLS_POLICY_SENTINEL"
        fx_sub_yaml.write_text(
            "services:\n"
            "  app:\n"
            "    image: example/app:latest\n"
            "    environment:\n"
            "      - RATE_LIMIT_EXTERNAL_API_INTERVAL=${RATE_LIMIT_EXTERNAL_API_INTERVAL:-7.5s}\n"
            f"      - ACOLYTE_ORCHESTRATOR_MTLS_ENFORCE=${{ACOLYTE_ORCHESTRATOR_MTLS_ENFORCE:-{mtls_policy_sentinel}}}\n"
            + "\n".join(allowlist_compose_lines)
            + "\n",
            encoding="utf-8",
        )
        fx_decl = tmp / "desired-env.lkal"
        fx_decl.write_text(
            '@lkal "0.1"\n'
            "mtls_allowlists {\n"
            + "\n".join(allowlist_lkal_lines)
            + "\n}\n"
            "rate_limits {\n"
            "  RATE_LIMIT_EXTERNAL_API_INTERVAL = 7.5s\n"
            "}\n"
            "mtls_policy {\n"
            "  ACOLYTE_ORCHESTRATOR_MTLS_ENFORCE = true\n"
            "}\n",
            encoding="utf-8",
        )
        fx_template = tmp / ".env.template"
        fx_template.write_text("", encoding="utf-8")
        fx_example = tmp / ".env.example"
        fx_example.write_text("", encoding="utf-8")
        errs_mtls_diff = kisage_env.verify_declaration(
            fx_decl,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_mtls_diff = "\n".join(errs_mtls_diff)
        check(
            "fixture verify_declaration fails when an mtls_policy compose default differs from declaration, naming the key without leaking values",
            bool(errs_mtls_diff)
            and "ACOLYTE_ORCHESTRATOR_MTLS_ENFORCE" in joined_mtls_diff
            and mtls_policy_sentinel not in joined_mtls_diff
            and "true" not in joined_mtls_diff,
            repr(errs_mtls_diff),
        )

        fx_sub_yaml.write_text(
            "services:\n"
            "  app:\n"
            "    image: example/app:latest\n"
            "    environment:\n"
            "      - RATE_LIMIT_EXTERNAL_API_INTERVAL=${RATE_LIMIT_EXTERNAL_API_INTERVAL:-7.5s}\n"
            "      - MTLS_LISTEN=${MTLS_LISTEN:-true}\n"
            + "\n".join(allowlist_compose_lines)
            + "\n",
            encoding="utf-8",
        )
        fx_decl_forbidden = tmp / "forbidden-mtls-listen.lkal"
        fx_decl_forbidden.write_text(
            '@lkal "0.1"\n'
            "mtls_allowlists {\n"
            + "\n".join(allowlist_lkal_lines)
            + "\n}\n"
            "rate_limits {\n"
            "  RATE_LIMIT_EXTERNAL_API_INTERVAL = 7.5s\n"
            "}\n"
            "mtls_policy {\n"
            "  MTLS_LISTEN = true\n"
            "}\n",
            encoding="utf-8",
        )
        errs_forbidden = kisage_env.verify_declaration(
            fx_decl_forbidden,
            fx_manifest,
            compose_root=fx_compose_root,
            env_template_path=fx_template,
            env_example_path=fx_example,
        )
        joined_forbidden = "\n".join(errs_forbidden)
        check(
            "fixture verify_declaration rejects MTLS_LISTEN with rule id dotenv-forbidden-key naming the key without leaking values",
            bool(errs_forbidden)
            and "dotenv-forbidden-key" in joined_forbidden
            and "MTLS_LISTEN" in joined_forbidden
            and "true" not in joined_forbidden,
            repr(errs_forbidden),
        )

    tpl_env = overrides.parse_env_file(
        (ROOT / ".env.template").read_text(encoding="utf-8")
    )
    ex_env = overrides.parse_env_file(
        (ROOT / ".env.example").read_text(encoding="utf-8")
    )
    tpl_or_ex_mismatches = sorted(
        k
        for k, exp_v in expected_str_map.items()
        if (k in tpl_env and tpl_env[k] != exp_v)
        or (k in ex_env and ex_env[k] != exp_v)
    )
    check(
        ".env.template and .env.example do not set any mtls_policy or llm_budgets key to a value differing from the declaration",
        len(declared_new_keys) == 18 and tpl_or_ex_mismatches == [],
        f"mismatched_keys={tpl_or_ex_mismatches}",
    )

    with tempfile.TemporaryDirectory() as tmp_parent:
        os.chmod(tmp_parent, 0o700)
        sandbox = pathlib.Path(tmp_parent) / "kisage-sandbox"
        decl_copy = pathlib.Path(tmp_parent) / "desired-env.lkal"
        shutil.copyfile(DECL_PATH, decl_copy)
        os.chmod(decl_copy, 0o600)

        rc_init = kisage_env.main(
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
        host_json = sandbox / "host.json"
        target_lkal = sandbox / "host-env.lkal"
        host_doc = (
            json.loads(host_json.read_text(encoding="utf-8"))
            if host_json.is_file()
            else {}
        )
        scope_paths = [
            entry.get("path")
            for res in host_doc.get("resources", [])
            for entry in res.get("scope", [])
        ]
        env = {**os.environ, "KISAGE_HOST_CONFIG": str(host_json)}
        plan_pre = subprocess.run(
            [kisage_bin, "plan", str(decl_copy), "-detailed-exitcode", "-json"],
            capture_output=True,
            text=True,
            env=env,
            check=False,
        )
        apply_res = subprocess.run(
            [kisage_bin, "apply", str(decl_copy), "-json"],
            capture_output=True,
            text=True,
            env=env,
            check=False,
        )
        apply_doc = json.loads(apply_res.stdout) if apply_res.stdout.strip() else {}
        plan_post = subprocess.run(
            [kisage_bin, "plan", str(decl_copy), "-detailed-exitcode", "-json"],
            capture_output=True,
            text=True,
            env=env,
            check=False,
        )
        converged_sections = (
            kisage_env.parse_lkal_sections(target_lkal.read_text(encoding="utf-8"))
            if target_lkal.is_file()
            else {}
        )
        check(
            "sandbox host.json grants Scope to /mtls_policy and /llm_budgets and init -> plan -> apply -> second plan converges both sections",
            rc_init == 0
            and "/mtls_policy" in scope_paths
            and "/llm_budgets" in scope_paths
            and plan_pre.returncode == 2
            and apply_res.returncode == 0
            and apply_doc.get("summary", {}).get("execution") == "Completed"
            and apply_doc.get("summary", {}).get("convergence") == "Converged"
            and plan_post.returncode == 0
            and converged_sections.get("mtls_policy") == expected_mtls_policy
            and converged_sections.get("llm_budgets") == expected_llm_budgets,
            f"scope={scope_paths} pre={plan_pre.returncode} apply={apply_res.returncode} post={plan_post.returncode}",
        )

        dotenv_text = (
            kisage_env.render_dotenv(target_lkal) if target_lkal.is_file() else ""
        )
        dotenv_lines = set(dotenv_text.splitlines())
        check(
            "render-dotenv output of converged sandbox contains mtls_policy and llm_budgets keys with booleans as true/false and integers bare",
            "# [mtls_policy]" in dotenv_lines
            and "# [llm_budgets]" in dotenv_lines
            and "ACOLYTE_ORCHESTRATOR_MTLS_ENFORCE=true" in dotenv_lines
            and "ACOLYTE_ORCHESTRATOR_PEER_IDENTITY_STRICT=false" in dotenv_lines
            and "MTLS_LISTEN=true" not in dotenv_lines
            and "LLM_NUM_CTX=12288" in dotenv_lines
            and "LLM_NUM_PREDICT=1200" in dotenv_lines
            and "SUMMARY_NUM_PREDICT=1200" in dotenv_lines
            and all(
                f"{k}={v}" in dotenv_lines for k, v in expected_str_map.items()
            ),
            f"missing={[k for k, v in expected_str_map.items() if f'{k}={v}' not in dotenv_lines]}",
        )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Portable kisage/LKAL acceptance suite (pass --audit-host-config to inspect live host .claude/settings.json and .mcp.json)"
    )
    parser.add_argument(
        "--audit-host-config",
        action="store_true",
        help="Explicitly audit gitignored host-local .claude/settings.json and .mcp.json in ROOT (fails closed if missing or invalid)",
    )
    args = parser.parse_args(argv)

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

    check_lkal_cli_gates(kisage_env)

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

        unchanged_file = pathlib.Path(tmp_check_dir) / "unchanged.lkal"
        unchanged_file.write_text(decl_raw_text, encoding="utf-8")
        os.chmod(unchanged_file, 0o600)
        unchanged_rc, _unchanged_out, unchanged_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(unchanged_file)]
        )
        check(
            "kisage-env.py verify exits 0 on an unchanged declaration copy",
            unchanged_rc == 0,
            unchanged_err,
        )

        reordered_text = decl_raw_text.replace(
            '    "recap-evaluator",\n    "pre-processor"',
            '    "pre-processor",\n    "recap-evaluator"',
        )
        reordered_file = pathlib.Path(tmp_check_dir) / "reordered-peers.lkal"
        reordered_file.write_text(reordered_text, encoding="utf-8")
        os.chmod(reordered_file, 0o600)
        reordered_rc, _reordered_out, reordered_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(reordered_file)]
        )
        check(
            "kisage-env.py verify exits 0 when peers in /mtls_allowlists are reordered",
            reordered_text != decl_raw_text and reordered_rc == 0,
            reordered_err,
        )

        missing_key_text = decl_raw_text.replace(
            '  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["alt-backend"]\n',
            "",
        )
        missing_key_file = pathlib.Path(tmp_check_dir) / "missing-key.lkal"
        missing_key_file.write_text(missing_key_text, encoding="utf-8")
        os.chmod(missing_key_file, 0o600)
        missing_key_rc, _missing_key_out, missing_key_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(missing_key_file)]
        )
        check(
            "kisage-env.py verify exits non-zero when an /mtls_allowlists key is missing",
            missing_key_text != decl_raw_text
            and missing_key_rc == 1
            and "PRE_PROCESSOR_MTLS_ALLOWED_PEERS" in missing_key_err,
            missing_key_err,
        )

        extra_key_text = decl_raw_text.replace(
            '  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["alt-backend"]\n',
            '  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["alt-backend"]\n'
            '  EXTRA_MTLS_ALLOWED_PEERS = ["alt-backend"]\n',
        )
        extra_key_file = pathlib.Path(tmp_check_dir) / "extra-key.lkal"
        extra_key_file.write_text(extra_key_text, encoding="utf-8")
        os.chmod(extra_key_file, 0o600)
        extra_key_rc, _extra_key_out, extra_key_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(extra_key_file)]
        )
        check(
            "kisage-env.py verify exits non-zero when an extra /mtls_allowlists key is declared",
            extra_key_text != decl_raw_text
            and extra_key_rc == 1
            and "EXTRA_MTLS_ALLOWED_PEERS" in extra_key_err,
            extra_key_err,
        )

        removed_peer_text = decl_raw_text.replace(
            '    "recap-evaluator",\n    "pre-processor"',
            '    "recap-evaluator"',
        )
        removed_peer_file = pathlib.Path(tmp_check_dir) / "removed-peer.lkal"
        removed_peer_file.write_text(removed_peer_text, encoding="utf-8")
        os.chmod(removed_peer_file, 0o600)
        removed_peer_rc, _removed_peer_out, removed_peer_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(removed_peer_file)]
        )
        check(
            "kisage-env.py verify exits non-zero when a peer is removed from /mtls_allowlists",
            removed_peer_text != decl_raw_text
            and removed_peer_rc == 1
            and "mtls_allowlists/NEWS_CREATOR_MTLS_ALLOWED_PEERS peer allowlist mismatch against deploy/host-prereqs.yaml"
            in removed_peer_err,
            removed_peer_err,
        )

        peer_sentinel = "rogue-peer-sentinel-987654"
        added_peer_text = decl_raw_text.replace(
            '  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["alt-backend"]',
            f'  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["alt-backend", "{peer_sentinel}"]',
        )
        added_peer_file = pathlib.Path(tmp_check_dir) / "added-peer.lkal"
        added_peer_file.write_text(added_peer_text, encoding="utf-8")
        os.chmod(added_peer_file, 0o600)
        added_peer_rc, added_peer_stdout, added_peer_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(added_peer_file)]
        )
        added_peer_out = added_peer_stdout + added_peer_err
        check(
            "kisage-env.py verify exits non-zero when a peer is added to /mtls_allowlists and never echoes peer values",
            added_peer_text != decl_raw_text
            and added_peer_rc == 1
            and "mtls_allowlists/PRE_PROCESSOR_MTLS_ALLOWED_PEERS peer allowlist mismatch against deploy/host-prereqs.yaml"
            in added_peer_err
            and peer_sentinel not in added_peer_out,
            added_peer_out,
        )

        replaced_peer_text = decl_raw_text.replace(
            '  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["alt-backend"]',
            f'  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["{peer_sentinel}"]',
        )
        replaced_peer_file = pathlib.Path(tmp_check_dir) / "replaced-peer.lkal"
        replaced_peer_file.write_text(replaced_peer_text, encoding="utf-8")
        os.chmod(replaced_peer_file, 0o600)
        replaced_peer_rc, replaced_peer_stdout, replaced_peer_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(replaced_peer_file)]
        )
        replaced_peer_out = replaced_peer_stdout + replaced_peer_err
        check(
            "kisage-env.py verify exits non-zero when a peer is replaced in /mtls_allowlists and never echoes peer values",
            replaced_peer_text != decl_raw_text
            and replaced_peer_rc == 1
            and "mtls_allowlists/PRE_PROCESSOR_MTLS_ALLOWED_PEERS peer allowlist mismatch against deploy/host-prereqs.yaml"
            in replaced_peer_err
            and peer_sentinel not in replaced_peer_out,
            replaced_peer_out,
        )

        dup_peer_text = decl_raw_text.replace(
            '  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["alt-backend"]',
            '  PRE_PROCESSOR_MTLS_ALLOWED_PEERS = ["alt-backend", "alt-backend"]',
        )
        dup_peer_file = pathlib.Path(tmp_check_dir) / "dup-peer.lkal"
        dup_peer_file.write_text(dup_peer_text, encoding="utf-8")
        os.chmod(dup_peer_file, 0o600)
        dup_peer_rc, dup_peer_stdout, dup_peer_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(dup_peer_file)]
        )
        dup_peer_out = dup_peer_stdout + dup_peer_err
        check(
            "kisage-env.py verify exits non-zero when /mtls_allowlists contains duplicate peers",
            dup_peer_text != decl_raw_text
            and dup_peer_rc == 1
            and "PRE_PROCESSOR_MTLS_ALLOWED_PEERS" in dup_peer_err
            and "alt-backend" not in dup_peer_out,
            dup_peer_out,
        )

        flag_change_text = decl_raw_text.replace(
            "  KNOWLEDGE_HOME_ENABLE_PAGE = true",
            "  KNOWLEDGE_HOME_ENABLE_PAGE = false",
        )
        flag_change_file = pathlib.Path(tmp_check_dir) / "flag-change.lkal"
        flag_change_file.write_text(flag_change_text, encoding="utf-8")
        os.chmod(flag_change_file, 0o600)
        flag_change_rc, flag_change_stdout, flag_change_err = run_cli_capture(
            kisage_env, ["verify", "--declaration", str(flag_change_file)]
        )
        flag_change_out = flag_change_stdout + flag_change_err
        check(
            "kisage-env.py verify reports no mtls_allowlists error when only /feature_flags changes (and flags template drift for KNOWLEDGE_HOME_ENABLE_PAGE)",
            flag_change_text != decl_raw_text
            and flag_change_rc == 1
            and "KNOWLEDGE_HOME_ENABLE_PAGE" in flag_change_out
            and "mtls_allowlists" not in flag_change_out,
            flag_change_out,
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

        # Missing managed key (passthrough key required in dotenv)
        missing_key_text = "\n".join(
            line
            for line in converged_with_local_secrets.splitlines()
            if not line.startswith("KNOWLEDGE_HOME_ENABLE_PAGE=")
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
            rc_miss != 0 and "KNOWLEDGE_HOME_ENABLE_PAGE" in (out_miss + err_miss),
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

    print(
        "7. Layer 3 Execution Boundary (audit-host-config structural indicator contract & repo .kisage isolation; portable mode uses synthetic fixtures, hook execution/OS isolation not verified)"
    )
    with tempfile.TemporaryDirectory() as tmp_audit_dir:
        os.chmod(tmp_audit_dir, 0o700)
        tmp_audit_path = pathlib.Path(tmp_audit_dir)
        missing_claude = tmp_audit_path / "missing-settings.json"
        missing_mcp = tmp_audit_path / "missing-.mcp.json"

        rc_miss_cfg, out_miss_cfg, err_miss_cfg = run_cli_capture(
            kisage_env,
            [
                "audit-host-config",
                "--claude-settings",
                str(missing_claude),
                "--mcp-config",
                str(missing_mcp),
            ],
        )
        check(
            "audit-host-config fails closed when host .claude/settings.json or .mcp.json is missing (no silent skip)",
            rc_miss_cfg == 1
            and "missing-settings.json" in err_miss_cfg
            and "missing-.mcp.json" in err_miss_cfg,
            repr(err_miss_cfg),
        )

        fx_claude = tmp_audit_path / "settings.json"
        fx_mcp = tmp_audit_path / ".mcp.json"
        audit_secret_sentinel_1 = "AUDIT_JSON_SECRET_SENTINEL_776655"
        audit_secret_sentinel_2 = "AUDIT_ENV_SECRET_SENTINEL_443322"

        fx_claude.write_text("null\n", encoding="utf-8")
        fx_mcp.write_text("null\n", encoding="utf-8")
        errs_null_root = kisage_env.audit_host_config(fx_claude, fx_mcp)

        fx_claude.write_text(
            json.dumps(
                {
                    "hooks": {
                        "PreToolUse": [
                            {
                                "matcher": "Edit|Write",
                                "description": ".kisage/*",
                                "hooks": [
                                    {
                                        "type": "command",
                                        "command": "echo noop",
                                        "comment": ".kisage/*",
                                    }
                                ],
                            },
                            {
                                "matcher": "Read",
                                "hooks": [
                                    {
                                        "type": "command",
                                        "command": 'case "$FILE" in .kisage/*) exit 1 ;; esac',
                                    }
                                ],
                            },
                        ]
                    }
                }
            ),
            encoding="utf-8",
        )
        fx_mcp.write_text(
            json.dumps(
                {
                    "mcpServers": {
                        "kisage": {
                            "command": "",
                            "args": ["mcp"],
                            "env": {"KISAGE_HOST_CONFIG": "/tmp/host.json"},
                        }
                    }
                }
            ),
            encoding="utf-8",
        )
        errs_metadata_or_empty_cmd = kisage_env.audit_host_config(fx_claude, fx_mcp)

        fx_claude.write_text(
            f'{{"broken": "{audit_secret_sentinel_1}"\n', encoding="utf-8"
        )
        fx_mcp.write_text(
            json.dumps(
                {
                    "mcpServers": {
                        "other": {
                            "command": "other",
                            "env": {"SECRET_TOKEN": audit_secret_sentinel_2},
                        },
                        "kisage": {"command": "kisage", "args": ["mcp"], "env": {}},
                    }
                }
            ),
            encoding="utf-8",
        )
        rc_bad_cfg, out_bad_cfg, err_bad_cfg = run_cli_capture(
            kisage_env,
            [
                "audit-host-config",
                "--claude-settings",
                str(fx_claude),
                "--mcp-config",
                str(fx_mcp),
            ],
        )
        combined_bad_cfg = out_bad_cfg + err_bad_cfg

        fx_claude.write_text(
            json.dumps(
                {
                    "hooks": {
                        "PreToolUse": [
                            {
                                "matcher": "Edit|Write|MultiEdit",
                                "hooks": [
                                    {
                                        "type": "command",
                                        "command": 'case "$FILE" in .env|.kisage/*) exit 1 ;; esac',
                                    }
                                ],
                            }
                        ]
                    }
                }
            ),
            encoding="utf-8",
        )
        fx_mcp.write_text(
            json.dumps(
                {
                    "mcpServers": {
                        "kisage": {
                            "command": "kisage",
                            "args": ["mcp"],
                            "env": {
                                "KISAGE_HOST_CONFIG": str(
                                    tmp_audit_path / ".kisage" / "host.json"
                                )
                            },
                        }
                    }
                }
            ),
            encoding="utf-8",
        )
        rc_valid_fx, out_valid_fx, err_valid_fx = run_cli_capture(
            kisage_env,
            [
                "audit-host-config",
                "--claude-settings",
                str(fx_claude),
                "--mcp-config",
                str(fx_mcp),
            ],
        )
        check(
            "audit-host-config rejects null/non-object roots, metadata-only/Read-only PreToolUse, missing MCP command, and malformed config without leaking secrets; accepts valid synthetic fixture (structural indicator check only; hook execution and OS isolation not verified)",
            len(errs_null_root) == 2
            and len(errs_metadata_or_empty_cmd) == 2
            and rc_bad_cfg == 1
            and "KISAGE_HOST_CONFIG" in combined_bad_cfg
            and audit_secret_sentinel_1 not in combined_bad_cfg
            and audit_secret_sentinel_2 not in combined_bad_cfg
            and rc_valid_fx == 0
            and "hook execution and OS isolation not verified" in out_valid_fx
            and err_valid_fx == "",
            f"errs_null={errs_null_root!r} errs_meta={errs_metadata_or_empty_cmd!r} rc_bad={rc_bad_cfg} rc_valid={rc_valid_fx}",
        )

    if args.audit_host_config:
        rc_host_cfg, out_host_cfg, err_host_cfg = run_cli_capture(
            kisage_env,
            [
                "audit-host-config",
                "--claude-settings",
                str(CLAUDE_SETTINGS),
                "--mcp-config",
                str(MCP_CONFIG),
            ],
        )
        check(
            "explicit --audit-host-config checks expected structural entries in live host .claude/settings.json and .mcp.json (hook execution and OS isolation not verified)",
            rc_host_cfg == 0,
            err_host_cfg.strip(),
        )
    else:
        print(
            "  NOTE  portable mode did not inspect host-local .claude/settings.json or .mcp.json "
            "(run 'python3 scripts/kisage-env.py audit-host-config' or pass '--audit-host-config' on an operator host; structural indicator check only, hook execution and OS isolation not verified)"
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
        "tests/scripts/test_kisage_clean_checkout.py",
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
        "compose-audit.yaml defines kisage-env-invariant job running lkal check, verify, test-kisage-env.py, and clean-checkout regression with setup-go",
        "kisage-env-invariant:" in wf_text
        and "actions/setup-go@" in wf_text
        and "lkal check --json deploy/desired-env.lkal" in wf_text
        and "python3 scripts/kisage-env.py verify" in wf_text
        and wf_text.find("lkal check --json deploy/desired-env.lkal")
        < wf_text.find("python3 scripts/kisage-env.py verify")
        and "python3 scripts/tests/test-kisage-env.py" in wf_text
        and "test_kisage_clean_checkout.py" in wf_text,
    )

    run_compose_lockstep_and_preflight_checks(kisage_env)
    run_mtls_policy_and_llm_budgets_checks(kisage_env, overrides, kisage_bin)

    print(f"\n{PASS} passed, {FAIL} failed")
    return 1 if FAIL else 0


if __name__ == "__main__":
    raise SystemExit(main())
