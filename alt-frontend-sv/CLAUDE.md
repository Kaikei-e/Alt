# alt-frontend-sv/CLAUDE.md

## Overview

Next-gen frontend for the Alt platform. Built with **SvelteKit 2.x**, **Svelte 5 Runes**, and **TailwindCSS v4**. Serves under `/sv` base path.

> For implementation details (routes, components, API), see `docs/services/alt-frontend-sv.md`.

## Quick Start

```bash
# Run tests
bun test

# Run E2E tests (requires stack)
bun run test:e2e

# Type check
bun run check

# Start dev server
bun dev

# Build
bun run build
```

## Architecture

```
Browser → [alt-frontend-sv :4173] → [auth-hub :8888] → [alt-backend :9101]
              /sv base path            JWT exchange      Connect-RPC
```

**Flow:**
1. User requests page with session cookie
2. `hooks.server.ts` validates session via Kratos
3. API calls exchange cookie for JWT via auth-hub
4. Connect-RPC/REST calls include `X-Alt-Backend-Token`

## Directory Structure

```
alt-frontend-sv/
├── CLAUDE.md
├── README.md
├── package.json
├── svelte.config.js          # Base path: /sv
├── playwright.config.ts      # E2E configuration
└── src/
    ├── app.css               # TailwindCSS v4 config
    ├── app.html
    ├── hooks.server.ts       # Auth middleware
    ├── lib/
    │   ├── api.ts            # API client + token exchange
    │   ├── components/       # UI components (bits-ui)
    │   └── stores/           # Runes-based stores
    └── routes/
        ├── +page.svelte      # Landing
        ├── login/            # Auth pages
        ├── sv/
        │   ├── home/         # Desktop dashboard
        │   ├── mobile/       # Mobile reader
        │   └── dashboard/    # Admin
        └── api/              # Server endpoints
```

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

| Test Type | Command | Framework |
|-----------|---------|-----------|
| Unit/Component | `bun test` | Vitest + Testing Library |
| E2E | `bun run test:e2e` | Playwright |
| Browser | `bun run test:client` | Vitest Browser |

Testing patterns:
- **Component tests**: Use `@testing-library/svelte`, mock API calls
- **Store tests**: Test Runes reactivity in isolation
- **E2E tests**: Page Object Model in `tests/e2e/`

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Runes Only**: Use `$state`, `$derived`, `$effect`, `$props` - never legacy syntax
3. **Base Path**: App runs under `/sv` - always use relative paths or `$app/paths`
4. **TailwindCSS v4**: CSS-first config in `src/app.css` - no `tailwind.config.js`
5. **Biome**: Run `bun run lint` and `bun run format` before commits
6. **SSR/CSR Split**: Use `+page.server.ts` for SSR data, `api.ts` for client fetches

## Configuration

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `BACKEND_BASE_URL` | http://alt-backend:9000 | REST API URL |
| `BACKEND_CONNECT_URL` | http://alt-backend:9101 | Connect-RPC URL |
| `AUTH_HUB_INTERNAL_URL` | http://auth-hub:8888 | Token exchange |
| `KRATOS_INTERNAL_URL` | http://kratos:4433 | Session validation |
| `KRATOS_PUBLIC_URL` | http://localhost:4433 | Browser redirects |

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| 404 on routes | Ensure `/sv` base path in links |
| Legacy syntax errors | Convert `export let` to `$props()` |
| Store not reactive | Use `$state()` not plain variables |
| E2E tests fail | Start stack with `make up` first |
| TailwindCSS not working | Check `src/app.css` imports |
| Auth redirects loop | Verify Kratos URLs in env |

## Key Commands

```bash
# Lint and format
bun run lint && bun run format

# Type check
bun run check

# Health check (when running)
curl http://localhost:4173/sv/health

# Run specific test file
bun test src/lib/api.test.ts

# E2E with UI
bun run test:e2e:ui
```

## Appendix: References

- [SvelteKit Documentation](https://kit.svelte.dev/docs)
- [Svelte 5 Runes](https://svelte.dev/docs/svelte/$state)
- [TailwindCSS v4](https://tailwindcss.com/docs)
- [Playwright](https://playwright.dev/)
- [Vitest](https://vitest.dev/)
