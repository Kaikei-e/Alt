# Recap Database Schema Contract

_Last reviewed: September 5, 2026_

**Location:** `recap-db`, `recap-migration-atlas`

This document outlines the schema for the `recap_db` PostgreSQL database, which stores data related to the Recap Worker's processing of RSS feed articles.

## Tables

### `recap_jobs`
Tracks each recap pipeline run (batch, manual, or morning-update) and its lifecycle status. `id` is an internal `BIGSERIAL`; `job_id` is the UUID every other table joins against.

| Column Name        | Type          | Constraints         | Description                                      |
|--------------------|---------------|----------------------|--------------------------------------------------|
| `id`               | `BIGSERIAL`   | `PRIMARY KEY`        | Internal row id                                  |
| `job_id`           | `UUID`        | `NOT NULL`, `UNIQUE`  | Job identifier used by every other recap table   |
| `kicked_at`        | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | When the job was kicked off               |
| `note`             | `TEXT`        |                      | Free-form operator note                          |
| `status`           | `TEXT`        | `NOT NULL`, `DEFAULT 'pending'`, `CHECK (pending, running, completed, failed, morning_completed)` | Job lifecycle status. There is no `morning_failed` value — a failed morning-update run stays `failed`. |
| `last_stage`       | `TEXT`        |                      | Most recent pipeline stage the job reached       |
| `updated_at`       | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | Timestamp when the row was last updated   |
| `user_id`          | `UUID`        |                      | Requesting user, when the job is user-scoped     |
| `trigger_source`   | `TEXT`        | `NOT NULL`, `DEFAULT 'system'` | What kicked the job (e.g. `system`, `morning`, manual API) |
| `window_days`      | `INTEGER`     | `NOT NULL`           | Recap window in days. The 7-day product window was retired in 2026-04 — current values in practice are `1` (morning-update) and `3` (batch/manual); `7` remains reachable only through the manual `/v1/generate/recaps/7days` trigger, with no schema default any more. |

### `recap_job_status_history`
Immutable, append-only event log of job status transitions (event-sourcing pattern), added alongside — not replacing — the mutable `recap_jobs.status` column, which `recap-worker` still updates in place as the denormalized current state. `reason` has repeatedly been the fastest path to root-causing multi-day incidents — see Known failure patterns below.

