#!/usr/bin/env python3
"""Tests for scripts/agent-harness/postmortem_ledger.py.

Run:
    python3 scripts/tests/test-postmortem-ledger.py
"""

from __future__ import annotations

import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / "scripts" / "agent-harness" / "postmortem_ledger.py"
GIT_BIN = shutil.which("git")
assert GIT_BIN is not None, "git executable not found in PATH"

FIXTURE_PM_COUNT = 3
FIXTURE_ROW_COUNT = 8
FIXTURE_OPEN_ROW_COUNT = 4
PM1_ROW_COUNT = 2
MIXED_ROW_COUNT = 2
USAGE_EXIT_CODE = 2
CHECKLIST_ROW_COUNT = 4


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


def init_repo(tmp: pathlib.Path) -> pathlib.Path:
    repo = tmp / "repo"
    repo.mkdir()
    run_git(repo, "init")
    (repo / "docs" / "postmortems").mkdir(parents=True)
    (repo / "docs" / "ADR").mkdir(parents=True)
    (repo / "docs" / "ADR" / "000100.md").write_text("# ADR 000100\n", encoding="utf-8")
    (repo / "docs" / "ADR" / "000123.md").write_text("# ADR 000123\n", encoding="utf-8")
    (repo / "alt-backend" / "app").mkdir(parents=True)
    (repo / "alt-backend" / "app" / "handler.go").write_text("package app\n", encoding="utf-8")
    (repo / "backend" / "frontend").mkdir(parents=True)
    (repo / "backend" / "frontend" / "sre").write_text("file\n", encoding="utf-8")
    patterns_dir = repo / ".claude" / "skills" / "failure-pattern-review" / "references"
    patterns_dir.mkdir(parents=True)
    (patterns_dir / "patterns.md").write_text(
        "# 失敗の型\n\n"
        "## FP-01 失敗を成功・空・healthy に写す\n\nDesc.\n\n"
        "## FP-02 境界の片側だけを新契約にする\n\nDesc.\n\n"
        "## FP-06 デプロイ単位が実行時の依存を外す\n\nDesc.\n",
        encoding="utf-8",
    )
    return repo


def write_readme(repo: pathlib.Path, entries: list[tuple[str, str]]) -> None:
    lines = [
        "# Alt Postmortems",
        "",
        "## インシデント一覧",
        "",
        "| インシデント | 発生日 | 重大度 | 影響サービス | 根本原因 |",
        "|:---|:---:|:---:|:---|:---|",
    ]
    for pm_id, filename in entries:
        lines.append(f"| [{pm_id}]({filename}) | 2026-04-01 | SEV-3 | svc | cause |")
    (repo / "docs" / "postmortems" / "README.md").write_text(
        "\n".join(lines) + "\n", encoding="utf-8"
    )


def make_valid_pm(
    pm_id: str,
    title: str = "Incident",
    action_rows: list[str] | None = None,
    pattern_block: str | None = None,
) -> str:
    if action_rows is None:
        action_rows = [
            "| 1 | 予防 | Add guard in `alt-backend/app/handler.go` | backend-team | 2026-05-01 | TODO |",
            "| 2 | 検知 | Add alert ([[000123]]) | obs-team | 2026-04-15 | 完了 |",
        ]
    if pattern_block is None:
        pattern_block = (
            "## 失敗の型と再発\n\n"
            "- 失敗の型: FP-01, FP-02\n"
            "- 再発: なし\n"
            "- 持ち越し: なし\n"
            "- 止められたはずの検査: 契約テストの追加\n"
        )
    parts = [
        f"# {pm_id}: {title}",
        "",
        "## 根本原因分析",
        "",
        "Root cause text.",
        "",
        pattern_block.rstrip(),
        "",
        "## アクションアイテム",
        "",
        "| # | カテゴリ | アクション | 担当 | 期限 | ステータス |",
        "|---|---|---|---|---|---|",
        *action_rows,
        "",
    ]
    return "\n".join(parts)


def run_cli(repo: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603 - fixed argv built in this test
        [sys.executable, str(SCRIPT_PATH), "--root", str(repo), *args],
        capture_output=True,
        text=True,
        check=False,
    )


