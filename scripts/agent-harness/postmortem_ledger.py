#!/usr/bin/env python3
"""Postmortem action-item ledger and CI gate for docs/postmortems/.

Modes:
  Ledger (default):
    Parse every docs/postmortems/PM-YYYY-NNN-*.md, extract action-item tables
    under headings containing "アクションアイテム", normalise statuses, and
    emit rows + unparsed list + summary counts.

  Gate / Consistency check (--check [--base REV]):
    - Index consistency (always when --check is passed): every PM-YYYY-NNN-*.md
      has a row in docs/postmortems/README.md (P9) and every PM-YYYY-NNN in the
      README index table has a corresponding file (P10).
    - Added postmortem gate (when --base REV is passed): validates P1..P9 on
      postmortem files added relative to git merge-base(REV, HEAD) (plus
      untracked added files). Existing postmortems are never gated.

Exit codes:
  0  Clean
  1  Gate / consistency findings (--check)
  2  CLI usage, missing directory/file, or git error
"""

from __future__ import annotations

import argparse
from dataclasses import asdict, dataclass
import datetime
import json
import pathlib
import re
import subprocess
import sys

PM_FILENAME_RE = re.compile(r"^(PM-\d{4}-\d{3})-(.+)\.md$")
PM_ID_RE = re.compile(r"\b(PM-\d{4}-\d{3})\b")
PM_SHORT_ID_RE = re.compile(r"(?<![A-Za-z0-9_-])PM-(\d{3})(?![0-9-])")
H2_RE = re.compile(r"^##\s+(.*)$")
H3_RE = re.compile(r"^###\s+(.*)$")
FENCE_RE = re.compile(r"^\s*```")
TABLE_SEP_RE = re.compile(r"^\s*\|(?:\s*:?-+:?\s*\|)+\s*$")
DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
LEADING_DATE_RE = re.compile(r"^\s*(\d{4}-\d{2}-\d{2})")
HTML_COMMENT_RE = re.compile(r"<!--.*?-->", re.S)

PATTERN_HEADING_RE = re.compile(r"^##\s+(FP-\d+)\s+")
FP_ID_RE = re.compile(r"^FP-\d+$")

# Evidence patterns for P5 (completed items, action/status cells only).
ADR_WIKILINK_RE = re.compile(r"\[\[(\d{6})(?:\|[^\]\n]*)?\]\]")
PR_REF_RE = re.compile(r"(?<![A-Za-z0-9_])PR\s*#\d+(?![A-Za-z0-9_])")
PULL_URL_RE = re.compile(r"https?://[^\s)>\]\"'`]+/pull/\d+(?![A-Za-z0-9_/])")
COMMIT_HASH_RE = re.compile(r"(?<![A-Za-z0-9-])[0-9a-f]{7,40}(?![A-Za-z0-9-])")
BACKTICK_SPAN_RE = re.compile(r"`([^`\n]+)`")
DROP_REASON_RE = re.compile(r"破棄理由\s*[:：]\s*(.+)")

FAILURE_PATTERN_KEYS = (
    "失敗の型",
    "再発",
    "持ち越し",
    "止められたはずの検査",
)
FAILURE_ITEM_RE = re.compile(
    r"^\s*[-*]\s+(?:\*\*)?(失敗の型|再発|持ち越し|止められたはずの検査)(?:\*\*)?\s*[:：]\s*(?:\*\*)?\s*(.*)$"
)

GATE_ALLOWED_STATUSES = frozenset({"TODO", "進行中", "完了", "破棄"})
PLACEHOLDER_OWNERS = frozenset({"", "要アサイン", "tbd", "未定", "-", "—"})

CHECKMARK_CHARS = ("✓", "✔", "✅", "☑")
CHECKBOX_DONE_RE = re.compile(r"\[x\]", re.IGNORECASE)

