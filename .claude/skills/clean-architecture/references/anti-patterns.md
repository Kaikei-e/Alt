# Anti-Patterns: Common Pitfalls and Smells in Alt

## Table of Contents
- [1. Handler Importing Driver / Raw SQL (`pgx`)](#1-handler-importing-driver--raw-sql-pgx)
- [2. Usecase Importing Infrastructure Libraries](#2-usecase-importing-infrastructure-libraries)
- [3. Port Interface Defined in Domain Layer](#3-port-interface-defined-in-domain-layer)
- [4. Port Defined on Implementation Side (in Gateway Package)](#4-port-defined-on-implementation-side-in-gateway-package)
- [5. Domain Entities Coupled with JSON / DB Tags or ORM Types](#5-domain-entities-coupled-with-json--db-tags-or-orm-types)
- [6. Handler Duplicating Driver and Business Work](#6-handler-duplicating-driver-and-business-work)
- [7. Reverse Dependency (Driver Importing Usecase)](#7-reverse-dependency-driver-importing-usecase)
- [8. God Usecase (Violating Single Responsibility)](#8-god-usecase-violating-single-responsibility)
- [9. "Mocks of Everything" Test Suites](#9-mocks-of-everything-test-suites)
- [10. Silent Nil-Guard for Unwired Dependencies](#10-silent-nil-guard-for-unwired-dependencies)
- [11. Driver Returns Domain Types](#11-driver-returns-domain-types)

---

## Examples observed in the repo

### 1. Handler Importing Driver / Raw SQL (`pgx`)
- **Smell**: A REST or Connect-RPC handler directly imports driver libraries (`pgx`, `database/sql`, Redis clients) and queries database rows or checks SQL-level errors.
- **Why it hurts**: Bypasses the Usecase and Gateway layers. Transports become coupled to database schema details, and unit testing the handler requires a real database connection or complex driver mocking.
- **Fix**: Move data fetching into a Port abstraction called by Usecase. Gateway executes queries and maps database errors to domain sentinel errors.
- **Example in Alt**:
  - `pre-processor/app/handler/summarize_handler.go:15` imports `"github.com/jackc/pgx/v5"` and directly inspects `errors.Is(err, pgx.ErrNoRows)` at line 366.
  - *Documented Exception*: `knowledge-sovereign/app/CLAUDE.md` documents that `handler/` and `usecase/` directly import `driver/sovereign_db` (no Port/Gateway, direct Handler→Driver shortcut). Such a shape must be documented, and the no-reverse-import rule still holds.

---

### 2. Usecase Importing Infrastructure Libraries
- **Smell**: A Usecase directly imports `httpx`, `asyncpg`, `redis`, `sqlalchemy`, or OpenTelemetry tracer packages.
- **Why it hurts**: Leaks infrastructural dependencies into pure business logic. Prevents testing Usecase in memory and forces business rules to change when HTTP clients or database libraries are updated.
- **Fix**: Define a narrow Port interface in `port/` (Usecase-owned) representing the capability (e.g. `LLMClient`, `ArticleFetcher`). Implement the Port in Gateway using the Driver.
- **Example in Alt**:
  - `acolyte-orchestrator/acolyte/usecase/graph/llm_parse.py:18` and `acolyte/usecase/graph/nodes/gatherer_node.py:20` directly run `import httpx` inside Usecase execution graphs.
  - `alt-backend/app/orchestrator/usecase/global_search_usecase/usecase.go:12-13` imports `"go.opentelemetry.io/otel/trace"`. Lines 15-18 document that this is intentional to accept an injected `trace.Tracer` port rather than the global registry, but it still leaks external telemetry types into Usecase (triggering a `WARN` in `check_layers.sh`).

---

### 3. Port Interface Defined in Domain Layer
- **Smell**: An interface describing an external service or database client is placed in the `domain/` package.
- **Why it hurts**: Domain models represent core concepts, value objects, and business calculations that should be pure and independent of I/O. Placing external communication interfaces in Domain implies the Domain orchestrates I/O.
- **Fix**: Move the Port interface to `port/` (Usecase-owned) where the consumer specifies what it needs.
- **Example in Alt**:
  - `alt-backend/app/domain/user_validator.go:10` defines `type AuthServiceClient interface` and a `UserValidator` calling I/O methods inside the Domain package.

---

### 4. Port Defined on Implementation Side (in Gateway Package)
- **Smell**: A Port interface is declared inside the `gateway/` or `driver/` package alongside its concrete implementation.
- **Why it hurts**: Violates the Dependency Inversion Principle. Ports belong to the consumer (Usecase), not the provider. Defining interfaces in the provider package creates an inverted conceptual coupling where callers depend on the provider's definition.
- **Fix**: Relocate the interface definition to `port/` (Usecase-owned). The Gateway package then imports the Port and implements it.

---

### 5. Domain Entities Coupled with JSON / DB Tags or ORM Types
- **Smell**: Domain structs contain struct tags like `json:"..."` or `db:"..."`, or import ORM/database types (`uuid.UUID` from third-party DB drivers, `sql.NullString`).
- **Why it hurts**: Couples the core domain model to transport serialization formats and relational database column names. Modifying an API response shape or renaming a column risks breaking core domain logic.
- **Fix**: Keep Domain entities clean of serialization tags. Define Transport DTOs in REST and Row structs in Driver, and let REST and Gateway perform explicit mappings to/from Domain.
- **Example in Alt**:
  - `alt-backend/app/domain/knowledge_home_item.go:36-57` contains both `json:"..."` and `db:"..."` tags on most fields.
  - `alt-backend/app/domain/tag_set_version.go:12` contains `json:"tag_set_version_id" db:"tag_set_version_id"`.
  - `alt-backend/app/domain/user_validator.go:17` contains `json:"id"` on the `User` domain struct.

---

### 6. Handler Duplicating Driver and Business Work
- **Smell**: REST handlers contain long blocks of logic performing URL parsing, SSRF validation, raw HTTP fetching, and error checking directly in the controller.
- **Why it hurts**: Leads to severe code duplication across endpoints. In past monorepo reviews, three related handlers duplicated ~600 lines of identical fetching and security logic.
- **Fix**: Extract the shared operations into a Usecase that orchestrates a security validator Port and an HTTP fetcher Port. Handlers remain thin adapters.

---

### 7. Reverse Dependency (Driver Importing Usecase)
- **Smell**: A `driver/` package imports `usecase/` or `service/` to reuse types or error codes.
- **Why it hurts**: Inverts the dependency direction completely. A database driver depending on a usecase causes dependency cycles and binds low-level storage to high-level policy.
- **Fix**: Move shared types to `domain/`, or duplicate simple driver-local constants instead of importing Usecase packages.
- **Example in Alt**:
  - `knowledge-sovereign/app/driver/sovereign_db/repository.go:44-46` explicitly documents this avoidance: `scoreOpMax` and `scoreOpSet` constants are mirrored locally rather than imported from `knowledge_home_projector` because `driver/ must not depend on usecase/ (Clean Architecture layer direction)`.

---

### 8. God Usecase (Violating Single Responsibility)
- **Smell**: A single Usecase struct handles 10+ disparate operations, mixes multiple business transactions, or accepts dozens of Ports.
- **Why it hurts**: Impossible to reason about in isolation, leads to huge mock setups in tests, and creates high merge contention.
- **Fix**: Split the monolithic usecase into dedicated, single-purpose usecases (e.g. `FetchArticleCursorUsecase`, `ArchiveArticleUsecase`).
- **Example in Alt**:
  - Documented in ADR-000572: `AltDBRepository` grew into an 8,978 LOC god object and `FetchFeedsPort` had 8 methods before being refactored into single-method interfaces (`FeedCursorPort`, `UnreadFeedCursorPort`) and dedicated modular usecases.

---

### 9. "Mocks of Everything" Test Suites
- **Smell**: A Usecase unit test sets up 8-10 GoMock expectations verifying every internal call sequence rather than testing business outcomes.
- **Why it hurts**: Tests become brittle and bound to implementation details. Any internal refactor that maintains identical behavior breaks all tests due to missing or unexpected mock calls.
- **Fix**: Provide simple in-memory fakes for Ports (e.g. an in-memory map for repositories). Test that given input X, output Y is produced and the fake repository has state Z, without asserting exact internal method call counts.
- **Guideline**: Follow `.claude/skills/tdd-workflow/SKILL.md` — test behavior, not symbol existence or mock invocation chains.

---

### 10. Silent Nil-Guard for Unwired Dependencies
- **Smell**: An optional or newly added Port dependency is guarded with `if u.port == nil { return nil }` inside business code.
- **Why it hurts**: If the Composition Root forgets to wire the Port in production, the code silently fails to perform its side effects without raising an error (silent degradation root cause addressed in ADR-000928).
- **Fix**: Validate all required dependencies in the constructor and `panic` or raise an error if any dependency is nil. Optional features must be explicitly guarded by configuration flags with loud startup logs.
- **Example in Alt**:
  - `alt-backend/app/orchestrator/usecase/select_lens_usecase/usecase.go:20-35` replaced `if u.clearPort == nil { return nil }` with an explicit constructor check:
    ```go
    if getLens == nil || getVersion == nil || selectPort == nil || clearPort == nil {
        panic("select_lens_usecase: all four knowledge_lens_port dependencies are required and must be wired at composition root (see .claude/rules/di-wiring.md)")
    }
    ```
  - Directly enforces CLAUDE.md Rule 8 and `.claude/rules/di-wiring.md`.

---

### 11. Driver Returns Domain Types
- **Smell**: A `driver/` package imports `domain/` and maps database rows, network responses, or error codes directly to or from Domain entities and sentinel errors.
- **Why it hurts**: It is not a Dependency Rule violation in Uncle Bob's sense (Domain is innermost), but Alt keeps Driver as pure I/O that owns its own row/response types, and the Gateway is the single place that maps Driver types ↔ Domain. A Driver that returns Domain types merges the Gateway's anti-corruption job into I/O code, so a schema or API change leaks straight into the Domain vocabulary. Alt keeps the code clean at all times, so this is enforced, not advised.
- **Fix**: Driver defines its own row struct; Gateway maps row ↔ Domain.
- **Example in Alt**:
  - `alt-backend/app/shared/driver/alt_db/fetch_article_driver.go` and `alt-backend/app/shared/driver/alt_db/save_article_driver.go` import `alt/domain` directly.
  - Search-indexer's `search-indexer/app/driver/backend_api/client.go` and pre-processor's `pre-processor/app/driver/` (e.g. `pre-processor/app/driver/db_articles.go` and `pre-processor/app/driver/summarizer_api.go`) have the same shape.
