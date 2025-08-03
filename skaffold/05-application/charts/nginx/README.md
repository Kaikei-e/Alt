# nginx Helm Chart

A Helm chart for nginx internal proxy/load balancer for Alt RSS Reader services.

## Description

This chart deploys nginx as an internal reverse proxy and load balancer for the Alt RSS Reader microservices architecture. It provides SSL termination, load balancing, rate limiting, and request routing to various backend services.

## Prerequisites

- Kubernetes 1.20+
- Helm 3.2.0+
- SSL certificates configured (via common-ssl dependency)
- Backend services deployed and accessible

## Installing the Chart

To install the chart with the release name `nginx`:

```bash
helm install nginx ./nginx
```

To install with production values:

```bash
helm install nginx ./nginx -f values-production.yaml
```

## Uninstalling the Chart

To uninstall/delete the `nginx` deployment:

```bash
helm uninstall nginx
```

## Configuration

The following table lists the configurable parameters and their default values.

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of nginx replicas | `2` |
| `image.repository` | nginx image repository | `nginx` |
| `image.tag` | nginx image tag | `1.25.3-alpine` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |

### Service Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | HTTP service port | `80` |
| `service.httpsPort` | HTTPS service port | `443` |
| `service.healthPort` | Health check port | `8080` |

### nginx Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `nginx.workerConnections` | nginx worker connections | `1024` |
| `nginx.keepaliveTimeout` | Keep-alive timeout | `65` |
| `nginx.logLevel` | Log level | `warn` |
| `nginx.clientMaxBodySize` | Maximum client body size | `10m` |
| `nginx.rateLimitRpm` | Rate limit per minute for API | `300` |
| `nginx.rateLimitRps` | Rate limit per second | `10` |

### Upstream Services

The chart configures upstreams for the following Alt services:

- **alt-backend**: Main backend API service
- **alt-frontend**: Frontend application
- **auth-service**: Authentication service
- **meilisearch**: Search engine service
- **tag-generator**: Tag generation service

### SSL Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ssl.enabled` | Enable SSL/TLS | `true` |
| `ssl.secretName` | SSL certificate secret name | `nginx-ssl-certs` |

### Autoscaling

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable horizontal pod autoscaler | `false` |
| `autoscaling.minReplicas` | Minimum number of replicas | `2` |
| `autoscaling.maxReplicas` | Maximum number of replicas | `10` |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU utilization | `80` |

## Routes and Load Balancing

The nginx proxy handles the following routes:

- `/` → `alt-frontend` (Frontend application)
- `/api/` → `alt-backend` (Backend API)
- `/v1/` → `alt-backend` (Backend API v1)
- `/auth/` → `auth-service` (Authentication)
- `/search/` → `meilisearch` (Search engine)
- `/tags/` → `tag-generator` (Tag generation)
- `/health` → Health check endpoint

## Production Deployment

For production environments, use the production values file:

```bash
helm install nginx ./nginx -f values-production.yaml
```

Production configuration includes:

- Increased replica count (3)
- Higher resource limits
- Enhanced security headers
- Stricter rate limiting
- Pod anti-affinity rules
- Autoscaling enabled

## Monitoring

The chart includes:

- Health check endpoints at `/nginx-health` and `/nginx-status`
- Prometheus metrics annotations
- Access and error logging
- Request tracing with unique request IDs

## Security Features

- SSL/TLS termination with modern ciphers
- Security headers (HSTS, CSP, etc.)
- Rate limiting per endpoint
- IP whitelisting for metrics endpoints
- Non-root container execution

## Troubleshooting

### Common Issues

1. **SSL Certificate Issues**: Ensure the SSL secret exists and contains valid certificates
2. **Backend Connection Issues**: Verify backend services are running and accessible
3. **Rate Limiting**: Check if requests are being rate limited in nginx logs

### Useful Commands

```bash
# Check nginx configuration
kubectl exec -it deployment/nginx -- nginx -t

# View nginx logs
kubectl logs deployment/nginx -f

# Check nginx status
kubectl exec -it deployment/nginx -- curl localhost:8080/nginx-status
```

## Dependencies

- `common-ssl`: SSL certificate management
- `common-secrets`: Secret management

## Contributing

1. Update values in `values.yaml` or `values-production.yaml`
2. Test with `helm template` and `helm lint`
3. Update this README if adding new features