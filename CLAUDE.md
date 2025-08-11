# CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- DO NOT use opus unless explicitly requested -->

## About this Project

This project is a mobile-first RSS reader built with a microservice architecture stack. The codebase follows a five-layer variant of Clean Architecture for the main application.

**Architecture Layers:**
- **REST (handler) → Usecase → Port → Gateway (ACL) → Driver**
- The Gateway acts as the Anti-Corruption Layer that shields the domain from external semantics

Helper applications are built with different architectures for specific purposes, but the entire implementation process is based on Test-Driven Development (TDD).

## Tech Stack Overview

### Core Technologies
- **Languages:** Go (backend), TypeScript (frontend), Python (ML/data processing)
- **Frameworks:** Echo (Go), Next.js/React (TypeScript)
- **Database:** PostgreSQL
- **Search Engine:** Meilisearch
- **LLM:** Gemma3:4b

### Application Stack Details

#### Main Application
- **alt-backend:** Go/Echo - Main backend service following Clean Architecture
- **alt-frontend:** TypeScript/React/Next.js - Mobile-first frontend (intentionally not Clean Architecture)
- **db:** PostgreSQL - Primary data store
- **meilisearch:** Full-text search capabilities

#### Helper Applications
- **pre-processor:** Go - Data preprocessing service
- **search-indexer:** Go - Search index management
- **tag-generator:** Python - ML-based tag generation
- **news-creator:** LLM-based content generation

#### Log Functionality
- **rask-log-forwarder:** Rust - Log forwarder service
- **rask-log-aggregator:** Rust - Log aggregator service

##### Specific CLAUDE.mds

- [alt-backend/app/CLAUDE.md](@alt-backend/app/CLAUDE.md)
- [alt-frontend/app/CLAUDE.md](@alt-frontend/app/CLAUDE.md)
- [pre-processor/app/CLAUDE.md](@pre-processor/app/CLAUDE.md)
- [search-indexer/app/CLAUDE.md](@search-indexer/app/CLAUDE.md)
- [tag-generator/app/CLAUDE.md](@tag-generator/app/CLAUDE.md)
- [news-creator/app/CLAUDE.md](@news-creator/app/CLAUDE.md)
- [rask-log-forwarder/app/CLAUDE.md](@rask-log-forwarder/app/CLAUDE.md)
- [rask-log-aggregator/app/CLAUDE.md](@rask-log-aggregator/app/CLAUDE.md)




## Architecture Design

### Clean Architecture Directory Layout
```
/alt-backend
├─ app/
│  ├─ cmd/           # Application entry points
│  ├─ rest/          # HTTP handlers & routers
│  ├─ usecase/       # Business logic orchestration
│  ├─ port/          # Interface definitions
│  ├─ gateway/       # Anti-corruption layer
│  ├─ driver/        # External integrations
│  ├─ domain/        # Core entities & value objects
│  └─ utils/         # Cross-cutting concerns
```

### Layer Responsibilities

| Layer | Purpose | Dependencies |
|-------|---------|--------------|
| **REST** | HTTP request/response mapping | → Usecase |
| **Usecase** | Business process orchestration | → Port |
| **Port** | Contract definitions | → Gateway |
| **Gateway** | External↔Domain translation | → Driver |
| **Driver** | Technical implementations | None |

## Development Guidelines

### Test-Driven Development (TDD)

**CRITICAL: Follow the Red-Green-Refactor cycle:**
1. **Red:** Write a failing test first. You write the interface or the function signature first that you want to test. And then you write the test.
2. **Green:** Write minimal code to pass the test. You write the code that passes the test.
3. **Refactor:** Improve code quality while keeping tests green. You refactor the code to make it more readable and maintainable.

**Testing Strategy:**
- Test only usecase and gateway layers
- Mock external dependencies using mock library eg. gomock
- Use table-driven tests for comprehensive coverage
- Aim for >80% code coverage in tested layers