CHECKLIST_ITEM_RE = re.compile(r"^\s*-\s+\[([ xX])\]\s+(.*)$")
CHECKLIST_OWNER_RE = re.compile(r"担当\s*(?:\*\*)?\s*[:：]\s*(?:\*\*)?\s*([^、,。\n*]+)")
CHECKLIST_DUE_RE = re.compile(r"期限\s*(?:\*\*)?\s*[:：]\s*(?:\*\*)?\s*(\d{4}-\d{2}-\d{2})")

OPEN_TOKENS = frozenset({"todo", "未着手", "未", "進行中", "in progress", "in_progress", "open"})
DONE_TOKENS = frozenset({"done", "完了"})
DROPPED_TOKENS = frozenset({"破棄", "won't do", "wont do", "dropped"})


@dataclass
class ActionRow:
    postmortem_id: str
    file: str
    item_id: str
    category: str
    action: str
    owner: str
    due: str
    status: str
    normalised_status: str


@dataclass
class ParsedActionItems:
    table_rows: list[ActionRow]
    checklist_rows: list[ActionRow]
    unmapped_tables: int

    @property
    def all_rows(self) -> list[ActionRow]:
        return [*self.table_rows, *self.checklist_rows]


def split_md_row(line: str) -> list[str]:
    """Split a Markdown table row into stripped cells, preserving \\| and `...` spans."""
    s = line.strip()
    if s.startswith("|"):
        s = s[1:]
    if s.endswith("|") and not s.endswith("\\|"):
        s = s[:-1]

    cells: list[str] = []
    buf: list[str] = []
    in_code = False
    i = 0
    n = len(s)
    while i < n:
        ch = s[i]
        if ch == "\\" and i + 1 < n and s[i + 1] == "|":
            buf.append("|")
            i += 2
            continue
        if ch == "`":
            if in_code:
                in_code = False
            elif "`" in s[i + 1 :]:
                in_code = True
            buf.append(ch)
            i += 1
            continue
        if ch == "|" and not in_code:
            cells.append("".join(buf).strip())
            buf = []
            i += 1
            continue
        buf.append(ch)
        i += 1
    cells.append("".join(buf).strip())
    return cells


def normalise_status(raw_status: str) -> str:
    """Normalise a raw postmortem status string into {open, done, dropped, unknown}."""
    stripped = raw_status.strip()
    if not stripped:
        return "unknown"

    if any(mark in stripped for mark in CHECKMARK_CHARS) or CHECKBOX_DONE_RE.search(stripped):
        return "done"

    # Strip parenthetical notes like "DONE (PR #286)" or "完了（2026-05-07）"
    head = re.split(r"[（(]", stripped, maxsplit=1)[0]
    head = head.strip(" *`_\t").casefold()

    if head in DONE_TOKENS:
        return "done"
    if head in DROPPED_TOKENS:
        return "dropped"
    if head in OPEN_TOKENS:
        return "open"
    return "unknown"


def _subheading_to_category(heading: str) -> str:
    if "カテゴリの説明" in heading:
        return ""
    cleaned = re.sub(r"\s*[（(][^）)]*[）)]\s*$", "", heading).strip()
    return cleaned


def _map_columns(header_cells: list[str]) -> dict[str, int]:
    mapping: dict[str, int] = {}
    for idx, col in enumerate(header_cells):
        c = col.strip()
        if c in ("#", "ID") and "item_id" not in mapping:
            mapping["item_id"] = idx
        elif c == "カテゴリ" and "category" not in mapping:
            mapping["category"] = idx
        elif c in ("アクション", "内容") and "action" not in mapping:
            mapping["action"] = idx
        elif c in ("担当", "担当者") and "owner" not in mapping:
            mapping["owner"] = idx
        elif c == "期限" and "due" not in mapping:
            mapping["due"] = idx
        elif c in ("状態", "ステータス", "Status", "status") and "status" not in mapping:
            mapping["status"] = idx
    return mapping


