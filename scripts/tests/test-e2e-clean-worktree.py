#!/usr/bin/env python3
"""Deterministic ARCH-05 verification: Playwright E2E must not dirty tracked files.

Verifies:
1. No tracked files exist under `alt-frontend-sv/tests/e2e/.auth/`.
2. `playwright.config.ts`, `tests/e2e/global-setup.ts`, and
   `tests/readme-capture/helpers.ts` never reference tracked
   `tests/e2e/.auth/storage.json` and point `storageState` to the gitignored
   `test-results/.auth/storage.json`.
3. `alt-frontend-sv/.gitignore` explicitly ignores `test-results/`,
   `playwright-report/`, `blob-report/`, and `tests/e2e/.auth/`.
4. `git check-ignore -q alt-frontend-sv/test-results/.auth/storage.json` succeeds.
5. Simulating `global-setup` writing `alt-frontend-sv/test-results/.auth/storage.json`
   inside an isolated git repository carrying the repo `.gitignore` rules leaves
   `git status --porcelain` completely clean.

Run:
    python3 scripts/tests/test-e2e-clean-worktree.py
"""

from __future__ import annotations

import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
FRONTEND_DIR = ROOT / "alt-frontend-sv"
GIT_BIN = shutil.which("git")
EXPECTED_STORAGE_PATH = "test-results/.auth/storage.json"
FORBIDDEN_STORAGE_REF = "tests/e2e/.auth/storage.json"
REQUIRED_GITIGNORE_ENTRIES = (
    "test-results/",
    "playwright-report/",
    "blob-report/",
    "tests/e2e/.auth/",
)


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        suffix = f" ({detail})" if detail else ""
        sys.stdout.write(f"  FAIL  {name}{suffix}\n")
        RESULTS.failed += 1


