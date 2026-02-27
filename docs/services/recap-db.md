# Recap Database Schema Contract

_Last reviewed: February 28, 2026_

**Location:** `recap-db`, `recap-migration-atlas`

This document outlines the schema for the `recap_db` PostgreSQL database, which stores data related to the Recap Worker's processing of RSS feed articles.

## Tables

### `recap_jobs`
Stores information about each recap job, including its status, start/end times, and associated metadata.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `UUID`       | `PRIMARY KEY`       | Unique identifier for the recap job              |
| `status`         | `TEXT`       | `NOT NULL`          | Current status of the job (e.g., 'pending', 'in_progress', 'completed', 'failed') |
| `created_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the job was created               |
| `updated_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the job was last updated          |
| `article_id`     | `UUID`       | `NOT NULL`, `UNIQUE`| ID of the article being recapped                 |
| `genre`          | `TEXT`       | `NOT NULL`          | Genre of the article                             |
| `prompt_version` | `TEXT`       | `NOT NULL`          | Version of the prompt used for the LLM           |

### `recap_sections`
Stores the generated recap sections for each article.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `UUID`       | `PRIMARY KEY`       | Unique identifier for the recap section          |
| `job_id`         | `UUID`       | `NOT NULL`, `FOREIGN KEY (recap_jobs.id)` | ID of the associated recap job                   |
| `section_type`   | `TEXT`       | `NOT NULL`          | Type of section (e.g., 'summary', 'key_points')  |
| `content`        | `TEXT`       | `NOT NULL`          | The generated recap content                      |
| `created_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the section was created           |
| `updated_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the section was last updated      |

### `recap_sources`
Stores the sources (citations) used in the recap sections.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `UUID`       | `PRIMARY KEY`       | Unique identifier for the source                 |
| `section_id`     | `UUID`       | `NOT NULL`, `FOREIGN KEY (recap_sections.id)` | ID of the associated recap section               |
| `source_text`    | `TEXT`       | `NOT NULL`          | The original text from the source                |
| `start_offset`   | `INTEGER`    | `NOT NULL`          | Start character offset in the original article   |
| `end_offset`     | `INTEGER`    | `NOT NULL`          | End character offset in the original article     |
| `created_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the source was recorded           |
| `updated_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the source was last updated       |

### `recap_cluster_evidence`
Holds deduplicated evidence links that were returned by recap-subworker clusters so the public `/v1/recap/7days` API can surface articles without re-running the clustering pipeline.

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
Captures rolling tag-to-genre priors emitted by the tag-generator so the Recap worker’s hybrid classifier can boost/refine genres deterministically.

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

The table’s comments describe the window label semantics and expected weight/sample_size ranges; it is refreshed whenever `scripts/build_label_graph.py` or the background tag-generation thread runs.

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

### Views

- `pulse_latest_generations` — Latest pulse generation per job
- `pulse_quality_stats` — Quality statistics by version and tier
- `pulse_syndication_stats` — Syndication detection statistics by version
