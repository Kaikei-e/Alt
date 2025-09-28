# CSR Controller Operations Guide

## Overview

This document provides comprehensive operational guidance for the CSR Controller system, covering deployment, monitoring, troubleshooting, and maintenance procedures.

## Table of Contents

1. [System Architecture](#system-architecture)
2. [Deployment](#deployment)
3. [Monitoring](#monitoring)
4. [Troubleshooting](#troubleshooting)
5. [Maintenance](#maintenance)
6. [Security](#security)
7. [Performance Optimization](#performance-optimization)
8. [Disaster Recovery](#disaster-recovery)
9. [Upgrades](#upgrades)
10. [Best Practices](#best-practices)

## System Architecture

### Components

- **CSR Controller**: Main controller for certificate signing requests
- **Certificate Monitor**: Continuous monitoring of certificate health
- **Certificate Rotation**: Automated certificate renewal
- **Certificate Lifecycle**: Certificate validation and repair

### Data Flow

```
Application → CSR Creation → CSR Controller → Certificate Signing → Secret Creation → Application
                                     ↓
                            Certificate Monitor → Alerting
                                     ↓
                            Certificate Rotation → Auto-renewal
```

## Deployment

### Prerequisites

- Kubernetes cluster v1.21+
- Helm v3.8+
- kubectl configured with cluster access
- CA certificates for signing

### Production Deployment

1. **Prepare environment**
   ```bash
   # Create namespace
   kubectl create namespace alt-production
   
   # Create CA secret
   kubectl create secret generic alt-production-ca-secret \
     --from-file=ca.crt=/path/to/ca.crt \
     --from-file=ca.key=/path/to/ca.key \
     -n alt-production
   ```

2. **Deploy CSR Controller**
   ```bash
   cd /path/to/charts/csr-controller
   ./scripts/deploy-production.sh
   ```

3. **Verify deployment**
   ```bash
   kubectl get pods -n alt-production
   kubectl get csr
   ```

### Configuration

Key configuration parameters in `values-production.yaml`:

- `csrController.signerName`: Certificate signer name
- `csrController.ca.secretName`: CA secret name
- `csrController.certRotation.schedule`: Rotation schedule
- `csrController.certMonitoring.interval`: Monitoring interval

## Monitoring

### Health Checks

```bash
# Check pod health
kubectl get pods -n alt-production -l app.kubernetes.io/name=csr-controller

# Check service endpoints
kubectl exec -n alt-production deployment/csr-controller -- wget -qO- http://localhost:8081/healthz

# Check metrics
kubectl port-forward -n alt-production svc/csr-controller 8080:8080
curl http://localhost:8080/metrics
```

### Key Metrics

- `csr_processing_duration_seconds`: CSR processing time
- `cert_days_until_expiry`: Certificate expiry countdown
- `cert_validation_errors_total`: Certificate validation errors
- `csr_approval_rate`: CSR approval success rate

### Alerting

Critical alerts to monitor:

1. **CSR Controller Down**
   ```
   up{job="csr-controller"} == 0
   ```

2. **Certificate Expiring Soon**
   ```
   cert_days_until_expiry < 7
   ```

3. **High CSR Processing Time**
   ```
   csr_processing_duration_seconds > 30
   ```

### Performance Monitoring

Use the performance monitoring script:

```bash
# Start continuous monitoring
./scripts/performance-monitor.sh monitor

# Generate performance report
./scripts/performance-monitor.sh report

# Get tuning suggestions
./scripts/performance-monitor.sh tune
```

## Troubleshooting

### Common Issues

#### 1. CSR Not Being Approved

**Symptoms:**
- CSRs remain in "Pending" state
- Applications cannot get certificates

**Diagnosis:**
```bash
# Check CSR details
kubectl describe csr <csr-name>

# Check controller logs
kubectl logs -n alt-production -l app.kubernetes.io/name=csr-controller
```

**Solutions:**
- Verify approval policy configuration
- Check DNS/IP patterns in approval rules
- Verify CA certificate validity

#### 2. Certificate Validation Errors

**Symptoms:**
- Certificate monitoring alerts
- Applications reporting SSL errors

**Diagnosis:**
```bash
# Check certificate details
kubectl get secret <cert-secret> -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text -noout

# Check certificate chain
openssl verify -CAfile ca.crt cert.pem
```

**Solutions:**
- Verify certificate chain
- Check certificate expiry
- Validate certificate SAN names

#### 3. High Resource Usage

**Symptoms:**
- High CPU/memory usage
- Pod restart due to OOM

**Diagnosis:**
```bash
# Check resource usage
kubectl top pods -n alt-production

# Check resource limits
kubectl describe pod <pod-name> -n alt-production
```

**Solutions:**
- Adjust resource limits
- Enable horizontal pod autoscaling
- Optimize certificate processing

### Debugging Commands

```bash
# Get detailed pod information
kubectl describe pod <pod-name> -n alt-production

# Check controller logs
kubectl logs -n alt-production -l app.kubernetes.io/name=csr-controller -f

# Check events
kubectl get events -n alt-production --sort-by=.metadata.creationTimestamp

# Check CSR processing
kubectl get csr -o wide

# Test certificate functionality
./scripts/integration-test.sh
```

## Maintenance

### Regular Tasks

#### Daily
- Monitor alert channels
- Check system health metrics
- Review CSR processing logs

#### Weekly
- Generate performance reports
- Review certificate expiry reports
- Check for pending CSRs

#### Monthly
- Update documentation
- Review and update monitoring thresholds
- Capacity planning review

### Certificate Lifecycle Management

1. **Automatic Rotation**
   - Scheduled daily at 2 AM
   - Renews certificates expiring within 30 days
   - Sends notifications on failures

2. **Manual Rotation**
   ```bash
   # Rotate specific certificate
   kubectl delete secret <cert-secret> -n <namespace>
   # CSR Controller will create new certificate
   ```

3. **Bulk Operations**
   ```bash
   # Rotate all certificates in namespace
   kubectl delete secrets -l app.kubernetes.io/managed-by=csr-controller -n <namespace>
   ```

### Backup and Recovery

1. **Backup CA Certificates**
   ```bash
   # Export CA secret
   kubectl get secret alt-production-ca-secret -n alt-production -o yaml > ca-backup.yaml
   ```

2. **Backup Configuration**
   ```bash
   # Export Helm values
   helm get values csr-controller -n alt-production > values-backup.yaml
   ```

3. **Recovery Process**
   ```bash
   # Restore CA secret
   kubectl apply -f ca-backup.yaml
   
   # Restore configuration
   helm upgrade csr-controller ./charts/csr-controller -f values-backup.yaml
   ```

## Security

### Access Control

1. **RBAC Configuration**
   - Service accounts with minimal permissions
   - Role-based access to CSR operations
   - Namespace isolation

2. **Network Security**
   - Network policies restricting pod communication
   - TLS encryption for all communications
   - Secure API endpoints

### Certificate Security

1. **CA Protection**
   - CA private key stored in Kubernetes secrets
   - Regular CA certificate rotation
   - Audit logging for CA operations

2. **Certificate Validation**
   - Automatic certificate chain validation
   - SAN name verification
   - Expiry monitoring

### Compliance

- SOC 2 Type II controls
- PCI DSS compliance for payment processing
- GDPR compliance for data protection

## Performance Optimization

### Resource Optimization

1. **CPU Optimization**
   - Monitor CPU usage patterns
   - Adjust CPU limits based on workload
   - Enable CPU-based autoscaling

2. **Memory Optimization**
   - Monitor memory usage trends
   - Optimize certificate caching
   - Implement memory-based alerting

### Scaling Configuration

1. **Horizontal Pod Autoscaling**
   ```yaml
   scaling:
     hpa:
       enabled: true
       minReplicas: 2
       maxReplicas: 5
       targetCPUUtilizationPercentage: 70
   ```

2. **Vertical Pod Autoscaling**
   - Use in development environments
   - Collect recommendations for production tuning
   - Avoid conflicts with HPA

### Performance Tuning

1. **Certificate Processing**
   - Implement connection pooling
   - Optimize API calls
   - Enable batch processing

2. **Monitoring Optimization**
   - Adjust monitoring intervals
   - Optimize metrics collection
   - Configure appropriate retention

## Disaster Recovery

### Backup Strategy

1. **Configuration Backup**
   - Daily Helm configuration backup
   - Version-controlled configuration files
   - Automated backup verification

2. **Certificate Backup**
   - CA certificate backup
   - Application certificate backup
   - Backup encryption and storage

### Recovery Procedures

1. **Complete System Recovery**
   ```bash
   # Restore namespace
   kubectl create namespace alt-production
   
   # Restore CA certificates
   kubectl apply -f ca-backup.yaml
   
   # Restore CSR Controller
   helm install csr-controller ./charts/csr-controller -f values-backup.yaml
   
   # Verify recovery
   ./scripts/integration-test.sh
   ```

2. **Partial Recovery**
   - Certificate-only recovery
   - Configuration-only recovery
   - Rollback procedures

### Testing Recovery

- Monthly disaster recovery drills
- Automated recovery testing
- Documentation updates

## Upgrades

### Preparation

1. **Pre-upgrade Checklist**
   - [ ] Backup current configuration
   - [ ] Backup CA certificates
   - [ ] Test upgrade in staging environment
   - [ ] Review release notes
   - [ ] Schedule maintenance window

2. **Compatibility Check**
   - Kubernetes version compatibility
   - Helm version compatibility
   - Certificate format compatibility

### Upgrade Process

1. **Staging Upgrade**
   ```bash
   # Upgrade in staging
   helm upgrade csr-controller ./charts/csr-controller -f values-staging.yaml
   
   # Test functionality
   ./scripts/integration-test.sh
   ```

2. **Production Upgrade**
   ```bash
   # Upgrade in production
   helm upgrade csr-controller ./charts/csr-controller -f values-production.yaml
   
   # Monitor upgrade
   kubectl rollout status deployment/csr-controller -n alt-production
   ```

### Rollback Procedures

```bash
# Rollback to previous version
helm rollback csr-controller -n alt-production

# Verify rollback
kubectl get pods -n alt-production
./scripts/integration-test.sh
```

## Best Practices

### Operations

1. **Monitoring**
   - Implement comprehensive monitoring
   - Set up proactive alerting
   - Regular performance reviews

2. **Documentation**
   - Keep runbooks updated
   - Document all procedures
   - Version control all configurations

3. **Testing**
   - Regular integration testing
   - Disaster recovery testing
   - Performance testing

### Development

1. **Configuration Management**
   - Use GitOps for configuration
   - Environment-specific configurations
   - Automated configuration validation

2. **Security**
   - Regular security audits
   - Vulnerability scanning
   - Compliance monitoring

### Operational Excellence

1. **Automation**
   - Automate routine tasks
   - Implement self-healing systems
   - Use infrastructure as code

2. **Continuous Improvement**
   - Regular post-incident reviews
   - Performance optimization
   - Feature enhancement planning

## Support and Escalation

### Support Channels

1. **Level 1 Support**
   - Basic troubleshooting
   - Health check verification
   - Alert acknowledgment

2. **Level 2 Support**
   - Complex troubleshooting
   - Configuration changes
   - Performance optimization

3. **Level 3 Support**
   - System architecture changes
   - Security incident response
   - Disaster recovery

### Escalation Procedures

1. **Critical Issues**
   - Immediate escalation to L3
   - Incident commander assignment
   - Executive notification

2. **Major Issues**
   - Escalate to L2 within 15 minutes
   - Regular status updates
   - Customer communication

### Contact Information

- **Operations Team**: ops-team@alt.production.local
- **Platform Engineering**: platform-engineering@alt.production.local
- **Security Team**: security-team@alt.production.local
- **Emergency Hotline**: +1-xxx-xxx-xxxx

## Appendix

### Useful Commands

```bash
# Quick health check
kubectl get pods -n alt-production -l app.kubernetes.io/name=csr-controller

# Check CSR processing
kubectl get csr | grep -E '(Pending|Failed)'

# Monitor logs
kubectl logs -n alt-production -l app.kubernetes.io/name=csr-controller -f

# Performance monitoring
./scripts/performance-monitor.sh report

# Run integration tests
./scripts/integration-test.sh
```

### Configuration Templates

See `values-production.yaml` for production configuration template.

### Troubleshooting Matrix

| Issue | Symptoms | Diagnosis | Solution |
|-------|----------|-----------|----------|
| CSR Not Approved | Pending CSRs | Check approval policy | Update DNS patterns |
| Certificate Expired | SSL errors | Check certificate expiry | Trigger renewal |
| High CPU | Performance issues | Check resource usage | Scale or optimize |
| Pod Crashes | Restart loops | Check logs | Fix configuration |

---

*This operations guide is maintained by the Platform Engineering team. Last updated: $(date)*