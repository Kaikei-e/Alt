#!/usr/bin/env python3
"""Tests for scripts/lint/check_suppressions.py.

Verifies both configuration-file suppression allowlist enforcement
(pyproject.toml, .golangci.yml, Cargo.toml) and source-code suppression
governance (Python, Go, Rust, TypeScript/Svelte/Deno), plus an end-to-end
check against the real repository.

Run:
    python3 scripts/tests/test-check-suppressions.py
"""

from __future__ import annotations

import datetime
import hashlib
import importlib.util
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import types
from collections.abc import Sequence
from typing import Protocol
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "lint" / "check_suppressions.py"
ALLOWLIST = ROOT / "scripts" / "lint" / "suppression_allowlist.toml"
GIT_BIN = shutil.which("git")
assert GIT_BIN is not None, "git executable not found in PATH"

USAGE_EXIT_CODE = 2


class _HasRule(Protocol):
    @property
    def rule(self) -> str: ...


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


def check_violations(
    name: str,
    violations: Sequence[_HasRule],
    expected_count: int,
    expected_rules: set[str],
) -> None:
    actual_rules = {v.rule for v in violations}
    check(
        name,
        len(violations) == expected_count and actual_rules == expected_rules,
        f"count={len(violations)} rules={actual_rules} violations={violations!r}",
    )


def load_checker() -> types.ModuleType:
    if not SCRIPT.is_file():
        sys.stderr.write(f"FAIL: checker script missing: {SCRIPT}\n")
        sys.exit(1)
    spec = importlib.util.spec_from_file_location("check_suppressions", SCRIPT)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    with mock.patch.dict(sys.modules, {"check_suppressions": module}):
        spec.loader.exec_module(module)
    return module


def git_env() -> dict[str, str]:
    env = os.environ.copy()
    env.update(
        {
            "GIT_AUTHOR_NAME": "Test Author",
            "GIT_AUTHOR_EMAIL": "test@example.com",
            "GIT_COMMITTER_NAME": "Test Committer",
            "GIT_COMMITTER_EMAIL": "test@example.com",
        }
    )
    return env


def run_git(repo: pathlib.Path, *args: str) -> str:
    assert GIT_BIN is not None
    res = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [GIT_BIN, "-C", str(repo), *args],
        env=git_env(),
        capture_output=True,
        text=True,
        check=True,
    )
    return res.stdout.strip()


def rules_of(violations: Sequence[_HasRule]) -> set[str]:
    return {v.rule for v in violations}


def make_fixture_repo(tmp: pathlib.Path) -> pathlib.Path:
    repo = tmp / "repo"
    repo.mkdir()
    (repo / "docs" / "ADR").mkdir(parents=True)
    (repo / "docs" / "ADR" / "000979.md").write_text("# ADR-000979\n", encoding="utf-8")
    return repo


MINIMAL_ALLOWLIST_TOML = """\
version = 1

[ruff]
allowed_global_ignores = ["E501", "RUF001", "RUF002", "RUF003"]
allowed_test_per_file_ignores = ["S101"]

[[bandit]]
path = "tag-generator/app/pyproject.toml"
status = "frozen-baseline"
reason = "Pre-existing tag-generator bandit baseline"
expires = "2027-05-22"
exclude_dirs = ["tests", ".venv"]
skips = ["B101", "B104"]

[[golangci_exclusions]]
config = ".golangci.yml"
path = '_test\\.go$'
linters = ["gosec"]
text = "G101"
status = "frozen-baseline"
reason = "Dummy tokens in test files"
expires = "2027-05-22"

[[golangci_exclusions]]
config = ".golangci.yml"
path_except = '^knowledge-sovereign/app/usecase/(knowledge_home_projector|knowledge_trail_projector)/[^/]+\\.go$'
linters = ["forbidigo"]
status = "frozen-baseline"
reason = "Scope forbidigo to projectors"
expires = "2027-05-22"

[[cargo_lint_allows]]
path = "recap-worker/recap-worker/Cargo.toml"
section = "lints.clippy"
status = "frozen-baseline"
reason = "Pre-existing pedantic allows"
expires = "2027-05-22"
allows = ["doc_markdown", "module_name_repetitions"]
"""

FIXED_TODAY = datetime.date(2026, 11, 22)


