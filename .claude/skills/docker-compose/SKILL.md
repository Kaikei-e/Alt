---
name: docker-compose
description: Docker Compose commands for Alt stack
---

# Docker Compose Operations

## Basic Commands

```bash
docker compose -f compose/compose.yaml -p alt up -d           # Start all
docker compose -f compose/compose.yaml -p alt up -d <service> # Start one
docker compose -f compose/compose.yaml -p alt logs <service> -f
docker compose -f compose/compose.yaml -p alt down
docker compose -f compose/compose.yaml -p alt ps
docker compose -f compose/compose.yaml -p alt restart <service>
```

## Profiles

| Profile | Services |
|---------|----------|
| `db` | PostgreSQL, Redis, ClickHouse |
| `auth` | Kratos, auth-hub, auth-token-manager |
| `core` | alt-backend, alt-frontend-sv |
| `workers` | pre-processor, search-indexer, mq-hub |
| `ai` | news-creator, tag-generator |
| `rag` | rag-orchestrator, Meilisearch |
| `recap` | recap-worker, recap-subworker |
| `logging` | rask-log-aggregator, rask-log-forwarder |
| `observability` | metrics, Grafana |

## Health Checks

```bash
curl http://localhost:3000/api/health   # Frontend
curl http://localhost:9000/v1/health    # Backend
curl http://localhost:7700/health       # Meilisearch
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Stack won't start | `docker compose down` then `up -d` |
| Port conflict | Check `docker ps` for conflicting containers |
| Service unhealthy | Check logs with `docker compose logs <service>` |