sys.stdout.write("1. ledger parses current table, legacy D-1 table, and unparsed file\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    pm1 = "PM-2026-001-current-style.md"
    pm2 = "PM-2026-002-legacy-style.md"
    pm3 = "PM-2026-003-no-table.md"
    (repo / "docs" / "postmortems" / pm1).write_text(make_valid_pm("PM-2026-001"), encoding="utf-8")
    (repo / "docs" / "postmortems" / pm2).write_text(
        "# PM-2026-002: Legacy\n\n"
        "## アクションアイテム\n\n"
        "### 予防（Prevent）\n\n"
        "| # | アクション | 担当 | 期限 | 状態 |\n"
        "|---|---|---|---|---|\n"
        "| P-1 | Fix retry loop | alice | 2026-03-20 | Done |\n"
        "| P-2 | Drop legacy | bob | 2026-03-25 | Won't do |\n\n"
        "### 検知（Detect）\n\n"
        "| # | アクション | 担当 | 期限 | 状態 |\n"
        "|---|---|---|---|---|\n"
        "| D-1 | Add alert for PM-2026-001 item | carol | 2026-03-30 | 未着手 |\n"
        "| D-2 | Check metric | dave | 2026-04-05 | 進行中 |\n"
        "| D-3 | Verify dashboard | eve | 2026-04-10 | ✓ |\n"
        "| D-4 | Weird status | frank | 2026-04-12 | 検討中 |\n",
        encoding="utf-8",
    )
    (repo / "docs" / "postmortems" / pm3).write_text(
        "# PM-2026-003: Prose only\n\n## アクションアイテム\n\nNo table or checklist here.\n",
        encoding="utf-8",
    )
    write_readme(
        repo,
        [("PM-2026-001", pm1), ("PM-2026-002", pm2), ("PM-2026-003", pm3)],
    )

    res = run_cli(repo, "--json")
    check("ledger exits 0", res.returncode == 0, f"rc={res.returncode}, err={res.stderr}")
    try:
        data = json.loads(res.stdout)
    except json.JSONDecodeError as exc:
        data = {}
        check("ledger --json is valid JSON", False, str(exc))
    else:
        check("ledger --json is valid JSON", True)
        rows = data.get("rows", [])
        summary = data.get("summary", {})
        unparsed = data.get("unparsed", [])
        check(
            "summary counts 3 postmortems",
            summary.get("postmortems") == FIXTURE_PM_COUNT,
            str(summary),
        )
        check("summary counts 8 rows", summary.get("rows") == FIXTURE_ROW_COUNT, str(summary))
        check("summary counts 1 unparsed", summary.get("unparsed") == 1, str(summary))
        check("pm3 listed in unparsed", any(pm3 in u for u in unparsed), str(unparsed))
        by_id = {(r["postmortem_id"], r["item_id"]): r for r in rows}
        check(
            "D-1 parsed with category 検知 and status open",
            by_id.get(("PM-2026-002", "D-1"), {}).get("category") == "検知"
            and by_id.get(("PM-2026-002", "D-1"), {}).get("normalised_status") == "open",
            str(by_id.get(("PM-2026-002", "D-1"))),
        )
        check(
            "P-1 Done -> done",
            by_id.get(("PM-2026-002", "P-1"), {}).get("normalised_status") == "done",
            str(by_id.get(("PM-2026-002", "P-1"))),
        )
        check(
            "P-2 Won't do -> dropped",
            by_id.get(("PM-2026-002", "P-2"), {}).get("normalised_status") == "dropped",
            str(by_id.get(("PM-2026-002", "P-2"))),
        )
        check(
            "D-2 進行中 -> open",
            by_id.get(("PM-2026-002", "D-2"), {}).get("normalised_status") == "open",
            str(by_id.get(("PM-2026-002", "D-2"))),
        )
        check(
            "D-3 checkmark -> done",
            by_id.get(("PM-2026-002", "D-3"), {}).get("normalised_status") == "done",
            str(by_id.get(("PM-2026-002", "D-3"))),
        )
        check(
            "D-4 unknown -> unknown",
            by_id.get(("PM-2026-002", "D-4"), {}).get("normalised_status") == "unknown",
            str(by_id.get(("PM-2026-002", "D-4"))),
        )

    sys.stdout.write("2. ledger filters (--open, --overdue, --match, --carried) and text output\n")
    txt_res = run_cli(repo)
    check(
        "text output has summary and lag disclaimer",
        txt_res.returncode == 0
        and "postmortems" in txt_res.stdout
        and "unparsed" in txt_res.stdout
        and ("lag" in txt_res.stdout.lower() or "乖離" in txt_res.stdout),
        txt_res.stdout,
    )

    open_res = run_cli(repo, "--open", "--json")
    open_rows = json.loads(open_res.stdout).get("rows", []) if open_res.returncode == 0 else []
    check(
        "--open includes open and unknown only (4 rows)",
        len(open_rows) == FIXTURE_OPEN_ROW_COUNT
        and all(r["normalised_status"] in {"open", "unknown"} for r in open_rows),
        str(open_rows),
    )

    overdue_res = run_cli(repo, "--overdue", "--today", "2026-04-20", "--json")
    overdue_rows = (
        json.loads(overdue_res.stdout).get("rows", []) if overdue_res.returncode == 0 else []
    )
    check(
        "--overdue before 2026-04-20 returns D-1, D-2, and unknown-status D-4",
        {(r["postmortem_id"], r["item_id"]) for r in overdue_rows}
        == {("PM-2026-002", "D-1"), ("PM-2026-002", "D-2"), ("PM-2026-002", "D-4")},
        str(overdue_rows),
    )

    match_res = run_cli(repo, "--match", "carol", "--json")
    match_rows = json.loads(match_res.stdout).get("rows", []) if match_res.returncode == 0 else []
    check(
        "--match matches owner",
        len(match_rows) == 1 and match_rows[0]["item_id"] == "D-1",
        str(match_rows),
    )

    match_file_res = run_cli(repo, "--match", "current-style", "--json")
    match_file_rows = (
        json.loads(match_file_res.stdout).get("rows", []) if match_file_res.returncode == 0 else []
    )
    check(
        "--match matches postmortem filename",
        len(match_file_rows) == PM1_ROW_COUNT
        and all(r["postmortem_id"] == "PM-2026-001" for r in match_file_rows),
        str(match_file_rows),
    )

    carried_res = run_cli(repo, "--carried", "--json")
    carried_rows = (
        json.loads(carried_res.stdout).get("rows", []) if carried_res.returncode == 0 else []
    )
    check(
        "--carried returns only D-1 citing PM-2026-001",
        len(carried_rows) == 1 and carried_rows[0]["item_id"] == "D-1",
        str(carried_rows),
    )

sys.stdout.write("3. gate passes well-formed new postmortem and ignores existing malformed one\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    old_pm = "PM-2026-001-old-malformed.md"
    # Malformed existing postmortem at base: no table, no failure-pattern section.
    (repo / "docs" / "postmortems" / old_pm).write_text(
        "# PM-2026-001: Old malformed\n\nJust prose.\n", encoding="utf-8"
    )
    write_readme(repo, [("PM-2026-001", old_pm)])
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    new_pm = "PM-2026-002-new-valid.md"
    (repo / "docs" / "postmortems" / new_pm).write_text(
        make_valid_pm(
            "PM-2026-002",
            action_rows=[
                "| 1 | 予防 | Fix handler in `alt-backend/app/handler.go` | alice | 2026-05-10 | 完了 |",
                "| 2 | 検知 | Wire alert (PR #42) | bob | 2026-05-12 | **完了** |",
                "| 3 | 緩和 | Patch config (commit `a1b2c3d`) | carol | 2026-05-15 | 完了 |",
                "| 4 | プロセス | Follow up [[000100]] | dave | 2026-05-20 | 進行中 |",
                "| 5 | プロセス | Drop idea（破棄理由: 別の恒久対策へ統合したため不要） | eve | 2026-05-21 | 破棄 |",
            ],
            pattern_block=(
                "## 失敗の型と再発\n\n"
                "- 失敗の型: 型の外: 外部電源断による物理ホスト停止\n"
                "- 再発: PM-2026-001\n"
                "- 持ち越し: なし\n"
                "- 止められたはずの検査: CI の統合テスト\n"
            ),
        ),
        encoding="utf-8",
    )
    write_readme(repo, [("PM-2026-001", old_pm), ("PM-2026-002", new_pm)])
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "add valid pm")

    res = run_cli(repo, "--check", "--base", base_rev)
    check(
        "valid new PM + old malformed PM exits 0",
        res.returncode == 0,
        f"rc={res.returncode}, out={res.stdout!r}, err={res.stderr!r}",
    )

sys.stdout.write("4. gate reports each of P1..P10 individually\n")


def run_single_violation(
    pm_content: str | None,
    readme_entries: list[tuple[str, str]] | None = None,
) -> subprocess.CompletedProcess[str]:
    with tempfile.TemporaryDirectory() as td:
        repo = init_repo(pathlib.Path(td))
        base_pm = "PM-2026-001-base.md"
        (repo / "docs" / "postmortems" / base_pm).write_text(
            make_valid_pm("PM-2026-001"), encoding="utf-8"
        )
        write_readme(repo, [("PM-2026-001", base_pm)])
        run_git(repo, "add", ".")
        run_git(repo, "commit", "-m", "base")
        base_rev = run_git(repo, "rev-parse", "HEAD")

        entries = [("PM-2026-001", base_pm)]
        if pm_content is not None:
            new_pm = "PM-2026-002-new.md"
            (repo / "docs" / "postmortems" / new_pm).write_text(pm_content, encoding="utf-8")
            entries.append(("PM-2026-002", new_pm))
        if readme_entries is not None:
            entries = readme_entries
        write_readme(repo, entries)
        run_git(repo, "add", ".")
        run_git(repo, "commit", "-m", "change")
        return run_cli(repo, "--check", "--base", base_rev)


r_p1 = run_single_violation(
    "# PM-2026-002: Missing table\n\n"
    "## 失敗の型と再発\n\n"
    "- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n\n"
    "## アクションアイテム\n\nNo table here.\n"
)
check("P1 triggered on missing table", r_p1.returncode == 1 and "P1" in r_p1.stdout, r_p1.stdout)

r_p2 = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        action_rows=["| 1 | 予防 | Do something | TBD | 2026-05-01 | TODO |"],
    )
)
check("P2 triggered on TBD owner", r_p2.returncode == 1 and "P2" in r_p2.stdout, r_p2.stdout)

