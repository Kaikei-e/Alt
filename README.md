# Alt

_The local-first, composable RSS knowledge pipeline._

Alt is a self-hosted RSS reader and “content refinery” that fetches feeds, scrubs & tags articles, indexes them for lightning-fast search and serves a clean reading UI – everything running in neatly isolated containers so you can swap parts out or scale them independently.
The project is built **mobile-first**, 100 % open-source (Apache-2.0), and engineered around a five-layer flavour of Clean Architecture with test-driven development at its core.:contentReference[oaicite:0]{index=0}

---

## Feature Highlights

| Category | What you get | Where it lives |
|----------|--------------|----------------|
| **Fast crawl** | Go workers pull and de-duplicate feeds in parallel | `alt-backend/` |
| **Readability cleanup** | Boiler-plate stripping & language detect | `pre-processor/` |
| **Auto-tagging** | spaCy + zero-shot NLI | `tag-generator/` |
| **Full-text & faceted search** | Meilisearch via a Go proxy | `search-indexer/` |
| **Daily digest bot** | LLM summariser | `news-creator/` |
| **One-command up** | `docker compose up` | `compose.yaml` |

---
