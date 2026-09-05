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
- news-creator-backend under load (local Ollama, `:11435`)

### ConnectTimeout

**Log pattern:**
```
[ERROR] LLM call failed: ConnectTimeout: Unable to connect to host
[ERROR] Gateway connection refused: news-creator:11434
```

**Root causes:**
- news-creator (proxy) or news-creator-backend (Ollama) down
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
# Check the HybridPrioritySemaphore proxy is responding. news-creator is
# published to the host at 127.0.0.1:11434 -- the bare "news-creator"
# hostname only resolves inside the alt-network. It has no Ollama-style
# /api/tags route (only /health, /health/deep, and its own routes).
curl -s http://localhost:11434/health

# Check which models are loaded/created on the Ollama backend itself
curl -s http://127.0.0.1:11435/api/tags | jq '.models[].name'
curl -s http://127.0.0.1:11435/api/ps | jq '.models[].name'

# Check queue depth (if available)
curl -s http://127.0.0.1:11435/api/ps | jq '.models[].details'
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
# From acolyte-orchestrator container. news-creator has no /api/tags route
# (see the note in step 2) -- use /health for a reachability check.
docker exec -it acolyte-orchestrator curl -s http://news-creator:11434/health

# Check DNS resolution. The image only installs curl (Dockerfile apt-get
# layer) -- nslookup/dig are not present, but getent is (glibc, always
# available on python:3.14-slim).
docker exec -it acolyte-orchestrator getent hosts news-creator
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

4. **Resume the failed run.** `scripts/` is not part of the
   acolyte-orchestrator image -- run this on the host from
   `acolyte-orchestrator/`:
   ```bash
   cd acolyte-orchestrator
   export ACOLYTE_DB_DSN="postgresql://acolyte_user:$(cat ../secrets/acolyte_db_password.txt)@localhost:5439/acolyte"
   export NEWS_CREATOR_URL="http://localhost:11434"
   export SEARCH_INDEXER_URL="http://localhost:9300"
   export CHECKPOINT_ENABLED=true
   uv run python scripts/resume_run.py --run-id <run_id>
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

1. **Restart news-creator-backend:**
   ```bash
   docker compose -f compose/compose.yaml -p alt restart news-creator-backend
   ```

2. **Wait for model load** (~30s for Gemma4 E4B):
   ```bash
   # Watch for model load completion
   docker compose -f compose/compose.yaml -p alt logs news-creator-backend -f | grep -i "loaded"
   ```

3. **Verify model is serving:**
   ```bash
   curl -s http://127.0.0.1:11435/api/generate -d '{"model":"gemma4-e4b-12k","prompt":"Hello","stream":false}' | jq '.response'
   ```

4. **Resume failed runs** (host-side, `scripts/` is not in the image -- see Procedure A step 4 for the full env setup):
   ```bash
   cd acolyte-orchestrator
   uv run python scripts/resume_run.py --run-id <run_id>
   ```

### Procedure D: Use Fallback Model

**Caution:** every service on this host shares one Ollama runner
(`news-creator-backend`) on a single GPU. `ACOLYTE_MODEL` must name a model
already created on that runner (see `news-creator/entrypoint-backend.sh`,
e.g. `gemma4-e4b-12k` / `gemma4-e4b-8k`) -- `gemma4:9b-it-q4_K_M` used in an
earlier version of this runbook is not one of them. Even switching between
two valid tags means Ollama is asked to serve a model other than the one
`news-creator`'s own `LLM_MODEL` defaults to, which risks the shared-runner
COLD_START/eviction thrashing this repository's postmortems warn about
(every other service alternating onto the same GPU pays a 100s+ TTFT). Treat
this procedure as a last resort and coordinate with whoever owns
`news-creator`/`compose/ai.yaml` before changing it.

1. **Update environment.** `gemma4-e4b-8k`'s Modelfile fixes `num_ctx` at
   8192 (`news-creator/entrypoint-backend.sh`), so `LLM_NUM_CTX` must drop
   to match -- leaving it at the shared default of 12288
   (`compose/acolyte.yaml`) is exactly the num_ctx alternation the caution
   above warns about, and it now also diverges from news-creator's own
   `LLM_NUM_CTX=12288` (`compose/ai.yaml`, [[000579]]), which is the other side
   of that alternation:
   ```bash
   # .env -- ACOLYTE_MODEL must already exist on news-creator-backend, and
   # LLM_NUM_CTX must match its fixed num_ctx.
   ACOLYTE_MODEL=gemma4-e4b-8k
   LLM_NUM_CTX=8192
   ```

2. **Restart orchestrator:**
   ```bash
   docker compose -f compose/compose.yaml -p alt restart acolyte-orchestrator
   ```

3. **Note:** A narrower-context model may produce lower quality output, and
   the first request after the switch pays the reload cost described above.

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
   time curl -s http://127.0.0.1:11435/api/generate \
     -d '{"model":"gemma4-e4b-12k","prompt":"Hello","stream":false}'
   # Should complete in <5s for simple prompt
   ```

2. **Run completes without timeout** (host-side resume, see Procedure A step 4 for the env setup):
   ```bash
   cd acolyte-orchestrator
   uv run python scripts/resume_run.py --run-id <run_id>
   ```

   Or start a fresh run. acolyte-orchestrator is served by uvicorn
   (HTTP/1.1 only), so `grpcurl` (which requires HTTP/2) cannot reach it --
   use the Connect-over-HTTP form instead:
   ```bash
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/StartReportRun \
     -H "Content-Type: application/json" \
     -d '{"report_id":"<uuid>"}'
   ```

3. **JSON output is complete:**
   ```bash
   # Check logs for successful JSON parsing
   docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator | \
     grep -E "(Parsed|sections|outline)" | tail -10
   ```
