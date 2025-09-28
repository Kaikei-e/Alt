#!/bin/bash

# Legacy Certificate System Migration Script
# This script migrates from legacy certificate management to CSR Controller

set -e

# Configuration
LEGACY_NAMESPACE="default"
TARGET_NAMESPACE="alt-production"
MIGRATION_BATCH_SIZE=5
MIGRATION_TIMEOUT=300
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

# Migration state tracking
MIGRATION_STATE_FILE="/tmp/csr-controller-migration-state.json"

# Initialize migration state
init_migration_state() {
    cat > "$MIGRATION_STATE_FILE" <<EOF
{
  "migration_id": "$(date +%Y%m%d_%H%M%S)",
  "start_time": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "in_progress",
  "steps": {
    "discovery": "pending",
    "backup": "pending",
    "analysis": "pending",
    "migration": "pending",
    "validation": "pending",
    "cleanup": "pending"
  },
  "certificates": [],
  "errors": []
}
EOF
    log_info "Migration state initialized: $MIGRATION_STATE_FILE"
}

# Update migration state
update_migration_state() {
    local step=$1
    local status=$2
    local message=$3
    
    # Update state using jq (if available) or simple replacement
    if command -v jq &> /dev/null; then
        local temp_file=$(mktemp)
        jq ".steps.${step} = \"${status}\"" "$MIGRATION_STATE_FILE" > "$temp_file"
        mv "$temp_file" "$MIGRATION_STATE_FILE"
    fi
    
    log_info "Migration step '$step' updated to '$status'"
    if [ -n "$message" ]; then
        log_info "$message"
    fi
}

# Discover legacy certificates
discover_legacy_certificates() {
    log_info "Discovering legacy certificates..."
    update_migration_state "discovery" "in_progress"
    
    local legacy_certs_file="/tmp/legacy-certificates.json"
    
    # Find all SSL-related secrets
    kubectl get secrets --all-namespaces -o json | \
        jq '.items[] | select(.type == "kubernetes.io/tls" or (.data | has("tls.crt")) or (.data | has("ca.crt"))) | {
            name: .metadata.name,
            namespace: .metadata.namespace,
            type: .type,
            data: .data | keys,
            created: .metadata.creationTimestamp
        }' > "$legacy_certs_file"
    
    # Find common-ssl related secrets
    kubectl get secrets --all-namespaces -l app.kubernetes.io/name=common-ssl -o json | \
        jq '.items[] | {
            name: .metadata.name,
            namespace: .metadata.namespace,
            type: .type,
            data: .data | keys,
            created: .metadata.creationTimestamp,
            labels: .metadata.labels
        }' >> "$legacy_certs_file"
    
    # Find manually created SSL secrets
    kubectl get secrets --all-namespaces -o json | \
        jq '.items[] | select(.metadata.name | contains("ssl") or contains("tls") or contains("cert")) | {
            name: .metadata.name,
            namespace: .metadata.namespace,
            type: .type,
            data: .data | keys,
            created: .metadata.creationTimestamp
        }' >> "$legacy_certs_file"
    
    local cert_count=$(cat "$legacy_certs_file" | wc -l)
    log_success "Discovered $cert_count potential certificate secrets"
    
    update_migration_state "discovery" "completed" "Found $cert_count certificate secrets"
}

