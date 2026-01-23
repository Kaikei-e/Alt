# alt-frontend/CLAUDE.md

## Overview

Frontend for Alt RSS reader. **Next.js 15**, **React 19**, **TypeScript 5.9**, **Chakra UI**.

> Details: `docs/services/alt-frontend.md`

## Commands

```bash
# Test (TDD first)
pnpm test                 # Unit (Vitest)
pnpm test:e2e            # E2E (Playwright)

# Dev
pnpm dev

# Lint & Build
pnpm lint && pnpm fmt
pnpm type-check && pnpm build
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Unit (Vitest)**: Component tests with `vi.mock()` for APIs
- **Integration (Vitest)**: API route tests with `node-mocks-http`
- **E2E (Playwright)**: User flows with Page Object Model

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Type Safety**: NEVER use `any`, use strict TypeScript
3. **Accessibility**: Follow WCAG 2.1 AA guidelines
4. **Auto-Archive**: Articles are archived on display (not on user click)
5. **POM Pattern**: E2E tests MUST use Page Objects in `tests/pages/`
