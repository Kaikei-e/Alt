# tag-generator/CLAUDE.md

## Overview

Tag generation with ML models (KeyBERT, sentence-transformers). **Python 3.14+**, **FastAPI**.

> Details: `docs/services/tag-generator.md`

## Commands

```bash
# Test (TDD first)
uv run pytest

# Coverage
uv run pytest --cov=tag_generator

# Type check
uv run pyrefly check .

# Lint
uv run ruff check && uv run ruff format

# Run (the real container entrypoint per Dockerfile.tag-generator; main.py is
# a separate standalone consumer/batch script that the running container does
# NOT execute — see [[000319]] for the incident this distinction caused)
uv run python auth_service.py
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Unit**: Test tag extraction with known inputs
- **Integration**: Full pipeline against a mocked/sanitized alt-data-hub client, not a real database — direct DB access was removed (ADR-000397)
- **ML Quality**: Bias detection, robustness testing

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Type Safety**: Use Python type hints throughout
3. **Memory Management**: Manual GC after batch processing
4. **Batch Processing**: Use optimal batch sizes (75 default)
