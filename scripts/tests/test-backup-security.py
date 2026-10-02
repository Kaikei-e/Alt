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
        """restore-verify.sh runs on host using Docker CLI to spin up temp verification containers."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "mock_docker.log"
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            log_file = backup_root / "restore.log"
            metrics_dir = backup_root / "metrics"

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
echo "docker $*" >> "{mock_log}"
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"testsnap123","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "information_schema.tables" ]]; then
    echo "12"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "testsnap123"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(log_file)
            env["METRICS_DIR"] = str(metrics_dir)
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            # Dry-run execution
            res = subprocess.run(
                ["bash", str(self.script_path), "--dry-run", "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertEqual(res.returncode, 0, f"Host restore verification dry-run failed: {res.stderr}\n{res.stdout}")
            self.assertIn("Restore verification PASSED", res.stdout)

    def test_pg_restore_failure_with_valid_archive_fails_closed(self):
        """pg_restore execution failure MUST NOT falsely succeed even if archive header is parseable (--list)."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "mock_docker.log"
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            log_file = backup_root / "restore.log"
            metrics_dir = backup_root / "metrics"

            container_root = Path(tmpdir) / "container_fs"
            container_root.mkdir()

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
echo "docker $*" >> "{mock_log}"
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"testsnap123","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "mktemp" ]]; then
    dir="/tmp/alt-restore-verify-ctr-test$RANDOM"
    echo "$dir"
    exit 0
