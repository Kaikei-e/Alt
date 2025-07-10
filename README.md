# Alt

**A mobile-first, AI-powered RSS knowledge pipeline built with microservices and Clean Architecture**

Alt is a modern RSS reader that transforms raw feeds into enriched, searchable knowledge. It fetches RSS feeds, cleans and analyzes articles, generates AI summaries and tags, indexes everything for blazing-fast search, and serves it all through an elegant mobile-first interface. The entire system is containerized, allowing each component to be scaled, replaced, or enhanced independently.

## 🎯 Core Features

- **Intelligent Feed Processing**: Automatically fetches, deduplicates, and enriches RSS feeds with language detection and quality scoring
- **AI-Powered Summarization**: Uses LLM (Gemma3:4b) to generate concise summaries and score their quality
- **Machine Learning Tagging**: Automatically categorizes articles with ML-generated tags for better discoverability
- **Lightning-Fast Search**: Full-text search powered by Meilisearch with faceted filtering
- **Mobile-First Design**: Responsive UI optimized for mobile devices with cursor-based pagination
- **Real-Time Monitoring**: Comprehensive logging infrastructure with Rust-based high-performance log aggregation
- **Test-Driven Development**: Every component built with TDD principles ensuring reliability and maintainability
- **Clean Architecture**: Five-layer architecture pattern keeping business logic independent from technical details

## 🏗️ Architecture Overview

Alt follows a microservices architecture where each service has a specific responsibility and communicates via well-defined APIs. The main backend service (`alt-backend`) implements a five-layer Clean Architecture pattern:

```
REST Handler → Usecase → Port → Gateway (ACL) → Driver
```

This design ensures:
- Business logic remains independent of frameworks and external services
- Easy testing through dependency injection and interface segregation
- Flexibility to change technical implementations without affecting core logic
- Clear separation of concerns across different layers

## 📦 Service Components

### Core Services

| Service | Technology | Responsibility |
|---------|------------|----------------|
| **alt-frontend** | TypeScript/React/Next.js | Mobile-first web UI with responsive design |
| **alt-backend** | Go/Echo | Main API server following Clean Architecture |
| **nginx** | Nginx | Reverse proxy and load balancer |

### Data Processing Pipeline

| Service | Technology | Responsibility |
|---------|------------|----------------|
| **pre-processor** | Go | Fetches feeds, cleans HTML, detects language, scores content |
| **tag-generator** | Python/ML | Generates contextual tags using machine learning |
| **news-creator** | Ollama/LLM | Creates AI-powered summaries using Gemma3:4b |
| **search-indexer** | Go | Manages Meilisearch indexes for fast searching |

### Data Stores

| Service | Technology | Purpose |
|---------|------------|---------|
| **db** | PostgreSQL | Primary relational database for all application data |
| **meilisearch** | Meilisearch | Full-text search engine with typo tolerance |
| **clickhouse** | ClickHouse | High-performance analytics database for logs |

### Logging Infrastructure

| Service | Technology | Purpose |
|---------|------------|---------|
| **rask-log-forwarder** | Rust | Ultra-fast sidecar for log collection (one per service) |
| **rask-log-aggregator** | Rust/Axum | Central log processing and analytics |

## 🔄 Data Flow

The system processes RSS feeds through a sophisticated pipeline:

### 1. Feed Registration
Users submit RSS feed URLs through the mobile-first frontend. The backend validates and stores these URLs in PostgreSQL.

### 2. Article Fetching & Processing
```mermaid
graph LR
    A[RSS Feeds] --> B[pre-processor]
    B --> C[Article Extraction]
    C --> D[Language Detection]
    D --> E[Quality Scoring]
    E --> F[PostgreSQL]
```

The pre-processor periodically fetches new articles from registered feeds, extracts clean content, detects the language, and scores the quality before storing in the database.

### 3. AI Enhancement
```mermaid
graph LR
    A[Raw Articles] --> B[news-creator]
    B --> C[LLM Processing]
    C --> D[Summary Generation]
    D --> E[Quality Check]
    E --> F[PostgreSQL]
    
    A --> G[tag-generator]
    G --> H[ML Model]
    H --> I[Tag Generation]
    I --> F
```

Articles are enhanced with AI-generated summaries and ML-based tags, making them more discoverable and easier to consume.

### 4. Search Indexing
```mermaid
graph LR
    A[Enhanced Articles] --> B[search-indexer]
    B --> C[Data Transformation]
    C --> D[Meilisearch]
    D --> E[Indexed Content]
```

The search-indexer continuously monitors for new or updated articles and indexes them in Meilisearch for instant full-text search.

### 5. User Interface
The frontend provides a clean, mobile-optimized interface for:
- Browsing feeds with infinite scroll
- Viewing article summaries
- Searching across all content
- Managing read status
- Accessing detailed statistics

