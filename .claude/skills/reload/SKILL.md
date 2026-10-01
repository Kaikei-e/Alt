---
name: reload
description: Re-reads Alt's authoritative project context — CLAUDE.md, the path-scoped rules in .claude/rules/, and the wiki entry point — and restates which of those rules bear on the work in progress. Invoke manually after a long session, after context compaction, or when the working assumptions have drifted from what the repo actually says (「コンテキスト再読み込み」). Prefer plan-context-loader when the goal is to pull ADRs and canonical contracts out of the vault for a design task, rather than to re-anchor on the standing rules.
allowed-tools: Read, Glob
disable-model-invocation: true
---

# Reload Project Context

Read the following and treat every line as authoritative for the remainder of the session:

1. `./CLAUDE.md` — the project's critical rules
2. `.claude/rules/*.md` — path-scoped standing constraints. Glob the directory rather than assuming
   which rule files exist
3. `docs/wiki/HOME.md` — the current navigation layer over ADRs, runbooks and plans

Skills are discovered from their own frontmatter and do not need to be read here.

## Output Checklist

When summarizing the reloaded context, output a short checklist structured as follows:

- [ ] **Standing Rules (`CLAUDE.md`)**: Restate which of the numbered Critical Rules in `CLAUDE.md` apply to the ongoing task (do not omit Rule 6: re-reading canonical contracts via `/plan-context-loader` before repair PRs touching Knowledge Trail, Knowledge Home, or append-first projections matters most for reload).
- [ ] **Path-Scoped Rules (`.claude/rules/`)**: Match active files against glob patterns in `.claude/rules/*.md` and list the binding constraints that apply to those files (e.g. `di-wiring.md`, `event-stream-consumer.md`, `knowledge-home.md`, `security-boundaries.md`).
- [ ] **Entry Points (`docs/wiki/HOME.md`)**: Identify the crystallized navigation entry points (service capsule in `docs/services/<name>.md`, canonical plans in `docs/plan/`, runbooks, or postmortems) relevant to the current work.