# Analyze certificate compatibility
analyze_certificate_compatibility() {
    log_info "Analyzing certificate compatibility..."
    update_migration_state "analysis" "in_progress"
    
    local analysis_report="/tmp/certificate-analysis.md"
    
    cat > "$analysis_report" <<EOF
# Certificate Migration Analysis Report

**Generated**: $(date)
**Migration ID**: $(cat "$MIGRATION_STATE_FILE" | jq -r '.migration_id')

## Certificate Inventory

EOF
    
    # Analyze each certificate
    local compatible_count=0
    local incompatible_count=0
    local warning_count=0
    
    kubectl get secrets --all-namespaces -o json | \
        jq -r '.items[] | select(.type == "kubernetes.io/tls" or (.data | has("tls.crt"))) | "\(.metadata.namespace) \(.metadata.name)"' | \
        while read -r namespace name; do
            log_info "Analyzing certificate: $namespace/$name"
            
            # Get certificate data
            local cert_data=$(kubectl get secret "$name" -n "$namespace" -o jsonpath='{.data.tls\.crt}' 2>/dev/null)
            
            if [ -n "$cert_data" ]; then
                # Decode and analyze certificate
                echo "$cert_data" | base64 -d > "/tmp/cert_analysis.pem"
                
                if openssl x509 -in "/tmp/cert_analysis.pem" -noout -text &>/dev/null; then
                    # Extract certificate information
                    local subject=$(openssl x509 -in "/tmp/cert_analysis.pem" -noout -subject | cut -d= -f2-)
                    local issuer=$(openssl x509 -in "/tmp/cert_analysis.pem" -noout -issuer | cut -d= -f2-)
                    local not_after=$(openssl x509 -in "/tmp/cert_analysis.pem" -noout -enddate | cut -d= -f2)
                    local san_names=$(openssl x509 -in "/tmp/cert_analysis.pem" -noout -text | grep -A1 "Subject Alternative Name" | tail -n1 | tr ',' '\n' | sed 's/DNS://g' | tr -d ' ')
                    
                    # Check expiry
                    local expiry_epoch=$(date -d "$not_after" +%s)
                    local current_epoch=$(date +%s)
                    local days_until_expiry=$(((expiry_epoch - current_epoch) / 86400))
                    
                    cat >> "$analysis_report" <<EOF

### Certificate: $namespace/$name

- **Subject**: $subject
- **Issuer**: $issuer
- **Expires**: $not_after ($days_until_expiry days)
- **SAN Names**: 
EOF
                    echo "$san_names" | while read -r san; do
                        if [ -n "$san" ]; then
                            echo "  - $san" >> "$analysis_report"
                        fi
                    done
                    
                    # Determine compatibility
                    if [ $days_until_expiry -lt 30 ]; then
                        echo "- **Status**: ⚠️ EXPIRING SOON - Will be renewed during migration" >> "$analysis_report"
                        warning_count=$((warning_count + 1))
                    elif [[ "$issuer" == *"kubernetes"* ]] || [[ "$issuer" == *"cluster"* ]]; then
                        echo "- **Status**: ✅ COMPATIBLE - Kubernetes-issued certificate" >> "$analysis_report"
                        compatible_count=$((compatible_count + 1))
                    else
                        echo "- **Status**: ❌ EXTERNAL CA - May need manual intervention" >> "$analysis_report"
                        incompatible_count=$((incompatible_count + 1))
                    fi
                else
                    echo "- **Status**: ❌ INVALID - Certificate format error" >> "$analysis_report"
                    incompatible_count=$((incompatible_count + 1))
                fi
                
                rm -f "/tmp/cert_analysis.pem"
            else
                echo "- **Status**: ❌ NO CERTIFICATE DATA" >> "$analysis_report"
                incompatible_count=$((incompatible_count + 1))
            fi
        done
    
    cat >> "$analysis_report" <<EOF

## Migration Summary

- **Compatible Certificates**: $compatible_count
- **Certificates with Warnings**: $warning_count
- **Incompatible Certificates**: $incompatible_count
- **Total Certificates**: $((compatible_count + warning_count + incompatible_count))

## Recommended Actions

1. **Compatible certificates** will be migrated automatically
2. **Expiring certificates** will be renewed during migration
3. **Incompatible certificates** require manual review and intervention

## Migration Strategy

### Phase 1: Preparation
- Backup all existing certificates
- Create new CSR Controller deployment
- Verify CSR Controller functionality

### Phase 2: Migration
- Migrate compatible certificates in batches
- Renew expiring certificates
- Create CSRs for new certificate requests

### Phase 3: Validation
- Verify all migrated certificates
- Test certificate functionality
- Update application configurations

### Phase 4: Cleanup
- Remove legacy certificate resources
- Update deployment configurations
- Monitor certificate health

EOF
    
    log_success "Certificate analysis completed: $analysis_report"
    update_migration_state "analysis" "completed" "Analysis report generated"
}

