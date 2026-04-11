# Alt Platform Backup & Restore Runbook

This runbook provides operational procedures for the Alt platform backup system.

## Overview

The Alt platform uses a 3-2-1 backup strategy:
- **3 copies** of data (production + local backup + offsite)
- **2 different media** (Docker volumes + Restic repository)
- **1 offsite** copy (via Tailscale VPN)

### Backup Tiers

| Tier | Location | Retention | Purpose |
|------|----------|-----------|---------|
| Hot | `/backups/postgres/` | 7 days | Quick point-in-time recovery |
| Warm | `/backups/restic-repo/` | 30 days | Local disaster recovery |
| Cold | Remote server (via `REMOTE_REPO`) | Configurable (default: 30 daily, 12 weekly, 6 monthly) | Offsite disaster recovery |

### Data Priority

| Priority | Data | RPO | RTO |
|----------|------|-----|-----|
| CRITICAL | alt-db, kratos-db, recap-db, rag-db | 1 hour | 4 hours |
| HIGH | Meilisearch, ClickHouse | 6 hours | 8 hours |
| MEDIUM | Redis Streams, OAuth tokens | 24 hours | 4 hours |
| LOW | Prometheus, Grafana, Ollama models | N/A | Regenerable |

---

## Daily Operations

### Verify Backup Status

```bash
# Check latest Restic snapshots
restic -r /backups/restic-repo snapshots --latest 5

# Check PostgreSQL backups
ls -lth /backups/postgres/*.dump | head -10

# Check backup metrics
cat /backups/metrics/backup_metrics.prom

# View recent backup logs
tail -100 /backups/logs/backup-*.log | grep -E "(SUCCESS|ERROR|WARN)"
```

### Manual Backup

```bash
# Full backup (all databases + volumes)
./scripts/backup/backup-all.sh

# PostgreSQL only
./scripts/backup/backup-all.sh --pg-only

# Volumes only (skip database dumps)
./scripts/backup/backup-all.sh --volumes-only

# Dry run (see what would be backed up)
./scripts/backup/backup-all.sh --dry-run
```

### Check Offsite Sync

```bash
# Verify Tailscale connectivity
./scripts/backup/sync-offsite.sh --check-only

# View remote snapshots
restic -r sftp:backup-server:/backups/alt/restic-repo snapshots

# Manual sync
./scripts/backup/sync-offsite.sh
```

---

## Restore Procedures

### Scenario 1: Restore Single PostgreSQL Database

Use this when you need to restore a specific database without affecting others.

```bash
# 1. List available PostgreSQL backups
ls -lth /backups/postgres/alt-db-*.dump

# 2. Stop the application (optional but recommended)
docker compose -f compose/compose.yaml -p alt stop alt-backend

# 3. Restore from pg_dump
docker exec -i alt-db pg_restore \
    -U postgres \
    -d alt \
    --clean \
    --if-exists \
    < /backups/postgres/alt-db-20260131_030000.dump

# 4. Restart application
docker compose -f compose/compose.yaml -p alt start alt-backend

# 5. Verify
curl http://localhost:9000/v1/health
```

### Scenario 2: Restore from Restic Snapshot

Use this for full volume restoration or when PostgreSQL dumps are unavailable.

```bash
# 1. List available snapshots
restic -r /backups/restic-repo snapshots

# 2. Find specific snapshot
restic -r /backups/restic-repo snapshots --tag 20260131

# 3. Stop all services
docker compose -f compose/compose.yaml -p alt down

# 4. Restore to temporary directory
restic -r /backups/restic-repo restore latest --target /tmp/restore

# 5. Restore specific volume
docker volume rm alt_db_data_17 2>/dev/null || true
docker volume create alt_db_data_17
docker run --rm \
    -v alt_db_data_17:/data \
    -v /tmp/restore/data/db_data_17:/backup:ro \
    busybox sh -c "cd /backup && cp -a . /data/"

# 6. Start services
docker compose -f compose/compose.yaml -p alt up -d

# 7. Verify
./scripts/backup/backup-all.sh --verify
```

### Scenario 3: Full Disaster Recovery

Use this when recovering from complete data loss.

