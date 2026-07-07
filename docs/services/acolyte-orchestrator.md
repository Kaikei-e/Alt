# Acolyte Orchestrator

_Last reviewed: July 7, 2026_

**Location:** `acolyte-orchestrator/`
**Port:** 8090 (Connect-RPC + REST)

## Role

- **Versioned Report Generation**: LangGraph-based pipeline for generating AI research reports with version control
- **Evidence Retrieval**: Hybrid search (vector + BM25) via search-indexer with RRF fusion
- **Claim-Based Writing**: Structured paragraph generation with citation tracking
- **Critic Loop**: Quality assurance via revision feedback (max 3 iterations)

## Architecture Overview

```mermaid
flowchart TB
    subgraph Client
        BFF[alt-butterfly-facade]
    end

    subgraph acolyte-orchestrator
        Connect[Connect-RPC<br/>AcolyteService]
        Usecase[Usecases]
        Graph[LangGraph Pipeline]
    end

    subgraph Pipeline["LangGraph Nodes"]
        direction LR
        Planner --> Gatherer --> Curator --> Hydrator
        Hydrator --> Compressor --> QuoteSelector --> FactNormalizer
        FactNormalizer --> SectionPlanner --> Writer --> Critic
        Critic -->|revise| Writer
        Critic -->|accept| Finalizer
    end

    subgraph External
        NewsCreator[news-creator<br/>:11434]
        SearchIndexer[search-indexer<br/>:9300]
        AcolyteDB[(acolyte-db<br/>:5438)]
    end

    BFF -->|Connect-RPC| Connect
    Connect --> Usecase --> Graph
    Graph --> NewsCreator
    Graph --> SearchIndexer
    Graph --> AcolyteDB
```

## Directory Structure

```
acolyte-orchestrator/
├── Dockerfile
├── CLAUDE.md
├── main.py                         # Application factory (Starlette + DI wiring)
├── pyproject.toml
├── uv.lock
├── acolyte/
│   ├── config/
│   │   └── settings.py             # Pydantic settings from env vars
│   ├── domain/
│   │   ├── report.py               # Report, ReportVersion entities
│   │   ├── brief.py                # ReportBrief (input specification)
│   │   ├── claim.py                # Claim-based writing model
│   │   ├── fact.py                 # Extracted facts with source tracking
│   │   ├── paragraph.py            # Paragraph with citations
│   │   ├── section_contract.py     # Section structure contract
│   │   ├── query_facet.py          # Multi-facet query expansion
│   │   ├── query_variant.py        # Query variants for hybrid search
│   │   ├── fusion.py               # RRF/CC fusion strategies
│   │   ├── source_map.py           # Evidence source tracking
│   │   ├── quote_selection.py      # Quote extraction model
│   │   ├── compressed_evidence.py  # Compressed evidence chunks
│   │   ├── executive_summary.py    # ES generation model
│   │   ├── critic_taxonomy.py      # Critic feedback taxonomy
│   │   ├── eval.py                 # Evaluation models
│   │   └── run.py                  # ReportRun entity
│   ├── port/
│   │   ├── report_repository.py    # ReportRepositoryPort
│   │   ├── llm_provider.py         # LLMProviderPort
│   │   ├── evidence_provider.py    # EvidenceProviderPort
│   │   ├── job_queue.py            # JobQueuePort
│   │   ├── content_store.py        # ContentStorePort
│   │   └── report_evaluator.py     # ReportEvaluatorPort
│   ├── gateway/
│   │   ├── postgres_report_gw.py   # PostgreSQL report repository
│   │   ├── postgres_job_gw.py      # PostgreSQL job queue (FOR UPDATE SKIP LOCKED)
│   │   ├── ollama_gw.py            # Ollama LLM client
│   │   ├── vllm_gw.py              # vLLM client (alternative)
│   │   ├── news_creator_gw.py      # news-creator gateway
│   │   ├── search_indexer_gw.py    # search-indexer gateway
│   │   ├── checkpoint_factory.py   # LangGraph checkpoint factory
│   │   ├── memory_report_gw.py     # In-memory report repo (testing)
│   │   ├── memory_job_gw.py        # In-memory job queue (testing)
│   │   └── memory_content_store.py # In-memory content store
│   ├── handler/
│   │   └── connect_service.py      # AcolyteConnectService implementation
│   ├── usecase/
│   │   ├── create_report_uc.py
│   │   ├── get_report_uc.py
│   │   ├── list_reports_uc.py
│   │   ├── start_run_uc.py
│   │   ├── rerun_section_uc.py
│   │   ├── graph/
│   │   │   ├── report_graph.py     # LangGraph pipeline builder
│   │   │   ├── state.py            # ReportGenerationState
│   │   │   ├── xml_parse.py        # XML response parsing
│   │   │   ├── llm_parse.py        # LLM output parsing
│   │   │   └── nodes/
│   │   │       ├── planner_node.py         # Query expansion, facet extraction
│   │   │       ├── gatherer_node.py        # Hybrid search (vector + BM25)
│   │   │       ├── curator_node.py         # Evidence curation
│   │   │       ├── hydrator_node.py        # Full body fetch (top-N)
│   │   │       ├── compressor_node.py      # Evidence compression
│   │   │       ├── quote_selector_node.py  # Quote extraction
│   │   │       ├── fact_normalizer_node.py # Fact normalization
│   │   │       ├── section_planner_node.py # Section structure planning
│   │   │       ├── writer_node.py          # Claim-based paragraph generation
│   │   │       ├── critic_node.py          # Quality feedback + revision loop
│   │   │       ├── finalizer_node.py       # DB persistence
│   │   │       └── extractor_node.py       # (legacy)
│   │   └── eval/
│   │       ├── eval_runner.py
│   │       ├── checklist_evaluator.py
│   │       └── rubric_evaluator.py
│   ├── gen/proto/                  # Generated protobuf + Connect-RPC stubs
│   │   └── alt/acolyte/v1/
│   ├── infra/
│   │   └── logging.py              # structlog configuration
│   └── driver/
└── tests/
    ├── unit/                       # Per-node unit tests
    ├── e2e/                        # Service boot + Connect-RPC round-trip
    └── contract/                   # Pact CDC tests (news-creator, search-indexer)
```

