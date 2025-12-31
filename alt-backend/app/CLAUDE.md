# alt-backend/CLAUDE.md

## Overview

Core backend service for the Alt RSS reader platform. Built with **Go 1.24+**, **Echo framework**, and **Clean Architecture** principles.

> For implementation details (routes, integrations, schemas), see `docs/alt-backend.md`.

## Quick Start

```bash
# Run tests
go test ./...

# Run with coverage
go test -race -cover ./...

# Generate mocks
make generate-mocks

# Start service
go run main.go
```

## Architecture

Five-layer Clean Architecture with strict dependency rules:

```
REST Handler → Usecase → Port → Gateway → Driver
```

- **REST**: HTTP handlers, routing (depends on Usecase)
- **Usecase**: Business logic orchestration (depends on Port)
- **Port**: Interface definitions/contracts (depends on Gateway)
- **Gateway**: Anti-corruption layer, domain translation (depends on Driver)
- **Driver**: External integrations (DB, APIs) - no dependencies

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

1. **RED**: Write a failing test that defines desired behavior
2. **GREEN**: Write minimal code to make the test pass
3. **REFACTOR**: Improve code quality while keeping tests green

Testing by layer:
- **Usecase**: Mock repository/gateway interfaces, test business logic in isolation
- **Gateway**: Mock drivers (DB client, HTTP client), test external interactions
- **Handler**: Use `net/http/httptest`, mock usecase layer

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Rate Limiting**: MUST enforce 5-second minimum intervals for external API calls
3. **Structured Logging**: Use `log/slog` with context for all operations
4. **Error Wrapping**: Always use `fmt.Errorf("context: %w", err)`
5. **Context Propagation**: Pass `context.Context` through entire call chain
6. **No Hardcoded Values**: Use environment variables via `config.NewConfig()`

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Import cycles | Check layer dependencies are correct |
| Rate limit errors | Verify 5-second minimum intervals |
| Mock interface mismatch | Regenerate mocks with `make generate-mocks` |
| Test failures | Check testify assertions and mock expectations |

## Key Commands

```bash
# Linting
gofmt -w . && goimports -w .

# Health check
curl http://localhost:9000/v1/health

# API endpoints
# POST /v1/feeds, GET /v1/feeds, GET /v1/articles, GET /v1/search
```

## Appendix: References

### Official Documentation
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Testing](https://go.dev/doc/testing)
- [Echo Framework](https://echo.labstack.com/)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Clean Architecture in Go - Three Dots Labs](https://threedots.tech/post/introducing-clean-architecture/)

### TDD & Testing
- [Learn Go with Tests](https://quii.gitbook.io/learn-go-with-tests/)
- [GoMock](https://github.com/uber-go/mock)
- [Testify](https://github.com/stretchr/testify)

### Architecture
- [Clean Architecture - Uncle Bob](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Go Project Layout](https://github.com/golang-standards/project-layout)
