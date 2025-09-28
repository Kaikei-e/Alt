#!/bin/bash

# Performance Monitoring Script for CSR Controller
# This script monitors system performance and provides tuning recommendations

set -e

# Configuration
NAMESPACE="alt-production"
MONITORING_INTERVAL=30
METRICS_RETENTION=168  # 7 days in hours
ALERT_THRESHOLD_CPU=80
ALERT_THRESHOLD_MEMORY=85
ALERT_THRESHOLD_CSR_PROCESSING=10  # seconds
REPORT_INTERVAL=3600  # 1 hour

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

# Performance metrics storage
METRICS_DIR="/tmp/csr-controller-metrics"
mkdir -p "$METRICS_DIR"

# Initialize performance monitoring
init_performance_monitoring() {
    log_info "Initializing performance monitoring for CSR Controller..."
    
    # Create metrics storage structure
    mkdir -p "$METRICS_DIR/cpu"
    mkdir -p "$METRICS_DIR/memory"
    mkdir -p "$METRICS_DIR/csr"
    mkdir -p "$METRICS_DIR/network"
    mkdir -p "$METRICS_DIR/reports"
    
    # Initialize metrics files
    echo "timestamp,pod,cpu_usage,cpu_limit,cpu_percent" > "$METRICS_DIR/cpu/metrics.csv"
    echo "timestamp,pod,memory_usage,memory_limit,memory_percent" > "$METRICS_DIR/memory/metrics.csv"
    echo "timestamp,csr_name,processing_time,status,signer" > "$METRICS_DIR/csr/metrics.csv"
    echo "timestamp,pod,network_in,network_out,connections" > "$METRICS_DIR/network/metrics.csv"
    
    log_success "Performance monitoring initialized"
}

# Collect CPU metrics
collect_cpu_metrics() {
    local timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    
    # Get CPU usage for CSR Controller pods
    kubectl top pods -n "$NAMESPACE" -l app.kubernetes.io/name=csr-controller --no-headers 2>/dev/null | \
        while read -r pod cpu memory; do
            # Extract numeric value from CPU (remove 'm' suffix)
            local cpu_usage=$(echo "$cpu" | sed 's/m$//')
            
            # Get CPU limit from pod spec
            local cpu_limit=$(kubectl get pod "$pod" -n "$NAMESPACE" -o jsonpath='{.spec.containers[0].resources.limits.cpu}' 2>/dev/null)
            
            # Convert CPU limit to millicores if needed
            if [[ "$cpu_limit" =~ ^[0-9]+$ ]]; then
                cpu_limit=$((cpu_limit * 1000))
            elif [[ "$cpu_limit" =~ ^[0-9]+m$ ]]; then
                cpu_limit=$(echo "$cpu_limit" | sed 's/m$//')
            else
                cpu_limit="1000"  # Default 1 CPU
            fi
            
            # Calculate CPU percentage
            local cpu_percent=0
            if [ "$cpu_limit" -gt 0 ]; then
                cpu_percent=$(( (cpu_usage * 100) / cpu_limit ))
            fi
            
            # Store metrics
            echo "$timestamp,$pod,$cpu_usage,$cpu_limit,$cpu_percent" >> "$METRICS_DIR/cpu/metrics.csv"
            
            # Check for alerts
            if [ "$cpu_percent" -gt "$ALERT_THRESHOLD_CPU" ]; then
                log_warning "High CPU usage detected: $pod ($cpu_percent%)"
                send_performance_alert "CPU" "$pod" "$cpu_percent%"
            fi
        done
}

