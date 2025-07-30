[![Backend Go Tests](https://github.com/Kaikei-e/Alt/actions/workflows/backend-go.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/backend-go.yaml)
[![Playwright Tests](https://github.com/Kaikei-e/Alt/actions/workflows/playwright.yml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/playwright.yml)
[![Unit and Component Tests](https://github.com/Kaikei-e/Alt/actions/workflows/vitest.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/vitest.yaml)
[![Pre-processor Quality Gates](https://github.com/Kaikei-e/Alt/actions/workflows/pre-processor-quality.yml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/pre-processor-quality.yml)
[![Search Indexer Tests](https://github.com/Kaikei-e/Alt/actions/workflows/search-indexer.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/search-indexer.yaml)
[![Tag Generator Tests](https://github.com/Kaikei-e/Alt/actions/workflows/tag-generator.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/tag-generator.yaml)


# Alt - AI-Powered RSS Knowledge Pipeline

> A mobile-first RSS reader built with microservices architecture, featuring AI-powered content enhancement, high-performance logging, and intelligent content discovery.

## Executive Summary

**Alt** is a sophisticated RSS reader platform that transforms traditional feed consumption into an intelligent knowledge discovery system. Built with a modern microservices architecture, Alt combines the simplicity of RSS with the power of artificial intelligence to deliver personalized, enhanced content experiences.

### Key Capabilities

- **📱 Mobile-First Design**: Optimized TypeScript/React frontend with responsive glassmorphism UI
- **🤖 AI Content Enhancement**: ML-powered automatic tagging and LLM-based article summarization
- **⚡ High-Performance Processing**: Go-based backend with Clean Architecture and TDD practices
- **🔍 Intelligent Search**: Meilisearch-powered full-text search with relevance scoring
- **📊 Advanced Analytics**: Rust-based high-performance logging with real-time metrics
- **☁️ Cloud-Native**: Kubernetes-ready with SSL/TLS, auto-scaling, and observability

### Target Audience

- **Knowledge Workers**: Professionals who need to stay informed across multiple domains
- **Researchers**: Academic and industry researchers tracking developments in their fields
- **Content Creators**: Writers, bloggers, and journalists seeking inspiration and trends
- **Technology Enthusiasts**: Developers interested in modern microservices architecture

---

## Features & Capabilities

### Core Features

#### 📰 Intelligent Content Aggregation
- **RSS Feed Management**: Subscribe to unlimited RSS feeds with automatic discovery
- **Duplicate Detection**: Advanced algorithms prevent content duplication across feeds
- **Language Detection**: Multi-language support with automatic language identification

#### 🧠 AI-Powered Enhancement
- **Automatic Tagging**: Machine learning models generate contextually relevant tags
- **Content Summarization**: LLM-powered summaries using Ollama (Gemma3:4b model)
- **Topic Classification**: Articles automatically categorized by subject matter

#### 🔍 Advanced Search & Discovery
- **Full-Text Search**: Lightning-fast search across all content using Meilisearch
- **Semantic Search**: AI-powered semantic matching beyond keyword search
- **Filter & Sort**: Advanced filtering by date, source, tags, reading status

### Technical Features

#### 🏗️ Modern Architecture
- **Microservices Design**: 8 specialized services with clear boundaries
- **Clean Architecture**: 5-layer pattern ensuring maintainability and testability
- **Event-Driven Communication**: Asynchronous processing with reliable delivery
- **API-First Design**: RESTful APIs with comprehensive OpenAPI documentation

#### 🚀 Performance & Scalability
- **High Throughput**: 100K+ logs/second processing capability
- **Low Latency**: Sub-5ms response times for critical operations
- **Horizontal Scaling**: Kubernetes-native auto-scaling based on demand
- **Efficient Resource Usage**: Optimized memory and CPU consumption

#### 🔒 Security & Reliability
- **Rate Limiting**: Intelligent rate limiting prevents abuse and ensures stability
- **Input Validation**: Comprehensive validation prevents injection attacks
- **Graceful Degradation**: System remains functional during partial failures

---

## Architecture Overview

### System Architecture

```mermaid
graph TB
    subgraph "User Interface"
        UI[Mobile-First Frontend<br/>TypeScript/React/Next.js]
    end

    subgraph "Gateway Layer"
        NGINX[NGINX Reverse Proxy<br/>Load Balancer]
    end

    subgraph "Core Services"
        API[alt-backend<br/>Go/Echo Clean Architecture]
        PREP[pre-processor<br/>Feed Processing]
        TAG[tag-generator<br/>Python ML Service]
        NEWS[news-creator<br/>LLM Summarization]
        SEARCH[search-indexer<br/>Index Management]
    end

    subgraph "Data Layer"
        PG[(PostgreSQL 16<br/>Primary Database)]
        MEILI[(Meilisearch<br/>Search Engine)]
        CLICK[(ClickHouse<br/>Analytics DB)]
    end

    subgraph "Logging Infrastructure"
        FORWARDER[rask-log-forwarder<br/>Rust Sidecar Collectors]
        AGGREGATOR[rask-log-aggregator<br/>Central Log Processor]
    end

    UI --> NGINX
    NGINX --> API
    API --> PG
    API --> MEILI

    PREP --> PG
    PREP --> NEWS
    TAG --> PG
    SEARCH --> PG
    SEARCH --> MEILI

    FORWARDER --> AGGREGATOR
    AGGREGATOR --> CLICK

    classDef frontend fill:#e1f5fe
    classDef backend fill:#f3e5f5
    classDef data fill:#e8f5e8
    classDef logging fill:#fff3e0

    class UI frontend
    class API,PREP,TAG,NEWS,SEARCH backend
    class PG,MEILI,CLICK data
    class FORWARDER,AGGREGATOR logging
```

### Data Processing Pipeline

```mermaid
sequenceDiagram
    participant User
    participant Frontend as alt-frontend
    participant Backend as alt-backend
    participant Preprocessor as pre-processor
    participant TagGen as tag-generator
    participant NewsCreator as news-creator
    participant SearchIndexer as search-indexer
    participant Database as PostgreSQL
    participant Search as Meilisearch

    User->>Frontend: Subscribe to RSS feed
    Frontend->>Backend: POST /api/feeds
    Backend->>Database: Store feed URL
    Backend->>Preprocessor: Trigger processing

    Preprocessor->>Preprocessor: Fetch RSS content
    Preprocessor->>Preprocessor: Clean HTML, detect language
    Preprocessor->>Preprocessor: Quality scoring
    Preprocessor->>Database: Store articles

    Preprocessor->>TagGen: Request tag generation
    TagGen->>TagGen: ML-based tag extraction
    TagGen->>Database: Store tags

    Preprocessor->>NewsCreator: Request summarization
    NewsCreator->>NewsCreator: LLM processing (Gemma3:4b)
    NewsCreator->>Database: Store summaries

    SearchIndexer->>Database: Query new content
    SearchIndexer->>Search: Update search indexes

    Frontend->>Backend: GET /api/feeds/enhanced
    Backend->>Database: Fetch enhanced articles
    Backend->>Frontend: Return enriched content
    Frontend->>User: Display enhanced articles
```

---

## Microservice Architecture

### Service Responsibilities

#### 🎯 alt-backend (Go/Echo)
**Primary API Gateway & Business Logic**

- **Technology**: Go 1.23+, Echo framework, Clean Architecture (5-layer)
- **Responsibilities**:
  - RESTful API endpoints for frontend communication
  - User feed management and subscription handling
  - Content aggregation and presentation logic
  - Authentication and authorization (future)
  - Rate limiting and request validation
- **Architecture Pattern**: REST → Usecase → Port → Gateway → Driver
- **Key Features**:
  - CSRF protection on state-changing endpoints
  - Structured logging with `slog`
  - Comprehensive test coverage (>80%)
  - Clean separation of concerns

```mermaid
graph LR
    subgraph "alt-backend Clean Architecture"
        REST[REST Layer<br/>HTTP Handlers]
        USE[Usecase Layer<br/>Business Logic]
        PORT[Port Layer<br/>Interfaces]
        GATE[Gateway Layer<br/>Anti-Corruption]
        DRIVER[Driver Layer<br/>External Systems]

        REST --> USE
        USE --> PORT
        PORT --> GATE
        GATE --> DRIVER
    end
```

#### 🔄 pre-processor (Go)
**RSS Feed Processing & Content Extraction**

- **Technology**: Go 1.23+, Custom HTML parser, Quality scoring algorithms
- **Responsibilities**:
  - RSS feed fetching with configurable intervals
  - HTML content cleaning and sanitization
  - Language detection and content normalization
  - Quality scoring using readability metrics
  - Content deduplication across feeds
- **Performance**: Handles 1000+ feeds with batched processing
- **Error Handling**: Comprehensive retry logic with exponential backoff

#### 🏷️ tag-generator (Python)
**ML-Powered Content Classification**

- **Technology**: Python 3.13, scikit-learn, UV package manager
- **Responsibilities**:
  - Automatic tag generation using ML models
  - Content classification and topic modeling
  - Multi-language text processing
  - Feature extraction from article content
  - Tag relevance scoring and filtering
- **ML Pipeline**: TF-IDF → Feature Extraction → Multi-class Classification
- **Development**: TDD with pytest, comprehensive model validation

#### 📝 news-creator (Ollama)
**LLM-Based Content Summarization**

- **Technology**: Ollama runtime, Gemma3:4b model, GPU acceleration
- **Responsibilities**:
  - Article summarization using large language models
  - Content quality assessment and filtering
  - Context-aware summary generation
  - Multi-format output (brief, detailed, bullet points)
- **Performance**: GPU-optimized with NVIDIA runtime support
- **Scaling**: Model caching and efficient batch processing

#### 🔍 search-indexer (Go)
**Search Index Management**

- **Technology**: Go 1.23+, Meilisearch client, Clean Architecture
- **Responsibilities**:
  - Real-time search index updates
  - Content synchronization with database
  - Search relevance optimization
  - Index health monitoring and maintenance
- **Features**: Incremental indexing, faceted search, typo tolerance

#### 🎨 alt-frontend (TypeScript/React)
**Mobile-First User Interface**

- **Technology**: TypeScript, React, Next.js (Pages Router), Chakra UI
- **Responsibilities**:
  - Responsive mobile-first user interface
  - Real-time content updates via SSE
  - Glassmorphism design system
- **Performance**: Virtual scrolling, lazy loading, optimized bundling
- **Testing**: Playwright for E2E, Vitest for unit tests

#### 📊 rask-log-forwarder (Rust)
**High-Performance Log Collection**

- **Technology**: Rust 1.87+, SIMD JSON parsing, Lock-free data structures
- **Responsibilities**:
  - Zero-copy log collection from Docker containers
  - SIMD-accelerated JSON parsing (>4GB/s throughput)
  - Service-aware log enrichment
  - Reliable delivery with disk fallback
- **Architecture**: Sidecar pattern with one forwarder per service
- **Performance**: >100K logs/second, <16MB memory per instance

#### 🏪 rask-log-aggregator (Rust/Axum)
**Centralized Log Processing**

- **Technology**: Rust 1.87+, Axum web framework, ClickHouse client
- **Responsibilities**:
  - Central log aggregation and processing
  - Real-time analytics and metrics generation
  - Log storage in ClickHouse for analytics
  - System health monitoring and alerting
- **Capabilities**: Stream processing, data compression, query optimization

---

## Data Flow & Processing

### RSS Content Enhancement Pipeline

```mermaid
flowchart TD
    START([RSS Feed URL]) --> FETCH[Fetch RSS Content]
    FETCH --> PARSE[Parse RSS XML]
    PARSE --> EXTRACT[Extract Articles]

    EXTRACT --> CLEAN[HTML Cleaning]
    CLEAN --> LANG[Language Detection]
    LANG --> QUALITY[Quality Scoring]

    QUALITY --> STORE_RAW[(Store Raw Articles)]

    STORE_RAW --> TAG_ML[ML Tag Generation]
    TAG_ML --> STORE_TAGS[(Store Tags)]

    STORE_RAW --> LLM_SUM[LLM Summarization]
    LLM_SUM --> STORE_SUM[(Store Summaries)]

    STORE_TAGS --> INDEX[Search Indexing]
    STORE_SUM --> INDEX
    INDEX --> SEARCH_DB[(Meilisearch)]

    SEARCH_DB --> API_SERVE[API Endpoint]
    API_SERVE --> FRONTEND[Frontend Display]
    FRONTEND --> USER([End User])

    classDef process fill:#e3f2fd
    classDef storage fill:#e8f5e8
    classDef ai fill:#fce4ec
    classDef endpoint fill:#fff3e0

    class FETCH,PARSE,EXTRACT,CLEAN,LANG,QUALITY process
    class STORE_RAW,STORE_TAGS,STORE_SUM,SEARCH_DB storage
    class TAG_ML,LLM_SUM ai
    class API_SERVE,FRONTEND,USER endpoint
```

### AI Enhancement Workflow

```mermaid
graph TB
    subgraph "Content Input"
        ARTICLE[Raw Article Content]
        META[Metadata & Context]
    end

    subgraph "Tag Generation Pipeline"
        PREP_TAG[Text Preprocessing]
        FEAT_EXT[Feature Extraction]
        ML_MODEL[ML Classification Model]
        TAG_FILTER[Tag Filtering & Ranking]
    end

    subgraph "Summarization Pipeline"
        PREP_SUM[Content Preparation]
        LLM[Gemma3:4b Model]
        SUM_POST[Summary Post-processing]
    end

    subgraph "Quality Assessment"
        READ_SCORE[Readability Scoring]
        REL_SCORE[Relevance Scoring]
        FINAL_SCORE[Final Quality Score]
    end

    subgraph "Output"
        ENHANCED[Enhanced Article]
        TAGS_OUT[Generated Tags]
        SUMMARY[Article Summary]
        SCORE[Quality Metrics]
    end

    ARTICLE --> PREP_TAG
    ARTICLE --> PREP_SUM
    ARTICLE --> READ_SCORE

    META --> FEAT_EXT
    META --> REL_SCORE

    PREP_TAG --> FEAT_EXT
    FEAT_EXT --> ML_MODEL
    ML_MODEL --> TAG_FILTER
    TAG_FILTER --> TAGS_OUT

    PREP_SUM --> LLM
    LLM --> SUM_POST
    SUM_POST --> SUMMARY

    READ_SCORE --> FINAL_SCORE
    REL_SCORE --> FINAL_SCORE
    FINAL_SCORE --> SCORE

    TAGS_OUT --> ENHANCED
    SUMMARY --> ENHANCED
    SCORE --> ENHANCED
```

---

## Deployment Architecture

### Kubernetes Architecture

```mermaid
graph TB
    subgraph "Ingress Layer"
        ING[NGINX Ingress Controller]
        ING_EXT[External NGINX Ingress]
    end

    subgraph "alt-apps Namespace"
        subgraph "Frontend Services"
            FE_POD[alt-frontend Pods]
            FE_SVC[Frontend Service]
        end

        subgraph "Backend Services"
            BE_POD[alt-backend Pods]
            BE_SVC[Backend Service]
        end

        subgraph "Processing Services"
            PREP_POD[pre-processor Pods]
            TAG_POD[tag-generator Pods]
            NEWS_POD[news-creator Pods]
            SEARCH_POD[search-indexer Pods]
        end
    end

    subgraph "alt-database Namespace"
        PG_STS[PostgreSQL StatefulSet]
        PG_SVC[PostgreSQL Service]
        PG_PVC[Persistent Volume]
    end

    subgraph "alt-search Namespace"
        MEILI_STS[Meilisearch StatefulSet]
        MEILI_SVC[Meilisearch Service]
        MEILI_PVC[Search Volume]
    end

    subgraph "alt-observability Namespace"
        LOG_AGG[rask-log-aggregator]
        CLICK_STS[ClickHouse StatefulSet]
        LOG_FORWARDERS[Log Forwarder Sidecars]
    end

    ING --> FE_SVC
    ING --> BE_SVC
    FE_SVC --> FE_POD
    BE_SVC --> BE_POD

    BE_POD --> PG_SVC
    PREP_POD --> PG_SVC
    TAG_POD --> PG_SVC
    SEARCH_POD --> PG_SVC
    SEARCH_POD --> MEILI_SVC

    PG_SVC --> PG_STS
    PG_STS --> PG_PVC
    MEILI_SVC --> MEILI_STS
    MEILI_STS --> MEILI_PVC

    LOG_FORWARDERS -.-> LOG_AGG
    LOG_AGG --> CLICK_STS

    classDef ingress fill:#e1f5fe
    classDef app fill:#f3e5f5
    classDef data fill:#e8f5e8
    classDef observability fill:#fff3e0

    class ING,ING_EXT ingress
    class FE_POD,BE_POD,PREP_POD,TAG_POD,NEWS_POD,SEARCH_POD app
    class PG_STS,MEILI_STS,PG_PVC,MEILI_PVC data
    class LOG_AGG,CLICK_STS,LOG_FORWARDERS observability
```

### Docker Compose Services

```mermaid
graph TB
    subgraph "Load Balancer"
        NGINX[nginx:latest<br/>Port 80]
    end

    subgraph "Frontend"
        FRONTEND[alt-frontend<br/>Next.js:3000]
    end

    subgraph "Backend Services"
        BACKEND[alt-backend<br/>Go:9000]
        PREPROCESSOR[pre-processor<br/>Go:9200]
        TAGGER[tag-generator<br/>Python:9400]
        CREATOR[news-creator<br/>Ollama:11434]
        INDEXER[search-indexer<br/>Go:9300]
    end

    subgraph "Data Stores"
        POSTGRES[(PostgreSQL 16<br/>Port 5432)]
        MEILISEARCH[(Meilisearch<br/>Port 7700)]
        CLICKHOUSE[(ClickHouse<br/>Port 8123)]
    end

    subgraph "Logging (Optional Profile)"
        AGGREGATOR[rask-log-aggregator<br/>Rust:9600]
        FE_LOGS[alt-frontend-logs<br/>Sidecar]
        BE_LOGS[alt-backend-logs<br/>Sidecar]
        PREP_LOGS[pre-processor-logs<br/>Sidecar]
        TAG_LOGS[tag-generator-logs<br/>Sidecar]
        NEWS_LOGS[news-creator-logs<br/>Sidecar]
        SEARCH_LOGS[search-indexer-logs<br/>Sidecar]
        MEILI_LOGS[meilisearch-logs<br/>Sidecar]
        DB_LOGS[db-logs<br/>Sidecar]
    end

    NGINX --> FRONTEND
    NGINX --> BACKEND

    BACKEND --> POSTGRES
    BACKEND --> MEILISEARCH
    PREPROCESSOR --> POSTGRES
    PREPROCESSOR --> CREATOR
    TAGGER --> POSTGRES
    INDEXER --> POSTGRES
    INDEXER --> MEILISEARCH

    FE_LOGS -.-> AGGREGATOR
    BE_LOGS -.-> AGGREGATOR
    PREP_LOGS -.-> AGGREGATOR
    TAG_LOGS -.-> AGGREGATOR
    NEWS_LOGS -.-> AGGREGATOR
    SEARCH_LOGS -.-> AGGREGATOR
    MEILI_LOGS -.-> AGGREGATOR
    DB_LOGS -.-> AGGREGATOR
    AGGREGATOR --> CLICKHOUSE

    classDef proxy fill:#e1f5fe
    classDef frontend fill:#e8f5e8
    classDef backend fill:#f3e5f5
    classDef data fill:#fff3e0
    classDef logging fill:#fce4ec

    class NGINX proxy
    class FRONTEND frontend
    class BACKEND,PREPROCESSOR,TAGGER,CREATOR,INDEXER backend
    class POSTGRES,MEILISEARCH,CLICKHOUSE data
    class AGGREGATOR,FE_LOGS,BE_LOGS,PREP_LOGS,TAG_LOGS,NEWS_LOGS,SEARCH_LOGS,MEILI_LOGS,DB_LOGS logging
```

---

## Technology Stack

### Programming Languages & Versions

| Language | Version | Usage | Key Features |
|----------|---------|--------|--------------|
| **Go** | 1.23+ | Backend services, processing | Generics, improved performance, structured logging |
| **TypeScript** | Latest | Frontend development | Type safety, modern ES features |
| **Python** | 3.13 | ML/AI services | Modern async, improved performance |
| **Rust** | 1.87+ (2024 edition) | High-performance logging | SIMD, zero-cost abstractions |

### Frameworks & Libraries

#### Backend (Go)
- **Echo v4**: High-performance HTTP framework
- **GORM**: ORM with PostgreSQL driver
- **gomock**: Mock generation for testing
- **slog**: Structured logging (stdlib)
- **testify**: Testing assertions and suites

#### Frontend (TypeScript)
- **Next.js**: React framework with Pages Router
- **React**: UI library with hooks
- **Chakra UI**: Component library with theming
- **Playwright**: End-to-end testing
- **Vitest**: Unit testing framework

#### ML/AI (Python)
- **UV**: Modern Python package manager
- **scikit-learn**: Machine learning library
- **transformers**: Hugging Face transformers
- **FastAPI**: API framework (if needed)
- **pytest**: Testing framework

#### Logging (Rust)
- **Tokio**: Async runtime
- **Axum**: Web framework
- **SIMD-JSON**: High-performance JSON parsing
- **Bollard**: Docker API client
- **ClickHouse**: Database client

### Infrastructure & Databases

#### Databases
- **PostgreSQL 16**: Primary relational database with SSL/TLS
- **Meilisearch v1.15.2**: Full-text search engine
- **ClickHouse 25.6**: Analytics database for logs

#### Container & Orchestration
- **Docker**: Containerization with multi-stage builds
- **Docker Compose**: Local development environment
- **Kubernetes**: Production orchestration
- **Kustomize**: Configuration management

#### Networking & Security
- **NGINX**: Reverse proxy and load balancer
- **SSL/TLS**: End-to-end encryption
- **Let's Encrypt**: Automated certificate management
- **CORS**: Cross-origin resource sharing

### Development Tools & Practices

#### Code Quality
- **TDD**: Test-driven development across all services
- **Clean Architecture**: Layered architecture in Go services
- **ESLint/Prettier**: TypeScript code formatting
- **Ruff**: Python linting and formatting
- **Clippy**: Rust linting

#### CI/CD & DevOps
- **Git**: Version control with conventional commits
- **GitHub Actions**: Continuous integration
- **Docker Registry**: Container image storage
- **Kubernetes**: Automated deployment and scaling

#### Monitoring & Observability
- **Structured Logging**: JSON logs across all services
- **Metrics Collection**: Performance and business metrics
- **Health Checks**: Service health monitoring
- **Distributed Tracing**: Request tracing (future)

---

## Key Design Patterns

### Clean Architecture Implementation

Alt's backend services follow Uncle Bob's Clean Architecture principles with a 5-layer variant:

```mermaid
graph TD
    subgraph "Clean Architecture Layers"
        subgraph "External Layer"
            REST[REST Layer<br/>HTTP Handlers, Routing]
        end

        subgraph "Application Layer"
            USECASE[Usecase Layer<br/>Business Logic Orchestration]
        end

        subgraph "Interface Layer"
            PORT[Port Layer<br/>Interface Definitions]
        end

        subgraph "Infrastructure Layer"
            GATEWAY[Gateway Layer<br/>Anti-Corruption Layer]
        end

        subgraph "Framework Layer"
            DRIVER[Driver Layer<br/>External Systems, DBs, APIs]
        end
    end

    REST --> USECASE
    USECASE --> PORT
    PORT --> GATEWAY
    GATEWAY --> DRIVER

    classDef external fill:#e3f2fd
    classDef application fill:#f3e5f5
    classDef interface fill:#e8f5e8
    classDef infrastructure fill:#fff3e0
    classDef framework fill:#fce4ec

    class REST external
    class USECASE application
    class PORT interface
    class GATEWAY infrastructure
    class DRIVER framework
```

#### Layer Responsibilities

1. **REST Layer**: HTTP request/response handling, input validation, error responses
2. **Usecase Layer**: Business logic orchestration, workflow coordination
3. **Port Layer**: Interface definitions, contracts between layers
4. **Gateway Layer**: Anti-corruption layer, external service translation
5. **Driver Layer**: Technical implementations, database access, API clients

### Test-Driven Development (TDD)

All services follow strict TDD practices with the Red-Green-Refactor cycle:

```mermaid
flowchart LR
    RED[🔴 RED<br/>Write Failing Test] --> GREEN[🟢 GREEN<br/>Write Minimal Code]
    GREEN --> REFACTOR[🔄 REFACTOR<br/>Improve Design]
    REFACTOR --> RED

    classDef red fill:#ffebee
    classDef green fill:#e8f5e8
    classDef refactor fill:#e3f2fd

    class RED red
    class GREEN green
    class REFACTOR refactor
```

#### Testing Strategy

- **Unit Tests**: >80% coverage for usecase and gateway layers
- **Integration Tests**: End-to-end workflow testing
- **Performance Tests**: Load testing for critical paths
- **Contract Tests**: API contract validation

### Microservice Communication Patterns

#### Synchronous Communication
- **REST APIs**: Service-to-service communication
- **HTTP Keep-Alive**: Connection pooling for performance
- **Circuit Breakers**: Failure isolation and recovery

#### Asynchronous Communication
- **Event-Driven**: Database triggers for workflow initiation
- **Message Queues**: Future implementation for scalability
- **Batch Processing**: Efficient bulk operations

### Logging and Observability Strategy

#### Sidecar Logging Pattern
Each service has a dedicated Rust-based log forwarder running as a sidecar container:

```mermaid
graph LR
    subgraph "Service Pod"
        APP[Application Container]
        SIDECAR[Log Forwarder Sidecar]
    end

    subgraph "Shared Resources"
        LOGS[Docker Logs Volume]
        NETWORK[Shared Network Namespace]
    end

    APP --> LOGS
    SIDECAR --> LOGS
    SIDECAR --> NETWORK

    classDef app fill:#e3f2fd
    classDef sidecar fill:#fff3e0
    classDef shared fill:#e8f5e8

    class APP app
    class SIDECAR sidecar
    class LOGS,NETWORK shared
```

#### Benefits of Sidecar Pattern
- **Isolation**: Log forwarder failures don't affect application
- **Performance**: Zero-copy log processing with SIMD acceleration
- **Scalability**: Each service scales independently
- **Flexibility**: Service-specific log processing rules

---

## Development Practices

### Code Quality Standards

#### Go Services
- **Structured Logging**: All logs use `slog` with context
- **Error Handling**: Comprehensive error wrapping with `fmt.Errorf`
- **Code Coverage**: Minimum 80% for business logic layers
- **Static Analysis**: `go vet`, `golangci-lint` for code quality

#### TypeScript Frontend
- **Type Safety**: Strict TypeScript configuration
- **Component Testing**: Playwright for E2E, Vitest for units
- **Performance**: Bundle optimization, lazy loading
- **Accessibility**: WCAG 2.1 AA compliance

#### Python ML Services
- **Type Hints**: Comprehensive type annotations
- **Testing**: pytest with fixtures and mocking
- **Code Quality**: Ruff for linting and formatting
- **Package Management**: UV for fast dependency resolution

#### Rust Logging Services
- **Memory Safety**: Zero unsafe code blocks
- **Performance**: SIMD optimizations, lock-free data structures
- **Error Handling**: `thiserror` and `anyhow` for error management
- **Testing**: Property-based testing with quickcheck

### Security Practices

#### Application Security
- **Input Validation**: All external inputs validated at entry points
- **SQL Injection Prevention**: Parameterized queries only
- **CSRF Protection**: Token-based protection for state changes
- **Content Security Policy**: Strict CSP headers

#### Infrastructure Security
- **TLS Everywhere**: All communications encrypted
- **Secret Management**: Environment variables, no hardcoded secrets
- **Network Segmentation**: Kubernetes network policies
- **Minimal Attack Surface**: Alpine-based container images

#### Operational Security
- **Principle of Least Privilege**: Service-specific database users
- **Audit Logging**: Comprehensive audit trails
- **Security Updates**: Automated dependency updates
- **Penetration Testing**: Regular security assessments

---


## 📈 Roadmap

### Planned Features
- [ ] Multi-user support with authentication
- [ ] Advanced filtering and saved searches
- [ ] Export functionality (OPML, JSON)
- [ ] Webhook notifications
- [ ] GraphQL API option
- [ ] Kubernetes deployment manifests

### Performance Goals
- Sub-100ms API response times
- Support for 10,000+ feeds
- Real-time updates via WebSocket
- Horizontal scaling capabilities

## 📄 License

This project is licensed under the Apache 2.0 License - see the LICENSE file for details.

## 🙏 Acknowledgments

- Built with inspiration from Clean Architecture principles by Robert C. Martin
- Powered by amazing open-source projects: Go, Rust, TypeScriptm, Echo, React, Next.js, PostgreSQL, Meilisearch, ClickHouse, Ollama
- Special thanks to the RSS community for keeping web feeds alive

---

For more detailed documentation, visit our [Wiki](https://github.com/yourusername/alt/wiki) or check the `docs/` directory.
```mermaid
graph TB
    %% Style definitions
    classDef frontend fill:#4a90e2,stroke:#2e5aa8,stroke-width:3px,color:#fff
    classDef backend fill:#50c878,stroke:#3aa860,stroke-width:3px,color:#fff
    classDef helper fill:#f39c12,stroke:#d68910,stroke-width:3px,color:#fff
    classDef database fill:#e74c3c,stroke:#c0392b,stroke-width:3px,color:#fff
    classDef logging fill:#9b59b6,stroke:#7d3c98,stroke-width:3px,color:#fff
    classDef external fill:#34495e,stroke:#2c3e50,stroke-width:3px,color:#fff
    classDef layer fill:#ecf0f1,stroke:#bdc3c7,stroke-width:2px,color:#2c3e50

    %% User Interface Layer
    subgraph UI["User Interface"]
        User[👤 User]
        Mobile[📱 Mobile Device]
        Desktop[💻 Desktop]
    end

    %% Nginx Reverse Proxy
    Nginx[nginx<br/>Reverse Proxy]:::external

    %% Frontend
    subgraph FE["Frontend Layer"]
        Frontend[alt-frontend<br/>TypeScript/React/Next.js<br/>Mobile-first Design]:::frontend
    end

    %% Main Backend - 5-Layer Clean Architecture
    subgraph Backend["alt-backend - 5-Layer Clean Architecture"]
        REST[REST Handler<br/>HTTP Handling]:::layer
        Usecase[Usecase<br/>Business Logic]:::layer
        Port[Port<br/>Interface Definitions]:::layer
        Gateway[Gateway ACL<br/>Anti-Corruption Layer]:::layer
        Driver[Driver<br/>External Integrations]:::layer

        REST --> Usecase
        Usecase --> Port
        Port --> Gateway
        Gateway --> Driver
    end

    %% Data Processing Pipeline
    subgraph Pipeline["Data Processing Pipeline"]
        PreProcessor[pre-processor<br/>Go<br/>Data Preprocessing<br/>Language Detection<br/>Auto-scoring]:::helper
        TagGenerator[tag-generator<br/>Python<br/>ML-based Tag Generation]:::helper
        NewsCreator[news-creator<br/>LLM Gemma3:4b<br/>Content Generation<br/>Summarization]:::helper
        SearchIndexer[search-indexer<br/>Go<br/>Search Index Management]:::helper
    end

    %% Logging Infrastructure
    subgraph LogInfra["Logging Infrastructure"]
        LogForwarder[rask-log-forwarder<br/>Rust<br/>Log Forwarding]:::logging
        LogAggregator[rask-log-aggregator<br/>Rust<br/>Log Aggregation<br/>Analytics]:::logging
    end

    %% Data Stores
    subgraph DataStore["Data Store Layer"]
        PostgreSQL[(PostgreSQL<br/>Primary Data Store)]:::database
        Meilisearch[(Meilisearch<br/>Full-text Search Engine)]:::database
        ClickHouse[(ClickHouse<br/>Analytics Database)]:::database
    end

    %% External Services
    Ollama[Ollama<br/>LLM Runtime]:::external
    RSSFeeds[RSS Feeds<br/>External RSS Sources]:::external

    %% Connection Relationships - User Flow
    User --> Mobile
    User --> Desktop
    Mobile --> Nginx
    Desktop --> Nginx
    Nginx --> Frontend
    Frontend --> REST

    %% Backend Internal Flow
    Driver --> PostgreSQL
    Driver --> PreProcessor
    Driver --> SearchIndexer

    %% Data Processing Flow
    PreProcessor --> TagGenerator
    PreProcessor --> NewsCreator
    TagGenerator --> PostgreSQL
    NewsCreator --> PostgreSQL
    NewsCreator -.-> Ollama
    SearchIndexer --> Meilisearch

    %% Logging Flow
    REST -.-> LogForwarder
    PreProcessor -.-> LogForwarder
    TagGenerator -.-> LogForwarder
    NewsCreator -.-> LogForwarder
    SearchIndexer -.-> LogForwarder
    LogForwarder --> LogAggregator
    LogAggregator --> ClickHouse

    %% External Data Sources
    Driver --> RSSFeeds
```
