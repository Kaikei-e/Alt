# tag-generator/CLAUDE.md

## Overview

Tag generation service for the Alt RSS reader platform. Built with **Python 3.13+** and **FastAPI**. Uses ML models (KeyBERT, sentence-transformers) for automated tag extraction.

> For batch-loop behavior and structlog config, see `docs/services/tag-generator.md`.

## Quick Start

```bash
# Run tests
uv run pytest

# Run with coverage
uv run pytest --cov=tag_generator

# Type checking
uv run mypy src/

# Linting
uv run ruff check && uv run ruff format

# Start service
uv run python main.py
```

## Architecture

Pipeline-based architecture:

```
Main Service → Tag Extractor → Article Fetcher → Tag Inserter
```

- **Tag Extractor**: ML model for extracting tags from content
- **Article Fetcher**: Retrieves articles from database
- **Tag Inserter**: Stores generated tags back to database

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

1. **RED**: Write a failing test
2. **GREEN**: Write minimal code to pass
3. **REFACTOR**: Improve quality, keep tests green

Testing layers:
- **Unit**: Test tag extraction logic with known inputs
- **Integration**: Full pipeline with database
- **ML Quality**: Bias detection, robustness testing

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Type Safety**: Use Python type hints throughout
3. **Memory Management**: Manual GC after batch processing
4. **Structured Logging**: Use `structlog` for all operations
5. **Batch Processing**: Use optimal batch sizes (75 default)

## Key Config

```bash
BATCH_LIMIT=75
PROCESSING_INTERVAL=60
MEMORY_CLEANUP_INTERVAL=25
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Memory leaks | Check GC after batch processing |
| Database connection | Verify PostgreSQL credentials |
| ML model loading | Ensure model files accessible |
| Slow processing | Tune BATCH_LIMIT |

## Appendix: References

### Official Documentation
- [FastAPI Documentation](https://fastapi.tiangolo.com/)
- [FastAPI Testing](https://fastapi.tiangolo.com/tutorial/testing/)
- [KeyBERT](https://maartengr.github.io/KeyBERT/)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Testing ML Systems - Eugene Yan](https://www.eugeneyan.com/writing/testing-ml/)

### Tools
- [pytest](https://docs.pytest.org/)
- [structlog](https://www.structlog.org/)
- [ruff](https://docs.astral.sh/ruff/)
