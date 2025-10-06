# search-indexer/CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- Use 'think' for basic analysis, 'ultrathink' for complex architectural decisions -->

## About search-indexer

This is the **search indexing service** of the Alt RSS reader platform, built with **Go 1.24+** and **Meilisearch**. The service indexes processed articles for fast full-text search capabilities.

**Critical Guidelines:**
- **TDD First:** Always write failing tests BEFORE implementation
- **Performance:** Optimize for high-throughput indexing
- **Reliability:** Implement proper error handling and retry logic
- **Structured Logging:** Use `log/slog` with context for all operations
- **Clean Architecture:** Maintain clear separation of concerns

## Architecture Overview

### Clean Architecture Layers
```
REST Handler → Usecase → Port → Gateway (ACL) → Driver
```

**Layer Responsibilities:**
- **REST**: HTTP endpoints for search operations
- **Usecase**: Business logic for indexing and searching
- **Port**: Interface definitions for external dependencies
- **Gateway**: Anti-corruption layer for Meilisearch integration
- **Driver**: Direct Meilisearch client implementation

### Directory Structure
```
/search-indexer/app/
├─ main.go                    # Application entry point
├─ server/                    # HTTP server implementation
│  ├─ indexer_server.go      # Main server logic
│  └─ server.go              # Server configuration
├─ usecase/                   # Business logic
│  ├─ index_articles.go      # Article indexing use case
│  └─ search_articles.go     # Article search use case
├─ port/                      # Interface definitions
│  └─ search_engine.go       # Search engine interface
├─ gateway/                   # Anti-corruption layer
│  └─ search_engine_gateway.go # Meilisearch gateway
├─ driver/                    # External integrations
│  ├─ database_driver.go     # PostgreSQL driver
│  └─ meilisearch_driver.go  # Meilisearch driver
├─ indexer/                   # Indexing logic
│  └─ indexer.go             # Batch indexing implementation
├─ tokenize/                  # Text tokenization
│  └─ tokenizer.go           # Text processing utilities
├─ logger/                    # Logging utilities
│  └─ logger.go              # Structured logging setup
└─ CLAUDE.md                 # This file
```

## TDD and Testing Strategy

### Test-Driven Development (TDD)
All development follows the Red-Green-Refactor cycle:

1. **Red**: Write a failing test
2. **Green**: Write minimal code to pass
3. **Refactor**: Improve code quality

### Testing Meilisearch Integration

#### Unit Tests
```go
func TestIndexArticles_Success(t *testing.T) {
    // Setup mocks
    mockRepo := &MockArticleRepository{}
    mockSearchEngine := &MockSearchEngine{}

    // Configure mocks
    mockRepo.On("GetArticlesWithTags", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
        Return([]*Article{testArticle}, &time.Time{}, "last-id", nil)
    mockSearchEngine.On("AddDocuments", mock.Anything).Return(nil)

    // Test
    usecase := NewIndexArticlesUsecase(mockRepo, mockSearchEngine, tokenizer)
    result, err := usecase.Execute(ctx, nil, "", 10)

    // Assert
    require.NoError(t, err)
    assert.Equal(t, 1, result.IndexedCount)
    mockRepo.AssertExpectations(t)
    mockSearchEngine.AssertExpectations(t)
}
```

#### Integration Tests
```go
func TestIndexAndSearch_Integration(t *testing.T) {
    // Setup real Meilisearch instance
    client := meilisearch.NewClient("http://localhost:7700", "")
    indexName := fmt.Sprintf("test_index_%d", time.Now().UnixNano())

    // Create test index
    _, err := client.CreateIndex(&meilisearch.IndexConfig{
        Uid: indexName,
        PrimaryKey: "id",
    })
    require.NoError(t, err)
    defer client.DeleteIndex(indexName)

    // Test indexing
    docs := []map[string]interface{}{
        {"id": "1", "title": "Test Article", "content": "Test content"},
    }
    task, err := client.Index(indexName).AddDocuments(docs)
    require.NoError(t, err)

    // Wait for indexing to complete
    _, err = client.WaitForTask(task.TaskUID, 5*time.Second, 50*time.Millisecond)
    require.NoError(t, err)

    // Test search
    var result meilisearch.SearchResponse
    err = client.Index(indexName).Search("Test", &meilisearch.SearchRequest{}, &result)
    require.NoError(t, err)
    assert.Len(t, result.Hits, 1)
}
```