## Configuration

### Environment Variables

#### Service

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST` | `0.0.0.0` | Server bind host |
| `PORT` | `8090` | Server port |
| `LOG_LEVEL` | `info` | Log level (debug/info/warning/error) |

#### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `ACOLYTE_DB_DSN` | `postgresql://postgres:password@localhost:5432/alt_db` | PostgreSQL connection string |
| `ACOLYTE_DB_PASSWORD_FILE` | - | Secret file path (Docker secrets) |
| `DB_POOL_MIN_SIZE` | `2` | Minimum pool connections |
| `DB_POOL_MAX_SIZE` | `10` | Maximum pool connections |

#### External Services

| Variable | Default | Description |
|----------|---------|-------------|
| `NEWS_CREATOR_URL` | `http://news-creator:11434` | Ollama LLM endpoint |
| `SEARCH_INDEXER_URL` | `http://search-indexer:9300` | search-indexer endpoint |

#### Auth

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVICE_SECRET` | - | Service token for internal auth |
| `SERVICE_TOKEN_FILE` | - | Secret file path |

#### LLM Provider

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_PROVIDER` | `ollama` | Provider selection (`ollama` or `vllm`) |
| `VLLM_API_KEY` | - | vLLM API key (if using vllm) |

#### LLM Defaults

| Variable | Default | Description |
|----------|---------|-------------|
| `DEFAULT_MODEL` | `gemma4-e4b-12k` | Default LLM model |
| `DEFAULT_NUM_PREDICT` | `2000` | Default max tokens |
| `LLM_NUM_CTX` | `12288` | Context window size |
| `LLM_STOP_TOKENS` | - | Comma-separated stop tokens |

#### LLM Mode Tuning

| Variable | Default | Description |
|----------|---------|-------------|
| `STRUCTURED_TEMPERATURE` | `0.0` | Temperature for structured output |
| `STRUCTURED_NUM_PREDICT` | `1024` | Max tokens for structured output |
| `LONGFORM_TEMPERATURE` | `0.7` | Temperature for longform generation |
| `LONGFORM_NUM_PREDICT` | `4000` | Max tokens for longform |
| `LONGFORM_THINK` | `false` | Enable thinking mode for longform |

#### Paragraph Generation

| Variable | Default | Description |
|----------|---------|-------------|
| `PARAGRAPH_NUM_PREDICT` | `1000` | Default paragraph tokens |
| `PARAGRAPH_NUM_PREDICT_ANALYSIS` | `1200` | Analysis section tokens |
| `PARAGRAPH_NUM_PREDICT_CONCLUSION` | `1500` | Conclusion section tokens |
| `PARAGRAPH_NUM_PREDICT_ES` | `600` | Executive summary tokens |

#### Fact Normalization

| Variable | Default | Description |
|----------|---------|-------------|
| `FACT_NUM_PREDICT` | `512` | Fact extraction tokens |
| `MAX_FACTS_TOTAL` | `20` | Maximum facts per report |

#### Job Worker

| Variable | Default | Description |
|----------|---------|-------------|
| `JOB_POLL_INTERVAL_SECONDS` | `5.0` | Job queue poll interval |
| `WORKER_ID` | `acolyte-1` | Worker identifier |

#### Checkpointing

| Variable | Default | Description |
|----------|---------|-------------|
| `CHECKPOINT_ENABLED` | `false` | Enable LangGraph checkpointing |

## API Endpoints

### REST

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |

### Connect-RPC (AcolyteService)

