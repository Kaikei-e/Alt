"""Clean-checkout & host-config audit regression tests for kisage-env.

Verifies that:
  1. `scripts/kisage-env.py verify` and `scripts/tests/test-kisage-env.py`
     succeed in a clean tracked-files checkout under `/tmp` where gitignored
     host-local files (`.claude/settings.json`, `.mcp.json`, `.kisage/`, `.env`)
     are completely absent, and portable output truthfully states that
     host-local configuration was not inspected.
  2. Explicit read-only host config auditing (`kisage-env.py audit-host-config`
     and `test-kisage-env.py --audit-host-config`) fails closed with a clear
     error when `.claude/settings.json` or `.mcp.json` is missing (no silent
     skip).
  3. Explicit host config structural auditing rejects malformed JSON, `null` or
     non-object JSON roots, metadata-only/Read-only/malformed-regex `PreToolUse`
     entries, and missing/empty MCP `command` or `KISAGE_HOST_CONFIG` without
     echoing raw file contents or secret values.
  4. Explicit host config structural auditing succeeds on a valid synthetic
     fixture (labeled as synthetic fixture and structural indicator check only;
     hook execution and OS isolation not verified).
"""

from __future__ import annotations

import importlib.util
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]

TRACKED_PREFIXES = (
    "compose/",
    "deploy/",
    "scripts/",
    "tests/scripts/",
    ".github/workflows/compose-audit.yaml",
    "docs/runbooks/kisage-env.md",
)
TRACKED_EXACT = {
    "Makefile",
    ".gitignore",
    ".env.template",
    ".env.example",
    "alt-backend/app/config/binary_backend.go",
}
FORBIDDEN_LOCAL_PATHS = (
    ".claude",
    ".claude/settings.json",
    ".mcp.json",
    ".kisage",
    ".env",
)


def materialize_clean_tracked_worktree(dst_root: pathlib.Path) -> None:
    """Copy current worktree versions of tracked files into `dst_root` without any gitignored local state."""
    tracked_raw = subprocess.check_output(
        ["git", "-C", str(REPO_ROOT), "ls-files", "-z"], text=False
    )
    rel_paths = [
        entry.decode("utf-8") for entry in tracked_raw.split(b"\0") if entry
    ]
    if "tests/scripts/test_kisage_clean_checkout.py" not in rel_paths:
        rel_paths.append("tests/scripts/test_kisage_clean_checkout.py")
    for rel in rel_paths:
        if rel in TRACKED_EXACT or rel.startswith(TRACKED_PREFIXES):
            src = REPO_ROOT / rel
            if src.is_file():
                dst = dst_root / rel
                dst.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(src, dst)