def run_git(cwd: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    if GIT_BIN is None:
        msg = "git executable not found in PATH"
        raise RuntimeError(msg)
    env = os.environ.copy()
    env.update(
        {
            "GIT_OPTIONAL_LOCKS": "0",
            "GIT_AUTHOR_NAME": "Test Author",
            "GIT_AUTHOR_EMAIL": "test@example.com",
            "GIT_COMMITTER_NAME": "Test Committer",
            "GIT_COMMITTER_EMAIL": "test@example.com",
        }
    )
    return subprocess.run(  # noqa: S603 - fixed git executable and arguments
        [GIT_BIN, "-C", str(cwd), *args],
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )


def validate_config_texts(
    playwright_config: str,
    global_setup: str,
    readme_helpers: str,
    frontend_gitignore: str,
) -> list[str]:
    """Pure validator for E2E storageState hygiene; unit-tested with negative cases."""
    errors: list[str] = []

    for label, text in (
        ("playwright.config.ts", playwright_config),
        ("tests/e2e/global-setup.ts", global_setup),
        ("tests/readme-capture/helpers.ts", readme_helpers),
    ):
        if FORBIDDEN_STORAGE_REF in text:
            errors.append(f"{label} still references tracked {FORBIDDEN_STORAGE_REF!r}")

    storage_state_matches = re.findall(
        r'storageState\s*:\s*(?:path\.resolve\s*\(\s*import\.meta\.dirname\s*,\s*)?["\']([^"\']+)["\']',
        playwright_config,
    )
    if EXPECTED_STORAGE_PATH not in storage_state_matches:
        errors.append(
            f"playwright.config.ts must set storageState to {EXPECTED_STORAGE_PATH!r}, "
            f"found {storage_state_matches!r}"
        )
    errors.extend(
        f"playwright.config.ts storageState {match!r} points outside test-results/"
        for match in storage_state_matches
        if not match.startswith("test-results/")
    )

    if EXPECTED_STORAGE_PATH not in global_setup:
        errors.append(f"tests/e2e/global-setup.ts must target {EXPECTED_STORAGE_PATH!r}")
    if "import.meta.dirname" not in global_setup:
        errors.append(
            "tests/e2e/global-setup.ts must anchor STORAGE_STATE_PATH via import.meta.dirname"
        )
    if "import.meta.dirname" not in playwright_config:
        errors.append("playwright.config.ts must anchor storageState via import.meta.dirname")

    helpers_ok = (
        EXPECTED_STORAGE_PATH in readme_helpers
        or "STORAGE_STATE_PATH" in readme_helpers
        or "ensureStorageState" in readme_helpers
    )
    if not helpers_ok:
        errors.append(
            "tests/readme-capture/helpers.ts must use test-results/.auth/storage.json "
            "or shared STORAGE_STATE_PATH/ensureStorageState helper"
        )

    gitignore_lines = {
        line.strip()
        for line in frontend_gitignore.splitlines()
        if line.strip() and not line.strip().startswith("#")
    }
    errors.extend(
        f"alt-frontend-sv/.gitignore missing entry {entry!r}"
        for entry in REQUIRED_GITIGNORE_ENTRIES
        if entry not in gitignore_lines
    )

    return errors


def test_validator_rejects_bad_configs() -> None:
    good_pw = 'use: { storageState: path.resolve(import.meta.dirname, "test-results/.auth/storage.json") }'
    good_setup = (
        "export const STORAGE_STATE_PATH = path.resolve("
        'import.meta.dirname, "../../test-results/.auth/storage.json");'
    )
    good_helpers = 'import { ensureStorageState } from "../e2e/global-setup";'
    good_ignore = "\n".join(REQUIRED_GITIGNORE_ENTRIES) + "\n"

    check(
        "validator accepts clean CWD-proof config",
        validate_config_texts(good_pw, good_setup, good_helpers, good_ignore) == [],
    )
    unanchored_pw = 'use: { storageState: "test-results/.auth/storage.json" }'
    check(
        "validator rejects unanchored relative storageState in playwright.config.ts",
        bool(validate_config_texts(unanchored_pw, good_setup, good_helpers, good_ignore)),
    )
    bad_pw = 'use: { storageState: "tests/e2e/.auth/storage.json" }'
    check(
        "validator rejects legacy tests/e2e/.auth/storage.json in playwright.config.ts",
        bool(validate_config_texts(bad_pw, good_setup, good_helpers, good_ignore)),
    )
    bad_outside = 'use: { storageState: "tmp/storage.json" }'
    check(
        "validator rejects storageState outside test-results/",
        bool(validate_config_texts(bad_outside, good_setup, good_helpers, good_ignore)),
    )
    bad_ignore = "test-results/\n"
    check(
        "validator rejects incomplete alt-frontend-sv/.gitignore",
        bool(validate_config_texts(good_pw, good_setup, good_helpers, bad_ignore)),
    )


def test_isolated_worktree_stays_clean() -> None:
    """Copy real .gitignore files into a temp git repo and simulate global-setup write."""
    with tempfile.TemporaryDirectory(prefix="alt-e2e-clean-") as tmp_str:
        tmp = pathlib.Path(tmp_str)
        init_res = run_git(tmp, "init", "-q")
        if init_res.returncode != 0:
            check("temp git init succeeds", False, init_res.stderr)
            return

        shutil.copyfile(ROOT / ".gitignore", tmp / ".gitignore")
        (tmp / "alt-frontend-sv").mkdir()
        shutil.copyfile(FRONTEND_DIR / ".gitignore", tmp / "alt-frontend-sv" / ".gitignore")
        run_git(tmp, "add", ".gitignore", "alt-frontend-sv/.gitignore")
        commit_res = run_git(tmp, "commit", "-q", "-m", "baseline")
        if commit_res.returncode != 0:
            check("temp git baseline commit succeeds", False, commit_res.stderr)
            return

        # Simulate global-setup writing storageState + Playwright artifacts
        storage_file = tmp / "alt-frontend-sv" / EXPECTED_STORAGE_PATH
        storage_file.parent.mkdir(parents=True, exist_ok=True)
        storage_file.write_text('{"cookies": [], "origins": []}\n', encoding="utf-8")

        for extra_dir in ("playwright-report", "blob-report", "tests/e2e/.auth"):
            d = tmp / "alt-frontend-sv" / extra_dir
            d.mkdir(parents=True, exist_ok=True)
            (d / "dummy.json").write_text("{}\n", encoding="utf-8")

        status_res = run_git(tmp, "status", "--porcelain")
        check(
            "simulated global-setup + playwright outputs leave git status --porcelain empty",
            status_res.returncode == 0 and status_res.stdout.strip() == "",
            f"rc={status_res.returncode} status={status_res.stdout!r}",
        )


def main() -> int:
    test_validator_rejects_bad_configs()

    # 1. No tracked or working-tree files under alt-frontend-sv/tests/e2e/.auth/
    tracked_auth = run_git(ROOT, "ls-files", "alt-frontend-sv/tests/e2e/.auth")
    auth_dir = FRONTEND_DIR / "tests" / "e2e" / ".auth"
    tracked_and_present = [
        rel for rel in tracked_auth.stdout.splitlines() if rel.strip() and (ROOT / rel).exists()
    ]
    check(
        "no tracked file present under alt-frontend-sv/tests/e2e/.auth/",
        tracked_auth.returncode == 0 and len(tracked_and_present) == 0,
        f"present={tracked_and_present!r}",
    )
    check(
        "alt-frontend-sv/tests/e2e/.auth/storage.json does not exist in working tree",
        not (auth_dir / "storage.json").exists(),
    )

    # 2. Static config & gitignore validation against the real repository
    pw_text = (FRONTEND_DIR / "playwright.config.ts").read_text(encoding="utf-8")
    setup_text = (FRONTEND_DIR / "tests" / "e2e" / "global-setup.ts").read_text(encoding="utf-8")
    helpers_text = (FRONTEND_DIR / "tests" / "readme-capture" / "helpers.ts").read_text(
        encoding="utf-8"
    )
    gitignore_text = (FRONTEND_DIR / ".gitignore").read_text(encoding="utf-8")

    errors = validate_config_texts(pw_text, setup_text, helpers_text, gitignore_text)
    check(
        "real Playwright config, global-setup, readme-capture helpers, and .gitignore are clean",
        len(errors) == 0,
        f"errors={errors!r}",
    )

    # 3. git check-ignore on the real repository
    ignore_res = run_git(ROOT, "check-ignore", "-q", f"alt-frontend-sv/{EXPECTED_STORAGE_PATH}")
    check(
        f"git check-ignore confirms alt-frontend-sv/{EXPECTED_STORAGE_PATH} is ignored",
        ignore_res.returncode == 0,
        f"rc={ignore_res.returncode} stderr={ignore_res.stderr!r}",
    )

    # 4. End-to-end worktree cleanliness simulation
    test_isolated_worktree_stays_clean()

    sys.stdout.write(
        f"\ne2e-clean-worktree tests: {RESULTS.passed} passed, {RESULTS.failed} failed\n"
    )
    return 0 if RESULTS.failed == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