def _extract_checklist_owner_due(action: str) -> tuple[str, str]:
    owner_match = CHECKLIST_OWNER_RE.search(action)
    due_match = CHECKLIST_DUE_RE.search(action)
    owner = owner_match.group(1).strip() if owner_match else ""
    due = due_match.group(1) if due_match else ""
    return owner, due


def parse_action_items(
    pm_id: str, filename: str, text: str
) -> ParsedActionItems:
    """Parse action-item tables and checklists under the 'アクションアイテム' heading."""
    lines = text.splitlines()
    in_ai = False
    in_fence = False
    current_cat = ""
    table_rows: list[ActionRow] = []
    checklist_rows: list[ActionRow] = []
    unmapped_tables = 0

    i = 0
    total = len(lines)
    while i < total:
        line = lines[i]
        if FENCE_RE.match(line):
            in_fence = not in_fence
            i += 1
            continue
        if in_fence:
            i += 1
            continue

        h2_match = H2_RE.match(line)
        if h2_match is not None:
            in_ai = "アクションアイテム" in h2_match.group(1)
            current_cat = ""
            i += 1
            continue

        if not in_ai:
            i += 1
            continue

        h3_match = H3_RE.match(line)
        if h3_match is not None:
            current_cat = _subheading_to_category(h3_match.group(1))
            i += 1
            continue

        if (
            line.lstrip().startswith("|")
            and i + 1 < total
            and TABLE_SEP_RE.match(lines[i + 1])
        ):
            header_cells = split_md_row(line)
            col_map = _map_columns(header_cells)
            i += 2
            if "action" not in col_map:
                unmapped_tables += 1
                while i < total and lines[i].lstrip().startswith("|"):
                    i += 1
                continue

            while i < total and lines[i].lstrip().startswith("|"):
                cells = split_md_row(lines[i])

                def cell(key: str) -> str:
                    idx = col_map.get(key)
                    if idx is None or idx >= len(cells):
                        return ""
                    return cells[idx]

                category = cell("category") or current_cat
                status = cell("status")
                table_rows.append(
                    ActionRow(
                        postmortem_id=pm_id,
                        file=filename,
                        item_id=cell("item_id"),
                        category=category,
                        action=cell("action"),
                        owner=cell("owner"),
                        due=cell("due"),
                        status=status,
                        normalised_status=normalise_status(status),
                    )
                )
                i += 1
            continue

        cl_match = CHECKLIST_ITEM_RE.match(line)
        if cl_match is not None:
            mark = cl_match.group(1)
            body_parts = [cl_match.group(2).strip()]
            i += 1
            while i < total:
                nxt = lines[i]
                if (
                    not nxt.strip()
                    or FENCE_RE.match(nxt)
                    or H2_RE.match(nxt)
                    or H3_RE.match(nxt)
                    or CHECKLIST_ITEM_RE.match(nxt)
                    or nxt.lstrip().startswith("|")
                    or re.match(r"^\s*[-*]\s+", nxt)
                ):
                    break
                body_parts.append(nxt.strip())
                i += 1
            action = " ".join(part for part in body_parts if part)
            owner, due = _extract_checklist_owner_due(action)
            checklist_rows.append(
                ActionRow(
                    postmortem_id=pm_id,
                    file=filename,
                    item_id=str(len(checklist_rows) + 1),
                    category=current_cat,
                    action=action,
                    owner=owner,
                    due=due,
                    status=f"[{mark}]",
                    normalised_status="done" if mark in ("x", "X") else "open",
                )
            )
            continue

        i += 1

    return ParsedActionItems(
        table_rows=table_rows,
        checklist_rows=checklist_rows,
        unmapped_tables=unmapped_tables,
    )