### Coding Standards

#### Go Development
- Use `log/slog` for structured logging
- Follow Go idioms and effective Go patterns
- Use `gomock` for mocking (maintained by Uber)
- Apply `gofmt` and `goimports` on every file change
- Use meaningful variable names (avoid single letters except for indices)

#### TypeScript Development
- Use ES6+ module syntax
- Apply ESLint and Prettier configurations
- Follow React hooks best practices
- Implement proper error boundaries

#### Python Development
- Use Python 3.13+
- Follow PEP 8 style guide
- Use type hints for better code clarity
- Implement proper exception handling

### Domain-Driven Design (DDD)

- Design microservices around business capabilities
- Each service should have a single, well-defined responsibility
- Use bounded contexts to define service boundaries
- Minimize inter-service dependencies

### Best Practices

#### Service Design
- **Single Responsibility:** Each microservice handles one business domain
- **API First:** Design APIs before implementation
- **Failure Resilience:** Implement circuit breakers and retries
- **Observability:** Comprehensive logging, metrics, and tracing

#### Data Management
- **Event Sourcing:** For cross-service data consistency
- **CQRS:** Separate read and write models where appropriate

#### Security
- **Secrets Management:** Use environment variables, never hardcode
- **Input Validation:** Validate all external inputs

#### CI/CD
- **Automated Testing:** Run tests on every commit
- **Code Quality Gates:** Enforce linting and coverage thresholds
- **Containerization:** Use Docker for consistent environments
- **Progressive Deployment:** Use feature flags and canary releases

## Local Development Environment

### Kind-based Development Setup

This project uses **kind (Kubernetes IN Docker)** for local development, providing a lightweight Kubernetes environment optimized for rapid iteration.

#### Quick Start
```bash
# 1. Setup development environment (one-time)
./setup-dev-env.sh

# 2. Build and load all images
./auto-build-alt.sh --all

# 3. Deploy services
cd skaffold/05-processing && skaffold run
```

#### Development Commands
- **Environment Setup**: `./setup-dev-env.sh`
- **Build Images**: `./auto-build-alt.sh --all`
- **Deploy Services**: `skaffold run`
- **Check Status**: `kubectl get pods -n alt-processing`
- **View Logs**: `kubectl logs -f <pod-name> -n alt-processing`

#### Kind Cluster Management
```bash
# List clusters
kind get clusters

# Switch context
kubectl config use-context kind-alt-prod

# Cluster info
kubectl cluster-info --context kind-alt-prod

# Delete cluster (if needed)
kind delete cluster --name alt-prod
```

### Database Migration with Atlas

This project uses **Atlas** for modern, Kubernetes-native database migration management with transaction safety and automated deployment integration.

#### Migration System Overview
- **Atlas CLI Integration**: Professional-grade database schema management
- **Helm Pre-upgrade Hooks**: Automatic migration execution before application deployment
- **Transaction Safety**: CONCURRENTLY operations converted to transaction-safe equivalents
- **Local Build**: No external registry dependencies, uses local Docker images
- **Security**: Dedicated RBAC and credentials isolation

#### Atlas Migration Commands
```bash
# Build Atlas migration container
./auto-build-alt.sh --services alt-atlas-migrations

# Test migration syntax (offline)
docker run --rm alt-atlas-migrations:latest syntax-check

# Deploy infrastructure with automatic migrations
cd skaffold/02-infrastructure && skaffold run -p prod
```

#### Migration Development Workflow
1. **Add New Migration**: Create SQL file in `migrations-atlas/migrations/`
   ```sql
   -- 20250812000100_add_new_feature.sql
   -- Migration: Add new feature table
   -- Created: 2025-08-12 00:01:00
   -- Atlas Version: v0.35
   
   CREATE TABLE new_feature (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       name TEXT NOT NULL,
       created_at TIMESTAMP DEFAULT NOW()
   );
   ```

