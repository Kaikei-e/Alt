---
title: Extending Acolyte
date: 2026-04-11
tags:
  - acolyte
  - extending
  - development
---

# Extending Acolyte

This document provides step-by-step checklists for common extension tasks, operational recipes, and testing patterns.

## Adding a New Pipeline Node

When you need to add a new step to the report generation pipeline:

1. **Create the node file** in `acolyte/usecase/graph/nodes/`:
   ```python
   # my_node.py
   from acolyte.usecase.graph.state import ReportGenerationState

   class MyNode:
       def __init__(self, llm: LLMProviderPort):
           self._llm = llm

       async def __call__(self, state: ReportGenerationState) -> dict:
           # Read from state
           input_data = state.get("some_field", [])
           
           # Process
           result = await self._process(input_data)
           
           # Return updates to state
           return {"my_output_field": result}
   ```

2. **Update state definition** in `acolyte/usecase/graph/state.py`:
   ```python
   class ReportGenerationState(TypedDict, total=False):
       # ... existing fields
       my_output_field: list[dict]  # Add your new field
   ```

3. **Add the node to the graph** in `acolyte/usecase/graph/report_graph.py`:
   ```python
   from acolyte.usecase.graph.nodes.my_node import MyNode

   def build_report_graph(...):
       graph.add_node("my_node", MyNode(llm))
       # Wire edges
       graph.add_edge("previous_node", "my_node")
       graph.add_edge("my_node", "next_node")
   ```

4. **Write tests first** (TDD) in `tests/unit/test_my_node.py`

5. **Add conditional routing** (if needed):
   ```python
   def should_continue_my_node(state: ReportGenerationState) -> str:
       if condition:
           return "more"
       return "done"

   graph.add_conditional_edges(
       "my_node",
       should_continue_my_node,
       {"more": "my_node", "done": "next_node"},
   )
   ```

## Modifying LLM Prompts

When you need to change how a node generates content:

1. **Locate the prompt** in the relevant node file (e.g., `planner_node.py`)

2. **Update the prompt template**:
   - Keep the `reasoning` field first in JSON schemas (ADR-632 pattern)
   - Use clear instructions to prevent meta-commentary
   - Add examples if the model struggles with format

3. **Adjust `num_predict`** if token requirements change:
   ```python
   response = await self._llm.generate(
       prompt=prompt,
       num_predict=6000,  # Increase for longer outputs
       temperature=0.1,
   )
   ```

4. **Update structured output schema** if response shape changes:
   ```python
   schema = {
       "type": "object",
       "properties": {
           "reasoning": {"type": "string"},  # Always first
           "sections": {"type": "array", ...},
       },
       "required": ["reasoning", "sections"],
   }
   ```

5. **Test with actual LLM outputs** - mocks don't catch prompt issues

## Adding a New Report Type

When you want to support a different kind of report:

1. **Add the type constant** in domain or config:
   ```python
   REPORT_TYPES = ["weekly_briefing", "trend_analysis", "deep_dive", "my_new_type"]
   ```

2. **Update PlannerNode** for type-specific outline generation:
   ```python
   def _get_prompt_for_type(self, report_type: str, brief: dict) -> str:
       if report_type == "my_new_type":
           return MY_NEW_TYPE_PROMPT.format(...)
       # ... existing types
   ```

3. **Configure WriterNode** prompts for the new type

4. **Update frontend** `/acolyte/new` form to include the new type

5. **Add tests** for the new type's outline and content generation

## Adding a New Evidence Source

When you want to retrieve evidence from a new source:

1. **Implement the `EvidenceProviderPort` protocol**:
   ```python
   # port/evidence_provider.py
   class EvidenceProviderPort(Protocol):
       async def search(self, query: str, limit: int = 20) -> list[Evidence]:
           ...
   ```

2. **Create the gateway** in `acolyte/gateway/`:
   ```python
   # my_source_gw.py
   class MySourceGateway:
       async def search(self, query: str, limit: int = 20) -> list[Evidence]:
           # Implement retrieval logic
   ```

3. **Wire via dependency injection** in `main.py`:
   ```python
   evidence_provider = MySourceGateway(...)
   graph = build_report_graph(llm, evidence_provider, ...)
   ```

4. **Add CDC tests** if the source is an external service

## Operational Recipes

### Starting a Report Run

```bash
# Via grpcurl
grpcurl -plaintext -d '{"report_id": "<uuid>"}' \
  localhost:8090 alt.acolyte.v1.AcolyteService/StartReportRun
```

### Checking Run Status

```bash
grpcurl -plaintext -d '{"run_id": "<uuid>"}' \
  localhost:8090 alt.acolyte.v1.AcolyteService/GetRunStatus
```

### Resuming a Failed Run

```bash
# Via resume script (uses LangGraph checkpoint)
docker exec -it acolyte-orchestrator \
  python scripts/resume_run.py --run-id <run_id>
```

### Viewing Report Content

