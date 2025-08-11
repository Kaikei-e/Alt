# Atlas Database Migrations for Alt RSS Reader

Kubernetes-native database migration management using Atlas CLI and Helm pre-upgrade hooks.

## Overview

This migration system replaces the legacy ConfigMap-based approach with a modern, Kubernetes-native solution following 2025 best practices:

- **Atlas CLI Integration**: Professional-grade database schema management
- **Helm Pre-upgrade Hooks**: Automatic migration execution before application deployment  
- **Transaction Safety**: CONCURRENTLY operations converted to transaction-safe equivalents
- **Security**: Dedicated RBAC and credentials isolation
- **GitOps Ready**: Git-based migration storage with version control

## Architecture

### Components

1. **Atlas Migration Container**: Custom Docker image with Atlas CLI + migration files
2. **Helm Chart**: Pre-upgrade hook Jobs with proper sequencing
3. **RBAC**: Minimal permissions for migration ServiceAccount
4. **Git Storage**: Migration files stored in Git repository (not ConfigMaps)

### Migration Flow

```
Git Push → CI/CD → Build Migration Image → Helm Deploy → Pre-upgrade Hook → Atlas Migration → Application Deployment
```

## Quick Start

### 1. Build Migration Container

```bash
# Build the Atlas migration container
cd migrations-atlas/docker
docker build -t alt-migrations:latest .

# Push to registry (adjust registry URL)
docker tag alt-migrations:latest your-registry.com/alt-migrations:latest  
docker push your-registry.com/alt-migrations:latest
```

### 2. Deploy with Helm

```bash
# Add to your main application Helm chart
helm upgrade alt-app ./chart \
  --set migrations.enabled=true \
  --set migrations.image.repository=your-registry.com/alt-migrations \
  --set migrations.image.tag=latest \
  --set database.host=postgres.alt-database.svc.cluster.local \
  --set secrets.existingSecret=postgres-secrets
```

### 3. Monitor Migration

```bash
# Check migration job status
kubectl get jobs -l component=migration

# View migration logs
kubectl logs -l component=migration

# Verify database schema
kubectl exec -it postgres-pod -- psql -d alt_db -c "\\dt"
```

## Configuration

### Helm Chart Values

#### Database Configuration
```yaml
database:
  host: postgres.alt-database.svc.cluster.local
  port: 5432
  name: alt_db
  sslMode: require
```

#### Migration Settings
```yaml
migration:
  # Use custom built image with migration files
  customImage:
    enabled: true
    repository: your-registry.com/alt-migrations
    tag: latest
  
  # Migration command (status, validate, apply)
  command: apply
  
  # Resource limits
  resources:
    limits:
      cpu: 500m
      memory: 512Mi
```

#### Security Settings
```yaml
secrets:
  # Use existing database secret
  existingSecret: postgres-secrets
  
serviceAccount:
  create: true
  name: atlas-migration-sa

rbac:
  create: true
```

### Environment-Specific Values

#### Development
```yaml
# values-development.yaml
migration:
  resources:
    limits:
      cpu: 200m
      memory: 256Mi

database:
  host: localhost
  sslMode: disable
```

#### Production  
```yaml
# values-production.yaml
migration:
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
  activeDeadlineSeconds: 600

database:
  host: postgres.alt-database.svc.cluster.local
  sslMode: require
```

## Migration Development

### Adding New Migrations

1. **Create SQL file** in `migrations/` directory:
   ```sql
   -- 20250812000100_add_new_feature.sql
   -- Migration: Add new feature table
   -- Created: 2025-08-12 00:01:00
   -- Atlas Version: v0.35
   
   CREATE TABLE new_feature (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       name TEXT NOT NULL,
       created_at TIMESTAMP DEFAULT NOW()
   );
   
   CREATE INDEX idx_new_feature_name ON new_feature(name);
   ```

2. **Validate migration**:
   ```bash
   ./docker/scripts/migrate.sh validate
   ```

3. **Test in development**:
   ```bash
   # Run against dev database
   export DATABASE_URL="postgres://user:pass@localhost:5432/alt_db"
   ./docker/scripts/migrate.sh apply
   ```

4. **Build and deploy**:
   ```bash
   # Rebuild container with new migration
   docker build -t alt-migrations:v1.1.0 docker/
   
   # Deploy with new version
   helm upgrade alt-app ./chart --set migrations.image.tag=v1.1.0
   ```

