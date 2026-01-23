# tag-generator/CLAUDE.md

## Overview

Tag generation with ML models (KeyBERT, sentence-transformers). **Python 3.13+**, **FastAPI**.

> Details: `docs/services/tag-generator.md`

## Commands

```bash
# Test (TDD first)
uv run pytest

# Coverage
uv run pytest --cov=tag_generator

# Type check
uv run mypy src/

# Lint
uv run ruff check && uv run ruff format

# Run
uv run python main.py
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Unit**: Test tag extraction with known inputs
- **Integration**: Full pipeline with database
- **ML Quality**: Bias detection, robustness testing

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Type Safety**: Use Python type hints throughout
3. **Memory Management**: Manual GC after batch processing
4. **Batch Processing**: Use optimal batch sizes (75 default)