def _check_allowlist_expires_cases(checker: types.ModuleType, repo: pathlib.Path) -> None:
    missing_exp = (
        MINIMAL_ALLOWLIST_TOML
        + '\n[[bandit]]\npath = "svc/pyproject.toml"\nadr = "ADR-000979"\n'
        + 'reason = "Approved exception missing expires"\n'
        + 'exclude_dirs = ["tests"]\nskips = ["B101"]\n'
    )
    missing_exp_path = repo / "missing_exp.toml"
    missing_exp_path.write_text(missing_exp, encoding="utf-8")
    _, errs = checker.load_allowlist(missing_exp_path, repo, today=FIXED_TODAY)
    check(
        "entry missing expires fails with config.allowlist.missing-expires",
        rules_of(errs) == {"config.allowlist.missing-expires"},
        repr(errs),
    )

    expired_entry = (
        MINIMAL_ALLOWLIST_TOML
        + '\n[[bandit]]\npath = "svc/pyproject.toml"\nadr = "ADR-000979"\n'
        + 'reason = "Approved exception already expired"\nexpires = "2026-11-21"\n'
        + 'exclude_dirs = ["tests"]\nskips = ["B101"]\n'
    )
    expired_path = repo / "expired.toml"
    expired_path.write_text(expired_entry, encoding="utf-8")
    _, errs = checker.load_allowlist(expired_path, repo, today=FIXED_TODAY)
    check(
        "entry with expires < today fails with config.allowlist.expired",
        rules_of(errs) == {"config.allowlist.expired"},
        repr(errs),
    )

    boundary_entry = (
        MINIMAL_ALLOWLIST_TOML
        + '\n[[bandit]]\npath = "svc/pyproject.toml"\nadr = "ADR-000979"\n'
        + 'reason = "Approved exception expiring today"\nexpires = "2026-11-22"\n'
        + 'exclude_dirs = ["tests"]\nskips = ["B101"]\n'
    )
    boundary_path = repo / "boundary.toml"
    boundary_path.write_text(boundary_entry, encoding="utf-8")
    _, errs = checker.load_allowlist(boundary_path, repo, today=FIXED_TODAY)
    check("entry with expires == today still passes", errs == [], repr(errs))

    extended_baseline = MINIMAL_ALLOWLIST_TOML.replace(
        'expires = "2027-05-22"',
        'expires = "2099-12-31"',
        1,
    )
    extended_path = repo / "extended_baseline.toml"
    extended_path.write_text(extended_baseline, encoding="utf-8")
    _, errs = checker.load_allowlist(extended_path, repo, today=FIXED_TODAY)
    check(
        "frozen-baseline entry extending expires past 2027-05-22 without ADR fails",
        "config.allowlist.expires-extended-without-adr" in rules_of(errs),
        repr(errs),
    )

    extended_with_adr = MINIMAL_ALLOWLIST_TOML.replace(
        'status = "frozen-baseline"\nreason = "Pre-existing tag-generator bandit baseline"\nexpires = "2027-05-22"',
        'status = "frozen-baseline"\nadr = "ADR-000979"\nreason = "Extended via ADR"\nexpires = "2028-05-22"',
        1,
    )
    extended_adr_path = repo / "extended_with_adr.toml"
    extended_adr_path.write_text(extended_with_adr, encoding="utf-8")
    _, errs = checker.load_allowlist(extended_adr_path, repo, today=FIXED_TODAY)
    check(
        "frozen-baseline entry extending expires past 2027-05-22 WITH valid ADR passes",
        errs == [],
        repr(errs),
    )