r_p3 = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        action_rows=["| 1 | 予防 | Do something | alice | 来月 | TODO |"],
    )
)
check(
    "P3 triggered on non-YYYY-MM-DD due", r_p3.returncode == 1 and "P3" in r_p3.stdout, r_p3.stdout
)

r_p4 = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        action_rows=["| 1 | 予防 | Do something | alice | 2026-05-01 | 未着手 |"],
    )
)
check(
    "P4 triggered on non-standard status", r_p4.returncode == 1 and "P4" in r_p4.stdout, r_p4.stdout
)

r_p5 = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        action_rows=[
            "| 1 | 予防 | Fixed the bug without citing anything | alice | 2026-05-01 | 完了 |"
        ],
    )
)
check(
    "P5 triggered on 完了 without evidence",
    r_p5.returncode == 1 and "P5" in r_p5.stdout,
    r_p5.stdout,
)

r_p6 = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        pattern_block=(
            "## 失敗の型と再発\n\n"
            "- 失敗の型: FP-01\n"
            "- 再発: なし\n"
            "- 持ち越し: \n"
            "- 止められたはずの検査: test\n"
        ),
    )
)
check("P6 triggered on empty 持ち越し", r_p6.returncode == 1 and "P6" in r_p6.stdout, r_p6.stdout)

