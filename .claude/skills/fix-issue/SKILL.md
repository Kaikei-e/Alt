---
name: fix-issue
description: Work a GitHub issue end to end in Alt — read the issue, locate the affected code, drive the fix test-first, and commit it. Use when the user points at an issue by number or URL and wants it fixed, or says "fix issue 42" / "この issue 直して".
allowed-tools: Bash, Read, Grep, Glob, Edit, Write
argument-hint: <issue-number>
---

# Fix a GitHub Issue

Target issue: `$ARGUMENTS`

## Workflow

1. **Read the issue**: `gh issue view $ARGUMENTS`. Restate in one line what behavior is wrong and what
   correct behavior would look like — the reproduction steps in the issue are the specification.
2. **Locate the affected code.** Search for the behavior, not the issue's wording; issues describe
   symptoms in user language while the defect lives in a specific layer.
3. **Fix it test-first** using the `tdd-workflow` skill: a failing test that reproduces the reported
   behavior comes before the fix, so there is evidence the defect existed and is gone.
4. **Run the touched service's gates** (`tdd-workflow` Phase 5) before declaring it done.
5. **Commit:**
   ```bash
   git commit -m "fix(<service>): <what changed>

   Fixes #$ARGUMENTS"
   ```
   Do not add a `Co-Authored-By` trailer — Alt's history does not carry them.

`git push` and `gh pr create` are not part of this workflow. They happen only on the user's explicit
instruction, run by the user.