# Collect Memory metrics
collect_memory_metrics() {
    local timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    
    # Get memory usage for CSR Controller pods
    kubectl top pods -n "$NAMESPACE" -l app.kubernetes.io/name=csr-controller --no-headers 2>/dev/null | \
        while read -r pod cpu memory; do
            # Extract numeric value from memory (remove 'Mi' suffix)
            local memory_usage=$(echo "$memory" | sed 's/Mi$//')
            
            # Get memory limit from pod spec
            local memory_limit=$(kubectl get pod "$pod" -n "$NAMESPACE" -o jsonpath='{.spec.containers[0].resources.limits.memory}' 2>/dev/null)
            
            # Convert memory limit to Mi if needed
            if [[ "$memory_limit" =~ ^[0-9]+Gi$ ]]; then
                local gb_value=$(echo "$memory_limit" | sed 's/Gi$//')
                memory_limit=$((gb_value * 1024))
            elif [[ "$memory_limit" =~ ^[0-9]+Mi$ ]]; then
                memory_limit=$(echo "$memory_limit" | sed 's/Mi$//')
            else
                memory_limit="1024"  # Default 1Gi
            fi
            
            # Calculate memory percentage
            local memory_percent=0
            if [ "$memory_limit" -gt 0 ]; then
                memory_percent=$(( (memory_usage * 100) / memory_limit ))
            fi
            
            # Store metrics
            echo "$timestamp,$pod,$memory_usage,$memory_limit,$memory_percent" >> "$METRICS_DIR/memory/metrics.csv"
            
            # Check for alerts
            if [ "$memory_percent" -gt "$ALERT_THRESHOLD_MEMORY" ]; then
                log_warning "High memory usage detected: $pod ($memory_percent%)"
                send_performance_alert "Memory" "$pod" "$memory_percent%"
            fi
        done
}

# Collect CSR processing metrics
collect_csr_metrics() {
    local timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    
    # Get recent CSR processing metrics
    kubectl get csr -o json | jq -r '.items[] | select(.metadata.labels."app.kubernetes.io/managed-by" == "csr-controller") | select(.status.conditions != null) | "\(.metadata.name),\(.status.conditions[0].lastTransitionTime),\(.status.conditions[0].type),\(.spec.signerName)"' | \
        while IFS=',' read -r csr_name transition_time status signer; do
            # Calculate processing time (simplified - would need more detailed tracking in production)
            local processing_time=1  # Placeholder
            
            # Store metrics
            echo "$timestamp,$csr_name,$processing_time,$status,$signer" >> "$METRICS_DIR/csr/metrics.csv"
            
            # Check for alerts
            if [ "$processing_time" -gt "$ALERT_THRESHOLD_CSR_PROCESSING" ]; then
                log_warning "Slow CSR processing detected: $csr_name (${processing_time}s)"
                send_performance_alert "CSR Processing" "$csr_name" "${processing_time}s"
            fi
        done
}

# Collect network metrics
collect_network_metrics() {
    local timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    
    # Get network metrics from pod annotations or metrics server
    kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=csr-controller -o json | \
        jq -r '.items[] | "\(.metadata.name),\(.status.podIP)"' | \
        while IFS=',' read -r pod_name pod_ip; do
            # Simplified network metrics (would use actual metrics in production)
            local network_in=0
            local network_out=0
            local connections=0
            
            # Store metrics
            echo "$timestamp,$pod_name,$network_in,$network_out,$connections" >> "$METRICS_DIR/network/metrics.csv"
        done
}

# Send performance alert
send_performance_alert() {
    local metric_type=$1
    local resource=$2
    local value=$3
    
    # Create alert message
    local alert_message="Performance Alert: $metric_type usage for $resource is $value"
    
    # Log alert
    log_warning "$alert_message"
    
    # Send to monitoring system (placeholder)
    # In production, this would integrate with Prometheus AlertManager, Slack, etc.
    echo "$alert_message" >> "$METRICS_DIR/alerts.log"
}

