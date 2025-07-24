#!/bin/bash

# Certificate Monitoring and Alerting Test Script
# This script tests the certificate monitoring and alerting functionality

set -e

# Configuration
NAMESPACE="cert-monitoring-test"
RELEASE_NAME="cert-monitoring-test"
TIMEOUT=300
LOG_LEVEL="debug"

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
    
    log_info "Cleanup completed"
}

# Setup function
setup() {
    log_info "Setting up certificate monitoring test environment..."
    
    # Create test namespace
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    # Label namespace
    kubectl label namespace $NAMESPACE app.kubernetes.io/managed-by=cert-monitoring-test --overwrite
    
    # Install chart with monitoring enabled
    helm install $RELEASE_NAME ./charts/csr-controller \
        --namespace $NAMESPACE \
        --set csrController.certMonitoring.enabled=true \
        --set csrController.certMonitoring.interval=30 \
        --set csrController.certMonitoring.alertThreshold="72h" \
        --set csrController.certMonitoring.logLevel="debug" \
        --set csrController.certLifecycle.enabled=true \
        --set csrController.certRotation.enabled=false \
        --wait --timeout=${TIMEOUT}s
    
    log_info "Certificate monitoring test environment setup completed"
}

# Test 1: Check monitoring pod deployment
test_monitoring_deployment() {
    log_info "Testing certificate monitoring deployment..."
    
    # Check if monitoring pod exists
    if kubectl get pod -l app.kubernetes.io/component=cert-monitor -n $NAMESPACE >/dev/null 2>&1; then
        log_success "Certificate monitoring pod found"
        
        # Wait for pod to be ready
        kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=cert-monitor \
            -n $NAMESPACE --timeout=${TIMEOUT}s
        
        if [ $? -eq 0 ]; then
            log_success "Certificate monitoring pod is ready"
            return 0
        else
            log_error "Certificate monitoring pod is not ready"
            return 1
        fi
    else
        log_error "Certificate monitoring pod not found"
        return 1
    fi
}

# Test 2: Check monitoring service
test_monitoring_service() {
    log_info "Testing certificate monitoring service..."
    
    # Check if monitoring service exists
    if kubectl get service ${RELEASE_NAME}-csr-controller-cert-monitor -n $NAMESPACE >/dev/null 2>&1; then
        log_success "Certificate monitoring service found"
        
        # Get service details
        service_type=$(kubectl get service ${RELEASE_NAME}-csr-controller-cert-monitor -n $NAMESPACE -o jsonpath='{.spec.type}')
        metrics_port=$(kubectl get service ${RELEASE_NAME}-csr-controller-cert-monitor -n $NAMESPACE -o jsonpath='{.spec.ports[?(@.name=="metrics")].port}')
        health_port=$(kubectl get service ${RELEASE_NAME}-csr-controller-cert-monitor -n $NAMESPACE -o jsonpath='{.spec.ports[?(@.name=="health")].port}')
        
        log_info "Service type: $service_type"
        log_info "Metrics port: $metrics_port"
        log_info "Health port: $health_port"
        
        if [ -n "$metrics_port" ] && [ -n "$health_port" ]; then
            log_success "Service configured correctly"
            return 0
        else
            log_error "Service configuration incomplete"
            return 1
        fi
    else
        log_error "Certificate monitoring service not found"
        return 1
    fi
}