| Column Name       | Type          | Constraints         | Description                                      |
|-------------------|---------------|----------------------|--------------------------------------------------|
| `id`              | `BIGSERIAL`   | `PRIMARY KEY`        | Unique identifier for the event                  |
| `job_id`          | `UUID`        | `NOT NULL`, `FOREIGN KEY (recap_jobs.job_id)` | Job this transition belongs to |
| `status`          | `TEXT`        | `NOT NULL`, `CHECK (pending, running, completed, failed, morning_completed)` | Status transitioned to |
| `stage`           | `TEXT`        |                      | Pipeline stage at the time of transition         |
| `transitioned_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | When the transition happened              |
| `reason`          | `TEXT`        |                      | Human-readable reason for the transition         |
| `actor`           | `TEXT`        | `DEFAULT 'system'`   | What caused the transition                       |

### `recap_job_articles`
Raw article backup fetched for a job, kept for dedup/replay and audit before downstream processing.

| Column Name        | Type          | Constraints         | Description                                      |
|---------------------|---------------|----------------------|--------------------------------------------------|
| `id`                | `BIGSERIAL`   | `PRIMARY KEY`        | Unique identifier for the row                    |
| `job_id`            | `UUID`        | `NOT NULL`, `FOREIGN KEY (recap_jobs.job_id)` | Owning job          |
| `article_id`        | `TEXT`        | `NOT NULL`           | Source article identifier                        |
| `title`             | `TEXT`        |                      | Article title                                    |
| `fulltext_html`     | `TEXT`        | `NOT NULL`           | Raw article HTML as fetched                       |
| `published_at`      | `TIMESTAMPTZ` |                      | Article's published timestamp                     |
| `source_url`        | `TEXT`        |                      | Article source URL                                |
| `lang_hint`         | `TEXT`        |                      | Language hint                                     |
| `normalized_hash`   | `TEXT`        | `NOT NULL`           | Hash used for near-duplicate detection            |
| `feed_id`           | `UUID`        |                      | Originating feed, for user-article tracking       |
| `original_user_id`  | `UUID`        |                      | User the article was fetched on behalf of         |

Unique on `(job_id, article_id)`.

### `recap_sections`
Lightweight per-genre pointer from a job to the LLM Responses API `response_id` used for that genre's clustering/summarization call. Distinct from `recap_outputs` below, which holds the actual generated content (and from the legacy, unwritten `recap_final_sections`).

| Column Name      | Type   | Constraints   | Description                          |
|------------------|--------|---------------|---------------------------------------|
| `job_id`         | `UUID` | `NOT NULL`    | Owning job                            |
| `genre`          | `TEXT` | `NOT NULL`    | Genre this section covers             |
| `response_id`    | `TEXT` |               | LLM Responses API response id         |

Primary key `(job_id, genre)`.

### `recap_outputs`
The only recap content table any live code path writes. Holds the actual generated recap per `(job_id, genre)`, written together with the `recap_sections` genre pointer (above) in one transaction via `persist_genre_output`. Actively read by the dashboard and the fetch endpoints (`/v1/recaps/7days`, `/v1/recaps/3days`, `/v1/recaps/search`).

| Column Name    | Type          | Constraints         | Description                                      |
|----------------|---------------|----------------------|---------------------------------------------------|
| `job_id`       | `UUID`        | `NOT NULL`           | Owning job                                         |
| `genre`        | `TEXT`        | `NOT NULL`           | Genre this section covers                          |
| `response_id`  | `TEXT`        | `NOT NULL`           | LLM Responses API response id                       |
| `title_ja`     | `TEXT`        | `NOT NULL`           | Japanese title                                      |
| `summary_ja`   | `TEXT`        | `NOT NULL`           | Japanese summary prose                              |
| `bullets_ja`   | `JSONB`       | `NOT NULL`           | Japanese bullet points                              |
| `created_at`   | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | Creation time                                |
| `body_json`    | `JSONB`       | `NOT NULL`           | Full structured output (clusters, evidence, metadata) |
| `updated_at`   | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | Last update time                             |
| `window_days`  | `INTEGER`     | `NOT NULL`, `DEFAULT 7` | Recap window the output belongs to. The 7-day default was never dropped when `recap_jobs.window_days` lost its default in the 3-day-only sweep — every reachable insert supplies the value explicitly, so this is a latent fallback rather than a live behaviour, but it means the column is not `NOT NULL`-only as the 3-day-only framing elsewhere in this doc might suggest. |
| `tags`         | `JSONB`       | `NOT NULL`, `DEFAULT '[]'` | KeyBERT-based semantic tags, tag-generator enriched |

Primary key `(job_id, genre)`.

### `recap_final_sections` (legacy, unused)
Original shape for per-genre recap content, created by the earliest migration (`20240101000200_create_recap_final_sections.sql`) with columns `(job_id, genre, response_id, title_ja, summary_ja, bullets_ja, created_at)` and PK `(job_id, genre)`. `recap_outputs` superseded it, but the table was never dropped. No live code path writes to it: its sole writer, `RecapDao::save_final_section`, is marked `#[allow(dead_code)]` and is unreachable from the pipeline, API, or scheduler. Its `INSERT` has also drifted out of sync with the table — it names `model_name`, `updated_at`, and `RETURNING id`, none of which exist on `recap_final_sections` — so if it were ever wired up it would fail at runtime rather than silently succeed. Treat this table as permanently empty.