# Backup legacy certificates
backup_legacy_certificates() {
    if [ "$BACKUP_ENABLED" = true ]; then
        log_info "Creating backup of legacy certificates..."
        update_migration_state "backup" "in_progress"
        
        local backup_dir="/tmp/legacy-certificates-backup-$(date +%Y%m%d_%H%M%S)"
        mkdir -p "$backup_dir"
        
        # Backup all SSL-related secrets
        kubectl get secrets --all-namespaces -o json | \
            jq '.items[] | select(.type == "kubernetes.io/tls" or (.data | has("tls.crt")) or (.data | has("ca.crt")))' | \
            jq -s '.' > "$backup_dir/ssl-secrets-backup.json"
        
        # Backup common-ssl secrets
        kubectl get secrets --all-namespaces -l app.kubernetes.io/name=common-ssl -o json > "$backup_dir/common-ssl-secrets-backup.json"
        
        # Backup CSRs
        kubectl get csr -o json > "$backup_dir/csr-backup.json"
        
        # Create restoration script
        cat > "$backup_dir/restore-legacy-certificates.sh" <<EOF
#!/bin/bash
# Legacy Certificate Restoration Script
# Generated: $(date)

set -e

echo "Restoring legacy certificates from backup..."

# Restore SSL secrets
kubectl apply -f ssl-secrets-backup.json

# Restore common-ssl secrets  
kubectl apply -f common-ssl-secrets-backup.json

# Restore CSRs
kubectl apply -f csr-backup.json

echo "Legacy certificate restoration completed"
EOF
        
        chmod +x "$backup_dir/restore-legacy-certificates.sh"
        
        log_success "Legacy certificates backed up to: $backup_dir"
        echo "$backup_dir" > "/tmp/legacy-backup-path"
        
        update_migration_state "backup" "completed" "Backup created at $backup_dir"
    fi
}

# Migrate certificates in batches
migrate_certificates() {
    log_info "Starting certificate migration..."
    update_migration_state "migration" "in_progress"
    
    local migration_log="/tmp/certificate-migration.log"
    echo "Certificate Migration Log - $(date)" > "$migration_log"
    
    # Get list of certificates to migrate
    local cert_list="/tmp/certificates-to-migrate.txt"
    kubectl get secrets --all-namespaces -o json | \
        jq -r '.items[] | select(.type == "kubernetes.io/tls" or (.data | has("tls.crt"))) | "\(.metadata.namespace) \(.metadata.name)"' > "$cert_list"
    
    local total_certs=$(wc -l < "$cert_list")
    local migrated_certs=0
    local failed_certs=0
    
    log_info "Found $total_certs certificates to migrate"
    
    # Process certificates in batches
    while IFS= read -r cert_info; do
        local namespace=$(echo "$cert_info" | awk '{print $1}')
        local name=$(echo "$cert_info" | awk '{print $2}')
        
        log_info "Migrating certificate: $namespace/$name"
        
        if migrate_single_certificate "$namespace" "$name"; then
            migrated_certs=$((migrated_certs + 1))
            echo "SUCCESS: $namespace/$name" >> "$migration_log"
        else
            failed_certs=$((failed_certs + 1))
            echo "FAILED: $namespace/$name" >> "$migration_log"
        fi
        
        # Batch processing pause
        if [ $((migrated_certs % MIGRATION_BATCH_SIZE)) -eq 0 ]; then
            log_info "Processed $migrated_certs certificates, pausing for 10 seconds..."
            sleep 10
        fi
        
    done < "$cert_list"
    
    log_info "Migration completed: $migrated_certs succeeded, $failed_certs failed"
    update_migration_state "migration" "completed" "Migrated $migrated_certs certificates"
}