r_p7 = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        pattern_block=(
            "## 失敗の型と再発\n\n"
            "- 失敗の型: FP-99\n"
            "- 再発: なし\n"
            "- 持ち越し: なし\n"
            "- 止められたはずの検査: test\n"
        ),
    )
)
check("P7 triggered on unknown FP-99", r_p7.returncode == 1 and "P7" in r_p7.stdout, r_p7.stdout)

r_p7b = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        pattern_block=(
            "## 失敗の型と再発\n\n"
            "- 失敗の型: よく分からない\n"
            "- 再発: なし\n"
            "- 持ち越し: なし\n"
            "- 止められたはずの検査: test\n"
        ),
    )
)
check(
    "P7 triggered on non-FP prose without 型の外",
    r_p7b.returncode == 1 and "P7" in r_p7b.stdout,
    r_p7b.stdout,
)

r_p8 = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        pattern_block=(
            "## 失敗の型と再発\n\n"
            "- 失敗の型: FP-01\n"
            "- 再発: PM-2026-999\n"
            "- 持ち越し: なし\n"
            "- 止められたはずの検査: test\n"
        ),
    )
)
check(
    "P8 triggered on missing PM-2026-999", r_p8.returncode == 1 and "P8" in r_p8.stdout, r_p8.stdout
)

r_p9 = run_single_violation(
    make_valid_pm("PM-2026-002"),
    readme_entries=[("PM-2026-001", "PM-2026-001-base.md")],
)
check(
    "P9 triggered when README misses PM-2026-002",
    r_p9.returncode == 1 and "P9" in r_p9.stdout,
    r_p9.stdout,
)

r_p10 = run_single_violation(
    None,
    readme_entries=[
        ("PM-2026-001", "PM-2026-001-base.md"),
        ("PM-2026-999", "PM-2026-999-ghost.md"),
    ],
)
check(
    "P10 triggered when README cites nonexistent PM-2026-999",
    r_p10.returncode == 1 and "P10" in r_p10.stdout,
    r_p10.stdout,
)

