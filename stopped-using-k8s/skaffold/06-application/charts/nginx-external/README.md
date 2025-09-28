# nginx-external Helm Chart

A Helm chart for nginx external ingress/reverse proxy for Alt RSS Reader external traffic.

## Overview

This chart deploys an external-facing nginx reverse proxy designed to handle public internet traffic for the Alt RSS Reader application. It provides SSL termination, rate limiting, DDoS protection, and load balancing for external requests.

## Features

- **External-facing LoadBalancer Service** with AWS NLB annotations
- **SSL Termination** with Let's Encrypt support
- **Rate Limiting** and DDoS protection
- **Security Headers** and hardened configuration
- **Load Balancing** to internal nginx or direct to services
- **Auto-scaling** with Horizontal Pod Autoscaler
- **Monitoring** with Prometheus ServiceMonitor
- **Network Policies** for traffic control
- **Pod Disruption Budget** for high availability

## Architecture

```
Internet -> AWS NLB -> nginx-external -> nginx-internal -> Alt Services
                                    \-> alt-frontend (direct)
                                    \-> alt-backend (direct)
```

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+
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
helm install nginx-external . \
  --namespace nginx-external \
  --create-namespace

# Production
helm install nginx-external . \
  -f values-production.yaml \
  --namespace nginx-external \
  --create-namespace
```

## Configuration

### Key Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of nginx replicas | `2` |
| `image.repository` | nginx image repository | `nginx` |
| `image.tag` | nginx image tag | `1.25.3-alpine` |
| `service.type` | Service type | `LoadBalancer` |
| `service.annotations` | Service annotations | AWS NLB configuration |
| `ssl.enabled` | Enable SSL/TLS | `true` |
| `ssl.secretName` | SSL certificate secret | `nginx-external-ssl-certs` |
| `autoscaling.enabled` | Enable HPA | `true` |
| `nginx.rateLimitRps` | Rate limit per second | `20` |
| `nginx.rateLimitRpm` | Rate limit per minute | `600` |

### Upstream Configuration

The chart supports multiple upstream configurations:

1. **nginx-internal** (recommended): Routes traffic to internal nginx
2. **Direct routing**: Routes directly to application services

```yaml
upstreams:
  - name: nginx-internal
    servers:
      - "nginx.nginx.svc.cluster.local:80"
      - "nginx.nginx.svc.cluster.local:443"
    loadBalancing: "least_conn"
    keepalive: 32
```

### Security Configuration

- **Rate Limiting**: Configurable per endpoint type
- **DDoS Protection**: Connection limits and burst control
- **GeoIP Blocking**: Optional country-based blocking
- **Security Headers**: HSTS, CSP, and other security headers
- **SSL/TLS**: TLS 1.2+ with secure cipher suites

### Production Configuration

The production values include:

- **Higher Resource Limits**: 4 CPU, 2Gi memory
- **Enhanced Autoscaling**: Up to 50 replicas
- **Stricter Rate Limiting**: Lower limits for production traffic
- **Advanced SSL Configuration**: OCSP stapling, session management
- **Enhanced Monitoring**: More frequent health checks
- **Node Affinity**: Dedicated external traffic nodes

## Monitoring

The chart includes:

- **Health Endpoints**: `/nginx-health`, `/external-health`
- **Metrics Endpoint**: `/nginx-status` (restricted access)
- **Prometheus Integration**: ServiceMonitor for metrics collection
- **Request Tracing**: Request ID generation and headers

## Network Security

- **Network Policies**: Restrict ingress/egress traffic
- **Pod Security Context**: Non-root user, read-only filesystem
- **Service Account**: Dedicated service account with minimal permissions

## Load Balancer Configuration

### AWS Network Load Balancer

The chart includes AWS NLB annotations for optimal performance:

```yaml
service:
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled: "true"
    service.beta.kubernetes.io/aws-load-balancer-backend-protocol: "tcp"
```

## SSL/TLS Configuration

### Certificate Management

SSL certificates are managed through the `common-ssl` dependency:

```yaml
ssl:
  enabled: true
  secretName: nginx-external-ssl-certs
```

### Let's Encrypt Integration

The chart supports Let's Encrypt ACME challenges:

```nginx
location /.well-known/acme-challenge/ {
  root /var/www/certbot;
  try_files $uri =404;
}
```

## Rate Limiting

Configurable rate limiting zones:

- **General Traffic**: 20 requests/second (default)
- **API Endpoints**: 600 requests/minute (default) 
- **Connection Limits**: 50 concurrent connections per IP

## Troubleshooting

### Common Issues

1. **SSL Certificate Issues**
   ```bash
   kubectl describe secret nginx-external-ssl-certs -n nginx-external
   ```

2. **Rate Limiting Too Aggressive**
   ```yaml
   nginx:
     rateLimitRps: 50  # Increase from default 20
   ```

3. **Upstream Connection Issues**
   ```bash
   kubectl logs -l app.kubernetes.io/name=nginx-external -n nginx-external
   ```

### Health Checks

- **Pod Health**: `curl http://pod-ip:8080/nginx-health`
- **External Health**: `curl https://domain/external-health`
- **Metrics**: `curl http://pod-ip:8080/nginx-status`

## Upgrading

```bash
# Check for changes
helm diff upgrade nginx-external . -f values-production.yaml

# Upgrade
helm upgrade nginx-external . -f values-production.yaml
```

## Dependencies

- **common-ssl**: SSL certificate management
- **common-secrets**: Secrets management and external secrets

## Contributing

1. Update values as needed
2. Test with `helm template`
3. Validate with `helm lint`
4. Update documentation

## License

This chart is part of the Alt RSS Reader project.