# Migrate single certificate
migrate_single_certificate() {
    local namespace=$1
    local name=$2
    
    # Get certificate data
    local cert_data=$(kubectl get secret "$name" -n "$namespace" -o jsonpath='{.data.tls\.crt}' 2>/dev/null)
    local key_data=$(kubectl get secret "$name" -n "$namespace" -o jsonpath='{.data.tls\.key}' 2>/dev/null)
    
    if [ -z "$cert_data" ] || [ -z "$key_data" ]; then
        log_warning "Certificate $namespace/$name has missing data, skipping"
        return 1
    fi
    
    # Decode and validate certificate
    echo "$cert_data" | base64 -d > "/tmp/migrate_cert.pem"
    echo "$key_data" | base64 -d > "/tmp/migrate_key.pem"
    
    if ! openssl x509 -in "/tmp/migrate_cert.pem" -noout -text &>/dev/null; then
        log_warning "Certificate $namespace/$name has invalid format, skipping"
        rm -f "/tmp/migrate_cert.pem" "/tmp/migrate_key.pem"
        return 1
    fi
    
    # Check if certificate is expiring soon
    local not_after=$(openssl x509 -in "/tmp/migrate_cert.pem" -noout -enddate | cut -d= -f2)
    local expiry_epoch=$(date -d "$not_after" +%s)
    local current_epoch=$(date +%s)
    local days_until_expiry=$(((expiry_epoch - current_epoch) / 86400))
    
    if [ $days_until_expiry -lt 30 ]; then
        log_info "Certificate $namespace/$name expires in $days_until_expiry days, will generate new CSR"
        
        # Generate new CSR for expiring certificate
        local service_name=$(echo "$name" | sed 's/-ssl-certs.*$//' | sed 's/-tls.*$//')
        
        if generate_csr_for_service "$service_name" "$namespace"; then
            log_success "New CSR generated for $namespace/$name"
        else
            log_error "Failed to generate CSR for $namespace/$name"
            rm -f "/tmp/migrate_cert.pem" "/tmp/migrate_key.pem"
            return 1
        fi
    else
        log_info "Certificate $namespace/$name is valid, preserving existing certificate"
        
        # Add labels for CSR Controller management
        kubectl label secret "$name" -n "$namespace" app.kubernetes.io/managed-by=csr-controller --overwrite
        kubectl label secret "$name" -n "$namespace" app.kubernetes.io/component=ssl-certificate --overwrite
        kubectl annotate secret "$name" -n "$namespace" csr-controller/migrated-from=legacy --overwrite
        kubectl annotate secret "$name" -n "$namespace" csr-controller/migration-date="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
    fi
    
    rm -f "/tmp/migrate_cert.pem" "/tmp/migrate_key.pem"
    return 0
}

# Generate CSR for service
generate_csr_for_service() {
    local service_name=$1
    local namespace=$2
    
    # Generate private key
    openssl genrsa -out "/tmp/migration_${service_name}.key" 2048
    
    # Create CSR configuration
    cat > "/tmp/migration_${service_name}.conf" <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = $service_name
O = Alt RSS Reader

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = $service_name
DNS.2 = $service_name.$namespace.svc.cluster.local
DNS.3 = localhost
IP.1 = 127.0.0.1
EOF
    
    # Generate CSR
    openssl req -new -key "/tmp/migration_${service_name}.key" -out "/tmp/migration_${service_name}.csr" -config "/tmp/migration_${service_name}.conf"
    
    # Create K8s CSR
    local csr_name="migration-${service_name}-${namespace}-$(date +%s)"
    cat > "/tmp/migration_${service_name}_csr.yaml" <<EOF
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: $csr_name
  labels:
    app.kubernetes.io/managed-by: csr-controller-migration
    app.kubernetes.io/component: ssl-certificate
    app.kubernetes.io/service: $service_name
    app.kubernetes.io/namespace: $namespace
spec:
  request: $(cat "/tmp/migration_${service_name}.csr" | base64 | tr -d '\n')
  signerName: "alt.production.local/ca"
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF
    
    # Apply CSR
    if kubectl apply -f "/tmp/migration_${service_name}_csr.yaml"; then
        log_success "CSR created for $service_name in $namespace"
        
        # Store private key temporarily
        kubectl create secret generic "${service_name}-migration-key" -n "$namespace" \
            --from-file=private.key="/tmp/migration_${service_name}.key" \
            --dry-run=client -o yaml | kubectl apply -f -
        
        # Cleanup temporary files
        rm -f "/tmp/migration_${service_name}.*"
        
        return 0
    else
        log_error "Failed to create CSR for $service_name in $namespace"
        rm -f "/tmp/migration_${service_name}.*"
        return 1
    fi
}

