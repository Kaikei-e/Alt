---
title: Acolyte Pipeline Recovery
date: 2026-04-11
tags:
  - runbook
  - acolyte
  - operations
  - pipeline
  - recovery
---

# Acolyte Pipeline Recovery

## Overview

This runbook covers recovery procedures when Acolyte pipelines fail or get stuck. It addresses checkpoint corruption, orphaned runs, and systematic recovery.

## Symptoms

### Orphaned Runs

**Indicators:**
- Runs stuck in `running` status for >30 minutes
- No log activity for the run
- Jobs in `claimed` status but no progress

**Detection query:**
```sql
SELECT run_id, report_id, started_at, 
       NOW() - started_at as duration
FROM report_runs 
WHERE run_status = 'running' 
  AND started_at < NOW() - INTERVAL '30 minutes';
```

### Checkpoint Corruption

**Indicators:**
- Resume fails with `KeyError` or `TypeError`
- "Invalid checkpoint data" in logs
- State fields missing expected keys

### Job Queue Issues

**Indicators:**
- Jobs stuck in `pending` despite workers running
- Multiple jobs for same run
- `claimed_by` set but no activity

## Investigation Steps

### 1. Identify Stuck Runs

```bash
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
SELECT 
  r.run_id,
  r.report_id,
  r.run_status,
  r.started_at,
  r.failure_message,
  j.job_status,
  j.claimed_by,
  j.claimed_at
FROM report_runs r
LEFT JOIN report_jobs j ON r.run_id = j.run_id
WHERE r.run_status IN ('pending', 'running')
ORDER BY r.started_at DESC
LIMIT 20;
"
```

### 2. Check Checkpoint State

```bash
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
SELECT 
  thread_id,
  checkpoint_id,
  created_at
FROM checkpoints
WHERE thread_id LIKE 'acolyte-run:%'
ORDER BY created_at DESC
LIMIT 10;
"
```

### 3. Check Worker Logs

```bash
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator --tail=200 | \
  grep -E "(run_id|Pipeline|crashed|failed|checkpoint)"
```

## Recovery Procedures

### Procedure A: Resume from Checkpoint

**When to use:** Run failed but checkpoint is valid.

1. **Identify the run:**
   ```bash
   export RUN_ID="<failed-run-id>"
   ```

2. **Verify checkpoint exists:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
     "SELECT COUNT(*) FROM checkpoints WHERE thread_id = 'acolyte-run:$RUN_ID';"
   ```

3. **Execute resume:**
   ```bash
   docker exec -it acolyte-orchestrator \
     python scripts/resume_run.py --run-id $RUN_ID
   ```

4. **Monitor progress:**
   ```bash
   docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator -f | grep $RUN_ID
   ```

### Procedure B: Mark Run as Failed and Restart

**When to use:** Checkpoint is corrupted or resume repeatedly fails.

1. **Mark the run as failed:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   UPDATE report_runs 
   SET run_status = 'failed', 
       failure_code = 'manual_abort',
       failure_message = 'Manually aborted for recovery',
       finished_at = NOW()
   WHERE run_id = '<run-id>';
   "
   ```

2. **Mark jobs as failed:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   UPDATE report_jobs 
   SET job_status = 'failed'
   WHERE run_id = '<run-id>';
   "
   ```

3. **Clear corrupted checkpoint:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   DELETE FROM checkpoint_writes WHERE thread_id = 'acolyte-run:<run-id>';
   DELETE FROM checkpoint_blobs WHERE thread_id = 'acolyte-run:<run-id>';
   DELETE FROM checkpoints WHERE thread_id = 'acolyte-run:<run-id>';
   "
   ```

4. **Start a fresh run:**
   ```bash
   grpcurl -plaintext -d '{"report_id":"<report-id>"}' \
     localhost:8090 alt.acolyte.v1.AcolyteService/StartReportRun
   ```

### Procedure C: Bulk Cleanup of Orphaned Runs

**When to use:** Multiple runs stuck after system restart.

1. **Identify all orphaned runs:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   SELECT run_id, report_id, started_at 
   FROM report_runs 
   WHERE run_status = 'running' 
     AND started_at < NOW() - INTERVAL '1 hour';
   "
   ```

2. **Mark all as failed:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   UPDATE report_runs 
   SET run_status = 'failed', 
       failure_code = 'orphan_cleanup',
       failure_message = 'Cleaned up orphaned run after system recovery',
       finished_at = NOW()
   WHERE run_status = 'running' 
     AND started_at < NOW() - INTERVAL '1 hour';
   "
   ```

3. **Update associated jobs:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   UPDATE report_jobs j
   SET job_status = 'failed'
   FROM report_runs r
   WHERE j.run_id = r.run_id
     AND r.failure_code = 'orphan_cleanup';
   "
   ```

### Procedure D: Reset Job Queue

**When to use:** Jobs stuck in `claimed` or `pending` despite healthy workers.

1. **Reset stuck jobs to pending:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   UPDATE report_jobs 
   SET job_status = 'pending',
       claimed_by = NULL,
       claimed_at = NULL,
       attempt_no = attempt_no + 1
   WHERE job_status IN ('claimed', 'running')
     AND claimed_at < NOW() - INTERVAL '30 minutes';
   "
   ```

2. **Restart orchestrator to pick up jobs:**
   ```bash
   docker compose -f compose/compose.yaml -p alt restart acolyte-orchestrator
   ```

## Prevention

### Checkpoint Best Practices

- Keep `CHECKPOINT_ENABLED=true` in production
- Monitor checkpoint table size (clean old checkpoints periodically)
- Set appropriate timeouts for LLM calls

### Monitoring Recommendations

- Alert on runs in `running` status >30 minutes
- Alert on jobs in `claimed` status >15 minutes
- Monitor `report_runs` failure rate

## Verification

After recovery:

1. **No orphaned runs:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
     "SELECT COUNT(*) FROM report_runs WHERE run_status = 'running' AND started_at < NOW() - INTERVAL '30 minutes';"
   # Expected: 0
   ```

2. **Job queue healthy:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c \
     "SELECT job_status, COUNT(*) FROM report_jobs GROUP BY job_status;"
   ```

3. **New runs complete successfully:**
   ```bash
   # Test with a new report
   grpcurl -plaintext -d '{"title":"Recovery Test"}' \
     localhost:8090 alt.acolyte.v1.AcolyteService/CreateReport
   ```