```bash
grpcurl -plaintext -d '{"report_id": "<uuid>"}' \
  localhost:8090 alt.acolyte.v1.AcolyteService/GetReport
```

### Listing Recent Reports

```bash
grpcurl -plaintext -d '{"limit": 10}' \
  localhost:8090 alt.acolyte.v1.AcolyteService/ListReports
```

### Health Check

```bash
grpcurl -plaintext \
  localhost:8090 alt.acolyte.v1.AcolyteService/HealthCheck
```

### Viewing Logs

```bash
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator --tail=100 -f
```

### Database Queries

```bash
# Check active runs
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
  "SELECT run_id, report_id, run_status, started_at FROM report_runs WHERE run_status IN ('pending', 'running') ORDER BY started_at DESC LIMIT 10;"

# Check version history
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
  "SELECT version_no, change_reason, created_at FROM report_versions WHERE report_id = '<uuid>' ORDER BY version_no DESC;"
```

## Testing Patterns

Acolyte follows Alt's TDD-first discipline: **RED → GREEN → REFACTOR**.

### Pipeline Node Tests

Test pattern: mock LLM responses, assert state changes.

```python
# tests/unit/test_planner_node.py
import pytest
from unittest.mock import AsyncMock

from acolyte.usecase.graph.nodes.planner_node import PlannerNode

@pytest.mark.asyncio
async def test_planner_generates_outline():
    # Arrange
    mock_llm = AsyncMock()
    mock_llm.generate.return_value = '{"reasoning": "...", "sections": [...]}'
    
    node = PlannerNode(mock_llm)
    state = {"brief": {"topic": "AI trends"}}
    
    # Act
    result = await node(state)
    
    # Assert
    assert "outline" in result
    assert len(result["outline"]) > 0
```

### E2E Tests

Test pattern: full service boot, Connect-RPC round-trip.

```python
# tests/e2e/test_service.py
import pytest
from httpx import AsyncClient

@pytest.mark.asyncio
async def test_health_check(app_client: AsyncClient):
    response = await app_client.post(
        "/alt.acolyte.v1.AcolyteService/HealthCheck",
        json={},
        headers={"Content-Type": "application/json"},
    )
    assert response.status_code == 200
    assert response.json()["status"] == "ok"
```

### CDC Tests (Pact)

Test pattern: consumer contracts for external services.

```python
# tests/contract/test_search_indexer_consumer.py
from pact import Consumer, Provider

pact = Consumer("acolyte-orchestrator").has_pact_with(Provider("search-indexer"))

def test_search_returns_articles(pact):
    pact.given("articles exist").upon_receiving(
        "a search request"
    ).with_request(
        method="GET",
        path="/v1/search",
        query={"q": "AI trends"},
    ).will_respond_with(
        status=200,
        body={"results": [...]},
    )
    
    with pact:
        # Test gateway code
        ...
```

### Running Tests

```bash
# All tests
cd acolyte-orchestrator && uv run pytest

# Unit tests only
uv run pytest tests/unit/ -v

# E2E tests only
uv run pytest tests/e2e/ -v

# Contract tests (Pact)
uv run pytest tests/contract/ -v --no-cov

# With coverage
uv run pytest --cov=acolyte

# Type check
uv run pyrefly check .

# Lint
uv run ruff check && uv run ruff format
```

## Common Extension Patterns

### Incremental Processing with Checkpointing

For nodes that process many items, use the incremental self-loop pattern:

```python
class IncrementalNode:
    def __init__(self, llm: LLMProviderPort, *, incremental: bool = False):
        self._llm = llm
        self._incremental = incremental

    async def __call__(self, state: ReportGenerationState) -> dict:
        work_items = state.get("work_items", [])
        cursor = state.get("cursor", 0)
        results = state.get("results", [])

        if self._incremental:
            # Process one item at a time
            item = work_items[cursor]
            result = await self._process(item)
            return {
                "results": results + [result],
                "cursor": cursor + 1,
            }
        else:
            # Process all items at once
            all_results = [await self._process(item) for item in work_items]
            return {"results": all_results}

def should_continue(state: ReportGenerationState) -> str:
    cursor = state.get("cursor", 0)
    work_items = state.get("work_items", [])
    if cursor < len(work_items):
        return "more"
    return "done"
```

### Structured Output with Fallback

For LLM calls that need reliable JSON:

```python
async def generate_validated(self, prompt: str, schema: dict) -> dict:
    response = await self._llm.generate(prompt, format="json")
    
    try:
        data = json.loads(response)
        # Validate against schema
        return data
    except (json.JSONDecodeError, ValidationError):
        # Fallback to fixed structure
        return self._get_fallback()
```

### Best Sections Tracking

For Writer node, track best non-error output for fallback:

```python
# Track best output per section
if not has_blocking_errors:
    best_sections[section_key] = body
    best_section_metrics[section_key] = {
        "blocking_count": 0,
        "char_len": len(body),
    }
```
