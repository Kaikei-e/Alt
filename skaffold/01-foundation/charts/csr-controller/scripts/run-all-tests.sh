#!/bin/bash

# Comprehensive Test Suite for CSR Controller
# This script runs all tests to validate the complete certificate management system

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_RESULTS_DIR="/tmp/csr-controller-test-results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="$TEST_RESULTS_DIR/test_run_$TIMESTAMP.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" | tee -a "$LOG_FILE"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" | tee -a "$LOG_FILE"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
}

# Test result tracking
TOTAL_TEST_SUITES=0
PASSED_TEST_SUITES=0
FAILED_TEST_SUITES=0

# Function to run test suite
run_test_suite() {
    local suite_name=$1
    local test_script=$2
    local test_args=$3
    
    log_info "Running test suite: $suite_name"
    TOTAL_TEST_SUITES=$((TOTAL_TEST_SUITES + 1))
    
    # Create test-specific results directory
    local suite_results_dir="$TEST_RESULTS_DIR/$suite_name"
    mkdir -p "$suite_results_dir"
    
    # Run the test script
    local suite_log_file="$suite_results_dir/test_results.log"
    
    if cd "$SCRIPT_DIR" && bash "$test_script" $test_args > "$suite_log_file" 2>&1; then
        log_success "PASSED: $suite_name"
        PASSED_TEST_SUITES=$((PASSED_TEST_SUITES + 1))
        
        # Extract test summary from log file
        if grep -q "Total tests:" "$suite_log_file"; then
            local test_summary=$(grep -A3 "Total tests:" "$suite_log_file")
            log_info "Test summary for $suite_name:"
            echo "$test_summary" | tee -a "$LOG_FILE"
        fi
        
        return 0
    else
        log_error "FAILED: $suite_name"
        FAILED_TEST_SUITES=$((FAILED_TEST_SUITES + 1))
        
        # Show last few lines of error
        log_error "Last 10 lines of error log:"
        tail -n 10 "$suite_log_file" | tee -a "$LOG_FILE"
        
        return 1
    fi
}

