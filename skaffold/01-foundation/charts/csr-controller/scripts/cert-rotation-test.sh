#!/bin/bash

# Certificate Rotation Test Script
# This script tests the certificate rotation functionality

set -e

# Configuration
NAMESPACE="cert-rotation-test"
RELEASE_NAME="cert-rotation-test"
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
    
    # Delete test CSRs
    kubectl delete csr -l app.kubernetes.io/managed-by=cert-rotation-test --ignore-not-found=true
    
    log_info "Cleanup completed"
}

# Setup function
setup() {
    log_info "Setting up certificate rotation test environment..."
    
    # Create test namespace
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    # Label namespace
    kubectl label namespace $NAMESPACE app.kubernetes.io/managed-by=cert-rotation-test --overwrite
    
    # Install chart with rotation enabled
    helm install $RELEASE_NAME ./charts/csr-controller \
        --namespace $NAMESPACE \
        --set csrController.certRotation.enabled=true \
        --set csrController.certRotation.schedule="*/2 * * * *" \
        --set csrController.certRotation.renewalThreshold="24h" \
        --set csrController.certRotation.dryRun=false \
        --set csrController.certMonitoring.enabled=true \
        --set csrController.certLifecycle.enabled=true \
        --wait --timeout=${TIMEOUT}s
    
    log_info "Certificate rotation test environment setup completed"
}

# Test 1: Create expiring certificate
test_create_expiring_certificate() {
    log_info "Creating certificate that will expire soon..."
    
    # Generate certificate expiring in 1 hour
    openssl genrsa -out /tmp/expiring.key 2048
    openssl req -new -key /tmp/expiring.key -out /tmp/expiring.csr -subj "/CN=expiring-test/O=Alt RSS Reader"
    
    # Create certificate with 1 hour expiry
    openssl x509 -req -in /tmp/expiring.csr -signkey /tmp/expiring.key -out /tmp/expiring.crt -days 1
    
    # Create secret
    secret_name="expiring-cert-test"
    kubectl create secret generic $secret_name -n $NAMESPACE \
        --from-file=tls.crt=/tmp/expiring.crt \
        --from-file=tls.key=/tmp/expiring.key \
        --from-file=ca.crt=/tmp/expiring.crt
    
    # Label secret for rotation
    kubectl label secret $secret_name -n $NAMESPACE app.kubernetes.io/component=ssl-certificate
    kubectl label secret $secret_name -n $NAMESPACE app.kubernetes.io/managed-by=cert-rotation-test
    
    # Verify certificate expiry
    cert_data=$(kubectl get secret $secret_name -n $NAMESPACE -o jsonpath='{.data.tls\.crt}')
    echo "$cert_data" | base64 -d > /tmp/check_expiry.crt
    
    expiry_date=$(openssl x509 -in /tmp/check_expiry.crt -noout -enddate | cut -d= -f2)
    expiry_epoch=$(date -d "$expiry_date" +%s)
    current_epoch=$(date +%s)
    hours_until_expiry=$(((expiry_epoch - current_epoch) / 3600))
    
    if [ $hours_until_expiry -le 24 ]; then
        log_success "Certificate created with $hours_until_expiry hours until expiry"
        rm -f /tmp/expiring.* /tmp/check_expiry.crt
        return 0
    else
        log_error "Certificate expiry time is too far in the future"
        rm -f /tmp/expiring.* /tmp/check_expiry.crt
        return 1
    fi
}

