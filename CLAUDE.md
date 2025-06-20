# CLAUDE.md

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
- **LLM:** Phi4-mini

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
1. **Red:** Write a failing test first
2. **Green:** Write minimal code to pass the test
3. **Refactor:** Improve code quality while keeping tests green

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