def list_postmortem_files(pm_dir: pathlib.Path) -> list[tuple[str, pathlib.Path]]:
    """Return sorted (pm_id, path) tuples for all PM-YYYY-NNN-*.md files."""
    entries: list[tuple[str, pathlib.Path]] = []
    for entry in sorted(pm_dir.iterdir(), key=lambda p: p.name):
        if not entry.is_file():
            continue
        match = PM_FILENAME_RE.match(entry.name)
        if match is not None:
            entries.append((match.group(1), entry))
    return entries


def _parse_date(raw: str) -> datetime.date | None:
    s = raw.strip()
    if not DATE_RE.match(s):
        return None
    try:
        return datetime.date.fromisoformat(s)
    except ValueError:
        return None


def _parse_leading_date(raw: str) -> datetime.date | None:
    m = LEADING_DATE_RE.match(raw)
    if m is None:
        return None
    try:
        return datetime.date.fromisoformat(m.group(1))
    except ValueError:
        return None


def _row_cites_other_pm(row: ActionRow) -> bool:
    for cited in PM_ID_RE.findall(row.action):
        if cited != row.postmortem_id:
            return True
    own_short = row.postmortem_id.rsplit("-", 1)[1]
    for short_num in PM_SHORT_ID_RE.findall(row.action):
        if short_num != own_short:
            return True
    return False


def _filter_rows(
    rows: list[ActionRow],
    only_open: bool,
    only_overdue: bool,
    today: datetime.date,
    match_re: re.Pattern[str] | None,
    only_carried: bool,
) -> list[ActionRow]:
    result: list[ActionRow] = []
    for row in rows:
        if only_open and row.normalised_status not in ("open", "unknown"):
            continue
        if only_overdue:
            if row.normalised_status not in ("open", "unknown"):
                continue
            due_date = _parse_leading_date(row.due)
            if due_date is None or due_date >= today:
                continue
        if match_re is not None:
            if not (
                match_re.search(row.action)
                or match_re.search(row.owner)
                or match_re.search(row.file)
            ):
                continue
        if only_carried and not _row_cites_other_pm(row):
            continue
        result.append(row)
    return result


def _truncate(text: str, limit: int) -> str:
    if len(text) <= limit:
        return text
    return text[: max(0, limit - 1)] + "…"


def run_ledger(
    root: pathlib.Path,
    only_open: bool,
    only_overdue: bool,
    today: datetime.date,
    match_re: re.Pattern[str] | None,
    only_carried: bool,
    as_json: bool,
) -> int:
    pm_dir = root / "docs" / "postmortems"
    if not pm_dir.is_dir():
        print(f"error: postmortems directory does not exist: {pm_dir}", file=sys.stderr)
        return 2

    files = list_postmortem_files(pm_dir)
    all_rows: list[ActionRow] = []
    unparsed: list[str] = []

    for pm_id, path in files:
        text = path.read_text(encoding="utf-8")
        parsed = parse_action_items(pm_id, path.name, text)
        rows = parsed.all_rows
        if not rows:
            unparsed.append(path.name)
        else:
            all_rows.extend(rows)

    filtered = _filter_rows(
        all_rows,
        only_open=only_open,
        only_overdue=only_overdue,
        today=today,
        match_re=match_re,
        only_carried=only_carried,
    )

    counts = {
        "postmortems": len(files),
        "rows": len(filtered),
        "open": sum(1 for r in filtered if r.normalised_status == "open"),
        "done": sum(1 for r in filtered if r.normalised_status == "done"),
        "dropped": sum(1 for r in filtered if r.normalised_status == "dropped"),
        "unknown": sum(1 for r in filtered if r.normalised_status == "unknown"),
        "unparsed": len(unparsed),
    }

    if as_json:
        payload = {
            "rows": [asdict(r) for r in filtered],
            "unparsed": unparsed,
            "summary": counts,
            "note": "statuses are as recorded in the postmortem files and may lag behind the code",
        }
        print(json.dumps(payload, ensure_ascii=False, indent=2))
        return 0

    if filtered:
        print("PM          | ID   | Cat      | Norm    | Due        | Owner            | Action")
        print("------------+------+----------+---------+------------+------------------+-------")
        for r in filtered:
            print(
                f"{r.postmortem_id:<11} | "
                f"{_truncate(r.item_id or '-', 4):<4} | "
                f"{_truncate(r.category or '-', 8):<8} | "
                f"{r.normalised_status:<7} | "
                f"{_truncate(r.due or '-', 10):<10} | "
                f"{_truncate(r.owner or '-', 16):<16} | "
                f"{_truncate(r.action, 72)}"
            )
    else:
        print("(no matching action items)")

    if unparsed:
        print(f"\nUnparsed ({len(unparsed)}): {', '.join(unparsed)}")

    print(
        "\nNote: statuses are as recorded in the postmortem files and may lag behind the code."
    )
    print(
        "Summary: "
        f"postmortems={counts['postmortems']} "
        f"rows={counts['rows']} "
        f"open={counts['open']} "
        f"done={counts['done']} "
        f"dropped={counts['dropped']} "
        f"unknown={counts['unknown']} "
        f"unparsed={counts['unparsed']}"
    )
    return 0