def load_module(path: pathlib.Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load module {name} from {path}")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


class KisageCleanCheckoutRegressionTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp_dir = tempfile.TemporaryDirectory(
            prefix="alt-kisage-clean-checkout-", dir="/tmp"
        )
        os.chmod(self.tmp_dir.name, 0o700)
        self.clean_root = pathlib.Path(self.tmp_dir.name)
        materialize_clean_tracked_worktree(self.clean_root)

        for rel in FORBIDDEN_LOCAL_PATHS:
            self.assertFalse(
                (self.clean_root / rel).exists(),
                f"clean checkout must not contain host-local path {rel}",
            )

    def tearDown(self) -> None:
        self.tmp_dir.cleanup()

    def test_portable_suite_succeeds_in_clean_checkout_without_host_local_files(
        self,
    ) -> None:
        verify_proc = subprocess.run(
            ["python3", str(self.clean_root / "scripts" / "kisage-env.py"), "verify"],
            cwd=str(self.clean_root),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(
            verify_proc.returncode,
            0,
            f"kisage-env.py verify failed in clean checkout: {verify_proc.stderr}",
        )

        suite_proc = subprocess.run(
            [
                "python3",
                str(self.clean_root / "scripts" / "tests" / "test-kisage-env.py"),
            ],
            cwd=str(self.clean_root),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(
            suite_proc.returncode,
            0,
            f"portable test-kisage-env.py failed in clean checkout:\nSTDOUT:\n{suite_proc.stdout}\nSTDERR:\n{suite_proc.stderr}",
        )
        self.assertIn("0 failed", suite_proc.stdout)
        self.assertIn(
            "portable mode did not inspect host-local .claude/settings.json or .mcp.json",
            suite_proc.stdout,
        )

        for rel in FORBIDDEN_LOCAL_PATHS:
            self.assertFalse(
                (self.clean_root / rel).exists(),
                f"portable suite must not create host-local path {rel}",
            )

    def test_explicit_host_config_audit_fails_closed_when_local_files_missing(
        self,
    ) -> None:
        cli_proc = subprocess.run(
            [
                "python3",
                str(self.clean_root / "scripts" / "kisage-env.py"),
                "audit-host-config",
            ],
            cwd=str(self.clean_root),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(
            cli_proc.returncode,
            1,
            f"audit-host-config must fail closed when local config files are absent: {cli_proc.stdout} {cli_proc.stderr}",
        )
        self.assertIn("settings.json", cli_proc.stderr)
        self.assertIn(".mcp.json", cli_proc.stderr)

        suite_audit_proc = subprocess.run(
            [
                "python3",
                str(self.clean_root / "scripts" / "tests" / "test-kisage-env.py"),
                "--audit-host-config",
            ],
            cwd=str(self.clean_root),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(
            suite_audit_proc.returncode,
            0,
            "test-kisage-env.py --audit-host-config must fail when host-local files are absent",
        )

    def test_explicit_host_config_audit_rejects_malformed_or_weak_config_without_leaking_secrets(
        self,
    ) -> None:
        kisage_env = load_module(
            self.clean_root / "scripts" / "kisage-env.py", "kisage_env_clean"
        )
        claude_path = self.clean_root / ".claude" / "settings.json"
        mcp_path = self.clean_root / ".mcp.json"
        claude_path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)

        secret_claude = "CLAUDE_CONFIG_SECRET_SENTINEL_918273"
        secret_mcp_raw = "MCP_RAW_SECRET_SENTINEL_645342"
        secret_mcp_env = "MCP_OTHER_SERVER_TOKEN_SENTINEL_112299"

        # 1. Malformed JSON with secret payloads
        claude_path.write_text(f'{{"broken": "{secret_claude}"\n', encoding="utf-8")
        mcp_path.write_text(f'{{"broken_mcp": "{secret_mcp_raw}"\n', encoding="utf-8")
        errs_malformed = kisage_env.audit_host_config(claude_path, mcp_path)
        self.assertTrue(len(errs_malformed) >= 2, repr(errs_malformed))
        joined_malformed = "\n".join(errs_malformed)
        self.assertNotIn(secret_claude, joined_malformed)
        self.assertNotIn(secret_mcp_raw, joined_malformed)

        # 2. Valid JSON but missing .kisage/* PreToolUse guard and missing KISAGE_HOST_CONFIG
        claude_path.write_text(
            json.dumps({"hooks": {"PreToolUse": [{"matcher": "Edit|Write", "hooks": []}]}}),
            encoding="utf-8",
        )
        mcp_path.write_text(
            json.dumps(
                {
                    "mcpServers": {
                        "other": {"command": "other", "env": {"API_KEY": secret_mcp_env}},
                        "kisage": {"command": "kisage", "args": ["mcp"], "env": {}},
                    }
                }
            ),
            encoding="utf-8",
        )
        proc_weak = subprocess.run(
            [
                "python3",
                str(self.clean_root / "scripts" / "kisage-env.py"),
                "audit-host-config",
            ],
            cwd=str(self.clean_root),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc_weak.returncode, 1)
        combined_weak = proc_weak.stdout + proc_weak.stderr
        self.assertNotIn(secret_mcp_env, combined_weak)
        self.assertIn("PreToolUse", combined_weak)
        self.assertIn("KISAGE_HOST_CONFIG", combined_weak)

        valid_claude_json = json.dumps(
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
        )
        valid_mcp_json = json.dumps(
            {
                "mcpServers": {
                    "kisage": {
                        "command": "kisage",
                        "args": ["mcp"],
                        "env": {
                            "KISAGE_HOST_CONFIG": str(
                                self.clean_root / ".kisage" / "host.json"
                            )
                        },
                    }
                }
            }
        )

        # 3. Valid JSON non-object roots: both null, single-file null, arrays, scalars
        claude_path.write_text("null\n", encoding="utf-8")
        mcp_path.write_text("null\n", encoding="utf-8")
        errs_both_null = kisage_env.audit_host_config(claude_path, mcp_path)
        self.assertEqual(len(errs_both_null), 2, repr(errs_both_null))

        claude_path.write_text("null\n", encoding="utf-8")
        mcp_path.write_text(valid_mcp_json, encoding="utf-8")
        errs_claude_null = kisage_env.audit_host_config(claude_path, mcp_path)
        self.assertEqual(len(errs_claude_null), 1, repr(errs_claude_null))
        self.assertIn("settings.json root must be a JSON object", errs_claude_null[0])

        claude_path.write_text(valid_claude_json, encoding="utf-8")
        mcp_path.write_text("null\n", encoding="utf-8")
        errs_mcp_null = kisage_env.audit_host_config(claude_path, mcp_path)
        self.assertEqual(len(errs_mcp_null), 1, repr(errs_mcp_null))
        self.assertIn(".mcp.json root must be a JSON object", errs_mcp_null[0])

        for non_obj in ("[]", "true", "123", '"scalar"'):
            claude_path.write_text(non_obj, encoding="utf-8")
            mcp_path.write_text(non_obj, encoding="utf-8")
            errs_non_obj = kisage_env.audit_host_config(claude_path, mcp_path)
            self.assertEqual(
                len(errs_non_obj),
                2,
                f"non-object root {non_obj!r} must fail for both files: {errs_non_obj!r}",
            )

        # 4. Negative PreToolUse fixtures: marker only in metadata, Read-only matcher, malformed regex, empty command
        mcp_path.write_text(valid_mcp_json, encoding="utf-8")
        negative_claude_payloads = [
            # Marker only in incidental metadata / comment field, not in command
            {
                "hooks": {
                    "PreToolUse": [
                        {
                            "matcher": "Edit|Write",
                            "description": "protects .kisage/*",
                            "hooks": [
                                {
                                    "type": "command",
                                    "command": "echo ok",
                                    "comment": ".kisage/*",
                                }
                            ],
                        }
                    ]
                }
            },
            # Matcher only covers Read, not both Edit and Write
            {
                "hooks": {
                    "PreToolUse": [
                        {
                            "matcher": "Read",
                            "hooks": [
                                {
                                    "type": "command",
                                    "command": 'case "$FILE" in .kisage/*) exit 1 ;; esac',
                                }
                            ],
                        }
                    ]
                }
            },
            # Matcher only covers Edit (missing Write)
            {
                "hooks": {
                    "PreToolUse": [
                        {
                            "matcher": "^Edit$",
                            "hooks": [
                                {
                                    "type": "command",
                                    "command": 'case "$FILE" in .kisage/*) exit 1 ;; esac',
                                }
                            ],
                        }
                    ]
                }
            },
            # Malformed regex in matcher
            {
                "hooks": {
                    "PreToolUse": [
                        {
                            "matcher": "[unclosed-regex",
                            "hooks": [
                                {
                                    "type": "command",
                                    "command": 'case "$FILE" in .kisage/*) exit 1 ;; esac',
                                }
                            ],
                        }
                    ]
                }
            },
            # Empty hook nodes / non-command type
            {
                "hooks": {
                    "PreToolUse": [
                        {
                            "matcher": "Edit|Write",
                            "hooks": [{"type": "prompt", "command": ".kisage/*"}],
                        }
                    ]
                }
            },
        ]
        for neg_payload in negative_claude_payloads:
            claude_path.write_text(json.dumps(neg_payload), encoding="utf-8")
            neg_errs = kisage_env.audit_host_config(claude_path, mcp_path)
            self.assertTrue(
                len(neg_errs) >= 1,
                f"expected negative PreToolUse payload to fail: {neg_payload!r}",
            )

        # 5. Negative .mcp.json fixtures: missing or empty command string
        claude_path.write_text(valid_claude_json, encoding="utf-8")
        for bad_cmd in (None, "", "   ", 123):
            srv_entry = {
                "args": ["mcp"],
                "env": {
                    "KISAGE_HOST_CONFIG": str(
                        self.clean_root / ".kisage" / "host.json"
                    )
                },
            }
            if bad_cmd is not None:
                srv_entry["command"] = bad_cmd
            mcp_path.write_text(
                json.dumps({"mcpServers": {"kisage": srv_entry}}),
                encoding="utf-8",
            )
            cmd_errs = kisage_env.audit_host_config(claude_path, mcp_path)
            self.assertTrue(
                any("command" in e for e in cmd_errs),
                f"expected missing/empty MCP command {bad_cmd!r} to fail: {cmd_errs!r}",
            )

    def test_explicit_host_config_audit_accepts_valid_synthetic_fixture(self) -> None:
        claude_path = self.clean_root / ".claude" / "settings.json"
        mcp_path = self.clean_root / ".mcp.json"
        claude_path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)

        # Synthetic fixture only (not actual host guarantee)
        claude_path.write_text(
            json.dumps(
                {
                    "hooks": {
                        "PreToolUse": [
                            {
                                "matcher": "Edit|Write|MultiEdit",
                                "hooks": [
                                    {
                                        "type": "command",
                                        "command": "case \"$FILE\" in .env|.kisage/*) exit 1 ;; esac",
                                    }
                                ],
                            }
                        ]
                    }
                }
            ),
            encoding="utf-8",
        )
        mcp_path.write_text(
            json.dumps(
                {
                    "mcpServers": {
                        "kisage": {
                            "command": "kisage",
                            "args": ["mcp"],
                            "env": {
                                "KISAGE_HOST_CONFIG": str(
                                    self.clean_root / ".kisage" / "host.json"
                                )
                            },
                        }
                    }
                }
            ),
            encoding="utf-8",
        )

        proc_ok = subprocess.run(
            [
                "python3",
                str(self.clean_root / "scripts" / "kisage-env.py"),
                "audit-host-config",
            ],
            cwd=str(self.clean_root),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(
            proc_ok.returncode,
            0,
            f"synthetic fixture audit failed: {proc_ok.stderr}",
        )
        self.assertIn(
            "hook execution and OS isolation not verified",
            proc_ok.stdout,
        )


if __name__ == "__main__":
    unittest.main()