r_p1_checklist = run_single_violation(
    "# PM-2026-002: Checklist instead of table\n\n"
    "## 失敗の型と再発\n\n"
    "- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n\n"
    "## アクションアイテム\n\n"
    "### 予防（Prevent）\n\n"
    "- [ ] Fix something — **担当: alice、期限: 2026-05-01**\n"
)
check(
    "P1 triggered when new PM uses checklist instead of table",
    r_p1_checklist.returncode == 1 and "P1" in r_p1_checklist.stdout,
    r_p1_checklist.stdout,
)

sys.stdout.write("4b. review follow-up gate & ledger checks (L1..L9)\n")

# L1: accidental P5 evidence strings must all be rejected; real ADR/PR/hash/path accepted
accidental_evidence_cases = [
    ("date slash", "2026/05/01 に対応済み", "alice"),
    ("service slash", "Go/Python/Rust の全サービスで確認", "alice"),
    ("p50/p95/p99", "p50/p95/p99 を計測した", "alice"),
    ("mime backtick", "`application/json` を返す", "alice"),
    ("plain 7-digit number", "1500000 件を再処理した", "alice"),
    ("8-digit date number", "20260501 に確認", "alice"),
    ("UUID", "550e8400-e29b-41d4-a716-446655440000 を処理", "alice"),
    ("bare #1 item ref", "デプロイは #1 に含む", "alice"),
    ("hex color #333", "色を #333 に統一", "alice"),
    ("nonexistent backtick path", "`does/not/exist.go` を修正", "alice"),
    ("nonexistent ADR wikilink", "[[999999]] で対応", "alice"),
    ("owner cell path", "対応完了", "backend/frontend/sre"),
]
for label, act, own in accidental_evidence_cases:
    r_acc = run_single_violation(
        make_valid_pm(
            "PM-2026-002",
            action_rows=[f"| 1 | 予防 | {act} | {own} | 2026-05-01 | 完了 |"],
        )
    )
    check(
        f"L1 P5 rejects accidental evidence ({label})",
        r_acc.returncode == 1 and "P5" in r_acc.stdout,
        f"rc={r_acc.returncode}, out={r_acc.stdout!r}",
    )

accepted_evidence_cases = [
    ("PR without space before Japanese", "PR #286で修正", "完了"),
    ("PR#N without space", "PR#286 で修正", "完了"),
    ("pull URL", "https://example.com/org/repo/pull/123 で修正", "完了"),
    ("commit hash adjacent to Japanese", "コミット1c7679801で修正", "完了"),
    ("existing ADR wikilink", "[[000123]] で対応", "完了"),
    ("existing backtick path", "`alt-backend/app/handler.go` を修正", "完了"),
    ("bold status **完了**", "PR #286で修正", "**完了**"),
]
for label, act, st in accepted_evidence_cases:
    r_ok = run_single_violation(
        make_valid_pm(
            "PM-2026-002",
            action_rows=[f"| 1 | 予防 | {act} | alice | 2026-05-01 | {st} |"],
        )
    )
    check(
        f"L1/L7 accepts valid evidence ({label})",
        r_ok.returncode == 0,
        f"rc={r_ok.returncode}, out={r_ok.stdout!r}",
    )

# L2: P11 non-standard filenames
for bad_name in ("PM-2026-064.md", "pm-2026-065-lowercase.md"):
    with tempfile.TemporaryDirectory() as td:
        repo = init_repo(pathlib.Path(td))
        base_pm = "PM-2026-001-base.md"
        (repo / "docs" / "postmortems" / base_pm).write_text(
            make_valid_pm("PM-2026-001"), encoding="utf-8"
        )
        write_readme(repo, [("PM-2026-001", base_pm)])
        run_git(repo, "add", ".")
        run_git(repo, "commit", "-m", "base")
        base_rev = run_git(repo, "rev-parse", "HEAD")

        (repo / "docs" / "postmortems" / bad_name).write_text("# bad\n", encoding="utf-8")
        run_git(repo, "add", ".")
        run_git(repo, "commit", "-m", "add bad filename")
        r_p11 = run_cli(repo, "--check", "--base", base_rev)
        check(
            f"L2 P11 triggered for {bad_name}",
            r_p11.returncode == 1 and "P11" in r_p11.stdout,
            r_p11.stdout,
        )