```bash
# 1. Ensure Docker and Docker Compose are installed

# 2. Clone repository (if needed)
git clone <repository-url> Alt
cd Alt

# 3. Restore secrets
# Copy from secure backup location to ./secrets/

# 4. If using offsite backup, restore Restic repo first
mkdir -p /backups/restic-repo
restic -r sftp:backup-server:/backups/alt/restic-repo copy \
    --repo /backups/restic-repo

# 5. List snapshots and choose restore point
restic -r /backups/restic-repo snapshots

# 6. Restore all volumes
SNAPSHOT_ID="latest"  # or specific ID like "abc123"

for vol in db_data_17 kratos_db_data recap_db_data rag_db_data meili_data clickhouse_data; do
    echo "Restoring $vol..."
    docker volume rm "alt_${vol}" 2>/dev/null || true
    docker volume create "alt_${vol}"

    # Extract from snapshot
    restic -r /backups/restic-repo restore "$SNAPSHOT_ID" \
        --target /tmp/restore \
        --include "/data/${vol}"

    # Copy to volume
    docker run --rm \
        -v "alt_${vol}:/data" \
        -v "/tmp/restore/data/${vol}:/backup:ro" \
        busybox sh -c "cd /backup && cp -a . /data/"

    rm -rf /tmp/restore
done

# 7. Start services
docker compose -f compose/compose.yaml -p alt up -d

# 8. Wait for health checks
sleep 30

# 9. Verify all services
curl http://localhost:9000/v1/health
curl http://localhost:7700/health
curl http://localhost:3000/api/health
```

### Scenario 4: Point-in-Time Recovery (PostgreSQL)

For recovering to a specific point in time when you have WAL archiving enabled.

```bash
# 1. Stop the database
docker compose -f compose/compose.yaml -p alt stop db

# 2. Remove current data
docker volume rm alt_db_data_17

# 3. Restore base backup
# ... (restore from Restic as above)

# 4. Configure recovery target
# Add to postgresql.conf:
# recovery_target_time = '2026-01-31 10:00:00'
# recovery_target_action = 'promote'

# 5. Start database in recovery mode
docker compose -f compose/compose.yaml -p alt up -d db

# 6. Monitor recovery
docker logs -f alt-db
```

---

## Using altctl for Backup/Restore

The `altctl` CLI provides simplified backup management.

```bash
# Create backup
altctl migrate backup

# List backups
altctl migrate list

# Verify backup integrity
altctl migrate verify --backup ./backups/20260131_030000

# Restore (requires --force if containers are running)
altctl migrate restore --from ./backups/20260131_030000

# Full restore with force
altctl migrate restore --from ./backups/20260131_030000 --force
```

---

## Automated Restore Verification

The platform includes automated restore verification that runs weekly (Sunday at 06:00) via supercronic inside the backup container.

### What It Verifies

1. **Restic Snapshot**: Restores the latest snapshot to a temporary directory
2. **PostgreSQL Dumps**: Spins up temporary PostgreSQL containers and attempts `pg_restore`
3. **Volume Data**: Checks that restored volume directories contain data
4. **Metrics**: Writes Prometheus metrics for monitoring

### Manual Verification

```bash
# Run full verification
./scripts/backup/restore-verify.sh

# Verify specific snapshot
./scripts/backup/restore-verify.sh --snapshot abc123

# Skip PostgreSQL verification
./scripts/backup/restore-verify.sh --skip-pg

# Dry run
./scripts/backup/restore-verify.sh --dry-run

# Keep temporary resources for inspection
./scripts/backup/restore-verify.sh --skip-cleanup
```

### Monitoring

Prometheus metrics are written to `/backups/metrics/backup_restore_verify.prom`:
- `backup_restore_verify_last_timestamp` - When the last verification ran
- `backup_restore_verify_success` - Whether it passed (0/1)
- `backup_restore_verify_duration_seconds` - How long it took

Grafana alerts:
- **backup-restore-verify-stale**: Warning if verification hasn't run in 8 days
- **backup-restore-verify-failed**: Critical if the last verification failed

### Troubleshooting Verification Failures

```bash
# Check verification logs
ls -lt /backups/logs/restore-verify-*.log | head -5
tail -100 /backups/logs/restore-verify-*.log

# Check metrics
cat /backups/metrics/backup_restore_verify.prom

# Run with verbose output
./scripts/backup/restore-verify.sh 2>&1 | tee /tmp/verify-debug.log
```

---

## Troubleshooting

### Backup Not Running

**Symptoms**: No new snapshots, stale metrics

**Check**:
```bash
# Check systemd timers
systemctl list-timers | grep alt-backup

# Check service status
systemctl status alt-backup.service

# View service logs
journalctl -u alt-backup.service -n 50
```

**Fix**:
```bash
# Restart timer
systemctl restart alt-backup-daily.timer

# Run manual backup
./scripts/backup/backup-all.sh
```