def _run_git(cwd: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(cwd), *args],
        capture_output=True,
        text=True,
        check=False,
    )


def _load_valid_fp_ids(root: pathlib.Path) -> set[str] | None:
    patterns_path = (
        root / ".claude" / "skills" / "failure-pattern-review" / "references" / "patterns.md"
    )
    if not patterns_path.is_file():
        return None
    ids: set[str] = set()
    for line in patterns_path.read_text(encoding="utf-8").splitlines():
        m = PATTERN_HEADING_RE.match(line)
        if m is not None:
            ids.add(m.group(1))
    return ids


def _parse_readme_index_ids(readme_path: pathlib.Path) -> set[str]:
    ids: set[str] = set()
    for line in readme_path.read_text(encoding="utf-8").splitlines():
        stripped = line.lstrip()
        if not stripped.startswith("|") or TABLE_SEP_RE.match(line):
            continue
        for pm_id in PM_ID_RE.findall(line):
            ids.add(pm_id)
    return ids


def _has_completion_evidence(row: ActionRow, root: pathlib.Path) -> bool:
    text = f"{row.action} {row.status}"
    for adr_id in ADR_WIKILINK_RE.findall(text):
        if (root / "docs" / "ADR" / f"{adr_id}.md").is_file():
            return True
    if PR_REF_RE.search(text) or PULL_URL_RE.search(text):
        return True
    for m in COMMIT_HASH_RE.finditer(text):
        tok = m.group(0)
        if any("a" <= c <= "f" for c in tok) and any(c.isdigit() for c in tok):
            return True
    root_resolved = root.resolve()
    for m in BACKTICK_SPAN_RE.finditer(text):
        cand = m.group(1).strip()
        if (
            "/" not in cand
            or any(ch.isspace() for ch in cand)
            or cand.startswith(("http://", "https://", "/"))
        ):
            continue
        target = (root / cand.rstrip("/")).resolve()
        try:
            target.relative_to(root_resolved)
        except ValueError:
            continue
        if target.exists():
            return True
    return False


def _parse_failure_pattern_section(
    text: str,
) -> tuple[bool, bool, dict[str, str]]:
    """Return (found_fp_section, immediately_after_rca, fields)."""
    lines = text.splitlines()
    h2_headings: list[str] = []
    in_section = False
    found_section = False
    in_fence = False
    fields: dict[str, str] = {}

    for line in lines:
        if FENCE_RE.match(line):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        h2_match = H2_RE.match(line)
        if h2_match is not None:
            heading = h2_match.group(1).strip()
            h2_headings.append(heading)
            if heading == "失敗の型と再発":
                in_section = True
                found_section = True
            elif in_section:
                in_section = False
            continue
        if not in_section:
            continue
        item_match = FAILURE_ITEM_RE.match(line)
        if item_match is not None:
            key = item_match.group(1)
            val = HTML_COMMENT_RE.sub("", item_match.group(2)).strip()
            fields[key] = val

    immediately_after_rca = False
    if found_section and "根本原因分析" in h2_headings:
        rca_idx = h2_headings.index("根本原因分析")
        fp_idx = h2_headings.index("失敗の型と再発")
        immediately_after_rca = fp_idx == rca_idx + 1

    return found_section, immediately_after_rca, fields