## Indexing Strategy

### Batch Processing
- **Batch Size**: Process articles in batches of 200 for optimal performance
- **Chunking**: Split large datasets into smaller chunks (10,000 documents max)
- **Primary Key**: Use stable `article_id` as primary key for upsert operations
- **Retry Logic**: Implement exponential backoff for transient failures

### Index Configuration
```go
// Apply index settings on startup
func applyIndexSettings(client *meilisearch.Client, indexName string) {
    // Searchable attributes
    searchableAttrs := []string{"title", "content", "summary", "tags"}
    client.Index(indexName).UpdateSearchableAttributes(&searchableAttrs)

    // Filterable attributes
    filterableAttrs := []string{"created_at", "feed_id", "tags", "status"}
    client.Index(indexName).UpdateFilterableAttributes(&filterableAttrs)

    // Ranking rules
    rankingRules := []string{"words", "typo", "proximity", "attribute", "sort", "exactness"}
    client.Index(indexName).UpdateRankingRules(&rankingRules)
}
```

## API Endpoints

### Search API
- **GET /v1/search**: Search articles with query parameters
  - `q`: Search query string
  - `limit`: Maximum number of results (default: 20)
  - `offset`: Pagination offset
  - `filters`: Filter by tags, feed_id, etc.

### Health Check
- **GET /health**: Service health status

## Configuration

### Environment Variables
```bash
# Meilisearch
MEILISEARCH_HOST=http://localhost:7700
MEILISEARCH_API_KEY=your-api-key

# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=alt_db
DB_SEARCH_INDEXER_USER=search_indexer
DB_SEARCH_INDEXER_PASSWORD=password

# Service
HTTP_ADDR=:9300
INDEX_INTERVAL=1m
INDEX_BATCH_SIZE=200
```

## Performance Optimization

### Indexing Performance
- **Batch Processing**: Use optimal batch sizes for throughput
- **Connection Pooling**: Reuse database connections
- **Memory Management**: Monitor memory usage during large batches
- **Async Processing**: Use goroutines for concurrent operations

### Search Performance
- **Index Optimization**: Configure proper ranking rules
- **Query Optimization**: Use filters to narrow search scope
- **Caching**: Implement result caching for frequent queries

## Development Workflow

### Running Tests
```bash
# Unit tests
go test ./...

# Integration tests (requires Meilisearch)
go test -tags=integration ./...

# Coverage
go test -cover ./...

# Benchmarks
go test -bench=. ./...
```

### Running the Service
```bash
# Development
go run main.go

# With Docker
docker build -t search-indexer .
docker run -p 9300:9300 search-indexer
```

## Troubleshooting

### Common Issues
- **Index Creation Failures**: Check Meilisearch connectivity and permissions
- **Search Timeouts**: Verify Meilisearch performance and configuration
- **Memory Issues**: Monitor batch sizes and memory usage
- **Database Connection**: Check PostgreSQL connectivity and credentials

### Debug Commands
```bash
# Check Meilisearch health
curl http://localhost:7700/health

# Check service health
curl http://localhost:9300/health

# Test search
curl "http://localhost:9300/v1/search?q=test&limit=10"
```

## References

- [Meilisearch Go Client](https://github.com/meilisearch/meilisearch-go)
- [Meilisearch Documentation](https://www.meilisearch.com/docs)
- [Testing Asynchronous Operations](https://www.meilisearch.com/docs/learn/advanced/asynchronous_operations)

