# CSR Controller Testing Guide

This document describes the comprehensive testing strategy for the CSR Controller certificate management system.

## Test Types

### 1. Integration Tests

The integration tests verify that all components of the certificate management system work together correctly.

#### Running Integration Tests

```bash
# Install the chart with tests enabled
helm install csr-controller ./charts/csr-controller --set tests.enabled=true

# Run the integration tests
helm test csr-controller

# Check test results
kubectl logs -l app.kubernetes.io/component=test
```

#### Test Coverage

- **CSR Controller Health Check**: Verifies that the CSR controller is running and responsive
- **Certificate Monitor Health Check**: Verifies that the certificate monitor is running and responsive
- **CSR Generation and Approval**: Tests the complete CSR workflow from generation to approval
- **Certificate Validation**: Tests certificate format validation and key pair matching
- **Metrics Endpoint**: Verifies that metrics are being collected and exposed
- **RBAC Permissions**: Tests that the service account has the correct permissions
- **Certificate Rotation Simulation**: Tests the certificate rotation logic

### 2. Performance Tests

The performance tests measure the system's ability to handle concurrent certificate requests.

#### Running Performance Tests

```bash
# Install the chart with performance tests enabled
helm install csr-controller ./charts/csr-controller --set tests.performance.enabled=true

# Run the performance tests
kubectl apply -f charts/csr-controller/templates/tests/cert-performance-test.yaml

# Check test results
kubectl logs job/csr-controller-performance-test
```

#### Performance Metrics

- **Throughput**: Requests per second
- **Latency**: Average, minimum, and maximum response times
- **Success Rate**: Percentage of successful certificate requests
- **Resource Usage**: CPU and memory consumption

#### Performance Thresholds

| Metric | Threshold |
|--------|-----------|
| Average Response Time | < 5 seconds |
| Success Rate | > 95% |
| Requests per Second | > 1 RPS |
| Memory Usage | < 256 MB |
| CPU Usage | < 80% |

### 3. Load Tests

Load tests simulate high-traffic scenarios to test system stability under stress.

#### Running Load Tests

```bash
# Enable load tests
helm upgrade csr-controller ./charts/csr-controller --set tests.load.enabled=true

# Run load tests
kubectl apply -f charts/csr-controller/templates/tests/cert-load-test.yaml

# Monitor system during load test
kubectl top pods -l app.kubernetes.io/name=csr-controller
```

#### Load Test Scenarios

- **Sustained Load**: 10 RPS for 10 minutes
- **Burst Load**: 50 RPS for 1 minute
- **Gradual Ramp**: 1-20 RPS over 5 minutes

### 4. Security Tests

Security tests verify that the system properly handles authentication and authorization.

#### Security Test Areas

- **RBAC Validation**: Verify service accounts have minimal required permissions
- **Certificate Validation**: Ensure only valid certificates are accepted
- **CSR Approval Policy**: Test auto-approval rules and restrictions
- **Secret Security**: Verify proper handling of sensitive data

#### Running Security Tests

```bash
# Run security-specific tests
kubectl apply -f charts/csr-controller/templates/tests/cert-security-test.yaml

# Check security test results
kubectl logs job/csr-controller-security-test
```

## Test Configuration

### Test Parameters

The test configuration can be customized in `values.yaml`:

```yaml
tests:
  enabled: true
  timeout: 300
  logLevel: "info"
  
  performance:
    enabled: true
    concurrentRequests: 5
    totalRequests: 50
    duration: 300
    
    thresholds:
      maxAverageResponseTime: 5000
      minSuccessRate: 95
      minRequestsPerSecond: 1
      maxMemoryUsage: 256
      maxCpuUsage: 80
```

### Environment-Specific Testing

Different test configurations for different environments:

#### Development Environment

```yaml
tests:
  performance:
    concurrentRequests: 2
    totalRequests: 10
    duration: 60
```

#### Staging Environment

```yaml
tests:
  performance:
    concurrentRequests: 10
    totalRequests: 100
    duration: 300
```

#### Production Environment

```yaml
tests:
  performance:
    concurrentRequests: 20
    totalRequests: 500
    duration: 600
```

## Continuous Integration

### Automated Testing Pipeline

The testing pipeline includes:

1. **Unit Tests**: Test individual components
2. **Integration Tests**: Test component interactions
3. **Performance Tests**: Measure system performance
4. **Security Tests**: Verify security controls
5. **Load Tests**: Test under stress conditions

### Test Automation

```bash
#!/bin/bash
# automated-test.sh

set -e

echo "Starting automated certificate system tests..."

# Install chart
helm install csr-controller ./charts/csr-controller \
  --set tests.enabled=true \
  --set tests.performance.enabled=true \
  --wait

# Run integration tests
echo "Running integration tests..."
helm test csr-controller

# Run performance tests
echo "Running performance tests..."
kubectl apply -f charts/csr-controller/templates/tests/cert-performance-test.yaml
kubectl wait --for=condition=complete job/csr-controller-performance-test --timeout=600s

# Check test results
integration_result=$(kubectl get pods -l app.kubernetes.io/component=test -o jsonpath='{.items[0].status.phase}')
performance_result=$(kubectl get job csr-controller-performance-test -o jsonpath='{.status.conditions[0].type}')

if [ "$integration_result" = "Succeeded" ] && [ "$performance_result" = "Complete" ]; then
  echo "✅ All tests passed!"
  exit 0
else
  echo "❌ Tests failed!"
  exit 1
fi
```

