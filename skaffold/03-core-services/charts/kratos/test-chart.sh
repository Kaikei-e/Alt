#!/bin/bash

# kratos Helm Chart Validation Script

set -e

CHART_DIR="$(dirname "$0")"
CHART_NAME="kratos"

echo "🔍 Validating kratos Helm Chart..."

# Function to print step headers
print_step() {
    echo ""
    echo "📋 $1"
    echo "----------------------------------------"
}

# Lint the chart
print_step "Step 1: Linting Chart"
helm lint "$CHART_DIR"

# Update dependencies
print_step "Step 2: Updating Dependencies"
helm dependency update "$CHART_DIR"

# Validate template with default values
print_step "Step 3: Validating Templates (Default Values)"
helm template "$CHART_NAME" "$CHART_DIR" --dry-run > /dev/null
echo "✅ Default values template validation passed"

# Validate template with production values
print_step "Step 4: Validating Templates (Production Values)"
helm template "$CHART_NAME" "$CHART_DIR" -f "$CHART_DIR/values-production.yaml" --dry-run > /dev/null
echo "✅ Production values template validation passed"

# Check dependencies
print_step "Step 5: Checking Dependencies"
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
print_step "Step 6: Configuration Validation"

# Check if init container (migration) is present
MIGRATION_ENABLED=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "kratos-migrate" || echo "0")
if [ "$MIGRATION_ENABLED" -gt 0 ]; then
    echo "✅ Migration init container configured"
else
    echo "❌ Migration init container missing"
    exit 1
fi

# Check if services are created
PUBLIC_SERVICE=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "kratos-public" || echo "0")
ADMIN_SERVICE=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "kratos-admin" || echo "0")
if [ "$PUBLIC_SERVICE" -gt 0 ] && [ "$ADMIN_SERVICE" -gt 0 ]; then
    echo "✅ Public and Admin services configured"
else
    echo "❌ Services missing"
    exit 1
fi

# Check if configmaps are created
CONFIG_CM=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "kratos-config" || echo "0")
SCHEMAS_CM=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "kratos-schemas" || echo "0")
if [ "$CONFIG_CM" -gt 0 ] && [ "$SCHEMAS_CM" -gt 0 ]; then
    echo "✅ ConfigMaps (config and schemas) configured"
else
    echo "❌ ConfigMaps missing"
    exit 1
fi

# Check if secrets are created
SECRET_ENABLED=$(helm template "$CHART_NAME" "$CHART_DIR" | grep -c "kratos.*-secrets" || echo "0")
if [ "$SECRET_ENABLED" -gt 0 ]; then
    echo "✅ Secrets configured"
else
    echo "❌ Secrets missing"
    exit 1
fi

# Check HPA in production
HPA_ENABLED=$(helm template "$CHART_NAME" "$CHART_DIR" -f "$CHART_DIR/values-production.yaml" | grep -c "HorizontalPodAutoscaler" || echo "0")
if [ "$HPA_ENABLED" -gt 0 ]; then
    echo "✅ HorizontalPodAutoscaler configured in production"
else
    echo "❌ HorizontalPodAutoscaler missing in production"
    exit 1
fi

# Check PDB in production
PDB_ENABLED=$(helm template "$CHART_NAME" "$CHART_DIR" -f "$CHART_DIR/values-production.yaml" | grep -c "PodDisruptionBudget" || echo "0")
if [ "$PDB_ENABLED" -gt 0 ]; then
    echo "✅ PodDisruptionBudget configured in production"
else
    echo "❌ PodDisruptionBudget missing in production"
    exit 1
fi

# Generate sample manifests for review
print_step "Step 7: Generating Sample Manifests"
mkdir -p /tmp/kratos-manifests

echo "📄 Generating default configuration..."
helm template "$CHART_NAME" "$CHART_DIR" > /tmp/kratos-manifests/default.yaml

echo "📄 Generating production configuration..."
helm template "$CHART_NAME" "$CHART_DIR" -f "$CHART_DIR/values-production.yaml" > /tmp/kratos-manifests/production.yaml

echo "✅ Sample manifests generated in /tmp/kratos-manifests/"

# Test database dependency
print_step "Step 8: Database Dependency Validation"
POSTGRES_DEP=$(helm template "$CHART_NAME" "$CHART_DIR" -f "$CHART_DIR/values-production.yaml" | grep -c "kratos-postgres" || echo "0")
if [ "$POSTGRES_DEP" -gt 0 ]; then
    echo "✅ Database dependency configured"
else
    echo "❌ Database dependency missing"
    exit 1
fi

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
echo "   1. Review generated manifests in /tmp/kratos-manifests/"
echo "   2. Update values as needed for your environment"
echo "   3. Deploy dependencies first: helm install kratos-postgres ../kratos-postgres"
echo "   4. Deploy kratos: helm install kratos $CHART_DIR"
echo "   5. For production: helm install kratos $CHART_DIR -f values-production.yaml"