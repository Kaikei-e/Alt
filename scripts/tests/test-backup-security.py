#!/usr/bin/env python3
"""
scripts/tests/test-backup-security.py
Comprehensive unit and security verification suite for Alt Platform D05 daemonless backup.

Covers:
1. Dockerfile client version known exact 18 & removal of docker-cli
2. Compose backup.yaml daemonless contracts (no socket, no proxy, no DOCKER_HOST)
3. ClickHouse backup disk XML configuration validity and schema
4. Host-only restore-verify.sh fail-fast container guard & host execution flow
5. Networked PG dump role/credentials pairing (alt_db_user + postgres_password, kratos_user)
6. Error propagation and fail-closed security for required vs optional services
7. HTTP mock tests for ClickHouse (POST, DB=rask_logs) and Meilisearch (POST + task poll)
"""

import os
import stat
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent.parent


DOCKER_MOCK_TEMPLATE = r"""#!/bin/bash
echo "docker $*" >> "@LOG@"
args="$*"
if [[ "$args" =~ exec\ alt-backup\ restic ]]; then
    if [[ "$args" =~ \ snapshots\  ]]; then
        if [[ "$args" =~ --json ]]; then
            if [[ "$args" =~ --tag\ scheduled ]]; then
                if [[ -n "@VOLUME_ID@" ]]; then
                    echo '[{"short_id":"@VOLUME_ID@","id":"@VOLUME_ID@full","time":"2026-10-02T12:00:00Z"}]'
                else
                    echo '[]'
                fi
            else
                echo '[{"short_id":"pgsnap1","time":"2026-10-03T12:00:00Z"}]'
            fi
        fi
        exit 0
    fi
    if [[ "$args" =~ \ dump\  ]]; then
        if [[ "$args" =~ /postgres/([a-z-]+)\.dump ]]; then
            db="${BASH_REMATCH[1]}"
            for d in @DUMP_DBS@; do
                if [[ "$d" == "$db" ]]; then
                    echo "pgdump:$db"
                    exit 0
                fi
            done
        fi
        echo "Fatal: no matching snapshot" >&2
        exit 1
    fi
    if [[ "$args" =~ \ restore\  ]]; then
        target=""
        a=("$@")
        for ((i=0; i<${#a[@]}; i++)); do
            if [[ "${a[i]}" == "--target" ]]; then target="${a[i+1]}"; fi
        done
        if [[ -n "$target" ]]; then
            rel="${target#/}"
            mkdir -p "@CONTAINER_ROOT@/$rel/data"
            touch "@CONTAINER_ROOT@/$rel/data/vol1"
        fi
        exit @RESTORE_RC@
    fi
    exit 0
fi
if [[ "$args" =~ mktemp ]]; then
    echo "/tmp/alt-restore-verify-ctr-test$RANDOM"
    exit 0
fi
if [[ "$1" == "cp" ]]; then
    if [[ @CP_RC@ -ne 0 ]]; then echo "docker cp: error" >&2; exit @CP_RC@; fi
    if [[ "$2" =~ ^[^:]+:(.*) ]]; then
        p="${BASH_REMATCH[1]}"; p="${p%/}"; p="${p%/.}"
        mkdir -p "$3"
        cp -r "@CONTAINER_ROOT@/${p#/}/." "$3/"
        exit 0
    fi
    exit 1
fi
if [[ "$args" =~ pg_isready ]]; then exit 0; fi
if [[ "$args" =~ pg_restore ]]; then
    echo "pg_restore stdin: $(cat)" >> "@LOG@"
    exit @PG_RESTORE_RC@
fi
if [[ "$args" =~ information_schema.tables ]]; then
    echo "@TABLE_COUNT@"
    exit 0
fi
exit 0
"""


def run_restore_verify(
    script_path,
    tmpdir,
    extra_args,
    dump_dbs="alt-db kratos-db recap-db rag-db pact-db",
    volume_id="volsnap1",
    restore_rc=0,
    cp_rc=0,
    pg_restore_rc=0,
    table_count=10,
):
    mock_bin = Path(tmpdir) / "bin"
    mock_bin.mkdir()
    mock_log = Path(tmpdir) / "mock_docker.log"
    backup_root = Path(tmpdir) / "backups"
    backup_root.mkdir()
    container_root = Path(tmpdir) / "container_fs"
    container_root.mkdir()
    body = DOCKER_MOCK_TEMPLATE
    for key, value in {
        "@LOG@": str(mock_log),
        "@VOLUME_ID@": volume_id,
        "@DUMP_DBS@": dump_dbs,
        "@CONTAINER_ROOT@": str(container_root),
        "@RESTORE_RC@": str(restore_rc),
        "@CP_RC@": str(cp_rc),
        "@PG_RESTORE_RC@": str(pg_restore_rc),
        "@TABLE_COUNT@": str(table_count),
    }.items():
        body = body.replace(key, value)
    docker = mock_bin / "docker"
    docker.write_text(body)
    docker.chmod(0o755)

    env = os.environ.copy()
    env["PATH"] = f"{mock_bin}:{env['PATH']}"
    env["BACKUP_ROOT"] = str(backup_root)
    env["LOG_FILE"] = str(backup_root / "restore.log")
    env["METRICS_DIR"] = str(backup_root / "metrics")
    env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
    env.pop("IN_CONTAINER", None)
    res = subprocess.run(
        ["bash", str(script_path), *extra_args], env=env, capture_output=True, text=True, timeout=30
    )
    return res, mock_log.read_text() if mock_log.exists() else ""



class TestBackupDockerfile(unittest.TestCase):
    """Verify docker/backup/Dockerfile specifications."""

    def setUp(self):
        self.dockerfile = REPO_ROOT / "docker/backup/Dockerfile"
        self.assertTrue(self.dockerfile.is_file(), "Dockerfile must exist")
        self.content = self.dockerfile.read_text()

    def test_no_docker_cli(self):
        """Dockerfile must NOT install or contain docker-cli."""
        self.assertNotIn("docker-cli", self.content, "docker-cli must be removed from restic-backup")

    def test_postgres_client_version_exact_18(self):
        """Dockerfile must specify base image or package with exact PostgreSQL 18 client."""
        has_pg18 = ("postgres:18" in self.content) or ("postgresql18-client" in self.content)
        self.assertTrue(has_pg18, "Dockerfile must use exact PostgreSQL 18 client base or package")

    def test_supercronic_pinned_and_verified(self):
        """Supercronic binary must be checksum verified."""
        self.assertIn("SUPERCRONIC_VERSION=", self.content)
        self.assertIn("sha256sum -c", self.content)