# Test 3: Test health endpoint
test_health_endpoint() {
    log_info "Testing certificate monitoring health endpoint..."
    
    # Get monitoring pod name
    monitor_pod=$(kubectl get pod -l app.kubernetes.io/component=cert-monitor -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if [ -n "$monitor_pod" ]; then
        # Test health endpoint
        if kubectl exec -n $NAMESPACE $monitor_pod -- wget -q --spider http://localhost:8081/; then
            log_success "Health endpoint is responding"
            return 0
        else
            log_error "Health endpoint is not responding"
            # Try to get more info about the pod
            kubectl describe pod $monitor_pod -n $NAMESPACE
            kubectl logs $monitor_pod -n $NAMESPACE
            return 1
        fi
    else
        log_error "Certificate monitoring pod not found"
        return 1
    fi
}

# Test 4: Test metrics endpoint
test_metrics_endpoint() {
    log_info "Testing certificate monitoring metrics endpoint..."
    
    # Get monitoring pod name
    monitor_pod=$(kubectl get pod -l app.kubernetes.io/component=cert-monitor -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if [ -n "$monitor_pod" ]; then
        # Test metrics endpoint
        metrics_output=$(kubectl exec -n $NAMESPACE $monitor_pod -- wget -q -O - http://localhost:8080/ 2>/dev/null)
        
        if echo "$metrics_output" | grep -q "cert_monitor_running"; then
            log_success "Metrics endpoint is responding with certificate metrics"
            log_info "Sample metrics:"
            echo "$metrics_output" | head -5
            return 0
        else
            log_error "Metrics endpoint is not responding with expected metrics"
            log_info "Actual output:"
            echo "$metrics_output"
            return 1
        fi
    else
        log_error "Certificate monitoring pod not found"
        return 1
    fi
}

# Test 5: Create test certificates for monitoring
test_certificate_creation() {
    log_info "Creating test certificates for monitoring..."
    
    # Create healthy certificate (valid for 30 days)
    openssl genrsa -out /tmp/healthy.key 2048
    openssl req -new -key /tmp/healthy.key -out /tmp/healthy.csr -subj "/CN=healthy-cert/O=Alt RSS Reader"
    openssl x509 -req -in /tmp/healthy.csr -signkey /tmp/healthy.key -out /tmp/healthy.crt -days 30
    
    # Create expiring certificate (valid for 1 day)
    openssl genrsa -out /tmp/expiring.key 2048
    openssl req -new -key /tmp/expiring.key -out /tmp/expiring.csr -subj "/CN=expiring-cert/O=Alt RSS Reader"
    openssl x509 -req -in /tmp/expiring.csr -signkey /tmp/expiring.key -out /tmp/expiring.crt -days 1
    
    # Create expired certificate (expired 1 day ago)
    openssl genrsa -out /tmp/expired.key 2048
    openssl req -new -key /tmp/expired.key -out /tmp/expired.csr -subj "/CN=expired-cert/O=Alt RSS Reader"
    openssl x509 -req -in /tmp/expired.csr -signkey /tmp/expired.key -out /tmp/expired.crt -days -1
    
    # Create secrets
    kubectl create secret generic healthy-cert-test -n $NAMESPACE \
        --from-file=tls.crt=/tmp/healthy.crt \
        --from-file=tls.key=/tmp/healthy.key \
        --from-file=ca.crt=/tmp/healthy.crt
    
    kubectl create secret generic expiring-cert-test -n $NAMESPACE \
        --from-file=tls.crt=/tmp/expiring.crt \
        --from-file=tls.key=/tmp/expiring.key \
        --from-file=ca.crt=/tmp/expiring.crt
    
    kubectl create secret generic expired-cert-test -n $NAMESPACE \
        --from-file=tls.crt=/tmp/expired.crt \
        --from-file=tls.key=/tmp/expired.key \
        --from-file=ca.crt=/tmp/expired.crt
    
    # Label secrets for monitoring
    kubectl label secret healthy-cert-test -n $NAMESPACE app.kubernetes.io/component=ssl-certificate
    kubectl label secret expiring-cert-test -n $NAMESPACE app.kubernetes.io/component=ssl-certificate
    kubectl label secret expired-cert-test -n $NAMESPACE app.kubernetes.io/component=ssl-certificate
    
    kubectl label secret healthy-cert-test -n $NAMESPACE app.kubernetes.io/managed-by=cert-monitoring-test
    kubectl label secret expiring-cert-test -n $NAMESPACE app.kubernetes.io/managed-by=cert-monitoring-test
    kubectl label secret expired-cert-test -n $NAMESPACE app.kubernetes.io/managed-by=cert-monitoring-test
    
    log_success "Test certificates created successfully"
    rm -f /tmp/healthy.* /tmp/expiring.* /tmp/expired.*
    return 0
}

# Test 6: Test certificate monitoring detection
test_certificate_monitoring() {
    log_info "Testing certificate monitoring detection..."
    
    # Wait for monitoring cycle to run
    log_info "Waiting for monitoring cycle to detect certificates..."
    sleep 45
    
    # Check metrics for certificate detection
    monitor_pod=$(kubectl get pod -l app.kubernetes.io/component=cert-monitor -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if [ -n "$monitor_pod" ]; then
        metrics_output=$(kubectl exec -n $NAMESPACE $monitor_pod -- wget -q -O - http://localhost:8080/ 2>/dev/null)
        
        # Check if certificates are detected
        if echo "$metrics_output" | grep -q "cert_monitor_total_certificates"; then
            total_certs=$(echo "$metrics_output" | grep "cert_monitor_total_certificates" | awk '{print $2}')
            log_success "Monitoring detected $total_certs certificates"
            
            # Check for expiring certificates
            if echo "$metrics_output" | grep -q "cert_monitor_expiring_certificates"; then
                expiring_certs=$(echo "$metrics_output" | grep "cert_monitor_expiring_certificates" | awk '{print $2}')
                log_success "Monitoring detected $expiring_certs expiring certificates"
                
                if [ "$expiring_certs" -gt 0 ]; then
                    log_success "Certificate monitoring is working correctly"
                    return 0
                else
                    log_warning "No expiring certificates detected (this may be expected)"
                    return 0
                fi
            else
                log_warning "Expiring certificates metric not found"
                return 0
            fi
        else
            log_error "Certificate monitoring metrics not found"
            log_info "Available metrics:"
            echo "$metrics_output"
            return 1
        fi
    else
        log_error "Certificate monitoring pod not found"
        return 1
    fi
}

# Test 7: Test monitoring configuration
test_monitoring_configuration() {
    log_info "Testing certificate monitoring configuration..."
    
    # Check ConfigMap
    if kubectl get configmap ${RELEASE_NAME}-csr-controller-cert-monitor-config -n $NAMESPACE >/dev/null 2>&1; then
        log_success "Monitoring configuration ConfigMap found"
        
        # Check configuration content
        config_content=$(kubectl get configmap ${RELEASE_NAME}-csr-controller-cert-monitor-config -n $NAMESPACE -o jsonpath='{.data.monitoring\.yaml}')
        
        if echo "$config_content" | grep -q "interval:"; then
            interval=$(echo "$config_content" | grep "interval:" | awk '{print $2}')
            log_success "Monitoring interval configured: $interval seconds"
        else
            log_warning "Monitoring interval not found in configuration"
        fi
        
        if echo "$config_content" | grep -q "alertThreshold:"; then
            threshold=$(echo "$config_content" | grep "alertThreshold:" | awk '{print $2}')
            log_success "Alert threshold configured: $threshold"
        else
            log_warning "Alert threshold not found in configuration"
        fi
        
        return 0
    else
        log_error "Monitoring configuration ConfigMap not found"
        return 1
    fi
}

# Test 8: Test monitoring logs
test_monitoring_logs() {
    log_info "Testing certificate monitoring logs..."
    
    # Get monitoring pod name
    monitor_pod=$(kubectl get pod -l app.kubernetes.io/component=cert-monitor -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')
    
    if [ -n "$monitor_pod" ]; then
        # Get recent logs
        logs_output=$(kubectl logs $monitor_pod -n $NAMESPACE --tail=50)
        
        if echo "$logs_output" | grep -q "Starting certificate monitoring"; then
            log_success "Monitoring service started successfully"
            
            # Check for certificate processing logs
            if echo "$logs_output" | grep -q "Processing certificate"; then
                log_success "Certificate processing logs found"
                return 0
            else
                log_warning "Certificate processing logs not found yet"
                return 0
            fi
        else
            log_error "Monitoring service startup logs not found"
            log_info "Recent logs:"
            echo "$logs_output"
            return 1
        fi
    else
        log_error "Certificate monitoring pod not found"
        return 1
    fi
}

# Test 9: Test alert configuration
test_alert_configuration() {
    log_info "Testing alert configuration..."
    
    # Check if alert configuration exists
    if kubectl get configmap ${RELEASE_NAME}-csr-controller-cert-monitor-config -n $NAMESPACE >/dev/null 2>&1; then
        config_content=$(kubectl get configmap ${RELEASE_NAME}-csr-controller-cert-monitor-config -n $NAMESPACE -o jsonpath='{.data.monitoring\.yaml}')
        
        # Check for alerting configuration
        if echo "$config_content" | grep -q "alerting:"; then
            log_success "Alerting configuration found"
            
            # Check for Slack configuration
            if echo "$config_content" | grep -q "slack:"; then
                log_success "Slack alerting configured"
            else
                log_warning "Slack alerting not configured"
            fi
            
            # Check for email configuration
            if echo "$config_content" | grep -q "email:"; then
                log_success "Email alerting configured"
            else
                log_warning "Email alerting not configured"
            fi
            
            return 0
        else
            log_warning "Alerting configuration not found"
            return 0
        fi
    else
        log_error "Monitoring configuration not found"
        return 1
    fi
}

# Test 10: Test prometheus integration
test_prometheus_integration() {
    log_info "Testing Prometheus integration..."
    
    # Check service annotations
    service_annotations=$(kubectl get service ${RELEASE_NAME}-csr-controller-cert-monitor -n $NAMESPACE -o jsonpath='{.metadata.annotations}')
    
    if echo "$service_annotations" | grep -q "prometheus.io/scrape"; then
        log_success "Prometheus scrape annotation found"
        
        if echo "$service_annotations" | grep -q "prometheus.io/port"; then
            log_success "Prometheus port annotation found"
            
            if echo "$service_annotations" | grep -q "prometheus.io/path"; then
                log_success "Prometheus path annotation found"
                return 0
            else
                log_warning "Prometheus path annotation not found"
                return 0
            fi
        else
            log_warning "Prometheus port annotation not found"
            return 0
        fi
    else
        log_warning "Prometheus scrape annotation not found"
        return 0
    fi
}

# Main test execution
main() {
    log_info "Starting Certificate Monitoring and Alerting Tests"
    log_info "================================================"
    
    # Trap cleanup function
    trap cleanup EXIT
    
    # Setup test environment
    setup
    
    # Run tests
    run_test "Monitoring Deployment" "test_monitoring_deployment"
    run_test "Monitoring Service" "test_monitoring_service"
    run_test "Health Endpoint" "test_health_endpoint"
    run_test "Metrics Endpoint" "test_metrics_endpoint"
    run_test "Certificate Creation" "test_certificate_creation"
    run_test "Certificate Monitoring" "test_certificate_monitoring"
    run_test "Monitoring Configuration" "test_monitoring_configuration"
    run_test "Monitoring Logs" "test_monitoring_logs"
    run_test "Alert Configuration" "test_alert_configuration"
    run_test "Prometheus Integration" "test_prometheus_integration"
    
    # Report results
    echo
    log_info "================================================"
    log_info "Certificate Monitoring Test Results:"
    log_info "================================================"
    log_info "Total tests: $TESTS_TOTAL"
    log_success "Passed: $TESTS_PASSED"
    log_error "Failed: $TESTS_FAILED"
    
    if [ $TESTS_FAILED -gt 0 ]; then
        log_error "Some tests failed. Please check the logs above."
        exit 1
    else
        log_success "All certificate monitoring tests passed!"
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
        --help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  --namespace <name>     Test namespace (default: cert-monitoring-test)"
            echo "  --timeout <seconds>    Test timeout (default: 300)"
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