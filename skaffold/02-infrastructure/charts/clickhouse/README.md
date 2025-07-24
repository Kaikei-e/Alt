# ClickHouse Helm Chart

A Helm chart for deploying ClickHouse analytics database optimized for the Alt RSS Reader project.

## Overview

This chart deploys a ClickHouse instance configured specifically for RSS feed analytics and data warehousing workloads. ClickHouse is a columnar database that excels at analytical queries and real-time analytics.

## Features

- **Analytics-optimized configuration**: Tuned for RSS feed metrics and user analytics
- **Scalable storage**: Configurable persistent volumes for data and logs
- **SSL/TLS support**: Optional SSL encryption with certificate management
- **Multi-port support**: HTTP (8123), TCP (9000), MySQL (9004), PostgreSQL (9005) compatibility
- **Security**: Non-root execution, security contexts, and network policies
- **Monitoring**: Built-in metrics and monitoring integration
- **Backup support**: Production backup configurations

## Database Configuration

- **Database name**: `alt_analytics`
- **Default user**: `clickhouse_user`
- **HTTP port**: 8123
- **TCP port**: 9000
- **Optimizations**: Columnar storage, compression (LZ4/ZSTD), analytics-focused settings

## Installation

### Basic Installation

```bash
helm install clickhouse ./clickhouse
```

### Production Installation

```bash
helm install clickhouse ./clickhouse -f values-production.yaml
```

### With Custom Values

```bash
helm install clickhouse ./clickhouse \
  --set auth.username=analytics_user \
  --set auth.database=custom_analytics \
  --set persistence.data.size=500Gi
```

## Configuration

### Key Configuration Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `auth.username` | ClickHouse username | `clickhouse_user` |
| `auth.database` | Analytics database name | `alt_analytics` |
| `service.httpPort` | HTTP interface port | `8123` |
| `service.tcpPort` | Native TCP port | `9000` |
| `persistence.data.size` | Data volume size | `100Gi` |
| `resources.requests.memory` | Memory request | `4Gi` |
| `clickhouse.compression.method` | Compression algorithm | `lz4` |

### Analytics Optimizations

The chart includes several optimizations for RSS analytics workloads:

- **Compression**: LZ4 for development, ZSTD for production
- **Memory settings**: Optimized for analytical queries
- **MergeTree configuration**: Tuned for time-series analytics data
- **Background processing**: Enhanced merge and mutation settings
- **Query profiles**: Separate profiles for different workload types

### SSL Configuration

Enable SSL by setting:

```yaml
ssl:
  enabled: true
  secretName: "clickhouse-ssl-certs"
  verificationMode: "strict"
```

### Production Settings

The production values include:
- Enhanced resource allocations (8 CPU, 32Gi memory)
- ZSTD compression for better storage efficiency
- Larger storage allocations (500Gi data, 50Gi logs)
- Multiple user accounts with different privileges
- Advanced monitoring and backup configurations

## Usage Examples

### Connecting to ClickHouse

#### HTTP Interface
```bash
curl http://clickhouse-user:password@clickhouse.namespace.svc.cluster.local:8123/
```

#### TCP Interface (using clickhouse-client)
```bash
clickhouse-client --host clickhouse.namespace.svc.cluster.local --port 9000 --user clickhouse_user --password password --database alt_analytics
```

### Analytics Queries for RSS Data

#### Popular Feeds Query
```sql
SELECT 
    feed_url, 
    COUNT(*) as view_count,
    AVG(read_time) as avg_read_time
FROM feed_metrics 
WHERE toDate(timestamp) >= today() - 7
GROUP BY feed_url
ORDER BY view_count DESC
LIMIT 10;
```

#### User Engagement Analytics
```sql
SELECT 
    toStartOfHour(timestamp) as hour,
    COUNT(DISTINCT user_id) as active_users,
    COUNT(*) as total_interactions
FROM user_activity
WHERE timestamp >= now() - INTERVAL 24 HOUR
GROUP BY hour
ORDER BY hour;
```

## Dependencies

- **common-ssl**: SSL certificate management (optional)
- **common-secrets**: Secret management (optional)

## Security Considerations

1. **Authentication**: Always use strong passwords and consider external secret management
2. **Network policies**: Enable network policies in production environments
3. **SSL/TLS**: Enable SSL for production deployments
4. **User permissions**: Use principle of least privilege for database users
5. **Resource limits**: Set appropriate resource limits to prevent resource exhaustion

## Monitoring

The chart supports monitoring through:
- ServiceMonitor for Prometheus integration
- ClickHouse system tables for internal metrics
- Custom dashboards for RSS analytics

## Backup and Recovery

Production configurations include:
- Daily automated backups
- S3-compatible storage integration
- 30-day retention policy
- Point-in-time recovery capabilities

## Troubleshooting

### Common Issues

1. **Pod won't start**: Check storage class and persistent volume availability
2. **Connection refused**: Verify service ports and network policies
3. **Out of memory**: Increase memory limits or optimize query memory usage
4. **Slow queries**: Review MergeTree settings and table partitioning

### Logs

Check ClickHouse logs:
```bash
kubectl logs -f statefulset/clickhouse
```

### Health Checks

The chart includes liveness and readiness probes using the `/ping` endpoint.

## Migration from PostgreSQL

For migrating analytics data from PostgreSQL:

1. Use ClickHouse's PostgreSQL table engine for real-time access
2. ETL existing data using ClickHouse's INSERT FROM SELECT
3. Configure materialized views for real-time analytics
4. Gradually migrate application queries to ClickHouse

## Performance Tuning

### For High-Volume RSS Analytics

1. **Partitioning**: Partition tables by date for better query performance
2. **Indexes**: Use appropriate primary keys and skip indexes
3. **Compression**: Choose compression based on query patterns vs storage costs
4. **Memory**: Allocate sufficient memory for query processing
5. **Background merges**: Tune merge settings for write-heavy workloads

## Support

For issues specific to this chart, please check:
1. Chart template syntax and values
2. ClickHouse logs and metrics
3. Kubernetes events and pod status
4. Storage and network connectivity

For ClickHouse-specific questions, refer to the official ClickHouse documentation.