class TestBackupComposeConfig(unittest.TestCase):
    """Verify compose/backup.yaml daemonless architecture."""

    def setUp(self):
        self.compose_file = REPO_ROOT / "compose/backup.yaml"
        self.assertTrue(self.compose_file.is_file(), "compose/backup.yaml must exist")
        self.content = self.compose_file.read_text()

    def test_no_docker_socket_mount(self):
        """No Docker socket bind mount allowed."""
        self.assertNotIn("/var/run/docker.sock", self.content)

    def test_no_docker_socket_proxy(self):
        """docker-socket-proxy service must be removed."""
        self.assertNotIn("docker-socket-proxy", self.content)
        self.assertNotIn("backup-docker-proxy", self.content)

    def test_no_docker_host_env(self):
        """DOCKER_HOST env var must not be present."""
        self.assertNotIn("DOCKER_HOST=", self.content)

    def test_required_secrets_mounted(self):
        """Verify necessary secrets are mounted into restic-backup."""
        for sec in [
            "restic_password",
            "postgres_password",
            "db_password",
            "kratos_db_password",
            "clickhouse_password",
            "meili_master_key",
        ]:
            self.assertIn(f"- {sec}", self.content, f"Secret {sec} must be mounted in backup.yaml")


class TestClickHouseBackupDiskConfig(unittest.TestCase):
    """Verify clickhouse/config/backup_disk.xml format and paths."""

    def setUp(self):
        self.xml_file = REPO_ROOT / "clickhouse/config/backup_disk.xml"
        self.assertTrue(self.xml_file.is_file(), "backup_disk.xml must exist")

    def test_valid_xml_structure(self):
        """XML must be well-formed with matching tags."""
        tree = ET.parse(self.xml_file)
        root = tree.getroot()
        self.assertEqual(root.tag, "clickhouse")

        disk_path = root.find("./storage_configuration/disks/backups/path")
        self.assertIsNotNone(disk_path, "Storage disk 'backups' path must be defined")
        self.assertEqual(disk_path.text.strip(), "/backups/clickhouse/")

        allowed_disk = root.find("./backups/allowed_disk")
        self.assertIsNotNone(allowed_disk)
        self.assertEqual(allowed_disk.text.strip(), "backups")

        allowed_path = root.find("./backups/allowed_path")
        self.assertIsNotNone(allowed_path)
        self.assertEqual(allowed_path.text.strip(), "/backups/clickhouse/")


