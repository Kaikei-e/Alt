# search-indexer/CLAUDE.md

## Overview

Search indexing service for the Alt RSS reader platform. Built with **Go 1.24+** and **Meilisearch**. Indexes processed articles for fast full-text search.

> For indexing loop details and Meilisearch config, see `docs/services/search-indexer.md`.

## Quick Start

```bash
# Run tests
go test ./...

# Run with integration tests (requires Meilisearch)
go test -tags=integration ./...

# Start service
go run main.go

# Check health
curl http://localhost:9300/health
```

## Architecture

Five-layer Clean Architecture:

```
REST Handler → Usecase → Port → Gateway → Driver
```

- **REST**: HTTP endpoints for search operations
- **Usecase**: Business logic for indexing and searching
- **Port**: Interface definitions (search engine contracts)
- **Gateway**: Anti-corruption layer for Meilisearch
- **Driver**: Direct Meilisearch client implementation

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing layers:
- **Usecase**: Mock repository and search engine interfaces
- **Gateway**: Mock Meilisearch driver
- **Integration**: Real Meilisearch instance with test indices

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Batch Size**: Process articles in batches of 200
3. **Primary Key**: Use `article_id` for upsert operations
4. **Retry Logic**: Implement exponential backoff for transient failures
5. **Structured Logging**: Use `log/slog` for all operations
6. **Cleanup Test Indices**: Always delete test indices after integration tests

## Indexing Strategy

```go
// Searchable: title, content, summary, tags
// Filterable: created_at, feed_id, tags, status
// Batch: 200 docs, max chunk 10K
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Index creation failures | Check Meilisearch connectivity |
| Search timeouts | Verify Meilisearch performance |
| Memory issues | Reduce INDEX_BATCH_SIZE |
| Stale test indices | Ensure cleanup in defer statements |

## Appendix: References

### Official Documentation
- [Meilisearch Documentation](https://www.meilisearch.com/docs)
- [Meilisearch Go Client](https://github.com/meilisearch/meilisearch-go)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Clean Architecture in Go](https://threedots.tech/post/introducing-clean-architecture/)