def _validate_added_postmortem(
    pm_id: str,
    path: pathlib.Path,
    root: pathlib.Path,
    existing_pm_ids: set[str],
    valid_fp_ids: set[str],
) -> list[str]:
    findings: list[str] = []
    text = path.read_text(encoding="utf-8")
    rel = f"docs/postmortems/{path.name}"

    parsed = parse_action_items(pm_id, path.name, text)
    if parsed.unmapped_tables > 0:
        findings.append(
            f"P1 {rel}: table under 'アクションアイテム' could not be mapped (missing action column)"
        )
    if not parsed.table_rows:
        findings.append(
            f"P1 {rel}: action-item table under 'アクションアイテム' is missing or has no rows"
        )
    else:
        for idx, row in enumerate(parsed.table_rows, 1):
            label = row.item_id or f"row#{idx}"
            if row.owner.strip().casefold() in PLACEHOLDER_OWNERS:
                findings.append(
                    f"P2 {rel} ({label}): owner is empty or placeholder ({row.owner!r})"
                )
            if _parse_date(row.due) is None:
                findings.append(
                    f"P3 {rel} ({label}): due date {row.due!r} is not valid YYYY-MM-DD"
                )
            status = row.status.strip(" *\t")
            if status not in GATE_ALLOWED_STATUSES:
                findings.append(
                    f"P4 {rel} ({label}): status {row.status!r} must be one of TODO, 進行中, 完了, 破棄"
                )
            elif status == "完了" and not _has_completion_evidence(row, root):
                findings.append(
                    f"P5 {rel} ({label}): completed item cites no evidence (existing [[NNNNNN]], PR #N or /pull/N, 7-40 hex commit, or existing backtick repo path)"
                )
            elif status == "破棄":
                drop_m = DROP_REASON_RE.search(row.action)
                drop_reason = drop_m.group(1).strip().rstrip("）)") if drop_m else ""
                if len(drop_reason) < 10:
                    findings.append(
                        f"P12 {rel} ({label}): 破棄 item must state '破棄理由:' followed by at least 10 characters in the action cell"
                    )

    found_fp_sec, after_rca, fp_fields = _parse_failure_pattern_section(text)
    if not found_fp_sec:
        findings.append(f"P6 {rel}: missing section '## 失敗の型と再発'")
    else:
        if not after_rca:
            findings.append(
                f"P6 {rel}: section '## 失敗の型と再発' must immediately follow '## 根本原因分析'"
            )
        for key in FAILURE_PATTERN_KEYS:
            if key not in fp_fields or not fp_fields[key].strip():
                findings.append(
                    f"P6 {rel}: section '## 失敗の型と再発' missing or empty '{key}:'"
                )

        fp_val = fp_fields.get("失敗の型", "").strip()
        if fp_val.startswith("型の外"):
            reason = re.sub(r"^[：:\s()（）・、。-]+", "", fp_val[len("型の外") :]).strip()
            if len(reason) < 10:
                findings.append(
                    f"P7 {rel}: 失敗の型 '型の外' must include a reason of at least 10 characters"
                )
        elif fp_val:
            tokens = [t for t in re.split(r"[,、/\s]+", fp_val) if t]
            if not tokens or any(not FP_ID_RE.match(t) for t in tokens):
                findings.append(
                    f"P7 {rel}: 失敗の型 ({fp_val!r}) must be a list of FP-NN ids or start with '型の外'"
                )
            else:
                unknown = [t for t in tokens if t not in valid_fp_ids]
                if unknown:
                    findings.append(
                        f"P7 {rel}: 失敗の型 cites unknown pattern id(s): {', '.join(unknown)}"
                    )

        for ref_key in ("再発", "持ち越し"):
            ref_val = fp_fields.get(ref_key, "").strip()
            if not ref_val:
                continue
            for cited_pm in PM_ID_RE.findall(ref_val):
                if cited_pm not in existing_pm_ids:
                    findings.append(
                        f"P8 {rel}: {ref_key} cites nonexistent postmortem {cited_pm}"
                    )

    return findings


