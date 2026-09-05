---
title: Acolyte Manual Regeneration
date: 2026-04-11
tags:
  - runbook
  - acolyte
  - operations
  - regeneration
---

# Acolyte Manual Regeneration

## Overview

This runbook covers procedures for manually regenerating reports or sections when automatic generation produces unsatisfactory results.

## When to Regenerate

### Full Report Regeneration

- Report quality is poor across all sections
- Scope/brief was incorrect and needs correction
- Evidence sources have been updated significantly
- Model or prompt changes require re-generation

### Section Regeneration

- Single section has quality issues
- Specific section needs updated citations
- User requested revision of one section

`RerunSection` is implemented (`acolyte/usecase/rerun_section_uc.py`): it
regenerates one section synchronously using the report's existing brief and
outline snapshot, and bumps the report version. It refuses
(`FAILED_PRECONDITION`) while a full generation run is active on the same
report. See Procedure E below.

## Procedures

acolyte-orchestrator is served by uvicorn (HTTP/1.1 only); `grpcurl`
requires HTTP/2 and cannot reach it at all. Every call below uses the
Connect-over-HTTP form instead: `curl` against
`http://localhost:8090/alt.acolyte.v1.AcolyteService/<Method>` with a JSON
body and `Content-Type: application/json`.

### Procedure A: Full Report Regeneration

1. **Identify the report:**
   ```bash
   # List recent reports
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/ListReports \
     -H "Content-Type: application/json" -d '{"limit": 10}'
   
   export REPORT_ID="<report-uuid>"
   ```

2. **Check current version:**
   ```bash
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/GetReport \
     -H "Content-Type: application/json" -d "{\"report_id\":\"$REPORT_ID\"}"
   ```

3. **Start regeneration run:**
   ```bash
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/StartReportRun \
     -H "Content-Type: application/json" -d "{\"report_id\":\"$REPORT_ID\"}"
   
   export RUN_ID="<returned-run-id>"
   ```

4. **Monitor progress:**
   ```bash
   # Check run status
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/GetRunStatus \
     -H "Content-Type: application/json" -d "{\"run_id\":\"$RUN_ID\"}"
   
   # Watch logs
   docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator -f | grep $RUN_ID
   ```

5. **Verify new version:**
   ```bash
   # Check version incremented
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/GetReport \
     -H "Content-Type: application/json" -d "{\"report_id\":\"$REPORT_ID\"}"
   
   # View version history
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/ListReportVersions \
     -H "Content-Type: application/json" -d "{\"report_id\":\"$REPORT_ID\",\"limit\":5}"
   ```

### Procedure B: Regenerate with Modified Scope

If the original scope was incorrect:

1. **Note current scope:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   SELECT scope_snapshot 
   FROM report_versions 
   WHERE report_id = '<report-id>' 
   ORDER BY version_no DESC 
   LIMIT 1;
   "
   ```

2. **Update report brief:**
   ```bash
   # entities is TEXT[], not JSONB -- use Postgres array literal syntax.
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   UPDATE report_briefs 
   SET topic = 'New Topic',
       entities = '{\"Entity A\",\"Entity B\"}'
   WHERE report_id = '<report-id>';
   "
   ```

3. **Start regeneration:**
   ```bash
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/StartReportRun \
     -H "Content-Type: application/json" -d "{\"report_id\":\"$REPORT_ID\"}"
   ```

### Procedure C: Force Regeneration After Failed Run

If the previous run failed and left the report in an inconsistent state:

1. **Check failed run details:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   SELECT run_id, failure_code, failure_message, finished_at
   FROM report_runs
   WHERE report_id = '<report-id>'
   ORDER BY started_at DESC
   LIMIT 5;
   "
   ```

2. **Ensure no active runs:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   SELECT COUNT(*) 
   FROM report_runs 
   WHERE report_id = '<report-id>' 
     AND run_status IN ('pending', 'running');
   "
   # Should be 0
   ```

3. **Start fresh run:**
   ```bash
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/StartReportRun \
     -H "Content-Type: application/json" -d "{\"report_id\":\"$REPORT_ID\"}"
   ```

### Procedure D: Batch Regeneration

For regenerating multiple reports:

1. **Identify reports to regenerate:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   SELECT report_id, title, current_version 
   FROM reports 
   WHERE current_version > 0
     AND created_at > '2026-04-01'
   ORDER BY created_at;
   " > /tmp/reports_to_regen.txt
   ```