# Validate migration
validate_migration() {
    log_info "Validating migration results..."
    update_migration_state "validation" "in_progress"
    
    local validation_report="/tmp/migration-validation.md"
    
    cat > "$validation_report" <<EOF
# Migration Validation Report

**Generated**: $(date)
**Migration ID**: $(cat "$MIGRATION_STATE_FILE" | jq -r '.migration_id')

## Validation Results

EOF
    
    # Check CSR Controller status
    if kubectl get pods -n "$TARGET_NAMESPACE" -l app.kubernetes.io/name=csr-controller | grep -q "Running"; then
        echo "✅ CSR Controller is running" >> "$validation_report"
    else
        echo "❌ CSR Controller is not running" >> "$validation_report"
    fi
    
    # Check certificate monitoring
    if kubectl get pods -n "$TARGET_NAMESPACE" -l app.kubernetes.io/component=cert-monitor | grep -q "Running"; then
        echo "✅ Certificate monitoring is running" >> "$validation_report"
    else
        echo "❌ Certificate monitoring is not running" >> "$validation_report"
    fi
    
    # Check migrated certificates
    local migrated_count=$(kubectl get secrets --all-namespaces -l app.kubernetes.io/managed-by=csr-controller | wc -l)
    echo "📊 Migrated certificates: $migrated_count" >> "$validation_report"
    
    # Check pending CSRs
    local pending_csrs=$(kubectl get csr | grep "Pending" | wc -l)
    echo "📋 Pending CSRs: $pending_csrs" >> "$validation_report"
    
    # Check approved CSRs
    local approved_csrs=$(kubectl get csr | grep "Approved" | wc -l)
    echo "✅ Approved CSRs: $approved_csrs" >> "$validation_report"
    
    log_success "Migration validation completed: $validation_report"
    update_migration_state "validation" "completed"
}

# Cleanup legacy resources
cleanup_legacy_resources() {
    log_info "Cleaning up legacy resources..."
    update_migration_state "cleanup" "in_progress"
    
    # Remove legacy common-ssl resources
    kubectl delete -l app.kubernetes.io/name=common-ssl --all-namespaces --ignore-not-found=true
    
    # Remove migration temporary secrets
    kubectl delete secrets -l app.kubernetes.io/managed-by=csr-controller-migration --all-namespaces --ignore-not-found=true
    
    # Remove completed migration CSRs
    kubectl delete csr -l app.kubernetes.io/managed-by=csr-controller-migration --ignore-not-found=true
    
    log_success "Legacy resource cleanup completed"
    update_migration_state "cleanup" "completed"
}

# Generate migration report
generate_migration_report() {
    log_info "Generating final migration report..."
    
    local report_file="/tmp/csr-controller-migration-report-$(date +%Y%m%d_%H%M%S).md"
    
    cat > "$report_file" <<EOF
# CSR Controller Migration Report

**Migration ID**: $(cat "$MIGRATION_STATE_FILE" | jq -r '.migration_id')
**Start Time**: $(cat "$MIGRATION_STATE_FILE" | jq -r '.start_time')
**End Time**: $(date -u +%Y-%m-%dT%H:%M:%SZ)
**Duration**: $(($(date +%s) - $(date -d "$(cat "$MIGRATION_STATE_FILE" | jq -r '.start_time')" +%s))) seconds

## Migration Summary

$(cat "$MIGRATION_STATE_FILE" | jq -r '.steps | to_entries[] | "- **\(.key | gsub("_"; " ") | ascii_upcase)**: \(.value)"')

## Post-Migration Status

### CSR Controller
- **Status**: $(kubectl get pods -n "$TARGET_NAMESPACE" -l app.kubernetes.io/name=csr-controller -o jsonpath='{.items[0].status.phase}')
- **Namespace**: $TARGET_NAMESPACE
- **Replicas**: $(kubectl get deployment -n "$TARGET_NAMESPACE" -l app.kubernetes.io/name=csr-controller -o jsonpath='{.items[0].spec.replicas}')

### Certificate Monitoring
- **Status**: $(kubectl get pods -n "$TARGET_NAMESPACE" -l app.kubernetes.io/component=cert-monitor -o jsonpath='{.items[0].status.phase}')
- **Monitoring Interval**: 5 minutes
- **Alert Threshold**: 30 days

### Migrated Resources
- **Total Certificates**: $(kubectl get secrets --all-namespaces -l app.kubernetes.io/managed-by=csr-controller | wc -l)
- **Active CSRs**: $(kubectl get csr | grep -c "Approved")
- **Pending CSRs**: $(kubectl get csr | grep -c "Pending")

## Backup Information

$(if [ -f "/tmp/legacy-backup-path" ]; then
    echo "- **Legacy Backup**: $(cat /tmp/legacy-backup-path)"
    echo "- **Restoration Script**: $(cat /tmp/legacy-backup-path)/restore-legacy-certificates.sh"
else
    echo "- **Legacy Backup**: Not created"
fi)

## Next Steps

1. **Monitor System Performance**
   - Check certificate rotation functionality
   - Verify monitoring and alerting
   - Monitor resource usage

2. **Validate Certificate Operations**
   - Test certificate renewal
   - Verify CSR approval process
   - Check certificate distribution

3. **Update Documentation**
   - Update operational procedures
   - Document new certificate management process
   - Train operations team

4. **Cleanup (After 30 days)**
   - Remove legacy backup files
   - Clean up temporary migration resources
   - Archive migration logs

## Support Information

- **Documentation**: /charts/csr-controller/README.md
- **Migration Logs**: /tmp/certificate-migration.log
- **Validation Report**: /tmp/migration-validation.md
- **Troubleshooting**: kubectl logs -n $TARGET_NAMESPACE -l app.kubernetes.io/name=csr-controller

---
*Report generated automatically by migration script*
EOF
    
    log_success "Migration report generated: $report_file"
}