def _added_postmortems_since(
    repo_root: pathlib.Path, base_rev: str
) -> tuple[list[pathlib.Path], list[pathlib.Path], str, str | None]:
    if not base_rev or base_rev.startswith("-"):
        return [], [], "", f"error: invalid base revision: {base_rev!r}"

    top_res = _run_git(repo_root, "rev-parse", "--show-toplevel")
    if top_res.returncode != 0:
        return [], [], "", f"error: {repo_root} is not inside a git repository: {top_res.stderr.strip()}"

    rev_res = _run_git(
        repo_root, "rev-parse", "--verify", "--quiet", f"{base_rev}^{{commit}}"
    )
    if rev_res.returncode != 0:
        return [], [], "", f"error: unknown base revision: {base_rev}"

    mb_res = _run_git(repo_root, "merge-base", base_rev, "HEAD")
    if mb_res.returncode != 0:
        return [], [], "", f"error: git merge-base failed for {base_rev}: {mb_res.stderr.strip()}"
    merge_base = mb_res.stdout.strip()

    diff_res = _run_git(
        repo_root,
        "diff",
        "--no-renames",
        "--diff-filter=A",
        "--name-only",
        "-z",
        merge_base,
        "--",
        "docs/postmortems",
    )
    if diff_res.returncode != 0:
        return [], [], "", f"error: git diff failed: {diff_res.stderr.strip()}"

    untracked_res = _run_git(
        repo_root,
        "ls-files",
        "--others",
        "--exclude-standard",
        "-z",
        "--",
        "docs/postmortems",
    )
    if untracked_res.returncode != 0:
        return [], [], "", f"error: git ls-files failed: {untracked_res.stderr.strip()}"

    rel_paths: set[str] = set()
    for out in (diff_res.stdout, untracked_res.stdout):
        for token in out.split("\0"):
            if token:
                rel_paths.add(token)

    pm_dir = (repo_root / "docs" / "postmortems").resolve()
    valid_added: list[pathlib.Path] = []
    invalid_added: list[pathlib.Path] = []
    for rel in sorted(rel_paths):
        p = (repo_root / rel).resolve()
        if not p.is_file() or p.suffix.lower() != ".md":
            continue
        if p == pm_dir / "README.md":
            continue
        if p.parent == pm_dir and PM_FILENAME_RE.match(p.name):
            valid_added.append(p)
        else:
            invalid_added.append(p)
    return valid_added, invalid_added, merge_base, None


