# Clean Architecture: Principles and Foundations

## Table of Contents
- [1. The Dependency Rule (Uncle Bob)](#1-the-dependency-rule-uncle-bob)
- [2. Screaming Architecture (Uncle Bob)](#2-screaming-architecture-uncle-bob)
- [3. Hexagonal Architecture: Ports and Adapters (Cockburn)](#3-hexagonal-architecture-ports-and-adapters-cockburn)
- [4. Consumer-Owned Interfaces (Go Wiki)](#4-consumer-owned-interfaces-go-wiki)
- [5. Composition Root (Seemann & Clean Architecture Ch.26)](#5-composition-root-seemann--clean-architecture-ch26)
- [6. Fowler's Core Patterns](#6-fowlers-core-patterns)
  - [Unit of Work](#unit-of-work)
  - [Humble Object](#humble-object)
  - [Avoiding Anemic Domain Models](#avoiding-anemic-domain-models)
- [7. Package & Component Principles (ADP / SDP / SAP)](#7-package--component-principles-adp--sdp--sap)
- [8. Fitness-Function Tools (Future Evolution)](#8-fitness-function-tools-future-evolution)

---

## 1. The Dependency Rule (Uncle Bob)

Source: [The Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html) (Robert C. Martin, 2012)

> "This rule says that source code dependencies can only point inwards. Nothing in an inner circle can know anything at all about something in an outer circle."

> "Typically the data that crosses the boundaries is simple data structures."

> "The software in this layer contains application specific business rules."

> "There's no rule that says you must always have just these four. However, The Dependency Rule always applies."

> "We usually resolve this apparent contradiction by using the Dependency Inversion Principle."

### Concrete Meaning in Alt
- **Inward dependencies**: `REST` (Handler) and `Driver` are the outermost circles; `Usecase` and `Port` sit inside; `Domain` is at the core. Code in `domain/` and `usecase/` never mentions `driver/`, `gateway/`, `sql`, `http`, or `connect`.
- **Simple data structures**: Data crossing between REST and Usecase, or between Usecase and Gateway, consists of pure domain values or simple input structs, free of transport serialization tags or database schema metadata.
- **Application business rules**: Usecases coordinate the specific interactions of a single feature (e.g. archiving an article, triggering a recap generation), orchestrating domain entities without knowing whether data comes from PostgreSQL, Meilisearch, or an mTLS RPC.
- **Dependency Inversion**: Usecase needs to save an entity. It calls a `Port` interface located in the service's `port/` package (logically owned by the Usecase). `Gateway` in the outer layer implements that interface and calls `Driver`. Control flows outward, but source code dependencies point inward.

---

## 2. Screaming Architecture (Uncle Bob)

Source: [Screaming Architecture](https://blog.cleancoder.com/uncle-bob/2011/09/30/Screaming-Architecture.html) (Robert C. Martin, 2011)

> "So what does the architecture of your application scream?"

### Concrete Meaning in Alt
- When browsing directories in Alt services (e.g. `alt-backend/app/orchestrator/usecase/`), folder and file names scream business intent: `archive_article_usecase`, `register_feed_usecase`, `select_lens_usecase`, `summarize_article_usecase`.
- They do not scream the framework or transport (not `PostgresService` or `EchoRoutes`). Frameworks and delivery mechanisms (`rest/`, `connect/`) are delivery details kept at the periphery.

---

## 3. Hexagonal Architecture: Ports and Adapters (Cockburn)

Source: [Hexagonal Architecture](https://alistair.cockburn.us/hexagonal-architecture/) (Alistair Cockburn, 2005)

> "Allow an application to equally be driven by users, programs, automated test or batch scripts, and to be developed and tested in isolation from its eventual run-time devices and databases."

Cockburn defines two primary roles:
- A **primary actor** (driving actor) initiates interaction with the application (e.g. HTTP client, CLI, batch job runner).
- A **secondary actor** (driven actor) is triggered by the application to complete a task (e.g. database, downstream RPC peer, search engine).

### Concrete Meaning in Alt
- **Driving side**: In Alt, primary actors enter through the `REST` layer (HTTP handlers, Connect-RPC handlers, Redis stream consumers). They convert requests into domain calls and invoke Usecases.
- **Driven side**: Usecases drive secondary actors via `Port` contracts. Gateways and Drivers adapt these calls to PostgreSQL (`alt-data-hub`), Meilisearch (`search-indexer`), or Ollama (`news-creator`).
- **Isolation**: Usecases can be tested completely in memory by stubbing the Ports, with zero running containers or network sockets.

---

## 4. Consumer-Owned Interfaces (Go Wiki)

Source: [Go Code Review Comments: Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)

> "Go interfaces generally belong in the package that uses values of the interface type, not the package that implements those values."

### Concrete Meaning in Alt
- `Port` interfaces belong in the package that consumes them (`port/`, logically owned by `usecase/`), never in `gateway/` or `driver/`.
- If `archive_article_usecase` needs to save an archive status, the interface `ArchiveArticlePort` lives in `orchestrator/port/archive_article_port/` (adjacent to the usecase consumer).
- The gateway (`orchestrator/gateway/archive_article_gateway/`) imports the port and implements it. This keeps the port narrow to what the consumer actually needs (Interface Segregation Principle).

---

## 5. Composition Root (Seemann & Clean Architecture Ch.26)

Sources:
- [Composition Root](https://blog.ploeh.dk/2011/07/28/CompositionRoot/) (Mark Seemann, 2011)
  > "A Composition Root is a (preferably) unique location in an application where modules are composed together."
- *Clean Architecture* (Robert C. Martin, 2017), Chapter 26 "The Main Component":
  In Clean Architecture, the Main component is the ultimate detail and the dirtiest component. It acts as an initial plugin that configures the system, creates instances of all factories, strategies, and facilities, and hands control over to high-level orchestrators without higher-level components knowing anything about it.

### Concrete Meaning in Alt
- Entrypoints like `alt-backend/app/cmd/backend/main.go`, `alt-backend/app/cmd/datahub/main.go`, `search-indexer/app/main.go`, `news-creator/app/main.py`, and DI modules (`alt-backend/app/di/container.go`) are the only files permitted to import all layers.
- They instantiate Drivers, wrap them in Gateways, inject Gateways into Usecases as Ports, and wire Usecases into Handlers.
- No business logic or SQL queries live in the Composition Root; its sole duty is assembly and configuration validation ([.claude/rules/di-wiring.md](../../../rules/di-wiring.md) (local-only)).

---

## 6. Fowler's Core Patterns

### Unit of Work
Source: [Unit of Work](https://martinfowler.com/eaaCatalog/unitOfWork.html) (Martin Fowler, 2002)

> "Maintains a list of objects affected by a business transaction and coordinates the writing out of changes and the resolution of concurrency problems."

**Alt Application**: Usecase coordinates transactional work through a capability-oriented Port or a Unit of Work contract. Raw database transaction objects (`*sql.Tx`, `pgx.Tx`) are strictly confined to Driver and Gateway implementations and never leaked into the Usecase method signature.

### Humble Object
Source: [Humble Object](https://martinfowler.com/bliki/HumbleObject.html) (Martin Fowler)

The Humble Object pattern separates logic that is difficult to test (such as UI widgets, network sockets, or asynchronous event loops) into a very thin layer with virtually no logic, delegating all actual decision-making to a testable collaborator.

**Alt Application**: Handlers in Alt (`rest/`, `handler/`) and Drivers (`driver/`) are Humble Objects. A handler only unpacks requests, checks syntactic validation, calls the Usecase, and maps the response. It contains no branching business rules, ensuring that nearly all application logic can be tested in fast, deterministic unit tests without spin-up overhead.

### Avoiding Anemic Domain Models
Source: [Anemic Domain Model](https://martinfowler.com/bliki/AnemicDomainModel.html) (Martin Fowler)

An anemic domain model treats entities as passive bags of getters and setters, stripping them of business behavior and moving all logic into procedural service scripts. Rich domain modeling keeps invariants, calculations, and domain validations directly on the domain objects themselves.

**Alt Application**: Entities in `domain/` enforce their own invariants during construction and mutation (e.g. validating state transitions, checking URL structure, computing scoring heuristics). Usecase handles orchestration between different entities and ports, but pure rules and invariants stay encapsulated inside domain models.

---

## 7. Package & Component Principles (ADP / SDP / SAP)

Source: [Package Principles](https://en.wikipedia.org/wiki/Package_principles) (Robert C. Martin)

- **Acyclic Dependencies Principle (ADP)**: The dependency graph between packages must have no cycles. In Alt, `check_layers.sh` and Go build tools enforce strictly directed dependency paths (`REST → Usecase → Port`, `Gateway → Port`, `Gateway → Driver`).
- **Stable Dependencies Principle (SDP)**: Depend in the direction of stability. A package should only depend on packages that are more stable (harder to change) than itself. In Alt, `Domain` has zero internal dependencies and is maximally stable; `Usecase` depends on stable `Port` abstractions; REST and Gateway depend inward on Port/Domain; Driver depends only on external libs (never Domain; Driver owns row/response types and Gateway maps them to Domain).
- **Stable Abstractions Principle (SAP)**: A package should be as abstract as it is stable. In Alt, the core layers (`Port`, `Domain`) are rich in abstractions and invariant types, while the unstable outer layers (`Driver`, `REST`) contain concrete implementations.

---

## 8. Fitness-Function Tools (Future Evolution)

To continuously automate architectural integrity as Alt expands, fitness-function tools can be adopted to formalize rules currently checked by `scripts/check_layers.sh`:

- **Go**:
  - `go-arch-lint` (`github.com/fe3dback/go-arch-lint`): Declarative YAML rules defining layer boundaries and allowed imports per package.
  - `depguard` (via `golangci-lint`): Enforces package import allowlists and denylists per package prefix (e.g. banning `driver` or `database/sql` inside `usecase`).
- **Python**:
  - `import-linter`: Formalizes layers and contracts in `pyproject.toml`, rejecting forbidden cross-layer imports (e.g. banning `httpx` or `gateway` in `usecase`).
- **TypeScript**:
  - `dependency-cruiser`: Validates visual and rule-based dependency graphs in frontend and Node/Deno services.
- **Rust**:
  - Language module visibility: Rigorous use of `pub(crate)` and internal module nesting; `recap-worker` uses concrete clients in `clients/`, while the trait seams are `pipeline/*` stage traits such as `DispatchStage`.