# L3: rename counts as added file (--no-renames)
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    old_pm = "PM-2026-001-old.md"
    (repo / "docs" / "postmortems" / old_pm).write_text(
        "# PM-2026-001: Old\n\nJust prose.\n", encoding="utf-8"
    )
    write_readme(repo, [("PM-2026-001", old_pm)])
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    new_pm = "PM-2026-001-renamed.md"
    run_git(repo, "mv", f"docs/postmortems/{old_pm}", f"docs/postmortems/{new_pm}")
    write_readme(repo, [("PM-2026-001", new_pm)])
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "rename pm")
    r_ren = run_cli(repo, "--check", "--base", base_rev)
    check(
        "L3 renamed malformed PM is gated and reports P1",
        r_ren.returncode == 1 and "P1" in r_ren.stdout,
        r_ren.stdout,
    )

# L4: unmappable table under action items -> P1 in gate; table + checklist combined in ledger
r_unmap = run_single_violation(
    "# PM-2026-002: Unmappable table\n\n"
    "## 根本原因分析\n\nRoot.\n\n"
    "## 失敗の型と再発\n\n"
    "- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n\n"
    "## アクションアイテム\n\n"
    "| 項目 | 値 |\n|---|---|\n| foo | bar |\n"
)
check(
    "L4 unmappable table triggers P1 in gate",
    r_unmap.returncode == 1 and "P1" in r_unmap.stdout,
    r_unmap.stdout,
)

with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    pm_mix = "PM-2026-001-mixed.md"
    (repo / "docs" / "postmortems" / pm_mix).write_text(
        "# PM-2026-001: Mixed\n\n"
        "## アクションアイテム\n\n"
        "| # | カテゴリ | アクション | 担当 | 期限 | ステータス |\n"
        "|---|---|---|---|---|---|\n"
        "| 1 | 予防 | Table item | alice | 2026-05-01 | TODO |\n\n"
        "### 検知（Detect）\n\n"
        "- [ ] Checklist item — 担当: bob、期限: 2026-05-02\n",
        encoding="utf-8",
    )
    write_readme(repo, [("PM-2026-001", pm_mix)])
    mix_res = run_cli(repo, "--json")
    mix_rows = json.loads(mix_res.stdout).get("rows", []) if mix_res.returncode == 0 else []
    check(
        "L4 ledger parses both table and checklist when both present",
        len(mix_rows) == MIXED_ROW_COUNT,
        str(mix_rows),
    )

# L5 + L6: --overdue with trailing text & --carried short form PM-NNN
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    pm_35 = "PM-2026-035-prior.md"
    pm_36 = "PM-2026-036-follow.md"
    (repo / "docs" / "postmortems" / pm_35).write_text(
        make_valid_pm("PM-2026-035"), encoding="utf-8"
    )
    (repo / "docs" / "postmortems" / pm_36).write_text(
        "# PM-2026-036: Follow\n\n"
        "## アクションアイテム\n\n"
        "| # | カテゴリ | アクション | 担当 | 期限 | 状態 |\n"
        "|---|---|---|---|---|---|\n"
        "| 1 | 予防 | PM-035 から継承した対策 | alice | 2026-04-19 〜継続 | TODO |\n"
        "| 2 | 検知 | 監視追加 | bob | 2026-09-08（本番反映） | 保留 |\n"
        "| 3 | 緩和 | Self ref PM-036 only | carol | 2026-12-01 | TODO |\n",
        encoding="utf-8",
    )
    write_readme(repo, [("PM-2026-035", pm_35), ("PM-2026-036", pm_36)])

    ov_res = run_cli(repo, "--overdue", "--today", "2026-10-01", "--json")
    ov_rows = json.loads(ov_res.stdout).get("rows", []) if ov_res.returncode == 0 else []
    check(
        "L5 --overdue parses leading YYYY-MM-DD with trailing text and includes unknown",
        {(r["postmortem_id"], r["item_id"]) for r in ov_rows}
        == {("PM-2026-035", "1"), ("PM-2026-036", "1"), ("PM-2026-036", "2")},
        str(ov_rows),
    )

    car_res = run_cli(repo, "--carried", "--json")
    car_rows = json.loads(car_res.stdout).get("rows", []) if car_res.returncode == 0 else []
    check(
        "L6 --carried matches short form PM-035 and ignores self PM-036",
        [(r["postmortem_id"], r["item_id"]) for r in car_rows] == [("PM-2026-036", "1")],
        str(car_rows),
    )

# L7 / L9: P6 missing section, P6 wrong position, P7 short 型の外, P8 on 持ち越し, P12
r_p6_missing = run_single_violation(
    "# PM-2026-002: Missing FP section\n\n"
    "## 根本原因分析\n\nRoot.\n\n"
    "## アクションアイテム\n\n"
    "| # | カテゴリ | アクション | 担当 | 期限 | ステータス |\n"
    "|---|---|---|---|---|---|\n"
    "| 1 | 予防 | Do something | alice | 2026-05-01 | TODO |\n"
)
check(
    "L9 P6 missing section",
    r_p6_missing.returncode == 1 and "P6" in r_p6_missing.stdout,
    r_p6_missing.stdout,
)

