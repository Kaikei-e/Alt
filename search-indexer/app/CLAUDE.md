# CLAUDE.md - Search Indexer Service

## About This Service

The search-indexer is a Go microservice that indexes processed articles into Meilisearch. It follows a Clean Architecture variant to keep business logic isolated from infrastructure.

- **Language**: Go 1.24+
- **Search Engine**: Meilisearch
- **Database**: PostgreSQL (as a read-only source for indexing)
- **Development**: TDD-first with `gomock` and table-driven tests.

## TDD and Testing Strategy

### TDD Guidelines
-   **Red-Green-Refactor**: This cycle is mandatory for all new features.
-   **Primary Targets**: The `usecase` and `gateway` packages are the main focus of unit tests.
-   **Mock Dependencies**: Mock the database repository and Meilisearch engine ports using `gomock`.

### Testing Meilisearch Integration

Testing Meilisearch requires handling its asynchronous nature.

1.  **Unit Tests**: Mock the Meilisearch client interface. Verify that your gateway correctly maps domain objects to search documents and calls the client's methods with the expected data.

2.  **Integration Tests**: These tests should run against a real, containerized Meilisearch instance.
    -   **Wait for Tasks**: After adding documents, you **must** use the returned `taskUID` to poll Meilisearch and wait for the indexing task to succeed before making assertions.
    -   **Isolate Tests**: Each test should create a new, uniquely named index and delete it during teardown to ensure test isolation.

**Example Integration Test:**
```go
func TestIndexAndSearch(t *testing.T) {
    // 1. Setup: Create a new Meilisearch client and a unique index for the test
    client := meilisearch.NewClient(...)
    indexName := fmt.Sprintf("test_index_%d", time.Now().UnixNano())
    _, err := client.CreateIndex(&meilisearch.IndexConfig{Uid: indexName, PrimaryKey: "id"})
    require.NoError(t, err)
    defer client.DeleteIndex(indexName)

    // 2. Act: Add documents to the index
    docs := []map[string]interface{}{
        {"id": 1, "title": "Test Article"},
    }
    task, err := client.Index(indexName).AddDocuments(docs)
    require.NoError(t, err)

    // 3. Wait: Poll for the task to complete
    _, err = client.WaitForTask(task.TaskUID, time.Second*5, time.Millisecond*50)
    require.NoError(t, err)

    // 4. Assert: Search for the document
    var result meilisearch.SearchResponse
    err = client.Index(indexName).Search("Test", &meilisearch.SearchRequest{}, &result)
    require.NoError(t, err)
    assert.Len(t, result.Hits, 1)
}
```

## Indexing Strategy

### Batching and Idempotency
-   **Batching**: Process articles in batches (e.g., 200 at a time) to limit memory usage and network overhead.
-   **Chunking**: For very large datasets, split them into smaller chunks (e.g., 10,000 documents) before sending them to Meilisearch.
-   **Primary Key**: Ensure every document has a unique and stable primary key (e.g., `article_id`). This is required by Meilisearch for `upsert` operations.
-   **Retry Logic**: Implement exponential backoff with jitter for transient failures from Meilisearch (e.g., 429, 5xx errors).

### Managing Index Settings

Index settings (e.g., searchable attributes, filterable attributes, ranking rules) should be managed as code.

-   **Configuration File**: Define your index settings in a configuration file (e.g., `search_settings.json`).
-   **Apply on Startup**: On application startup, read the configuration file and apply the settings to your Meilisearch index using the `Update...` methods (e.g., `UpdateSearchableAttributes`).

```go
// Example: Applying settings on startup
func applyIndexSettings(client *meilisearch.Client, indexName string) {
    // Read settings from a config file
    settings := ... 

    client.Index(indexName).UpdateSearchableAttributes(&settings.SearchableAttributes)
    client.Index(indexName).UpdateFilterableAttributes(&settings.FilterableAttributes)
    // ... and so on
}
```

## Security

-   **Least Privilege**: The database user for the indexer should have read-only permissions.
-   **Sanitization**: Do not log sensitive query payloads or PII.
-   **Network**: Enforce HTTPS for all external communications.

## References

-   [Meilisearch Go Client Documentation](https://github.com/meilisearch/meilisearch-go)
-   [Testing Asynchronous Operations in Meilisearch](https://www.meilisearch.com/docs/learn/advanced/asynchronous_operations)
-   [Meilisearch Index Settings API](https://www.meilisearch.com/docs/reference/api/settings)