class TestRestoreVerifyScript(unittest.TestCase):
    """Verify restore-verify.sh container fail-fast guard and bash syntax."""

    def setUp(self):
        self.script_path = REPO_ROOT / "scripts/backup/restore-verify.sh"
        self.assertTrue(self.script_path.is_file())

    def test_bash_n_syntax(self):
        """restore-verify.sh must pass bash -n check."""
        res = subprocess.run(["bash", "-n", str(self.script_path)], capture_output=True, text=True)
        self.assertEqual(res.returncode, 0, f"bash -n failed: {res.stderr}")

    def test_fail_fast_when_in_container(self):
        """restore-verify.sh must exit non-zero immediately when IN_CONTAINER is set."""
        env = os.environ.copy()
        env["IN_CONTAINER"] = "1"
        res = subprocess.run(
            ["bash", str(self.script_path), "--dry-run"],
            env=env,
            capture_output=True,
            text=True,
            timeout=15,
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("must be run on the host", res.stderr)

    def test_host_flow_mock_docker(self):
        """Dry-run on the host plans the volume check and per-database dump fetches without restoring anything."""
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--dry-run", "--skip-cleanup"])
            self.assertEqual(res.returncode, 0, f"{res.stderr}\n{res.stdout}")
            self.assertIn("Restore verification PASSED", res.stdout)
            self.assertNotIn("pg_restore", log)

    def test_volume_snapshot_selected_by_scheduled_tag(self):
        """The /data check uses the latest snapshot carrying the volume backup tag, not the newest snapshot of any kind."""
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"])
            self.assertEqual(res.returncode, 0, f"{res.stdout}\n{res.stderr}")
            self.assertIn("snapshots --tag scheduled --json", log)
            self.assertIn("Using snapshot: volsnap1", res.stdout)
            self.assertIn("restore volsnap1", log)

    def test_dumps_come_from_latest_good_pg_snapshot_per_database(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"])
            self.assertEqual(res.returncode, 0, f"{res.stdout}\n{res.stderr}")
            for db in ["alt-db", "kratos-db", "recap-db", "rag-db", "pact-db"]:
                self.assertIn(f"dump latest /postgres/{db}.dump --tag pg-{db}", log)
                self.assertIn(f"pg_restore stdin: pgdump:{db}", log)
                self.assertIn(f"PostgreSQL restore verification passed for {db}", res.stdout)

    def test_optional_database_without_dump_snapshot_is_skipped(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"], dump_dbs="alt-db kratos-db")
            self.assertEqual(res.returncode, 0, f"{res.stdout}\n{res.stderr}")
            self.assertIn("PostgreSQL restore verification passed for alt-db", res.stdout)
            self.assertNotIn("pg_restore stdin: pgdump:rag-db", log)

    def test_required_database_without_dump_snapshot_fails(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"], dump_dbs="alt-db")
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)

    def test_explicit_snapshot_id_selects_the_volume_snapshot_only(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(
                self.script_path, tmpdir, ["--snapshot", "customsnap99", "--skip-cleanup"], volume_id="customsnap99"
            )
            self.assertEqual(res.returncode, 0, f"{res.stdout}\n{res.stderr}")
            self.assertIn("Using snapshot: customsnap99", res.stdout)
            self.assertIn("restore customsnap99", log)
            self.assertIn("dump latest /postgres/alt-db.dump --tag pg-alt-db", log)

    def test_explicit_snapshot_id_outside_volume_snapshots_fails(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--snapshot", "pgsnap1", "--skip-cleanup"])
            self.assertNotEqual(res.returncode, 0)
            self.assertNotIn("restore pgsnap1", log)

    def test_no_volume_snapshot_fails(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"], volume_id="")
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)

    def test_pg_restore_failure_fails_closed(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"], pg_restore_rc=1)
            self.assertNotEqual(res.returncode, 0, "Restore must FAIL when pg_restore fails")
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)
            self.assertNotIn("PostgreSQL dump format valid", res.stdout)

    def test_restore_requires_table_count_greater_than_zero(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"], table_count=0)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("zero public tables restored", res.stdout + res.stderr)

    def test_failed_volume_restore_skips_pg_verify(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"], restore_rc=1)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)
            self.assertNotIn("pg_restore", log)

    def test_failed_docker_cp_skips_pg_verify(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"], cp_rc=1)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)
            self.assertNotIn("pg_restore", log)

    def test_container_namespace_docker_cp(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"])
            self.assertEqual(res.returncode, 0, f"{res.stdout}\n{res.stderr}")
            self.assertIn("docker cp alt-backup:", log)

    def test_restore_uses_matching_pg18_images(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"])
            self.assertEqual(res.returncode, 0, f"{res.stdout}\n{res.stderr}")
            self.assertNotIn("postgres:17-alpine", log)
            self.assertIn("postgres:18.6-alpine", log)
            self.assertNotIn("postgres:18.6-bookworm", log)
            self.assertIn("alt-recap-db", log)
            self.assertIn("shared_preload_libraries=pg_cron", log)
            self.assertIn("cron.database_name=verify_db", log)
            self.assertIn("cron.launch_active_jobs=off", log)
            self.assertNotIn("/var/lib/postgresql/data", log)
            self.assertIn(":/var/lib/postgresql", log)
            self.assertIn("--no-owner", log)
            self.assertIn("--no-privileges", log)

    def test_restore_includes_pact_db(self):
        content = self.script_path.read_text()
        self.assertIn("pact-db", content)

    def test_fetched_dumps_live_in_private_temp_dir(self):
        content = self.script_path.read_text()
        self.assertIn("umask 077", content)
        self.assertIn('mktemp -d "/tmp/${VERIFY_PREFIX}-dumps', content)
        self.assertIn('dump latest "/postgres/${db}.dump" --tag "pg-${db}"', content)



class TestBackupAllScriptBehavior(unittest.TestCase):
    """Verify backup-all.sh offline behavior, argv contracts, role credentials, and error handling."""

    def setUp(self):
        self.script_path = REPO_ROOT / "scripts/backup/backup-all.sh"
        self.assertTrue(self.script_path.is_file())

    def test_bash_n_syntax(self):
        """backup-all.sh must pass bash -n check."""
        res = subprocess.run(["bash", "-n", str(self.script_path)], capture_output=True, text=True, timeout=15)
        self.assertEqual(res.returncode, 0, f"bash -n failed: {res.stderr}")

    def test_in_container_enforcement(self):
        """backup-all.sh must refuse to run if IN_CONTAINER is not set."""
        env = os.environ.copy()
        env.pop("IN_CONTAINER", None)
        res = subprocess.run(["bash", str(self.script_path)], env=env, capture_output=True, text=True, timeout=15)
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("must run IN the backup container", res.stdout + res.stderr)

    def test_no_checkpoint_crash_consistency_false_claim(self):
        """backup-all.sh must not claim CHECKPOINT provides crash-consistency or PITR for raw volume copies."""
        content = self.script_path.read_text()
        self.assertNotIn("providing crash-consistency", content, "False crash-consistency claim must be removed")
        self.assertIn("auxiliary", content, "Must explain that raw volume copies are auxiliary/best-effort")

    def test_backup_all_configurable_backup_root_and_log_file_no_mkdir_mock(self):
        """backup-all.sh must support configurable BACKUP_ROOT, LOG_FILE, and METRICS_DIR without mkdir mocks."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            secrets_dir = Path(tmpdir) / "secrets"
            secrets_dir.mkdir()
            backup_dir = Path(tmpdir) / "custom_backups"
            backup_dir.mkdir()
            (backup_dir / "clickhouse").mkdir()
            custom_log = backup_dir / "logs" / "custom_backup.log"
            custom_metrics = backup_dir / "metrics"

            (secrets_dir / "restic_password").write_text("secret_restic")
            (secrets_dir / "postgres_password").write_text("secret_postgres")
            (secrets_dir / "kratos_db_password").write_text("secret_kratos")

            for cmd in ["restic", "pg_dump", "psql", "curl", "jq"]:
                p = mock_bin / cmd
                p.write_text("#!/bin/bash\nexit 0\n")
                p.chmod(0o755)

            (mock_bin / "getent").write_text("""#!/bin/bash
if [[ "$2" == "alt-db" || "$2" == "kratos-db" ]]; then
    exit 0
fi
exit 1
""")
            (mock_bin / "getent").chmod(0o755)

            env = {
                "PATH": f"{mock_bin}:{os.environ['PATH']}",
                "IN_CONTAINER": "1",
                "BACKUP_ROOT": str(backup_dir),
                "LOG_FILE": str(custom_log),
                "METRICS_DIR": str(custom_metrics),
                "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
                "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
                "KRATOS_DB_PASSWORD_FILE": str(secrets_dir / "kratos_db_password"),
            }

            # Run with real system /bin/mkdir (no mock mkdir!)
            res = subprocess.run(
                ["bash", str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertEqual(res.returncode, 0, f"backup-all.sh must succeed with real mkdir and custom BACKUP_ROOT/LOG_FILE: {res.stdout}\n{res.stderr}")
            self.assertTrue(custom_log.is_file(), f"LOG_FILE {custom_log} must be created")
            self.assertIn("Backup completed successfully", custom_log.read_text())

    def test_successful_backup_flow_and_argv_contracts(self):
        """Test full backup run with mock commands, verifying actual argv, roles, and HTTP POST queries."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "commands.log"
            secrets_dir = Path(tmpdir) / "secrets"
            secrets_dir.mkdir()
            backup_dir = Path(tmpdir) / "backups"
            backup_dir.mkdir()
            (backup_dir / "clickhouse").mkdir()

            # Create test secret files
            (secrets_dir / "postgres_password").write_text("secret_alt_postgres_pass")
            (secrets_dir / "db_password").write_text("secret_alt_app_pass")
            (secrets_dir / "kratos_db_password").write_text("secret_kratos_pass")
            (secrets_dir / "recap_db_password").write_text("secret_recap_pass")
            (secrets_dir / "rag_db_password").write_text("secret_rag_pass")
            (secrets_dir / "pact_db_password").write_text("secret_pact_pass")
            (secrets_dir / "clickhouse_password").write_text("secret_ch_pass")
            (secrets_dir / "meili_master_key").write_text("secret_meili_key")
            (secrets_dir / "restic_password").write_text("secret_restic_pass")

            # Mock restic
            (mock_bin / "restic").write_text(f"""#!/bin/bash
echo "restic $*" >> "{mock_log}"
if [[ "$*" =~ "--stdin" ]]; then
    cat > /dev/null
    exit 0
fi
if [[ "$*" =~ "snapshots" ]]; then
    echo '[]'
    exit 0
fi
if [[ "$*" =~ "stats" ]]; then
    echo '{{"total_size": 1024}}'
    exit 0
fi
exit 0
""")
            (mock_bin / "restic").chmod(0o755)

            # Mock pg_dump
            (mock_bin / "pg_dump").write_text(f"""#!/bin/bash
echo "pg_dump PGPASS=$(cat "$PGPASSFILE") $*" >> "{mock_log}"
echo "fake dump payload"
exit 0
""")
            (mock_bin / "pg_dump").chmod(0o755)

            # Mock psql
            (mock_bin / "psql").write_text(f"""#!/bin/bash
echo "psql PGPASS=$(cat "$PGPASSFILE") $*" >> "{mock_log}"
exit 0
""")
            (mock_bin / "psql").chmod(0o755)

            # Mock getent (simulate all services present in DNS)
            (mock_bin / "getent").write_text("""#!/bin/bash
exit 0
""")
            (mock_bin / "getent").chmod(0o755)

            # Mock curl
            (mock_bin / "curl").write_text(f"""#!/bin/bash
echo "curl $*" >> "{mock_log}"
out_file=""
args=("$@")
for ((i=0; i<${{#args[@]}}; i++)); do
    if [[ "${{args[i]}}" == "-o" && $((i+1)) -lt ${{#args[@]}} ]]; then
        out_file="${{args[i+1]}}"
    fi
done

if [[ "$*" =~ "/snapshots" ]]; then
    [[ -n "$out_file" ]] && echo '{{"taskUid": 42, "status": "enqueued"}}' > "$out_file"
    echo -n "202"
    exit 0
elif [[ "$*" =~ "/tasks/42" ]]; then
    [[ -n "$out_file" ]] && echo '{{"status": "succeeded"}}' > "$out_file"
    echo -n "200"
    exit 0
elif [[ "$*" =~ "clickhouse" ]]; then
    [[ -n "$out_file" ]] && echo "OK" > "$out_file"
    echo -n "200"
    exit 0
else
    echo -n "200"
    exit 0
fi
""")
            (mock_bin / "curl").chmod(0o755)

            # Mock jq
            (mock_bin / "jq").write_text("""#!/bin/bash
if [[ "$*" =~ "taskUid" ]]; then
    echo "42"
elif [[ "$*" =~ "status" ]]; then
    echo "succeeded"
elif [[ "$*" =~ "length" ]]; then
    echo "0"
elif [[ "$*" =~ "total_size" ]]; then
    echo "1024"
else
    echo ""
fi
""")
            (mock_bin / "jq").chmod(0o755)

            env = {
                "PATH": f"{mock_bin}:{os.environ['PATH']}",
                "IN_CONTAINER": "1",
                "BACKUP_ROOT": str(backup_dir),
                "LOG_FILE": str(backup_dir / "logs" / "backup.log"),
                "METRICS_DIR": str(backup_dir / "metrics"),
                "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
                "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
                "KRATOS_DB_PASSWORD_FILE": str(secrets_dir / "kratos_db_password"),
                "RECAP_DB_PASSWORD_FILE": str(secrets_dir / "recap_db_password"),
                "RAG_DB_PASSWORD_FILE": str(secrets_dir / "rag_db_password"),
                "PACT_DB_PASSWORD_FILE": str(secrets_dir / "pact_db_password"),
                "CLICKHOUSE_PASSWORD_FILE": str(secrets_dir / "clickhouse_password"),
                "MEILI_MASTER_KEY_FILE": str(secrets_dir / "meili_master_key"),
                "MEILI_POLL_INTERVAL": "1",
                "MEILI_POLL_TIMEOUT": "5",
            }

            res = subprocess.run(
                ["bash", str(self.script_path), "--prune", "--verify"],
                env=env,
                capture_output=True,
                text=True,
                timeout=30,
            )
            self.assertEqual(res.returncode, 0, f"backup-all.sh failed:\nSTDOUT:\n{res.stdout}\nSTDERR:\n{res.stderr}")
            self.assertIn("Backup completed successfully", res.stdout)

            log_content = mock_log.read_text()

            # Verify PostgreSQL argv and exact credentials pairing
            self.assertIn("pg_dump PGPASS=alt-db:*:alt:alt_db_user:secret_alt_postgres_pass -h alt-db -U alt_db_user -d alt", log_content)
            self.assertIn("pg_dump PGPASS=kratos-db:*:kratos:kratos_user:secret_kratos_pass -h kratos-db -U kratos_user -d kratos", log_content)

            # Verify ClickHouse HTTP POST native BACKUP with DB=rask_logs
            self.assertIn("BACKUP DATABASE rask_logs TO Disk('backups'", log_content)
            self.assertIn("-X POST", log_content)

            # Verify Meilisearch snapshot trigger and task poll
            self.assertIn("http://meilisearch:7700/snapshots", log_content)
            self.assertIn("http://meilisearch:7700/tasks/42", log_content)

            # Verify restic operations
            self.assertIn("backup --tag scheduled", log_content)
            self.assertIn("forget --keep-hourly 24", log_content)
            self.assertIn("check", log_content)

    def test_missing_credentials_fails_closed(self):
        """Missing credentials for detected database must cause backup failure (fail-closed)."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            secrets_dir = Path(tmpdir) / "secrets"
            secrets_dir.mkdir()
            backup_dir = Path(tmpdir) / "backups"
            backup_dir.mkdir()
            (backup_dir / "clickhouse").mkdir()

            # Only create restic password, missing alt-db password!
            (secrets_dir / "restic_password").write_text("secret_restic")

            # Fake commands
            for cmd in ["restic", "pg_dump", "psql", "curl", "jq"]:
                p = mock_bin / cmd
                p.write_text("#!/bin/bash\nexit 0\n")
                p.chmod(0o755)

            # getent reports services exist
            (mock_bin / "getent").write_text("#!/bin/bash\nexit 0\n")
            (mock_bin / "getent").chmod(0o755)

            env = {
                "PATH": f"{mock_bin}:{os.environ['PATH']}",
                "IN_CONTAINER": "1",
                "BACKUP_ROOT": str(backup_dir),
                "LOG_FILE": str(backup_dir / "logs" / "backup.log"),
                "METRICS_DIR": str(backup_dir / "metrics"),
                "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
                "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "non_existent_pwd_file"),
            }

            res = subprocess.run(
                ["bash", str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertNotEqual(res.returncode, 0, "Missing credentials file must fail closed with non-zero exit")
            self.assertIn("Missing credentials file", res.stdout + res.stderr)
            self.assertIn("Backup FAILED", res.stdout + res.stderr)

    def test_required_dns_failure_fails_closed(self):
        """Required DB (alt-db) DNS failure must fail whole backup non-zero."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            secrets_dir = Path(tmpdir) / "secrets"
            secrets_dir.mkdir()
            backup_dir = Path(tmpdir) / "backups"
            backup_dir.mkdir()
            (backup_dir / "clickhouse").mkdir()

            (secrets_dir / "restic_password").write_text("secret_restic")
            (secrets_dir / "postgres_password").write_text("secret_postgres")

            for cmd in ["restic", "pg_dump", "psql", "curl", "jq"]:
                p = mock_bin / cmd
                p.write_text("#!/bin/bash\nexit 0\n")
                p.chmod(0o755)

            # getent fails for all DNS queries
            (mock_bin / "getent").write_text("#!/bin/bash\nexit 1\n")
            (mock_bin / "getent").chmod(0o755)

            env = {
                "PATH": f"{mock_bin}:{os.environ['PATH']}",
                "IN_CONTAINER": "1",
                "BACKUP_ROOT": str(backup_dir),
                "LOG_FILE": str(backup_dir / "logs" / "backup.log"),
                "METRICS_DIR": str(backup_dir / "metrics"),
                "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
                "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
            }

            res = subprocess.run(
                ["bash", str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertNotEqual(res.returncode, 0, "Required DB DNS failure must fail whole backup")
            self.assertIn("Required database service alt-db", res.stdout + res.stderr)
            self.assertIn("Backup FAILED", res.stdout + res.stderr)

    def test_optional_service_skip_without_daemon(self):
        """Optional service not resolvable in DNS is safely skipped without error when required services pass."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            secrets_dir = Path(tmpdir) / "secrets"
            secrets_dir.mkdir()
            backup_dir = Path(tmpdir) / "backups"
            backup_dir.mkdir()
            (backup_dir / "clickhouse").mkdir()

            (mock_bin / "mkdir").write_text(f"""#!/bin/bash
args=("$@")
new_args=()
for arg in "${{args[@]}}"; do
    if [[ "$arg" == /backups/* ]]; then
        rel="${{arg#/backups/}}"
        target="{backup_dir}/$rel"
        new_args+=("$target")
    else
        new_args+=("$arg")
    fi
done
/bin/mkdir "${{new_args[@]}}"
""")
            (mock_bin / "mkdir").chmod(0o755)

            (secrets_dir / "restic_password").write_text("secret_restic")
            (secrets_dir / "postgres_password").write_text("secret_postgres")
            (secrets_dir / "kratos_db_password").write_text("secret_kratos")

            for cmd in ["restic", "pg_dump", "psql", "curl", "jq"]:
                p = mock_bin / cmd
                p.write_text("#!/bin/bash\nexit 0\n")
                p.chmod(0o755)

            # getent: resolves alt-db and kratos-db, but fails for recap/rag/pact/clickhouse/meili
            (mock_bin / "getent").write_text("""#!/bin/bash
if [[ "$2" == "alt-db" || "$2" == "kratos-db" ]]; then
    exit 0
fi
exit 1
""")
            (mock_bin / "getent").chmod(0o755)

            env = {
                "PATH": f"{mock_bin}:{os.environ['PATH']}",
                "IN_CONTAINER": "1",
                "BACKUP_ROOT": str(backup_dir),
                "LOG_FILE": str(backup_dir / "logs" / "backup.log"),
                "METRICS_DIR": str(backup_dir / "metrics"),
                "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
                "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
                "KRATOS_DB_PASSWORD_FILE": str(secrets_dir / "kratos_db_password"),
            }

            res = subprocess.run(
                ["bash", str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertEqual(res.returncode, 0, f"Optional services absence should not fail backup: {res.stdout}")
            self.assertIn("Optional database service recap-db (recap-db) not found in DNS; skipping", res.stdout)
            self.assertIn("Backup completed successfully", res.stdout)

    def test_clickhouse_error_fails_closed(self):
        """ClickHouse backup failure or missing credentials when detected must fail whole backup."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            secrets_dir = Path(tmpdir) / "secrets"
            secrets_dir.mkdir()
            backup_dir = Path(tmpdir) / "backups"
            backup_dir.mkdir()
            (backup_dir / "clickhouse").mkdir()

            (secrets_dir / "restic_password").write_text("secret_restic")
            (secrets_dir / "postgres_password").write_text("secret_postgres")
            (secrets_dir / "kratos_db_password").write_text("secret_kratos")
            # missing clickhouse_password!

            for cmd in ["restic", "pg_dump", "psql", "curl", "jq"]:
                p = mock_bin / cmd
                p.write_text("#!/bin/bash\nexit 0\n")
                p.chmod(0o755)

            # clickhouse is in DNS
            (mock_bin / "getent").write_text("""#!/bin/bash
if [[ "$2" == "alt-db" || "$2" == "kratos-db" || "$2" == "clickhouse" ]]; then
    exit 0
fi
exit 1
""")
            (mock_bin / "getent").chmod(0o755)

            env = {
                "PATH": f"{mock_bin}:{os.environ['PATH']}",
                "IN_CONTAINER": "1",
                "BACKUP_ROOT": str(backup_dir),
                "LOG_FILE": str(backup_dir / "logs" / "backup.log"),
                "METRICS_DIR": str(backup_dir / "metrics"),
                "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
                "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
                "KRATOS_DB_PASSWORD_FILE": str(secrets_dir / "kratos_db_password"),
                "CLICKHOUSE_PASSWORD_FILE": str(secrets_dir / "clickhouse_password"),
            }

            res = subprocess.run(["bash", str(self.script_path)], env=env, capture_output=True, text=True, timeout=15)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Missing credentials file", res.stdout)
            self.assertIn("Backup FAILED", res.stdout)

    def test_meilisearch_task_failure_fails_closed(self):
        """Meilisearch snapshot failure during polling must fail whole backup."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            secrets_dir = Path(tmpdir) / "secrets"
            secrets_dir.mkdir()
            backup_dir = Path(tmpdir) / "backups"
            backup_dir.mkdir()
            (backup_dir / "clickhouse").mkdir()

            (secrets_dir / "restic_password").write_text("secret_restic")
            (secrets_dir / "postgres_password").write_text("secret_postgres")
            (secrets_dir / "kratos_db_password").write_text("secret_kratos")
            (secrets_dir / "meili_master_key").write_text("secret_meili")

            for cmd in ["restic", "pg_dump", "psql"]:
                p = mock_bin / cmd
                p.write_text("#!/bin/bash\nexit 0\n")
                p.chmod(0o755)

            # meilisearch is in DNS
            (mock_bin / "getent").write_text("""#!/bin/bash
if [[ "$2" == "alt-db" || "$2" == "kratos-db" || "$2" == "meilisearch" ]]; then
    exit 0
fi
exit 1
""")
            (mock_bin / "getent").chmod(0o755)

            # curl: snapshot triggered, but task status is failed
            (mock_bin / "curl").write_text("""#!/bin/bash
out_file=""
args=("$@")
for ((i=0; i<${#args[@]}; i++)); do
    if [[ "${args[i]}" == "-o" && $((i+1)) -lt ${#args[@]} ]]; then
        out_file="${args[i+1]}"
    fi
done

if [[ "$*" =~ "/snapshots" ]]; then
    [[ -n "$out_file" ]] && echo '{"taskUid": 99}' > "$out_file"
    echo -n "202"
    exit 0
elif [[ "$*" =~ "/tasks/99" ]]; then
    [[ -n "$out_file" ]] && echo '{"status": "failed", "error": {"message": "disk full"}}' > "$out_file"
    echo -n "200"
    exit 0
fi
echo -n "200"
exit 0
""")
            (mock_bin / "curl").chmod(0o755)

            (mock_bin / "jq").write_text("""#!/bin/bash
if [[ "$*" =~ "taskUid" ]]; then
    echo "99"
elif [[ "$*" =~ "status" ]]; then
    echo "failed"
elif [[ "$*" =~ "error.message" ]]; then
    echo "disk full"
else
    echo ""
fi
""")
            (mock_bin / "jq").chmod(0o755)

            env = {
                "PATH": f"{mock_bin}:{os.environ['PATH']}",
                "IN_CONTAINER": "1",
                "BACKUP_ROOT": str(backup_dir),
                "LOG_FILE": str(backup_dir / "logs" / "backup.log"),
                "METRICS_DIR": str(backup_dir / "metrics"),
                "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
                "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
                "KRATOS_DB_PASSWORD_FILE": str(secrets_dir / "kratos_db_password"),
                "MEILI_MASTER_KEY_FILE": str(secrets_dir / "meili_master_key"),
                "MEILI_POLL_INTERVAL": "1",
                "MEILI_POLL_TIMEOUT": "3",
            }

            res = subprocess.run(["bash", str(self.script_path)], env=env, capture_output=True, text=True, timeout=15)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Meilisearch snapshot task failed", res.stdout)
            self.assertIn("Backup FAILED", res.stdout)

    def test_restic_failure_propagates_non_zero(self):
        """Restic backup failure must fail whole backup."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            secrets_dir = Path(tmpdir) / "secrets"
            secrets_dir.mkdir()
            backup_dir = Path(tmpdir) / "backups"
            backup_dir.mkdir()
            (backup_dir / "clickhouse").mkdir()

            (secrets_dir / "restic_password").write_text("secret_restic")
            (secrets_dir / "postgres_password").write_text("secret_postgres")
            (secrets_dir / "kratos_db_password").write_text("secret_kratos")

            for cmd in ["pg_dump", "psql", "curl", "jq"]:
                p = mock_bin / cmd
                p.write_text("#!/bin/bash\nexit 0\n")
                p.chmod(0o755)

            (mock_bin / "getent").write_text("""#!/bin/bash
if [[ "$2" == "alt-db" || "$2" == "kratos-db" ]]; then
    exit 0
fi
exit 1
""")
            (mock_bin / "getent").chmod(0o755)

            # Restic fails
            (mock_bin / "restic").write_text("""#!/bin/bash
if [[ "$*" =~ "--stdin" ]]; then
    cat > /dev/null
    exit 0
fi
if [[ "$*" =~ " tag " ]]; then
    exit 0
fi
echo "Fatal: repository locked" >&2
exit 1
""")
            (mock_bin / "restic").chmod(0o755)

            env = {
                "PATH": f"{mock_bin}:{os.environ['PATH']}",
                "IN_CONTAINER": "1",
                "BACKUP_ROOT": str(backup_dir),
                "LOG_FILE": str(backup_dir / "logs" / "backup.log"),
                "METRICS_DIR": str(backup_dir / "metrics"),
                "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
                "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
                "KRATOS_DB_PASSWORD_FILE": str(secrets_dir / "kratos_db_password"),
            }

            res = subprocess.run(["bash", str(self.script_path)], env=env, capture_output=True, text=True, timeout=15)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Restic volume backup failed", res.stdout)
            self.assertIn("Backup FAILED", res.stdout)


class TestBackupAllNoPlaintextNoArgvSecrets(unittest.TestCase):
    """Credentials stay off command lines and dumps never reach disk unencrypted."""

    FIXTURE_FILES = {
        "postgres_password": "secret_alt_postgres_pass",
        "kratos_db_password": "secret_kratos_pass",
        "clickhouse_password": "secret_ch_pass",
        "meili_master_key": "secret_meili_key",
        "restic_password": "secret_restic_pass",
    }

    def setUp(self):
        self.script_path = REPO_ROOT / "scripts/backup/backup-all.sh"

    def _run(self, tmpdir, extra_args=(), restic_script=None, drop_restic_password=False, pg_dump_rc=0):
        mock_bin = Path(tmpdir) / "bin"
        mock_bin.mkdir()
        mock_log = Path(tmpdir) / "commands.log"
        secrets_dir = Path(tmpdir) / "secrets"
        secrets_dir.mkdir()
        backup_dir = Path(tmpdir) / "backups"
        backup_dir.mkdir()
        (backup_dir / "clickhouse").mkdir()
        for name, value in self.FIXTURE_FILES.items():
            if drop_restic_password and name == "restic_password":
                continue
            (secrets_dir / name).write_text(value)

        scripts = {
            "restic": restic_script
            or f"""#!/bin/bash
echo "restic $*" >> "{mock_log}"
if [[ "$*" =~ "--stdin" ]]; then
    cat > /dev/null
    exit 0
fi
if [[ "$*" =~ "snapshots" ]]; then echo '[]'; fi
if [[ "$*" =~ "stats" ]]; then echo '{{"total_size": 1}}'; fi
exit 0
""",
            "pg_dump": f"""#!/bin/bash
echo "pg_dump env_has_PGPASSWORD=${{PGPASSWORD:+yes}} file=$PGPASSFILE mode=$(stat -c %a "$PGPASSFILE") args=$*" >> "{mock_log}"
echo "fake dump payload"
exit {pg_dump_rc}
""",
            "psql": f"""#!/bin/bash
echo "psql env_has_PGPASSWORD=${{PGPASSWORD:+yes}} args=$*" >> "{mock_log}"
""",
            "getent": '#!/bin/bash\n[[ "$2" == "alt-db" || "$2" == "kratos-db" || "$2" == "clickhouse" || "$2" == "meilisearch" ]]\n',
            "curl": f"""#!/bin/bash
echo "curl argv: $*" >> "{mock_log}"
out_file=""; cfg=""
args=("$@")
for ((i=0; i<${{#args[@]}}; i++)); do
    case "${{args[i]}}" in
        -o) out_file="${{args[i+1]}}" ;;
        -K|--config) cfg="${{args[i+1]}}" ;;
    esac
done
if [[ -n "$cfg" && "$cfg" != "-" ]]; then
    echo "curl cfg mode=$(stat -c %a "$cfg") body=$(cat "$cfg")" >> "{mock_log}"
fi
if [[ "$*" =~ "/snapshots" ]]; then
    [[ -n "$out_file" ]] && echo '{{"taskUid": 42}}' > "$out_file"
    echo -n 202
elif [[ "$*" =~ "/tasks/42" ]]; then
    [[ -n "$out_file" ]] && echo '{{"status": "succeeded"}}' > "$out_file"
    echo -n 200
else
    echo -n 200
fi
""",
        }
        scripts["jq"] = (
            "#!/bin/bash\n"
            'if [[ "$*" =~ "taskUid" ]]; then echo 42\n'
            'elif [[ "$*" =~ "status" ]]; then echo succeeded\n'
            'elif [[ "$*" == *".id"* ]]; then exec /usr/bin/jq "$@"\n'
            "else echo 0; fi\n"
        )
        for name, body in scripts.items():
            f = mock_bin / name
            f.write_text(body)
            f.chmod(0o755)

        env = {
            "PATH": f"{mock_bin}:{os.environ['PATH']}",
            "IN_CONTAINER": "1",
            "BACKUP_ROOT": str(backup_dir),
            "LOG_FILE": str(backup_dir / "logs" / "backup.log"),
            "METRICS_DIR": str(backup_dir / "metrics"),
            "RESTIC_REPOSITORY": str(backup_dir / "restic-repo"),
            "RESTIC_PASSWORD_FILE": str(secrets_dir / "restic_password"),
            "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
            "KRATOS_DB_PASSWORD_FILE": str(secrets_dir / "kratos_db_password"),
            "CLICKHOUSE_PASSWORD_FILE": str(secrets_dir / "clickhouse_password"),
            "MEILI_MASTER_KEY_FILE": str(secrets_dir / "meili_master_key"),
            "MEILI_POLL_INTERVAL": "1",
            "MEILI_POLL_TIMEOUT": "5",
        }
        res = subprocess.run(
            ["bash", str(self.script_path), *extra_args],
            env=env, capture_output=True, text=True, timeout=30,
        )
        log = mock_log.read_text() if mock_log.exists() else ""
        return res, log, backup_dir

    def test_no_secret_value_on_any_command_line(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, _ = self._run(tmpdir)
            self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
            for line in log.splitlines():
                if line.startswith("curl cfg"):
                    continue
                for value in self.FIXTURE_FILES.values():
                    self.assertNotIn(value, line, f"secret leaked on command line: {line}")
            self.assertNotIn("env_has_PGPASSWORD=yes", log)

    def test_curl_credentials_come_from_private_config_file(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, _ = self._run(tmpdir)
            self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
            cfg_lines = [l for l in log.splitlines() if l.startswith("curl cfg")]
            self.assertTrue(any("secret_ch_pass" in l for l in cfg_lines))
            self.assertTrue(any("secret_meili_key" in l for l in cfg_lines))
            for l in cfg_lines:
                self.assertIn("mode=600", l)

    def test_pg_dump_uses_private_passfile(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, _ = self._run(tmpdir)
            self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
            dump_lines = [l for l in log.splitlines() if l.startswith("pg_dump")]
            self.assertEqual(len(dump_lines), 2)
            for l in dump_lines:
                self.assertIn("mode=600", l)

    def test_dumps_stream_into_restic_and_never_touch_disk(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, backup_dir = self._run(tmpdir, ("--pg-only",))
            self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
            self.assertEqual(list(backup_dir.rglob("*.dump")), [])
            stdin_lines = [l for l in log.splitlines() if l.startswith("restic") and "--stdin" in l]
            self.assertEqual(len(stdin_lines), 2)
            for l in stdin_lines:
                self.assertNotIn("--tag pg-", l, "selectable tags must not be present at backup time")
            self.assertTrue(any("--stdin-filename postgres/alt-db.dump" in l for l in stdin_lines))
            tag_lines = [l for l in log.splitlines() if l.startswith("restic") and " tag " in l]
            self.assertTrue(any("--add pg-dump" in l and "--add pg-alt-db" in l for l in tag_lines))
            self.assertTrue(any("--stdin-filename postgres/kratos-db.dump" in l for l in stdin_lines))

    def test_dry_run_creates_no_dump_paths(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, backup_dir = self._run(tmpdir, ("--dry-run",))
            self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
            self.assertEqual(list(backup_dir.rglob("*.dump")), [])
            self.assertFalse((backup_dir / "postgres").exists())

    def test_missing_restic_password_is_an_error(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, backup_dir = self._run(tmpdir, drop_restic_password=True)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("restic password file", (res.stdout + res.stderr).lower())
            self.assertNotIn("pg_dump", log)
            self.assertEqual(list(backup_dir.rglob("*.dump")), [])

    def test_failed_dump_stream_fails_backup(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            restic = f"""#!/bin/bash
echo "restic $*" >> "{Path(tmpdir) / 'commands.log'}"
if [[ "$*" =~ "--stdin" ]]; then cat > /dev/null; exit 1; fi
exit 0
"""
            res, log, _ = self._run(tmpdir, ("--pg-only",), restic_script=restic)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Backup FAILED", res.stdout)

    def test_pg_dump_failure_forgets_snapshot_by_id_and_leaves_no_selectable_tag(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            log_path = Path(tmpdir) / "commands.log"
            restic = f"""#!/bin/bash
echo "restic $*" >> "{log_path}"
if [[ "$*" =~ "--stdin" ]]; then cat > /dev/null; exit 0; fi
if [[ "$*" =~ "snapshots" && "$*" =~ "--tag pgdump-" ]]; then
    echo '[{{"id":"deadbeef01"}}]'
    exit 0
fi
if [[ "$*" =~ "snapshots" ]]; then echo '[]'; fi
exit 0
"""
            res, log, _ = self._run(tmpdir, ("--pg-only",), restic_script=restic, pg_dump_rc=1)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Backup FAILED", res.stdout)
            self.assertIn("forget deadbeef01", log)
            self.assertFalse([l for l in log.splitlines() if l.startswith("restic") and " tag " in l and "--add" in l])
            for l in log.splitlines():
                if l.startswith("restic") and "--stdin" in l:
                    self.assertNotIn("pg-alt-db", l)

    def test_failed_snapshot_cleanup_is_reported_as_error(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            log_path = Path(tmpdir) / "commands.log"
            restic = f"""#!/bin/bash
echo "restic $*" >> "{log_path}"
if [[ "$*" =~ "--stdin" ]]; then cat > /dev/null; exit 0; fi
if [[ "$*" =~ "snapshots" && "$*" =~ "--tag pgdump-" ]]; then echo '[{{"id":"deadbeef01"}}]'; exit 0; fi
if [[ "$*" =~ "forget" && ! "$*" =~ "--prune" ]]; then echo "Fatal: locked" >&2; exit 1; fi
if [[ "$*" =~ "snapshots" ]]; then echo '[]'; fi
exit 0
"""
            res, log, _ = self._run(tmpdir, ("--pg-only",), restic_script=restic, pg_dump_rc=1)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Failed to remove incomplete dump snapshot deadbeef01", res.stdout)

    def test_scripts_have_no_inline_credentials(self):
        for name in ("backup-all.sh", "sync-offsite.sh", "restore-verify.sh", "backup-metrics.sh"):
            content = (REPO_ROOT / "scripts/backup" / name).read_text()
            self.assertNotIn("PGPASSWORD", content, name)
            self.assertNotRegex(content, r"Bearer \$\{?[A-Za-z_]", name)
            self.assertNotIn('-u "${', content, name)


class TestRestoreVerifyStreamedDumps(unittest.TestCase):
    def test_restore_verify_does_not_search_restored_tree_for_dumps(self):
        content = (REPO_ROOT / "scripts/backup/restore-verify.sh").read_text()
        self.assertNotIn("-name \"${db}", content)

    def test_metrics_read_dump_status_from_snapshots_not_plaintext_dir(self):
        content = (REPO_ROOT / "scripts/backup/backup-metrics.sh").read_text()
        self.assertNotIn("POSTGRES_BACKUP_DIR", content)
        self.assertIn("pg-${db}", content)


if __name__ == "__main__":
    unittest.main(verbosity=2)
