# Environment Variables

Complete reference for all environment variables used by Recap Worker.

## Required Variables

### Database

| Variable | Description | Example |
|----------|-------------|---------|
| `RECAP_DB_DSN` | PostgreSQL connection string for recap-db | `postgresql://user:pass@localhost:5432/recap_db` |

### Database Connection Pool

| Variable | Default | Description |
|----------|---------|-------------|
| `RECAP_DB_MAX_CONNECTIONS` | `50` | Maximum number of connections in the pool. Lowered from 100 to support multiple worker instances (5 instances × 50 = 250, within DB limit) |
| `RECAP_DB_MIN_CONNECTIONS` | `5` | Minimum number of connections to maintain in the pool |
| `RECAP_DB_ACQUIRE_TIMEOUT_SECS` | `60` | Maximum time to wait when acquiring a connection from the pool (seconds) |
| `RECAP_DB_IDLE_TIMEOUT_SECS` | `600` | Maximum time a connection can remain idle before being closed (seconds) |
| `RECAP_DB_MAX_LIFETIME_SECS` | `1800` | Maximum lifetime of a connection before it is recycled (seconds) |

### External Services

| Variable | Description | Example |
|----------|-------------|---------|
| `ALT_BACKEND_BASE_URL` | Base URL for the authenticated article feed API | `http://alt-backend:9000` |
| `SUBWORKER_BASE_URL` | Base URL for recap-subworker clustering service | `http://recap-subworker:8080` |
| `NEWS_CREATOR_BASE_URL` | Base URL for news-creator LLM summarizer | `http://news-creator:8000` |

## Optional Variables

### Server Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `RECAP_WORKER_HTTP_BIND` | `0.0.0.0:9005` | HTTP server bind address for Axum control plane |

### LLM Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_MAX_CONCURRENCY` | `1` | Maximum number of simultaneous clustering jobs (summary generation is always sequential) |
| `LLM_PROMPT_VERSION` | `recap-ja-v2` | Prompt blueprint version sent to news-creator |
| `LLM_SUMMARY_TIMEOUT_SECS` | `600` | Timeout for summary generation requests to news-creator (seconds). Summary generation runs sequentially in a queue, so longer timeouts are safe |

### Alt Backend Client

| Variable | Default | Description |
|----------|---------|-------------|
| `ALT_BACKEND_CONNECT_TIMEOUT_MS` | `3000` | Connection timeout for HTTP calls to alt-backend |
| `ALT_BACKEND_READ_TIMEOUT_MS` | `20000` | Read timeout for HTTP calls to alt-backend |
| `ALT_BACKEND_TOTAL_TIMEOUT_MS` | `30000` | Maximum time allowed for a full alt-backend request |

### Tag Generator Client

| Variable | Default | Description |
|----------|---------|-------------|
| `TAG_GENERATOR_BASE_URL` | `http://tag-generator:9400` | Base URL for the tag-generator ML service |
| `TAG_GENERATOR_CONNECT_TIMEOUT_MS` | `3000` | Connection timeout for tag-generator HTTP calls |
| `TAG_GENERATOR_TOTAL_TIMEOUT_MS` | `30000` | Total timeout for tag-generator HTTP calls |
| `TAG_GENERATOR_SERVICE_TOKEN` | — | Optional bearer token sent when talking to tag-generator |

### External Service Tokens

| Variable | Description |
|----------|-------------|
| `ALT_BACKEND_SERVICE_TOKEN` | Optional bearer token that Recap Worker forwards to alt-backend for authenticated feeds |

### Batch Processing

| Variable | Default | Description |
|----------|---------|-------------|
| `RECAP_WINDOW_DAYS` | `7` | Lookback window (in days) for the recap job |
| `RECAP_GENRES` | `ai,tech,business,politics,health,sports,science,entertainment,world,security,product,design,culture,environment,lifestyle,art_culture,developer_insights,pro_it_media,consumer_tech,global_politics,environment_policy,society_justice,travel_lifestyle,security_policy,business_finance,ai_research,ai_policy,games_puzzles,other` | Comma-separated genres processed during each run |

### Genre Model & Refinement Controls

| Variable | Default | Description |
|----------|---------|-------------|
| `RECAP_GENRE_MODEL_WEIGHTS` | — | Path to the genre classifier weights bundle (use `resources/genre_classifier_weights.json` in prod) |
| `RECAP_GENRE_MODEL_THRESHOLD` | `0.5` | Confidence threshold for assigning genres |
| `RECAP_GENRE_REFINE_ENABLED` | `false` | Whether the genre refinement pipeline (graph + LLM) runs |
| `RECAP_GENRE_REFINE_REQUIRE_TAGS` | `true` | Skip refinement for articles that never receive any genre tags |
| `RECAP_GENRE_REFINE_ROLLOUT_PERCENT` | `100` | Percentage of articles eligible for genre refinement (0–100) |

### Tag Label Graph Cache