r_p6_pos = run_single_violation(
    "# PM-2026-002: Wrong position\n\n"
    "## 根本原因分析\n\nRoot.\n\n"
    "## 対応の評価\n\nEval.\n\n"
    "## 失敗の型と再発\n\n"
    "- 失敗の型: FP-01\n- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n\n"
    "## アクションアイテム\n\n"
    "| # | カテゴリ | アクション | 担当 | 期限 | ステータス |\n"
    "|---|---|---|---|---|---|\n"
    "| 1 | 予防 | Do something | alice | 2026-05-01 | TODO |\n"
)
check(
    "L7/L9 P6 wrong section position",
    r_p6_pos.returncode == 1 and "P6" in r_p6_pos.stdout,
    r_p6_pos.stdout,
)

r_p7_short = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        pattern_block=(
            "## 失敗の型と再発\n\n"
            "- 失敗の型: 型の外: 短い\n"
            "- 再発: なし\n- 持ち越し: なし\n- 止められたはずの検査: test\n"
        ),
    )
)
check(
    "L7 P7 rejects 型の外 with <10 char reason",
    r_p7_short.returncode == 1 and "P7" in r_p7_short.stdout,
    r_p7_short.stdout,
)

r_p8_carried = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        pattern_block=(
            "## 失敗の型と再発\n\n"
            "- 失敗の型: FP-01\n- 再発: なし\n"
            "- 持ち越し: PM-2026-999\n- 止められたはずの検査: test\n"
        ),
    )
)
check(
    "L9 P8 triggered on 持ち越し citing nonexistent PM",
    r_p8_carried.returncode == 1 and "P8" in r_p8_carried.stdout,
    r_p8_carried.stdout,
)

r_p12_missing = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        action_rows=["| 1 | 予防 | Drop this item | alice | 2026-05-01 | 破棄 |"],
    )
)
check(
    "L7/L9 P12 triggered when 破棄 lacks 破棄理由:",
    r_p12_missing.returncode == 1 and "P12" in r_p12_missing.stdout,
    r_p12_missing.stdout,
)

r_p12_short = run_single_violation(
    make_valid_pm(
        "PM-2026-002",
        action_rows=["| 1 | 予防 | Drop item 破棄理由: 不要 | alice | 2026-05-01 | 破棄 |"],
    )
)
check(
    "L7/L9 P12 triggered when 破棄理由: <10 chars",
    r_p12_short.returncode == 1 and "P12" in r_p12_short.stdout,
    r_p12_short.stdout,
)