# Main migration function
main() {
    log_info "Starting CSR Controller Migration from Legacy System"
    log_info "================================================="
    
    # Initialize migration
    init_migration_state
    
    # Run migration steps
    discover_legacy_certificates
    analyze_certificate_compatibility
    backup_legacy_certificates
    migrate_certificates
    validate_migration
    cleanup_legacy_resources
    generate_migration_report
    
    # Update final state
    if command -v jq &> /dev/null; then
        local temp_file=$(mktemp)
        jq '.status = "completed" | .end_time = "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"' "$MIGRATION_STATE_FILE" > "$temp_file"
        mv "$temp_file" "$MIGRATION_STATE_FILE"
    fi
    
    # Final summary
    log_info "================================================="
    log_success "CSR Controller Migration Completed Successfully!"
    log_info "================================================="
    
    log_info "Migration Summary:"
    log_info "- Duration: $(($(date +%s) - $(date -d "$(cat "$MIGRATION_STATE_FILE" | jq -r '.start_time')" +%s))) seconds"
    log_info "- State File: $MIGRATION_STATE_FILE"
    
    if [ -f "/tmp/legacy-backup-path" ]; then
        log_info "- Legacy Backup: $(cat /tmp/legacy-backup-path)"
    fi
    
    log_info ""
    log_info "Post-Migration Tasks:"
    log_info "1. Monitor system performance for 24 hours"
    log_info "2. Validate certificate operations"
    log_info "3. Update operational procedures"
    log_info "4. Schedule legacy resource cleanup"
}

# Display usage information
usage() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  --legacy-namespace <name>     Source namespace (default: default)"
    echo "  --target-namespace <name>     Target namespace (default: alt-production)"
    echo "  --batch-size <number>         Migration batch size (default: 5)"
    echo "  --timeout <seconds>           Migration timeout (default: 300)"
    echo "  --no-backup                   Skip backup creation"
    echo "  --no-rollback                 Skip automatic rollback on failure"
    echo "  --dry-run                     Run in dry-run mode"
    echo "  --help                        Show this help message"
}

# Parse command line arguments
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --legacy-namespace)
            LEGACY_NAMESPACE="$2"
            shift 2
            ;;
        --target-namespace)
            TARGET_NAMESPACE="$2"
            shift 2
            ;;
        --batch-size)
            MIGRATION_BATCH_SIZE="$2"
            shift 2
            ;;
        --timeout)
            MIGRATION_TIMEOUT="$2"
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
    init_migration_state
    discover_legacy_certificates
    analyze_certificate_compatibility
    log_success "Dry-run migration analysis completed"
    exit 0
fi

# Run the migration
main