# Alt Platform Architecture Evolution

A series of architectural case studies from the Alt platform, documenting how the system evolved from initial design through production — including the debates, wrong turns, and self-corrections along the way.

## Articles

### 1. [Knowledge Home: From Concept to Production](knowledge-home-design-evolution.md)

The design and phased implementation of Knowledge Home — Alt's unified knowledge entry point. Built on an immutable data model (event sourcing, CQRS, disposable projections), delivered through 7 implementation phases with a contract-first API methodology and a 13-block UI architecture.

### 2. [Knowledge Sovereign: A Case Study in Bounded Context Evolution](knowledge-sovereign-bounded-context.md)

An 11-round architectural debate over whether to extract a microservice from the monolithic backend. The team's recommendation shifted from "build a microservice" to "logical module first, physical extraction gated on readiness" — a case study in evidence-based self-correction, referencing Shopify's modular monolith and Martin Fowler's guidance.

### 3. [Real-time News Detection and Alert Architecture in F#](realtime-detection-fsharp-architecture.md)

Two F# 10/.NET 10 services for real-time article processing: breaking news detection via burst analysis and embedding-based clustering (news-pulser), and user-defined alert rules via a functional evaluation engine (news-vigil). Demonstrates the Functional Core / Effectful Shell pattern in a production microservice context.

### 4. [AI Pipeline Evolution: RAG Redesign and Data Quality Architecture](ai-pipeline-evolution.md)

How incremental RAG fixes led to an architectural reset — from stateless retrieval + prompt stitching to stateful conversation orchestration with an explicit planner. Plus: the Tier1 sidecar pattern for filtering low-quality content at the ingestion boundary.

### 5. [Acolyte: From Concept to Production](acolyte-design-evolution.md)

The design and implementation of Acolyte — Alt's versioned report generation orchestrator. Built on LangGraph for pipeline orchestration with a version-first data model (immutable snapshots, field-level change tracking). Features an 11-node pipeline with checkpointing, evidence grounding through QuoteSelector and FactNormalizer, and Connect-RPC API boundaries.

## Themes

These documents share several recurring architectural principles:

- **Immutable data models**: Append-only event logs with disposable, rebuildable projections
- **Contract-first design**: Define API shapes and acceptance criteria before writing implementation code
- **Degradation-aware architecture**: Every external dependency has an explicit failure behavior
- **Evidence-based decisions**: Challenge assumptions against industry precedents; change course when evidence warrants it
- **Filter at the boundary**: Validate quality at system edges, not scattered across internal services

## Tech Stack

- **Backend**: Go, F# 10/.NET 10, Python 3.14+
- **Frontend**: SvelteKit, Svelte 5 Runes
- **Data**: PostgreSQL, Redis Streams, Meilisearch
- **AI/ML**: Ollama (Gemma 4), LangGraph, mxbai-embed-large embeddings
- **Protocols**: Connect-RPC, SSE
- **Infrastructure**: Docker Compose, nginx
- **Patterns**: Event Sourcing, CQRS, Clean Architecture, Functional Core / Effectful Shell, Version-First Data Model
