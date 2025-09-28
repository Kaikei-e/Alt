# MeiliSearch Helm Chart

A Helm chart for deploying MeiliSearch, a fast and relevant full-text search engine optimized for RSS feed content in the Alt RSS Reader project.

## Overview

This chart deploys MeiliSearch configured specifically for indexing and searching RSS feed content, including:

- RSS article content search
- Feed metadata search  
- Optimized search rankings for RSS data
- API key management for different access levels
- Production-ready persistence and backup strategies

## Installation

### Basic Installation

```bash
helm install meilisearch ./charts/meilisearch
```

### Production Installation

```bash
helm install meilisearch ./charts/meilisearch \
  --values ./charts/meilisearch/values-production.yaml \
  --namespace alt-production
```

### Development Installation

```bash
helm install meilisearch ./charts/meilisearch \
  --values ./charts/meilisearch/values-development.yaml \
  --namespace alt-development
```

## Configuration

### Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of MeiliSearch replicas | `1` |
| `image.repository` | MeiliSearch image repository | `getmeili/meilisearch` |
| `image.tag` | MeiliSearch image tag | `v1.11.0` |
| `service.port` | Service port | `7700` |
| `auth.masterKeyEnabled` | Enable master key authentication | `true` |
| `rssSearch.enabled` | Enable RSS-specific configuration | `true` |
| `persistence.size` | Data persistence size | `20Gi` |

### Environment-Specific Configurations

#### Development
- Single replica
- Smaller resource limits
- Debug logging enabled
- Simple authentication
- No SSL

#### Staging  
- 2 replicas for HA testing
- Medium resource allocation
- SSL enabled
- External secrets integration
- Backup testing enabled

#### Production
- 3 replicas for high availability
- Optimized resource allocation
- SSL and security hardening
- External secrets management
- Full backup and monitoring

### RSS Search Configuration

The chart includes specialized configuration for RSS content:

#### Indexes
- `rss_content`: For RSS article content
- `rss_feeds`: For RSS feed metadata

#### Search Optimizations
- Custom ranking rules for RSS relevance
- RSS-specific stop words and synonyms
- Optimized faceting for categories and tags
- Time-based sorting for recent content

#### API Keys
- `search-api-key`: Read-only search access
- `admin-api-key`: Full administrative access
- `rss-index-api-key`: RSS indexing operations

## Security

### Authentication
- Master key for administrative operations
- Separate API keys for different access levels
- External secret integration for production

### Network Security
- SSL/TLS support with certificate management
- Network policies (when enabled)
- Service mesh compatibility

### Pod Security
- Non-root container execution
- Read-only root filesystem
- Dropped capabilities
- Security context enforcement

## Persistence

### Data Storage
- Primary data stored in persistent volumes
- Configurable storage classes
- Size scaling per environment

### Backups
- Automated snapshots with configurable schedules
- Database dumps for disaster recovery
- Separate backup storage configuration

## Monitoring

### Metrics
- Prometheus ServiceMonitor integration
- Custom metrics for RSS search operations
- Performance monitoring dashboards

### Health Checks
- Liveness and readiness probes
- Configurable probe timeouts
- Health endpoint monitoring

## Scaling

### Horizontal Scaling
- StatefulSet-based deployment
- Anti-affinity rules for HA
- Load balancing across replicas

### Vertical Scaling
- Resource requests and limits
- Environment-specific sizing
- Memory and CPU optimization

## Troubleshooting

### Common Issues

1. **Pod fails to start**: Check persistent volume provisioning
2. **Search performance**: Verify resource allocation and index size
3. **Authentication errors**: Validate API key configuration
4. **SSL issues**: Check certificate validity and paths

### Debug Commands

```bash
# Check pod status
kubectl get pods -l app.kubernetes.io/name=meilisearch

# View logs
kubectl logs -l app.kubernetes.io/name=meilisearch

# Check configuration
kubectl describe configmap <release-name>-meilisearch

# Test health endpoint
kubectl port-forward svc/<release-name>-meilisearch 7700:7700
curl http://localhost:7700/health
```

## Dependencies

- `common-ssl`: SSL certificate management
- `common-secrets`: Secret management utilities

## Version Compatibility

- Kubernetes: 1.19+
- Helm: 3.0+
- MeiliSearch: v1.11.0