## 🚀 Getting Started

### Prerequisites
- Docker and Docker Compose
- 8GB RAM minimum (for running all services)
- 8GB VRAM minimum (for AI summarization)
- 20GB disk space

### Quick Start

1. Clone the repository:
```bash
git clone https://github.com/yourusername/alt.git
cd alt
```

2. Set up environment variables:
```bash
cp .env.example .env
# Edit .env with your configuration
```

3. Start all services(No logging):
```bash
docker compose up -d
```

With Rask Loggers
```bash
docker compose --profile logging up --build -d
```

4. Access the application:
- Frontend: http://localhost:3000
- Backend API: http://localhost:8080
- Meilisearch: http://localhost:7700

### Development Setup

For development with hot-reloading:

```bash
# Start core services only
docker compose up db meilisearch ollama

# Run backend locally
cd alt-backend/app
go run main.go

# Run frontend locally
cd alt-frontend/app
pnpm install
pnpm dev
```

## 🧪 Testing

Every component is built with Test-Driven Development (TDD):

### Backend Testing (Go)
```bash
cd alt-backend/app
go test ./... -v
```

### Frontend Testing (TypeScript)
```bash
cd alt-frontend/app
pnpm test:all  # Runs both Playwright and Vitest
```

### Python Services Testing
```bash
cd tag-generator/app
uv run pytest
```

## 🔧 Configuration

### Environment Variables

Key configuration options:

```bash
# Database
DATABASE_URL=postgresql://user:pass@localhost:5432/alt

# Search
MEILI_MASTER_KEY=your-master-key
MEILI_HOST=http://meilisearch:7700

# AI/LLM
OLLAMA_HOST=http://ollama:11434
LLM_MODEL=gemma3:4b

# Logging
LOG_LEVEL=info
USE_RASK_LOGGER=true
```

### Service Configuration

Each service can be configured independently:
- `alt-backend/config/config.go` - Backend configuration
- `alt-frontend/.env.local` - Frontend environment
- `pre-processor/config.yaml` - Processor settings
- `tag-generator/settings.py` - ML pipeline config

## 📊 Monitoring & Observability

### Logging Architecture

Every service outputs structured logs that are collected by Rust-based forwarders:

```
Service → stdout/stderr → rask-log-forwarder → rask-log-aggregator → ClickHouse
```

Features:
- Zero-copy log collection
- SIMD-accelerated JSON parsing
- Lock-free buffering
- Guaranteed delivery with disk fallback

### Health Checks

All services expose health endpoints:
- `/health` - Basic health status
- `/health/ready` - Readiness probe
- `/health/live` - Liveness probe

## 🔐 Security

### API Security
- CSRF protection on all state-changing endpoints
- Rate limiting to prevent abuse
- Input validation and sanitization
- Secure headers (CSP, HSTS, etc.)

### Data Security
- All external URLs validated against security rules
- HTML content sanitized before storage
- Database connections use SSL
- Secrets managed via environment variables

## 🤝 Contributing

We follow strict development guidelines:

1. **Test First**: Write failing tests before implementation
2. **Clean Architecture**: Respect layer boundaries
3. **Code Quality**: Use linters and formatters
4. **Documentation**: Update docs with changes
5. **Commit Messages**: Follow conventional commits

### Development Workflow

1. Create feature branch from `main`
2. Write failing tests (RED)
3. Implement minimal code to pass (GREEN)
4. Refactor for clarity (REFACTOR)
5. Ensure all tests pass
6. Submit pull request

## 📚 Technical Deep Dive

### Clean Architecture Implementation

The `alt-backend` service demonstrates our five-layer architecture:

**REST Handler Layer**
- Handles HTTP requests/responses
- Input validation
- Authentication/authorization
- Error response formatting

**Usecase Layer**
- Orchestrates business logic
- Transaction boundaries
- Business rule enforcement
- Use case specific operations

**Port Layer**
- Defines interfaces (contracts)
- Enables dependency inversion
- Isolates business logic from technical details

**Gateway Layer (Anti-Corruption Layer)**
- Translates between domain and external representations
- Protects domain from external changes
- Handles external service communication

**Driver Layer**
- Implements technical integrations
- Database access
- External API calls
- File system operations

### Performance Optimizations

**Backend**
- Connection pooling for database
- Rate limiting for external APIs
- Efficient pagination with cursors
- Concurrent feed processing

**Frontend**
- Code splitting and lazy loading
- Optimistic UI updates
- Prefetching for smooth navigation
- Virtual scrolling for large lists

**Search**
- Incremental indexing
- Typo tolerance
- Faceted search
- Highlight matching terms

**Logging**
- Lock-free data structures
- Zero-copy parsing
- Batch transmission
- Compression for storage

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
