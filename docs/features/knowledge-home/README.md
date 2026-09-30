# Knowledge Home

Knowledge Home is Alt's central knowledge discovery surface. It transforms raw RSS articles into a personalized, explainable feed where every item tells you *why* it appeared. Built on an **immutable, event-sourced CQRS architecture**, Knowledge Home treats events as the source of truth and read models as disposable projections that can be rebuilt at any time.

The system spans multiple services: **alt-backend** hosts the API handlers and usecases, **knowledge-sovereign** owns all durable state, the event log, and the projectors (ADR 000944) via a dedicated database, and **alt-frontend-sv** renders the UI through a BFF layer. External services like **pre-processor** (summaries) and **tag-generator** (tags) feed events into the pipeline.

```mermaid
graph LR
  subgraph "Producers"
    RSS["RSS Feeds"]
    PP["pre-processor"]
    TG["tag-generator"]
  end

  subgraph "alt-backend"
    API["Connect-RPC<br/>Handlers"]
    UC["Usecases"]
  end

  subgraph "knowledge-sovereign"
    EV["knowledge_events"]
    KP["Knowledge<br/>Projector"]
    HI["knowledge_home_items"]
    TD["today_digest_view"]
    RC["recall_candidate_view"]
  end

  subgraph "Frontend"
    FE["alt-frontend-sv"]
    BFF["alt-butterfly-facade"]
  end

  RSS --> API
  PP -->|SaveArticleSummary| API
  TG -->|SaveArticleTags| API
  API --> UC
  UC -->|AppendKnowledgeEvent| EV
  KP -->|consume events| EV
  KP -->|write| HI
  KP -->|write| TD
  KP -->|write| RC
  FE --> BFF --> API
  API -->|read via Connect-RPC| HI
  API -->|read via Connect-RPC| TD
  API -->|read via Connect-RPC| RC
```

## Reading Order

| # | Document | What You'll Learn |
|---|----------|-------------------|
| 1 | [Architecture](./architecture.md) | Design invariants, service boundaries, database schema |
| 2 | [Data Flow](./data-flow.md) | End-to-end event lifecycle, projector mechanics, scoring |
| 3 | [API Reference](./api-reference.md) | Connect-RPC endpoints, message schemas, service quality |
| 4 | [Extending](./extending.md) | How to add events, signals, projections; operational recipes |

## Key Terms

| Term | Definition |
|------|------------|
| **Event Log** | Append-only `knowledge_events` table. Source of truth for all state. |
| **Projection** | Read-optimized table derived from events (e.g., `knowledge_home_items`). Disposable and rebuildable. |
| **Projector** | Background job that consumes events and writes projections. Checkpoint-based, idempotent. |
| **Checkpoint** | Tracks the last processed `event_seq` for each projector. Enables incremental catch-up. |
| **Backfill** | One-time job that generates synthetic `ArticleCreated` events for pre-existing articles. |
| **Reproject** | Rebuild projections from scratch by replaying the event log. Used for schema migrations and algorithm changes. |
| **Lens** | A saved viewpoint (tag/feed/recency filters) that changes which items appear in the Home feed. |
| **Why-reason** | Explains why an item was surfaced (e.g., `new_unread`, `tag_hotspot`, `summary_completed`). |
| **Supersede** | When a summary or tag set is replaced by a newer version, the item shows an "updated" badge. |
| **Recall Candidate** | An item surfaced for re-engagement based on past interaction signals. |
| **TodayDigest** | Daily aggregation snapshot: article counts, top tags, availability flags. |
| **Knowledge Sovereign** | Independent microservice that owns all Knowledge Home writes and durable state. |

## Quick Links

| Area | Path |
|------|------|
| Domain models | `alt-backend/app/domain/knowledge_event.go`, `knowledge_home_item.go`, `today_digest.go`, `recall_candidate.go`, `recall_signal.go` |
| Projectors | `knowledge-sovereign/app/usecase/knowledge_home_projector/` |
| Projector runner / workers | `knowledge-sovereign/app/main_workers.go` |
| API handler | `alt-backend/app/orchestrator/connect/v2/knowledge_home/handler.go` |
| Admin handler | `alt-backend/app/orchestrator/connect/v2/knowledge_home_admin/handler.go` |
| Port interfaces | `alt-backend/app/orchestrator/port/knowledge_home_port/`, `today_digest_port/`, `recall_candidate_port/`, `recall_signal_port/`, `alt-backend/app/shared/port/knowledge_event_port/` |
| Sovereign service | `knowledge-sovereign/app/main.go`, `knowledge-sovereign/app/handler/` |
| Sovereign client | `alt-backend/app/shared/driver/sovereign_client/` |
| Sovereign proto | `proto/services/sovereign/v1/sovereign.proto` |
| Public API proto | `proto/alt/knowledge_home/v1/knowledge_home.proto` |
| Admin API proto | `proto/alt/knowledge_home/v1/knowledge_home_admin.proto` |
| Feature flags | `alt-backend/app/domain/feature_flag.go`, `alt-backend/app/orchestrator/gateway/feature_flag_gateway/gateway.go` |
| Migrations | `knowledge-sovereign/migrations/` |
| Frontend hooks | `alt-frontend-sv/src/lib/hooks/useKnowledgeHome.svelte.ts`, `useRecallRail.svelte.ts`, `useLens.svelte.ts`, `useStreamUpdates.svelte.ts` |
| Frontend components | `alt-frontend-sv/src/lib/components/knowledge-home/` |
| BFF routing | `alt-butterfly-facade/internal/handler/proxy_handler.go`, `admin_proxy_handler.go` |
