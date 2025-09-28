# CSR Controller

A comprehensive Kubernetes Certificate Signing Request (CSR) management system with automated certificate lifecycle management, monitoring, and alerting.

## Overview

The CSR Controller provides automated SSL/TLS certificate management for Kubernetes applications using the native Kubernetes certificates.k8s.io API. It handles certificate signing, rotation, monitoring, and alerting with production-ready features.

## Features

- **Automated Certificate Signing**: Automatic approval and signing of CSRs based on configurable policies
- **Certificate Lifecycle Management**: Automated certificate renewal, validation, and repair
- **Continuous Monitoring**: Real-time certificate health monitoring with alerting
- **Performance Optimization**: Built-in performance monitoring and tuning recommendations
- **Production Ready**: Comprehensive security, monitoring, and operational features
- **Multi-Environment Support**: Separate configurations for development, staging, and production

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Application   │    │  CSR Controller │    │ Certificate     │
│                 │───▶│                 │───▶│ Monitor         │
│   Creates CSR   │    │  Signs CSR      │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │                       │
                                ▼                       ▼
                       ┌─────────────────┐    ┌─────────────────┐
                       │  Cert Rotation  │    │    Alerting     │
                       │                 │    │                 │
                       │ Auto-renewal    │    │ Slack/Email     │
                       └─────────────────┘    └─────────────────┘
```

## Quick Start

### Prerequisites

- Kubernetes cluster v1.21+
- Helm v3.8+
- kubectl configured with cluster access

### Installation

1. **Create namespace and CA secret**
   ```bash
   kubectl create namespace alt-production
   
   kubectl create secret generic alt-production-ca-secret \
     --from-file=ca.crt=/path/to/ca.crt \
     --from-file=ca.key=/path/to/ca.key \
     -n alt-production
   ```

2. **Install CSR Controller**
   ```bash
   helm install csr-controller ./charts/csr-controller \
     --namespace alt-production \
     --values values-production.yaml
   ```

3. **Verify installation**
   ```bash
   kubectl get pods -n alt-production
   kubectl get csr
   ```

### Basic Usage

1. **Create a CSR**
   ```yaml
   apiVersion: certificates.k8s.io/v1
   kind: CertificateSigningRequest
   metadata:
     name: my-app-csr
   spec:
     request: <base64-encoded-csr>
     signerName: "alt.production.local/ca"
     usages:
     - digital signature
     - key encipherment
     - server auth
   ```

2. **Apply the CSR**
   ```bash
   kubectl apply -f my-app-csr.yaml
   ```

3. **Check certificate**
   ```bash
   kubectl get csr my-app-csr
   kubectl get csr my-app-csr -o jsonpath='{.status.certificate}' | base64 -d
   ```

## Configuration

### Key Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `csrController.signerName` | Certificate signer name | `alt.local/ca` |
| `csrController.ca.secretName` | CA secret name | `ca-secret` |
| `csrController.certRotation.enabled` | Enable automatic rotation | `true` |
| `csrController.certMonitoring.enabled` | Enable monitoring | `true` |
| `csrController.approvalPolicy.autoApprove` | Auto-approve CSRs | `true` |

### Environment-Specific Configurations

- **Development**: `values.yaml`
- **Staging**: `values-staging.yaml`
- **Production**: `values-production.yaml`

## Components

### CSR Controller

The main controller that:
- Watches for new CSRs
- Validates CSR requests against approval policies
- Signs approved CSRs using the configured CA
- Creates Kubernetes secrets with signed certificates

### Certificate Monitor

Continuous monitoring component that:
- Monitors certificate health and expiry
- Sends alerts for expiring certificates
- Validates certificate chains and key pairs
- Collects metrics for monitoring systems

### Certificate Rotation

Automated rotation system that:
- Identifies expiring certificates
- Generates new CSRs for renewal
- Replaces expired certificates
- Maintains certificate continuity

### Certificate Lifecycle Management

Comprehensive lifecycle management that:
- Validates certificate formats and chains
- Repairs invalid certificates
- Cleans up expired CSRs
- Manages certificate metadata

## Monitoring

### Health Checks

```bash
# Check controller health
kubectl exec -n alt-production deployment/csr-controller -- wget -qO- http://localhost:8081/healthz