# Setup function
setup_test_environment() {
    log_info "Setting up comprehensive test environment..."
    
    # Create results directory
    mkdir -p "$TEST_RESULTS_DIR"
    
    # Initialize log file
    echo "CSR Controller Comprehensive Test Suite" > "$LOG_FILE"
    echo "Test run started at: $(date)" >> "$LOG_FILE"
    echo "========================================" >> "$LOG_FILE"
    
    # Check prerequisites
    check_prerequisites
    
    log_info "Test environment setup completed"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites for all test suites..."
    
    # Check required tools
    local required_tools=("kubectl" "helm" "openssl" "docker" "git")
    
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
    
    # Check if we're in the correct directory
    if [ ! -f "../Chart.yaml" ]; then
        log_error "Must be run from the scripts directory of the csr-controller chart"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Cleanup function
cleanup() {
    log_info "Performing cleanup after test run..."
    
    # Kill any background processes
    jobs -p | xargs -r kill 2>/dev/null || true
    
    # Clean up any test namespaces that might be left
    local test_namespaces=(
        "csr-controller-test"
        "cert-rotation-test"
        "cert-monitoring-test"
        "performance-test"
        "integration-test"
    )
    
    for ns in "${test_namespaces[@]}"; do
        if kubectl get namespace "$ns" &>/dev/null; then
            log_info "Cleaning up test namespace: $ns"
            kubectl delete namespace "$ns" --ignore-not-found=true --timeout=60s &
        fi
    done
    
    # Wait for cleanup to complete
    wait
    
    log_info "Cleanup completed"
}

# Generate test report
generate_test_report() {
    log_info "Generating comprehensive test report..."
    
    local report_file="$TEST_RESULTS_DIR/test_report_$TIMESTAMP.html"
    
    cat > "$report_file" <<EOF
<!DOCTYPE html>
<html>
<head>
    <title>CSR Controller Test Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background-color: #f0f0f0; padding: 20px; border-radius: 5px; }
        .summary { background-color: #e8f5e8; padding: 15px; margin: 20px 0; border-radius: 5px; }
        .failure { background-color: #ffe8e8; padding: 15px; margin: 20px 0; border-radius: 5px; }
        .test-suite { margin: 20px 0; padding: 10px; border: 1px solid #ddd; border-radius: 5px; }
        .passed { border-left: 5px solid #4CAF50; }
        .failed { border-left: 5px solid #f44336; }
        pre { background-color: #f5f5f5; padding: 10px; border-radius: 3px; overflow-x: auto; }
        .timestamp { color: #666; font-size: 0.9em; }
    </style>
</head>
<body>
    <div class="header">
        <h1>CSR Controller Comprehensive Test Report</h1>
        <p class="timestamp">Generated on: $(date)</p>
        <p>Test run ID: $TIMESTAMP</p>
    </div>
    
    <div class="summary">
        <h2>Test Summary</h2>
        <p><strong>Total Test Suites:</strong> $TOTAL_TEST_SUITES</p>
        <p><strong>Passed:</strong> $PASSED_TEST_SUITES</p>
        <p><strong>Failed:</strong> $FAILED_TEST_SUITES</p>
        <p><strong>Success Rate:</strong> $(( PASSED_TEST_SUITES * 100 / TOTAL_TEST_SUITES ))%</p>
    </div>
EOF
    
    # Add detailed results for each test suite
    for suite_dir in "$TEST_RESULTS_DIR"/*; do
        if [ -d "$suite_dir" ]; then
            local suite_name=$(basename "$suite_dir")
            local suite_log="$suite_dir/test_results.log"
            
            if [ -f "$suite_log" ]; then
                local status="passed"
                if grep -q "FAILED" "$suite_log"; then
                    status="failed"
                fi
                
                cat >> "$report_file" <<EOF
    <div class="test-suite $status">
        <h3>$suite_name</h3>
        <pre>$(cat "$suite_log")</pre>
    </div>
EOF
            fi
        fi
    done
    
    cat >> "$report_file" <<EOF
    
    <div class="footer">
        <p>Report generated by CSR Controller Test Suite</p>
        <p>For more information, see individual test logs in: $TEST_RESULTS_DIR</p>
    </div>
</body>
</html>
EOF
    
    log_success "Test report generated: $report_file"
}

# Main test execution
main() {
    log_info "Starting CSR Controller Comprehensive Test Suite"
    log_info "=================================================="
    
    # Setup test environment
    setup_test_environment
    
    # Set up cleanup trap
    trap cleanup EXIT
    
    # Run all test suites
    log_info "Running all test suites..."
    
    # 1. Integration Tests
    run_test_suite "integration-tests" "integration-test.sh" "--timeout 600"
    
    # 2. Certificate Rotation Tests
    run_test_suite "cert-rotation-tests" "cert-rotation-test.sh" "--timeout 300"
    
    # 3. Certificate Monitoring Tests
    run_test_suite "cert-monitoring-tests" "monitoring-test.sh" "--timeout 300"
    
    # Generate comprehensive report
    generate_test_report
    
    # Final summary
    echo | tee -a "$LOG_FILE"
    log_info "==================================================" 
    log_info "COMPREHENSIVE TEST SUITE RESULTS"
    log_info "=================================================="
    log_info "Total test suites: $TOTAL_TEST_SUITES"
    log_success "Passed: $PASSED_TEST_SUITES"
    log_error "Failed: $FAILED_TEST_SUITES"
    
    if [ $FAILED_TEST_SUITES -gt 0 ]; then
        log_error "Some test suites failed. Please check the detailed logs."
        log_info "Test results available in: $TEST_RESULTS_DIR"
        exit 1
    else
        log_success "All test suites passed! CSR Controller system is fully functional."
        log_info "Test results available in: $TEST_RESULTS_DIR"
        exit 0
    fi
}

# Display usage information
usage() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  --results-dir <path>   Directory for test results (default: /tmp/csr-controller-test-results)"
    echo "  --skip-cleanup         Skip cleanup after tests"
    echo "  --verbose              Enable verbose logging"
    echo "  --help                 Show this help message"
    echo ""
    echo "Test Suites:"
    echo "  - Integration Tests: Complete system integration testing"
    echo "  - Certificate Rotation Tests: Automated certificate rotation"
    echo "  - Certificate Monitoring Tests: Monitoring and alerting functionality"
    echo ""
    echo "This script runs all test suites and generates a comprehensive report."
}

# Parse command line arguments
SKIP_CLEANUP=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --results-dir)
            TEST_RESULTS_DIR="$2"
            shift 2
            ;;
        --skip-cleanup)
            SKIP_CLEANUP=true
            shift
            ;;
        --verbose)
            VERBOSE=true
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

# Modify cleanup behavior if requested
if [ "$SKIP_CLEANUP" = true ]; then
    cleanup() {
        log_info "Skipping cleanup as requested"
    }
fi

# Run the comprehensive test suite
main