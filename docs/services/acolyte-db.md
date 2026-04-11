# Acolyte DB

_Last reviewed: April 11, 2026_

**Location:** `acolyte-migration-atlas/`
**Port:** 5438

## Role

- **Versioned Report Storage**: Mutable current state + immutable version snapshots
- **Change Tracking**: Field-level change items per version
- **Job Queue**: Row-level locking with `FOR UPDATE SKIP LOCKED`

## Architecture Overview

```mermaid
erDiagram
    reports ||--o{ report_versions : "has versions"
    reports ||--o{ report_sections : "has sections"
    reports ||--o| report_briefs : "has brief"
    reports ||--o{ report_runs : "has runs"

    report_versions ||--o{ report_change_items : "tracks changes"
    report_sections ||--o{ report_section_versions : "has versions"
    report_runs ||--o{ report_jobs : "has jobs"

    reports {
        uuid report_id PK
        text title
        text report_type
        int current_version
        uuid latest_successful_run_id
        timestamptz created_at
    }

    report_briefs {
        uuid report_id PK,FK
        text topic
        text report_type
        text time_range
        text[] entities
        text[] exclude_topics
        jsonb constraints_jsonb
    }

    report_versions {
        uuid report_id PK,FK
        int version_no PK
        bigserial change_seq
        text change_reason
        timestamptz created_at
        text prompt_template_version
        jsonb scope_snapshot
        jsonb outline_snapshot
        text summary_snapshot
    }

    report_change_items {
        uuid report_id PK,FK
        int version_no PK,FK
        text field_name PK
        text change_kind
        text old_fingerprint
        text new_fingerprint
    }

    report_sections {
        uuid report_id PK,FK
        text section_key PK
        int current_version
        int display_order
    }

    report_section_versions {
        uuid report_id PK,FK
        text section_key PK,FK
        int version_no PK
        text body
        jsonb citations_jsonb
        timestamptz created_at
    }

    report_runs {
        uuid run_id PK
        uuid report_id FK
        int target_version_no
        text run_status
        text planner_model
        text writer_model
        text critic_model
        timestamptz started_at
        timestamptz finished_at
        text failure_code
        text failure_message
    }

    report_jobs {
        uuid job_id PK
        uuid run_id FK
        text job_status
        int attempt_no
        text claimed_by
        timestamptz claimed_at
        timestamptz available_at
        timestamptz created_at
    }
```

## Tables

### reports

Mutable current state (minimal fields).

| Column | Type | Description |
|--------|------|-------------|
| `report_id` | UUID PK | Report identifier |
| `title` | TEXT | Report title |
| `report_type` | TEXT | Type (e.g., `weekly_briefing`) |
| `current_version` | INT | Current version number |
| `latest_successful_run_id` | UUID | Last successful run |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

### report_briefs

Typed input specification for report generation.

| Column | Type | Description |
|--------|------|-------------|
| `report_id` | UUID PK/FK | Report reference |
| `topic` | TEXT | Main topic |
| `report_type` | TEXT | Report type |
| `time_range` | TEXT | Time range (e.g., `7d`) |
| `entities` | TEXT[] | Entity filter |
| `exclude_topics` | TEXT[] | Exclusion filter |
| `constraints_jsonb` | JSONB | Additional constraints |

### report_versions

Immutable snapshots (one row per version bump).

| Column | Type | Description |
|--------|------|-------------|
| `report_id` | UUID PK/FK | Report reference |
| `version_no` | INT PK | Version number |
| `change_seq` | BIGSERIAL | Global change sequence |
| `change_reason` | TEXT | Version change reason |
| `created_at` | TIMESTAMPTZ | Version timestamp |
| `prompt_template_version` | TEXT | Prompt template used |
| `scope_snapshot` | JSONB | Scope at this version |
| `outline_snapshot` | JSONB | Outline at this version |
| `summary_snapshot` | TEXT | Summary at this version |

### report_change_items

Field-level change tracking per version.

| Column | Type | Description |
|--------|------|-------------|
| `report_id` | UUID PK/FK | Report reference |
| `version_no` | INT PK/FK | Version reference |
| `field_name` | TEXT PK | Changed field name |
| `change_kind` | TEXT | `added`/`updated`/`removed`/`regenerated` |
| `old_fingerprint` | TEXT | Previous content hash |
| `new_fingerprint` | TEXT | New content hash |

### report_sections

Mutable section state.

| Column | Type | Description |
|--------|------|-------------|
| `report_id` | UUID PK/FK | Report reference |
| `section_key` | TEXT PK | Section identifier |
| `current_version` | INT | Current section version |
| `display_order` | INT | Display ordering |

### report_section_versions

Immutable section content snapshots.

| Column | Type | Description |
|--------|------|-------------|
| `report_id` | UUID PK/FK | Report reference |
| `section_key` | TEXT PK/FK | Section reference |
| `version_no` | INT PK | Section version |
| `body` | TEXT | Section content |
| `citations_jsonb` | JSONB | Citation metadata |
| `created_at` | TIMESTAMPTZ | Version timestamp |

### report_runs

Execution records (one per generation attempt).

