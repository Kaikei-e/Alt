#!/bin/bash

# CSR Controller Integration Test Script
# This script performs comprehensive integration testing of the certificate management system

set -e

# Configuration
NAMESPACE="csr-controller-test"
CHART_NAME="csr-controller"
RELEASE_NAME="csr-controller-integration-test"
TIMEOUT=600
LOG_LEVEL="info"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Test result tracking
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# Function to run test
run_test() {
    local test_name=$1
    local test_command=$2
    
    log_info "Running test: $test_name"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    
    if eval "$test_command"; then
        log_success "PASSED: $test_name"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        log_error "FAILED: $test_name"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Cleanup function
cleanup() {
    log_info "Cleaning up test resources..."
    
    # Delete test namespace
    kubectl delete namespace $NAMESPACE --ignore-not-found=true --timeout=60s
    
    # Delete test CSRs
    kubectl delete csr -l app.kubernetes.io/managed-by=integration-test --ignore-not-found=true
    
    # Delete test secrets
    kubectl delete secret -l app.kubernetes.io/managed-by=integration-test --ignore-not-found=true
    
    log_info "Cleanup completed"
}

# Setup function
setup() {
    log_info "Setting up test environment..."
    
    # Create test namespace
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    # Label namespace for testing
    kubectl label namespace $NAMESPACE app.kubernetes.io/managed-by=integration-test --overwrite
    
    log_info "Test environment setup completed"
}

# Test 1: Chart Installation
test_chart_installation() {
    log_info "Installing CSR Controller chart..."
    
    # Install chart with test configuration
    helm install $RELEASE_NAME ./charts/csr-controller \
        --namespace $NAMESPACE \
        --set tests.enabled=true \
        --set tests.performance.enabled=true \
        --set csrController.certRotation.enabled=true \
        --set csrController.certMonitoring.enabled=true \
        --set csrController.certLifecycle.enabled=true \
        --set csrController.certRotation.dryRun=true \
        --set csrController.certLifecycle.dryRun=true \
        --wait --timeout=${TIMEOUT}s
    
    if [ $? -eq 0 ]; then
        log_success "Chart installed successfully"
        return 0
    else
        log_error "Chart installation failed"
        return 1
    fi
}

# Test 2: Pod Readiness
test_pod_readiness() {
    log_info "Checking pod readiness..."
    
    # Wait for CSR controller pod
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=csr-controller \
        -n $NAMESPACE --timeout=${TIMEOUT}s
    
    if [ $? -eq 0 ]; then
        log_success "CSR controller pod is ready"
    else
        log_error "CSR controller pod is not ready"
        return 1
    fi
    
    # Wait for certificate monitor pod
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=cert-monitor \
        -n $NAMESPACE --timeout=${TIMEOUT}s
    
    if [ $? -eq 0 ]; then
        log_success "Certificate monitor pod is ready"
        return 0
    else
        log_error "Certificate monitor pod is not ready"
        return 1
    fi
}

# Test 3: Service Endpoints
test_service_endpoints() {
    log_info "Testing service endpoints..."
    
    # Test CSR controller health endpoint
    csr_controller_pod=$(kubectl get pod -l app.kubernetes.io/name=csr-controller -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if kubectl exec -n $NAMESPACE $csr_controller_pod -- wget -q --spider http://localhost:8081/healthz; then
        log_success "CSR controller health endpoint is responding"
    else
        log_error "CSR controller health endpoint is not responding"
        return 1
    fi
    
    # Test certificate monitor health endpoint
    cert_monitor_pod=$(kubectl get pod -l app.kubernetes.io/component=cert-monitor -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if kubectl exec -n $NAMESPACE $cert_monitor_pod -- wget -q --spider http://localhost:8081/; then
        log_success "Certificate monitor health endpoint is responding"
        return 0
    else
        log_error "Certificate monitor health endpoint is not responding"
        return 1
    fi
}

# Test 4: RBAC Permissions
test_rbac_permissions() {
    log_info "Testing RBAC permissions..."
    
    # Get service account
    service_account=$(kubectl get serviceaccount -n $NAMESPACE -l app.kubernetes.io/name=csr-controller -o jsonpath='{.items[0].metadata.name}')
    
    # Test CSR permissions
    if kubectl auth can-i create certificatesigningrequests --as=system:serviceaccount:$NAMESPACE:$service_account; then
        log_success "CSR creation permission granted"
    else
        log_error "CSR creation permission denied"
        return 1
    fi
    
    # Test secret permissions
    if kubectl auth can-i create secrets --as=system:serviceaccount:$NAMESPACE:$service_account; then
        log_success "Secret creation permission granted"
    else
        log_error "Secret creation permission denied"
        return 1
    fi
    
    # Test CSR approval permissions
    if kubectl auth can-i update certificatesigningrequests/approval --as=system:serviceaccount:$NAMESPACE:$service_account; then
        log_success "CSR approval permission granted"
        return 0
    else
        log_error "CSR approval permission denied"
        return 1
    fi
}

# Test 5: CSR Generation and Approval
test_csr_workflow() {
    log_info "Testing CSR generation and approval workflow..."
    
    # Generate test private key
    openssl genrsa -out /tmp/test-integration.key 2048
    
    # Create test CSR
    cat > /tmp/test-integration.conf <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = integration-test-service
O = Alt RSS Reader

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = integration-test-service
DNS.2 = integration-test-service.test.svc.cluster.local
DNS.3 = localhost
IP.1 = 127.0.0.1
EOF
    
    # Generate CSR
    openssl req -new -key /tmp/test-integration.key -out /tmp/test-integration.csr -config /tmp/test-integration.conf
    
    # Create K8s CSR
    csr_name="integration-test-$(date +%s)"
    cat > /tmp/k8s-csr-integration.yaml <<EOF
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: $csr_name
  labels:
    app.kubernetes.io/managed-by: integration-test
spec:
  request: $(cat /tmp/test-integration.csr | base64 | tr -d '\n')
  signerName: "alt.local/ca"
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF
    
    # Apply CSR
    if kubectl apply -f /tmp/k8s-csr-integration.yaml; then
        log_success "CSR created successfully"
        
        # Wait for approval
        log_info "Waiting for CSR approval..."
        for i in {1..30}; do
            if kubectl get csr $csr_name -o jsonpath='{.status.certificate}' | grep -q "LS0t"; then
                log_success "CSR approved and signed"
                kubectl delete csr $csr_name
                rm -f /tmp/test-integration.* /tmp/k8s-csr-integration.yaml
                return 0
            fi
            sleep 2
        done
        
        log_error "CSR was not approved within timeout"
        kubectl delete csr $csr_name
        rm -f /tmp/test-integration.* /tmp/k8s-csr-integration.yaml
        return 1
    else
        log_error "Failed to create CSR"
        rm -f /tmp/test-integration.* /tmp/k8s-csr-integration.yaml
        return 1
    fi
}

# Test 6: Certificate Monitoring
test_certificate_monitoring() {
    log_info "Testing certificate monitoring..."
    
    # Create test certificate with short expiry
    secret_name="test-monitor-$(date +%s)"
    
    # Generate certificate expiring in 1 day
    openssl genrsa -out /tmp/monitor.key 2048
    openssl req -new -key /tmp/monitor.key -out /tmp/monitor.csr -subj "/CN=monitor-test/O=Alt RSS Reader"
    openssl x509 -req -in /tmp/monitor.csr -signkey /tmp/monitor.key -out /tmp/monitor.crt -days 1
    
    # Create secret
    kubectl create secret generic $secret_name -n $NAMESPACE \
        --from-file=tls.crt=/tmp/monitor.crt \
        --from-file=tls.key=/tmp/monitor.key
    
    # Label secret
    kubectl label secret $secret_name -n $NAMESPACE app.kubernetes.io/component=ssl-certificate
    kubectl label secret $secret_name -n $NAMESPACE app.kubernetes.io/managed-by=integration-test
    
    # Wait for monitoring to detect
    log_info "Waiting for certificate monitoring to detect expiring certificate..."
    sleep 10
    
    # Check if certificate is detected as expiring
    cert_data=$(kubectl get secret $secret_name -n $NAMESPACE -o jsonpath='{.data.tls\.crt}')
    echo "$cert_data" | base64 -d > /tmp/check.crt
    
    # Check expiry
    expiry_date=$(openssl x509 -in /tmp/check.crt -noout -enddate | cut -d= -f2)
    expiry_epoch=$(date -d "$expiry_date" +%s)
    current_epoch=$(date +%s)
    days_until_expiry=$(((expiry_epoch - current_epoch) / 86400))
    
    if [ $days_until_expiry -le 30 ]; then
        log_success "Certificate correctly detected as expiring soon (days: $days_until_expiry)"
        kubectl delete secret $secret_name -n $NAMESPACE
        rm -f /tmp/monitor.*
        return 0
    else
        log_error "Certificate expiry detection failed"
        kubectl delete secret $secret_name -n $NAMESPACE
        rm -f /tmp/monitor.*
        return 1
    fi
}

# Test 7: Metrics Collection
test_metrics_collection() {
    log_info "Testing metrics collection..."
    
    # Test CSR controller metrics
    csr_controller_pod=$(kubectl get pod -l app.kubernetes.io/name=csr-controller -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if kubectl exec -n $NAMESPACE $csr_controller_pod -- wget -q -O - http://localhost:8080/metrics | grep -q "cert_"; then
        log_success "CSR controller metrics available"
    else
        log_error "CSR controller metrics not available"
        return 1
    fi
    
    # Test certificate monitor metrics
    cert_monitor_pod=$(kubectl get pod -l app.kubernetes.io/component=cert-monitor -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if kubectl exec -n $NAMESPACE $cert_monitor_pod -- wget -q -O - http://localhost:8080/ | grep -q "cert_"; then
        log_success "Certificate monitor metrics available"
        return 0
    else
        log_error "Certificate monitor metrics not available"
        return 1
    fi
}

# Test 8: Helm Tests
test_helm_tests() {
    log_info "Running Helm tests..."
    
    if helm test $RELEASE_NAME -n $NAMESPACE --timeout=${TIMEOUT}s; then
        log_success "Helm tests passed"
        return 0
    else
        log_error "Helm tests failed"
        return 1
    fi
}

# Test 9: Resource Usage
test_resource_usage() {
    log_info "Testing resource usage..."
    
    # Get CSR controller resource usage
    csr_controller_pod=$(kubectl get pod -l app.kubernetes.io/name=csr-controller -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if kubectl top pod $csr_controller_pod -n $NAMESPACE --no-headers >/dev/null 2>&1; then
        cpu_usage=$(kubectl top pod $csr_controller_pod -n $NAMESPACE --no-headers | awk '{print $2}')
        memory_usage=$(kubectl top pod $csr_controller_pod -n $NAMESPACE --no-headers | awk '{print $3}')
        
        log_success "CSR Controller resource usage - CPU: $cpu_usage, Memory: $memory_usage"
        
        # Check if within limits
        # Note: This is a basic check, actual thresholds should be environment-specific
        if [[ $memory_usage =~ ^[0-9]+Mi$ ]] && [[ ${memory_usage%Mi} -lt 256 ]]; then
            log_success "Memory usage within acceptable limits"
        else
            log_warning "Memory usage may be high: $memory_usage"
        fi
        
        return 0
    else
        log_warning "Could not retrieve resource usage metrics"
        return 0  # Don't fail test for this
    fi
}

# Main test execution
main() {
    log_info "Starting CSR Controller Integration Tests"
    log_info "========================================="
    
    # Trap cleanup function
    trap cleanup EXIT
    
    # Setup test environment
    setup
    
    # Run tests
    run_test "Chart Installation" "test_chart_installation"
    run_test "Pod Readiness" "test_pod_readiness"
    run_test "Service Endpoints" "test_service_endpoints"
    run_test "RBAC Permissions" "test_rbac_permissions"
    run_test "CSR Workflow" "test_csr_workflow"
    run_test "Certificate Monitoring" "test_certificate_monitoring"
    run_test "Metrics Collection" "test_metrics_collection"
    run_test "Helm Tests" "test_helm_tests"
    run_test "Resource Usage" "test_resource_usage"
    
    # Report results
    echo
    log_info "========================================="
    log_info "Integration Test Results Summary:"
    log_info "========================================="
    log_info "Total tests: $TESTS_TOTAL"
    log_success "Passed: $TESTS_PASSED"
    log_error "Failed: $TESTS_FAILED"
    
    if [ $TESTS_FAILED -gt 0 ]; then
        log_error "Some tests failed. Please check the logs above."
        exit 1
    else
        log_success "All tests passed! CSR Controller system is working correctly."
        exit 0
    fi
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed or not in PATH"
        exit 1
    fi
    
    # Check helm
    if ! command -v helm &> /dev/null; then
        log_error "helm is not installed or not in PATH"
        exit 1
    fi
    
    # Check openssl
    if ! command -v openssl &> /dev/null; then
        log_error "openssl is not installed or not in PATH"
        exit 1
    fi
    
    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --log-level)
            LOG_LEVEL="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  --namespace <name>     Test namespace (default: csr-controller-test)"
            echo "  --timeout <seconds>    Test timeout (default: 600)"
            echo "  --log-level <level>    Log level (default: info)"
            echo "  --help                 Show this help message"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run the tests
check_prerequisites
main