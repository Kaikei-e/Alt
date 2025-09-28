# Layer 07: Processing

## 1. Responsibilities

The `07-processing` layer is responsible for deploying the entire asynchronous data processing pipeline of the Alt project. It manages a suite of microservices that handle background tasks, including data ingestion, preprocessing, AI/ML-based content enrichment, search indexing, and external service integration.

## 2. Directory Structure

```
/07-processing/
├── charts/                   # Helm charts for processing microservices
│   ├── pre-processor/        # Go service for initial RSS feed processing
│   ├── pre-processor-sidecar/ # CronJob for Inoreader API integration
│   ├── tag-generator/        # Python ML service for tag generation
│   ├── news-creator/         # LLM-based service for content generation
│   ├── search-indexer/       # Go service to index data in MeiliSearch
│   ├── auth-token-manager/   # Deno service for OAuth token management
│   └── rask-log-aggregator/  # (Chart exists but is not deployed via Skaffold)
└── skaffold.yaml             # Skaffold configuration for this layer
```

## 3. Build Artifacts

This layer is responsible for building numerous container images for the processing pipeline:

- **Shared Base Images**: To promote code reuse and consistency, this layer builds shared base images:
  - `kaikei/shared-auth-go`: A base image with common authentication logic for Go services.
  - `kaikei/shared-auth-python`: A base image with common authentication logic for Python services.
- **Microservice Images**:
  - `kaikei/pre-processor`
  - `kaikei/pre-processor-sidecar`
  - `kaikei/search-indexer`
  - `kaikei/tag-generator`
  - `kaikei/news-creator`
  - `kaikei/auth-token-manager`

Note: `kaikei/rask-log-aggregator` is explicitly excluded from the Skaffold build process and is managed via Docker Compose instead.

## 4. Deployed Components

All services in this layer are deployed into the `alt-processing` namespace, creating a dedicated environment for the data pipeline.

| Helm Release              | Chart Path                      | Description                                                                                                                            |
| :------------------------ | :------------------------------ | :------------------------------------------------------------------------------------------------------------------------------------- |
| `pre-processor`           | `charts/pre-processor`          | A Go service that acts as the entry point to the pipeline, fetching and normalizing RSS feeds.                                         |
| `pre-processor-sidecar`   | `charts/pre-processor-sidecar`  | A `CronJob` that performs auxiliary tasks for the pre-processor, such as interacting with the Inoreader API on a schedule.             |
| `search-indexer`          | `charts/search-indexer`         | A Go service that takes processed articles and pushes them into MeiliSearch to make them available for full-text search.               |
| `tag-generator`           | `charts/tag-generator`          | A Python-based ML service that uses NLP to analyze article content and automatically generate relevant tags.                         |
| `news-creator`            | `charts/news-creator`           | An LLM-based service that performs advanced content transformations, such as generating summaries or related articles.                 |
| `auth-token-manager`      | `charts/auth-token-manager`     | A Deno/TypeScript service responsible for securely managing and refreshing OAuth2 access tokens for external services like Inoreader. |

## 5. Configuration Evolution and Strategy

The `skaffold.yaml` in this layer shows clear evidence of a maturing and battle-tested configuration, with comments referencing past incidents driving improvements:

- **Incident-Driven Hardening**: Comments like "INCIDENT 82 FIX" and "INCIDENT 89 FIX" indicate that the build process has been refined over time. This includes standardizing image repository names, enforcing the use of Buildkit for consistent builds, and adopting a unified `gitCommit` tagging policy.
- **Local Development Focus**: The `dev` profile consistently uses `image.pullPolicy: "Never"`, enforcing a workflow where developers test with their locally built images, enabling a tight feedback loop.
- **Centralized Build Context**: All images are built from the root `../../` context, ensuring that any shared files or modules are accessible during the build process.

## 6. Profiles

- **`dev` (Default), `staging`, `prod`**: Standard profiles for different environments, primarily switching between different sets of `values.yaml` files.
- **`schedule-mode`**: A specialized profile for debugging. It deploys the `pre-processor-sidecar` as a long-running `Deployment` instead of a `CronJob`, making it easier to inspect logs, exec into the container, and debug its behavior interactively.