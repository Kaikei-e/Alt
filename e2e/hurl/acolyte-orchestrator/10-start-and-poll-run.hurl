# Run lifecycle: CreateReport(with scope) → StartReportRun → poll
# GetRunStatus until completed → GetReport asserts a version was
# committed.
#
# The pipeline drives the LangGraph (planner → gatherer → curator →
# hydrator → quote_selector → fact_normalizer → section_planner →
# writer → critic → finalizer). LLM calls land on
# news-creator-ollama-stub:11435; evidence retrieval lands on
# search-indexer:9300 (seeded by the run.sh pre-step).
#
# Strict assertion: terminal state MUST be "completed". A "failed"
# terminal state surfaces as a test failure here so the stub /
# pipeline mismatch is visible — see docs/services/acolyte-orchestrator
# for the LangGraph contract.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/CreateReport
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/acolyte-orchestrator/report-create-with-topic.json;
HTTP 200
[Captures]
report_id: jsonpath "$.reportId"
[Asserts]
jsonpath "$.reportId" matches "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"

POST {{base_url}}/alt.acolyte.v1.AcolyteService/StartReportRun
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}"
}
HTTP 200
[Captures]
run_id: jsonpath "$.runId"
[Asserts]
jsonpath "$.runId" matches "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"

# Single-shot status check immediately after StartReportRun. The
# background asyncio.create_task should have scheduled the pipeline
# and the run row should be in pending/running/succeeded — pipeline
# converges in <1s against the stub, so succeeded is reachable here.
# This catches the regression where the background task never starts.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/GetRunStatus
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "runId": "{{run_id}}"
}
HTTP 200
[Asserts]
jsonpath "$.run.runId" == "{{run_id}}"
jsonpath "$.run.reportId" == "{{report_id}}"
jsonpath "$.run.runStatus" matches "^(pending|running|succeeded)$"

# Poll until terminal state. retry: 240 × retry-interval: 5000 ms =
# 20 minute ceiling. Real-LLM pipeline ETA is 7-8 min per the review
# doc; against the stub it converges in <1s, so the headroom is
# generous. Exits as soon as runStatus reaches "succeeded".
# Postgres job gateway sets the terminal value to "succeeded" (NOT
# "completed" — that string only appears in the orchestrator's log
# event message). See postgres_job_gw.py:complete_run for the source
# of truth.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/GetRunStatus
Content-Type: application/json
Connect-Protocol-Version: 1
[Options]
retry: 240
retry-interval: 5000
{
  "runId": "{{run_id}}"
}
HTTP 200
[Asserts]
jsonpath "$.run.runStatus" == "succeeded"

# Confirm the finalizer committed a new version of the report. Capture
# one of the section_keys for the RerunSection scenario below.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/GetReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}"
}
HTTP 200
[Captures]
section_key: jsonpath "$.sections[0].sectionKey"
[Asserts]
jsonpath "$.report.reportId" == "{{report_id}}"
jsonpath "$.report.currentVersion" >= 1
jsonpath "$.sections" isCollection
jsonpath "$.sections" count >= 1
jsonpath "$.sections[0].sectionKey" isString

# ListReportVersions must reflect the finalizer commit.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/ListReportVersions
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}",
  "limit": 10
}
HTTP 200
[Asserts]
jsonpath "$.versions" isCollection
jsonpath "$.versions" count >= 1
jsonpath "$.versions[0].versionNo" >= 1

# RerunSection on the first section. Synchronous in the current
# handler (returns empty run_id, see connect_service.py:rerun_section).
POST {{base_url}}/alt.acolyte.v1.AcolyteService/RerunSection
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}",
  "sectionKey": "{{section_key}}"
}
HTTP 200

# Cleanup so reruns of the suite don't accumulate rows.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/DeleteReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}"
}
HTTP 200
