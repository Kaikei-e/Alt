#!/bin/bash

# nginx-external Helm Chart Validation Script

set -e

CHART_DIR="$(dirname "$0")"
CHART_NAME="nginx-external"

echo "🔍 Validating nginx-external Helm Chart..."

# Function to print step headers
print_step() {
    echo ""
    echo "📋 $1"
    echo "----------------------------------------"
}

# Lint the chart
print_step "Step 1: Linting Chart"
helm lint "$CHART_DIR"

# Validate template with default values
print_step "Step 2: Validating Templates (Default Values)"
helm template "$CHART_NAME" "$CHART_DIR" --dry-run > /dev/null
echo "✅ Default values template validation passed"

# Validate template with production values
print_step "Step 3: Validating Templates (Production Values)"
helm template "$CHART_NAME" "$CHART_DIR" -f "$CHART_DIR/values-production.yaml" --dry-run > /dev/null
echo "✅ Production values template validation passed"

# Check dependencies
print_step "Step 4: Checking Dependencies"
if [ -f "$CHART_DIR/Chart.lock" ]; then
    echo "✅ Chart.lock exists"
else
    echo "❌ Chart.lock missing - run 'helm dependency update'"
    exit 1
fi

if [ -d "$CHART_DIR/charts" ]; then
    echo "✅ Dependencies downloaded"
    ls -la "$CHART_DIR/charts"
else
    echo "❌ Dependencies not downloaded - run 'helm dependency update'"
    exit 1
fi

# Validate specific configurations
print_step "Step 5: Configuration Validation"

# Check if SSL is enabled by default
SSL_ENABLED=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "ssl-certs" || echo "0")
if [ "$SSL_ENABLED" -gt 0 ]; then
    echo "✅ SSL configuration present"
else
    echo "❌ SSL configuration missing"
    exit 1
fi

# Check if LoadBalancer service is created
LB_SERVICE=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "type: LoadBalancer" || echo "0")
if [ "$LB_SERVICE" -gt 0 ]; then
    echo "✅ LoadBalancer service configured"
else
    echo "❌ LoadBalancer service missing"
    exit 1
fi

# Check if HPA is enabled
HPA_ENABLED=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "HorizontalPodAutoscaler" || echo "0")
if [ "$HPA_ENABLED" -gt 0 ]; then
    echo "✅ HorizontalPodAutoscaler configured"
else
    echo "❌ HorizontalPodAutoscaler missing"
    exit 1
fi

# Check if NetworkPolicy is present
NETPOL_ENABLED=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "NetworkPolicy" || echo "0")
if [ "$NETPOL_ENABLED" -gt 0 ]; then
    echo "✅ NetworkPolicy configured"
else
    echo "❌ NetworkPolicy missing"
    exit 1
fi

# Generate sample manifests for review
print_step "Step 6: Generating Sample Manifests"
mkdir -p /tmp/nginx-external-manifests

echo "📄 Generating default configuration..."
helm template "$CHART_NAME" "$CHART_DIR" > /tmp/nginx-external-manifests/default.yaml

echo "📄 Generating production configuration..."
helm template "$CHART_NAME" "$CHART_DIR" -f "$CHART_DIR/values-production.yaml" > /tmp/nginx-external-manifests/production.yaml

echo "✅ Sample manifests generated in /tmp/nginx-external-manifests/"

# Final summary
print_step "Summary"
echo "✅ Chart validation completed successfully!"
echo ""
echo "📋 Chart Information:"
echo "   - Name: $CHART_NAME"
echo "   - Version: $(grep '^version:' "$CHART_DIR/Chart.yaml" | awk '{print $2}')"
echo "   - App Version: $(grep '^appVersion:' "$CHART_DIR/Chart.yaml" | awk '{print $2}')"
echo ""
echo "🚀 Ready for deployment!"
echo ""
echo "📖 Next steps:"
echo "   1. Review generated manifests in /tmp/nginx-external-manifests/"
echo "   2. Update values as needed for your environment"
echo "   3. Deploy with: helm install nginx-external $CHART_DIR"
echo "   4. For production: helm install nginx-external $CHART_DIR -f values-production.yaml"