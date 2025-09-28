# kratos Helm Chart

A Helm chart for Ory Kratos identity management service for Alt RSS Reader.

## Overview

This chart deploys Ory Kratos, a next-generation identity management system that provides authentication, user management, and self-service flows. It's designed to integrate with the Alt RSS Reader microservice architecture.

## Features

- **Identity Management** with email-based authentication
- **Self-Service Flows** for registration, login, recovery, and verification
- **Multi-Factor Authentication** with TOTP and lookup secrets
- **Database Migration** with init container support
- **High Availability** with autoscaling and pod disruption budgets
- **Security** with non-root containers and read-only filesystem
- **Monitoring** with health checks and optional ServiceMonitor
- **Environment-specific Configuration** for development and production

## Architecture

```
Alt Frontend ──┐
               ├─→ Kratos Public API (4433) ──┐
External APIs ─┘                              ├─→ Kratos ──→ Kratos PostgreSQL
                                              │
Alt Backend ────→ Kratos Admin API (4434) ────┘
```

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+
- PostgreSQL database (provided by kratos-postgres dependency)
- SSL certificates (provided by common-ssl dependency)
- Secrets management (provided by common-secrets dependency)

## Installation

### Add Dependencies

```bash
helm dependency update
```

### Install Chart

```bash
# Development
helm install kratos . \
  --namespace alt-auth-dev \
  --create-namespace

# Production
helm install kratos . \
  -f values-production.yaml \
  --namespace alt-auth \
  --create-namespace
```

## Configuration

### Key Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of Kratos replicas | `1` |
| `image.repository` | Kratos image repository | `oryd/kratos` |
| `image.tag` | Kratos image tag | `v1.2.0` |
| `namespace` | Deployment namespace | `alt-auth-dev` |
| `kratos.dev` | Enable development mode | `true` |
| `kratos.publicUrl` | Public API base URL | Templated |
| `kratos.adminUrl` | Admin API base URL | Templated |
| `kratos.frontendUrl` | Frontend URL for redirects | `http://localhost:3000` |
| `database.enabled` | Enable database dependency | `true` |
| `autoscaling.enabled` | Enable HPA | `false` |
| `ssl.enabled` | Enable SSL/TLS | `false` |

### Identity Schema

The chart includes a comprehensive identity schema for Alt RSS Reader users:

- **Email**: Primary identifier with verification/recovery support
- **Name**: First and last name fields
- **Tenant ID**: Multi-tenant support
- **Preferences**: User settings including:
  - Theme (light/dark/auto)
  - Language (en/ja)
  - Notifications (email/push)
  - Feed settings (auto-mark read, summary length)

### Authentication Methods

Supported authentication methods:

- **Password**: With HaveIBeenPwned integration
- **TOTP**: Time-based one-time passwords
- **Lookup Secrets**: Backup recovery codes
- **Magic Links**: Passwordless authentication
- **Verification Codes**: Email-based verification

### Environment Configuration

#### Development (default)
- Single replica
- Development mode enabled
- Local frontend URLs
- Relaxed security settings
- In-cluster secrets

#### Production (values-production.yaml)
- Multiple replicas with autoscaling
- Production mode (no dev flag)
- HTTPS URLs
- Stricter security settings
- External Secrets Operator integration
- Network policies
- Node affinity and tolerations

## Dependencies

This chart depends on:

- **kratos-postgres**: PostgreSQL database for Kratos
- **common-ssl**: SSL certificate management
- **common-secrets**: Secrets management

Dependencies are automatically installed when you run `helm dependency update`.

## Services

The chart creates two services:

- **kratos-public** (port 4433): Public API for self-service flows
- **kratos-admin** (port 4434): Admin API for identity management

## Database

Kratos requires a PostgreSQL database. The chart includes:

- **Database Migration**: Init container runs migrations automatically
- **Connection Configuration**: DSN with SSL support
- **Connection Pooling**: Configurable connection limits

## Health Checks

The deployment includes comprehensive health checks:

- **Liveness Probe**: `/health/alive` endpoint
- **Readiness Probe**: `/health/ready` endpoint
- **Configurable Timeouts**: Environment-specific settings

## Security

Security features include:

- **Non-root Container**: Runs as user 10001
- **Read-only Filesystem**: Immutable container filesystem
- **Dropped Capabilities**: All Linux capabilities dropped
- **Secret Management**: Cookie and cipher secrets
- **Network Policies**: Optional traffic restrictions

## Monitoring

Optional monitoring integration:

- **ServiceMonitor**: Prometheus metrics collection
- **Health Endpoints**: Built-in health checking
- **Request Tracing**: Request ID generation

## Autoscaling

Production autoscaling configuration:

- **HPA**: CPU and memory-based scaling
- **Pod Disruption Budget**: Maintain availability during updates
- **Anti-affinity**: Spread pods across nodes

## Migration from Kubernetes YAML

This chart replaces the following Kubernetes resources:

- `auth-service/deployments/k8s/base/kratos-deployment.yaml`
- `auth-service/deployments/k8s/base/kratos-services.yaml`
- `auth-service/deployments/k8s/base/kratos-configmap.yaml`
- `auth-service/deployments/k8s/base/kratos-schema-configmap.yaml`
- `auth-service/deployments/k8s/base/kratos-secret.yaml`

## Troubleshooting

### Common Issues

1. **Migration Failures**
   ```bash
   kubectl logs -l app.kubernetes.io/name=kratos -c kratos-migrate
   ```

2. **Database Connection Issues**
   ```bash
   kubectl get secret kratos-postgres-credentials -o yaml
   ```

3. **Configuration Issues**
   ```bash
   kubectl get configmap kratos-config -o yaml
   ```

### Health Checks

- **Pod Health**: `curl http://pod-ip:4434/health/alive`
- **Service Health**: `curl http://kratos-admin:4434/health/ready`

## Upgrading

```bash
# Check for changes
helm diff upgrade kratos . -f values-production.yaml

# Upgrade
helm upgrade kratos . -f values-production.yaml
```

## Testing

Run the included test script:

```bash
./test-chart.sh
```

This validates:
- Chart linting
- Template generation
- Dependency resolution
- Configuration validation
- Component presence

## Contributing

1. Update values as needed
2. Test with `./test-chart.sh`
3. Validate with `helm lint`
4. Update documentation

## License

This chart is part of the Alt RSS Reader project.