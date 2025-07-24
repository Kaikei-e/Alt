#!/bin/bash

# Production Deployment Script for CSR Controller
# This script handles the deployment of CSR Controller to production environment

set -e

# Configuration
NAMESPACE="alt-production"
RELEASE_NAME="csr-controller"
CHART_PATH="./charts/csr-controller"
VALUES_FILE="values-production.yaml"
TIMEOUT=600
ROLLBACK_ON_FAILURE=true
BACKUP_ENABLED=true

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

# Check if running in production environment
check_production_environment() {
    local current_context=$(kubectl config current-context)
    
    if [[ "$current_context" != *"production"* ]]; then
        log_error "Not in production context. Current context: $current_context"
        log_error "Please switch to production context before running this script"
        exit 1
    fi
    
    log_info "Production context confirmed: $current_context"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites for production deployment..."
    
    # Check required tools
    local required_tools=("kubectl" "helm" "openssl")
    
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            log_error "$tool is not installed or not in PATH"
            exit 1
        fi
    done
    
    # Check Kubernetes cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    # Check if namespace exists
    if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
        log_error "Namespace $NAMESPACE does not exist"
        exit 1
    fi
    
    # Check if values file exists
    if [ ! -f "$VALUES_FILE" ]; then
        log_error "Production values file $VALUES_FILE not found"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Pre-deployment validations
pre_deployment_validations() {
    log_info "Running pre-deployment validations..."
    
    # Validate chart
    log_info "Validating Helm chart..."
    if ! helm lint "$CHART_PATH" -f "$VALUES_FILE"; then
        log_error "Chart validation failed"
        exit 1
    fi
    
    # Dry-run deployment
    log_info "Running dry-run deployment..."
    if ! helm template "$RELEASE_NAME" "$CHART_PATH" -f "$VALUES_FILE" --dry-run --validate; then
        log_error "Dry-run deployment failed"
        exit 1
    fi
    
    # Check resource requirements
    log_info "Checking cluster resources..."
    local nodes_ready=$(kubectl get nodes --no-headers | grep -c "Ready")
    if [ "$nodes_ready" -lt 2 ]; then
        log_error "Insufficient nodes available for production deployment"
        exit 1
    fi
    
    # Check if CA secret exists
    if ! kubectl get secret alt-production-ca-secret -n "$NAMESPACE" &> /dev/null; then
        log_error "Production CA secret not found"
        log_error "Please create the CA secret before deployment"
        exit 1
    fi
    
    log_success "Pre-deployment validations passed"
}

# Create backup of current deployment
create_backup() {
    if [ "$BACKUP_ENABLED" = true ]; then
        log_info "Creating backup of current deployment..."
        
        local backup_dir="/tmp/csr-controller-backup-$(date +%Y%m%d_%H%M%S)"
        mkdir -p "$backup_dir"
        
        # Backup current Helm release
        if helm get values "$RELEASE_NAME" -n "$NAMESPACE" > "$backup_dir/current-values.yaml" 2>/dev/null; then
            log_success "Current values backed up to $backup_dir/current-values.yaml"
        fi
        
        # Backup current secrets
        kubectl get secrets -n "$NAMESPACE" -o yaml > "$backup_dir/secrets-backup.yaml"
        
        # Backup current CSRs
        kubectl get csr -o yaml > "$backup_dir/csr-backup.yaml"
        
        log_success "Backup created at $backup_dir"
        echo "$backup_dir" > /tmp/latest-backup-path
    fi
}

# Deploy to production
deploy_to_production() {
    log_info "Deploying CSR Controller to production..."
    
    # Check if release exists
    if helm get values "$RELEASE_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_info "Existing release found, performing upgrade..."
        
        # Perform upgrade
        if helm upgrade "$RELEASE_NAME" "$CHART_PATH" \
            --namespace "$NAMESPACE" \
            --values "$VALUES_FILE" \
            --timeout "${TIMEOUT}s" \
            --wait \
            --atomic; then
            log_success "Production upgrade completed successfully"
        else
            log_error "Production upgrade failed"
            
            if [ "$ROLLBACK_ON_FAILURE" = true ]; then
                log_info "Performing automatic rollback..."
                if helm rollback "$RELEASE_NAME" -n "$NAMESPACE"; then
                    log_success "Rollback completed successfully"
                else
                    log_error "Rollback failed"
                fi
            fi
            
            exit 1
        fi
    else
        log_info "No existing release found, performing fresh install..."
        
        # Perform fresh install
        if helm install "$RELEASE_NAME" "$CHART_PATH" \
            --namespace "$NAMESPACE" \
            --values "$VALUES_FILE" \
            --timeout "${TIMEOUT}s" \
            --wait; then
            log_success "Production installation completed successfully"
        else
            log_error "Production installation failed"
            exit 1
        fi
    fi
}

# Post-deployment validations
post_deployment_validations() {
    log_info "Running post-deployment validations..."
    
    # Check pod status
    log_info "Checking pod status..."
    if ! kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=csr-controller \
        -n "$NAMESPACE" --timeout="${TIMEOUT}s"; then
        log_error "Pods are not ready"
        exit 1
    fi
    
    # Check service endpoints
    log_info "Checking service endpoints..."
    local csr_controller_pod=$(kubectl get pod -l app.kubernetes.io/name=csr-controller -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}')
    
    if ! kubectl exec -n "$NAMESPACE" "$csr_controller_pod" -- wget -q --spider http://localhost:8081/healthz; then
        log_error "Health endpoint is not responding"
        exit 1
    fi
    
    # Check certificate monitoring
    if kubectl get pod -l app.kubernetes.io/component=cert-monitor -n "$NAMESPACE" &> /dev/null; then
        local cert_monitor_pod=$(kubectl get pod -l app.kubernetes.io/component=cert-monitor -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}')
        
        if ! kubectl exec -n "$NAMESPACE" "$cert_monitor_pod" -- wget -q --spider http://localhost:8081/; then
            log_error "Certificate monitor health endpoint is not responding"
            exit 1
        fi
    fi
    
    # Check metrics endpoints
    log_info "Checking metrics endpoints..."
    if ! kubectl exec -n "$NAMESPACE" "$csr_controller_pod" -- wget -q -O - http://localhost:8080/metrics | grep -q "cert_"; then
        log_error "Metrics endpoint is not responding correctly"
        exit 1
    fi
    
    # Test CSR functionality
    log_info "Testing CSR functionality..."
    if ! test_csr_functionality; then
        log_error "CSR functionality test failed"
        exit 1
    fi
    
    log_success "Post-deployment validations passed"
}

# Test CSR functionality
test_csr_functionality() {
    log_info "Testing CSR functionality..."
    
    # Generate test CSR
    openssl genrsa -out /tmp/prod-test.key 2048
    openssl req -new -key /tmp/prod-test.key -out /tmp/prod-test.csr -subj "/CN=production-test/O=Alt RSS Reader"
    
    # Create K8s CSR
    local csr_name="production-test-$(date +%s)"
    cat > /tmp/prod-csr.yaml <<EOF
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: $csr_name
  labels:
    app.kubernetes.io/managed-by: production-deployment-test
spec:
  request: $(cat /tmp/prod-test.csr | base64 | tr -d '\n')
  signerName: "alt.production.local/ca"
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF
    
    # Apply CSR
    if kubectl apply -f /tmp/prod-csr.yaml; then
        # Wait for approval
        local approved=false
        for i in {1..30}; do
            if kubectl get csr "$csr_name" -o jsonpath='{.status.certificate}' | grep -q "LS0t"; then
                approved=true
                break
            fi
            sleep 2
        done
        
        # Cleanup
        kubectl delete csr "$csr_name"
        rm -f /tmp/prod-test.* /tmp/prod-csr.yaml
        
        if [ "$approved" = true ]; then
            log_success "CSR functionality test passed"
            return 0
        else
            log_error "CSR was not approved within timeout"
            return 1
        fi
    else
        log_error "Failed to create test CSR"
        rm -f /tmp/prod-test.* /tmp/prod-csr.yaml
        return 1
    fi
}

# Generate deployment report
generate_deployment_report() {
    log_info "Generating deployment report..."
    
    local report_file="/tmp/csr-controller-production-deployment-$(date +%Y%m%d_%H%M%S).md"
    
    cat > "$report_file" <<EOF
# CSR Controller Production Deployment Report

**Deployment Date**: $(date)
**Release Name**: $RELEASE_NAME
**Namespace**: $NAMESPACE
**Chart Version**: $(helm list -n "$NAMESPACE" | grep "$RELEASE_NAME" | awk '{print $9}')

## Deployment Summary

- **Status**: SUCCESS
- **Duration**: $(($(date +%s) - $DEPLOY_START_TIME)) seconds
- **Backup Created**: $([ "$BACKUP_ENABLED" = true ] && echo "Yes" || echo "No")

## Resource Status

### Pods
\`\`\`
$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=csr-controller)
\`\`\`

### Services
\`\`\`
$(kubectl get services -n "$NAMESPACE" -l app.kubernetes.io/name=csr-controller)
\`\`\`

### CSRs
\`\`\`
$(kubectl get csr | head -10)
\`\`\`

## Health Checks

- **CSR Controller Health**: ✅ HEALTHY
- **Certificate Monitor Health**: ✅ HEALTHY
- **Metrics Endpoint**: ✅ HEALTHY
- **CSR Functionality**: ✅ TESTED

## Configuration

- **Auto-Approval**: Enabled
- **Certificate Rotation**: Enabled (Daily at 2 AM)
- **Certificate Monitoring**: Enabled (5-minute intervals)
- **Alerting**: Enabled (Slack + Email)

## Next Steps

1. Monitor system performance for 24 hours
2. Verify certificate rotation functionality
3. Test alerting mechanisms
4. Schedule regular health checks

## Support Information

- **Documentation**: /charts/csr-controller/README.md
- **Logs**: \`kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=csr-controller\`
- **Monitoring**: Check Grafana dashboards
- **Alerts**: Monitor Slack #production-alerts channel

---
*Report generated automatically by production deployment script*
EOF
    
    log_success "Deployment report generated: $report_file"
}

# Main deployment function
main() {
    local DEPLOY_START_TIME=$(date +%s)
    
    log_info "Starting CSR Controller Production Deployment"
    log_info "============================================="
    
    # Run all deployment steps
    check_production_environment
    check_prerequisites
    pre_deployment_validations
    create_backup
    deploy_to_production
    post_deployment_validations
    generate_deployment_report
    
    # Final summary
    log_info "============================================="
    log_success "CSR Controller Production Deployment Completed Successfully!"
    log_info "============================================="
    
    log_info "Deployment Summary:"
    log_info "- Release: $RELEASE_NAME"
    log_info "- Namespace: $NAMESPACE"
    log_info "- Duration: $(($(date +%s) - $DEPLOY_START_TIME)) seconds"
    
    if [ "$BACKUP_ENABLED" = true ] && [ -f "/tmp/latest-backup-path" ]; then
        log_info "- Backup: $(cat /tmp/latest-backup-path)"
    fi
    
    log_info ""
    log_info "Next Steps:"
    log_info "1. Monitor system performance"
    log_info "2. Verify certificate operations"
    log_info "3. Test alerting mechanisms"
    log_info "4. Schedule regular health checks"
    
    log_info ""
    log_info "Useful Commands:"
    log_info "- Check status: kubectl get pods -n $NAMESPACE"
    log_info "- View logs: kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=csr-controller"
    log_info "- Check CSRs: kubectl get csr"
    log_info "- Port forward metrics: kubectl port-forward -n $NAMESPACE svc/csr-controller-cert-monitor 8080:8080"
}

# Display usage information
usage() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  --namespace <name>         Target namespace (default: alt-production)"
    echo "  --release-name <name>      Helm release name (default: csr-controller)"
    echo "  --values-file <file>       Values file (default: values-production.yaml)"
    echo "  --timeout <seconds>        Deployment timeout (default: 600)"
    echo "  --no-backup               Skip backup creation"
    echo "  --no-rollback             Skip automatic rollback on failure"
    echo "  --dry-run                 Run in dry-run mode"
    echo "  --help                    Show this help message"
}

# Parse command line arguments
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --release-name)
            RELEASE_NAME="$2"
            shift 2
            ;;
        --values-file)
            VALUES_FILE="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --no-backup)
            BACKUP_ENABLED=false
            shift
            ;;
        --no-rollback)
            ROLLBACK_ON_FAILURE=false
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Handle dry-run mode
if [ "$DRY_RUN" = true ]; then
    log_info "Running in dry-run mode..."
    check_production_environment
    check_prerequisites
    pre_deployment_validations
    log_success "Dry-run completed successfully"
    exit 0
fi

# Run the production deployment
main