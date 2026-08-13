---
name: reload
description: Re-reads Alt's authoritative project context — CLAUDE.md, the path-scoped rules in .claude/rules/, and the wiki entry point — and restates which of those rules bear on the work in progress. Invoke manually after a long session, after context compaction, or when the working assumptions have drifted from what the repo actually says (「コンテキスト再読み込み」). Prefer plan-context-loader when the goal is to pull ADRs and canonical contracts out of the vault for a design task, rather than to re-anchor on the standing rules.
allowed-tools: Read, Glob, Bash
disable-model-invocation: true
---

# Reload Project Context

Read the following and treat every line as authoritative for the remainder of the session:

1. `./CLAUDE.md` — the project's critical rules
2. `.claude/rules/*.md` — path-scoped standing constraints. Glob the directory rather than assuming
   which rule files exist
3. `docs/wiki/HOME.md` — the current navigation layer over ADRs, runbooks and plans

Then state, in a few lines, which rules bear on the work currently in progress. Reloading is only
worth the tokens if it changes what you do next.

Skills are discovered from their own frontmatter and do not need to be read here.