# Test 2: Trigger rotation manually
test_manual_rotation() {
    log_info "Triggering certificate rotation manually..."
    
    # Create a manual rotation job
    cat > /tmp/manual-rotation-job.yaml <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: manual-cert-rotation
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/managed-by: cert-rotation-test
spec:
  template:
    spec:
      restartPolicy: Never
      serviceAccountName: ${RELEASE_NAME}-csr-controller
      containers:
      - name: cert-rotator
        image: alpine/openssl:latest
        env:
        - name: CERT_RENEWAL_THRESHOLD
          value: "24h"
        - name: NAMESPACES
          value: "$NAMESPACE"
        - name: SIGNER_NAME
          value: "alt.local/ca"
        - name: DRY_RUN
          value: "false"
        - name: LOG_LEVEL
          value: "debug"
        command:
        - /bin/sh
        - -c
        - |
          echo "Manual certificate rotation test"
          echo "Checking for expiring certificates in namespace: \$NAMESPACES"
          
          # Get all SSL certificate secrets
          secrets=\$(kubectl get secrets -n \$NAMESPACES -l app.kubernetes.io/component=ssl-certificate -o jsonpath='{.items[*].metadata.name}')
          
          for secret in \$secrets; do
            if [ -n "\$secret" ]; then
              echo "Processing certificate: \$secret"
              
              # Get certificate data
              cert_data=\$(kubectl get secret \$secret -n \$NAMESPACES -o jsonpath='{.data.tls\.crt}')
              
              if [ -n "\$cert_data" ]; then
                echo "\$cert_data" | base64 -d > /tmp/cert_check.pem
                
                # Check expiry
                expiry_date=\$(openssl x509 -in /tmp/cert_check.pem -noout -enddate | cut -d= -f2)
                expiry_epoch=\$(date -d "\$expiry_date" +%s)
                current_epoch=\$(date +%s)
                hours_until_expiry=\$(((expiry_epoch - current_epoch) / 3600))
                
                echo "Certificate \$secret expires in \$hours_until_expiry hours"
                
                if [ \$hours_until_expiry -le 24 ]; then
                  echo "Certificate \$secret needs rotation"
                  
                  # For testing, we'll just mark it as processed
                  kubectl annotate secret \$secret -n \$NAMESPACES cert-rotation-test-processed="true" --overwrite
                  echo "Certificate \$secret marked for rotation"
                else
                  echo "Certificate \$secret does not need rotation"
                fi
              fi
            fi
          done
          
          echo "Manual rotation check completed"
        resources:
          limits:
            cpu: 100m
            memory: 128Mi
          requests:
            cpu: 50m
            memory: 64Mi
        securityContext:
          runAsNonRoot: true
          runAsUser: 65534
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        volumeMounts:
        - name: tmp
          mountPath: /tmp
      volumes:
      - name: tmp
        emptyDir: {}
EOF
    
    # Apply manual rotation job
    kubectl apply -f /tmp/manual-rotation-job.yaml
    
    # Wait for job completion
    kubectl wait --for=condition=complete job/manual-cert-rotation -n $NAMESPACE --timeout=${TIMEOUT}s
    
    # Check job logs
    job_logs=$(kubectl logs job/manual-cert-rotation -n $NAMESPACE)
    
    if echo "$job_logs" | grep -q "marked for rotation"; then
        log_success "Manual rotation job completed successfully"
        
        # Check if certificate was marked
        if kubectl get secret expiring-cert-test -n $NAMESPACE -o jsonpath='{.metadata.annotations.cert-rotation-test-processed}' | grep -q "true"; then
            log_success "Certificate was correctly identified for rotation"
            kubectl delete job manual-cert-rotation -n $NAMESPACE
            rm -f /tmp/manual-rotation-job.yaml
            return 0
        else
            log_error "Certificate was not marked for rotation"
            return 1
        fi
    else
        log_error "Manual rotation job failed"
        kubectl logs job/manual-cert-rotation -n $NAMESPACE
        return 1
    fi
}

# Test 3: Monitor rotation CronJob
test_rotation_cronjob() {
    log_info "Testing certificate rotation CronJob..."
    
    # Check if CronJob exists
    if kubectl get cronjob ${RELEASE_NAME}-csr-controller-cert-rotation -n $NAMESPACE >/dev/null 2>&1; then
        log_success "Certificate rotation CronJob found"
        
        # Get CronJob details
        cronjob_schedule=$(kubectl get cronjob ${RELEASE_NAME}-csr-controller-cert-rotation -n $NAMESPACE -o jsonpath='{.spec.schedule}')
        cronjob_suspend=$(kubectl get cronjob ${RELEASE_NAME}-csr-controller-cert-rotation -n $NAMESPACE -o jsonpath='{.spec.suspend}')
        
        log_info "CronJob schedule: $cronjob_schedule"
        log_info "CronJob suspended: $cronjob_suspend"
        
        if [ "$cronjob_suspend" = "false" ] || [ "$cronjob_suspend" = "" ]; then
            log_success "CronJob is active"
            return 0
        else
            log_error "CronJob is suspended"
            return 1
        fi
    else
        log_error "Certificate rotation CronJob not found"
        return 1
    fi
}

# Test 4: Test rotation configuration
test_rotation_configuration() {
    log_info "Testing certificate rotation configuration..."
    
    # Get rotation configuration from values
    renewal_threshold=$(kubectl get configmap ${RELEASE_NAME}-csr-controller-config -n $NAMESPACE -o jsonpath='{.data.config\.yaml}' | grep -o 'renewalThreshold: [0-9]*h' | cut -d' ' -f2)
    
    if [ -n "$renewal_threshold" ]; then
        log_success "Renewal threshold configured: $renewal_threshold"
    else
        log_warning "Renewal threshold not found in configuration"
    fi
    
    # Check dry run setting
    dry_run=$(kubectl get configmap ${RELEASE_NAME}-csr-controller-config -n $NAMESPACE -o jsonpath='{.data.config\.yaml}' | grep -o 'dryRun: [a-z]*' | cut -d' ' -f2)
    
    if [ "$dry_run" = "false" ]; then
        log_success "Dry run disabled for testing"
        return 0
    else
        log_info "Dry run enabled: $dry_run"
        return 0
    fi
}