sys.stdout.write("5. usage / git errors exit 2 & no-added-PM check prints merge base\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    write_readme(repo, [])
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base_rev = run_git(repo, "rev-parse", "HEAD")
    res_bad_rev = run_cli(repo, "--check", "--base", "does-not-exist")
    check(
        "unknown base rev exits 2",
        res_bad_rev.returncode == USAGE_EXIT_CODE,
        f"rc={res_bad_rev.returncode}",
    )
    res_base_no_check = run_cli(repo, "--base", base_rev)
    check(
        "L9 --base without --check exits 2 with usage message",
        res_base_no_check.returncode == USAGE_EXIT_CODE and bool(res_base_no_check.stderr.strip()),
        f"rc={res_base_no_check.returncode}, err={res_base_no_check.stderr!r}",
    )
    res_no_add = run_cli(repo, "--check", "--base", base_rev)
    check(
        "L8 --check with no added PM prints merge base and exits 0",
        res_no_add.returncode == 0 and base_rev in res_no_add.stdout,
        f"rc={res_no_add.returncode}, out={res_no_add.stdout!r}",
    )

sys.stdout.write("6. ledger parses Markdown checklists under category subheadings\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    pm_cl = "PM-2026-028-checklist-style.md"
    (repo / "docs" / "postmortems" / pm_cl).write_text(
        "# PM-2026-028: Checklist postmortem\n\n"
        "## アクションアイテム\n\n"
        "### 予防（Prevent）\n\n"
        "- [x] **[Platform] Done item without owner** — 完了: 2026-04-16\n"
        "- [ ] **[Platform] Open item** — 何かを直す。**担当: platform、期限: 2026-04-30**\n\n"
        "### 検知 (Detect)\n\n"
        "- [X] **[Platform] Uppercase X item** — 完了: 2026-04-17\n"
        "- [ ] Wrapped multiline item\n"
        "  detail line — **担当:** recap-worker\n"
        "  owner、**期限:** 2026-05-15。\n",
        encoding="utf-8",
    )
    write_readme(repo, [("PM-2026-028", pm_cl)])

    cl_res = run_cli(repo, "--json")
    check("checklist ledger exits 0", cl_res.returncode == 0, cl_res.stderr)
    cl_data = json.loads(cl_res.stdout)
    cl_rows = cl_data.get("rows", [])
    cl_summary = cl_data.get("summary", {})
    check("checklist summary unparsed == 0", cl_summary.get("unparsed") == 0, str(cl_summary))
    check("checklist parsed 4 rows", len(cl_rows) == CHECKLIST_ROW_COUNT, str(cl_rows))
    by_cl_id = {r["item_id"]: r for r in cl_rows}
    check(
        "item 1: category=予防, norm=done, owner='', due='', no checkbox prefix",
        by_cl_id.get("1", {}).get("category") == "予防"
        and by_cl_id.get("1", {}).get("normalised_status") == "done"
        and by_cl_id.get("1", {}).get("owner") == ""
        and by_cl_id.get("1", {}).get("due") == ""
        and not by_cl_id.get("1", {}).get("action", "").startswith("- ["),
        str(by_cl_id.get("1")),
    )
    check(
        "item 2: category=予防, norm=open, owner=platform, due=2026-04-30",
        by_cl_id.get("2", {}).get("category") == "予防"
        and by_cl_id.get("2", {}).get("normalised_status") == "open"
        and by_cl_id.get("2", {}).get("owner") == "platform"
        and by_cl_id.get("2", {}).get("due") == "2026-04-30",
        str(by_cl_id.get("2")),
    )
    check(
        "item 3: category=検知, [X] -> done",
        by_cl_id.get("3", {}).get("category") == "検知"
        and by_cl_id.get("3", {}).get("normalised_status") == "done",
        str(by_cl_id.get("3")),
    )
    check(
        "item 4: wrapped multiline owner=recap-worker owner, due=2026-05-15",
        by_cl_id.get("4", {}).get("category") == "検知"
        and by_cl_id.get("4", {}).get("normalised_status") == "open"
        and by_cl_id.get("4", {}).get("owner") == "recap-worker owner"
        and by_cl_id.get("4", {}).get("due") == "2026-05-15",
        str(by_cl_id.get("4")),
    )

sys.stdout.write("7. real-repository run\n")
expected_pm_count = len(
    list((ROOT / "docs" / "postmortems").glob("PM-[0-9][0-9][0-9][0-9]-[0-9][0-9][0-9]-*.md"))
)
real_res = subprocess.run(  # noqa: S603 - fixed argv built in this test
    [sys.executable, str(SCRIPT_PATH), "--json"],
    capture_output=True,
    text=True,
    check=False,
)
check(
    "real repository ledger exits 0",
    real_res.returncode == 0,
    f"rc={real_res.returncode}, err={real_res.stderr}",
)
if real_res.returncode == 0:
    real_data = json.loads(real_res.stdout)
    check(
        f"real repository summary counts {expected_pm_count} postmortems",
        real_data.get("summary", {}).get("postmortems") == expected_pm_count,
        str(real_data.get("summary")),
    )
    check(
        "real repository has parsed rows > 0",
        real_data.get("summary", {}).get("rows", 0) > 0,
        str(real_data.get("summary")),
    )
    check(
        "real repository has unparsed == 0",
        real_data.get("summary", {}).get("unparsed") == 0,
        str(real_data.get("unparsed")),
    )
    pinned = {
        "PM-2026-024": (13, 4, 9),
        "PM-2026-028": (16, 11, 5),
        "PM-2026-029": (10, 4, 6),
    }
    real_rows = real_data.get("rows", [])
    for pm_id, (exp_total, exp_done, exp_open) in pinned.items():
        pm_rows = [r for r in real_rows if r.get("postmortem_id") == pm_id]
        got_done = sum(1 for r in pm_rows if r.get("normalised_status") == "done")
        got_open = sum(1 for r in pm_rows if r.get("normalised_status") == "open")
        check(
            f"L9 {pm_id} pinned counts ({exp_total} rows: {exp_done} done, {exp_open} open)",
            (len(pm_rows), got_done, got_open) == (exp_total, exp_done, exp_open),
            f"got={(len(pm_rows), got_done, got_open)}",
        )

sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
sys.exit(1 if RESULTS.failed else 0)