| Variable | Default | Description |
|----------|---------|-------------|
| `TAG_LABEL_GRAPH_WINDOW` | `7d` | Lookback window label used when loading the `tag_label_graph` |
| `TAG_LABEL_GRAPH_TTL_SECONDS` | `900` | How long the cached graph stays valid before it is refreshed |

### Retry Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_MAX_RETRIES` | `3` | Maximum number of retry attempts for HTTP calls |
| `HTTP_BACKOFF_BASE_MS` | `250` | Initial delay for exponential backoff (milliseconds) |
| `HTTP_BACKOFF_CAP_MS` | `10000` | Upper bound on backoff delay (milliseconds) |

### OpenTelemetry Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `OTEL_EXPORTER_ENDPOINT` | — | OTLP endpoint (when unset tracing uses the JSON fmt layer only) |
| `OTEL_SAMPLING_RATIO` | `1.0` | Sampling ratio between 0.0 (none) and 1.0 (all spans) |

### Tokenization Helpers

| Variable | Default | Description |
|----------|---------|-------------|
| `SUDACHI_CONFIG_PATH` | — | Path to a Sudachi config file; when omitted the embedded tokenizer configuration is used (only relevant when building with the `with-sudachi` feature) |

## Example `.env` File

```bash
# Database
RECAP_DB_DSN=postgresql://recap_user:recap_pass@localhost:5432/recap_db

# Database connection pool (optional, defaults shown)
RECAP_DB_MAX_CONNECTIONS=50
RECAP_DB_MIN_CONNECTIONS=5
RECAP_DB_ACQUIRE_TIMEOUT_SECS=60
RECAP_DB_IDLE_TIMEOUT_SECS=600
RECAP_DB_MAX_LIFETIME_SECS=1800

# External services
ALT_BACKEND_BASE_URL=http://alt-backend:9000
SUBWORKER_BASE_URL=http://recap-subworker:8080
NEWS_CREATOR_BASE_URL=http://news-creator:8000
TAG_GENERATOR_BASE_URL=http://tag-generator:9400

# Server
RECAP_WORKER_HTTP_BIND=0.0.0.0:9005

# LLM
LLM_MAX_CONCURRENCY=1
LLM_PROMPT_VERSION=recap-ja-v2
LLM_SUMMARY_TIMEOUT_SECS=600

# Batch processing
RECAP_WINDOW_DAYS=7
RECAP_GENRES=ai,tech,business,politics,health,sports,science,entertainment,world,security,product,design,culture,environment,lifestyle,art_culture,developer_insights,pro_it_media,consumer_tech,global_politics,environment_policy,society_justice,travel_lifestyle,security_policy,business_finance,ai_research,ai_policy,games_puzzles,other

# Genre refinement
RECAP_GENRE_MODEL_WEIGHTS=resources/genre_classifier_weights.json
RECAP_GENRE_MODEL_THRESHOLD=0.5
RECAP_GENRE_REFINE_ENABLED=false
RECAP_GENRE_REFINE_REQUIRE_TAGS=true
RECAP_GENRE_REFINE_ROLLOUT_PERCENT=100

# Tag label graph cache
TAG_LABEL_GRAPH_WINDOW=7d
TAG_LABEL_GRAPH_TTL_SECONDS=900

# Graph pre-refresh settings
RECAP_PRE_REFRESH_GRAPH_ENABLED=true
RECAP_PRE_REFRESH_TIMEOUT_SECS=300

# Alt backend HTTP client
ALT_BACKEND_CONNECT_TIMEOUT_MS=3000
ALT_BACKEND_READ_TIMEOUT_MS=20000
ALT_BACKEND_TOTAL_TIMEOUT_MS=30000
ALT_BACKEND_SERVICE_TOKEN=alt-backend-service-token-placeholder

# Tag generator HTTP client
TAG_GENERATOR_CONNECT_TIMEOUT_MS=3000
TAG_GENERATOR_TOTAL_TIMEOUT_MS=30000
TAG_GENERATOR_SERVICE_TOKEN=tag-generator-service-token-placeholder

# Retry
HTTP_MAX_RETRIES=3
HTTP_BACKOFF_BASE_MS=250
HTTP_BACKOFF_CAP_MS=10000

# OpenTelemetry
OTEL_EXPORTER_ENDPOINT=http://otel-collector:4317
OTEL_SAMPLING_RATIO=1.0

# Tokenization (Sudachi feature builds only)
SUDACHI_CONFIG_PATH=/etc/recap-worker/sudachi_dict.json
```

## Validation

All environment variables are validated at startup. Missing required values or malformed entries (e.g., `RECAP_GENRE_REFINE_ROLLOUT_PERCENT` > 100) cause the service to exit with a descriptive error.

## Production Recommendations

- Use secrets management (e.g., Kubernetes secrets, AWS Secrets Manager).
- Never commit `.env` files to version control.
- Use different values for development, staging, and production.
- Rotate database passwords regularly.
- Monitor the OTLP endpoint and sampling ratio whenever tracing is enabled.