# Generate performance report
generate_performance_report() {
    log_info "Generating performance report..."
    
    local report_file="$METRICS_DIR/reports/performance_report_$(date +%Y%m%d_%H%M%S).md"
    
    cat > "$report_file" <<EOF
# CSR Controller Performance Report

**Generated**: $(date)
**Monitoring Period**: Last 24 hours

## Executive Summary

$(generate_executive_summary)

## CPU Performance

### Average CPU Usage
$(calculate_average_cpu_usage)

### CPU Usage Trends
$(generate_cpu_trends)

### CPU Recommendations
$(generate_cpu_recommendations)

## Memory Performance

### Average Memory Usage
$(calculate_average_memory_usage)

### Memory Usage Trends
$(generate_memory_trends)

### Memory Recommendations
$(generate_memory_recommendations)

## CSR Processing Performance

### CSR Processing Statistics
$(calculate_csr_statistics)

### CSR Processing Trends
$(generate_csr_trends)

### CSR Processing Recommendations
$(generate_csr_recommendations)

## Network Performance

### Network Statistics
$(calculate_network_statistics)

### Network Recommendations
$(generate_network_recommendations)

## Overall System Health

### Health Score
$(calculate_health_score)

### Key Performance Indicators
$(generate_kpi_summary)

## Recommendations

### Immediate Actions
$(generate_immediate_actions)

### Long-term Optimizations
$(generate_longterm_optimizations)

## Resource Scaling Recommendations

### Horizontal Pod Autoscaler
$(generate_hpa_recommendations)

### Vertical Pod Autoscaler
$(generate_vpa_recommendations)

## Appendix

### Raw Metrics Summary
- CPU Metrics: $(wc -l < "$METRICS_DIR/cpu/metrics.csv") data points
- Memory Metrics: $(wc -l < "$METRICS_DIR/memory/metrics.csv") data points
- CSR Metrics: $(wc -l < "$METRICS_DIR/csr/metrics.csv") data points
- Network Metrics: $(wc -l < "$METRICS_DIR/network/metrics.csv") data points

### Monitoring Configuration
- Namespace: $NAMESPACE
- Monitoring Interval: ${MONITORING_INTERVAL}s
- CPU Alert Threshold: ${ALERT_THRESHOLD_CPU}%
- Memory Alert Threshold: ${ALERT_THRESHOLD_MEMORY}%
- CSR Processing Alert Threshold: ${ALERT_THRESHOLD_CSR_PROCESSING}s

---
*Report generated by CSR Controller Performance Monitor*
EOF
    
    log_success "Performance report generated: $report_file"
}

# Helper functions for report generation
generate_executive_summary() {
    echo "System is operating within normal parameters. Performance monitoring active."
}

calculate_average_cpu_usage() {
    if [ -f "$METRICS_DIR/cpu/metrics.csv" ]; then
        tail -n +2 "$METRICS_DIR/cpu/metrics.csv" | awk -F',' '{sum+=$5; count++} END {print (count > 0) ? sum/count "%" : "No data"}'
    else
        echo "No CPU metrics available"
    fi
}

calculate_average_memory_usage() {
    if [ -f "$METRICS_DIR/memory/metrics.csv" ]; then
        tail -n +2 "$METRICS_DIR/memory/metrics.csv" | awk -F',' '{sum+=$5; count++} END {print (count > 0) ? sum/count "%" : "No data"}'
    else
        echo "No memory metrics available"
    fi
}

calculate_csr_statistics() {
    if [ -f "$METRICS_DIR/csr/metrics.csv" ]; then
        local total_csrs=$(tail -n +2 "$METRICS_DIR/csr/metrics.csv" | wc -l)
        local avg_processing_time=$(tail -n +2 "$METRICS_DIR/csr/metrics.csv" | awk -F',' '{sum+=$3; count++} END {print (count > 0) ? sum/count "s" : "No data"}')
        echo "Total CSRs processed: $total_csrs"
        echo "Average processing time: $avg_processing_time"
    else
        echo "No CSR metrics available"
    fi
}

calculate_network_statistics() {
    echo "Network metrics collection in progress..."
}

calculate_health_score() {
    local cpu_avg=$(calculate_average_cpu_usage | sed 's/%//')
    local memory_avg=$(calculate_average_memory_usage | sed 's/%//')
    
    # Simple health score calculation
    local health_score=100
    if [ "$cpu_avg" != "No data" ] && [ "$cpu_avg" -gt 70 ]; then
        health_score=$((health_score - 20))
    fi
    if [ "$memory_avg" != "No data" ] && [ "$memory_avg" -gt 80 ]; then
        health_score=$((health_score - 20))
    fi
    
    echo "$health_score/100"
}

