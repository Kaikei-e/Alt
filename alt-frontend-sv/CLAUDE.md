# alt-frontend-sv/CLAUDE.md

## Overview

Primary frontend. **SvelteKit 2.x**, **Svelte 5 Runes**, **TailwindCSS v4**, **TypeScript 7 (tsgo)**. Serves at root path (`/`).

> Details: `docs/services/alt-frontend-sv.md`

## Commands

```bash
# Test (TDD first)
bun test                  # Unit/Component
bun run test:e2e          # E2E (requires stack)

# Dev
bun dev

# Lint & Type Check & Build
bun run lint && bun run format
bun run check && bun run build   # tsgo-based type check (primary, 10x fast)
bun run check:tsc                # tsc-based type check (TS5.9 fallback)
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Component**: Use `@testing-library/svelte`, mock API calls
- **Store**: Test Runes reactivity in isolation
- **E2E**: Page Object Model in `tests/e2e/`

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Runes Only**: Use `$state`, `$derived`, `$effect`, `$props` - NEVER legacy syntax
3. **Root Path**: App runs at `/` - use relative paths or `$app/paths`
4. **TailwindCSS v4**: CSS-first config in `src/app.css` - no `tailwind.config.js`
5. **Biome**: Run `bun run lint && bun run format` before commits