def run_check(root: pathlib.Path, base_rev: str | None) -> int:
    pm_dir = root / "docs" / "postmortems"
    if not pm_dir.is_dir():
        print(f"error: postmortems directory does not exist: {pm_dir}", file=sys.stderr)
        return 2

    readme_path = pm_dir / "README.md"
    if not readme_path.is_file():
        print(f"error: postmortems README does not exist: {readme_path}", file=sys.stderr)
        return 2

    files = list_postmortem_files(pm_dir)
    existing_pm_ids = {pm_id for pm_id, _ in files}
    readme_ids = _parse_readme_index_ids(readme_path)

    findings: list[str] = []

    # Added-file gate (B)
    if base_rev is not None:
        valid_added, invalid_added, merge_base, git_err = _added_postmortems_since(
            root, base_rev
        )
        if git_err is not None:
            print(git_err, file=sys.stderr)
            return 2

        root_resolved = root.resolve()
        for bad_path in invalid_added:
            rel_bad = bad_path.relative_to(root_resolved).as_posix()
            findings.append(
                f"P11 {rel_bad}: Markdown file under docs/postmortems/ must be named PM-YYYY-NNN-<slug>.md (except README.md)"
            )

        if valid_added:
            valid_fp_ids = _load_valid_fp_ids(root)
            if valid_fp_ids is None:
                print(
                    "error: patterns reference not found at "
                    ".claude/skills/failure-pattern-review/references/patterns.md",
                    file=sys.stderr,
                )
                return 2
            for path in valid_added:
                match = PM_FILENAME_RE.match(path.name)
                assert match is not None
                pm_id = match.group(1)
                findings.extend(
                    _validate_added_postmortem(pm_id, path, root, existing_pm_ids, valid_fp_ids)
                )
        elif not invalid_added:
            print(
                f"No added postmortems relative to merge base {merge_base}; running index consistency check."
            )

    # Index consistency (C) across all files and README.md
    for pm_id, path in files:
        if pm_id not in readme_ids:
            findings.append(
                f"P9 docs/postmortems/{path.name}: {pm_id} has no row in docs/postmortems/README.md"
            )
    for cited_id in sorted(readme_ids - existing_pm_ids):
        findings.append(
            f"P10 docs/postmortems/README.md: {cited_id} has a row in README.md but no file in docs/postmortems/"
        )

    for finding in findings:
        print(finding)

    print(f"Summary: {len(findings)} finding(s)")
    return 1 if findings else 0


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Postmortem action-item ledger and CI gate."
    )
    parser.add_argument(
        "--root",
        default=None,
        help="Repository root directory (defaults to repo containing this script).",
    )
    parser.add_argument("--open", action="store_true", help="Show only open or unknown rows.")
    parser.add_argument(
        "--overdue",
        action="store_true",
        help="Show only open rows with due date before --today.",
    )
    parser.add_argument(
        "--today",
        default=None,
        help="Reference date (YYYY-MM-DD) for --overdue (defaults to today).",
    )
    parser.add_argument(
        "--match",
        default=None,
        help="Regex matching action text, owner, or postmortem filename.",
    )
    parser.add_argument(
        "--carried",
        action="store_true",
        help="Show only rows whose action text cites another postmortem id (PM-YYYY-NNN).",
    )
    parser.add_argument("--json", action="store_true", help="Emit JSON output.")
    parser.add_argument(
        "--check",
        action="store_true",
        help="Run index consistency (and added-postmortem gate when --base is set).",
    )
    parser.add_argument(
        "--base",
        default=None,
        help="Base git revision for gating added postmortems (requires --check).",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    root = (
        pathlib.Path(args.root).resolve()
        if args.root is not None
        else pathlib.Path(__file__).resolve().parents[2]
    )
    if not root.is_dir():
        print(f"error: root directory does not exist: {root}", file=sys.stderr)
        return 2

    if args.base is not None and not args.check:
        print("error: --base requires --check", file=sys.stderr)
        return 2

    if args.check:
        return run_check(root, args.base)

    today = datetime.date.today()
    if args.today is not None:
        parsed_today = _parse_date(args.today)
        if parsed_today is None:
            print(f"error: invalid --today date (expected YYYY-MM-DD): {args.today!r}", file=sys.stderr)
            return 2
        today = parsed_today

    match_re: re.Pattern[str] | None = None
    if args.match is not None:
        try:
            match_re = re.compile(args.match)
        except re.error as exc:
            print(f"error: invalid --match regex {args.match!r}: {exc}", file=sys.stderr)
            return 2

    return run_ledger(
        root=root,
        only_open=args.open,
        only_overdue=args.overdue,
        today=today,
        match_re=match_re,
        only_carried=args.carried,
        as_json=args.json,
    )


if __name__ == "__main__":
    sys.exit(main())