| RPC | Description |
|-----|-------------|
| `CreateReport` | Create a new report with optional brief |
| `GetReport` | Get report with current sections |
| `ListReports` | Paginated report list |
| `GetReportVersion` | Get specific version snapshot |
| `ListReportVersions` | List version history with change items |
| `DiffReportVersions` | Diff between two versions |
| `StartReportRun` | Start generation pipeline |
| `GetRunStatus` | Get run status and jobs |
| `StreamRunProgress` | Stream run progress events (server-streaming) |
| `RerunSection` | Regenerate a specific section |
| `HealthCheck` | Health check |

## LangGraph Pipeline

The report generation pipeline consists of 11 nodes:

| Node | Role |
|------|------|
| **Planner** | Query expansion, facet extraction from brief |
| **Gatherer** | Hybrid search (vector + BM25) with RRF fusion |
| **Curator** | Evidence curation and ranking |
| **Hydrator** | Fetch full article bodies (top-N) |
| **Compressor** | Compress evidence to fit context window |
| **QuoteSelector** | Extract key quotes with source tracking |
| **FactNormalizer** | Normalize facts across sources |
| **SectionPlanner** | Plan section structure with claim contracts |
| **Writer** | Claim-based paragraph generation with citations |
| **Critic** | Quality feedback, triggers revision loop (max 3) |
| **Finalizer** | Persist to database, bump version |

### Pipeline Checkpointing

When `CHECKPOINT_ENABLED=true`:
- Uses PostgreSQL-backed LangGraph checkpointer
- Enables resume from any node after crash
- `durability="sync"` ensures persistence before next step
- Critical for long-running pipelines (70+ minutes)

## Health Check

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8090/health"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 30s
```

### Manual Verification

```bash
# REST health
curl http://localhost:8090/health

# Connect-RPC health (via grpcurl)
grpcurl -plaintext localhost:8090 alt.acolyte.v1.AcolyteService/HealthCheck
```

## Related Services

| Service | Relationship |
|---------|-------------|
| `acolyte-db` | PostgreSQL storage for reports and versions |
| `news-creator` | LLM inference plane (Ollama) |
| `search-indexer` | Evidence retrieval (hybrid search) |
| `alt-butterfly-facade` | BFF routing to Acolyte API |

## Development

### Running Locally

```bash
cd acolyte-orchestrator

# Install dependencies
uv sync

# Run tests (TDD first!)
uv run pytest

# Type check
uv run pyrefly check .

# Lint
uv run ruff check && uv run ruff format

# Run server
uv run uvicorn main:create_app --factory --host 0.0.0.0 --port 8090
```

### Docker

```bash
# Build and run
docker compose -f compose/acolyte.yaml up --build acolyte-orchestrator -d

# Logs
docker compose -f compose/acolyte.yaml logs -f acolyte-orchestrator
```

### Proto Code Generation

```bash
cd proto && buf generate --template buf.gen.acolyte.yaml
```

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Pipeline stuck | Checkpoint corruption | Clear checkpoints, restart run |
| LLM timeout | Model overloaded | Increase `OLLAMA_TIMEOUT`, check news-creator capacity |
| Empty sections | No evidence found | Check search-indexer connectivity, verify article indexing |
| Revision loop exhausted | Quality threshold unmet | Review critic feedback, adjust prompts |
| Connection refused | Service not ready | Wait for health check; verify port 8090 exposed |

## Known failure patterns

Cross-cutting incident knowledge lives in [[crystallized-knowledge]]; symptom-first entry points are in the [[README|runbooks index]].

- **Empty report sections while every layer returns HTTP 200** → search-indexer started requiring `X-Service-Token` and the gateway swallowed the 401 as a warning, so reports were generated from zero evidence for 24h → PM-2026-025. Auth-boundary changes must ship consumer-side token injection in the same deploy; treat 401 as fail-fast, never as a silent degrade.
- **Resume after crash re-runs a long node from its start** → the LangGraph checkpointer persists only at super-step (node) boundaries; resume is a replay from the node head, never mid-loop. Split multi-item loops into per-item self-loop super-steps, use `durability="sync"`, and keep node side effects idempotent → [[000673]], [[000679]], [[000690]], [[acolyte-checkpoint-resume]].
- **Crashed run turns zombie or is resumed by the wrong pipeline** → in-flight job rows from a dead process are orphans by definition; boot-time sweep must seal them as `failed` (keeping at most one resume candidate inside an age window), and resume queries must filter on the `trigger_source` discriminator → [[000708]], [[000709]], PM-2026-024, [[acolyte-pipeline-recovery]].
- **Truncated or invalid JSON from structured LLM calls** → three known Gemma4/Ollama bugs: thinking tokens consume `num_predict`, `think=false` + `format` ignores the format, and `/api/generate` ignores `think`. Design around them with a deterministic main path (LLM as secondary), micro-generation, and tiny schemas → [[000665]], [[000671]], [[000675]], [[acolyte-llm-timeout]].
- **mTLS handshake failures although certs on disk are fresh** → a long-lived nginx TLS sidecar kept serving its in-memory old cert after pki-agent rotated the files; "cert renewed" and "cert being served" are different facts. Restart the sidecar, then verify the served cert, not the file → PM-2026-029, [[pki-agent-recovery]].