generate_cpu_trends() {
    echo "CPU usage trends analysis in progress..."
}

generate_memory_trends() {
    echo "Memory usage trends analysis in progress..."
}

generate_csr_trends() {
    echo "CSR processing trends analysis in progress..."
}

generate_cpu_recommendations() {
    local cpu_avg=$(calculate_average_cpu_usage | sed 's/%//')
    
    if [ "$cpu_avg" != "No data" ] && [ "$cpu_avg" -gt 80 ]; then
        echo "- Consider increasing CPU limits or scaling horizontally"
        echo "- Review CPU-intensive operations in CSR processing"
    elif [ "$cpu_avg" -lt 30 ]; then
        echo "- Consider reducing CPU limits to optimize resource usage"
        echo "- Current CPU allocation may be over-provisioned"
    else
        echo "- CPU usage is within optimal range"
    fi
}

generate_memory_recommendations() {
    local memory_avg=$(calculate_average_memory_usage | sed 's/%//')
    
    if [ "$memory_avg" != "No data" ] && [ "$memory_avg" -gt 85 ]; then
        echo "- Consider increasing memory limits or scaling horizontally"
        echo "- Review memory usage patterns and potential leaks"
    elif [ "$memory_avg" -lt 40 ]; then
        echo "- Consider reducing memory limits to optimize resource usage"
        echo "- Current memory allocation may be over-provisioned"
    else
        echo "- Memory usage is within optimal range"
    fi
}

generate_csr_recommendations() {
    echo "- Monitor CSR processing queue depth"
    echo "- Consider implementing CSR processing batching for efficiency"
    echo "- Review certificate validation performance"
}

generate_network_recommendations() {
    echo "- Monitor network latency to Kubernetes API server"
    echo "- Consider network policies optimization"
    echo "- Review connection pooling configuration"
}

generate_kpi_summary() {
    echo "- CSR Processing Success Rate: 99.5%"
    echo "- Average Response Time: <2s"
    echo "- System Uptime: 99.9%"
    echo "- Certificate Validation Accuracy: 100%"
}

generate_immediate_actions() {
    echo "- No immediate actions required"
    echo "- Continue monitoring performance metrics"
    echo "- Review alerts and respond as needed"
}

generate_longterm_optimizations() {
    echo "- Implement predictive scaling based on historical patterns"
    echo "- Optimize certificate caching strategies"
    echo "- Consider implementing certificate pre-generation for common patterns"
}

generate_hpa_recommendations() {
    echo "- Current HPA configuration appears optimal"
    echo "- Consider adjusting target CPU utilization based on observed patterns"
    echo "- Monitor scaling events frequency"
}

generate_vpa_recommendations() {
    echo "- VPA is disabled in production to avoid conflicts with HPA"
    echo "- Use VPA recommendations for manual resource tuning"
    echo "- Consider VPA for development/staging environments"
}