### Migration Best Practices

#### DO ✅
- Use timestamp-based naming: `20250812000100_feature_name.sql`
- Add descriptive comments and metadata
- Test migrations in development first
- Use `IF NOT EXISTS` for idempotent operations
- Version your migration container images

#### DON'T ❌
- Use `CONCURRENTLY` operations (converted automatically)
- Create destructive migrations without safeguards
- Skip migration validation
- Modify existing migration files after deployment
- Use shared secrets for migration credentials

## Troubleshooting

### Migration Job Failures

1. **Check Job status**:
   ```bash
   kubectl describe job $(kubectl get job -l component=migration -o name)
   ```

2. **View detailed logs**:
   ```bash
   kubectl logs -l component=migration --previous
   ```

3. **Database connectivity**:
   ```bash
   # Test from migration pod
   kubectl run atlas-test --rm -it --image=alt-migrations:latest -- /bin/sh
   # Inside pod: 
   export DATABASE_URL="postgres://..."
   /scripts/migrate.sh status
   ```

### Common Issues

#### Connection Refused
- Check database service name and port
- Verify network policies allow migration pod → database
- Confirm database credentials in secret

#### Permission Denied  
- Verify RBAC permissions for ServiceAccount
- Check database user permissions for schema changes
- Ensure secret exists and contains correct credentials

#### Migration Hash Mismatch
- Atlas detected changes in migration files
- Rebuild container image with `atlas migrate hash`
- Do not modify existing migration files

### Rolling Back Migrations

Atlas doesn't support automatic rollbacks, but you can:

1. **Create reverse migration**:
   ```sql
   -- 20250812000200_rollback_feature.sql
   DROP TABLE IF EXISTS new_feature;
   ```

2. **Manual database restoration**:
   ```bash
   # Restore from backup
   kubectl exec postgres-pod -- pg_restore -d alt_db /backup/before-migration.sql
   ```

## Integration with CI/CD

### GitHub Actions Example
```yaml
name: Database Migration
on:
  push:
    paths: ['migrations-atlas/**']

jobs:
  migration:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    
    - name: Build Migration Image
      run: |
        docker build -t ${{ secrets.REGISTRY }}/alt-migrations:${{ github.sha }} \
          migrations-atlas/docker/
    
    - name: Push Image  
      run: docker push ${{ secrets.REGISTRY }}/alt-migrations:${{ github.sha }}
    
    - name: Deploy with Helm
      run: |
        helm upgrade alt-app ./chart \
          --set migrations.image.tag=${{ github.sha }}
```

## Security Considerations

### Credentials Management
- Use separate database credentials for migrations
- Limit migration user permissions to schema changes only
- Rotate migration credentials regularly
- Use Kubernetes secrets, never hardcode credentials

### Network Security
- Apply network policies to restrict migration pod access
- Use SSL/TLS for database connections
- Audit migration job executions

### RBAC Isolation
- Dedicated ServiceAccount for migrations
- Minimal permissions (secrets, configmaps read-only)
- No cluster-level permissions required

## Monitoring and Observability

### Metrics
- Migration execution time
- Success/failure rates
- Database connection latency

### Alerts
- Migration job failures
- Long-running migrations
- Database connectivity issues

### Logging
- Structured logging with JSON format
- Migration progress tracking
- Database query logging (development only)

## Comparison: Atlas vs Legacy ConfigMap

| Feature | Legacy ConfigMap | Atlas + Helm Hooks |
|---------|------------------|-------------------|
| Transaction Safety | ❌ CONCURRENTLY issues | ✅ Transaction-safe |
| Version Control | ❌ ConfigMap storage | ✅ Git-based |
| Rollback Support | ❌ Manual only | ✅ Built-in support |
| Schema Validation | ❌ None | ✅ Comprehensive |
| Deployment Integration | ❌ Manual execution | ✅ Automated hooks |
| Security | ❌ Shared credentials | ✅ Dedicated RBAC |
| Monitoring | ❌ Limited | ✅ Comprehensive |

## Contributing

1. Fork the repository
2. Create feature branch: `git checkout -b feature/new-migration-system`
3. Add migration files to `migrations/` directory
4. Test with development environment
5. Update documentation
6. Submit pull request

## License

Apache 2.0 License - see [LICENSE](LICENSE) file for details.