### `recap_cluster_evidence`
Holds deduplicated evidence links that were returned by recap-subworker clusters so the `/v1/recaps/7days` and `/v1/recaps/3days` fetch APIs can surface articles without re-running the clustering pipeline.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `BIGSERIAL`  | `PRIMARY KEY`       | Unique identifier for the evidence row          |
| `cluster_row_id` | `BIGINT`     | `NOT NULL`, `FOREIGN KEY (recap_subworker_clusters.id)` | Which cluster produced this link (cascades on delete). |
| `article_id`     | `TEXT`       | `NOT NULL`          | Article identifier that supplied the supporting link (text UUID). |
| `title`          | `TEXT`       |                     | Optional article title snapshot.                |
| `source_url`     | `TEXT`       |                     | URL used by the cluster.                         |
| `published_at`   | `TIMESTAMPTZ`|                     | Article's published timestamp.                   |
| `lang`           | `TEXT`       |                     | Language hint for the evidence link.             |
| `rank`           | `SMALLINT`   | `NOT NULL`, `DEFAULT 0` | Order within the cluster to control UI display. |
| `created_at`     | `TIMESTAMPTZ`| `NOT NULL`, `DEFAULT NOW()` | Insertion time for audit purposes.           |

Unique and secondary indexes keep lookups fast:

- `uniq_recap_cluster_evidence_article` on `(cluster_row_id, article_id)` prevents duplicate links per cluster.
- `idx_recap_cluster_evidence_cluster_rank` on `(cluster_row_id, rank)` accelerates ordered evidence slides.
- `idx_recap_cluster_evidence_article` on `(article_id)` lets Recap worker count how many clusters reference an article.

### `tag_label_graph`
Captures rolling tag-to-genre priors so the Recap worker's hybrid classifier can boost/refine genres deterministically. Built and rewritten by **`recap-subworker`**'s `TagLabelGraphBuilder`, which aggregates `recap_genre_learning_results` (not the tag-generator service). Triggered via `recap-subworker`'s `/admin/build-graph`, `/admin/learning`, `/admin/learning-jobs`, or its background learning scheduler (`RECAP_SUBWORKER_LEARNING_SCHEDULER_ENABLED`).

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `window_label`   | `TEXT`       | `NOT NULL`          | Sliding window label such as `7d` (primary key part). |
| `genre`          | `TEXT`       | `NOT NULL`          | Genre name (primary key part).                   |
| `tag`            | `TEXT`       | `NOT NULL`          | Normalized tag string (primary key part).        |
| `weight`         | `REAL`       | `NOT NULL`, `CHECK (weight >= 0 AND weight <= 1)` | Normalised association strength. |
| `sample_size`    | `INTEGER`    | `NOT NULL`, `DEFAULT 0`, `CHECK (sample_size >= 0)` | Number of articles that contributed. |
| `last_observed_at`| `TIMESTAMPTZ`|                     | Latest observation used to surface freshness.    |
| `updated_at`     | `TIMESTAMPTZ`| `NOT NULL`, `DEFAULT NOW()` | When the row was refreshed.               |

Indexes:
- `idx_tag_label_graph_genre` (`genre`, `tag`) powers lookups inside the Recap worker.

The table’s comments describe the window label semantics and expected weight/sample_size ranges.

### `recap_genre_learning_results`
Tracks the inputs/outputs of the refine stage for offline evaluation, replay scripts, and auditing.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `job_id`         | `UUID`       | `NOT NULL`          | Recap job identifier (primary key part).         |
| `article_id`     | `TEXT`       | `NOT NULL`          | Article identifier (primary key part).           |
| `coarse_candidates` | `JSONB`   | `NOT NULL`          | Coarse stage candidate list with scores/keywords. |
| `refine_decision` | `JSONB`     | `NOT NULL`          | Final genre, confidence, strategy, LLm trace info. |
| `tag_profile`    | `JSONB`     | `NOT NULL`          | Top tag signals, confidences, entropy data.      |
| `graph_context`  | `JSONB`     | `NOT NULL`, `DEFAULT '[]'::JSONB` | Graph edges that were available during refinement. |
| `feedback`       | `JSONB`     |                     | Optional manual feedback/corrections.            |
| `telemetry`      | `JSONB`     |                     | Latency/count metrics captured during refine.    |
| `timestamps`     | `JSONB`     | `NOT NULL`          | Coarse/refine timetags for audit.                |
| `created_at`     | `TIMESTAMPTZ`| `NOT NULL`, `DEFAULT NOW()` | Creation time for the record.           |
| `updated_at`     | `TIMESTAMPTZ`| `NOT NULL`, `DEFAULT NOW()` | Last update timestamp.                    |

