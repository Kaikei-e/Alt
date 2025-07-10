# Alt

*The local-first, composable RSS knowledge pipeline.*

Alt is a self-hosted RSS reader and “content refinery” that fetches feeds, scrubs & tags articles, indexes them for lightning-fast search and serves a clean reading UI – everything running in neatly isolated containers so you can swap parts out or scale them independently.
The project is built **mobile-first**, 100 % open-source (Apache-2.0), and engineered around a five-layer flavour of Clean Architecture with test-driven development at its core.

-----

## Feature Highlights

|Category                      |What you get                                                  |Where it lives   |
|------------------------------|--------------------------------------------------------------|-----------------|
|**Fast crawl**                |Go workers pull and de-duplicate feeds in parallel            |`alt-backend/`   |
|**Readability cleanup**       |Pre-processing & language detect articles for AI summarization|`pre-processor/` |
|**Auto-scoring summaries**    |Using LLM to score summaries and remove bad ones              |`pre-processor/` |
|**Auto-tagging**              |ML tagging via Python                                         |`tag-generator/` |
|**Full-text & faceted search**|Meilisearch via a Go proxy                                    |`search-indexer/`|
|**Auto summarization**        |LLM summariser                                                |`news-creator/`  |
|**One-command up**            |`docker compose up`                                           |`compose.yaml`   |

-----

## Tech Stack

- **Go** for backend services and data processing
- **TypeScript / React / Next.js** for the mobile-first frontend
- **Python** for machine learning tasks (tag generation)
- **Rust** for log forwarding and aggregation (stores logs in ClickHouse)
- **ClickHouse** for high-performance analytical data storage
- **PostgreSQL** as the primary data store
- **Meilisearch** for full-text search
- **Ollama** with the gemma3:4b model for LLM summarization
- **Docker Compose** orchestrates all services

## Service Overview

|Service                |Tech            |Purpose                                                   |
|-----------------------|----------------|----------------------------------------------------------|
|**nginx**              |Nginx           |Reverse proxy for frontend and backend                    |
|**alt-frontend**       |Next.js / React |Web UI with mobile-first design                           |
|**alt-backend**        |Go + Echo       |Fetches RSS feeds, exposes REST API                       |
|**pre-processor**      |Go              |Cleans articles, detects language, scores LLM summaries   |
|**tag-generator**      |Python + KeyBERT|Generates article tags using ML                           |
|**search-indexer**     |Go + Meilisearch|Indexes articles for fast search                          |
|**news-creator**       |Ollama (LLM)    |Summarises and scores content                             |
|**db**                 |PostgreSQL      |Stores all persistent data                                |
|**meilisearch**        |Meilisearch     |Search engine service                                     |
|**rask-log-forwarder** |Rust            |Sidecar that streams logs to aggregator                   |
|**rask-log-aggregator**|Rust + Axum     |Central log processing service (stores logs in ClickHouse)|
|**migrate**            |Go              |Runs database schema migrations                           |

Each service runs in its own container so components can be scaled or swapped independently. The containers communicate over an internal Docker network and can be started with a single `docker compose up` command.

## Project Characteristics

- Embraces a microservice approach where small, focused containers cooperate via HTTP and message passing.
- Clean Architecture principles guide the main Go services, keeping business rules isolated from infrastructure.
- Test-driven development and automated health checks help maintain reliability.
- The system is designed to be composable: you can replace any service (for example, the search engine or tag generator) without affecting the rest of the pipeline.

## Backend Processing Flow

The backend is a pipeline of microservices that work together to fetch, process, and index content from RSS feeds. Each service is a container that communicates with others over the network.

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

### Flow Description

1. **Feed Registration**: A user submits a new RSS feed URL through the **alt-frontend**.
1. **Save Feed URL**: The **alt-backend** receives the URL and saves it to the `feed_links` table in the **PostgreSQL** database.
1. **Article Fetching**: The **pre-processor** service periodically fetches new articles from the registered feed URLs.
1. **Save Articles**: The fetched articles are parsed and stored in the `articles` table in the database.
1. **Article Summarization**: The **pre-processor** sends the content of new articles to the **news-creator** service.
1. **Return Summary**: The **news-creator**, using an LLM (Phi-3-mini), generates a summary and returns it.
1. **Save Summary**: The **pre-processor** saves the summary to the `article_summaries` table.
1. **Tag Generation**: The **tag-generator** service fetches articles that haven’t been tagged yet.
1. **Generate & Save Tags**: It uses an ML model to generate tags from the article’s content and saves them to the `article_tags` and `feed_tags` tables.
1. **Search Indexing**: The **search-indexer** service fetches new and updated articles from the database.
1. **Index Articles**: It sends the article data to **Meilisearch** for indexing.
1. **Frontend API**: The **alt-backend** provides a REST API for the **alt-frontend** to display feeds, articles, and search results.
1. **Search API**: When a user searches, the **alt-backend** queries the **Meilisearch** index and returns the results.
