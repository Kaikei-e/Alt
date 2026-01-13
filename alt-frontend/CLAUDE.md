# alt-frontend/CLAUDE.md

## Overview

Frontend service for the Alt RSS reader platform. Built with **Next.js 15**, **React 19**, **TypeScript 5.9**, and **Chakra UI**.

> For implementation details (routing tree, testing matrix, components), see `docs/services/alt-frontend.md`.

## Quick Start

```bash
# Install dependencies
pnpm install

# Development server
pnpm dev

# Run tests
pnpm test                 # Unit tests (Vitest)
pnpm test:e2e            # E2E tests (Playwright)

# Lint and format
pnpm lint && pnpm fmt

# Type check and build
pnpm type-check && pnpm build
```

## Architecture

Next.js 15 App Router with React Server Components:

```
app/
├─ layout.tsx       # Root layout with providers
├─ home/            # Main dashboard
├─ desktop/         # Desktop-specific pages
├─ api/             # API routes
└─ (auth)/          # Auth-protected routes
```

Theme System: Vaporwave | Liquid-Beige | Alt-Paper

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

1. **RED**: Write a failing test
2. **GREEN**: Write minimal code to pass
3. **REFACTOR**: Improve quality, keep tests green

Testing layers:
- **Unit (Vitest)**: Component tests with `vi.mock()` for APIs
- **Integration (Vitest)**: API route tests with `node-mocks-http`
- **E2E (Playwright)**: User flows with Page Object Model pattern

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Type Safety**: Avoid `any`, use strict TypeScript
3. **Accessibility**: Follow WCAG 2.1 AA guidelines
4. **Performance**: Optimize for Core Web Vitals (LCP, FID, CLS)
5. **Auto-Archive**: Articles are archived on display (not on user click)
6. **POM Pattern**: E2E tests MUST use Page Objects in `tests/pages/`

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Hydration mismatch | Check Server vs Client component boundaries |
| Test selector errors | Verify `data-testid` in component implementation |
| Build failures | Run `pnpm type-check` before build |
| API errors | Check backend connectivity and auth-hub integration |

## Key Patterns

- **Server Components**: Data fetching, static content
- **Client Components**: Interactivity, state management (`"use client"`)
- **userEvent**: Use for realistic user interactions in tests
- **waitFor**: Use for async operations in tests

## Appendix: References

### Official Documentation
- [Next.js 15 Documentation](https://nextjs.org/docs)
- [React 19 Documentation](https://react.dev)
- [Chakra UI](https://chakra-ui.com)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [React & Next.js Modern Best Practices](https://strapi.io/blog/react-and-nextjs-in-2025-modern-best-practices)

### TDD & Testing
- [Learn TDD in Next.js](https://learntdd.in/next/)
- [Vitest Documentation](https://vitest.dev)
- [Playwright Documentation](https://playwright.dev)
- [Testing Library](https://testing-library.com/docs/react-testing-library/intro/)

### Performance
- [Core Web Vitals](https://web.dev/vitals/)
- [Next.js Performance](https://nextjs.org/docs/app/building-your-application/optimizing)
