# search-indexer/CLAUDE.md

## Overview

Search indexing service with Meilisearch. **Go 1.24+**, Clean Architecture.

> Details: `docs/services/search-indexer.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Integration tests (requires Meilisearch)
go test -tags=integration ./...

# Run
go run main.go
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Usecase**: Mock repository and search engine interfaces
- **Gateway**: Mock Meilisearch driver
- **Integration**: Real Meilisearch with test indices

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Batch Size**: Process articles in batches of 200
3. **Primary Key**: Use `article_id` for upsert operations
4. **Retry Logic**: Implement exponential backoff
5. **Cleanup**: ALWAYS delete test indices after integration tests
