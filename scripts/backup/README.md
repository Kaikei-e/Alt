# Alt Platform Backup Scripts

Comprehensive backup system implementing 3-2-1 backup strategy for the Alt platform.

## Quick Start

### 1. Initialize Restic Repository

```bash
# Generate Restic password
openssl rand -base64 32 > secrets/restic_password.txt

# Create backup directory
sudo mkdir -p /backups/{postgres,restic-repo,logs,metrics}
sudo chown -R $USER:$USER /backups

# Initialize repository
./scripts/backup/backup-all.sh --init
```

### 2. Run First Backup

```bash
./scripts/backup/backup-all.sh
```

### 3. Verify Backup

```bash
restic -r /backups/restic-repo snapshots
restic -r /backups/restic-repo check
```

## Scripts

| Script | Purpose |
|--------|---------|
| `backup-all.sh` | Master backup script for all databases and volumes |
| `sync-offsite.sh` | Sync to offsite backup server via Tailscale |
| `backup-metrics.sh` | Generate Prometheus-compatible metrics |
| `restore-verify.sh` | Automated restore verification (runs weekly) |
| `crontab` | supercronic schedule for container-based scheduling |
| `alt-backup.env` | Shared environment configuration |

## Usage

### backup-all.sh

```bash
# Full backup
./scripts/backup/backup-all.sh

# Initialize repository (first run)
./scripts/backup/backup-all.sh --init

# PostgreSQL only (for hourly backups)
./scripts/backup/backup-all.sh --pg-only

# Volumes only
./scripts/backup/backup-all.sh --volumes-only

# With prune (remove old snapshots)
./scripts/backup/backup-all.sh --prune

# Verify after backup
./scripts/backup/backup-all.sh --verify

# Dry run
./scripts/backup/backup-all.sh --dry-run
```

### sync-offsite.sh

```bash
# Check connectivity
./scripts/backup/sync-offsite.sh --check-only

# Full sync
./scripts/backup/sync-offsite.sh

# With remote prune
./scripts/backup/sync-offsite.sh --prune-remote

# Verify remote
./scripts/backup/sync-offsite.sh --verify
```

### restore-verify.sh

```bash
# Full restore verification
./scripts/backup/restore-verify.sh

# Verify specific snapshot
./scripts/backup/restore-verify.sh --snapshot abc123

# Skip PostgreSQL verification
./scripts/backup/restore-verify.sh --skip-pg

# Dry run
./scripts/backup/restore-verify.sh --dry-run
```

## Installation

### systemd Timers

Install the systemd timers for automated backups:

```bash
# Copy service and timer files
sudo cp scripts/backup/systemd/*.service /etc/systemd/system/
sudo cp scripts/backup/systemd/*.timer /etc/systemd/system/

# Update paths in service files if needed
sudo sed -i 's|/home/koko/Documents/dev/Alt|YOUR_PATH|g' /etc/systemd/system/alt-backup*.service

# Reload systemd
sudo systemctl daemon-reload

# Enable and start timers
sudo systemctl enable --now alt-backup-hourly.timer
sudo systemctl enable --now alt-backup-daily.timer
sudo systemctl enable --now alt-backup-offsite.timer

# Verify timers
systemctl list-timers | grep alt-backup
```

### Docker Compose Profile

Use the backup profile for container-based backups:

```bash
# Start backup container
docker compose -f compose/compose.yaml -p alt --profile backup up -d restic-backup

# Run backup inside container
docker exec alt-backup restic snapshots

# Interactive shell
docker exec -it alt-backup sh
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `RESTIC_REPOSITORY` | Restic repo path | `/backups/restic-repo` |
| `RESTIC_PASSWORD_FILE` | Password file | `/run/secrets/restic_password` |
| `BACKUP_DIR` | Base backup directory | `/backups` |
| `HEALTHCHECK_URL` | Healthchecks.io URL | (none) |
| `TAILSCALE_HOST` | Offsite server | `backup-server` |
| `REMOTE_REPO` | Remote Restic repo | `sftp:${TAILSCALE_HOST}:/backups/alt/restic-repo` |
| `REMOTE_KEEP_DAILY` | Remote daily retention | `30` |
| `REMOTE_KEEP_WEEKLY` | Remote weekly retention | `12` |
| `REMOTE_KEEP_MONTHLY` | Remote monthly retention | `6` |

### Healthchecks.io Integration

Set up monitoring at https://healthchecks.io:

1. Create monitors for:
   - `alt-backup-hourly` (1 hour interval, 30 min grace)
   - `alt-backup-daily` (24 hour interval, 2 hour grace)
   - `alt-backup-offsite` (24 hour interval, 4 hour grace)

2. Add URLs to environment:
   ```bash
   export HEALTHCHECK_URL="https://hc-ping.com/your-uuid"
   ```

## Monitoring

### Prometheus Metrics

The `backup-metrics.sh` script generates Prometheus-compatible metrics:

```bash
./scripts/backup/backup-metrics.sh
cat /backups/metrics/backup_metrics.prom
```

Configure Prometheus to scrape via node_exporter textfile collector:
```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'backup'
    static_configs:
      - targets: ['localhost:9100']
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'backup_.*'
        action: keep
```

### Grafana Alerts

Import alert rules from `observability/alerts/backup-rules.yaml` via Grafana UI:
1. Go to Alerting > Alert rules
2. Click Import
3. Upload or paste the YAML file

## Retention Policy

| Tier | Location | Retention |
|------|----------|-----------|
| Hot (PostgreSQL) | `/backups/postgres/` | 7 days |
| Warm (Restic) | `/backups/restic-repo/` | 24 hourly, 7 daily, 4 weekly, 3 monthly |
| Cold (Offsite) | Remote server | 30 daily, 12 weekly, 6 monthly |

## Troubleshooting

See the full runbook at `docs/runbooks/backup-restore.md`.

### Common Issues

**Repository locked**:
```bash
restic -r /backups/restic-repo unlock
```

**Disk space full**:
```bash
# Emergency prune
restic -r /backups/restic-repo forget --keep-last 3 --prune
find /backups/postgres -mtime +3 -delete
```

**Offsite unreachable**:
```bash
tailscale status
tailscale ping backup-server
ssh backup-server 'echo OK'
```

## Security

- Restic repository is encrypted with the password in `secrets/restic_password.txt`
- Store a copy of the password in a secure location (password manager, safe)
- Without the password, backups cannot be restored
- Offsite sync uses SSH key authentication over Tailscale VPN
