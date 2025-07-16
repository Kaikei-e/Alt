# Deploy CLI

A Go CLI tool for deploying the Alt RSS Reader microservice architecture to Kubernetes using Helm charts. This tool is a complete rewrite of the original `deploy-opt.sh` shell script, implementing Clean Architecture principles and providing enhanced functionality.

## Features

- **Clean Architecture**: Five-layer architecture (REST → Usecase → Port → Gateway → Driver)
- **Environment Support**: Development, staging, and production environments
- **Helm Integration**: Full Helm chart deployment with templating and dry-run capabilities
- **Kubernetes Operations**: Comprehensive kubectl operations for resource management
- **Colored Output**: Enhanced terminal output with color coding for better UX
- **Dry Run Mode**: Template charts without applying changes
- **Validation**: Pre-deployment validation of cluster access and requirements
- **Structured Logging**: Comprehensive logging with context for debugging
- **Error Handling**: Robust error handling with proper context and retry logic

## Prerequisites

- Go 1.24+ installed
- `helm` command available in PATH
- `kubectl` command available in PATH
- Access to a Kubernetes cluster (for actual deployment)

## Installation

1. Clone the repository
2. Navigate to the deploy-cli directory
3. Build the CLI tool:

```bash
go build -o deploy-cli cmd/main.go
```

## Usage

### Basic Deployment

```bash
# Deploy to development environment
IMAGE_PREFIX=myregistry/alt ./deploy-cli deploy development

# Deploy to production environment
IMAGE_PREFIX=myregistry/alt ./deploy-cli deploy production

# Deploy with custom tag
IMAGE_PREFIX=myregistry/alt TAG_BASE=20250115-abc123 ./deploy-cli deploy production
```

### Dry Run

```bash
# Template charts without deploying
IMAGE_PREFIX=myregistry/alt ./deploy-cli deploy production --dry-run
```

### Advanced Options

```bash
# Deploy and restart services
IMAGE_PREFIX=myregistry/alt ./deploy-cli deploy production --restart

# Deploy to custom namespace
IMAGE_PREFIX=myregistry/alt ./deploy-cli deploy production --namespace my-namespace

# Deploy with custom timeout
IMAGE_PREFIX=myregistry/alt ./deploy-cli deploy production --timeout 10m

# Deploy with custom charts directory
IMAGE_PREFIX=myregistry/alt ./deploy-cli deploy production --charts-dir /path/to/charts
```

### Other Commands

```bash
# Validate deployment configuration
./deploy-cli validate production

# Clean up deployment resources
./deploy-cli cleanup production

# Show version information
./deploy-cli version

# Show help
./deploy-cli --help
```

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `IMAGE_PREFIX` | Container image registry prefix (e.g., `myregistry/alt`) | Yes |
| `TAG_BASE` | Image tag base (e.g., `20250115-abc123`) | No |

## Architecture

The CLI tool follows Clean Architecture principles with the following layers:

### Layer Structure

```
REST (CLI Commands)
    ↓
Usecase (Business Logic)
    ↓
Port (Interfaces)
    ↓
Gateway (Anti-Corruption Layer)
    ↓
Driver (External Integrations)
```

### Key Components

- **Domain Layer**: Core business entities and value objects
- **Driver Layer**: External integrations (helm, kubectl, filesystem)
- **Gateway Layer**: Anti-corruption layer that protects domain from external changes
- **Usecase Layer**: Business logic orchestration
- **REST Layer**: CLI interface and command handling

## Deployment Process

The CLI tool follows a structured deployment process:

1. **Pre-deployment Validation**
   - Validate required commands (helm, kubectl)
   - Check cluster accessibility
   - Validate chart configurations

2. **Storage Infrastructure Setup**
   - Create required storage classes
   - Setup persistent volumes
   - Validate storage capacity

3. **Namespace Management**
   - Create required namespaces
   - Setup environment-specific namespace routing

4. **Chart Deployment**
   - Deploy infrastructure charts (databases, storage, networking)
   - Deploy application charts (microservices)
   - Deploy operational charts (monitoring, backup)

5. **Post-deployment Operations**
   - Restart deployments if requested
   - Validate deployment health
   - Monitor rollout status

## Chart Configuration

Charts are deployed in the following order:

### Infrastructure Charts
- common-config, common-ssl, common-secrets
- postgres, auth-postgres, kratos-postgres, kratos
- clickhouse, meilisearch
- nginx, nginx-external, monitoring

### Application Charts
- alt-backend, auth-service
- pre-processor, search-indexer, tag-generator
- news-creator, rask-log-aggregator, alt-frontend

### Operational Charts
- migrate, backup

## Namespace Mapping

### Development/Staging
- Single namespace: `alt-dev` / `alt-staging`

### Production
- `alt-apps`: Application services
- `alt-database`: Database services
- `alt-search`: Search services
- `alt-auth`: Authentication services
- `alt-ingress`: Ingress controllers
- `alt-observability`: Monitoring services

## Error Handling

The CLI tool provides comprehensive error handling with:

- Structured error messages with context
- Colored output for error visibility
- Retry logic for transient failures
- Proper cleanup on deployment failures
- Validation before destructive operations

## Development

### Running Tests

```bash
# Run all tests
go test ./... -v

# Run specific package tests
go test ./domain -v

# Run tests with coverage
go test ./... -cover
```

### Building

```bash
# Build for current platform
go build -o deploy-cli cmd/main.go

# Build for different platforms
GOOS=linux GOARCH=amd64 go build -o deploy-cli-linux cmd/main.go
GOOS=darwin GOARCH=amd64 go build -o deploy-cli-darwin cmd/main.go
```

## Comparison with Original Script

| Feature | Original Script | Go CLI Tool |
|---------|----------------|-------------|
| Architecture | Monolithic shell script | Clean Architecture |
| Error Handling | Basic | Comprehensive with context |
| Testing | None | TDD with comprehensive tests |
| Logging | Basic echo statements | Structured logging |
| Validation | Limited | Comprehensive pre-deployment validation |
| Maintainability | Difficult | High (Clean Architecture) |
| Extensibility | Limited | Easy to extend |
| Performance | Good | Excellent with parallel operations |

## Troubleshooting

### Common Issues

1. **kubectl not accessible**
   - Ensure kubectl is installed and in PATH
   - Verify Kubernetes cluster access

2. **helm not found**
   - Install Helm 3.x
   - Verify helm command availability

3. **Chart not found**
   - Check charts directory path
   - Verify chart structure

4. **Permission denied**
   - Check file permissions
   - Verify cluster RBAC permissions

### Debug Mode

Enable verbose logging for debugging:

```bash
./deploy-cli deploy production --verbose
```

## Contributing

1. Follow Clean Architecture principles
2. Write tests before implementation (TDD)
3. Use structured logging
4. Handle errors properly with context
5. Follow Go best practices and conventions

## License

This project is part of the Alt RSS Reader microservice architecture.