### Restic Repository Locked

**Symptoms**: `Fatal: unable to create lock`

**Fix**:
```bash
# Check for stale locks
restic -r /backups/restic-repo list locks

# Remove stale locks (be careful!)
restic -r /backups/restic-repo unlock

# If still locked, check for running processes
ps aux | grep restic
```

### Disk Space Full

**Symptoms**: Backup fails with "no space left on device"

**Fix**:
```bash
# Check disk usage
df -h /backups

# Remove old PostgreSQL backups
find /backups/postgres -name "*.dump" -mtime +3 -delete

# Prune Restic repository
restic -r /backups/restic-repo forget \
    --keep-hourly 6 \
    --keep-daily 3 \
    --keep-weekly 2 \
    --prune

# Check Restic repository size
restic -r /backups/restic-repo stats
```

### Offsite Sync Failing

**Symptoms**: Remote snapshots are stale, connectivity errors

**Check**:
```bash
# Check Tailscale
tailscale status
tailscale ping backup-server

# Check SSH
ssh backup-server 'echo OK'

# Check sync logs
cat /backups/logs/sync-offsite-*.log | tail -50
```

**Fix**:
```bash
# Reconnect Tailscale
sudo tailscale up

# Test manual sync
./scripts/backup/sync-offsite.sh --check-only

# Re-run sync
./scripts/backup/sync-offsite.sh
```

### Corrupted Backup

**Symptoms**: Restic check fails, restore errors

**Check**:
```bash
# Verify repository integrity
restic -r /backups/restic-repo check

# Check specific snapshot
restic -r /backups/restic-repo check --read-data-subset=1%
```

**Fix**:
```bash
# If corruption is limited, repair
restic -r /backups/restic-repo repair index
restic -r /backups/restic-repo repair snapshots

# If severe, restore from offsite
restic -r sftp:backup-server:/backups/alt/restic-repo copy \
    --repo /backups/restic-repo-new

mv /backups/restic-repo /backups/restic-repo-corrupted
mv /backups/restic-repo-new /backups/restic-repo
```

---

## Maintenance

### Weekly Tasks

- [ ] Review backup logs for warnings
- [ ] Verify Healthchecks.io status
- [ ] Check Prometheus metrics in Grafana
- [ ] Verify offsite sync is current

### Monthly Tasks

- [ ] Perform test restore of one database
- [ ] Run `restic check` on both local and remote repos
- [ ] Review and update retention policies if needed
- [ ] Verify Tailscale connectivity
- [ ] Test alert notifications

### Quarterly Tasks

- [ ] Full disaster recovery drill
- [ ] Measure actual RTO/RPO
- [ ] Review and update this runbook
- [ ] Rotate Restic password (update in secrets)
- [ ] Verify backup encryption

---

## Configuration Reference

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `RESTIC_REPOSITORY` | Path to Restic repository | `/backups/restic-repo` |
| `RESTIC_PASSWORD_FILE` | Path to password file | `/run/secrets/restic_password` |
| `BACKUP_DIR` | Base backup directory | `/backups` |
| `HEALTHCHECK_URL` | Healthchecks.io ping URL | (none) |
| `TAILSCALE_HOST` | Offsite backup server | `backup-server` |
| `REMOTE_REPO` | Remote Restic repository | `sftp:${TAILSCALE_HOST}:/backups/alt/restic-repo` |
| `REMOTE_KEEP_DAILY` | Remote daily retention | `30` |
| `REMOTE_KEEP_WEEKLY` | Remote weekly retention | `12` |
| `REMOTE_KEEP_MONTHLY` | Remote monthly retention | `6` |

### File Locations

| Path | Purpose |
|------|---------|
| `/backups/postgres/` | PostgreSQL dump files |
| `/backups/restic-repo/` | Local Restic repository |
| `/backups/logs/` | Backup operation logs |
| `/backups/metrics/` | Prometheus metrics |
| `./scripts/backup/` | Backup scripts |
| `./secrets/restic_password.txt` | Restic password |

### Healthchecks.io Monitors

| Monitor | Expected Interval | Grace Period |
|---------|-------------------|--------------|
| `alt-backup-hourly` | 1 hour | 30 minutes |
| `alt-backup-daily` | 24 hours | 2 hours |
| `alt-backup-offsite` | 24 hours | 4 hours |

---

## Contact

For backup-related issues:
- Check this runbook first
- Review logs in `/backups/logs/`
- Check Grafana dashboards
- Escalate to platform team if unresolved