2. **Create regeneration script:**
   ```bash
   # scripts/batch_regen.sh
   for REPORT_ID in $(cat /tmp/report_ids.txt); do
     echo "Regenerating $REPORT_ID"
     curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/StartReportRun \
       -H "Content-Type: application/json" -d "{\"report_id\":\"$REPORT_ID\"}"
     
     # Wait between runs to avoid overwhelming LLM
     sleep 60
   done
   ```

3. **Monitor batch progress:**
   ```bash
   docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
   SELECT run_status, COUNT(*) 
   FROM report_runs 
   WHERE started_at > NOW() - INTERVAL '1 hour'
   GROUP BY run_status;
   "
   ```

### Procedure E: Regenerate a Single Section

For a targeted fix instead of a full report regeneration:

1. **Rerun the section:**
   ```bash
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/RerunSection \
     -H "Content-Type: application/json" \
     -d "{\"report_id\":\"$REPORT_ID\",\"section_key\":\"<section-key>\"}"
   ```
   `section_key` must match an existing `report_sections.section_key` for
   the report; the response's `run_id` field is currently always empty
   (the rerun runs synchronously, not as a tracked background run).

2. **Verify the version bumped:**
   ```bash
   curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/GetReport \
     -H "Content-Type: application/json" \
     -d "{\"report_id\":\"$REPORT_ID\"}"
   ```

## Quality Checks

After regeneration, verify quality:

### Check Section Count

```bash
curl -s http://localhost:8090/alt.acolyte.v1.AcolyteService/GetReport \
  -H "Content-Type: application/json" -d "{\"report_id\":\"$REPORT_ID\"}" | jq '.sections | length'
```

### Check Section Lengths

```bash
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
SELECT section_key, LENGTH(body) as char_count
FROM report_section_versions sv
JOIN report_sections s ON sv.report_id = s.report_id AND sv.section_key = s.section_key
WHERE sv.report_id = '<report-id>'
  AND sv.version_no = s.current_version;
"
```

### Check Citations Present

```bash
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
SELECT section_key, jsonb_array_length(citations_jsonb) as citation_count
FROM report_section_versions sv
JOIN report_sections s ON sv.report_id = s.report_id AND sv.section_key = s.section_key
WHERE sv.report_id = '<report-id>'
  AND sv.version_no = s.current_version;
"
```

### Check for Meta-Commentary

Look for phrases like "I cannot", "As an AI", "data was not provided":

```bash
docker exec -it acolyte-db psql -U acolyte_user -d acolyte -c "
SELECT section_key, 
       body LIKE '%I cannot%' OR body LIKE '%As an AI%' as has_meta
FROM report_section_versions sv
JOIN report_sections s ON sv.report_id = s.report_id AND sv.section_key = s.section_key
WHERE sv.report_id = '<report-id>'
  AND sv.version_no = s.current_version;
"
```

## Troubleshooting

### Run Completes But Version Not Incremented

Check finalizer logs:
```bash
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator | grep -A5 "Finalizer"
```

### Generation Takes Too Long

Check which node is slow. Each node logs its own `"<Name> completed"` line on exit (there is no matching start line, so use timestamps between consecutive lines to estimate duration):
```bash
docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator | \
  grep -E "(Planner|Gatherer|Curator|Hydrator|Compressor|QuoteSelector|FactNormalizer|Section planner|Writer|Critic|Finalizer|Extractor) completed"
```

Typical timings:
- Planner: 20-30s
- Gatherer: <1s
- Curator: 60-120s
- Writer (×3 sections): 60-100s each
- Critic: 25-30s
- Total: 7-10 minutes

### Output Quality Poor

1. Check evidence was retrieved:
   ```bash
   # In logs, look for Gatherer output (evidence_count field)
   docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator | grep "Gatherer completed"
   ```

2. Check curator filtered appropriately:
   ```bash
   # sections_curated / total_curated fields
   docker compose -f compose/compose.yaml -p alt logs acolyte-orchestrator | grep "Curator completed"
   ```

3. If evidence is missing, check search-indexer:
   ```bash
   curl "http://localhost:7700/v1/search?q=<topic>&limit=5"
   ```
