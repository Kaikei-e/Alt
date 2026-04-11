---
title: Acolyte LLM Timeout Diagnosis and Recovery
date: 2026-04-11
tags:
  - runbook
  - acolyte
  - operations
  - llm
  - timeout
---

# Acolyte LLM Timeout Diagnosis and Recovery

## Overview

This runbook covers diagnosis and recovery for LLM-related timeouts in the Acolyte pipeline. These are the most common failure mode, typically caused by model overload, insufficient `num_predict`, or network issues.

## Symptoms

### ReadTimeout

**Log pattern:**
```
[ERROR] Pipeline crashed: ReadTimeout: timed out
[ERROR] LLM call failed at node writer: ReadTimeout
```

**Root causes:**
- `num_predict` too low (thinking tokens exhaust budget)
- LLM service overloaded (queued requests)
- Network latency to AIX

### ConnectTimeout

**Log pattern:**
```
[ERROR] LLM call failed: ConnectTimeout: Unable to connect to host
[ERROR] Gateway connection refused: aix:11436
```

**Root causes:**
- AIX/Ollama service down
- Network connectivity issue
- Container not started

### JSON Truncation

**Log pattern:**
```
[WARNING] JSON parse failed: Expecting ',' or ']'
[ERROR] Structured output incomplete: missing closing brace
```

**Root causes:**
- `num_predict` exhausted before JSON completed
- Thinking tokens consumed token budget
- Model generated very long reasoning

## Investigation Steps

### 1. Identify the Failing Node

```bash
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator --tail=100 | \
  grep -E "(Pipeline crashed|LLM call failed|node)" | tail -20
```

Common timeout nodes:
- **Planner**: Generates section outline
- **Curator**: Scores evidence relevance
- **Writer**: Generates section bodies (most common)
- **Critic**: Analyzes for failure modes

### 2. Check LLM Service Health

```bash
# Check Ollama is responding
curl -s http://aix:11436/api/tags | jq '.models[].name'

# Check model is loaded
curl -s http://aix:11436/api/ps | jq '.models[].name'

# Check queue depth (if available)
curl -s http://aix:11436/api/ps | jq '.models[].details'
```

### 3. Analyze Token Usage

Look for `eval_count` vs `response_len` mismatch in logs:

```bash
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator | \
  grep -E "(eval_count|response_len|num_predict)" | tail -20
```

**Healthy pattern:**
```
eval_count=4500 response_len=3800
```

**Truncation pattern:**
```
eval_count=6000 response_len=28  # Almost all tokens used on thinking
```

### 4. Check Network Connectivity

```bash
# From acolyte-orchestrator container
docker exec -it acolyte-orchestrator curl -s http://aix:11436/api/tags

# Check DNS resolution
docker exec -it acolyte-orchestrator nslookup aix
```

## Resolution Procedures

### Procedure A: Increase num_predict

For JSON truncation or thinking token exhaustion:

1. **Identify the affected node** from logs

2. **Update num_predict** in the node configuration:
   ```python
   # In the node file (e.g., writer_node.py)
   response = await self._llm.generate(
       prompt=prompt,
       num_predict=6000,  # Increase from default 4096
       temperature=0.1,
   )
   ```

3. **Rebuild and restart:**
   ```bash
   docker compose -f compose/compose.yaml -p alt up --build -d acolyte-orchestrator
   ```

4. **Resume the failed run:**
   ```bash
   docker exec -it acolyte-orchestrator \
     python scripts/resume_run.py --run-id <run_id>
   ```

### Procedure B: Increase HTTP Timeout

For ReadTimeout with long-running generation:

1. **Check current timeout** in settings:
   ```python
   # acolyte/config/settings.py
   llm_timeout_seconds: int = 300  # 5 minutes
   ```

2. **Increase if needed:**
   ```python
   llm_timeout_seconds: int = 600  # 10 minutes
   ```

3. **Rebuild and restart**

### Procedure C: Restart LLM Service

If Ollama is unresponsive or overloaded:

1. **Restart AIX:**
   ```bash
   docker compose -f compose/compose.yaml -p alt restart aix
   ```

2. **Wait for model load** (~30s for Gemma4 26B):
   ```bash
   # Watch for model load completion
   docker compose -f compose/compose.yaml -p alt logs aix -f | grep -i "loaded"
   ```

3. **Verify model is serving:**
   ```bash
   curl -s http://aix:11436/api/generate -d '{"model":"gemma4:26b-it-q4_K_M","prompt":"Hello","stream":false}' | jq '.response'
   ```

4. **Resume failed runs:**
   ```bash
   docker exec -it acolyte-orchestrator \
     python scripts/resume_run.py --run-id <run_id>
   ```

### Procedure D: Use Fallback Model

If primary model is consistently failing:

1. **Update environment:**
   ```bash
   # In .env or compose file
   ACOLYTE_MODEL=gemma4:9b-it-q4_K_M  # Smaller model
   ```

2. **Restart orchestrator:**
   ```bash
   docker compose -f compose/compose.yaml -p alt restart acolyte-orchestrator
   ```

3. **Note:** Smaller models may produce lower quality output

## Prevention

### Recommended num_predict Values

| Node | Recommended | Rationale |
|------|-------------|-----------|
| Planner | 512 | Short structured outline |
| Curator | 2048 | Medium scoring responses |
| QuoteSelector | 4096 | Multiple quotes per article |
| FactNormalizer | 6000 | Long reasoning + facts |
| SectionPlanner | 4096 | Claim planning |
| Writer | 6000 | Full section bodies |
| Critic | 4096 | Failure mode analysis |

### Reasoning-First JSON Pattern

Always put `reasoning` field first in JSON schemas:

```json
{
  "reasoning": "... absorbs thinking tokens ...",
  "actual_output": "..."
}
```

This ensures thinking tokens are captured in a structured field rather than causing truncation.

### Monitoring Recommendations

- Alert on `ReadTimeout` count >5 in 10 minutes
- Alert on `eval_count` / `response_len` ratio >100 (indicates truncation)
- Track node execution times (baseline + alert on 2x deviation)

## Verification

After recovery:

1. **LLM responds quickly:**
   ```bash
   time curl -s http://aix:11436/api/generate \
     -d '{"model":"gemma4:26b-it-q4_K_M","prompt":"Hello","stream":false}'
   # Should complete in <5s for simple prompt
   ```

2. **Run completes without timeout:**
   ```bash
   docker exec -it acolyte-orchestrator \
     python scripts/resume_run.py --run-id <run_id>
   
   # Or start fresh run
   grpcurl -plaintext -d '{"report_id":"<uuid>"}' \
     localhost:8090 alt.acolyte.v1.AcolyteService/StartReportRun
   ```

3. **JSON output is complete:**
   ```bash
   # Check logs for successful JSON parsing
   docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator | \
     grep -E "(Parsed|sections|outline)" | tail -10
   ```