# Check certificate monitor health
kubectl exec -n alt-production deployment/cert-monitor -- wget -qO- http://localhost:8081/healthz
```

### Metrics

The system exposes Prometheus metrics at `:8080/metrics`:

- `csr_processing_duration_seconds`: CSR processing time
- `cert_days_until_expiry`: Days until certificate expiry
- `cert_validation_errors_total`: Certificate validation errors
- `csr_approval_rate`: CSR approval success rate

### Alerting

Built-in alerting for:
- Certificate expiry (30, 7, 1 day warnings)
- CSR processing failures
- System component failures
- Performance degradation

## Security

### Features

- **Secure CA Management**: CA private keys stored in Kubernetes secrets
- **RBAC Integration**: Role-based access control for CSR operations
- **Network Policies**: Restricted network access between components
- **Audit Logging**: Comprehensive audit trail for all operations

### Best Practices

1. **CA Security**
   - Use hardware security modules (HSM) for CA keys
   - Implement CA key rotation procedures
   - Monitor CA certificate expiry

2. **Access Control**
   - Implement least-privilege RBAC
   - Use service accounts for applications
   - Regular access reviews

3. **Network Security**
   - Enable network policies
   - Use TLS for all communications
   - Implement pod security policies

## Performance

### Optimization Features

- **Horizontal Pod Autoscaling**: Automatic scaling based on load
- **Resource Optimization**: CPU and memory optimization
- **Connection Pooling**: Efficient API connections
- **Batch Processing**: Bulk certificate operations

### Performance Monitoring

```bash
# Start performance monitoring
./scripts/performance-monitor.sh monitor

# Generate performance report
./scripts/performance-monitor.sh report

# Get tuning recommendations
./scripts/performance-monitor.sh tune
```

## Operations

### Deployment

```bash
# Production deployment
./scripts/deploy-production.sh

# Staging deployment
./scripts/deploy-staging.sh
```

### Testing

```bash
# Run integration tests
./scripts/integration-test.sh

# Run certificate rotation tests
./scripts/cert-rotation-test.sh

# Run monitoring tests
./scripts/monitoring-test.sh

# Run all tests
./scripts/run-all-tests.sh
```

### Migration

```bash
# Migrate from legacy certificate system
./scripts/migrate-from-legacy.sh
```

## Troubleshooting

### Common Issues

1. **CSR Not Being Approved**
   - Check approval policy configuration
   - Verify DNS/IP patterns
   - Check controller logs

2. **Certificate Validation Errors**
   - Verify certificate chain
   - Check certificate expiry
   - Validate SAN names

3. **Performance Issues**
   - Check resource usage
   - Review scaling configuration
   - Monitor API rate limits

### Debug Commands

```bash
# Check CSR status
kubectl get csr -o wide

# View controller logs
kubectl logs -n alt-production -l app.kubernetes.io/name=csr-controller

# Check certificate details
kubectl get secret <cert-secret> -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text -noout
```

## Development

### Contributing

1. Fork the repository
2. Create a feature branch
3. Implement changes with tests
4. Submit a pull request

### Building

```bash
# Build container image
docker build -t csr-controller:latest .

# Run tests
go test ./...

# Lint code
golangci-lint run
```

### Development Environment

```bash
# Start development environment
make dev-start

# Run local tests
make test

# Deploy to dev cluster
make dev-deploy
```

## Documentation

- [Operations Guide](docs/OPERATIONS_GUIDE.md) - Comprehensive operational procedures
- [API Reference](docs/API_REFERENCE.md) - API documentation
- [Configuration Reference](docs/CONFIGURATION.md) - Configuration options
- [Troubleshooting Guide](docs/TROUBLESHOOTING.md) - Common issues and solutions

## Support

### Community

- [GitHub Issues](https://github.com/alt/csr-controller/issues)
- [Discussions](https://github.com/alt/csr-controller/discussions)
- [Wiki](https://github.com/alt/csr-controller/wiki)

### Enterprise Support

- **Email**: support@alt.production.local
- **Slack**: #csr-controller-support
- **Phone**: +1-xxx-xxx-xxxx

## Changelog

### v1.0.0

- Initial release with complete CSR management
- Automated certificate lifecycle management
- Comprehensive monitoring and alerting
- Production-ready security features
- Performance optimization and tuning

### v0.9.0

- Beta release with core functionality
- Certificate rotation implementation
- Basic monitoring features
- Security hardening

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Kubernetes SIG-Auth for certificates.k8s.io API
- cert-manager project for inspiration
- OpenSSL project for cryptographic operations
- Prometheus and Grafana for monitoring capabilities

---

## Getting Help

If you encounter issues or have questions:

1. Check the [troubleshooting guide](docs/TROUBLESHOOTING.md)
2. Search [existing issues](https://github.com/alt/csr-controller/issues)
3. Create a [new issue](https://github.com/alt/csr-controller/issues/new)
4. Join our [community discussions](https://github.com/alt/csr-controller/discussions)

For urgent production issues, contact our support team at support@alt.production.local.