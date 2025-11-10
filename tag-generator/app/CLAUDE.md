# tag-generator/CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- Use 'think' for basic analysis, 'ultrathink' for complex architectural decisions -->

## About tag-generator

> For the latest batch-loop behavior, cursor safeguards, and structlog config, read `docs/tag-generator.md`.

This is the **tag generation service** of the Alt RSS reader platform, built with **Python 3.13+** and **FastAPI**. The service automatically generates relevant tags for RSS feed articles using machine learning models.

**Critical Guidelines:**
- **TDD First:** Always write failing tests BEFORE implementation
- **ML Quality:** Ensure high-quality tag generation with proper validation
- **Performance:** Optimize for batch processing and memory efficiency
- **Structured Logging:** Use structured logging for all operations
- **Type Safety:** Use Python type hints throughout the codebase

## Architecture Overview

### Service Architecture
```
Main Service → Tag Extractor → Article Fetcher → Tag Inserter
```

**Component Responsibilities:**
- **Main Service**: Orchestrates the tag generation pipeline
- **Tag Extractor**: ML model for extracting tags from content
- **Article Fetcher**: Retrieves articles from database
- **Tag Inserter**: Stores generated tags back to database

### Directory Structure
```
/tag-generator/app/
├─ main.py                    # Application entry point
├─ tag_extractor/             # ML model components
│  ├─ extract.py             # Tag extraction logic
│  └─ models/                # ML model files
├─ article_fetcher/           # Database access
│  └─ fetch.py               # Article retrieval
├─ tag_inserter/              # Database operations
│  └─ upsert_tags.py         # Tag storage
├─ tag_generator/             # Main service logic
│  └─ logging_config.py      # Logging configuration
├─ auth_service.py            # Authentication service
├─ tests/                     # Test suite
│  ├─ unit/                  # Unit tests
│  ├─ integration/           # Integration tests
│  └─ fixtures/              # Test fixtures
├─ pyproject.toml            # Project configuration
└─ CLAUDE.md                 # This file
```

## TDD and Testing Strategy

### Test-Driven Development (TDD)
All development follows the Red-Green-Refactor cycle:

1. **Red**: Write a failing test
2. **Green**: Write minimal code to pass
3. **Refactor**: Improve code quality

### Testing Layers

#### Unit Tests
```python
# Test tag extraction
def test_extract_tags_success():
    extractor = TagExtractor()
    content = "This is about machine learning and artificial intelligence"

    tags = extractor.extract_tags(content)

    assert len(tags) > 0
    assert any("machine learning" in tag.lower() for tag in tags)
```

#### Integration Tests
```python
# Test full pipeline
def test_tag_generation_pipeline():
    service = TagGeneratorService()
    article_id = "test-article-123"

    result = service.generate_tags_for_article(article_id)

    assert result.success is True
    assert len(result.tags) > 0
    assert result.article_id == article_id
```

#### ML Model Testing
```python
# Test model quality
def test_model_bias_detection():
    extractor = TagExtractor()

    # Test with diverse content
    content1 = "Technology news about AI"
    content2 = "Sports news about football"

    tags1 = extractor.extract_tags(content1)
    tags2 = extractor.extract_tags(content2)

    # Ensure different content produces different tags
    assert set(tags1) != set(tags2)
```

## Machine Learning Components

### Tag Extraction
- **Model**: Pre-trained NLP model for tag extraction
- **Preprocessing**: Text cleaning and normalization
- **Postprocessing**: Tag validation and filtering
- **Confidence Scoring**: Assign confidence scores to generated tags

### Quality Assurance
- **Bias Detection**: Test for demographic bias in tag generation
- **Robustness Testing**: Validate against adversarial inputs
- **Performance Benchmarking**: Monitor inference speed and memory usage

## Configuration

### Environment Variables
```bash
# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=alt_db
DB_TAG_GENERATOR_USER=tag_generator
DB_TAG_GENERATOR_PASSWORD=password

# Processing
PROCESSING_INTERVAL=60
BATCH_LIMIT=75
PROGRESS_LOG_INTERVAL=10
MEMORY_CLEANUP_INTERVAL=25

# Health Monitoring
HEALTH_CHECK_INTERVAL=10
MAX_CONSECUTIVE_EMPTY_CYCLES=20
```

## Performance Optimization

### Memory Management
- **Garbage Collection**: Manual GC after processing batches
- **Memory Monitoring**: Track memory usage during processing
- **Batch Processing**: Process articles in optimal batch sizes

### Database Optimization
- **Connection Pooling**: Reuse database connections
- **Query Optimization**: Use efficient queries for article retrieval
- **Batch Operations**: Use batch inserts for tag storage

## Development Workflow

### Running Tests
```bash
# Unit tests
uv run pytest tests/unit/

# Integration tests
uv run pytest tests/integration/

# All tests
uv run pytest

# Coverage
uv run pytest --cov=tag_generator

# Type checking
uv run mypy src/
```

### Code Quality
```bash
# Linting
uv run ruff check

# Formatting
uv run ruff format

# Security check
uv run bandit -r src/
```

### Running the Service
```bash
# Development
uv run python main.py

# Production
uv run python main.py --config production
```

## API Endpoints

### Tag Generation
- **POST /api/v1/generate-tags**: Generate tags for specific article
- **POST /api/v1/batch-generate**: Generate tags for multiple articles

### Health and Monitoring
- **GET /health**: Service health status
- **GET /metrics**: Performance metrics

## Troubleshooting

### Common Issues
- **Memory Leaks**: Check garbage collection and memory cleanup
- **Database Connection**: Verify PostgreSQL connectivity
- **ML Model Loading**: Ensure model files are accessible
- **Performance Issues**: Monitor batch sizes and processing intervals

### Debug Commands
```bash
# Check service health
curl http://localhost:8000/health

# Test tag generation
curl -X POST http://localhost:8000/api/v1/generate-tags \
  -H "Content-Type: application/json" \
  -d '{"article_id": "test-123", "content": "Test content"}'

# View logs
docker logs tag-generator -f
```

## References

- [FastAPI Testing](https://fastapi.tiangolo.com/tutorial/testing/)
- [Pytest Documentation](https://docs.pytest.org/)
- [Testing ML Systems](https://www.eugeneyan.com/writing/testing-ml/)
- [Python Type Hints](https://docs.python.org/3/library/typing.html)