def test_allowlist_validation(checker: types.ModuleType) -> None:
    sys.stdout.write("allowlist validation\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        repo = make_fixture_repo(pathlib.Path(tmp_str))

        good_path = repo / "good.toml"
        good_path.write_text(MINIMAL_ALLOWLIST_TOML, encoding="utf-8")
        _, errs = checker.load_allowlist(good_path, repo, today=FIXED_TODAY)
        check("valid allowlist loads cleanly", errs == [], repr(errs))

        with_adr = (
            MINIMAL_ALLOWLIST_TOML
            + '\n[[bandit]]\npath = "svc/pyproject.toml"\nadr = "ADR-000979"\n'
            + 'reason = "Approved exception for svc"\nexpires = "2027-05-22"\n'
            + 'exclude_dirs = ["tests"]\nskips = ["B101"]\n'
        )
        adr_path = repo / "adr.toml"
        adr_path.write_text(with_adr, encoding="utf-8")
        _, errs = checker.load_allowlist(adr_path, repo, today=FIXED_TODAY)
        check("entry with existing ADR, reason, and future expires passes", errs == [], repr(errs))

        _check_allowlist_expires_cases(checker, repo)

        bad_adr = (
            MINIMAL_ALLOWLIST_TOML
            + '\n[[bandit]]\npath = "svc/pyproject.toml"\nadr = "ADR-999999"\n'
            + 'reason = "Missing ADR document"\nexpires = "2027-05-22"\n'
            + 'exclude_dirs = ["tests"]\nskips = ["B101"]\n'
        )
        bad_adr_path = repo / "bad_adr.toml"
        bad_adr_path.write_text(bad_adr, encoding="utf-8")
        _, errs = checker.load_allowlist(bad_adr_path, repo, today=FIXED_TODAY)
        check(
            "entry referencing nonexistent ADR fails",
            "config.allowlist.invalid" in rules_of(errs),
            repr(errs),
        )

        no_auth = (
            MINIMAL_ALLOWLIST_TOML
            + '\n[[bandit]]\npath = "svc/pyproject.toml"\n'
            + 'reason = "Neither baseline nor ADR"\nexpires = "2027-05-22"\n'
            + 'exclude_dirs = ["tests"]\nskips = ["B101"]\n'
        )
        no_auth_path = repo / "no_auth.toml"
        no_auth_path.write_text(no_auth, encoding="utf-8")
        _, errs = checker.load_allowlist(no_auth_path, repo, today=FIXED_TODAY)
        check(
            "entry without frozen-baseline or ADR fails",
            "config.allowlist.invalid" in rules_of(errs),
            repr(errs),
        )

        short_reason = (
            MINIMAL_ALLOWLIST_TOML
            + '\n[[bandit]]\npath = "svc/pyproject.toml"\nadr = "ADR-000979"\n'
            + 'reason = "ok"\nexpires = "2027-05-22"\n'
            + 'exclude_dirs = ["tests"]\nskips = ["B101"]\n'
        )
        short_path = repo / "short.toml"
        short_path.write_text(short_reason, encoding="utf-8")
        _, errs = checker.load_allowlist(short_path, repo, today=FIXED_TODAY)
        check(
            "entry with too-short reason fails",
            "config.allowlist.invalid" in rules_of(errs),
            repr(errs),
        )

        forged_baseline = (
            MINIMAL_ALLOWLIST_TOML
            + '\n[[bandit]]\npath = "new-svc/pyproject.toml"\nstatus = "frozen-baseline"\n'
            + 'reason = "Attempted unapproved baseline"\nexpires = "2027-05-22"\n'
            + 'exclude_dirs = ["tests"]\nskips = ["B101"]\n'
        )
        forged_path = repo / "forged.toml"
        forged_path.write_text(forged_baseline, encoding="utf-8")
        _, errs = checker.load_allowlist(forged_path, repo, today=FIXED_TODAY)
        check(
            "forged frozen-baseline on new service without ADR fails",
            "config.allowlist.invalid" in rules_of(errs),
            repr(errs),
        )

        ruff_expanded = MINIMAL_ALLOWLIST_TOML.replace(
            'allowed_global_ignores = ["E501", "RUF001", "RUF002", "RUF003"]',
            'allowed_global_ignores = ["E501", "RUF001", "RUF002", "RUF003", "F401"]',
        )
        ruff_exp_path = repo / "ruff_exp.toml"
        ruff_exp_path.write_text(ruff_expanded, encoding="utf-8")
        _, errs = checker.load_allowlist(ruff_exp_path, repo, today=FIXED_TODAY)
        check(
            "expanding [ruff] allowed_global_ignores beyond policy ceiling without ADR fails",
            "config.allowlist.invalid" in rules_of(errs),
            repr(errs),
        )


def test_pyproject_checks(checker: types.ModuleType) -> None:
    sys.stdout.write("pyproject.toml enforcement\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        repo = make_fixture_repo(pathlib.Path(tmp_str))
        al_path = repo / "allowlist.toml"
        al_path.write_text(MINIMAL_ALLOWLIST_TOML, encoding="utf-8")
        allowlist, _ = checker.load_allowlist(al_path, repo)

        valid_pyproject = """\
[tool.ruff.lint]
ignore = ["E501", "RUF001", "RUF002", "RUF003"]

[tool.ruff.lint.per-file-ignores]
"tests/**" = ["S101"]
"test_*.py" = ["S101"]
"**/test_*.py" = ["S101"]
"evaluation/tests/**" = ["S101"]
"""
        errs = checker.check_pyproject("svc/pyproject.toml", valid_pyproject, allowlist)
        check("valid ruff ignore and test per-file-ignores pass", errs == [], repr(errs))

        legacy_keys = """\
[tool.ruff]
ignore = ["E501"]
extend-ignore = ["F401"]
per-file-ignores = { "tests/**" = ["S101"] }
extend-per-file-ignores = { "tests/**" = ["S101"] }
"""
        errs = checker.check_pyproject("svc/pyproject.toml", legacy_keys, allowlist)
        check_violations(
            "legacy [tool.ruff] ignore/per-file-ignores keys are rejected",
            errs,
            expected_count=4,
            expected_rules={"config.pyproject.legacy-ruff-key"},
        )

        disallowed_ignore = """\
[tool.ruff.lint]
ignore = ["E501", "S101", "ALL", "F4*"]
extend-ignore = ["B008"]
"""
        errs = checker.check_pyproject("svc/pyproject.toml", disallowed_ignore, allowlist)
        check(
            "disallowed ignore rules, ALL, wildcards, and extend-ignore are rejected",
            {
                "config.pyproject.ruff-ignore-disallowed",
                "config.pyproject.ruff-extend-ignore",
            }
            <= rules_of(errs),
            repr(errs),
        )

        bad_pfi_path = """\
[tool.ruff.lint.per-file-ignores]
"src/**" = ["S101"]
"*.py" = ["S101"]
"{src,tests}/**" = ["S101"]
"""
        errs = checker.check_pyproject("svc/pyproject.toml", bad_pfi_path, allowlist)
        check_violations(
            "non-test per-file-ignores path patterns are rejected",
            errs,
            expected_count=3,
            expected_rules={"config.pyproject.ruff-per-file-ignores-path"},
        )

        bad_pfi_rule = """\
[tool.ruff.lint.per-file-ignores]
"tests/**" = ["S101", "F401"]

[tool.ruff.lint.extend-per-file-ignores]
"tests/**" = ["ALL"]
"""
        errs = checker.check_pyproject("svc/pyproject.toml", bad_pfi_rule, allowlist)
        check(
            "non-S101 rules in per-file-ignores / extend-per-file-ignores are rejected",
            rules_of(errs) == {"config.pyproject.ruff-per-file-ignores-rule"},
            repr(errs),
        )

        bandit_subset = """\
[tool.bandit]
exclude_dirs = ["tests"]
skips = ["B101"]
"""
        errs = checker.check_pyproject("tag-generator/app/pyproject.toml", bandit_subset, allowlist)
        check("allowlisted bandit subset passes", errs == [], repr(errs))

        bandit_extra = """\
[tool.bandit]
exclude_dirs = ["tests", ".venv", "src"]
skips = ["B101", "B104", "B301"]
"""
        errs = checker.check_pyproject("tag-generator/app/pyproject.toml", bandit_extra, allowlist)
        check(
            "extra bandit skip and exclude_dirs are rejected",
            {
                "config.pyproject.bandit-skip-disallowed",
                "config.pyproject.bandit-exclude-dir-disallowed",
            }
            == rules_of(errs),
            repr(errs),
        )

        bandit_new_svc = """\
[tool.bandit]
exclude_dirs = ["tests"]
skips = ["B101"]
"""
        errs = checker.check_pyproject("new-service/pyproject.toml", bandit_new_svc, allowlist)
        check(
            "unlisted service adding [tool.bandit] is rejected",
            rules_of(errs) == {"config.pyproject.bandit-unallowlisted"},
            repr(errs),
        )


def test_golangci_and_cargo_checks(checker: types.ModuleType) -> None:
    sys.stdout.write(".golangci.yml and Cargo.toml enforcement\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        repo = make_fixture_repo(pathlib.Path(tmp_str))
        al_path = repo / "allowlist.toml"
        al_path.write_text(MINIMAL_ALLOWLIST_TOML, encoding="utf-8")
        allowlist, _ = checker.load_allowlist(al_path, repo)

        valid_go_cfg = """\
version: "2"
linters:
  enable:
    - gosec
    - forbidigo
  exclusions:
    generated: lax
    rules:
      - path: '_test\\.go$'
        linters:
          - gosec
        text: 'G101'
      - path-except: '^knowledge-sovereign/app/usecase/(knowledge_home_projector|knowledge_trail_projector)/[^/]+\\.go$'
        linters:
          - forbidigo
"""
        errs = checker.check_golangci(".golangci.yml", valid_go_cfg, allowlist)
        check("frozen .golangci.yml exclusions pass", errs == [], repr(errs))

        disabled_go_cfg = """\
version: "2"
linters:
  disable:
    - gosec
"""
        errs = checker.check_golangci(".golangci.yml", disabled_go_cfg, allowlist)
        check(
            "linters.disable is rejected",
            rules_of(errs) == {"config.golangci.disable-forbidden"},
            repr(errs),
        )

        widened_go_cfg = """\
version: "2"
linters:
  exclusions:
    rules:
      - path: '_test\\.go$'
        linters:
          - gosec
      - path: 'internal/foo\\.go$'
        linters:
          - errcheck
"""
        errs = checker.check_golangci(".golangci.yml", widened_go_cfg, allowlist)
        check_violations(
            "widened or newly added golangci exclusion rules are rejected",
            errs,
            expected_count=2,
            expected_rules={"config.golangci.exclusion-unallowlisted"},
        )

        valid_cargo = """\
[lints.rust]
unsafe_code = "deny"

[lints.clippy]
all = { level = "warn", priority = -1 }
doc_markdown = "allow"
"""
        errs = checker.check_cargo_toml(
            "recap-worker/recap-worker/Cargo.toml", valid_cargo, allowlist
        )
        check("allowlisted Cargo.toml lint allows pass", errs == [], repr(errs))

        bad_cargo = """\
[lints.rust]
dead_code = "allow"

[lints.clippy]
doc_markdown = "allow"
unwrap_used = { level = "allow", priority = 1 }

[workspace.lints.clippy]
pedantic = "allow"
"""
        errs = checker.check_cargo_toml(
            "recap-worker/recap-worker/Cargo.toml", bad_cargo, allowlist
        )
        check_violations(
            "unallowlisted Cargo.toml allows (string, table, workspace) are rejected",
            errs,
            expected_count=3,
            expected_rules={"config.cargo.lint-allow-unallowlisted"},
        )

        unlisted_cargo = """\
[lints.clippy]
doc_markdown = "allow"
"""
        errs = checker.check_cargo_toml("plecto/Cargo.toml", unlisted_cargo, allowlist)
        check(
            "Cargo.toml allow in unlisted crate is rejected",
            rules_of(errs) == {"config.cargo.lint-allow-unallowlisted"},
            repr(errs),
        )


def test_python_source_checks(checker: types.ModuleType) -> None:
    sys.stdout.write("Python source suppression governance\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        repo = make_fixture_repo(pathlib.Path(tmp_str))

        errs = checker.check_python_source(
            "app/a.py",
            "x = 1  # noqa: S603 -- fixed argv command\n",
            repo,
            today=FIXED_TODAY,
        )
        check("inline # noqa: CODE -- <reason> passes", errs == [], repr(errs))

        errs = checker.check_python_source(
            "app/a.py",
            "x = 1  # noqa: S603  # fixed argv command\n",
            repo,
            today=FIXED_TODAY,
        )
        check("inline # noqa: CODE  # <reason> passes", errs == [], repr(errs))

        errs = checker.check_python_source(
            "app/a.py", "x = 1  # noqa: F401\n", repo, today=FIXED_TODAY
        )
        check(
            "reasonless inline # noqa: CODE without baseline is rejected",
            rules_of(errs) == {"source.python.noqa-missing-reason"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "x = 1  # noqa: S314  # nosec B314\n",
            repo,
            today=FIXED_TODAY,
        )
        check(
            "inline # noqa: CODE followed only by # nosec without reason is rejected",
            rules_of(errs)
            == {"source.python.noqa-missing-reason", "source.python.nosec-missing-reason"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "x = 1  # nosec\ny = 2  # nosec -- missing code\n",
            repo,
            today=FIXED_TODAY,
        )
        check_violations(
            "bare Python # nosec without B<NNN> code is rejected",
            errs,
            expected_count=2,
            expected_rules={"source.python.nosec-missing-code"},
        )

        errs = checker.check_python_source(
            "app/a.py",
            "x = 1  # nosec B310\n",
            repo,
            today=FIXED_TODAY,
        )
        check(
            "new reasonless Python # nosec B310 without baseline is rejected",
            rules_of(errs) == {"source.python.nosec-missing-reason"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "a = '0.0.0.0'  # noqa: S104 - container bind  # nosec B104\n"
            "b = '0.0.0.0'  # nosec B104  # noqa: S104 - container bind\n"
            "c = 1  # nosec B310 -- local health endpoint\n",
            repo,
            today=FIXED_TODAY,
        )
        check(
            "Python # nosec B<NNN> with own or shared same-line reason passes",
            errs == [],
            repr(errs),
        )

        baseline_line = "x = 1  # noqa: F401"
        baseline_sha = hashlib.sha256(" ".join(baseline_line.split()).encode("utf-8")).hexdigest()
        baseline_map = {("app/a.py", ("F401",), baseline_sha): 1}
        errs = checker.check_python_source(
            "app/a.py",
            "\n\nx = 1  # noqa: F401\n",
            repo,
            today=FIXED_TODAY,
            python_noqa_baseline=baseline_map,
        )
        check(
            "baseline-exempted reasonless # noqa passes even after line shift",
            errs == [],
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "x = 1  # noqa: F401\nx = 1  # noqa: F401\n",
            repo,
            today=FIXED_TODAY,
            python_noqa_baseline=baseline_map,
        )
        check_violations(
            "second reasonless # noqa exceeding baseline multiplicity is rejected",
            errs,
            expected_count=1,
            expected_rules={"source.python.noqa-missing-reason"},
        )

        errs = checker.check_python_source("app/a.py", "x = 1  # noqa\n", repo, today=FIXED_TODAY)
        check(
            "bare # noqa without code is rejected",
            rules_of(errs) == {"source.python.noqa-missing-code"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py", "x = 1  # noqa: ALL\n", repo, today=FIXED_TODAY
        )
        check(
            "# noqa: ALL is rejected",
            rules_of(errs) == {"source.python.noqa-missing-code"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py", "# ruff: noqa\nx = 1\n", repo, today=FIXED_TODAY
        )
        check(
            "file-level # ruff: noqa without ADR is rejected",
            "source.broad.missing-adr" in rules_of(errs),
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py", "# flake8: noqa\nx = 1\n", repo, today=FIXED_TODAY
        )
        check(
            "file-level # flake8: noqa without ADR is rejected",
            "source.broad.missing-adr" in rules_of(errs),
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "# ADR-999999: legacy shim requires file-level ignore (expires: 2027-05-22)\n# ruff: noqa: F401\n",
            repo,
            today=FIXED_TODAY,
        )
        check(
            "file-level # ruff: noqa with nonexistent ADR is rejected",
            rules_of(errs) == {"source.broad.nonexistent-adr"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "# ADR-000979 expires: 2027-05-22\n# ruff: noqa\n",
            repo,
            today=FIXED_TODAY,
        )
        check(
            "file-level # ruff: noqa with ADR and expires but no reason is rejected",
            rules_of(errs) == {"source.broad.missing-reason"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "# ADR-000979: vendored bridge requires file-level ignore\n# ruff: noqa: F401\n",
            repo,
            today=FIXED_TODAY,
        )
        check(
            "file-level # ruff: noqa missing expires is rejected",
            rules_of(errs) == {"source.broad.missing-expires"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "# ADR-000979: vendored bridge requires file-level ignore (expires: 2026-11-21)\n# ruff: noqa: F401\n",
            repo,
            today=FIXED_TODAY,
        )
        check(
            "file-level # ruff: noqa with expired date is rejected",
            rules_of(errs) == {"source.broad.expired"},
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/a.py",
            "# ADR-000979: vendored bridge requires file-level ignore (expires: 2027-05-22)\n# ruff: noqa: F401\n",
            repo,
            today=FIXED_TODAY,
        )
        check(
            "file-level # ruff: noqa with valid ADR + reason + future expires passes",
            errs == [],
            repr(errs),
        )

        fixture_code = 'SAMPLE = "# ruff: noqa\\n# noqa\\n"\n'
        errs = checker.check_python_source("app/test_a.py", fixture_code, repo, today=FIXED_TODAY)
        check("directives inside Python string literals are ignored", errs == [], repr(errs))


def test_sql_suppression_classification(checker: types.ModuleType) -> None:
    sys.stdout.write("SQL suppression classification (S608/G201/G202/B608)\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        repo = make_fixture_repo(pathlib.Path(tmp_str))

        py_valid = (
            'q1 = f"DELETE FROM {tbl}"  # noqa: S608 — identifier (trusted constant table)  # nosec B608\n'
            'q2 = f"SELECT {col} FROM t"  # noqa: S608 -- 識別子 (信頼境界内 allowlist)\n'
            'msg = f"select {n} items"  # noqa: S608  # not-sql (internal error message)\n'
            'msg2 = f"select {n} items"  # noqa: S608 -- 非SQL (internal template)\n'
        )
        errs = checker.check_python_source("app/db.py", py_valid, repo, today=FIXED_TODAY)
        check("valid Python S608/B608 classifications pass", errs == [], repr(errs))

        errs = checker.check_python_source(
            "app/db.py",
            'q = f"WHERE id = {val}"  # noqa: S608 -- value from trusted config\n',
            repo,
            today=FIXED_TODAY,
        )
        check(
            "Python S608 value interpolation is rejected",
            "source.sql.value-interpolation-forbidden" in rules_of(errs),
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/db.py",
            'q = f"WHERE id = {val}"  # noqa: S608 -- 値 (信頼境界内)\n',
            repo,
            today=FIXED_TODAY,
        )
        check(
            "Python S608 Japanese 値 interpolation is rejected",
            "source.sql.value-interpolation-forbidden" in rules_of(errs),
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/db.py",
            'q = f"FROM {tbl}"  # noqa: S608 -- identifier from untrusted request\n',
            repo,
            today=FIXED_TODAY,
        )
        check(
            "Python S608 untrusted boundary is rejected",
            "source.sql.untrusted-boundary-forbidden" in rules_of(errs),
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/db.py",
            'q = f"FROM {tbl}"  # noqa: S608 -- 識別子 (信頼境界外)\n',
            repo,
            today=FIXED_TODAY,
        )
        check(
            "Python S608 Japanese 信頼境界外 is rejected",
            "source.sql.untrusted-boundary-forbidden" in rules_of(errs),
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/db.py",
            'q = f"FROM {tbl}"  # noqa: S608 -- validated db name above\n',
            repo,
            today=FIXED_TODAY,
        )
        check(
            "Python S608 missing kind and trust boundary is rejected",
            {"source.sql.missing-kind", "source.sql.missing-trust-boundary"} == rules_of(errs),
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/db.py",
            'q = f"FROM {tbl}"  # nosec B608 -- external input table\n',
            repo,
            today=FIXED_TODAY,
        )
        check(
            "standalone Python # nosec B608 enforces SQL classification",
            "source.sql.untrusted-boundary-forbidden" in rules_of(errs),
            repr(errs),
        )

        errs = checker.check_python_source(
            "app/db.py",
            'q = f"SELECT {x}"  # noqa: E501 -- long line  # nosec B608\n',
            repo,
            today=FIXED_TODAY,
        )
        check(
            "same-line # noqa: E501 + # nosec B608 enforces SQL classification on B608",
            {"source.sql.missing-kind", "source.sql.missing-trust-boundary"} <= rules_of(errs),
            repr(errs),
        )

        go_valid = (
            "package db\nfunc f() {\n"
            '    _ = "SELECT " + col // #nosec G202 -- identifier from trusted allowlist\n'
            '    _ = fmt.Sprintf("SELECT %s", tbl) // #nosec G201 -- 識別子 (constant)\n'
            "}\n"
        )
        errs = checker.check_go_source("svc/db.go", go_valid, repo, today=FIXED_TODAY)
        check("valid Go G201/G202 classifications pass", errs == [], repr(errs))

        go_bad = (
            "package db\nfunc f() {\n"
            '    _ = "SELECT " + v // #nosec G202 -- value from trusted config\n'
            '    _ = fmt.Sprintf("FROM %s", t) // #nosec G201 -- identifier from external param\n'
            '    _ = "SELECT " + col // #nosec G202 -- safe string concatenation\n'
            "}\n"
        )
        errs = checker.check_go_source("svc/db.go", go_bad, repo, today=FIXED_TODAY)
        check(
            "Go G201/G202 enforce value, untrusted, kind, and trust boundary rules",
            {
                "source.sql.value-interpolation-forbidden",
                "source.sql.untrusted-boundary-forbidden",
                "source.sql.missing-kind",
                "source.sql.missing-trust-boundary",
            }
            == rules_of(errs),
            repr(errs),
        )


def test_go_source_checks(checker: types.ModuleType) -> None:
    sys.stdout.write("Go source suppression governance\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        repo = make_fixture_repo(pathlib.Path(tmp_str))

        good_go = """\
package foo

func f() {
    _ = bar() //nolint:noctx // test only local request
    _ = baz() /*nolint:errcheck*/ // cleanup is best-effort
    _ = qux() // #nosec G304 -- path from temp directory
    _ = q2() //#nosec G304 G703 // operator-configured path
    //nolinter prose comment must not false-positive
    s := "//nolint\\n// #nosec\\n"
    r := `//nolint:noctx`
    _, _ = s, r
}
"""
        errs = checker.check_go_source("svc/a.go", good_go, repo)
        check("valid Go nolint/nosec and string literals pass", errs == [], repr(errs))

        bad_pre_pkg = "//nolint:errcheck // reason text here\npackage foo\n"
        errs = checker.check_go_source("svc/a.go", bad_pre_pkg, repo)
        check(
            "pre-package //nolint without ADR is rejected",
            rules_of(errs) == {"source.broad.missing-adr"},
            repr(errs),
        )

        good_pre_pkg = (
            "// ADR-000979: generated adapter file requires package-level nolint (expires: 2027-05-22)\n"
            "//nolint:errcheck\npackage foo\n"
        )
        errs = checker.check_go_source("svc/a.go", good_pre_pkg, repo, today=FIXED_TODAY)
        check(
            "pre-package //nolint with existing ADR + reason + expires passes",
            errs == [],
            repr(errs),
        )

        bad_blanket = """\
package foo

func f() {
    _ = a() //nolint
    _ = b() //nolint:all // suppress everything
    _ = c() //nolint:errcheck,all // suppress everything
}
"""
        errs = checker.check_go_source("svc/a.go", bad_blanket, repo)
        check_violations(
            "bare //nolint and //nolint:all without ADR are rejected",
            errs,
            expected_count=3,
            expected_rules={"source.broad.missing-adr"},
        )

        bad_nolint_gosec = """\
package foo

func f() {
    _ = a() //nolint:gosec // reason text here
    _ = b() //nolint:errcheck,gosec // reason text here
    _ = c() /*nolint:gosec // reason text here*/
    _ = d() //nolint:GOSEC // uppercase gosec linter name
    _ = e() //nolint: gosec // spaced after colon
    _ = f2() // nolint: gosec // spaced after slash and colon
    _ = g() //nolint:errcheck, gosec // spaced after comma
}
"""
        errs = checker.check_go_source("svc/a.go", bad_nolint_gosec, repo)
        check_violations(
            "inline/block //nolint:gosec (case-insensitive, spaced, single or multi) is forbidden",
            errs,
            expected_count=7,
            expected_rules={"source.go.nolint-gosec-forbidden"},
        )
        check(
            "nolint-gosec-forbidden message guides author to // #nosec G<NNN> -- <reason>",
            all("#nosec" in v.message for v in errs),
            repr(errs),
        )

        bad_file_nolint_gosec = "//nolint:gosec // file-level suppression\npackage foo\n"
        errs = checker.check_go_source("svc/a.go", bad_file_nolint_gosec, repo)
        check_violations(
            "file-level //nolint:gosec before package is forbidden",
            errs,
            expected_count=1,
            expected_rules={"source.go.nolint-gosec-forbidden"},
        )

        bad_file_spaced_nolint_gosec = (
            "//nolint: gosec // spaced file-level suppression\npackage foo\n"
        )
        errs = checker.check_go_source("svc/a.go", bad_file_spaced_nolint_gosec, repo)
        check_violations(
            "file-level spaced //nolint: gosec before package is forbidden",
            errs,
            expected_count=1,
            expected_rules={"source.go.nolint-gosec-forbidden"},
        )

        bad_file_nolint_gosec_with_adr = (
            "// ADR-000979: attempted file-level gosec nolint with ADR\n"
            "//nolint:gosec\npackage foo\n"
        )
        errs = checker.check_go_source("svc/a.go", bad_file_nolint_gosec_with_adr, repo)
        check_violations(
            "file-level //nolint:gosec before package is forbidden even with ADR",
            errs,
            expected_count=1,
            expected_rules={"source.go.nolint-gosec-forbidden"},
        )

        bad_inline_nolint = """\
package foo

func f() {
    _ = a() //nolint:noctx
    _ = b() //nolint:noctx // ok
    _ = c() /*nolint:noctx*/
}
"""
        errs = checker.check_go_source("svc/a.go", bad_inline_nolint, repo)
        check_violations(
            "Go nolint missing or too-short inline reason is rejected",
            errs,
            expected_count=3,
            expected_rules={"source.go.nolint-missing-reason"},
        )

        bad_nosec = """\
package foo

func f() {
    _ = a() // #nosec G304
    _ = b() // #nosec G304 -- ok
    _ = c() // #nosec -- missing rule id
    _ = d() // #nosec
}
"""
        errs = checker.check_go_source("svc/a.go", bad_nosec, repo)
        check_violations(
            "Go #nosec missing rule or reason is rejected",
            errs,
            expected_count=4,
            expected_rules={"source.go.nosec-missing-reason", "source.go.nosec-missing-rule"},
        )


def test_rust_and_ts_source_checks(checker: types.ModuleType) -> None:
    sys.stdout.write("Rust and TS/Svelte/Deno source suppression governance\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        repo = make_fixture_repo(pathlib.Path(tmp_str))

        good_rs = """\
#[allow(dead_code)]
fn helper() {}

fn sample() {
    let s = "#![allow(clippy::all)]";
    let r = r#"#[allow(dead_code)] mod fake;"#;
    let _ = (s, r);
}
"""
        errs = checker.check_rust_source("svc/src/lib.rs", good_rs, repo)
        check("Rust item-level #[allow] and string literals pass", errs == [], repr(errs))

        bad_crate_allow = "#![allow(clippy::too_many_arguments)]\nfn f() {}\n"
        errs = checker.check_rust_source("svc/src/lib.rs", bad_crate_allow, repo)
        check(
            "Rust #![allow(...)] without ADR is rejected",
            rules_of(errs) == {"source.broad.missing-adr"},
            repr(errs),
        )

        good_crate_allow = (
            "// ADR-000979: wit-bindgen generated code requires crate allow (expires: 2027-05-22)\n"
            "#![allow(clippy::too_many_arguments)]\n"
        )
        errs = checker.check_rust_source(
            "svc/src/lib.rs", good_crate_allow, repo, today=FIXED_TODAY
        )
        check(
            "Rust #![allow(...)] with valid ADR + reason + expires passes", errs == [], repr(errs)
        )

        expired_crate_allow = (
            "// ADR-000979: wit-bindgen generated code requires crate allow (expires=2026-11-21)\n"
            "#![allow(clippy::too_many_arguments)]\n"
        )
        errs = checker.check_rust_source(
            "svc/src/lib.rs", expired_crate_allow, repo, today=FIXED_TODAY
        )
        check(
            "Rust #![allow(...)] with expired expires=YYYY-MM-DD fails",
            rules_of(errs) == {"source.broad.expired"},
            repr(errs),
        )

        bad_mod_allow = """\
#[allow(dead_code)]
mod snapshot_id;

#[cfg(test)]
#[allow(clippy::float_cmp)]
mod tests {}

#[allow(
    dead_code,
)]
pub(crate) mod extra;
"""
        errs = checker.check_rust_source("svc/src/lib.rs", bad_mod_allow, repo)
        check_violations(
            "Rust #[allow(...)] on mod declarations (including cfg/multiline) is rejected",
            errs,
            expected_count=3,
            expected_rules={"source.broad.missing-adr"},
        )

        spaced_and_expect_rs = """\
#! [allow(dead_code)]
#[expect(dead_code)]
mod m1 {}
# [allow(dead_code)]
mod m2 {}
#![expect(clippy::too_many_arguments)]
#! [expect(unused)]
"""
        errs = checker.check_rust_source("svc/src/spaced.rs", spaced_and_expect_rs, repo)
        check_violations(
            "spaced Rust # [allow]/#! [allow] and #[expect]/#![expect] without ADR are rejected",
            errs,
            expected_count=5,
            expected_rules={"source.broad.missing-adr"},
        )

        good_ts = """\
// biome-ignore lint/suspicious/noExplicitAny: playwright type mismatch
const a: any = 1;
// biome-ignore lint: test double shape not needed
const b = 2;
// deno-lint-ignore no-explicit-any
const c: any = 3;
// eslint-disable-next-line no-console
console.log(a, b, c);
const str = "/* eslint-disable */ // biome-ignore-all // deno-lint-ignore-file";
"""
        errs = checker.check_ts_source("web/src/a.ts", good_ts, repo)
        check("valid Biome/Deno/eslint-next-line and strings pass", errs == [], repr(errs))

        bad_ts_broad = """\
// biome-ignore-all lint: file-wide disable
// deno-lint-ignore-file
/* eslint-disable */
"""
        errs = checker.check_ts_source("web/src/a.ts", bad_ts_broad, repo)
        check_violations(
            "TS/Deno/Biome file-wide disables without ADR are rejected",
            errs,
            expected_count=3,
            expected_rules={"source.broad.missing-adr"},
        )

        good_ts_broad = """\
// ADR-000979: vendored third-party adapter requires file-level ignore (expires: 2027-05-22)
// deno-lint-ignore-file
"""
        errs = checker.check_ts_source("web/src/a.ts", good_ts_broad, repo, today=FIXED_TODAY)
        check(
            "deno-lint-ignore-file with valid ADR + reason + expires passes",
            errs == [],
            repr(errs),
        )

        bad_ts_line = """\
// deno-lint-ignore
// deno-lint-ignore -- reason without rule name
// biome-ignore lint/suspicious/noExplicitAny
// biome-ignore lint/suspicious/noExplicitAny: ok
// biome-ignore
"""
        errs = checker.check_ts_source("web/src/a.ts", bad_ts_line, repo)
        check_violations(
            "rule-less deno-lint-ignore and reason-less biome-ignore are rejected",
            errs,
            expected_count=5,
            expected_rules={
                "source.deno.ignore-missing-rule",
                "source.biome.ignore-missing-reason",
                "source.biome.ignore-missing-rule",
            },
        )

        svelte_bad = "<!-- biome-ignore lint/a11y/noSvgWithoutTitle -->\n<svg />\n"
        errs = checker.check_ts_source("web/src/Comp.svelte", svelte_bad, repo)
        check(
            "Svelte HTML comment biome-ignore without reason is rejected",
            rules_of(errs) == {"source.biome.ignore-missing-reason"},
            repr(errs),
        )


def test_exclusions_and_cli(checker: types.ModuleType) -> None:
    sys.stdout.write("path exclusions and CLI integration\n")
    for excluded in (
        "stopped-using-k8s/a.go",
        "scripts/harness/corpus/C-22/after.py",
        "pkg/foo.pb.go",
        "pkg/foo.connect.go",
        "api/server.gen.go",
        "svc/gen/client.py",
        "alt-frontend-sv/src/lib/gen/foo.ts",
        "docs/README.md",
    ):
        check(f"excludes {excluded}", checker.is_excluded_source_path(excluded))

    for included in (
        "alt-backend/app/main.go",
        "news-creator/app/main.py",
        "plecto/src/lib.rs",
        "alt-frontend-sv/src/lib/utils.ts",
    ):
        check(f"includes {included}", not checker.is_excluded_source_path(included))

    with tempfile.TemporaryDirectory() as tmp_str:
        repo = make_fixture_repo(pathlib.Path(tmp_str))
        run_git(repo, "init")
        (repo / "scripts" / "lint").mkdir(parents=True)
        al_path = repo / "scripts" / "lint" / "suppression_allowlist.toml"
        al_path.write_text(MINIMAL_ALLOWLIST_TOML, encoding="utf-8")
        (repo / "app.py").write_text(
            "x = 1  # noqa: F401 -- re-exported symbol\n", encoding="utf-8"
        )
        corpus_case = repo / "scripts" / "harness" / "corpus" / "C-22"
        corpus_case.mkdir(parents=True)
        (corpus_case / "after.py").write_text("# ruff: noqa: TRY003\nx = 1\n", encoding="utf-8")
        (corpus_case / "pyproject.toml").write_text(
            '[tool.ruff.lint]\nignore = ["F401"]\n',
            encoding="utf-8",
        )
        run_git(repo, "add", ".")

        res_ok = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [sys.executable, str(SCRIPT), "--repo-root", str(repo), "--allowlist", str(al_path)],
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            "CLI exits 0 on clean git repo (ignoring scripts/harness/corpus fixtures)",
            res_ok.returncode == 0,
            res_ok.stdout + res_ok.stderr,
        )

        (repo / "untracked_bad.go").write_text(
            "package main\nfunc u() { _ = 1 //nolint:noctx\n}\n",
            encoding="utf-8",
        )
        res_untracked = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [sys.executable, str(SCRIPT), "--repo-root", str(repo), "--allowlist", str(al_path)],
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            "CLI detects violation in untracked (un-staged) file before git add",
            res_untracked.returncode == 1
            and "untracked_bad.go:2:" in (res_untracked.stdout + res_untracked.stderr),
            f"rc={res_untracked.returncode} out={res_untracked.stdout + res_untracked.stderr}",
        )
        (repo / "untracked_bad.go").unlink()

        (repo / "bad.go").write_text(
            "package main\nfunc f() { _ = 1 //nolint:noctx\n}\n",
            encoding="utf-8",
        )
        run_git(repo, "add", "bad.go")
        res_fail = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [sys.executable, str(SCRIPT), "--repo-root", str(repo), "--allowlist", str(al_path)],
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            "CLI exits 1 and reports bad.go:2 violation",
            res_fail.returncode == 1
            and "bad.go:2:" in (res_fail.stdout + res_fail.stderr)
            and "source.go.nolint-missing-reason" in (res_fail.stdout + res_fail.stderr),
            f"rc={res_fail.returncode} out={res_fail.stdout + res_fail.stderr}",
        )

        res_missing = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [
                sys.executable,
                str(SCRIPT),
                "--repo-root",
                str(repo),
                "--allowlist",
                str(repo / "missing.toml"),
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            "CLI exits 2 when allowlist is missing",
            res_missing.returncode == USAGE_EXIT_CODE,
            f"rc={res_missing.returncode}",
        )


def test_real_repository() -> None:
    sys.stdout.write("real repository verification\n")
    check("suppression_allowlist.toml exists", ALLOWLIST.is_file(), str(ALLOWLIST))
    res = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [sys.executable, str(SCRIPT), "--today", "2026-11-22"],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "real repository passes check_suppressions.py with zero violations on 2026-11-22",
        res.returncode == 0,
        f"rc={res.returncode}\n{res.stdout}{res.stderr}",
    )
    res_expired = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [sys.executable, str(SCRIPT), "--today", "2027-05-23"],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "real repository flags expired suppressions on 2027-05-23",
        res_expired.returncode == 1
        and "config.allowlist.expired" in (res_expired.stdout + res_expired.stderr)
        and "source.broad.expired" in (res_expired.stdout + res_expired.stderr),
        f"rc={res_expired.returncode}\n{res_expired.stdout}{res_expired.stderr}",
    )


def main() -> int:
    checker = load_checker()
    test_allowlist_validation(checker)
    test_pyproject_checks(checker)
    test_golangci_and_cargo_checks(checker)
    test_python_source_checks(checker)
    test_sql_suppression_classification(checker)
    test_go_source_checks(checker)
    test_rust_and_ts_source_checks(checker)
    test_exclusions_and_cli(checker)
    test_real_repository()
    sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
    return 1 if RESULTS.failed else 0


if __name__ == "__main__":
    sys.exit(main())
