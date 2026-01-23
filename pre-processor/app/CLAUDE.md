# pre-processor/CLAUDE.md

## Overview

Feed processing and article summarization service. **Go 1.24+**, three-layer architecture.

> Details: `docs/services/pre-processor.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Integration tests
go test -tags=integration ./...

# Coverage
go test -cover ./...

# Run
go run main.go
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Service**: Mock repository, test business logic
- **Handler**: Use `httptest`, mock service layer
- **Integration**: Real database, mock external APIs

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Rate Limiting**: YOU MUST enforce 5-second minimum for external APIs
3. **Circuit Breakers**: Use `go-circuitbreaker` for external calls
4. **Context**: Pass `context.Context` with timeouts
5. **Error Wrapping**: Use `fmt.Errorf("context: %w", err)`
