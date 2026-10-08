# alt-frontend-sv

SvelteKit frontend for the Alt platform, serving desktop and mobile experiences at the root `/` path.

Reference architecture, state management, and configuration are documented in [docs/services/alt-frontend-sv.md](../docs/services/alt-frontend-sv.md).

## Tech Stack

| Component | Technology |
|-----------|------------|
| Framework | SvelteKit 3.0 + Svelte 5.57 |
| Runtime | Bun 1.x |
| Styling | TailwindCSS v4 + bits-ui |
| API | Connect-RPC + REST |
| Auth | Ory Kratos via auth-hub |
| Testing | Vitest (unit) + Playwright (E2E) |
| Lint/Format | Biome |

## Quick Start

```bash
# Install dependencies
bun install

# Start development server
bun dev

# Build for production
bun run build

# Type check
bun run check

# Run tests
bun run test          # Unit tests (vitest — do not use bare `bun test`)
bun run test:e2e      # E2E tests (requires stack running)

# Lint and format
bun run lint
bun run format
```

## Routes Structure

```
src/routes/
├── +page.svelte           # Landing page
├── (app)/                 # Authenticated application routes
│   ├── home/              # Knowledge home
│   ├── feeds/             # Feeds view
│   ├── articles/          # Articles view
│   ├── knowledge/         # Knowledge trail
│   ├── dashboard/         # Dashboard
│   └── settings/          # User settings
├── login/                 # Authentication
├── register/              # Registration
├── auth/                  # Auth callbacks
├── eval-dashboard/        # Evaluation dashboard
└── api/                   # SvelteKit API endpoints
```

## Development Notes

- **Base Path**: App runs at `/`.
- **Testing**: Use `bun run test` (vitest), never bare `bun test`.
- **Runes Only**: Do not use legacy `export let` or `$:` syntax.
- **TailwindCSS v4**: CSS-first configuration in `src/app.css`.
- **TypeScript**: Pinned to TypeScript 6; do not upgrade to TS 7 (see `CLAUDE.md`).

## Related Documentation

- [Workflow Guidelines](./CLAUDE.md)
- [Architecture Details](../docs/services/alt-frontend-sv.md)
- [Project CLAUDE.md](../CLAUDE.md)
