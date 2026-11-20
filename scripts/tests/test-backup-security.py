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

from __future__ import annotations

import os
import re
import shutil
import subprocess
import tempfile
import unittest
import xml.etree.ElementTree as ET
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING, ClassVar

if TYPE_CHECKING:
    from collections.abc import Sequence

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
BASH_BIN = shutil.which("bash")
EXPECTED_REQUIRED_PG_DUMPS = 2


def _require_bash() -> str:
    assert BASH_BIN is not None, "bash executable not found on PATH"
    return BASH_BIN


@dataclass(frozen=True)
class MockDockerConfig:
    dump_dbs: str = "alt-db kratos-db recap-db rag-db pact-db"
    volume_id: str = "volsnap1"
    restore_rc: int = 0
    cp_rc: int = 0
    pg_restore_rc: int = 0
    table_count: int = 10


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
    script_path: Path,
    tmpdir: str,
    extra_args: Sequence[str],
    config: MockDockerConfig | None = None,
) -> tuple[subprocess.CompletedProcess[str], str]:
    cfg = config or MockDockerConfig()
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
        "@VOLUME_ID@": cfg.volume_id,
        "@DUMP_DBS@": cfg.dump_dbs,
        "@CONTAINER_ROOT@": str(container_root),
        "@RESTORE_RC@": str(cfg.restore_rc),
        "@CP_RC@": str(cfg.cp_rc),
        "@PG_RESTORE_RC@": str(cfg.pg_restore_rc),
        "@TABLE_COUNT@": str(cfg.table_count),
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
    bash_bin = _require_bash()
    res = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [bash_bin, str(script_path), *extra_args],
        env=env,
        capture_output=True,
        text=True,
        timeout=30,
        check=False,
    )
    return res, mock_log.read_text() if mock_log.exists() else ""


class TestBackupDockerfile(unittest.TestCase):
    """Verify docker/backup/Dockerfile specifications."""

    def setUp(self) -> None:
        self.dockerfile = REPO_ROOT / "docker/backup/Dockerfile"
        assert self.dockerfile.is_file(), "Dockerfile must exist"
        self.content = self.dockerfile.read_text()

    def test_no_docker_cli(self) -> None:
        """Dockerfile must NOT install or contain docker-cli."""
        assert "docker-cli" not in self.content, "docker-cli must be removed from restic-backup"

    def test_postgres_client_version_exact_18(self) -> None:
        """Dockerfile must specify base image or package with exact PostgreSQL 18 client."""
        has_pg18 = ("postgres:18" in self.content) or ("postgresql18-client" in self.content)
        assert has_pg18, "Dockerfile must use exact PostgreSQL 18 client base or package"

    def test_supercronic_pinned_and_verified(self) -> None:
        """Supercronic binary must be checksum verified."""
        assert "SUPERCRONIC_VERSION=" in self.content, self.content
        assert "sha256sum -c" in self.content, self.content


class TestBackupComposeConfig(unittest.TestCase):
    """Verify compose/backup.yaml daemonless architecture."""

    def setUp(self) -> None:
        self.compose_file = REPO_ROOT / "compose/backup.yaml"
        assert self.compose_file.is_file(), "compose/backup.yaml must exist"
        self.content = self.compose_file.read_text()

    def test_no_docker_socket_mount(self) -> None:
        """No Docker socket bind mount allowed."""
        assert "/var/run/docker.sock" not in self.content, self.content

    def test_no_docker_socket_proxy(self) -> None:
        """docker-socket-proxy service must be removed."""
        assert "docker-socket-proxy" not in self.content, self.content
        assert "backup-docker-proxy" not in self.content, self.content

    def test_no_docker_host_env(self) -> None:
        """DOCKER_HOST env var must not be present."""
        assert "DOCKER_HOST=" not in self.content, self.content

    def test_required_secrets_mounted(self) -> None:
        """Verify necessary secrets are mounted into restic-backup."""
        for sec in [
            "restic_password",
            "postgres_password",
            "db_password",
            "kratos_db_password",
            "clickhouse_password",
            "meili_master_key",
        ]:
            assert f"- {sec}" in self.content, f"Secret {sec} must be mounted in backup.yaml"