fi
if [[ "$*" =~ "restore" && "$*" =~ "--target" ]]; then
    target=""
    args=("$@")
    for ((i=0; i<${{#args[@]}}; i++)); do
        if [[ "${{args[i]}}" == "--target" && $((i+1)) -lt ${{#args[@]}} ]]; then
            target="${{args[i+1]}}"
        fi
    done
    if [[ -n "$target" ]]; then
        rel="${{target#/}}"
        ctr_target="{container_root}/$rel"
        mkdir -p "$ctr_target/data"
        touch "$ctr_target/data/vol1"
        # Create dump file inside restored snapshot in container namespace
        mkdir -p "$ctr_target/backups/postgres"
        echo "fake dump content" > "$ctr_target/backups/postgres/alt-db-20261002.dump"
        echo "fake kratos dump" > "$ctr_target/backups/postgres/kratos-db-20261002.dump"
    fi
    exit 0
fi
if [[ "$1" == "cp" ]]; then
    src="$2"
    dest="$3"
    if [[ "$src" =~ ^[^:]+:(.*) ]]; then
        ctr_path="${{BASH_REMATCH[1]}}"
        ctr_path="${{ctr_path%/}}"
        ctr_path="${{ctr_path%/.}}"
        rel="${{ctr_path#/}}"
        real_src="{container_root}/$rel"
        mkdir -p "$dest"
        cp -r "$real_src/." "$dest/"
        exit 0
    fi
    exit 1
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "pg_restore" ]]; then
    if [[ "$*" =~ "--list" ]]; then
        # Archive header parse succeeds!
        echo "; Archive created by pg_dump"
        exit 0
    fi
    # BUT actual data restore execution fails!
    echo "pg_restore: error: could not execute query: table corrupted" >&2
    exit 1
fi
if [[ "$*" =~ "information_schema.tables" ]]; then
    echo "0"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "testsnap123"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(log_file)
            env["METRICS_DIR"] = str(metrics_dir)
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            res = subprocess.run(
                ["bash", str(self.script_path), "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertNotEqual(res.returncode, 0, "Restore must FAIL when pg_restore fails, even if --list passes")
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)
            self.assertNotIn("PostgreSQL dump format valid", res.stdout)

    def test_restore_uses_matching_pg18_images(self):
        """restore-verify.sh must use PG18 images matching Compose: pgvector for rag-db, alt-recap-db with pg_cron flags for recap-db, alpine for others, and --no-owner --no-privileges."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "mock_docker.log"
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            log_file = backup_root / "restore.log"
            metrics_dir = backup_root / "metrics"
            container_root = Path(tmpdir) / "container_fs"
            container_root.mkdir()

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
echo "docker $*" >> "{mock_log}"
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"testsnap123","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "mktemp" ]]; then
    dir="/tmp/alt-restore-verify-ctr-test$RANDOM"
    echo "$dir"
    exit 0
fi
if [[ "$*" =~ "restore" && "$*" =~ "--target" ]]; then
    target=""
    args=("$@")
    for ((i=0; i<${{#args[@]}}; i++)); do
        if [[ "${{args[i]}}" == "--target" && $((i+1)) -lt ${{#args[@]}} ]]; then
            target="${{args[i+1]}}"
        fi
    done
    if [[ -n "$target" ]]; then
        rel="${{target#/}}"
        ctr_target="{container_root}/$rel"
        mkdir -p "$ctr_target/data"
        touch "$ctr_target/data/vol1"
        mkdir -p "$ctr_target/backups/postgres"
        for db in alt-db kratos-db recap-db rag-db pact-db; do
            echo "dump $db" > "$ctr_target/backups/postgres/${{db}}-20261002.dump"
        done
    fi
    exit 0
fi
if [[ "$1" == "cp" ]]; then
    src="$2"
    dest="$3"
    if [[ "$src" =~ ^[^:]+:(.*) ]]; then
        ctr_path="${{BASH_REMATCH[1]}}"
        ctr_path="${{ctr_path%/}}"
        ctr_path="${{ctr_path%/.}}"
        rel="${{ctr_path#/}}"
        real_src="{container_root}/$rel"
        mkdir -p "$dest"
        cp -r "$real_src/." "$dest/"
        exit 0
    fi
    exit 1
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "pg_restore" ]]; then
    exit 0
fi
if [[ "$*" =~ "information_schema.tables" ]]; then
    echo "15"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "testsnap123"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(log_file)
            env["METRICS_DIR"] = str(metrics_dir)
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            res = subprocess.run(
                ["bash", str(self.script_path), "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertEqual(res.returncode, 0, f"Restore verification should pass: {res.stdout}\n{res.stderr}")

            log_content = mock_log.read_text()
            # Must NOT use postgres:17-alpine
            self.assertNotIn("postgres:17-alpine", log_content, "Must not use postgres:17-alpine")
            # Must use PG18 alpine for standard dbs
            self.assertIn("postgres:18.6-alpine", log_content)
            # Must use actual built Compose image alt-recap-db (NOT plain bookworm missing pg_cron)
            self.assertNotIn("postgres:18.6-bookworm", log_content, "Must NOT use plain bookworm lacking pg_cron")
            self.assertIn("alt-recap-db", log_content, "Must use actual built Compose image alt-recap-db")
            # Must configure pg_cron flags to preload and disable active restored jobs
            self.assertIn("shared_preload_libraries=pg_cron", log_content)
            self.assertIn("cron.database_name=verify_db", log_content)
            self.assertIn("cron.launch_active_jobs=off", log_content)
            # Must use PG18 volume mount /var/lib/postgresql (not legacy /var/lib/postgresql/data)
            self.assertNotIn("/var/lib/postgresql/data", log_content, "Must not use legacy /var/lib/postgresql/data mount")
            self.assertIn(":/var/lib/postgresql", log_content, "Must use PG18 /var/lib/postgresql mount")
            # Must include --no-owner and --no-privileges in pg_restore
            self.assertIn("--no-owner", log_content, "pg_restore must include --no-owner")
            self.assertIn("--no-privileges", log_content, "pg_restore must include --no-privileges")

    def test_container_namespace_docker_cp(self):
        """restore-verify.sh must docker cp restored data from container namespace to host temp dir."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "mock_docker.log"
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            log_file = backup_root / "restore.log"
            metrics_dir = backup_root / "metrics"
            container_root = Path(tmpdir) / "container_fs"
            container_root.mkdir()

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
echo "docker $*" >> "{mock_log}"
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"testsnap123","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "mktemp" ]]; then
    dir="/tmp/alt-restore-verify-ctr-test$RANDOM"
    echo "$dir"
    exit 0
fi
if [[ "$*" =~ "restore" && "$*" =~ "--target" ]]; then
    target=""
    args=("$@")
    for ((i=0; i<${{#args[@]}}; i++)); do
        if [[ "${{args[i]}}" == "--target" && $((i+1)) -lt ${{#args[@]}} ]]; then
            target="${{args[i+1]}}"
        fi
    done
    if [[ -n "$target" ]]; then
        rel="${{target#/}}"
        ctr_target="{container_root}/$rel"
        mkdir -p "$ctr_target/data"
        touch "$ctr_target/data/vol1"
        mkdir -p "$ctr_target/backups/postgres"
        echo "dump alt" > "$ctr_target/backups/postgres/alt-db-20261002.dump"
        echo "dump kratos" > "$ctr_target/backups/postgres/kratos-db-20261002.dump"
    fi
    exit 0
fi
if [[ "$1" == "cp" ]]; then
    src="$2"
    dest="$3"
    if [[ "$src" =~ ^[^:]+:(.*) ]]; then
        ctr_path="${{BASH_REMATCH[1]}}"
        ctr_path="${{ctr_path%/}}"
        ctr_path="${{ctr_path%/.}}"
        rel="${{ctr_path#/}}"
        real_src="{container_root}/$rel"
        mkdir -p "$dest"
        cp -r "$real_src/." "$dest/"
        exit 0
    fi
    exit 1
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "pg_restore" ]]; then
    exit 0
fi
if [[ "$*" =~ "information_schema.tables" ]]; then
    echo "10"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "testsnap123"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(log_file)
            env["METRICS_DIR"] = str(metrics_dir)
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            res = subprocess.run(
                ["bash", str(self.script_path), "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertEqual(res.returncode, 0, f"Restore must succeed when docker cp copies container output: {res.stdout}\n{res.stderr}")
            log_content = mock_log.read_text()
            self.assertIn("docker cp alt-backup:", log_content, "Must invoke docker cp from alt-backup container")

    def test_restore_reads_snapshot_target_not_live_dir(self):
        """restore-verify.sh must inspect dumps extracted into RESTORE_DIR, not live POSTGRES_BACKUP_DIR."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "mock_docker.log"
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            log_file = backup_root / "restore.log"
            metrics_dir = backup_root / "metrics"
            live_pg_dir = backup_root / "postgres"
            live_pg_dir.mkdir()
            # Leave live_pg_dir completely empty!

            container_root = Path(tmpdir) / "container_fs"
            container_root.mkdir()

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
echo "docker $*" >> "{mock_log}"
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"testsnap123","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "mktemp" ]]; then
    dir="/tmp/alt-restore-verify-ctr-test$RANDOM"
    echo "$dir"
    exit 0
fi
if [[ "$*" =~ "restore" && "$*" =~ "--target" ]]; then
    target=""
    args=("$@")
    for ((i=0; i<${{#args[@]}}; i++)); do
        if [[ "${{args[i]}}" == "--target" && $((i+1)) -lt ${{#args[@]}} ]]; then
            target="${{args[i+1]}}"
        fi
    done
    if [[ -n "$target" ]]; then
        rel="${{target#/}}"
        ctr_target="{container_root}/$rel"
        mkdir -p "$ctr_target/data"
        touch "$ctr_target/data/vol1"
        mkdir -p "$ctr_target/backups/postgres"
        # Write dumps ONLY into restored target directory!
        echo "alt dump" > "$ctr_target/backups/postgres/alt-db-20261002.dump"
        echo "kratos dump" > "$ctr_target/backups/postgres/kratos-db-20261002.dump"
    fi
    exit 0
fi
if [[ "$1" == "cp" ]]; then
    src="$2"
    dest="$3"
    if [[ "$src" =~ ^[^:]+:(.*) ]]; then
        ctr_path="${{BASH_REMATCH[1]}}"
        ctr_path="${{ctr_path%/}}"
        ctr_path="${{ctr_path%/.}}"
        rel="${{ctr_path#/}}"
        real_src="{container_root}/$rel"
        mkdir -p "$dest"
        cp -r "$real_src/." "$dest/"
        exit 0
    fi
    exit 1
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "pg_restore" ]]; then
    exit 0
fi
if [[ "$*" =~ "information_schema.tables" ]]; then
    echo "10"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "testsnap123"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(log_file)
            env["METRICS_DIR"] = str(metrics_dir)
            env["POSTGRES_BACKUP_DIR"] = str(live_pg_dir)
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            res = subprocess.run(
                ["bash", str(self.script_path), "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertEqual(res.returncode, 0, f"Restore must succeed reading dumps extracted from snapshot: {res.stdout}\n{res.stderr}")
            self.assertIn("PostgreSQL restore verification passed for alt-db", res.stdout)
            self.assertIn("PostgreSQL restore verification passed for kratos-db", res.stdout)

    def test_restore_includes_pact_db(self):
        """restore-verify.sh must include pact-db in verified databases."""
        content = self.script_path.read_text()
        self.assertIn("pact-db", content, "pact-db must be included in restore databases list")

    def test_restore_requires_table_count_greater_than_zero(self):
        """pg_restore with 0 restored tables must fail verification."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            container_root = Path(tmpdir) / "container_fs"
            container_root.mkdir()

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"testsnap123","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "mktemp" ]]; then
    dir="/tmp/alt-restore-verify-ctr-test$RANDOM"
    echo "$dir"
    exit 0
fi
if [[ "$*" =~ "restore" && "$*" =~ "--target" ]]; then
    target=""
    args=("$@")
    for ((i=0; i<${{#args[@]}}; i++)); do
        if [[ "${{args[i]}}" == "--target" && $((i+1)) -lt ${{#args[@]}} ]]; then
            target="${{args[i+1]}}"
        fi
    done
    if [[ -n "$target" ]]; then
        rel="${{target#/}}"
        ctr_target="{container_root}/$rel"
        mkdir -p "$ctr_target/data"
        touch "$ctr_target/data/vol1"
        mkdir -p "$ctr_target/backups/postgres"
        echo "alt dump" > "$ctr_target/backups/postgres/alt-db-20261002.dump"
        echo "kratos dump" > "$ctr_target/backups/postgres/kratos-db-20261002.dump"
    fi
    exit 0
fi
if [[ "$1" == "cp" ]]; then
    src="$2"
    dest="$3"
    if [[ "$src" =~ ^[^:]+:(.*) ]]; then
        ctr_path="${{BASH_REMATCH[1]}}"
        ctr_path="${{ctr_path%/}}"
        ctr_path="${{ctr_path%/.}}"
        rel="${{ctr_path#/}}"
        real_src="{container_root}/$rel"
        mkdir -p "$dest"
        cp -r "$real_src/." "$dest/"
        exit 0
    fi
    exit 1
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "pg_restore" ]]; then
    exit 0
fi
if [[ "$*" =~ "information_schema.tables" ]]; then
    # Return 0 tables restored!
    echo "0"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "testsnap123"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(backup_root / "restore.log")
            env["METRICS_DIR"] = str(backup_root / "metrics")
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            res = subprocess.run(
                ["bash", str(self.script_path), "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertNotEqual(res.returncode, 0, "Restore must FAIL when table count is 0")
            self.assertIn("zero public tables restored", res.stdout + res.stderr)
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)

    def test_failed_restic_writes_partial_dump_must_fail_overall_no_pg_verify(self):
        """When restic fails non-zero despite writing a partial dump, restore verification MUST fail non-zero with no PG verify."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "mock_docker.log"
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            container_root = Path(tmpdir) / "container_fs"
            container_root.mkdir()

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
echo "docker $*" >> "{mock_log}"
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"testsnap123","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "mktemp" ]]; then
    dir="/tmp/alt-restore-verify-ctr-mktemp-$RANDOM"
    echo "$dir"
    exit 0
fi
if [[ "$*" =~ "restore" && "$*" =~ "--target" ]]; then
    target=""
    args=("$@")
    for ((i=0; i<${{#args[@]}}; i++)); do
        if [[ "${{args[i]}}" == "--target" && $((i+1)) -lt ${{#args[@]}} ]]; then
            target="${{args[i+1]}}"
        fi
    done
    if [[ -n "$target" ]]; then
        rel="${{target#/}}"
        ctr_target="{container_root}/$rel"
        mkdir -p "$ctr_target/data"
        touch "$ctr_target/data/vol1"
        mkdir -p "$ctr_target/backups/postgres"
        # Partial valid dump written before restic failed!
        echo "partial dump content" > "$ctr_target/backups/postgres/alt-db-20261002.dump"
        echo "partial dump content" > "$ctr_target/backups/postgres/kratos-db-20261002.dump"
    fi
    echo "restic fatal: network timeout during snapshot restore" >&2
    exit 1
fi
if [[ "$1" == "cp" ]]; then
    src="$2"
    dest="$3"
    if [[ "$src" =~ ^[^:]+:(.*) ]]; then
        ctr_path="${{BASH_REMATCH[1]}}"
        ctr_path="${{ctr_path%/}}"
        ctr_path="${{ctr_path%/.}}"
        rel="${{ctr_path#/}}"
        real_src="{container_root}/$rel"
        mkdir -p "$dest"
        cp -r "$real_src/." "$dest/"
        exit 0
    fi
    exit 1
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "pg_restore" ]]; then
    echo "pg_restore executed!" >> "{mock_log}"
    exit 0
fi
if [[ "$*" =~ "information_schema.tables" ]]; then
    echo "10"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "testsnap123"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(backup_root / "restore.log")
            env["METRICS_DIR"] = str(backup_root / "metrics")
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            res = subprocess.run(
                ["bash", str(self.script_path), "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertNotEqual(res.returncode, 0, "Restore must FAIL when restic restore exits non-zero")
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)
            log_content = mock_log.read_text()
            self.assertNotIn("pg_restore", log_content, "pg_restore must NOT be executed after restic failure")

    def test_failed_docker_cp_must_fail_overall_no_pg_verify(self):
        """When docker cp fails, restore verification MUST fail non-zero with no PG verify."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "mock_docker.log"
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            container_root = Path(tmpdir) / "container_fs"
            container_root.mkdir()

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
echo "docker $*" >> "{mock_log}"
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"testsnap123","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "mktemp" ]]; then
    dir="/tmp/alt-restore-verify-ctr-mktemp-$RANDOM"
    echo "$dir"
    exit 0
fi
if [[ "$*" =~ "restore" && "$*" =~ "--target" ]]; then
    exit 0
fi
if [[ "$1" == "cp" ]]; then
    echo "docker cp: error copying from container" >&2
    exit 1
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "pg_restore" ]]; then
    echo "pg_restore executed!" >> "{mock_log}"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "testsnap123"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(backup_root / "restore.log")
            env["METRICS_DIR"] = str(backup_root / "metrics")
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            res = subprocess.run(
                ["bash", str(self.script_path), "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertNotEqual(res.returncode, 0, "Restore must FAIL when docker cp fails")
            self.assertIn("Restore verification FAILED", res.stdout + res.stderr)
            log_content = mock_log.read_text()
            self.assertNotIn("pg_restore", log_content, "pg_restore must NOT be executed after docker cp failure")

    def test_successful_selected_snapshot_copyback(self):
        """Successful selected snapshot copyback from container namespace to separate host temp namespace."""
        with tempfile.TemporaryDirectory() as tmpdir:
            mock_bin = Path(tmpdir) / "bin"
            mock_bin.mkdir()
            mock_log = Path(tmpdir) / "mock_docker.log"
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            container_root = Path(tmpdir) / "container_fs"
            container_root.mkdir()

            docker_script = mock_bin / "docker"
            docker_script.write_text(f"""#!/bin/bash
echo "docker $*" >> "{mock_log}"
if [[ "$*" =~ "snapshots" ]]; then
    echo '[{{"short_id":"customsnap99","time":"2026-10-02T12:00:00Z"}}]'
    exit 0
fi
if [[ "$*" =~ "mktemp" ]]; then
    dir="/tmp/alt-restore-verify-ctr-custom-$RANDOM"
    echo "$dir"
    exit 0
fi
if [[ "$*" =~ "restore" && "$*" =~ "--target" ]]; then
    target=""
    args=("$@")
    for ((i=0; i<${{#args[@]}}; i++)); do
        if [[ "${{args[i]}}" == "--target" && $((i+1)) -lt ${{#args[@]}} ]]; then
            target="${{args[i+1]}}"
        fi
    done
    if [[ -n "$target" ]]; then
        rel="${{target#/}}"
        ctr_target="{container_root}/$rel"
        mkdir -p "$ctr_target/data"
        touch "$ctr_target/data/vol1"
        mkdir -p "$ctr_target/backups/postgres"
        echo "valid alt dump" > "$ctr_target/backups/postgres/alt-db-20261002.dump"
        echo "valid kratos dump" > "$ctr_target/backups/postgres/kratos-db-20261002.dump"
    fi
    exit 0
fi
if [[ "$1" == "cp" ]]; then
    src="$2"
    dest="$3"
    if [[ "$src" =~ ^[^:]+:(.*) ]]; then
        ctr_path="${{BASH_REMATCH[1]}}"
        ctr_path="${{ctr_path%/}}"
        ctr_path="${{ctr_path%/.}}"
        rel="${{ctr_path#/}}"
        real_src="{container_root}/$rel"
        mkdir -p "$dest"
        cp -r "$real_src/." "$dest/"
        exit 0
    fi
    exit 1
fi
if [[ "$*" =~ "pg_isready" ]]; then
    exit 0
fi
if [[ "$*" =~ "pg_restore" ]]; then
    exit 0
fi
if [[ "$*" =~ "information_schema.tables" ]]; then
    echo "10"
    exit 0
fi
exit 0
""")
            docker_script.chmod(0o755)

            jq_script = mock_bin / "jq"
            jq_script.write_text("""#!/bin/bash
if [[ "$1" == "length" ]]; then
    echo "1"
elif [[ "$1" =~ "short_id" ]]; then
    echo "customsnap99"
else
    echo "1"
fi
""")
            jq_script.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{mock_bin}:{env['PATH']}"
            env["BACKUP_ROOT"] = str(backup_root)
            env["LOG_FILE"] = str(backup_root / "restore.log")
            env["METRICS_DIR"] = str(backup_root / "metrics")
            env["RESTIC_REPOSITORY"] = str(backup_root / "restic-repo")
            env.pop("IN_CONTAINER", None)

            res = subprocess.run(
                ["bash", str(self.script_path), "--snapshot", "customsnap99", "--skip-cleanup"],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertEqual(res.returncode, 0, f"Restore verification should pass: {res.stdout}\n{res.stderr}")
            self.assertIn("Using snapshot: customsnap99", res.stdout)
            self.assertIn("Restore verification PASSED", res.stdout)

    def test_real_verification_invalid_restore_dir_fails_closed_no_live_fallback(self):
        """During real verification (not dry-run), an invalid or missing RESTORE_DIR must fail closed and never fall back to live dumps."""
        with tempfile.TemporaryDirectory() as tmpdir:
            backup_root = Path(tmpdir) / "backups"
            backup_root.mkdir()
            live_pg = backup_root / "postgres"
            live_pg.mkdir()
            # Live directory has valid dumps
            (live_pg / "alt-db-20261002.dump").write_text("live alt dump")
            (live_pg / "kratos-db-20261002.dump").write_text("live kratos dump")

            # Source functions from script without executing main
            bash_cmd = f"""
source <(grep -v '^main ' "{self.script_path}")
DRY_RUN=false
RESTORE_DIR="/nonexistent/restore/dir"
POSTGRES_BACKUP_DIR="{live_pg}"
verify_pg_restore
"""
            res = subprocess.run(
                ["bash", "-c", bash_cmd],
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertNotEqual(res.returncode, 0, "verify_pg_restore must fail closed when RESTORE_DIR is invalid in real mode")
            self.assertIn("RESTORE_DIR missing or invalid during real PostgreSQL verification", res.stdout + res.stderr)



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
                "POSTGRES_BACKUP_DIR": str(backup_dir / "postgres"),
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
echo "pg_dump PGPASSWORD=$PGPASSWORD $*" >> "{mock_log}"
echo "fake dump payload"
exit 0
""")
            (mock_bin / "pg_dump").chmod(0o755)

            # Mock psql
            (mock_bin / "psql").write_text(f"""#!/bin/bash
echo "psql PGPASSWORD=$PGPASSWORD $*" >> "{mock_log}"
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
                "POSTGRES_BACKUP_DIR": str(backup_dir / "postgres"),
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
            self.assertIn("pg_dump PGPASSWORD=secret_alt_postgres_pass -h alt-db -U alt_db_user -d alt", log_content)
            self.assertIn("pg_dump PGPASSWORD=secret_kratos_pass -h kratos-db -U kratos_user -d kratos", log_content)

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
                "POSTGRES_BACKUP_DIR": str(backup_dir / "postgres"),
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
                "POSTGRES_BACKUP_DIR": str(backup_dir / "postgres"),
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
                "POSTGRES_BACKUP_DIR": str(backup_dir / "postgres"),
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
                "POSTGRES_BACKUP_DIR": str(backup_dir / "postgres"),
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
                "POSTGRES_BACKUP_DIR": str(backup_dir / "postgres"),
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
                "POSTGRES_BACKUP_DIR": str(backup_dir / "postgres"),
                "ALT_DB_PASSWORD_FILE": str(secrets_dir / "postgres_password"),
                "KRATOS_DB_PASSWORD_FILE": str(secrets_dir / "kratos_db_password"),
            }

            res = subprocess.run(["bash", str(self.script_path)], env=env, capture_output=True, text=True, timeout=15)
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("Restic volume backup failed", res.stdout)
            self.assertIn("Backup FAILED", res.stdout)


if __name__ == "__main__":
    unittest.main(verbosity=2)