Indexes:
- `idx_recap_genre_learning_results_job_id` on `job_id`.
- `idx_recap_genre_learning_results_article_id` on `article_id`.
- GIN indexes on `refine_decision`, `tag_profile`, and `coarse_candidates` accelerate JSON path filters.

Comments explain each column’s role (coarse candidates, refine decision, tag profile, graph context, feedback, telemetry, timestamps) so downstream services understand what to expect before clogging the graph builder.

### `morning_letters` / `morning_letter_sources` / `morning_article_groups`
Backing store for the Morning Letter feature (recap-worker's morning-update daemon, `/v1/morning/letters/*`). Superseded the earlier `morning_daily_summaries` / `morning_daily_evidence` tables, which are no longer written by any code path.

`morning_article_groups` records overnight article dedup groups (`group_id`, `article_id`, `is_primary` for the group's representative article).

`morning_letters` holds one row per `(target_date, edition_timezone)`:

| Column Name                  | Type          | Constraints         | Description                                      |
|-------------------------------|---------------|----------------------|--------------------------------------------------|
| `id`                          | `UUID`        | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | Unique identifier                  |
| `target_date`                 | `DATE`        | `NOT NULL`           | Edition date                                      |
| `edition_timezone`            | `VARCHAR(64)` | `NOT NULL`, `DEFAULT 'Asia/Tokyo'` | Timezone the edition date is anchored to |
| `source_recap_job_id`         | `UUID`        |                      | Recap job whose output was folded into this letter |
| `is_degraded`                 | `BOOLEAN`     | `NOT NULL`, `DEFAULT FALSE` | Set when generation fell back to a degraded mode |
| `schema_version`              | `INTEGER`     | `NOT NULL`, `DEFAULT 1` | Result payload schema version                  |
| `generation_revision`         | `INTEGER`     | `NOT NULL`, `DEFAULT 1` | Bumped on regeneration for the same date       |
| `result_jsonb`                | `JSONB`       | `NOT NULL`           | Generated letter content                          |
| `model`                       | `VARCHAR(100)`|                      | LLM model used to generate the letter             |
| `generation_metadata_jsonb`   | `JSONB`       | `NOT NULL`, `DEFAULT '{}'` | Generation metadata                        |
| `created_at`                  | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | Creation time                              |

Unique on `(target_date, edition_timezone)`.

`morning_letter_sources` cites the articles behind each section of a letter: `letter_id` (FK, cascades), `section_key`, `article_id`, `source_type` (default `overnight`), `position`. Primary key `(letter_id, section_key, article_id)`.

### `pulse_generations`
Tracks Evening Pulse generation runs with version control and status.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `BIGSERIAL`  | `PRIMARY KEY`       | Unique identifier for the generation run        |
| `job_id`         | `UUID`       | `NOT NULL`          | Job UUID for this generation run                |
| `target_date`    | `DATE`       | `NOT NULL`, `DEFAULT CURRENT_DATE` | Target date for the pulse generation  |
| `version`        | `TEXT`       | `NOT NULL`, `CHECK (v2, v3, v4)` | Pulse algorithm version              |
| `status`         | `TEXT`       | `NOT NULL`, `CHECK (running, succeeded, failed)` | Generation status         |
| `topics_count`   | `INT`        | `NOT NULL`, `DEFAULT 0` | Number of topics generated (0-3)             |
| `started_at`     | `TIMESTAMPTZ`| `NOT NULL`, `DEFAULT NOW()` | Generation start time                   |
| `finished_at`    | `TIMESTAMPTZ`|                     | Generation finish time                           |
| `config_snapshot`| `JSONB`      | `NOT NULL`, `DEFAULT ‘{}’` | Configuration snapshot at generation time   |
| `result_payload` | `JSONB`      |                     | Full JSON result for succeeded generations       |
| `error_message`  | `TEXT`       |                     | Error message for failed generations             |

Indexes: `idx_pulse_generations_job_id`, `idx_pulse_generations_version_status`, `idx_pulse_generations_started_at`, `idx_pulse_generations_target_date`, GIN on `config_snapshot`.

### `pulse_cluster_diagnostics`
Per-cluster quality metrics and syndication detection results.

| Column Name         | Type         | Constraints         | Description                                   |
|---------------------|--------------|---------------------|-----------------------------------------------|
| `id`                | `BIGSERIAL`  | `PRIMARY KEY`       | Unique identifier                            |
| `generation_id`     | `BIGINT`     | `NOT NULL`, `FK (pulse_generations.id) ON DELETE CASCADE` | Parent generation |
| `cluster_id`        | `BIGINT`     | `NOT NULL`          | Cluster identifier                            |
| `cohesion`          | `REAL`       | `NOT NULL`          | Title cohesion score (0.0-1.0)               |
| `ambiguity`         | `REAL`       | `NOT NULL`          | Ambiguity score (0.0-1.0, lower is better)   |
| `entity_consistency`| `REAL`       | `NOT NULL`          | Entity consistency score (0.0-1.0)           |
| `quality_tier`      | `TEXT`       | `NOT NULL`, `CHECK (ok, caution, ng)` | Diagnosed quality tier        |
| `syndication_status`| `TEXT`       | `CHECK (original, canonical_match, wire_source, title_similar)` | Syndication detection |
| `article_count`     | `INT`        | `NOT NULL`          | Number of articles in cluster                 |
| `top_entities`      | `JSONB`      | `NOT NULL`, `DEFAULT ‘[]’` | Top entities from cluster articles       |

Unique: `(generation_id, cluster_id)`.

### `pulse_selection_log`
Topic selection decisions with scoring breakdown.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `BIGSERIAL`  | `PRIMARY KEY`       | Unique identifier                               |
| `generation_id`  | `BIGINT`     | `NOT NULL`, `FK (pulse_generations.id) ON DELETE CASCADE` | Parent generation |
| `topic_rank`     | `INT`        | `NOT NULL`, `CHECK (1-3)` | Topic rank (1-3)                            |
| `cluster_id`     | `BIGINT`     | `NOT NULL`          | Selected cluster identifier                      |
| `role`           | `TEXT`       | `NOT NULL`, `CHECK (need_to_know, trend, serendipity)` | Assigned role       |
| `impact_score`   | `REAL`       | `NOT NULL`          | Impact score component                           |
| `burst_score`    | `REAL`       | `NOT NULL`          | Burst score component                            |
| `novelty_score`  | `REAL`       | `NOT NULL`          | Novelty score component                          |
| `recency_score`  | `REAL`       | `NOT NULL`          | Recency score component                          |
| `final_score`    | `REAL`       | `NOT NULL`          | Final weighted score                             |
| `rationale`      | `TEXT`       | `NOT NULL`          | Human-readable rationale for selection           |

Unique: `(generation_id, topic_rank)`.

### `notification_outbox`
Transactional outbox for user-facing push notifications, produced inside the same local transaction as the recap completion that triggers them. It is co-located with recap job state (rather than in `alt-db`, which owns the per-device push queue) precisely so the outbox row and the business state change share one commit — an outbox only gives its guarantee (message sent iff the transaction commits) when the two live in the same database. A dual write into `alt-db` instead would silently lose notifications whenever the second write failed. A relay in `recap-worker` forwards rows to `alt-data-hub` over mTLS; delivery to the device itself is tracked separately in `alt-db`'s `push_deliveries`.

| Column Name       | Type          | Constraints         | Description                                      |
|-------------------|---------------|----------------------|---------------------------------------------------|
| `id`              | `UUID`        | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | Unique identifier              |
| `dedupe_key`      | `TEXT`        | `NOT NULL`, `UNIQUE` | Derived from the business fact (e.g. `recap:<job_id>`), never generated at send time, so a retry at any layer produces the same key and re-forwarding is harmless. |
| `user_id`         | `UUID`        | `NOT NULL`           | Notification recipient                            |
| `kind`            | `TEXT`        | `NOT NULL`           | Notification type                                 |
| `payload`         | `JSONB`       | `NOT NULL`           | Notification body                                 |
| `occurred_at`     | `TIMESTAMPTZ` | `NOT NULL`           | Business time supplied by the producer; deliberately not defaulted so the row's meaning does not depend on insert time. |
| `created_at`      | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT clock_timestamp()` | Row insertion time                 |
| `state`           | `TEXT`        | `NOT NULL`, `DEFAULT 'pending'`, `CHECK (pending, forwarding, forwarded, dead)` | Relay state |
| `attempts`        | `INT`         | `NOT NULL`, `DEFAULT 0` | Forward attempt count                          |
| `next_attempt_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT clock_timestamp()` | Doubles as the claim lease: the relay stamps it forward when it claims a row, so a row orphaned by a crashed relay re-enters the same claim query once the lease expires — there is no separate reclaim sweeper. |
| `locked_by`       | `TEXT`        |                      | Identity of the relay instance currently holding the claim |
| `last_error`      | `TEXT`        |                      | Most recent forward failure                       |
| `forwarded_at`    | `TIMESTAMPTZ` |                      | When the row was successfully forwarded            |

Index: `notification_outbox_claim_idx` on `(next_attempt_at, id)`, partial over `state IN ('pending', 'forwarding')` so it stays sized by backlog rather than by history.

Relay wiring lives in `recap-worker`: gated by `RECAP_NOTIFICATION_RELAY_ENABLED`, logging `notification_outbox_relay_disabled` at startup when off, and exporting `notification_outbox_oldest_pending_age_seconds` / `notification_outbox_last_tick_timestamp_seconds` gauges — see `docs/services/recap-worker.md`.

### `classification_job_queue`
Persistent retry/reclaim queue backing `recap-worker`'s `ClassificationJobQueue` (`src/app.rs`), so classification chunks survive worker restarts instead of being lost mid-job.

| Column Name      | Type          | Constraints         | Description                                      |
|------------------|---------------|----------------------|---------------------------------------------------|
| `id`             | `SERIAL`      | `PRIMARY KEY`        | Unique identifier                                |
| `recap_job_id`   | `UUID`        | `NOT NULL`           | Owning recap job                                 |
| `chunk_idx`      | `INT`         | `NOT NULL`           | Chunk index within the job                       |
| `status`         | `VARCHAR(20)` | `NOT NULL`, `DEFAULT 'pending'`, `CHECK (pending, running, completed, failed, retrying)` | Chunk status |
| `texts`          | `JSONB`       | `NOT NULL`           | Input texts for the chunk                        |
| `result`         | `JSONB`       |                      | Classification result once completed             |
| `error_message`  | `TEXT`        |                      | Last error, if any                               |
| `retry_count`    | `INT`         | `DEFAULT 0`          | Retries attempted so far                         |
| `max_retries`    | `INT`         | `DEFAULT 3`          | Retry ceiling                                    |
| `created_at`     | `TIMESTAMPTZ` | `DEFAULT NOW()`      | Row creation time                                |
| `started_at`     | `TIMESTAMPTZ` |                      | When a worker picked up the chunk                |
| `completed_at`   | `TIMESTAMPTZ` |                      | When the chunk finished                          |
| `next_retry_at`  | `TIMESTAMPTZ` |                      | Backoff deadline enforced by the pick query itself, added so retry backoff does not depend on a worker holding its permit through a `sleep()`. |

Unique on `(recap_job_id, chunk_idx)`. The pickable partial index (`idx_classification_queue_pickable`, on `created_at`) covers `status IN ('pending', 'retrying', 'running')` — the last so `pick_next_job` can reclaim stale `running` rows left behind by a worker that died mid-lease, without degrading to a sequential scan.

### Other operational tables
The following exist in the schema and are written by `recap-worker` / `recap-subworker` but are not detailed here; see the referenced migration for exact columns:
- `recap_worker_config` (`20251120000000_create_recap_worker_config.sql`) — insert-only, latest-wins store for graph-boost thresholds and other tuning parameters learned from genre classification.
- `recap_stage_state`, `recap_failed_tasks`, `recap_job_stage_logs` (`20251212000000_add_job_state_tracking.sql`) — pipeline resume checkpoints, retry-pending failed tasks, and per-stage start/finish logs.
- `recap_preprocess_metrics` (`20240101000100_create_recap_job_tables.sql`) — per-job preprocessing counters (e.g. `total_articles_fetched` vs `articles_processed`), keyed `(job_id, metric)`.
- `recap_run_diagnostics` (`20251212000001_add_recap_run_diagnostics.sql`) — per-run aggregated cluster-similarity statistics, one row per `recap_subworker_runs.id`.
- `recap_system_metrics` (`20251208000000_create_recap_system_metrics.sql`) — JSONB metrics keyed by `job_id` and `metric_type` (classification/clustering/summarization).
- `admin_jobs` (`20251209000000_create_admin_jobs_table.sql`) — async admin task tracking for `recap-subworker` (graph rebuild / learning jobs).
- `log_errors` (`20251209010000_create_log_errors.sql`) — parsed error-log rows for dashboard surfacing.
- `recap_evaluation_runs` (`20260304100000_create_recap_evaluation_runs.sql`) — persisted evaluation results (genre/cluster/summary/pipeline) keyed by `evaluation_id`.

### Views

- `pulse_latest_generations` — Latest pulse generation per job
- `pulse_quality_stats` — Quality statistics by version and tier
- `pulse_syndication_stats` — Syndication detection statistics by version

## Known failure patterns

- Jobs stuck in impossible states ("pending but last_stage=persist") → mutable UPDATE-based status tracking with no atomic history record; fixed by adding an immutable status-history table alongside it, written transactionally with the same status `UPDATE` — the origin of the project-wide event-sourcing invariants → [[000114]] (repair-only precursor: [[000113]]).
- Crashed morning job resumed for 7h as a batch recap pipeline → the `trigger_source` discriminator existed in the schema but every insert path left it at its default (dead schema); recovery required manual `UPDATE recap_jobs SET status='failed'`, and `RESUMABLE_MAX_AGE` was cut 12h→4h. A resume-attempt cap is the most reliable abandon signal → PM-2026-024.
- Silent SQL failure in the same incident → binding an f64 into `make_interval` failed on every call and single-layer `anyhow` wrapping hid the SQL error entirely → PM-2026-024.
- `recap_jobs` polluted with ~48 `pending` rows/day, misread as "Recap is stuck" → morning-update rode on the shared jobs table without its own lifecycle; separated with the dedicated terminal status `morning_completed` → [[000897]].
- recap-db OOM-killed as part of the pipeline-wide cascade → undersized `mem_limit` (raised to 2G); size limits once from measured production-scale peaks plus margin, not in small increments → PM-2026-001.
- Investigations: state tables beat logs → `recap_job_status_history.reason` contained the full root cause of a 4-day outage, and `recap_failed_tasks` segmentation is what finally separated four look-alike incidents; note that quality-degradation failures never reach `recap_failed_tasks` → PM-2026-031, PM-2026-033, PM-2026-038.
