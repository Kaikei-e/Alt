# alt-backend/CLAUDE.md

## Overview

Core backend for Alt RSS platform. **Go 1.24+**, **Echo**, Clean Architecture.

> Details: `docs/services/alt-backend.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Coverage
go test -race -cover ./...

# Mocks
make generate-mocks

# Run
go run main.go
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Usecase**: Mock ports, test business logic
- **Gateway**: Mock drivers, test external calls
- **Handler**: Use `httptest`, mock usecases

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Rate Limiting**: YOU MUST enforce 5-second minimum for external APIs
3. **Error Wrapping**: Use `fmt.Errorf("context: %w", err)`
4. **Context**: Pass `context.Context` through entire call chain
5. **Logging**: Use `log/slog` with structured context