class TestClickHouseBackupDiskConfig(unittest.TestCase):
    """Verify clickhouse/config/backup_disk.xml format and paths."""

    def setUp(self) -> None:
        self.xml_file = REPO_ROOT / "clickhouse/config/backup_disk.xml"
        assert self.xml_file.is_file(), "backup_disk.xml must exist"

    def test_valid_xml_structure(self) -> None:
        """XML must be well-formed with matching tags."""
        tree = ET.parse(self.xml_file)  # noqa: S314 - parses trusted repo XML config in this test
        root = tree.getroot()
        assert root.tag == "clickhouse", root.tag

        disk_path = root.find("./storage_configuration/disks/backups/path")
        assert disk_path is not None, "Storage disk 'backups' path must be defined"
        assert (disk_path.text or "").strip() == "/backups/clickhouse/", disk_path.text

        allowed_disk = root.find("./backups/allowed_disk")
        assert allowed_disk is not None, "backups/allowed_disk must be defined"
        assert (allowed_disk.text or "").strip() == "backups", allowed_disk.text

        allowed_path = root.find("./backups/allowed_path")
        assert allowed_path is not None, "backups/allowed_path must be defined"
        assert (allowed_path.text or "").strip() == "/backups/clickhouse/", allowed_path.text


class TestRestoreVerifyScript(unittest.TestCase):
    """Verify restore-verify.sh container fail-fast guard and bash syntax."""

    def setUp(self) -> None:
        self.script_path = REPO_ROOT / "scripts/backup/restore-verify.sh"
        assert self.script_path.is_file(), self.script_path

    def test_bash_n_syntax(self) -> None:
        """restore-verify.sh must pass bash -n check."""
        bash_bin = _require_bash()
        res = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [bash_bin, "-n", str(self.script_path)], capture_output=True, text=True, check=False
        )
        assert res.returncode == 0, f"bash -n failed: {res.stderr}"

    def test_fail_fast_when_in_container(self) -> None:
        """restore-verify.sh must exit non-zero immediately when IN_CONTAINER is set."""
        bash_bin = _require_bash()
        env = os.environ.copy()
        env["IN_CONTAINER"] = "1"
        res = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [bash_bin, str(self.script_path), "--dry-run"],
            env=env,
            capture_output=True,
            text=True,
            timeout=15,
            check=False,
        )
        assert res.returncode != 0, res.stderr
        assert "must be run on the host" in res.stderr, res.stderr

    def test_host_flow_mock_docker(self) -> None:
        """Dry-run on the host plans the volume check and per-database dump fetches without restoring anything."""
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--dry-run", "--skip-cleanup"])
            assert res.returncode == 0, f"{res.stderr}\n{res.stdout}"
            assert "Restore verification PASSED" in res.stdout, res.stdout
            assert "pg_restore" not in log, log

    def test_volume_snapshot_selected_by_scheduled_tag(self) -> None:
        """The /data check uses the latest snapshot carrying the volume backup tag, not the newest snapshot of any kind."""
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"])
            assert res.returncode == 0, f"{res.stdout}\n{res.stderr}"
            assert "snapshots --tag scheduled --json" in log, log
            assert "Using snapshot: volsnap1" in res.stdout, res.stdout
            assert "restore volsnap1" in log, log

    def test_dumps_come_from_latest_good_pg_snapshot_per_database(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"])
            assert res.returncode == 0, f"{res.stdout}\n{res.stderr}"
            for db in ["alt-db", "kratos-db", "recap-db", "rag-db", "pact-db"]:
                assert f"dump latest /postgres/{db}.dump --tag pg-{db}" in log, log
                assert f"pg_restore stdin: pgdump:{db}" in log, log
                assert f"PostgreSQL restore verification passed for {db}" in res.stdout, res.stdout

    def test_optional_database_without_dump_snapshot_is_skipped(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(
                self.script_path,
                tmpdir,
                ["--skip-cleanup"],
                config=MockDockerConfig(dump_dbs="alt-db kratos-db"),
            )
            assert res.returncode == 0, f"{res.stdout}\n{res.stderr}"
            assert "PostgreSQL restore verification passed for alt-db" in res.stdout, res.stdout
            assert "pg_restore stdin: pgdump:rag-db" not in log, log

    def test_required_database_without_dump_snapshot_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, _log = run_restore_verify(
                self.script_path,
                tmpdir,
                ["--skip-cleanup"],
                config=MockDockerConfig(dump_dbs="alt-db"),
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Restore verification FAILED" in res.stdout + res.stderr, res.stdout + res.stderr

    def test_explicit_snapshot_id_selects_the_volume_snapshot_only(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(
                self.script_path,
                tmpdir,
                ["--snapshot", "customsnap99", "--skip-cleanup"],
                config=MockDockerConfig(volume_id="customsnap99"),
            )
            assert res.returncode == 0, f"{res.stdout}\n{res.stderr}"
            assert "Using snapshot: customsnap99" in res.stdout, res.stdout
            assert "restore customsnap99" in log, log
            assert "dump latest /postgres/alt-db.dump --tag pg-alt-db" in log, log

    def test_explicit_snapshot_id_outside_volume_snapshots_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(
                self.script_path, tmpdir, ["--snapshot", "pgsnap1", "--skip-cleanup"]
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "restore pgsnap1" not in log, log

    def test_no_volume_snapshot_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, _log = run_restore_verify(
                self.script_path,
                tmpdir,
                ["--skip-cleanup"],
                config=MockDockerConfig(volume_id=""),
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Restore verification FAILED" in res.stdout + res.stderr, res.stdout + res.stderr

    def test_pg_restore_failure_fails_closed(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, _log = run_restore_verify(
                self.script_path,
                tmpdir,
                ["--skip-cleanup"],
                config=MockDockerConfig(pg_restore_rc=1),
            )
            assert res.returncode != 0, "Restore must FAIL when pg_restore fails"
            assert "Restore verification FAILED" in res.stdout + res.stderr, res.stdout + res.stderr
            assert "PostgreSQL dump format valid" not in res.stdout, res.stdout

    def test_restore_requires_table_count_greater_than_zero(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, _log = run_restore_verify(
                self.script_path,
                tmpdir,
                ["--skip-cleanup"],
                config=MockDockerConfig(table_count=0),
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "zero public tables restored" in res.stdout + res.stderr, res.stdout + res.stderr

    def test_failed_volume_restore_skips_pg_verify(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(
                self.script_path,
                tmpdir,
                ["--skip-cleanup"],
                config=MockDockerConfig(restore_rc=1),
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Restore verification FAILED" in res.stdout + res.stderr, res.stdout + res.stderr
            assert "pg_restore" not in log, log

    def test_failed_docker_cp_skips_pg_verify(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(
                self.script_path,
                tmpdir,
                ["--skip-cleanup"],
                config=MockDockerConfig(cp_rc=1),
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Restore verification FAILED" in res.stdout + res.stderr, res.stdout + res.stderr
            assert "pg_restore" not in log, log

    def test_container_namespace_docker_cp(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"])
            assert res.returncode == 0, f"{res.stdout}\n{res.stderr}"
            assert "docker cp alt-backup:" in log, log

    def test_restore_uses_matching_pg18_images(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log = run_restore_verify(self.script_path, tmpdir, ["--skip-cleanup"])
            assert res.returncode == 0, f"{res.stdout}\n{res.stderr}"
            assert "postgres:17-alpine" not in log, log
            assert "postgres:18.6-alpine" in log, log
            assert "postgres:18.6-bookworm" not in log, log
            assert "alt-recap-db" in log, log
            assert "shared_preload_libraries=pg_cron" in log, log
            assert "cron.database_name=verify_db" in log, log
            assert "cron.launch_active_jobs=off" in log, log
            assert "/var/lib/postgresql/data" not in log, log
            assert ":/var/lib/postgresql" in log, log
            assert "--no-owner" in log, log
            assert "--no-privileges" in log, log

    def test_restore_includes_pact_db(self) -> None:
        content = self.script_path.read_text()
        assert "pact-db" in content, content

    def test_fetched_dumps_live_in_private_temp_dir(self) -> None:
        content = self.script_path.read_text()
        assert "umask 077" in content, content
        assert 'mktemp -d "/tmp/${VERIFY_PREFIX}-dumps' in content, content
        assert 'dump latest "/postgres/${db}.dump" --tag "pg-${db}"' in content, content


class TestBackupAllScriptBehavior(unittest.TestCase):
    """Verify backup-all.sh offline behavior, argv contracts, role credentials, and error handling."""

    def setUp(self) -> None:
        self.script_path = REPO_ROOT / "scripts/backup/backup-all.sh"
        assert self.script_path.is_file(), self.script_path

    def test_bash_n_syntax(self) -> None:
        """backup-all.sh must pass bash -n check."""
        bash_bin = _require_bash()
        res = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [bash_bin, "-n", str(self.script_path)],
            capture_output=True,
            text=True,
            timeout=15,
            check=False,
        )
        assert res.returncode == 0, f"bash -n failed: {res.stderr}"

    def test_in_container_enforcement(self) -> None:
        """backup-all.sh must refuse to run if IN_CONTAINER is not set."""
        bash_bin = _require_bash()
        env = os.environ.copy()
        env.pop("IN_CONTAINER", None)
        res = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [bash_bin, str(self.script_path)],
            env=env,
            capture_output=True,
            text=True,
            timeout=15,
            check=False,
        )
        assert res.returncode != 0, res.stdout + res.stderr
        assert "must run IN the backup container" in res.stdout + res.stderr, (
            res.stdout + res.stderr
        )

    def test_no_checkpoint_crash_consistency_false_claim(self) -> None:
        """backup-all.sh must not claim CHECKPOINT provides crash-consistency or PITR for raw volume copies."""
        content = self.script_path.read_text()
        assert "providing crash-consistency" not in content, (
            "False crash-consistency claim must be removed"
        )
        assert "auxiliary" in content, (
            "Must explain that raw volume copies are auxiliary/best-effort"
        )

    def test_backup_all_configurable_backup_root_and_log_file_no_mkdir_mock(self) -> None:
        """backup-all.sh must support configurable BACKUP_ROOT, LOG_FILE, and METRICS_DIR without mkdir mocks."""
        bash_bin = _require_bash()
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
            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [bash_bin, str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
                check=False,
            )
            assert res.returncode == 0, (
                f"backup-all.sh must succeed with real mkdir and custom BACKUP_ROOT/LOG_FILE: {res.stdout}\n{res.stderr}"
            )
            assert custom_log.is_file(), f"LOG_FILE {custom_log} must be created"
            log_text = custom_log.read_text()
            assert "Backup completed successfully" in log_text, log_text

    def test_successful_backup_flow_and_argv_contracts(self) -> None:
        """Test full backup run with mock commands, verifying actual argv, roles, and HTTP POST queries."""
        bash_bin = _require_bash()
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

            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [bash_bin, str(self.script_path), "--prune", "--verify"],
                env=env,
                capture_output=True,
                text=True,
                timeout=30,
                check=False,
            )
            assert res.returncode == 0, (
                f"backup-all.sh failed:\nSTDOUT:\n{res.stdout}\nSTDERR:\n{res.stderr}"
            )
            assert "Backup completed successfully" in res.stdout, res.stdout

            log_content = mock_log.read_text()

            # Verify PostgreSQL argv and exact credentials pairing
            assert (
                "pg_dump PGPASS=alt-db:*:alt:alt_db_user:secret_alt_postgres_pass -h alt-db -U alt_db_user -d alt"
                in log_content
            ), log_content
            assert (
                "pg_dump PGPASS=kratos-db:*:kratos:kratos_user:secret_kratos_pass -h kratos-db -U kratos_user -d kratos"
                in log_content
            ), log_content

            # Verify ClickHouse HTTP POST native BACKUP with DB=rask_logs
            assert "BACKUP DATABASE rask_logs TO Disk('backups'" in log_content, log_content
            assert "-X POST" in log_content, log_content

            # Verify Meilisearch snapshot trigger and task poll
            assert "http://meilisearch:7700/snapshots" in log_content, log_content
            assert "http://meilisearch:7700/tasks/42" in log_content, log_content

            # Verify restic operations
            assert "backup --tag scheduled" in log_content, log_content
            assert "forget --keep-hourly 24" in log_content, log_content
            assert "check" in log_content, log_content

    def test_missing_credentials_fails_closed(self) -> None:
        """Missing credentials for detected database must cause backup failure (fail-closed)."""
        bash_bin = _require_bash()
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

            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [bash_bin, str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
                check=False,
            )
            assert res.returncode != 0, (
                "Missing credentials file must fail closed with non-zero exit"
            )
            assert "Missing credentials file" in res.stdout + res.stderr, res.stdout + res.stderr
            assert "Backup FAILED" in res.stdout + res.stderr, res.stdout + res.stderr

    def test_required_dns_failure_fails_closed(self) -> None:
        """Required DB (alt-db) DNS failure must fail whole backup non-zero."""
        bash_bin = _require_bash()
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

            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [bash_bin, str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
                check=False,
            )
            assert res.returncode != 0, "Required DB DNS failure must fail whole backup"
            assert "Required database service alt-db" in res.stdout + res.stderr, (
                res.stdout + res.stderr
            )
            assert "Backup FAILED" in res.stdout + res.stderr, res.stdout + res.stderr

    def test_optional_service_skip_without_daemon(self) -> None:
        """Optional service not resolvable in DNS is safely skipped without error when required services pass."""
        bash_bin = _require_bash()
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

            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [bash_bin, str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
                check=False,
            )
            assert res.returncode == 0, (
                f"Optional services absence should not fail backup: {res.stdout}"
            )
            assert (
                "Optional database service recap-db (recap-db) not found in DNS; skipping"
                in res.stdout
            ), res.stdout
            assert "Backup completed successfully" in res.stdout, res.stdout

    def test_clickhouse_error_fails_closed(self) -> None:
        """ClickHouse backup failure or missing credentials when detected must fail whole backup."""
        bash_bin = _require_bash()
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

            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [bash_bin, str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
                check=False,
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Missing credentials file" in res.stdout, res.stdout
            assert "Backup FAILED" in res.stdout, res.stdout

    def test_meilisearch_task_failure_fails_closed(self) -> None:
        """Meilisearch snapshot failure during polling must fail whole backup."""
        bash_bin = _require_bash()
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

            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [bash_bin, str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
                check=False,
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Meilisearch snapshot task failed" in res.stdout, res.stdout
            assert "Backup FAILED" in res.stdout, res.stdout

    def test_restic_failure_propagates_non_zero(self) -> None:
        """Restic backup failure must fail whole backup."""
        bash_bin = _require_bash()
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

            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [bash_bin, str(self.script_path)],
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
                check=False,
            )
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Restic volume backup failed" in res.stdout, res.stdout
            assert "Backup FAILED" in res.stdout, res.stdout


class TestBackupAllNoPlaintextNoArgvSecrets(unittest.TestCase):
    """Credentials stay off command lines and dumps never reach disk unencrypted."""

    FIXTURE_FILES: ClassVar[dict[str, str]] = {
        "postgres_password": "secret_alt_postgres_pass",
        "kratos_db_password": "secret_kratos_pass",
        "clickhouse_password": "secret_ch_pass",
        "meili_master_key": "secret_meili_key",
        "restic_password": "secret_restic_pass",
    }

    def setUp(self) -> None:
        self.script_path = REPO_ROOT / "scripts/backup/backup-all.sh"

    def _run(
        self,
        tmpdir: str,
        extra_args: Sequence[str] = (),
        *,
        restic_script: str | None = None,
        drop_restic_password: bool = False,
        pg_dump_rc: int = 0,
    ) -> tuple[subprocess.CompletedProcess[str], str, Path]:
        bash_bin = _require_bash()
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
        res = subprocess.run(  # noqa: S603 - fixed argv built in this test
            [bash_bin, str(self.script_path), *extra_args],
            env=env,
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )
        log = mock_log.read_text() if mock_log.exists() else ""
        return res, log, backup_dir

    def test_no_secret_value_on_any_command_line(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, _ = self._run(tmpdir)
            assert res.returncode == 0, res.stdout + res.stderr
            for line in log.splitlines():
                if line.startswith("curl cfg"):
                    continue
                for value in self.FIXTURE_FILES.values():
                    assert value not in line, f"secret leaked on command line: {line}"
            assert "env_has_PGPASSWORD=yes" not in log, log

    def test_curl_credentials_come_from_private_config_file(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, _ = self._run(tmpdir)
            assert res.returncode == 0, res.stdout + res.stderr
            cfg_lines = [line for line in log.splitlines() if line.startswith("curl cfg")]
            assert any("secret_ch_pass" in line for line in cfg_lines), cfg_lines
            assert any("secret_meili_key" in line for line in cfg_lines), cfg_lines
            for line in cfg_lines:
                assert "mode=600" in line, line

    def test_pg_dump_uses_private_passfile(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, _ = self._run(tmpdir)
            assert res.returncode == 0, res.stdout + res.stderr
            dump_lines = [line for line in log.splitlines() if line.startswith("pg_dump")]
            assert len(dump_lines) == EXPECTED_REQUIRED_PG_DUMPS, dump_lines
            for line in dump_lines:
                assert "mode=600" in line, line

    def test_dumps_stream_into_restic_and_never_touch_disk(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, backup_dir = self._run(tmpdir, ("--pg-only",))
            assert res.returncode == 0, res.stdout + res.stderr
            dumped = list(backup_dir.rglob("*.dump"))
            assert dumped == [], dumped
            stdin_lines = [
                line for line in log.splitlines() if line.startswith("restic") and "--stdin" in line
            ]
            assert len(stdin_lines) == EXPECTED_REQUIRED_PG_DUMPS, stdin_lines
            for line in stdin_lines:
                assert "--tag pg-" not in line, "selectable tags must not be present at backup time"
            assert any("--stdin-filename postgres/alt-db.dump" in line for line in stdin_lines), (
                stdin_lines
            )
            tag_lines = [
                line for line in log.splitlines() if line.startswith("restic") and " tag " in line
            ]
            assert any(
                "--add pg-dump" in line and "--add pg-alt-db" in line for line in tag_lines
            ), tag_lines
            assert any(
                "--stdin-filename postgres/kratos-db.dump" in line for line in stdin_lines
            ), stdin_lines

    def test_dry_run_creates_no_dump_paths(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, _log, backup_dir = self._run(tmpdir, ("--dry-run",))
            assert res.returncode == 0, res.stdout + res.stderr
            dumped = list(backup_dir.rglob("*.dump"))
            assert dumped == [], dumped
            assert not (backup_dir / "postgres").exists(), backup_dir

    def test_missing_restic_password_is_an_error(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            res, log, backup_dir = self._run(tmpdir, drop_restic_password=True)
            assert res.returncode != 0, res.stdout + res.stderr
            assert "restic password file" in (res.stdout + res.stderr).lower(), (
                res.stdout + res.stderr
            )
            assert "pg_dump" not in log, log
            dumped = list(backup_dir.rglob("*.dump"))
            assert dumped == [], dumped

    def test_failed_dump_stream_fails_backup(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            restic = f"""#!/bin/bash
echo "restic $*" >> "{Path(tmpdir) / "commands.log"}"
if [[ "$*" =~ "--stdin" ]]; then cat > /dev/null; exit 1; fi
exit 0
"""
            res, _log, _ = self._run(tmpdir, ("--pg-only",), restic_script=restic)
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Backup FAILED" in res.stdout, res.stdout

    def test_pg_dump_failure_forgets_snapshot_by_id_and_leaves_no_selectable_tag(self) -> None:
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
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Backup FAILED" in res.stdout, res.stdout
            assert "forget deadbeef01" in log, log
            added_tags = [
                line
                for line in log.splitlines()
                if line.startswith("restic") and " tag " in line and "--add" in line
            ]
            assert not added_tags, added_tags
            for line in log.splitlines():
                if line.startswith("restic") and "--stdin" in line:
                    assert "pg-alt-db" not in line, line

    def test_failed_snapshot_cleanup_is_reported_as_error(self) -> None:
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
            res, _log, _ = self._run(tmpdir, ("--pg-only",), restic_script=restic, pg_dump_rc=1)
            assert res.returncode != 0, res.stdout + res.stderr
            assert "Failed to remove incomplete dump snapshot deadbeef01" in res.stdout, res.stdout

    def test_scripts_have_no_inline_credentials(self) -> None:
        for name in ("backup-all.sh", "sync-offsite.sh", "restore-verify.sh", "backup-metrics.sh"):
            content = (REPO_ROOT / "scripts/backup" / name).read_text()
            assert "PGPASSWORD" not in content, name
            assert not re.search(r"Bearer \$\{?[A-Za-z_]", content), name
            assert '-u "${' not in content, name


class TestRestoreVerifyStreamedDumps(unittest.TestCase):
    def test_restore_verify_does_not_search_restored_tree_for_dumps(self) -> None:
        content = (REPO_ROOT / "scripts/backup/restore-verify.sh").read_text()
        assert '-name "${db}' not in content, content

    def test_metrics_read_dump_status_from_snapshots_not_plaintext_dir(self) -> None:
        content = (REPO_ROOT / "scripts/backup/backup-metrics.sh").read_text()
        assert "POSTGRES_BACKUP_DIR" not in content, content
        assert "pg-${db}" in content, content


if __name__ == "__main__":
    unittest.main(verbosity=2)