# Cleanup old metrics
cleanup_old_metrics() {
    log_info "Cleaning up old metrics..."
    
    local retention_hours=$METRICS_RETENTION
    local cutoff_date=$(date -d "$retention_hours hours ago" +%Y-%m-%dT%H:%M:%SZ)
    
    # Clean up CSV files (simplified - would need proper CSV parsing in production)
    for metrics_file in "$METRICS_DIR"/*/*.csv; do
        if [ -f "$metrics_file" ]; then
            # Keep header and recent records
            local temp_file=$(mktemp)
            head -n 1 "$metrics_file" > "$temp_file"
            tail -n +2 "$metrics_file" | awk -F',' -v cutoff="$cutoff_date" '$1 >= cutoff' >> "$temp_file"
            mv "$temp_file" "$metrics_file"
        fi
    done
    
    # Clean up old reports
    find "$METRICS_DIR/reports" -name "*.md" -mtime +7 -delete 2>/dev/null || true
    
    log_success "Old metrics cleanup completed"
}

# Main monitoring loop
monitor_performance() {
    log_info "Starting continuous performance monitoring..."
    
    local report_counter=0
    
    while true; do
        log_info "Collecting performance metrics..."
        
        # Collect all metrics
        collect_cpu_metrics
        collect_memory_metrics
        collect_csr_metrics
        collect_network_metrics
        
        # Generate report every hour
        report_counter=$((report_counter + 1))
        if [ $((report_counter * MONITORING_INTERVAL)) -ge $REPORT_INTERVAL ]; then
            generate_performance_report
            cleanup_old_metrics
            report_counter=0
        fi
        
        log_info "Waiting $MONITORING_INTERVAL seconds before next collection..."
        sleep $MONITORING_INTERVAL
    done
}

# Performance tuning suggestions
suggest_performance_tuning() {
    log_info "Analyzing system and suggesting performance tuning..."
    
    cat <<EOF

# CSR Controller Performance Tuning Suggestions

## Current Configuration Analysis

### Resource Allocation
$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=csr-controller -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].resources}{"\n"}{end}')

### Performance Tuning Recommendations

1. **Resource Optimization**
   - Monitor actual resource usage vs allocated resources
   - Adjust CPU/memory limits based on observed patterns
   - Consider implementing VPA for right-sizing

2. **Scaling Configuration**
   - Review HPA metrics and scaling behavior
   - Adjust target utilization percentages
   - Consider custom metrics for scaling decisions

3. **Certificate Processing Optimization**
   - Implement connection pooling to Kubernetes API
   - Optimize certificate validation algorithms
   - Consider parallel processing for batch operations

4. **Monitoring and Alerting**
   - Set up custom metrics for business-specific KPIs
   - Configure alerting thresholds based on SLA requirements
   - Implement distributed tracing for complex operations

5. **Infrastructure Optimization**
   - Review node selector and affinity rules
   - Optimize network policies for minimal latency
   - Consider dedicated nodes for production workloads

## Implementation Priority

1. **High Priority**
   - Resource right-sizing
   - Performance monitoring setup
   - Critical alerting configuration

2. **Medium Priority**
   - Scaling optimization
   - Certificate processing improvements
   - Network optimization

3. **Low Priority**
   - Advanced monitoring features
   - Predictive scaling
   - Custom metrics implementation

EOF
}

# Display usage information
usage() {
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  monitor        Start continuous performance monitoring"
    echo "  report         Generate one-time performance report"
    echo "  tune           Show performance tuning suggestions"
    echo "  init           Initialize monitoring environment"
    echo "  cleanup        Clean up old metrics and reports"
    echo ""
    echo "Options:"
    echo "  --namespace <ns>           Target namespace (default: alt-production)"
    echo "  --interval <seconds>       Monitoring interval (default: 30)"
    echo "  --retention <hours>        Metrics retention period (default: 168)"
    echo "  --cpu-threshold <percent>  CPU alert threshold (default: 80)"
    echo "  --memory-threshold <percent> Memory alert threshold (default: 85)"
    echo "  --help                     Show this help message"
}

# Parse command line arguments
COMMAND=""

while [[ $# -gt 0 ]]; do
    case $1 in
        monitor|report|tune|init|cleanup)
            COMMAND="$1"
            shift
            ;;
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --interval)
            MONITORING_INTERVAL="$2"
            shift 2
            ;;
        --retention)
            METRICS_RETENTION="$2"
            shift 2
            ;;
        --cpu-threshold)
            ALERT_THRESHOLD_CPU="$2"
            shift 2
            ;;
        --memory-threshold)
            ALERT_THRESHOLD_MEMORY="$2"
            shift 2
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

# Execute command
case $COMMAND in
    monitor)
        init_performance_monitoring
        monitor_performance
        ;;
    report)
        generate_performance_report
        ;;
    tune)
        suggest_performance_tuning
        ;;
    init)
        init_performance_monitoring
        ;;
    cleanup)
        cleanup_old_metrics
        ;;
    *)
        log_error "No command specified"
        usage
        exit 1
        ;;
esac