# metrics/CLAUDE.md

## Overview

System health analyzer with ClickHouse. **Python 3.13+**, generates Japanese Markdown reports.

> Details: `docs/services/metrics.md`

## Commands

```bash
# Install
uv sync

# Test (TDD first)
uv run pytest -v

# Lint
uv run ruff check src/ tests/
uv run ruff format src/ tests/

# Run analysis
uv run python -m alt_metrics analyze --hours 24 --verbose

# Connection test
uv run python -m alt_metrics validate
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

```bash
# RED -> GREEN -> REFACTOR
uv run pytest -v --cov=alt_metrics
```

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Pydantic Models**: Use for type-safe data models
3. **Structlog**: Use for structured logging
4. **Custom Exceptions**: `CollectorError`, `ConfigurationError`, etc.
