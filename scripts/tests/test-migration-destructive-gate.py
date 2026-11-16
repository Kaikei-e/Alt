#!/usr/bin/env python3
"""Tests for scripts/agent-harness/migration_destructive_gate.py.

PM-2026-057 / PM-2026-060: destructive migrations that run automatically on
deploy without a recorded recovery/backfill plan silently wipe or hide data.
This gate checks only git-tracked *.sql files added under any `migrations`
directory relative to the merge base with `--base`.

Run:
    python3 scripts/tests/test-migration-destructive-gate.py
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import os
import pathlib
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
GATE_PATH = ROOT / "scripts" / "agent-harness" / "migration_destructive_gate.py"

spec = importlib.util.spec_from_file_location("migration_destructive_gate", GATE_PATH)
assert spec is not None and spec.loader is not None, "gate script missing"
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)

PASS = 0
FAIL = 0


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        suffix = f" ({detail})" if detail else ""
        print(f"  FAIL  {name}{suffix}")
        FAIL += 1


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
    res = subprocess.run(
        ["git", "-C", str(repo), *args],
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
    (repo / "README.md").write_text("init\n", encoding="utf-8")
    run_git(repo, "add", "README.md")
    run_git(repo, "commit", "-m", "initial")
    return repo


def write_file_and_add(repo: pathlib.Path, rel: str, content: str) -> pathlib.Path:
    target = repo / rel
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")
    run_git(repo, "add", rel)
    return target


def invoke_gate(repo: pathlib.Path, base: str) -> tuple[int, str, str]:
    stdout_buf = io.StringIO()
    stderr_buf = io.StringIO()
    with (
        contextlib.redirect_stdout(stdout_buf),
        contextlib.redirect_stderr(stderr_buf),
    ):
        rc = gate.main(["--base", base, "--root", str(repo)])
    return rc, stdout_buf.getvalue(), stderr_buf.getvalue()


print("1. each destructive statement kind in an added migration fails")
DESTRUCTIVE_CASES = [
    ("DROP TABLE", "migrations-atlas/migrations/20260101_drop.sql", "DROP TABLE old_feeds;\n"),
    (
        "DROP COLUMN",
        "knowledge-sovereign/migrations/00099_drop_col.sql",
        "ALTER TABLE items DROP COLUMN legacy_flag;\n",
    ),
    (
        "TRUNCATE",
        "clickhouse/migrations/099_truncate.sql",
        "TRUNCATE TABLE logs;\n",
    ),
    (
        "DELETE FROM",
        "recap-migration-atlas/migrations/20260101_del.sql",
        "DELETE FROM recap_jobs;\n",
    ),
    (
        "ALTER COLUMN TYPE USING NULL",
        "rag-migration-atlas/migrations/20260802_widen.sql",
        "ALTER TABLE rag_chunks\n  ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);\n",
    ),
    (
        "UPDATE SET NULL",
        "acolyte-migration-atlas/migrations/20260101_nullify.sql",
        "UPDATE reports SET summary = NULL;\n",
    ),
    (
        "case-insensitive lowercase",
        "pre-processor-migration-atlas/migrations/20260101_lower.sql",
        "drop table if exists scratch_jobs;\n",
    ),
]
for label, rel, sql in DESTRUCTIVE_CASES:
    with tempfile.TemporaryDirectory() as td:
        repo = init_repo(pathlib.Path(td))
        base_rev = run_git(repo, "rev-parse", "HEAD")
        write_file_and_add(repo, rel, sql)
        run_git(repo, "commit", "-m", f"add {label}")
        rc, out, err = invoke_gate(repo, base_rev)
        check(
            f"{label}: exits 1 and prints M1 finding",
            rc == 1 and f"M1 {rel}:" in out and "without a destructive-ok comment" in out,
            f"rc={rc}, out={out!r}, err={err!r}",
        )

print("2. added migration with valid destructive-ok comment passes")
for label, rel, sql in DESTRUCTIVE_CASES:
    with tempfile.TemporaryDirectory() as td:
        repo = init_repo(pathlib.Path(td))
        base_rev = run_git(repo, "rev-parse", "HEAD")
        documented = (
            "-- destructive-ok: table is a disposable projection rebuilt from the event log\n"
            + sql
        )
        write_file_and_add(repo, rel, documented)
        run_git(repo, "commit", "-m", f"add {label} with reason")
        rc, out, err = invoke_gate(repo, base_rev)
        check(
            f"{label}: passes with destructive-ok comment",
            rc == 0,
            f"rc={rc}, out={out!r}, err={err!r}",
        )

print("3. short destructive-ok comment (under 20 characters) fails")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_short.sql",
        "-- destructive-ok: safe enough\nDROP TABLE scratch;\n",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("short comment (<20 chars) exits 1", rc == 1 and "M1 " in out, f"rc={rc}, out={out!r}")

with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_spaces.sql",
        "-- destructive-ok:                     \nTRUNCATE scratch;\n",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("whitespace-only comment fails", rc == 1, f"rc={rc}, out={out!r}")

print("4. destructive text inside comments and string literals is ignored")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_comments.sql",
        (
            "-- DROP TABLE foo; TRUNCATE bar; DELETE FROM baz;\n"
            "/*\n"
            "   ALTER TABLE x DROP COLUMN y;\n"
            "   UPDATE x SET z = NULL;\n"
            "   ALTER TABLE rag_chunks ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);\n"
            "*/\n"
            "INSERT INTO audit_log (note) VALUES ('DROP TABLE users');\n"
            "CREATE TABLE ok_table (id INT PRIMARY KEY);\n"
        ),
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("comments and string literals ignored -> exit 0", rc == 0, f"rc={rc}, out={out!r}")

print("5. DELETE and UPDATE with WHERE pass")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_where.sql",
        (
            "DELETE FROM jobs WHERE status = 'obsolete';\n"
            "UPDATE jobs SET error = NULL WHERE status = 'retried';\n"
        ),
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("DELETE/UPDATE with WHERE exits 0", rc == 0, f"rc={rc}, out={out!r}")

print("6. destructive statement in an existing file at base is not reported")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    write_file_and_add(
        repo,
        "rag-migration-atlas/migrations/20260802_old.sql",
        "ALTER TABLE rag_chunks ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);\n",
    )
    run_git(repo, "commit", "-m", "historical destructive migration")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    # Even if touched on the branch, it already existed at the merge base.
    write_file_and_add(
        repo,
        "rag-migration-atlas/migrations/20260802_old.sql",
        "ALTER TABLE rag_chunks ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);\n-- note\n",
    )
    run_git(repo, "commit", "-m", "comment existing migration")
    rc, out, err = invoke_gate(repo, base_rev)
    check("existing migration at base is not examined", rc == 0, f"rc={rc}, out={out!r}")

print("7. non-migration .sql file and untracked file are ignored")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(repo, "scripts/sql/cleanup.sql", "DROP TABLE temp_cache;\n")
    untracked = repo / "migrations-atlas" / "migrations" / "999999_untracked.sql"
    untracked.parent.mkdir(parents=True, exist_ok=True)
    untracked.write_text("DROP TABLE untracked;\n", encoding="utf-8")
    rc, out, err = invoke_gate(repo, base_rev)
    check("non-migration and untracked sql ignored -> exit 0", rc == 0, f"rc={rc}, out={out!r}")

print("8. accurate line number reporting after block comment")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_lines.sql",
        "/* line 1\n   line 2 */\n\nDROP TABLE target;\n",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check(
        "finding reports line 4",
        rc == 1 and "M1 migrations-atlas/migrations/20260101_lines.sql:4:" in out,
        f"rc={rc}, out={out!r}",
    )

print("9. unknown base revision and non-git root exit 2")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    res_rev = subprocess.run(
        [sys.executable, str(GATE_PATH), "--base", "does-not-exist", "--root", str(repo)],
        capture_output=True,
        text=True,
    )
    check("unknown --base exits 2", res_rev.returncode == 2, f"rc={res_rev.returncode}")

with tempfile.TemporaryDirectory() as td:
    res_nogit = subprocess.run(
        [sys.executable, str(GATE_PATH), "--base", "HEAD", "--root", td],
        capture_output=True,
        text=True,
    )
    check("non-git --root exits 2", res_nogit.returncode == 2, f"rc={res_nogit.returncode}")

print("10. M1: WHERE before keyword or at deeper paren depth does not hide unconditional delete/update")
M1_PROBES = [
    (
        "DO block IF EXISTS WHERE then DELETE",
        "DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_tables WHERE tablename='x') THEN DELETE FROM x; END IF; END $$;",
    ),
    (
        "DO block IF EXISTS WHERE then UPDATE SET NULL",
        "DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_tables WHERE tablename='x') THEN UPDATE rag_chunks SET embedding = NULL; END IF; END $$;",
    ),
    (
        "CTE WHERE before DELETE",
        "WITH k AS (SELECT id FROM t WHERE f) DELETE FROM t2;",
    ),
    (
        "DELETE USING subquery WHERE",
        "DELETE FROM t USING (SELECT 1 FROM u WHERE u.a=1) s;",
    ),
]
for label, sql in M1_PROBES:
    with tempfile.TemporaryDirectory() as td:
        repo = init_repo(pathlib.Path(td))
        base_rev = run_git(repo, "rev-parse", "HEAD")
        write_file_and_add(repo, "migrations-atlas/migrations/20260101_m1.sql", sql + "\n")
        rc, out, err = invoke_gate(repo, base_rev)
        check(
            f"M1 {label} -> exit 1",
            rc == 1 and "M1 migrations-atlas/migrations/20260101_m1.sql:" in out,
            f"rc={rc}, out={out!r}",
        )

with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_m1_ok.sql",
        "DELETE FROM t USING (SELECT 1 FROM u WHERE u.a=1) s WHERE t.id = s.id;\n",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("M1 top-level WHERE after USING subquery passes", rc == 0, f"rc={rc}, out={out!r}")

print("11. M2: E'' backslash escapes and dollar-quoted strings cannot mask following SQL")
M2_PROBES = [
    ("E'' escaped apostrophe", r"INSERT INTO notes VALUES (E'it\'s fine');  DROP TABLE users;"),
    ("dollar-quoted apostrophe", "COMMENT ON TABLE t IS $$it's a table$$;  DROP TABLE users;"),
    ("tagged dollar-quoted apostrophe", "COMMENT ON TABLE t IS $msg$it's a table$msg$;  DROP TABLE users;"),
]
for label, sql in M2_PROBES:
    with tempfile.TemporaryDirectory() as td:
        repo = init_repo(pathlib.Path(td))
        base_rev = run_git(repo, "rev-parse", "HEAD")
        write_file_and_add(repo, "migrations-atlas/migrations/20260101_m2.sql", sql + "\n")
        rc, out, err = invoke_gate(repo, base_rev)
        check(
            f"M2 {label} -> catches DROP TABLE",
            rc == 1 and "DROP TABLE" in out,
            f"rc={rc}, out={out!r}",
        )

print("12. M3: optional COLUMN, parenthesized USING (NULL::...), new destructive kinds and safe neighbours")
M3_POSITIVES = [
    ("ALTER TABLE ALTER TYPE USING NULL (no COLUMN)", "ALTER TABLE t ALTER c TYPE text USING NULL;\n"),
    ("USING (NULL::vector(1024))", "ALTER TABLE rag_chunks ALTER COLUMN embedding TYPE vector(1024) USING (NULL::vector(1024));\n"),
    ("ALTER TABLE DROP col (no COLUMN)", "ALTER TABLE t DROP c;\n"),
    ("DROP SCHEMA", "DROP SCHEMA legacy_schema;\n"),
    ("DROP DATABASE", "DROP DATABASE legacy_db;\n"),
    ("DROP TYPE CASCADE", "DROP TYPE old_enum CASCADE;\n"),
    ("DROP VIEW CASCADE", "DROP VIEW old_view CASCADE;\n"),
    ("ClickHouse DROP PARTITION", "ALTER TABLE logs DROP PARTITION '202601';\n"),
    ("ClickHouse ALTER TABLE DELETE without WHERE", "ALTER TABLE logs DELETE;\n"),
    ("ClickHouse ALTER TABLE DELETE with WHERE", "ALTER TABLE logs DELETE WHERE level = 'DEBUG';\n"),
    ("ClickHouse MODIFY TTL", "ALTER TABLE logs MODIFY TTL timestamp + INTERVAL 1 DAY DELETE;\n"),
]
for label, sql in M3_POSITIVES:
    with tempfile.TemporaryDirectory() as td:
        repo = init_repo(pathlib.Path(td))
        base_rev = run_git(repo, "rev-parse", "HEAD")
        write_file_and_add(repo, "migrations-atlas/migrations/20260101_m3_pos.sql", sql)
        rc, out, err = invoke_gate(repo, base_rev)
        check(f"M3 positive: {label} -> exit 1", rc == 1 and "M1 " in out, f"rc={rc}, out={out!r}")

M3_NEGATIVES = [
    ("DROP CONSTRAINT", "ALTER TABLE t DROP CONSTRAINT fk_user;\n"),
    ("DROP DEFAULT", "ALTER TABLE t ALTER COLUMN c DROP DEFAULT;\n"),
    ("DROP NOT NULL", "ALTER TABLE t ALTER c DROP NOT NULL;\n"),
    ("DROP EXPRESSION", "ALTER TABLE t ALTER COLUMN c DROP EXPRESSION IF EXISTS;\n"),
    ("DROP IDENTITY", "ALTER TABLE t ALTER c DROP IDENTITY IF EXISTS;\n"),
    ("DROP INDEX", "DROP INDEX IF EXISTS idx_old;\n"),
    ("DROP VIEW", "DROP VIEW IF EXISTS v_old;\n"),
    ("ADD CONSTRAINT ON DELETE CASCADE", "ALTER TABLE t DROP CONSTRAINT fk, ADD CONSTRAINT fk FOREIGN KEY (x) REFERENCES p(x) ON DELETE CASCADE;\n"),
]
for label, sql in M3_NEGATIVES:
    with tempfile.TemporaryDirectory() as td:
        repo = init_repo(pathlib.Path(td))
        base_rev = run_git(repo, "rev-parse", "HEAD")
        write_file_and_add(repo, "migrations-atlas/migrations/20260101_m3_neg.sql", sql)
        rc, out, err = invoke_gate(repo, base_rev)
        check(f"M3 negative: {label} -> exit 0", rc == 0, f"rc={rc}, out={out!r}")

print("13. M4: destructive-ok is per-statement and ignored in block comments/strings")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_m4_two.sql",
        (
            "-- destructive-ok: table is a disposable projection rebuilt from event log\n"
            "DROP TABLE temp_one;\n"
            "DROP TABLE temp_two;\n"
        ),
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check(
        "M4 one ok-comment does not excuse second destructive statement",
        rc == 1 and ":3:" in out and ":2:" not in out,
        f"rc={rc}, out={out!r}",
    )

with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_m4_block.sql",
        (
            "/*\n"
            "-- destructive-ok: block comment must not excuse the statement below\n"
            "*/\n"
            "DROP TABLE temp_one;\n"
        ),
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("M4 ok-comment inside /* */ does not excuse", rc == 1, f"rc={rc}, out={out!r}")

with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_m4_str.sql",
        (
            "SELECT '\n"
            "-- destructive-ok: string literal must not excuse anything at all\n"
            "';\n"
            "DROP TABLE temp_one;\n"
        ),
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("M4 ok-comment inside string literal does not excuse", rc == 1, f"rc={rc}, out={out!r}")

with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_m4_do_inner.sql",
        (
            "DO $$\n"
            "BEGIN\n"
            "  -- destructive-ok: projection table is rebuilt from event log on demand\n"
            "  DELETE FROM proj;\n"
            "END\n"
            "$$;\n"
        ),
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("M4 inner ok-comment in DO block excuses that statement", rc == 0, f"rc={rc}, out={out!r}")

with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    write_file_and_add(
        repo,
        "migrations-atlas/migrations/20260101_m4_do_outer.sql",
        (
            "-- destructive-ok: projection table is rebuilt from event log on demand\n"
            "DO $$\n"
            "BEGIN\n"
            "  DELETE FROM proj;\n"
            "END\n"
            "$$;\n"
        ),
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("M4 ok-comment directly above DO statement excuses it", rc == 0, f"rc={rc}, out={out!r}")

print("14. M5: explicit message with merge base when 0 migration files are added")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_rev = run_git(repo, "rev-parse", "HEAD")
    rc, out, err = invoke_gate(repo, base_rev)
    check(
        "M5 output states no migration files added and includes merge base",
        rc == 0 and base_rev in out and "no migration files added" in out.lower(),
        f"rc={rc}, out={out!r}",
    )

print("15. smoke detector across all existing tracked migration files in repo")
ls_out = subprocess.run(
    ["git", "-C", str(ROOT), "ls-files", "-z"],
    capture_output=True,
    check=True,
).stdout
mig_files = [
    p for p in ls_out.decode("utf-8").split("\0")
    if p and gate.is_migration_sql(p)
]
flagged_existing: list[str] = []
raised_err: Exception | None = None
for rel in mig_files:
    try:
        text = (ROOT / rel).read_text(encoding="utf-8", errors="replace")
        found = gate.find_destructive_statements(text)
        if found:
            flagged_existing.append(f"{rel} ({len(found)}: {', '.join(k for _, k in found)})")
    except Exception as exc:  # pragma: no cover
        raised_err = exc
        break
check(
    f"scanned {len(mig_files)} existing migration files without exception",
    len(mig_files) > 0 and raised_err is None,
    f"err={raised_err!r}",
)
print(f"  INFO  existing migration files with findings: {len(flagged_existing)}/{len(mig_files)}")
for item in flagged_existing:
    print(f"    - {item}")

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
