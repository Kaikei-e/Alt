# alt-frontend-sv/CLAUDE.md

## Overview

Next-gen frontend. **SvelteKit 2.x**, **Svelte 5 Runes**, **TailwindCSS v4**. Serves under `/sv` base path.

> Details: `docs/services/alt-frontend-sv.md`

## Commands

```bash
# Test (TDD first)
bun test                  # Unit/Component
bun run test:e2e          # E2E (requires stack)

# Dev
bun dev

# Lint & Build
bun run lint && bun run format
bun run check && bun run build
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Component**: Use `@testing-library/svelte`, mock API calls
- **Store**: Test Runes reactivity in isolation
- **E2E**: Page Object Model in `tests/e2e/`

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Runes Only**: Use `$state`, `$derived`, `$effect`, `$props` - NEVER legacy syntax
3. **Base Path**: App runs under `/sv` - always use relative paths or `$app/paths`
4. **TailwindCSS v4**: CSS-first config in `src/app.css` - no `tailwind.config.js`
5. **Biome**: Run `bun run lint && bun run format` before commits