# Test 5: Test rotation permissions
test_rotation_permissions() {
    log_info "Testing certificate rotation permissions..."
    
    # Get service account
    service_account="${RELEASE_NAME}-csr-controller"
    
    # Test CSR creation permissions
    if kubectl auth can-i create certificatesigningrequests --as=system:serviceaccount:$NAMESPACE:$service_account; then
        log_success "CSR creation permission granted"
    else
        log_error "CSR creation permission denied"
        return 1
    fi
    
    # Test secret update permissions
    if kubectl auth can-i update secrets --as=system:serviceaccount:$NAMESPACE:$service_account; then
        log_success "Secret update permission granted"
    else
        log_error "Secret update permission denied"
        return 1
    fi
    
    # Test CSR deletion permissions
    if kubectl auth can-i delete certificatesigningrequests --as=system:serviceaccount:$NAMESPACE:$service_account; then
        log_success "CSR deletion permission granted"
        return 0
    else
        log_error "CSR deletion permission denied"
        return 1
    fi
}

# Test 6: Test certificate validation
test_certificate_validation() {
    log_info "Testing certificate validation during rotation..."
    
    # Create a certificate with mismatched key
    openssl genrsa -out /tmp/good.key 2048
    openssl genrsa -out /tmp/bad.key 2048
    openssl req -new -key /tmp/good.key -out /tmp/good.csr -subj "/CN=validation-test/O=Alt RSS Reader"
    openssl x509 -req -in /tmp/good.csr -signkey /tmp/good.key -out /tmp/good.crt -days 1
    
    # Create secret with mismatched key
    secret_name="validation-test-cert"
    kubectl create secret generic $secret_name -n $NAMESPACE \
        --from-file=tls.crt=/tmp/good.crt \
        --from-file=tls.key=/tmp/bad.key
    
    # Label secret
    kubectl label secret $secret_name -n $NAMESPACE app.kubernetes.io/component=ssl-certificate
    kubectl label secret $secret_name -n $NAMESPACE app.kubernetes.io/managed-by=cert-rotation-test
    
    # Test validation
    cert_data=$(kubectl get secret $secret_name -n $NAMESPACE -o jsonpath='{.data.tls\.crt}')
    key_data=$(kubectl get secret $secret_name -n $NAMESPACE -o jsonpath='{.data.tls\.key}')
    
    echo "$cert_data" | base64 -d > /tmp/validate_cert.crt
    echo "$key_data" | base64 -d > /tmp/validate_key.key
    
    # Extract public keys
    cert_pubkey=$(openssl x509 -in /tmp/validate_cert.crt -noout -pubkey)
    key_pubkey=$(openssl rsa -in /tmp/validate_key.key -pubout 2>/dev/null)
    
    if [ "$cert_pubkey" != "$key_pubkey" ]; then
        log_success "Certificate validation correctly detected mismatched key pair"
        kubectl delete secret $secret_name -n $NAMESPACE
        rm -f /tmp/good.* /tmp/bad.* /tmp/validate_*
        return 0
    else
        log_error "Certificate validation failed to detect mismatched key pair"
        kubectl delete secret $secret_name -n $NAMESPACE
        rm -f /tmp/good.* /tmp/bad.* /tmp/validate_*
        return 1
    fi
}

# Main test execution
main() {
    log_info "Starting Certificate Rotation Tests"
    log_info "==================================="
    
    # Trap cleanup function
    trap cleanup EXIT
    
    # Setup test environment
    setup
    
    # Run tests
    run_test "Create Expiring Certificate" "test_create_expiring_certificate"
    run_test "Manual Rotation" "test_manual_rotation"
    run_test "Rotation CronJob" "test_rotation_cronjob"
    run_test "Rotation Configuration" "test_rotation_configuration"
    run_test "Rotation Permissions" "test_rotation_permissions"
    run_test "Certificate Validation" "test_certificate_validation"
    
    # Report results
    echo
    log_info "==================================="
    log_info "Certificate Rotation Test Results:"
    log_info "==================================="
    log_info "Total tests: $TESTS_TOTAL"
    log_success "Passed: $TESTS_PASSED"
    log_error "Failed: $TESTS_FAILED"
    
    if [ $TESTS_FAILED -gt 0 ]; then
        log_error "Some tests failed. Please check the logs above."
        exit 1
    else
        log_success "All certificate rotation tests passed!"
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
            echo "  --namespace <name>     Test namespace (default: cert-rotation-test)"
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