2. **Test Locally**: Validate migration syntax
   ```bash
   docker run --rm alt-atlas-migrations:latest syntax-check
   ```

3. **Deploy**: Migrations run automatically via pre-upgrade hooks
   ```bash
   cd skaffold/02-infrastructure && skaffold run -p prod
   ```

#### Migration Files Location
- **Source**: `migrations-atlas/migrations/`
- **Docker Context**: `migrations-atlas/docker/`
- **Helm Integration**: `skaffold/02-infrastructure/charts/postgres/`

#### Troubleshooting Migrations
```bash
# Check migration job status
kubectl get jobs -n alt-database -l component=atlas-migration

# View migration logs
kubectl logs -n alt-database -l component=atlas-migration

# Manual migration status check (requires running database)
kubectl run atlas-debug --rm -it --image=alt-atlas-migrations:latest -- status
```

## Working with Claude Code

### Effective Prompting
- Use "think" keywords for complex tasks:
  - `think` → basic analysis
  - `think hard` → deeper analysis
  - `think harder` → extensive analysis
  - `ultrathink` → maximum analysis depth

### Workflow Recommendations
1. **Plan First:** Create detailed implementation plans before coding
2. **Incremental Changes:** Work in small, testable increments
3. **Verify Changes:** Use plan mode before auto-accept mode
4. **Commit Often:** Make atomic commits with clear messages

### Memory Management
- Use `CLAUDE.md` for project-specific context
- Keep instructions concise and actionable
- Reference specific patterns and examples
- Update documentation as patterns evolve

## Implementation Process

### For New Features
1. **Understand Requirements:** Analyze the business need thoroughly
2. **Design API Contract:** Define request/response structures
3. **Write Integration Test:** Test the feature end-to-end
4. **Implement Layer by Layer:**
   - Start with failing handler test
   - Implement usecase with tests
   - Define port interfaces
   - Implement gateway with tests
   - Add driver implementation
5. **Refactor:** Improve code quality iteratively
6. **Document:** Update API docs and architecture decisions

### For Bug Fixes
1. **Reproduce:** Write a failing test that demonstrates the bug
2. **Fix:** Make minimal changes to pass the test
3. **Verify:** Ensure no regression in existing tests
4. **Refactor:** Improve surrounding code if needed

### Code Review Checklist
- [ ] Tests written before implementation
- [ ] All tests passing
- [ ] Code coverage maintained/improved
- [ ] No linting errors
- [ ] Clear commit messages
- [ ] Documentation updated
- [ ] No hardcoded values
- [ ] Error handling implemented
- [ ] Logging added for debugging

## Common Patterns

### Error Handling
```go
if err != nil {
    slog.Error("operation failed",
        "error", err,
        "context", contextInfo)
    return fmt.Errorf("operation failed: %w", err)
}
```

### Testing Pattern
```go
func TestXxx(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        // test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

### Dependency Injection
```go
type Service struct {
    repo Repository
    logger *slog.Logger
}

func NewService(repo Repository, logger *slog.Logger) *Service {
    return &Service{
        repo: repo,
        logger: logger,
    }
}
```

## Troubleshooting

### Common Issues
1. **Import Cycles:** Ensure proper layer dependencies
2. **Test Failures:** Check mocks match interfaces
3. **Performance:** Profile before optimizing
4. **Integration Issues:** Verify service contracts

### Debug Tips
- Use structured logging with context
- Write reproducible test cases
- Use debugger for complex issues
- Check service health endpoints

## References

- [Clean Architecture by Robert C. Martin](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Effective Go](https://golang.org/doc/effective_go.html)
- [Domain-Driven Design](https://martinfowler.com/bliki/DomainDrivenDesign.html)
- [Test-Driven Development](https://martinfowler.com/bliki/TestDrivenDevelopment.html)
- [Microservices Best Practices](https://microservices.io/patterns/index.html)