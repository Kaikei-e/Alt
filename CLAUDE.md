# CLAUDE.md - The Alt Project

## Overview

Alt is an AI-augmented RSS knowledge platform. Docker Compose-first orchestration with TDD-first development.

> Service-specific details live in `docs/<service>.md` snapshots. Each service's `CLAUDE.md` focuses on workflow guidelines.

## Quick Reference

```bash
# Start stack
make up

# Stop stack
make down

# Tear down with volumes
make down-volumes

# Rebuild
make build
```

## Service Matrix

| Service | Language | Framework | CLAUDE.md |
|---------|----------|-----------|-----------|
| alt-backend | Go 1.24+ | Echo | `alt-backend/app/CLAUDE.md` |
| alt-frontend | TypeScript | Next.js 15 | `alt-frontend/CLAUDE.md` |
| pre-processor | Go 1.24+ | Custom | `pre-processor/app/CLAUDE.md` |
| search-indexer | Go 1.24+ | Meilisearch | `search-indexer/app/CLAUDE.md` |
| tag-generator | Python 3.13+ | FastAPI | `tag-generator/app/CLAUDE.md` |
| news-creator | Python 3.11+ | FastAPI + Ollama | `news-creator/app/CLAUDE.md` |
| auth-hub | Go 1.24+ | Echo | `auth-hub/CLAUDE.md` |
| rask-log-aggregator | Rust 1.87+ | Axum | `rask-log-aggregator/app/CLAUDE.md` |
| rask-log-forwarder | Rust 1.87+ | Custom | `rask-log-forwarder/app/CLAUDE.md` |
| auth-token-manager | Deno 2.x | Custom | `auth-token-manager/CLAUDE.md` |
| alt-perf | Deno 2.x | Astral | `alt-perf/CLAUDE.md` |
| altctl | Go 1.24+ | Cobra | `altctl/CLAUDE.md` |

## Development Principles

### TDD First

**IMPORTANT**: Always write failing tests BEFORE implementation.

1. **RED**: Write a failing test
2. **GREEN**: Write minimal code to pass
3. **REFACTOR**: Improve quality, keep tests green

### Testing Commands by Language

| Language | Command |
|----------|---------|
| Go | `go test ./...` |
| Python | `uv run pytest` |
| Rust | `cargo test` |
| TypeScript | `pnpm test` |
| Deno | `deno test` |

### Clean Architecture

Most services follow layered architecture:

```
Handler → Usecase → Port → Gateway → Driver
```

Maintain strict boundaries. Update mocks alongside interface changes.

## Orchestration

### Docker Compose Profiles

```bash
# Default stack
make up

# With AI pipeline (GPU required)
docker compose --profile ollama up -d

# With log forwarders
docker compose --profile logging up -d

# Combined
docker compose --profile ollama --profile logging up -d
```

### Health Checks

```bash
curl http://localhost:3000/api/health   # Frontend
curl http://localhost:9000/v1/health    # Backend
curl http://localhost:7700/health       # Meilisearch
```

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Compose-First**: Docker Compose is source of truth (not K8s)
3. **Rate Limiting**: External APIs require 5-second minimum intervals
4. **Secrets in .env**: Never commit credentials
5. **docs/*.md for Details**: Keep CLAUDE.md files concise

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Stack won't start | Run `make down-volumes` then `make up` |
| Tests failing | Check mock interfaces match implementations |
| Rate limit errors | Verify 5-second intervals |
| Import cycles (Go) | Check layer dependencies |

## Appendix: References

### Claude Code
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Using CLAUDE.md Files](https://claude.com/blog/using-claude-md-files)

### Architecture
- [Clean Architecture - Uncle Bob](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Clean Architecture in Go](https://threedots.tech/post/introducing-clean-architecture/)

### TDD
- [Learn Go with Tests](https://quii.gitbook.io/learn-go-with-tests/)
- [Learn TDD in Next.js](https://learntdd.in/next/)
- [Testing ML Systems](https://www.eugeneyan.com/writing/testing-ml/)

### Language-Specific
- [Effective Go](https://go.dev/doc/effective_go)
- [The Rust Performance Book](https://nnethercote.github.io/perf-book/)
- [FastAPI Documentation](https://fastapi.tiangolo.com/)
- [Next.js 15 Documentation](https://nextjs.org/docs)
- [Deno Documentation](https://docs.deno.com/)