| Column | Type | Description |
|--------|------|-------------|
| `run_id` | UUID PK | Run identifier |
| `report_id` | UUID FK | Report reference |
| `target_version_no` | INT | Target version |
| `run_status` | TEXT | `pending`/`running`/`succeeded`/`failed`/`cancelled` |
| `planner_model` | TEXT | Model used for planner |
| `writer_model` | TEXT | Model used for writer |
| `critic_model` | TEXT | Model used for critic |
| `started_at` | TIMESTAMPTZ | Start timestamp |
| `finished_at` | TIMESTAMPTZ | End timestamp |
| `failure_code` | TEXT | Error code |
| `failure_message` | TEXT | Error message |

### report_jobs

Job queue with row-level locking.

| Column | Type | Description |
|--------|------|-------------|
| `job_id` | UUID PK | Job identifier |
| `run_id` | UUID FK | Run reference |
| `job_status` | TEXT | `pending`/`claimed`/`running`/`succeeded`/`failed` |
| `attempt_no` | INT | Retry count |
| `claimed_by` | TEXT | Worker ID |
| `claimed_at` | TIMESTAMPTZ | Claim timestamp |
| `available_at` | TIMESTAMPTZ | Available time (for delayed retry) |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

## Indexes

```sql
-- Version ordering
CREATE INDEX idx_report_versions_change_seq ON report_versions(change_seq);

-- Run queries
CREATE INDEX idx_report_runs_report_id ON report_runs(report_id);
CREATE INDEX idx_report_runs_active ON report_runs(run_status)
    WHERE run_status IN ('pending', 'running');

-- Job queue (FOR UPDATE SKIP LOCKED)
CREATE INDEX idx_report_jobs_claimable ON report_jobs(available_at)
    WHERE job_status = 'pending';

-- Brief topic search
CREATE INDEX idx_report_briefs_topic ON report_briefs(topic);
```

## Design Principles

### No updated_at

Uses `version_no` + `change_items` for tracking changes. This avoids:
- Clock skew issues
- Lost update detection complexity
- Ambiguous "last modified" semantics

### JSONB for Auxiliary Only

JSONB is used only for:
- `citations_jsonb` — Citation metadata
- `constraints_jsonb` — Additional constraints
- `scope_snapshot`, `outline_snapshot` — Version snapshots

NOT for core queryable fields (topic, report_type, etc.).

### Job Queue Pattern

Uses PostgreSQL row-level locking:

```sql
SELECT * FROM report_jobs
WHERE job_status = 'pending' AND available_at <= NOW()
ORDER BY available_at
FOR UPDATE SKIP LOCKED
LIMIT 1;
```

This ensures:
- No polling-based race conditions
- Atomic job claiming
- Automatic retry with `available_at` delay

## Migrations

Migrations are managed via [Atlas](https://atlasgo.io/):

```
acolyte-migration-atlas/
├── migrations/
│   ├── atlas.hcl                           # Atlas configuration
│   ├── atlas.sum                           # Migration checksum
│   ├── 20260409000000_create_acolyte_tables.sql
│   └── 20260410000000_create_report_briefs.sql
└── docker/
    ├── Dockerfile                          # Atlas migrator image
    └── scripts/
        └── migrate.sh                      # Migration entrypoint
```

### Running Migrations

```bash
# Via Docker Compose
docker compose -f compose/acolyte.yaml up acolyte-db-migrator

# Manual
atlas migrate apply --url "postgresql://user:pass@host:5438/acolyte_db?sslmode=disable"
```

## Compose Integration

```yaml
services:
  acolyte-db:
    image: postgres:18
    environment:
      POSTGRES_DB: acolyte_db
      POSTGRES_USER: acolyte_user
      POSTGRES_PASSWORD_FILE: /run/secrets/acolyte_db_password
    ports:
      - "5438:5432"
    volumes:
      - acolyte_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U acolyte_user -d acolyte_db"]
      interval: 10s
      timeout: 5s
      retries: 5

  acolyte-db-migrator:
    build:
      context: ./acolyte-migration-atlas
    depends_on:
      acolyte-db:
        condition: service_healthy
    environment:
      DATABASE_URL: postgresql://acolyte_user:${ACOLYTE_DB_PASSWORD}@acolyte-db:5432/acolyte_db
```

## Health Check

```bash
# PostgreSQL readiness
pg_isready -h localhost -p 5438 -U acolyte_user -d acolyte_db

# Connection test
psql -h localhost -p 5438 -U acolyte_user -d acolyte_db -c "SELECT 1"
```

## Related Services

| Service | Relationship |
|---------|-------------|
| `acolyte-orchestrator` | Primary consumer; stores reports and versions |
| `acolyte-db-migrator` | Runs Atlas migrations on startup |

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Migration failed | Schema drift | Check `atlas.sum`, run `atlas migrate hash` |
| Job stuck in claimed | Worker crash | Reset job status, check `claimed_at` timeout |
| Version gaps | Failed runs | Expected behavior; versions only bump on success |
| Slow queries | Missing index | Check `EXPLAIN ANALYZE`, add appropriate index |
