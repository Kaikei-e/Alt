---
title: Acolyte Degraded Mode Operations
date: 2026-04-11
tags:
  - runbook
  - acolyte
  - operations
  - degraded-mode
---

# Acolyte Degraded Mode Operations

## Overview

This runbook covers Acolyte operations when external dependencies are unavailable or degraded. The system can operate in limited modes depending on which services are affected.

## Dependency Health Matrix

| Dependency | Impact When Down | Degraded Behavior |
|------------|------------------|-------------------|
| acolyte-db | Full outage | No operations possible |
| AIX (Gemma4) | Generation blocked | Existing reports readable, new runs fail |
| search-indexer | Evidence retrieval blocked | Runs fail at Gatherer node |
| alt-butterfly-facade | API blocked | Direct access possible (port 8090) |

## Symptoms

### LLM Unavailable

**Indicators:**
- `ReadTimeout` errors in logs
- `ConnectTimeout` errors
- Runs stuck in `running` status
- `Pipeline crashed` log entries

**Log patterns:**
```
[ERROR] Pipeline crashed: ReadTimeout: timed out
[ERROR] LLM call failed: ConnectTimeout: Unable to connect to host
```

### Database Connection Lost

**Indicators:**
- `ConnectionError` in logs
- All API calls fail with 500
- No new runs can be started

**Log patterns:**
```
[ERROR] Database connection failed: ConnectionRefusedError
[ERROR] Pool exhausted: cannot acquire connection
```

### search-indexer Down

**Indicators:**
- Runs fail at Gatherer node
- `evidence` array is empty
- `404 Not Found` from search-indexer

**Log patterns:**
```
[ERROR] GathererNode failed: HTTPStatusError: 404 Not Found
[WARNING] No evidence retrieved for query
```

## Investigation Steps

### 1. Check Service Health

```bash
# Check acolyte-orchestrator health
curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/HealthCheck \
  -H "Content-Type: application/json" -d '{}'

# Check acolyte-db
docker exec -it acolyte-db pg_isready -U acolyte_user

# Check AIX/Ollama
curl -s http://aix:11436/api/tags

# Check search-indexer
curl -s http://localhost:7700/health
```

### 2. Check Active Runs

```bash
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
  "SELECT run_id, report_id, run_status, started_at, failure_message 
   FROM report_runs 
   WHERE run_status IN ('pending', 'running') 
   ORDER BY started_at DESC LIMIT 10;"
```

### 3. Check Recent Failures

```bash
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
  "SELECT run_id, failure_code, failure_message, finished_at 
   FROM report_runs 
   WHERE run_status = 'failed' 
   ORDER BY finished_at DESC LIMIT 10;"
```

### 4. Check Logs

```bash
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator --tail=100 | grep -E "(ERROR|WARN|crashed|failed)"
```

## Resolution Procedures

### LLM Recovery

1. **Verify AIX/Ollama is running:**
   ```bash
   docker compose -f compose/compose.yaml -p alt ps aix
   ```

2. **Restart AIX if needed:**
   ```bash
   docker compose -f compose/compose.yaml -p alt restart aix
   ```

3. **Wait for model load** (Gemma4 26B takes ~30s to load)

4. **Resume failed runs:**
   ```bash
   docker exec -it acolyte-orchestrator \
     python scripts/resume_run.py --run-id <run_id>
   ```

### Database Recovery

1. **Check acolyte-db container:**
   ```bash
   docker compose -f compose/compose.yaml -p alt ps acolyte-db
   ```

2. **Restart if needed:**
   ```bash
   docker compose -f compose/compose.yaml -p alt restart acolyte-db
   ```

3. **Wait for migrations:**
   ```bash
   docker compose -f compose/compose.yaml -p alt logs acolyte-db-migrator --tail=20
   ```

4. **Restart orchestrator** to re-establish connection pool:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart acolyte-orchestrator
   ```

### search-indexer Recovery

1. **Check search-indexer and Meilisearch:**
   ```bash
   docker compose -f compose/compose.yaml -p alt ps search-indexer meilisearch
   ```

2. **Restart search-indexer:**
   ```bash
   docker compose -f compose/compose.yaml -p alt restart search-indexer
   ```

3. **Verify search works:**
   ```bash
   curl "http://localhost:7700/v1/search?q=test"
   ```

4. **Failed runs must be restarted** (Gatherer failures are not resumable via checkpoint)

## Degraded Mode Operations

### Read-Only Mode

When LLM or search-indexer is down but database is healthy:

- `GetReport` works
- `ListReports` works
- `GetReportVersion` works
- `ListReportVersions` works
- `CreateReport` works (creates record, no generation)
- `StartReportRun` will queue but jobs will fail

### Manual Workarounds

**Skip Gatherer with cached evidence:**
Not currently supported. Evidence retrieval is mandatory.

**Use alternative LLM:**
Update `ACOLYTE_LLM_URL` in environment and restart orchestrator.

## Verification

After recovery, verify:

1. **Health check passes:**
   ```bash
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/HealthCheck \
     -H "Content-Type: application/json" -d '{}'
   ```

2. **New run completes:**
   ```bash
   # Create test report
   grpcurl -plaintext -d '{"title":"Recovery Test","report_type":"weekly_briefing"}' \
     localhost:8090 alt.acolyte.v1.AcolyteService/CreateReport
   
   # Start run
   grpcurl -plaintext -d '{"report_id":"<uuid>"}' \
     localhost:8090 alt.acolyte.v1.AcolyteService/StartReportRun
   
   # Check status
   grpcurl -plaintext -d '{"run_id":"<uuid>"}' \
     localhost:8090 alt.acolyte.v1.AcolyteService/GetRunStatus
   ```

3. **No stuck runs:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
     "SELECT COUNT(*) FROM report_runs WHERE run_status = 'running' AND started_at < NOW() - INTERVAL '30 minutes';"
   ```