## Test Reporting

### Test Results

Test results are available in multiple formats:

1. **Kubernetes Logs**: `kubectl logs -l app.kubernetes.io/component=test`
2. **Helm Test Output**: `helm test csr-controller`
3. **Metrics Dashboard**: Grafana dashboard with test metrics
4. **Test Reports**: JUnit XML format for CI/CD integration

### Metrics Collection

Test metrics are collected and stored:

```yaml
# Example test metrics
test_duration_seconds: 120
test_success_rate: 0.98
test_requests_per_second: 5.2
test_average_response_time_ms: 850
test_max_response_time_ms: 2100
test_min_response_time_ms: 200
```

### Dashboard Integration

Test metrics can be visualized in Grafana:

- **Test Success Rate**: Percentage of passing tests over time
- **Performance Trends**: Response time and throughput trends
- **Resource Usage**: CPU and memory usage during tests
- **Error Rates**: Test failure rates and error types

## Troubleshooting

### Common Test Failures

#### CSR Controller Not Ready

```bash
# Check controller status
kubectl get pods -l app.kubernetes.io/name=csr-controller

# Check controller logs
kubectl logs -l app.kubernetes.io/name=csr-controller
```

#### Certificate Validation Failures

```bash
# Check certificate format
kubectl get secret test-cert -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text

# Verify certificate chain
kubectl get secret test-cert -o jsonpath='{.data.ca\.crt}' | base64 -d > ca.crt
kubectl get secret test-cert -o jsonpath='{.data.tls\.crt}' | base64 -d > cert.crt
openssl verify -CAfile ca.crt cert.crt
```

#### Performance Issues

```bash
# Check resource usage
kubectl top pods -l app.kubernetes.io/name=csr-controller

# Check for resource constraints
kubectl describe pods -l app.kubernetes.io/name=csr-controller

# Check pending CSRs
kubectl get csr | grep Pending
```

#### RBAC Issues

```bash
# Check service account permissions
kubectl auth can-i create certificatesigningrequests --as=system:serviceaccount:default:csr-controller

# Check cluster role binding
kubectl get clusterrolebinding | grep csr-controller
```

### Test Debugging

Enable debug logging for detailed test information:

```yaml
tests:
  logLevel: "debug"
  
logging:
  level: "debug"
```

## Best Practices

### Test Design

1. **Isolation**: Each test should be independent and not affect others
2. **Cleanup**: Always clean up test resources after completion
3. **Timeouts**: Set appropriate timeouts for all operations
4. **Assertions**: Include clear assertions with meaningful error messages
5. **Repeatability**: Tests should produce consistent results

### Test Data Management

1. **Synthetic Data**: Use generated test data, not production data
2. **Data Cleanup**: Remove test data after each test run
3. **Namespace Isolation**: Use dedicated namespaces for testing
4. **Resource Limits**: Set appropriate resource limits for test pods

### Performance Testing

1. **Baseline**: Establish performance baselines for comparison
2. **Gradual Load**: Start with low load and gradually increase
3. **Monitoring**: Monitor system resources during tests
4. **Regression Detection**: Alert on performance regressions

## Integration with CI/CD

### GitHub Actions

```yaml
name: Certificate System Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - name: Checkout code
      uses: actions/checkout@v2
    
    - name: Setup Kubernetes
      uses: engineerd/setup-kind@v0.5.0
    
    - name: Install chart
      run: |
        helm install csr-controller ./charts/csr-controller \
          --set tests.enabled=true \
          --wait
    
    - name: Run tests
      run: |
        helm test csr-controller
        kubectl apply -f charts/csr-controller/templates/tests/cert-performance-test.yaml
    
    - name: Collect test results
      run: |
        kubectl logs -l app.kubernetes.io/component=test > test-results.log
        kubectl logs job/csr-controller-performance-test > performance-results.log
    
    - name: Upload results
      uses: actions/upload-artifact@v2
      with:
        name: test-results
        path: "*-results.log"
```

### GitLab CI

```yaml
stages:
  - test
  - performance
  - security

integration-test:
  stage: test
  script:
    - helm install csr-controller ./charts/csr-controller --set tests.enabled=true --wait
    - helm test csr-controller
  artifacts:
    reports:
      junit: test-results.xml

performance-test:
  stage: performance
  script:
    - kubectl apply -f charts/csr-controller/templates/tests/cert-performance-test.yaml
    - kubectl wait --for=condition=complete job/csr-controller-performance-test --timeout=600s
  artifacts:
    reports:
      performance: performance-results.json

security-test:
  stage: security
  script:
    - kubectl apply -f charts/csr-controller/templates/tests/cert-security-test.yaml
    - kubectl wait --for=condition=complete job/csr-controller-security-test --timeout=300s
  artifacts:
    reports:
      security: security-results.json
```

## Conclusion

This comprehensive testing strategy ensures that the CSR Controller certificate management system is reliable, performant, and secure. Regular testing helps maintain system quality and prevents regressions.

For additional support or questions about testing, please refer to the main documentation or contact the development team.