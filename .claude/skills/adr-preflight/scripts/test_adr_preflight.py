#!/usr/bin/env python3
"""Black-box unit and integration tests for adr_preflight.py."""

from __future__ import annotations

import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import textwrap
import time
import unittest
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
REAL_SCRIPT_PATH = SCRIPT_DIR / "adr_preflight.py"
REAL_REPO_ROOT = SCRIPT_DIR.parents[3]
VALIDATE_SUMMARY = "OK: 4 docs, 1 typed edges, no cycles"


def _render_adr(
    adr_id: str,
    title: str,
    affected_services_block: str,
    status: str = "accepted",
) -> str:
    header = textwrap.dedent(
        f"""\
        ---
        title: {title}
        date: 2026-01-15
        status: {status}
        """
    )
    body = textwrap.dedent(
        f"""\
        ---
        # ADR-{adr_id}: {title}

        ## Status

        {status.capitalize()}

        ## Context

        Context for {adr_id}.

        ## Decision

        Decision for {adr_id}.

        ## Consequences

        Consequences for {adr_id}.
        """
    )
    return f"{header}{affected_services_block}\n{body}"


class AdrPreflightTests(unittest.TestCase):
    """Hermetic tests against a temporary git repository and stub docdag."""

    def setUp(self) -> None:
        self._tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmpdir.cleanup)
        self.tmp_root = Path(self._tmpdir.name).resolve()

        self.repo = self.tmp_root / "repo"
        self.bin_dir = self.tmp_root / "bin"
        self.state_dir = self.tmp_root / "state"
        self.outside_dir = self.tmp_root / "outside"
        self.docdag_log = self.tmp_root / "docdag_invocations.jsonl"
        self.docdag_config = self.tmp_root / "docdag_config.json"

        self.repo.mkdir()
        self.bin_dir.mkdir()
        self.outside_dir.mkdir()
        (self.outside_dir / "external.py").write_text("x = 1\n", encoding="utf-8")

        (self.repo / "docdag.yaml").write_text(
            textwrap.dedent(
                """\
                preset: adr
                dir: docs/ADR
                id_width: 6
                filename: "{id}.md"
                """
            ),
            encoding="utf-8",
        )
        (self.repo / ".gitignore").write_text("ignored/\n", encoding="utf-8")

        adr_dir = self.repo / "docs" / "ADR"
        adr_dir.mkdir(parents=True)

        (adr_dir / "000001.md").write_text(
            _render_adr(
                "000001",
                "Backend baseline architecture",
                "affected_services: [alt-backend]",
            ),
            encoding="utf-8",
        )
        (adr_dir / "000002.md").write_text(
            _render_adr(
                "000002",
                "Harvester and MQ hub contract",
                'affected_services:\n  - alt-harvester\n  - "mq-hub"',
            ),
            encoding="utf-8",
        )
        (adr_dir / "000003.md").write_text(
            _render_adr(
                "000003",
                "Tag generator pipeline rules",
                "affected_services: [tag-generator]",
            ),
            encoding="utf-8",
        )
        (adr_dir / "000004.md").write_text(
            _render_adr(
                "000004",
                "Superseded backend record",
                "affected_services: [alt-backend]",
                status="superseded",
            ),
            encoding="utf-8",
        )

        fixture_scripts_dir = (
            self.repo / ".claude" / "skills" / "adr-preflight" / "scripts"
        )
        fixture_scripts_dir.mkdir(parents=True)
        self.fixture_script = fixture_scripts_dir / "adr_preflight.py"
        shutil.copy2(REAL_SCRIPT_PATH, self.fixture_script)

        self._git("init", "-q")
        self._git("config", "user.name", "Test User")
        self._git("config", "user.email", "test@example.com")
        self._git("config", "commit.gpgsign", "false")
        self._git("add", ".")
        self._git("commit", "-q", "-m", "initial fixture")

        self._binding_rows: list[list[str]] = [
            ["000001", "Backend baseline architecture", "docs/ADR/000001.md"],
            ["000002", "Harvester and MQ hub contract", "docs/ADR/000002.md"],
            ["000003", "Tag generator pipeline rules", "docs/ADR/000003.md"],
        ]
        self.configure_docdag()
        self._write_docdag_stub()

    def _git(self, *args: str) -> None:
        subprocess.run(
            ["git", *args],
            cwd=self.repo,
            check=True,
            capture_output=True,
            text=True,
        )

    def configure_docdag(
        self,
        *,
        validate_exit: int = 0,
        validate_stdout: str = f"{VALIDATE_SUMMARY}\n",
        validate_stderr: str = "",
        query_exit: int = 0,
        query_stderr: str = "",
        binding_rows: list[list[str]] | None = None,
    ) -> None:
        if binding_rows is not None:
            self._binding_rows = binding_rows
        payload = {
            "validate_exit": validate_exit,
            "validate_stdout": validate_stdout,
            "validate_stderr": validate_stderr,
            "query_exit": query_exit,
            "query_stderr": query_stderr,
            "binding_rows": self._binding_rows,
        }
        self.docdag_config.write_text(json.dumps(payload), encoding="utf-8")

    def _write_docdag_stub(self) -> None:
        stub_path = self.bin_dir / "docdag"
        stub_source = textwrap.dedent(
            f"""\
            #!{sys.executable}
            import json
            import os
            import pathlib
            import sys

            CONFIG_PATH = pathlib.Path({str(self.docdag_config)!r})
            LOG_PATH = pathlib.Path({str(self.docdag_log)!r})

            with LOG_PATH.open("a", encoding="utf-8") as handle:
                record = {{"argv": sys.argv[1:], "cwd": os.getcwd()}}
                handle.write(json.dumps(record) + "\\n")

            cfg = json.loads(CONFIG_PATH.read_text(encoding="utf-8"))
            args = sys.argv[1:]

            if "validate" in args:
                out = cfg.get("validate_stdout", "")
                err = cfg.get("validate_stderr", "")
                if out:
                    sys.stdout.write(out)
                if err:
                    sys.stderr.write(err)
                sys.exit(int(cfg.get("validate_exit", 0)))

            if "query" in args and "--binding" in args:
                q_exit = int(cfg.get("query_exit", 0))
                if q_exit != 0:
                    q_err = cfg.get("query_stderr", "query failed\\n")
                    if q_err:
                        sys.stderr.write(q_err)
                    sys.exit(q_exit)
                for row in cfg.get("binding_rows", []):
                    sys.stdout.write("\\t".join(row) + "\\n")
                sys.exit(0)

            sys.stderr.write(f"unexpected stub docdag args: {{args!r}}\\n")
            sys.exit(2)
            """
        )
        stub_path.write_text(stub_source, encoding="utf-8")
        stub_path.chmod(stub_path.stat().st_mode | stat.S_IXUSR)

    def add_binding_adr(
        self,
        adr_id: str,
        title: str,
        affected_services_inline: str,
        *,
        commit: bool = True,
    ) -> None:
        rel_path = f"docs/ADR/{adr_id}.md"
        (self.repo / rel_path).write_text(
            _render_adr(
                adr_id,
                title,
                f"affected_services: {affected_services_inline}",
            ),
            encoding="utf-8",
        )
        if commit:
            self._git("add", rel_path)
            self._git("commit", "-q", "-m", f"add {adr_id}")
        self._binding_rows.append([adr_id, title, rel_path])
        self.configure_docdag()

    def docdag_invocations(self) -> list[dict[str, object]]:
        if not self.docdag_log.exists():
            return []
        lines = self.docdag_log.read_text(encoding="utf-8").splitlines()
        return [json.loads(line) for line in lines if line.strip()]

    def build_path_without_docdag(self) -> str:
        essential_dir = self.tmp_root / "essential_bin"
        essential_dir.mkdir(exist_ok=True)
        git_bin = shutil.which("git")
        self.assertIsNotNone(git_bin, "git must exist on PATH")
        assert git_bin is not None
        for name, target in (
            ("git", Path(git_bin)),
            ("python3", Path(sys.executable)),
            ("python", Path(sys.executable)),
        ):
            link = essential_dir / name
            if not link.exists():
                link.symlink_to(target)

        entries = [str(essential_dir)]
        for raw_dir in os.environ.get("PATH", "").split(os.pathsep):
            if not raw_dir:
                continue
            if (Path(raw_dir) / "docdag").exists():
                continue
            entries.append(raw_dir)
        clean_path = os.pathsep.join(entries)
        self.assertIsNone(shutil.which("docdag", path=clean_path))
        self.assertIsNotNone(shutil.which("git", path=clean_path))
        return clean_path

    def run_preflight(
        self,
        *args: str,
        stdin_data: str | None = None,
        cwd: Path | None = None,
        env_overrides: dict[str, str | None] | None = None,
        path_value: str | None = None,
    ) -> subprocess.CompletedProcess[str]:
        env = os.environ.copy()
        env.pop("ALT_ADR_PREFLIGHT", None)
        env["ALT_ADR_PREFLIGHT_STATE_DIR"] = str(self.state_dir)
        env["GIT_CEILING_DIRECTORIES"] = str(self.tmp_root)
        if path_value is not None:
            env["PATH"] = path_value
        else:
            env["PATH"] = f"{self.bin_dir}{os.pathsep}{env.get('PATH', '')}"
        if env_overrides:
            for key, value in env_overrides.items():
                if value is None:
                    env.pop(key, None)
                else:
                    env[key] = value
        return subprocess.run(
            [sys.executable, str(self.fixture_script), *args],
            input=stdin_data,
            text=True,
            capture_output=True,
            cwd=cwd or self.repo,
            env=env,
            check=False,
        )

    def make_pretool_payload(
        self,
        file_path: str | Path,
        *,
        session_id: str = "sess-1",
        agent_id: str | None = None,
        tool_name: str = "Edit",
    ) -> str:
        payload: dict[str, object] = {
            "session_id": session_id,
            "cwd": str(self.repo),
            "hook_event_name": "PreToolUse",
            "tool_name": tool_name,
            "tool_input": {"file_path": str(file_path)},
        }
        if agent_id is not None:
            payload["agent_id"] = agent_id
        return json.dumps(payload)

    def make_session_start_payload(
        self,
        *,
        session_id: str = "sess-1",
        source: str = "startup",
    ) -> str:
        return json.dumps(
            {
                "session_id": session_id,
                "cwd": str(self.repo),
                "hook_event_name": "SessionStart",
                "source": source,
            }
        )

    def test_01_first_pretooluse_holds_and_lists_governing_adrs(self) -> None:
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        res = self.run_preflight("--hook", stdin_data=payload)

        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertEqual(res.stdout, "")
        self.assertTrue(
            res.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=res.stderr,
        )
        first_line = res.stderr.splitlines()[0]
        self.assertIn("alt-backend", first_line)
        self.assertIn(VALIDATE_SUMMARY, res.stderr)
        self.assertIn("  000002  Harvester and MQ hub contract", res.stderr)
        self.assertIn("  000001  Backend baseline architecture", res.stderr)
        self.assertLess(
            res.stderr.index("  000002  "),
            res.stderr.index("  000001  "),
        )
        self.assertNotIn("000003", res.stderr)
        self.assertNotIn("000004", res.stderr)
        self.assertIn("docdag context", res.stderr)
        self.assertIn("--depth 2 --budget 1500", res.stderr)
        self.assertIn("--service", res.stderr)
        self.assertIn("supersede", res.stderr.lower())
        self.assertIn("retry", res.stderr.lower())
        self.assertLess(len(res.stderr), 6000)

    def test_02_repeat_pretooluse_exits_0_without_invoking_docdag(self) -> None:
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        first = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(first.returncode, 2)
        self.assertTrue(first.stderr.startswith("ADR PREFLIGHT (not an error)"))

        calls_after_first = len(self.docdag_invocations())
        self.assertGreater(calls_after_first, 0)

        second = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(second.returncode, 0, msg=second.stderr)
        self.assertEqual(second.stdout, "")
        self.assertEqual(second.stderr, "")
        self.assertEqual(len(self.docdag_invocations()), calls_after_first)

    def test_03_different_scope_in_same_session_is_held(self) -> None:
        first_payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        first = self.run_preflight("--hook", stdin_data=first_payload)
        self.assertEqual(first.returncode, 2)
        self.assertTrue(first.stderr.startswith("ADR PREFLIGHT (not an error)"))

        second_payload = self.make_pretool_payload(
            self.repo / "tag-generator/app/main.py"
        )
        second = self.run_preflight("--hook", stdin_data=second_payload)
        self.assertEqual(second.returncode, 2, msg=second.stderr)
        self.assertTrue(
            second.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=second.stderr,
        )
        self.assertIn("tag-generator", second.stderr.splitlines()[0])
        self.assertIn("  000003  Tag generator pipeline rules", second.stderr)
        self.assertNotIn("  000001  ", second.stderr)
        self.assertNotIn("  000002  ", second.stderr)

    def test_04_same_scope_with_different_agent_id_is_held_again(self) -> None:
        main_payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        first = self.run_preflight("--hook", stdin_data=main_payload)
        self.assertEqual(first.returncode, 2)
        self.assertTrue(first.stderr.startswith("ADR PREFLIGHT (not an error)"))

        subagent_payload = self.make_pretool_payload(
            self.repo / "alt-backend/app/x.go",
            agent_id="worker-subagent-1",
        )
        second = self.run_preflight("--hook", stdin_data=subagent_payload)
        self.assertEqual(second.returncode, 2, msg=second.stderr)
        self.assertTrue(
            second.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=second.stderr,
        )

    def test_05_modifying_adr_corpus_invalidates_receipt(self) -> None:
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        first = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(first.returncode, 2)
        self.assertTrue(first.stderr.startswith("ADR PREFLIGHT (not an error)"))

        second = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(second.returncode, 0)

        adr1 = self.repo / "docs/ADR/000001.md"
        adr1.write_text(
            adr1.read_text(encoding="utf-8") + "\nClarified invariant.\n",
            encoding="utf-8",
        )

        third = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(third.returncode, 2, msg=third.stderr)
        self.assertTrue(
            third.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=third.stderr,
        )

    def test_06_validate_exit_1_blocks_and_retry_remains_blocked(self) -> None:
        self.configure_docdag(
            validate_exit=1,
            validate_stdout="ERROR docs/ADR/000001.md: missing section Consequences\n",
        )
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")

        first = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(first.returncode, 2, msg=first.stderr)
        self.assertTrue(
            first.stderr.startswith("ADR PREFLIGHT BLOCKED"),
            msg=first.stderr,
        )
        self.assertIn(
            "ERROR docs/ADR/000001.md: missing section Consequences",
            first.stderr,
        )
        fix_lines = [
            line for line in first.stderr.splitlines() if line.startswith("FIX:")
        ]
        self.assertTrue(fix_lines, msg=first.stderr)
        self.assertIn("docdag validate", fix_lines[0])

        retry = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(retry.returncode, 2, msg=retry.stderr)
        self.assertTrue(
            retry.stderr.startswith("ADR PREFLIGHT BLOCKED"),
            msg=retry.stderr,
        )

    def test_07_validate_exit_3_fix_mentions_stale_config_or_binary(self) -> None:
        self.configure_docdag(
            validate_exit=3,
            validate_stdout="docdag: unknown configuration field\n",
        )
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        res = self.run_preflight("--hook", stdin_data=payload)

        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(
            res.stderr.startswith("ADR PREFLIGHT BLOCKED"),
            msg=res.stderr,
        )
        fix_lines = [
            line for line in res.stderr.splitlines() if line.startswith("FIX:")
        ]
        self.assertTrue(fix_lines, msg=res.stderr)
        fix_lower = fix_lines[0].lower()
        self.assertIn("stale", fix_lower)
        self.assertTrue(
            "config" in fix_lower or "binary" in fix_lower,
            msg=fix_lines[0],
        )

    def test_08_missing_docdag_on_path_exits_2_with_install_fix(self) -> None:
        wf_dir = self.repo / ".github" / "workflows"
        wf_dir.mkdir(parents=True, exist_ok=True)
        (wf_dir / "adr-graph.yml").write_text(
            "env:\n  DOCDAG_VERSION: v9.9.9\n",
            encoding="utf-8",
        )
        no_docdag_path = self.build_path_without_docdag()
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        res = self.run_preflight(
            "--hook",
            stdin_data=payload,
            path_value=no_docdag_path,
        )

        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(
            res.stderr.startswith("ADR PREFLIGHT BLOCKED"),
            msg=res.stderr,
        )
        self.assertIn("docdag is not on PATH", res.stderr)
        fix_lines = [
            line for line in res.stderr.splitlines() if line.startswith("FIX:")
        ]
        self.assertTrue(fix_lines, msg=res.stderr)
        self.assertIn(
            "go install github.com/Kaikei-e/DocDag/cmd/docdag@v9.9.9",
            fix_lines[0],
        )
        self.assertIn("DOCDAG_VERSION", fix_lines[0])
        self.assertIn(".github/workflows/adr-graph.yml", fix_lines[0])

    def test_09_kill_switch_disables_pretooluse_and_reports_on_session_start(
        self,
    ) -> None:
        disabled_env = {"ALT_ADR_PREFLIGHT": "disabled"}
        pre_res = self.run_preflight(
            "--hook",
            stdin_data=self.make_pretool_payload(self.repo / "alt-backend/app/x.go"),
            env_overrides=disabled_env,
        )
        self.assertEqual(pre_res.returncode, 0, msg=pre_res.stderr)
        self.assertEqual(pre_res.stdout, "")
        self.assertEqual(pre_res.stderr, "")
        self.assertEqual(self.docdag_invocations(), [])

        start_res = self.run_preflight(
            "--hook",
            stdin_data=self.make_session_start_payload(),
            env_overrides=disabled_env,
        )
        self.assertEqual(start_res.returncode, 0, msg=start_res.stderr)
        self.assertIn("adr_preflight_disabled", start_res.stdout)
        self.assertEqual(self.docdag_invocations(), [])

    def test_10_exempt_paths_exit_0_without_invoking_docdag(self) -> None:
        exempt_targets = [
            self.repo / "docs/ADR/000005.md",
            self.outside_dir / "external.py",
            self.repo / "ignored/file.txt",
        ]
        for target in exempt_targets:
            with self.subTest(target=str(target)):
                res = self.run_preflight(
                    "--hook",
                    stdin_data=self.make_pretool_payload(target),
                )
                self.assertEqual(res.returncode, 0, msg=res.stderr)
                self.assertEqual(res.stdout, "")
                self.assertEqual(res.stderr, "")
        self.assertEqual(self.docdag_invocations(), [])

    def test_11_root_level_file_gets_repo_root_scope_with_zero_adr_wording(
        self,
    ) -> None:
        res = self.run_preflight(
            "--hook",
            stdin_data=self.make_pretool_payload(self.repo / "Makefile"),
        )
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(
            res.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=res.stderr,
        )
        self.assertIn("repo-root", res.stderr.splitlines()[0])
        self.assertIn("Scope repo-root: 0 governing binding ADRs", res.stderr)
        self.assertIn("docdag query --binding --fields id,title", res.stderr)

    def test_12_real_worktree_uses_worktree_root_and_isolated_receipt(self) -> None:
        main_edit = self.make_pretool_payload(self.repo / "alt-backend/main.go")
        main_held = self.run_preflight("--hook", stdin_data=main_edit)
        self.assertEqual(main_held.returncode, 2, msg=main_held.stderr)
        self.assertTrue(main_held.stderr.startswith("ADR PREFLIGHT (not an error)"))

        main_retry = self.run_preflight("--hook", stdin_data=main_edit)
        self.assertEqual(main_retry.returncode, 0, msg=main_retry.stderr)

        worktree_root = self.repo / ".claude" / "worktrees" / "w1"
        self._git("worktree", "add", "-q", "-b", "wt-w1", str(worktree_root))
        self.assertTrue((worktree_root / "docs" / "ADR" / "000001.md").is_file())

        wt_payload = self.make_pretool_payload(worktree_root / "alt-backend/x.go")
        res = self.run_preflight("--hook", stdin_data=wt_payload)
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(
            res.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=res.stderr,
        )
        first_line = res.stderr.splitlines()[0]
        self.assertIn("alt-backend", first_line)
        self.assertNotIn(".claude", first_line)
        self.assertIn("  000002  ", res.stderr)
        self.assertIn("  000001  ", res.stderr)
        self.assertEqual(
            self.docdag_invocations()[-1]["cwd"],
            str(worktree_root.resolve()),
        )

        wt_followup = self.run_preflight("--hook", stdin_data=wt_payload)
        self.assertEqual(wt_followup.returncode, 0, msg=wt_followup.stderr)

    def test_13_codex_apply_patch_holds_both_scopes_then_allows_followup(self) -> None:
        patch_cmd = textwrap.dedent(
            """\
            *** Begin Patch
            *** Update File: tag-generator/a.py
            @@
            -old = 1
            +old = 2
            *** Add File: mq-hub/b.go
            +package main
            *** End Patch
            """
        )
        codex_payload = json.dumps(
            {
                "session_id": "sess-codex",
                "cwd": str(self.repo),
                "hook_event_name": "PreToolUse",
                "tool_name": "apply_patch",
                "tool_input": {"command": patch_cmd},
            }
        )
        first = self.run_preflight("--hook", stdin_data=codex_payload)
        self.assertEqual(first.returncode, 2, msg=first.stderr)
        self.assertTrue(
            first.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=first.stderr,
        )
        first_line = first.stderr.splitlines()[0]
        self.assertIn("tag-generator", first_line)
        self.assertIn("mq-hub", first_line)
        self.assertIn("  000003  ", first.stderr)
        self.assertIn("  000002  ", first.stderr)

        claude_followup = self.make_pretool_payload(
            self.repo / "mq-hub/c.go",
            session_id="sess-codex",
        )
        second = self.run_preflight("--hook", stdin_data=claude_followup)
        self.assertEqual(second.returncode, 0, msg=second.stderr)
        self.assertEqual(second.stderr, "")

    def test_14_github_directory_uses_ci_cd_alias(self) -> None:
        self.add_binding_adr("000005", "CI workflow pinning policy", "[ci-cd]")
        res = self.run_preflight(
            "--hook",
            stdin_data=self.make_pretool_payload(
                self.repo / ".github/workflows/x.yml"
            ),
        )
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(
            res.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=res.stderr,
        )
        self.assertIn(".github", res.stderr.splitlines()[0])
        self.assertIn("  000005  CI workflow pinning policy", res.stderr)

    def test_15_session_start_prints_summary_and_clears_session_receipts(
        self,
    ) -> None:
        edit_payload = self.make_pretool_payload(
            self.repo / "alt-backend/app/x.go",
            session_id="sess-restart",
        )
        held = self.run_preflight("--hook", stdin_data=edit_payload)
        self.assertEqual(held.returncode, 2)
        self.assertTrue(held.stderr.startswith("ADR PREFLIGHT (not an error)"))

        allowed = self.run_preflight("--hook", stdin_data=edit_payload)
        self.assertEqual(allowed.returncode, 0)

        other_session = self.state_dir / "other-session"
        (other_session / "main").mkdir(parents=True)
        (other_session / "main" / "alt-backend").write_text("old", encoding="utf-8")
        eight_days_ago = time.time() - 8 * 86400
        os.utime(other_session, (eight_days_ago, eight_days_ago))

        start = self.run_preflight(
            "--hook",
            stdin_data=self.make_session_start_payload(session_id="sess-restart"),
        )
        self.assertEqual(start.returncode, 0, msg=start.stderr)
        self.assertTrue(
            start.stdout.startswith("adr_preflight_enabled"),
            msg=start.stdout,
        )
        self.assertIn(VALIDATE_SUMMARY, start.stdout)
        self.assertTrue(other_session.exists())

        held_again = self.run_preflight("--hook", stdin_data=edit_payload)
        self.assertEqual(held_again.returncode, 2, msg=held_again.stderr)
        self.assertTrue(
            held_again.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=held_again.stderr,
        )

    def test_16_session_start_when_validate_fails_exits_0_and_warns_blocked(
        self,
    ) -> None:
        self.configure_docdag(
            validate_exit=1,
            validate_stdout="ERROR docs/ADR/000002.md: dangling link [[999999]]\n",
        )
        res = self.run_preflight(
            "--hook",
            stdin_data=self.make_session_start_payload(),
        )
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        self.assertTrue(
            res.stdout.startswith("adr_preflight_enabled"),
            msg=res.stdout,
        )
        self.assertIn("blocked", res.stdout.lower())
        self.assertIn(
            "ERROR docs/ADR/000002.md: dangling link [[999999]]",
            res.stdout,
        )

    def test_17_malformed_stdin_on_hook_exits_0(self) -> None:
        res = self.run_preflight("--hook", stdin_data="{not valid json")
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        self.assertEqual(res.stdout, "")
        self.assertEqual(res.stderr, "")

    def test_18_cli_service_lists_exact_matching_binding_rows(self) -> None:
        backend_res = self.run_preflight("--service", "alt-backend")
        self.assertEqual(backend_res.returncode, 0, msg=backend_res.stderr)
        lines = backend_res.stdout.splitlines()
        self.assertEqual(len(lines), 1, msg=backend_res.stdout)
        cols = lines[0].split("\t")
        self.assertEqual(
            cols,
            ["000001", "Backend baseline architecture", "docs/ADR/000001.md"],
        )

        none_res = self.run_preflight("--service", "nosuch")
        self.assertEqual(none_res.returncode, 0, msg=none_res.stderr)
        self.assertEqual(none_res.stdout, "")

    def test_19_cli_path_exits_1_when_validate_fails_and_0_with_brief_when_valid(
        self,
    ) -> None:
        self.configure_docdag(
            validate_exit=1,
            validate_stdout="ERROR docs/ADR/000001.md: cycle detected\n",
        )
        bad = self.run_preflight("alt-backend/app/x.go")
        self.assertEqual(bad.returncode, 1, msg=bad.stderr)
        self.assertIn(
            "ERROR docs/ADR/000001.md: cycle detected",
            bad.stdout,
        )

        self.configure_docdag(validate_exit=0)
        good = self.run_preflight("alt-backend/app/x.go")
        self.assertEqual(good.returncode, 0, msg=good.stderr)
        self.assertIn(VALIDATE_SUMMARY, good.stdout)
        self.assertIn("alt-backend", good.stdout)
        self.assertIn("  000002  Harvester and MQ hub contract", good.stdout)
        self.assertIn("  000001  Backend baseline architecture", good.stdout)
        self.assertNotIn("000003", good.stdout)
        self.assertNotIn("000004", good.stdout)
        self.assertIn("docdag context", good.stdout)
        self.assertIn("--depth 2 --budget 1500", good.stdout)
        self.assertIn("--service", good.stdout)
        self.assertIn("supersede", good.stdout.lower())
        self.assertNotIn("ADR PREFLIGHT (not an error)", good.stdout)
        self.assertNotIn("retry", good.stdout.lower())

    def test_20_brief_with_30_binding_adrs_lists_only_8_newest_and_states_count(
        self,
    ) -> None:
        for num in range(101, 131):
            adr_id = f"{num:06d}"
            title = (
                f"Search indexer governance decision {adr_id} "
                f"{'detail-' * 12}".strip()
            )
            self.add_binding_adr(
                adr_id,
                title,
                "[search-indexer]",
                commit=False,
            )
        self._git("add", "docs/ADR")
        self._git("commit", "-q", "-m", "add 30 search-indexer ADRs")

        res = self.run_preflight(
            "--hook",
            stdin_data=self.make_pretool_payload(
                self.repo / "search-indexer/app/main.go"
            ),
        )
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(
            res.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=res.stderr,
        )
        self.assertIn(
            "Scope search-indexer: 30 governing binding ADRs (8 newest shown):",
            res.stderr,
        )
        for newest_num in range(123, 131):
            self.assertIn(f"  {newest_num:06d}  ", res.stderr)
        for older_num in range(101, 123):
            self.assertNotIn(f"{older_num:06d}", res.stderr)
        self.assertLess(len(res.stderr), 6000)

    def test_21_committing_docdag_yaml_only_change_reholds_scope(self) -> None:
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        first = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(first.returncode, 2)
        self.assertTrue(first.stderr.startswith("ADR PREFLIGHT (not an error)"))

        second = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(second.returncode, 0, msg=second.stderr)

        cfg_file = self.repo / "docdag.yaml"
        cfg_file.write_text(
            cfg_file.read_text(encoding="utf-8") + "acyclic_union: true\n",
            encoding="utf-8",
        )
        self._git("add", "docdag.yaml")
        self._git("commit", "-q", "-m", "update docdag.yaml only")

        third = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(third.returncode, 2, msg=third.stderr)
        self.assertTrue(
            third.stderr.startswith("ADR PREFLIGHT (not an error)"),
            msg=third.stderr,
        )

    def test_22_state_dir_as_regular_file_reports_internal_error(self) -> None:
        blocker_file = self.tmp_root / "state_blocker_file"
        blocker_file.write_text("not a directory\n", encoding="utf-8")

        res = self.run_preflight(
            "--hook",
            stdin_data=self.make_pretool_payload(self.repo / "alt-backend/app/x.go"),
            env_overrides={"ALT_ADR_PREFLIGHT_STATE_DIR": str(blocker_file)},
        )
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(
            res.stderr.startswith("ADR PREFLIGHT BLOCKED"),
            msg=res.stderr,
        )
        self.assertIn("internal error", res.stderr)
        self.assertIn("ALT_ADR_PREFLIGHT=disabled", res.stderr)

    def test_23_unrelated_git_repo_is_exempt(self) -> None:
        other_repo = self.tmp_root / "other_repo"
        (other_repo / "docs" / "ADR").mkdir(parents=True)
        (other_repo / "docdag.yaml").write_text(
            'preset: adr\ndir: docs/ADR\nid_width: 6\nfilename: "{id}.md"\n',
            encoding="utf-8",
        )
        (other_repo / "docs" / "ADR" / "000001.md").write_text(
            _render_adr(
                "000001",
                "Other repo decision",
                "affected_services: [other-svc]",
            ),
            encoding="utf-8",
        )
        for git_args in (
            ("init", "-q"),
            ("config", "user.name", "Test User"),
            ("config", "user.email", "test@example.com"),
            ("config", "commit.gpgsign", "false"),
            ("add", "."),
            ("commit", "-q", "-m", "init other repo"),
        ):
            subprocess.run(
                ["git", *git_args],
                cwd=other_repo,
                check=True,
                capture_output=True,
                text=True,
            )

        payload = json.dumps(
            {
                "session_id": "sess-other",
                "cwd": str(other_repo),
                "hook_event_name": "PreToolUse",
                "tool_name": "Edit",
                "tool_input": {"file_path": str(other_repo / "other-svc/app.py")},
            }
        )
        res = self.run_preflight("--hook", stdin_data=payload, cwd=other_repo)
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        self.assertEqual(res.stdout, "")
        self.assertEqual(res.stderr, "")
        self.assertEqual(self.docdag_invocations(), [])

        cli_res = self.run_preflight(
            str(other_repo / "other-svc/app.py"),
            cwd=other_repo,
        )
        self.assertEqual(cli_res.returncode, 0, msg=cli_res.stderr)
        self.assertIn("No non-exempt scopes", cli_res.stdout)

    def test_24_session_start_validates_script_checkout_not_payload_cwd(self) -> None:
        payload = json.dumps(
            {
                "session_id": "sess-cwd-mismatch",
                "cwd": str(self.outside_dir),
                "hook_event_name": "SessionStart",
                "source": "startup",
            }
        )
        res = self.run_preflight("--hook", stdin_data=payload, cwd=self.outside_dir)
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        self.assertTrue(
            res.stdout.startswith("adr_preflight_enabled"),
            msg=res.stdout,
        )
        self.assertIn(VALIDATE_SUMMARY, res.stdout)
        self.assertEqual(
            self.docdag_invocations()[-1]["cwd"],
            str(self.repo.resolve()),
        )

    def test_25_docdag_yaml_is_exempt_even_when_validate_exits_3(self) -> None:
        self.configure_docdag(
            validate_exit=3,
            validate_stdout="docdag: invalid config\n",
        )
        cfg_res = self.run_preflight(
            "--hook",
            stdin_data=self.make_pretool_payload(self.repo / "docdag.yaml"),
        )
        self.assertEqual(cfg_res.returncode, 0, msg=cfg_res.stderr)
        self.assertEqual(cfg_res.stdout, "")
        self.assertEqual(cfg_res.stderr, "")
        self.assertEqual(self.docdag_invocations(), [])

        cli_cfg = self.run_preflight("docdag.yaml")
        self.assertEqual(cli_cfg.returncode, 1)  # validate fails on fallback root

        self.configure_docdag(validate_exit=0)
        cli_clean = self.run_preflight("docdag.yaml")
        self.assertEqual(cli_clean.returncode, 0, msg=cli_clean.stderr)
        self.assertIn("No non-exempt scopes", cli_clean.stdout)

        self.configure_docdag(
            validate_exit=3,
            validate_stdout="docdag: invalid config\n",
        )
        other_root = self.run_preflight(
            "--hook",
            stdin_data=self.make_pretool_payload(self.repo / "Makefile"),
        )
        self.assertEqual(other_root.returncode, 2, msg=other_root.stderr)
        self.assertTrue(other_root.stderr.startswith("ADR PREFLIGHT BLOCKED"))

    def test_26_patch_spanning_main_and_worktree_same_scope_keys_receipt_by_root(
        self,
    ) -> None:
        worktree_root = self.repo / ".claude" / "worktrees" / "w-shared"
        self._git("worktree", "add", "-q", "-b", "wt-shared", str(worktree_root))

        patch_cmd = textwrap.dedent(
            f"""\
            *** Begin Patch
            *** Add File: {self.repo / "alt-backend/a.go"}
            +package a
            *** Add File: {worktree_root / "alt-backend/b.go"}
            +package b
            *** End Patch
            """
        )
        payload = json.dumps(
            {
                "session_id": "sess-multi-root",
                "cwd": str(self.repo),
                "hook_event_name": "PreToolUse",
                "tool_name": "apply_patch",
                "tool_input": {"command": patch_cmd},
            }
        )
        first = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(first.returncode, 2, msg=first.stderr)
        self.assertTrue(first.stderr.startswith("ADR PREFLIGHT (not an error)"))

        calls_after_first = len(self.docdag_invocations())
        second = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(second.returncode, 0, msg=second.stderr)
        self.assertEqual(second.stderr, "")
        self.assertEqual(len(self.docdag_invocations()), calls_after_first)

    def test_27_second_edit_to_already_dirty_adr_reholds_scope(self) -> None:
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        self.assertEqual(self.run_preflight("--hook", stdin_data=payload).returncode, 2)
        self.assertEqual(self.run_preflight("--hook", stdin_data=payload).returncode, 0)

        adr1 = self.repo / "docs/ADR/000001.md"
        original = adr1.read_text(encoding="utf-8")
        adr1.write_text(original + "\nFirst dirty edit.\n", encoding="utf-8")

        self.assertEqual(self.run_preflight("--hook", stdin_data=payload).returncode, 2)
        self.assertEqual(self.run_preflight("--hook", stdin_data=payload).returncode, 0)

        adr1.write_text(original + "\nSecond dirty edit with new bytes.\n", encoding="utf-8")
        reheld = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(reheld.returncode, 2, msg=reheld.stderr)
        self.assertTrue(reheld.stderr.startswith("ADR PREFLIGHT (not an error)"))

    def test_28_uncommitted_docdag_yaml_causes_fingerprint_internal_error(self) -> None:
        self._git("rm", "--cached", "-q", "docdag.yaml")
        self._git("commit", "-q", "-m", "drop docdag.yaml from HEAD")

        res = self.run_preflight(
            "--hook",
            stdin_data=self.make_pretool_payload(self.repo / "alt-backend/app/x.go"),
        )
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(res.stderr.startswith("ADR PREFLIGHT BLOCKED"), msg=res.stderr)
        self.assertIn("internal error", res.stderr)
        self.assertIn("ALT_ADR_PREFLIGHT=disabled", res.stderr)

    def test_29_session_start_internal_exception_exits_0_with_message(self) -> None:
        outside_script = self.outside_dir / "adr_preflight.py"
        shutil.copy2(REAL_SCRIPT_PATH, outside_script)
        env = os.environ.copy()
        env.pop("ALT_ADR_PREFLIGHT", None)
        env["ALT_ADR_PREFLIGHT_STATE_DIR"] = str(self.state_dir)
        env["GIT_CEILING_DIRECTORIES"] = str(self.tmp_root)

        res = subprocess.run(
            [sys.executable, str(outside_script), "--hook"],
            input=self.make_session_start_payload(),
            text=True,
            capture_output=True,
            cwd=self.outside_dir,
            env=env,
            check=False,
        )
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        self.assertTrue(
            res.stdout.startswith("adr_preflight_enabled: internal error ("),
            msg=res.stdout,
        )
        self.assertIn("code edits will be blocked until it is fixed", res.stdout)

    def test_30_service_missing_adr_file_does_not_misreport_missing_docdag(
        self,
    ) -> None:
        self.configure_docdag(
            binding_rows=[
                ["000999", "Ghost ADR", "docs/ADR/000999.md"],
            ]
        )
        bad_file = self.run_preflight("--service", "alt-backend")
        self.assertEqual(bad_file.returncode, 2, msg=bad_file.stderr)
        self.assertNotIn("docdag is not on PATH", bad_file.stderr)

        no_docdag_path = self.build_path_without_docdag()
        missing_bin = self.run_preflight(
            "--service",
            "alt-backend",
            path_value=no_docdag_path,
        )
        self.assertEqual(missing_bin.returncode, 2, msg=missing_bin.stderr)
        self.assertIn("docdag is not on PATH", missing_bin.stderr)

    def test_31_brief_truncates_over_6000_chars_with_12_scopes(self) -> None:
        scopes = [f"service-{idx:02d}" for idx in range(1, 13)]
        adr_counter = 100
        for scope in scopes:
            for item in range(8):
                adr_counter += 1
                adr_id = f"{adr_counter:06d}"
                title = f"Scope {scope} item {item} " + ("x" * 135)
                self.add_binding_adr(adr_id, title, f"[{scope}]", commit=False)
        self._git("add", "docs/ADR")
        self._git("commit", "-q", "-m", "add 96 ADRs across 12 scopes")

        patch_lines = ["*** Begin Patch"]
        for scope in scopes:
            patch_lines.append(f"*** Add File: {scope}/main.go")
            patch_lines.append("+package main")
        patch_lines.append("*** End Patch")

        payload = json.dumps(
            {
                "session_id": "sess-trunc",
                "cwd": str(self.repo),
                "hook_event_name": "PreToolUse",
                "tool_name": "apply_patch",
                "tool_input": {"command": "\n".join(patch_lines)},
            }
        )
        res = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(res.stderr.startswith("ADR PREFLIGHT (not an error)"))
        self.assertLess(len(res.stderr), 6000)
        self.assertIn(
            "... (brief truncated; run the --service command per scope)",
            res.stderr,
        )
        self.assertIn("docdag context <id> --depth 2 --budget 1500", res.stderr)
        self.assertIn("List all for a service:", res.stderr)
        self.assertIn(
            "If a planned change contradicts a binding ADR, stop and supersede it "
            "with a new ADR rather than contradicting it silently.",
            res.stderr,
        )
        last_line = res.stderr.strip().splitlines()[-1]
        self.assertIn("retrying the same edit now succeeds", last_line)

    def test_32_relative_file_path_resolves_against_payload_cwd(self) -> None:
        sub_cwd = self.repo / "alt-backend"
        sub_cwd.mkdir()
        payload = json.dumps(
            {
                "session_id": "sess-rel",
                "cwd": str(sub_cwd),
                "hook_event_name": "PreToolUse",
                "tool_name": "Edit",
                "tool_input": {"file_path": "app/relative.go"},
            }
        )
        res = self.run_preflight("--hook", stdin_data=payload, cwd=self.outside_dir)
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(res.stderr.startswith("ADR PREFLIGHT (not an error)"))
        self.assertIn("alt-backend", res.stderr.splitlines()[0])
        self.assertIn("  000002  ", res.stderr)

    def test_33_notebook_path_payload_is_gated(self) -> None:
        payload = json.dumps(
            {
                "session_id": "sess-nb",
                "cwd": str(self.repo),
                "hook_event_name": "PreToolUse",
                "tool_name": "NotebookEdit",
                "tool_input": {
                    "notebook_path": str(self.repo / "tag-generator/explore.ipynb"),
                },
            }
        )
        res = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        self.assertTrue(res.stderr.startswith("ADR PREFLIGHT (not an error)"))
        self.assertIn("tag-generator", res.stderr.splitlines()[0])
        self.assertIn("  000003  ", res.stderr)

    def test_34_apply_patch_delete_and_move_to_are_gated(self) -> None:
        patch_cmd = textwrap.dedent(
            """\
            *** Begin Patch
            *** Delete File: alt-backend/old.go
            *** Update File: tag-generator/src.py
            *** Move to: mq-hub/moved.py
            @@
            -a = 1
            +a = 2
            *** End Patch
            """
        )
        payload = json.dumps(
            {
                "session_id": "sess-del-move",
                "cwd": str(self.repo),
                "hook_event_name": "PreToolUse",
                "tool_name": "apply_patch",
                "tool_input": {"command": patch_cmd},
            }
        )
        res = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(res.returncode, 2, msg=res.stderr)
        first_line = res.stderr.splitlines()[0]
        self.assertIn("alt-backend", first_line)
        self.assertIn("tag-generator", first_line)
        self.assertIn("mq-hub", first_line)

    def test_35_cli_changed_reports_service_scope_only_and_exits_1_on_invalid(
        self,
    ) -> None:
        svc_file = self.repo / "tag-generator" / "worker.py"
        svc_file.parent.mkdir(parents=True)
        svc_file.write_text("v = 1\n", encoding="utf-8")
        self._git("add", "tag-generator/worker.py")
        self._git("commit", "-q", "-m", "add tracked service file")

        svc_file.write_text("v = 2\n", encoding="utf-8")
        (self.repo / "docs" / "notes.md").write_text("untracked\n", encoding="utf-8")

        good = self.run_preflight("--changed")
        self.assertEqual(good.returncode, 0, msg=good.stderr)
        self.assertIn("Scope tag-generator:", good.stdout)
        self.assertIn("  000003  ", good.stdout)
        self.assertNotIn("Scope docs:", good.stdout)

        self.configure_docdag(
            validate_exit=1,
            validate_stdout="ERROR docs/ADR/000001.md: broken\n",
        )
        bad = self.run_preflight("--changed")
        self.assertEqual(bad.returncode, 1, msg=bad.stderr)
        self.assertIn("ERROR docs/ADR/000001.md: broken", bad.stdout)

    def test_36_pretooluse_query_failure_blocks_without_writing_receipt(self) -> None:
        self.configure_docdag(query_exit=1, query_stderr="query boom\n")
        payload = self.make_pretool_payload(self.repo / "alt-backend/app/x.go")
        bad = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(bad.returncode, 2, msg=bad.stderr)
        self.assertTrue(bad.stderr.startswith("ADR PREFLIGHT BLOCKED"), msg=bad.stderr)
        self.assertIn("internal error", bad.stderr)

        self.configure_docdag(query_exit=0)
        retry = self.run_preflight("--hook", stdin_data=payload)
        self.assertEqual(retry.returncode, 2, msg=retry.stderr)
        self.assertTrue(retry.stderr.startswith("ADR PREFLIGHT (not an error)"), msg=retry.stderr)

    def test_37_session_start_missing_docdag_exits_0_and_carries_fix(self) -> None:
        wf_dir = self.repo / ".github" / "workflows"
        wf_dir.mkdir(parents=True, exist_ok=True)
        (wf_dir / "adr-graph.yml").write_text(
            "env:\n  DOCDAG_VERSION: v9.9.9\n",
            encoding="utf-8",
        )
        no_docdag_path = self.build_path_without_docdag()
        res = self.run_preflight(
            "--hook",
            stdin_data=self.make_session_start_payload(),
            path_value=no_docdag_path,
        )
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        self.assertTrue(res.stdout.startswith("adr_preflight_enabled"), msg=res.stdout)
        self.assertIn("blocked", res.stdout.lower())
        self.assertIn("FIX:", res.stdout)
        self.assertIn("docdag@v9.9.9", res.stdout)


@unittest.skipUnless(shutil.which("docdag"), "docdag binary not found on PATH")
class AdrPreflightRealRepoIntegrationTests(unittest.TestCase):
    """Integration check against the real repository ADR graph."""

    def test_real_repo_service_alt_backend_contains_001010(self) -> None:
        self.assertTrue((REAL_REPO_ROOT / "docdag.yaml").is_file())
        with tempfile.TemporaryDirectory() as tmp_state:
            env = os.environ.copy()
            env.pop("ALT_ADR_PREFLIGHT", None)
            env["ALT_ADR_PREFLIGHT_STATE_DIR"] = tmp_state
            res = subprocess.run(
                [sys.executable, str(REAL_SCRIPT_PATH), "--service", "alt-backend"],
                text=True,
                capture_output=True,
                cwd=REAL_REPO_ROOT,
                env=env,
                check=False,
            )
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        self.assertIn("001010", res.stdout)


if __name__ == "__main__":
    unittest.main()
