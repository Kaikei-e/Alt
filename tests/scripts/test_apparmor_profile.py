#!/usr/bin/env python3
"""RED/GREEN TDD suite for Alt's custom PostgreSQL Docker AppArmor remediation.

This test module is 100% Python standard-library (`unittest`, `subprocess`,
`tempfile`, `pathlib`, `hashlib`, `os`, `re`, `stat`) and runs completely
offline without `sudo`, a live Docker daemon, AppArmor kernel access, or
external Python packages.

Expected Target Artifacts & Interfaces
======================================
1. `deploy/apparmor/alt-docker-postgres`
   - Custom named AppArmor profile `alt-docker-postgres` derived from Moby's
     `profiles/apparmor/template.go` (`baseTemplate`).
   - Preserves all 15 Docker `deny` rules (`@{PROC}` and `/sys` hardening +
     `deny mount,`) and base rules (`network,`, `capability,`, `file,`,
     `umount,`).
   - Minimally extends `signal` rules to allow intra-container signaling across
     literal and Ubuntu 26.04 stacked peers:
       * `signal (receive) peer=unconfined,`
       * `signal (receive) peer=dockerd,`
       * `signal (receive) peer=/usr/bin/dockerd,`
       * `signal (send,receive) peer=alt-docker-postgres,`
       * `signal (send,receive) peer=alt-docker-postgres//&unconfined,`
       * `signal (send,receive) peer=unconfined//&alt-docker-postgres,`
   - Forbids broad signal wildcards (`peer=*`, `peer=**`, `peer=@{*}`, bare
     `signal,`) and forbids unrelated `ptrace` expansion beyond Docker's
     template rule (`ptrace (trace,read,tracedby,readby) peer=docker-default,`).

2. `scripts/probe-apparmor-postgres.sh`
   - Environment interface:
       * `DOCKER_BIN` (default: `docker`)
       * `TIMEOUT_BIN` (default: `timeout`)
       * `APPARMOR_PROFILE_NAME` (default: `alt-docker-postgres`)
       * `PROBE_IMAGE` (default: `postgres:17.11-alpine`)
       * `PROBE_TIMEOUT_SECONDS` (default: `15`)
       * `APPARMOR_ATTR_PATH` (default inside container:
         `/proc/self/attr/apparmor/current`, fallback `/proc/self/attr/current`)
   - Behavior:
       * Runs a bounded-timeout ephemeral container with
         `--security-opt apparmor=${APPARMOR_PROFILE_NAME}`.
       * Verifies the container's runtime AppArmor label is
         `${APPARMOR_PROFILE_NAME}`, `${APPARMOR_PROFILE_NAME}//&unconfined`, or
         `unconfined//&${APPARMOR_PROFILE_NAME}` (rejecting `unconfined` and
         `docker-default*`).
       * Spawns a distinct child process that installs a `USR1` trap, signals
         readiness, receives `kill -USR1` from the parent process without
         `|| true` error masking, records receipt, and exits 0 within a bounded
         timeout.

3. `scripts/check-apparmor-profile.sh`
   - Flags:
       * `--static-only`: validate source & installed host profile existence,
         readability, exact SHA-256 parity, and `apparmor_parser --preprocess`
         syntax validation without calling the Docker runtime probe.
       * `--probe` (default when no flag is given, or explicit `--probe`): run
         all static checks and then execute `PROBE_SCRIPT`.
   - Environment interface:
       * `APPARMOR_SOURCE_PROFILE` (default:
         `<repo>/deploy/apparmor/alt-docker-postgres`)
       * `APPARMOR_HOST_PROFILE` (default:
         `/etc/apparmor.d/alt-docker-postgres`)
       * `APPARMOR_PROFILE_NAME` (default: `alt-docker-postgres`)
       * `APPARMOR_PARSER_BIN` (default: `apparmor_parser`)
       * `PROBE_SCRIPT` (default: `<repo>/scripts/probe-apparmor-postgres.sh`)

4. `scripts/install-apparmor-profile.sh`
   - Flags:
       * `--no-probe`: pass `--static-only` to `CHECK_SCRIPT` after loading.
   - Environment interface:
       * `APPARMOR_SOURCE_PROFILE` (default:
         `<repo>/deploy/apparmor/alt-docker-postgres`)
       * `APPARMOR_HOST_PROFILE` (default:
         `/etc/apparmor.d/alt-docker-postgres`)
       * `APPARMOR_PARSER_BIN` (default: `apparmor_parser`)
       * `INSTALL_BIN` (default: `install`)
       * `CHECK_SCRIPT` (default: `<repo>/scripts/check-apparmor-profile.sh`)
       * `SKIP_RUNTIME_PROBE` (default: `0`)
   - Behavior:
       * Validates syntax via `"$APPARMOR_PARSER_BIN" --preprocess
         "$APPARMOR_SOURCE_PROFILE"` BEFORE writing `APPARMOR_HOST_PROFILE`.
       * Installs `APPARMOR_SOURCE_PROFILE` to `APPARMOR_HOST_PROFILE` (mode
         `0644`).
       * Always executes `"$APPARMOR_PARSER_BIN" -r -W "$APPARMOR_HOST_PROFILE"`
         even when the file on disk is unchanged, restoring lost kernel state.
       * Fails closed on any parser/load error and runs `CHECK_SCRIPT` before
         reporting success.

5. Base `compose/*.yaml` files & compatibility overrides (`compose/compose.apparmor.yaml`, `compose/compose.staging.apparmor.yaml`)
   - Production Compose files (`db.yaml`, `auth.yaml`, `acolyte.yaml`, `pact.yaml`,
     `rag.yaml`, `recap.yaml`, `sovereign.yaml`) select
     `apparmor:alt-docker-postgres` by default with no `apparmor:unconfined`
     fallback.
   - `compose/compose.staging.yaml` omits the host-specific `apparmor:alt-docker-postgres`
     profile (and forbids `apparmor:unconfined`) so portable CI runners without
     `/etc/apparmor.d/alt-docker-postgres` use Docker's built-in `docker-default`
     confinement without failing container init, while
     `compose/compose.staging.apparmor.yaml` provides the explicit `!override`
     for Ubuntu 26.04 hosts.
   - Compatibility Compose override files using `!override` on `security_opt`
     select `apparmor:alt-docker-postgres` for the 8 production PostgreSQL
     services and 6 staging PostgreSQL services respectively.

6. `docs/runbooks/postgres-apparmor.md`
   - Operator runbook documenting root cause, provisioning, validation, opt-in
     Compose override commands, and rollback.

Run offline with:
    python3 -m unittest discover -s tests/scripts -p 'test_apparmor_profile.py' -v
"""

from __future__ import annotations

import hashlib
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
PROFILE_PATH = REPO_ROOT / "deploy" / "apparmor" / "alt-docker-postgres"
INSTALL_SCRIPT = REPO_ROOT / "scripts" / "install-apparmor-profile.sh"
CHECK_SCRIPT = REPO_ROOT / "scripts" / "check-apparmor-profile.sh"
PROBE_SCRIPT = REPO_ROOT / "scripts" / "probe-apparmor-postgres.sh"
PROD_OVERRIDE_PATH = REPO_ROOT / "compose" / "compose.apparmor.yaml"
STAGING_OVERRIDE_PATH = REPO_ROOT / "compose" / "compose.staging.apparmor.yaml"
RUNBOOK_PATH = REPO_ROOT / "docs" / "runbooks" / "postgres-apparmor.md"

EXPECTED_PROD_DB_SERVICES = {
    "db",
    "pre-processor-db",
    "kratos-db",
    "acolyte-db",
    "pact-db",
    "rag-db",
    "recap-db",
    "knowledge-sovereign-db",
}

EXPECTED_STAGING_DB_SERVICES = {
    "knowledge-sovereign-db",
    "rag-db",
    "recap-db",
    "acolyte-db",
    "alt-backend-db",
    "auth-hub-db",
}

EXPECTED_DOCKER_DENY_RULES = [
    "deny @{PROC}/* w,",
    "deny @{PROC}/{[^1-9],[^1-9][^0-9],[^1-9s][^0-9y][^0-9s],[^1-9][^0-9][^0-9][^0-9]*}/** w,",
    "deny @{PROC}/sys/[^k]** w,",
    "deny @{PROC}/sys/kernel/{?,??,[^s][^h][^m]**} w,",
    "deny @{PROC}/sysrq-trigger rwklx,",
    "deny @{PROC}/kcore rwklx,",
    "deny mount,",
    "deny /sys/[^f]*/** wklx,",
    "deny /sys/f[^s]*/** wklx,",
    "deny /sys/fs/[^c]*/** wklx,",
    "deny /sys/fs/c[^g]*/** wklx,",
    "deny /sys/fs/cg[^r]*/** wklx,",
    "deny /sys/firmware/** rwklx,",
    "deny /sys/devices/virtual/powercap/** rwklx,",
    "deny /sys/kernel/security/** rwklx,",
]


def _write_executable(path: Path, content: str) -> Path:
    path.write_text(content, encoding="utf-8")
    path.chmod(path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return path


def _parse_override_services(yaml_text: str) -> tuple[dict[str, list[str]], dict[str, bool]]:
    """Dependency-free parser for Compose security_opt override files."""
    services: dict[str, list[str]] = {}
    has_override_tag: dict[str, bool] = {}
    in_services = False
    current_svc: str | None = None
    in_sec_opt = False

    for raw_line in yaml_text.splitlines():
        line = raw_line.split("#", 1)[0].rstrip()
        if not line.strip():
            continue
        indent = len(line) - len(line.lstrip(" "))
        stripped = line.strip()

        if indent == 0:
            in_services = stripped == "services:"
            current_svc = None
            in_sec_opt = False
            continue

        if not in_services:
            continue

        if indent == 2 and stripped.endswith(":"):
            current_svc = stripped[:-1].strip()
            services[current_svc] = []
            has_override_tag[current_svc] = False
            in_sec_opt = False
            continue

        if current_svc is None:
            continue

        if indent == 4 and stripped.startswith("security_opt:"):
            in_sec_opt = True
            rest = stripped[len("security_opt:") :].strip()
            if "!override" in rest:
                has_override_tag[current_svc] = True
                rest = rest.replace("!override", "").strip()
            if rest.startswith("[") and rest.endswith("]"):
                inner = rest[1:-1].strip()
                if inner:
                    for item in inner.split(","):
                        val = item.strip().strip("\"'")
                        if val:
                            services[current_svc].append(val)
            continue

        if indent == 4 and not stripped.startswith("-"):
            in_sec_opt = False
            continue

        if in_sec_opt and stripped.startswith("-"):
            val = stripped[1:].strip().strip("\"'")
            services[current_svc].append(val)

    return services, has_override_tag


class TestAppArmorProfileStaticContent(unittest.TestCase):
    """Verify `deploy/apparmor/alt-docker-postgres` rules and minimal privilege scope."""

    def test_profile_exists_and_declares_named_profile(self) -> None:
        self.assertTrue(
            PROFILE_PATH.is_file(),
            f"Missing AppArmor profile file: {PROFILE_PATH}",
        )
        content = PROFILE_PATH.read_text(encoding="utf-8")
        self.assertIn("#include <tunables/global>", content)
        self.assertIn("#include <abstractions/base>", content)
        self.assertRegex(
            content,
            r"profile\s+alt-docker-postgres\s+flags=\(attach_disconnected,\s*mediate_deleted\)\s*\{",
        )
        for base_grant in ("network,", "capability,", "file,", "umount,"):
            self.assertIn(base_grant, content, f"Missing base grant {base_grant}")

    def test_profile_preserves_all_docker_default_deny_rules(self) -> None:
        self.assertTrue(PROFILE_PATH.is_file(), f"Missing {PROFILE_PATH}")
        content = PROFILE_PATH.read_text(encoding="utf-8")
        normalized_lines = {
            line.split("#", 1)[0].strip()
            for line in content.splitlines()
            if line.split("#", 1)[0].strip()
        }
        for deny_rule in EXPECTED_DOCKER_DENY_RULES:
            self.assertIn(
                deny_rule,
                normalized_lines,
                f"Docker default hardening rule missing from profile: {deny_rule}",
            )

    def test_profile_minimally_extends_signal_peers_without_wildcards_or_ptrace_creep(
        self,
    ) -> None:
        self.assertTrue(PROFILE_PATH.is_file(), f"Missing {PROFILE_PATH}")
        content = PROFILE_PATH.read_text(encoding="utf-8")
        active_lines = [
            line.split("#", 1)[0].strip()
            for line in content.splitlines()
            if line.split("#", 1)[0].strip()
        ]

        signal_lines = [line for line in active_lines if line.startswith("signal")]
        expected_signals = {
            "signal (receive) peer=unconfined,",
            "signal (receive) peer=runc,",
            "signal (receive) peer=dockerd,",
            "signal (receive) peer=/usr/bin/dockerd,",
            "signal (send,receive) peer=alt-docker-postgres,",
            "signal (send,receive) peer=alt-docker-postgres//&unconfined,",
            "signal (send,receive) peer=unconfined//&alt-docker-postgres,",
        }
        self.assertEqual(
            set(signal_lines),
            expected_signals,
            f"Signal rules must match exact minimal peer set. Got: {signal_lines}",
        )

        for line in signal_lines:
            self.assertNotIn("peer=*", line, "Wildcard signal peer is forbidden")
            self.assertNotIn("peer=@{", line, "Variable wildcard signal peer is forbidden")

        ptrace_lines = [line for line in active_lines if line.startswith("ptrace")]
        self.assertEqual(
            ptrace_lines,
            ["ptrace (trace,read,tracedby,readby) peer=alt-docker-postgres,"],
            f"ptrace must be scoped only to alt-docker-postgres. Got: {ptrace_lines}",
        )


class TestCurrentDockerProfileCompatibility(unittest.TestCase):
    def test_abi_and_socket_family_hardening(self) -> None:
        policy = PROFILE_PATH.read_text(encoding="utf-8")
        for rule in ("abi <abi/3.0>,", "deny network alg,", "deny network vsock,"):
            with self.subTest(rule=rule):
                self.assertIn(rule, policy)


class TestAppArmorRuntimeProbeScript(unittest.TestCase):
    """Verify `scripts/probe-apparmor-postgres.sh` signal delivery and label checks."""

    def setUp(self) -> None:
        self.tmpdir_obj = tempfile.TemporaryDirectory()
        self.tmpdir = Path(self.tmpdir_obj.name)
        self.bin_dir = self.tmpdir / "bin"
        self.bin_dir.mkdir()
        self.log_file = self.tmpdir / "stub.log"
        self.attr_file = self.tmpdir / "apparmor_current"
        self.attr_file.write_text(
            "alt-docker-postgres//&unconfined (enforce)\n", encoding="utf-8"
        )

        # Stub `timeout` that records invocation and execs the wrapped command
        _write_executable(
            self.bin_dir / "timeout",
            "#!/usr/bin/env bash\n"
            f'echo "timeout $*" >> "{self.log_file}"\n'
            'shift\n'
            'exec "$@"\n',
        )

    def tearDown(self) -> None:
        self.tmpdir_obj.cleanup()

    def _make_executing_docker_stub(self, extra_shell_prefix: str = "") -> Path:
        """Create a stub `docker` that validates flags and executes the probe's `sh -c` script."""
        return _write_executable(
            self.bin_dir / "docker",
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            f'echo "docker $*" >> "{self.log_file}"\n'
            'if [[ "${1:-}" == "rm" ]]; then\n'
            "  exit 0\n"
            "fi\n"
            'if [[ "${1:-}" != "run" ]]; then\n'
            '  echo "expected docker run, got: $*" >&2\n'
            "  exit 2\n"
            "fi\n"
            'while [[ $# -gt 0 ]]; do\n'
            '  if [[ "$1" == "-c" ]]; then\n'
            "    shift\n"
            '    script_body="$1"\n'
            "    shift\n"
            '    if [[ "${1:-}" == "--" ]]; then shift; fi\n'
            f'    export APPARMOR_ATTR_PATH="{self.attr_file}"\n'
            f"    {extra_shell_prefix}\n"
            '    exec sh -c "$script_body" -- "$@"\n'
            "  fi\n"
            "  shift\n"
            "done\n"
            'echo "missing -c in docker run invocation" >&2\n'
            "exit 2\n",
        )

    def _base_env(self) -> dict[str, str]:
        return {
            **os.environ,
            "PATH": f"{self.bin_dir}:{os.environ.get('PATH', '')}",
            "DOCKER_BIN": str(self.bin_dir / "docker"),
            "TIMEOUT_BIN": str(self.bin_dir / "timeout"),
            "APPARMOR_PROFILE_NAME": "alt-docker-postgres",
            "PROBE_IMAGE": "postgres:17.11-alpine",
            "PROBE_TIMEOUT_SECONDS": "10",
            "APPARMOR_ATTR_PATH": str(self.attr_file),
        }

    def test_probe_script_has_no_or_true_masking(self) -> None:
        self.assertTrue(
            PROBE_SCRIPT.is_file(), f"Missing runtime probe script: {PROBE_SCRIPT}"
        )
        content = PROBE_SCRIPT.read_text(encoding="utf-8")
        for line in content.splitlines():
            code = line.split("#", 1)[0]
            self.assertNotIn(
                "|| true",
                code,
                f"Runtime probe must not mask errors with '|| true': {line}",
            )
            self.assertNotIn(
                "|| :",
                code,
                f"Runtime probe must not mask errors with '|| :': {line}",
            )

    def test_probe_succeeds_with_stacked_label_and_distinct_process_signal_receipt(
        self,
    ) -> None:
        self.assertTrue(PROBE_SCRIPT.is_file(), f"Missing {PROBE_SCRIPT}")
        self._make_executing_docker_stub()
        proc = subprocess.run(
            ["bash", str(PROBE_SCRIPT)],
            env=self._base_env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(
            proc.returncode,
            0,
            f"Probe failed on valid stacked label:\nstdout={proc.stdout}\nstderr={proc.stderr}",
        )
        stub_log = self.log_file.read_text(encoding="utf-8")
        self.assertIn("timeout 10", stub_log)
        self.assertIn("--pull=never", stub_log)
        self.assertIn("--network=none", stub_log)
        self.assertIn("--user postgres", stub_log)
        self.assertIn("--security-opt apparmor=alt-docker-postgres", stub_log)
        self.assertIn("docker rm -f alt-apparmor-probe-", stub_log)

    def test_probe_succeeds_with_unstacked_profile_label(self) -> None:
        self.assertTrue(PROBE_SCRIPT.is_file(), f"Missing {PROBE_SCRIPT}")
        self.attr_file.write_text("alt-docker-postgres (enforce)\n", encoding="utf-8")
        self._make_executing_docker_stub()
        proc = subprocess.run(
            ["bash", str(PROBE_SCRIPT)],
            env=self._base_env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)

    def test_probe_fails_when_runtime_label_is_unconfined_or_docker_default(
        self,
    ) -> None:
        self.assertTrue(PROBE_SCRIPT.is_file(), f"Missing {PROBE_SCRIPT}")
        self._make_executing_docker_stub()
        for bad_label in (
            "unconfined\n",
            "docker-default (enforce)\n",
            "docker-default//&unconfined (enforce)\n",
        ):
            with self.subTest(label=bad_label.strip()):
                self.attr_file.write_text(bad_label, encoding="utf-8")
                proc = subprocess.run(
                    ["bash", str(PROBE_SCRIPT)],
                    env=self._base_env(),
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertNotEqual(
                    proc.returncode,
                    0,
                    f"Probe must reject wrong runtime label: {bad_label.strip()}",
                )

    def test_probe_fails_when_interprocess_kill_is_denied(self) -> None:
        self.assertTrue(PROBE_SCRIPT.is_file(), f"Missing {PROBE_SCRIPT}")
        # Override `kill` inside the container shell to simulate EACCES signal denial
        deny_kill = (
            'script_body="kill() { echo \\"kill: Permission denied\\" >&2; return 1; }; '
            '$script_body"'
        )
        self._make_executing_docker_stub(extra_shell_prefix=deny_kill)
        proc = subprocess.run(
            ["bash", str(PROBE_SCRIPT)],
            env=self._base_env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(
            proc.returncode,
            0,
            "Probe must fail non-zero when inter-process signal is denied",
        )

    def test_probe_fails_when_signal_is_not_delivered_to_child(self) -> None:
        self.assertTrue(PROBE_SCRIPT.is_file(), f"Missing {PROBE_SCRIPT}")
        # Override `kill` to return 0 without delivering USR1 to the child process
        swallow_kill = 'script_body="kill() { return 0; }; $script_body"'
        self._make_executing_docker_stub(extra_shell_prefix=swallow_kill)
        proc = subprocess.run(
            ["bash", str(PROBE_SCRIPT)],
            env=self._base_env(),
            capture_output=True,
            text=True,
            timeout=10,
            check=False,
        )
        self.assertNotEqual(
            proc.returncode,
            0,
            "Probe must fail when child process never receives USR1 (even if kill exits 0)",
        )


class TestCheckAppArmorProfileScript(unittest.TestCase):
    """Verify `scripts/check-apparmor-profile.sh` failure and success modes."""

    def setUp(self) -> None:
        self.tmpdir_obj = tempfile.TemporaryDirectory()
        self.tmpdir = Path(self.tmpdir_obj.name)
        self.bin_dir = self.tmpdir / "bin"
        self.bin_dir.mkdir()
        self.log_file = self.tmpdir / "check.log"

        self.source_profile = self.tmpdir / "source-alt-docker-postgres"
        if PROFILE_PATH.is_file():
            self.source_profile.write_bytes(PROFILE_PATH.read_bytes())
        else:
            self.source_profile.write_text(
                "profile alt-docker-postgres {}\n", encoding="utf-8"
            )

        self.host_profile = self.tmpdir / "etc-apparmor-alt-docker-postgres"
        self.host_profile.write_bytes(self.source_profile.read_bytes())

        self.parser_stub = _write_executable(
            self.bin_dir / "apparmor_parser",
            "#!/usr/bin/env bash\n"
            f'echo "apparmor_parser $*" >> "{self.log_file}"\n'
            "exit 0\n",
        )
        self.probe_stub = _write_executable(
            self.bin_dir / "probe-stub.sh",
            "#!/usr/bin/env bash\n"
            f'echo "probe $*" >> "{self.log_file}"\n'
            "exit 0\n",
        )

    def tearDown(self) -> None:
        self.tmpdir_obj.cleanup()

    def _env(self) -> dict[str, str]:
        return {
            **os.environ,
            "PATH": f"{self.bin_dir}:{os.environ.get('PATH', '')}",
            "APPARMOR_SOURCE_PROFILE": str(self.source_profile),
            "APPARMOR_HOST_PROFILE": str(self.host_profile),
            "APPARMOR_PROFILE_NAME": "alt-docker-postgres",
            "APPARMOR_PARSER_BIN": str(self.parser_stub),
            "PROBE_SCRIPT": str(self.probe_stub),
        }

    def test_checker_fails_when_host_profile_missing(self) -> None:
        self.assertTrue(CHECK_SCRIPT.is_file(), f"Missing {CHECK_SCRIPT}")
        self.host_profile.unlink()
        proc = subprocess.run(
            ["bash", str(CHECK_SCRIPT)],
            env=self._env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(proc.returncode, 0)
        if self.log_file.exists():
            self.assertNotIn("probe", self.log_file.read_text(encoding="utf-8"))

    def test_checker_compiles_without_accessing_root_owned_cache(self) -> None:
        _write_executable(
            self.parser_stub,
            '#!/usr/bin/env bash\n'
            'if [[ "$1" == "-Q" ]]; then\n'
            '  for arg in "$@"; do\n'
            '    if [[ "$arg" == "-K" ]]; then exit 0; fi\n'
            '  done\n'
            '  echo "Failed setting up policy cache: Permission denied" >&2\n'
            '  exit 1\n'
            'fi\n'
            'exit 0\n',
        )
        proc = subprocess.run(
            ["bash", str(CHECK_SCRIPT), "--static-only"],
            env=self._env(), capture_output=True, text=True, check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)

    def test_checker_fails_when_host_profile_is_stale_vs_source(self) -> None:
        self.assertTrue(CHECK_SCRIPT.is_file(), f"Missing {CHECK_SCRIPT}")
        self.host_profile.write_text(
            self.source_profile.read_text(encoding="utf-8") + "\n# drifted\n",
            encoding="utf-8",
        )
        proc = subprocess.run(
            ["bash", str(CHECK_SCRIPT)],
            env=self._env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(
            proc.returncode,
            0,
            "Checker must fail when installed host profile SHA-256 differs from repo source",
        )
        if self.log_file.exists():
            self.assertNotIn("probe", self.log_file.read_text(encoding="utf-8"))

    def test_checker_fails_when_apparmor_parser_reports_syntax_error(self) -> None:
        self.assertTrue(CHECK_SCRIPT.is_file(), f"Missing {CHECK_SCRIPT}")
        for fail_flag in ("--preprocess", "-Q"):
            with self.subTest(fail_flag=fail_flag):
                self.log_file.write_text("", encoding="utf-8")
                _write_executable(
                    self.parser_stub,
                    "#!/usr/bin/env bash\n"
                    f'echo "apparmor_parser $*" >> "{self.log_file}"\n'
                    f'if [[ "$1" == "{fail_flag}" ]]; then\n'
                    '  echo "AppArmor parser error" >&2\n'
                    "  exit 1\n"
                    "fi\n"
                    "exit 0\n",
                )
                proc = subprocess.run(
                    ["bash", str(CHECK_SCRIPT)],
                    env=self._env(),
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertNotEqual(proc.returncode, 0)
                self.assertNotIn("probe", self.log_file.read_text(encoding="utf-8"))

    def test_checker_fails_when_runtime_probe_fails(self) -> None:
        self.assertTrue(CHECK_SCRIPT.is_file(), f"Missing {CHECK_SCRIPT}")
        _write_executable(
            self.probe_stub,
            "#!/usr/bin/env bash\n"
            f'echo "probe $*" >> "{self.log_file}"\n'
            'echo "probe failed: signal denied" >&2\n'
            "exit 1\n",
        )
        proc = subprocess.run(
            ["bash", str(CHECK_SCRIPT)],
            env=self._env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("probe", self.log_file.read_text(encoding="utf-8"))

    def test_checker_succeeds_in_default_probe_mode_and_static_only_mode(self) -> None:
        self.assertTrue(CHECK_SCRIPT.is_file(), f"Missing {CHECK_SCRIPT}")
        proc = subprocess.run(
            ["bash", str(CHECK_SCRIPT)],
            env=self._env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        log_text = self.log_file.read_text(encoding="utf-8")
        self.assertIn("--preprocess", log_text)
        self.assertIn("probe", log_text)

        # Reset log and verify --static-only skips probe
        self.log_file.write_text("", encoding="utf-8")
        proc_static = subprocess.run(
            ["bash", str(CHECK_SCRIPT), "--static-only"],
            env=self._env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc_static.returncode, 0, proc_static.stderr)
        static_log = self.log_file.read_text(encoding="utf-8")
        self.assertIn("--preprocess", static_log)
        self.assertNotIn("probe", static_log)


class TestInstallAppArmorProfileScript(unittest.TestCase):
    """Verify `scripts/install-apparmor-profile.sh` ordering, idempotency, and kernel reload."""

    def setUp(self) -> None:
        self.tmpdir_obj = tempfile.TemporaryDirectory()
        self.tmpdir = Path(self.tmpdir_obj.name)
        self.bin_dir = self.tmpdir / "bin"
        self.bin_dir.mkdir()
        self.log_file = self.tmpdir / "install.log"

        self.source_profile = self.tmpdir / "source-alt-docker-postgres"
        if PROFILE_PATH.is_file():
            self.source_profile.write_bytes(PROFILE_PATH.read_bytes())
        else:
            self.source_profile.write_text(
                "profile alt-docker-postgres {}\n", encoding="utf-8"
            )
        self.host_profile = self.tmpdir / "etc" / "apparmor.d" / "alt-docker-postgres"
        self.host_profile.parent.mkdir(parents=True, exist_ok=True)

        self.parser_stub = _write_executable(
            self.bin_dir / "apparmor_parser",
            "#!/usr/bin/env bash\n"
            f'echo "apparmor_parser $*" >> "{self.log_file}"\n'
            "exit 0\n",
        )
        self.check_stub = _write_executable(
            self.bin_dir / "check-stub.sh",
            "#!/usr/bin/env bash\n"
            f'echo "check $*" >> "{self.log_file}"\n'
            "exit 0\n",
        )

    def tearDown(self) -> None:
        self.tmpdir_obj.cleanup()

    def _env(self) -> dict[str, str]:
        return {
            **os.environ,
            "PATH": f"{self.bin_dir}:{os.environ.get('PATH', '')}",
            "APPARMOR_SOURCE_PROFILE": str(self.source_profile),
            "APPARMOR_HOST_PROFILE": str(self.host_profile),
            "APPARMOR_PARSER_BIN": str(self.parser_stub),
            "CHECK_SCRIPT": str(self.check_stub),
        }

    def test_installer_validates_syntax_before_writing_host_profile(self) -> None:
        self.assertTrue(INSTALL_SCRIPT.is_file(), f"Missing {INSTALL_SCRIPT}")
        for fail_flag in ("--preprocess", "-Q"):
            with self.subTest(fail_flag=fail_flag):
                self.log_file.write_text("", encoding="utf-8")
                if self.host_profile.exists():
                    self.host_profile.unlink()
                _write_executable(
                    self.parser_stub,
                    "#!/usr/bin/env bash\n"
                    f'echo "apparmor_parser $*" >> "{self.log_file}"\n'
                    f'if [[ "$1" == "{fail_flag}" ]]; then\n'
                    '  echo "syntax/compile error in source profile" >&2\n'
                    "  exit 1\n"
                    "fi\n"
                    "exit 0\n",
                )
                proc = subprocess.run(
                    ["bash", str(INSTALL_SCRIPT)],
                    env=self._env(),
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertNotEqual(proc.returncode, 0)
                self.assertFalse(
                    self.host_profile.exists(),
                    f"Installer must not write host profile when pre-install {fail_flag} check fails",
                )
                log_text = self.log_file.read_text(encoding="utf-8")
                self.assertNotIn("-r", log_text)
                self.assertNotIn("check", log_text)

    def test_installer_reloads_kernel_profile_even_when_host_file_is_unchanged(
        self,
    ) -> None:
        self.assertTrue(INSTALL_SCRIPT.is_file(), f"Missing {INSTALL_SCRIPT}")
        # Pre-populate identical host file to simulate lost kernel state with intact disk file
        self.host_profile.write_bytes(self.source_profile.read_bytes())
        before_digest = hashlib.sha256(self.host_profile.read_bytes()).hexdigest()

        proc = subprocess.run(
            ["bash", str(INSTALL_SCRIPT)],
            env=self._env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        after_digest = hashlib.sha256(self.host_profile.read_bytes()).hexdigest()
        self.assertEqual(before_digest, after_digest)

        log_lines = self.log_file.read_text(encoding="utf-8").splitlines()
        self.assertTrue(
            any("--preprocess" in line for line in log_lines),
            f"Expected --preprocess before load in {log_lines}",
        )
        self.assertTrue(
            any("-r" in line and "-W" in line and str(self.host_profile) in line for line in log_lines),
            f"Installer must reload kernel policy (-r -W) even when file is unchanged: {log_lines}",
        )
        self.assertTrue(
            any(line.startswith("check") for line in log_lines),
            f"Installer must run post-install verification check: {log_lines}",
        )

    def test_installer_fails_when_kernel_load_fails(self) -> None:
        self.assertTrue(INSTALL_SCRIPT.is_file(), f"Missing {INSTALL_SCRIPT}")
        _write_executable(
            self.parser_stub,
            "#!/usr/bin/env bash\n"
            f'echo "apparmor_parser $*" >> "{self.log_file}"\n'
            'for arg in "$@"; do\n'
            '  if [[ "$arg" == "-r" ]]; then\n'
            '    echo "kernel load failed" >&2\n'
            "    exit 1\n"
            "  fi\n"
            "done\n"
            "exit 0\n",
        )
        proc = subprocess.run(
            ["bash", str(INSTALL_SCRIPT)],
            env=self._env(),
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(proc.returncode, 0)
        self.assertNotIn("check", self.log_file.read_text(encoding="utf-8"))


class TestOptInComposeOverridesAndRunbook(unittest.TestCase):
    """Verify base compose files and compatibility overrides cover the 8 prod and 6 staging DBs."""

    def test_base_postgres_services_require_the_confined_profile(self) -> None:
        targets = {
            "db.yaml": {"db", "pre-processor-db"},
            "auth.yaml": {"kratos-db"},
            "acolyte.yaml": {"acolyte-db"},
            "pact.yaml": {"pact-db"},
            "rag.yaml": {"rag-db"},
            "recap.yaml": {"recap-db"},
            "sovereign.yaml": {"knowledge-sovereign-db"},
        }
        for filename, expected in targets.items():
            content = (REPO_ROOT / "compose" / filename).read_text(encoding="utf-8")
            services, _ = _parse_override_services(content)
            for service in expected:
                with self.subTest(file=filename, service=service):
                    self.assertEqual(
                        services.get(service), ["apparmor:alt-docker-postgres"]
                    )
            self.assertNotIn("apparmor:unconfined", content)

    def test_staging_base_postgres_services_stay_portable_without_unconfined_fallback(
        self,
    ) -> None:
        content = (REPO_ROOT / "compose" / "compose.staging.yaml").read_text(
            encoding="utf-8"
        )
        services, _ = _parse_override_services(content)
        for service in EXPECTED_STAGING_DB_SERVICES:
            with self.subTest(service=service):
                self.assertIn(service, services)
                self.assertEqual(
                    services.get(service),
                    [],
                    f"{service} in compose.staging.yaml must not hardcode host-specific apparmor:alt-docker-postgres",
                )
        self.assertNotIn("apparmor:alt-docker-postgres", content)
        self.assertNotIn("apparmor:unconfined", content)

    def test_production_opt_in_override_covers_exact_8_postgres_services(self) -> None:
        self.assertTrue(
            PROD_OVERRIDE_PATH.is_file(),
            f"Missing production opt-in AppArmor override: {PROD_OVERRIDE_PATH}",
        )
        services, has_override = _parse_override_services(
            PROD_OVERRIDE_PATH.read_text(encoding="utf-8")
        )
        self.assertEqual(
            set(services.keys()),
            EXPECTED_PROD_DB_SERVICES,
            f"Production override must cover exact 8 DB services. Got: {set(services.keys())}",
        )
        for svc, opts in services.items():
            self.assertEqual(
                opts,
                ["apparmor:alt-docker-postgres"],
                f"{svc} in {PROD_OVERRIDE_PATH} must set apparmor:alt-docker-postgres",
            )
            self.assertTrue(
                has_override.get(svc, False),
                f"{svc} in {PROD_OVERRIDE_PATH} must use !override on security_opt so it replaces apparmor:unconfined",
            )

    def test_staging_opt_in_override_covers_exact_6_staging_postgres_services(
        self,
    ) -> None:
        self.assertTrue(
            STAGING_OVERRIDE_PATH.is_file(),
            f"Missing staging opt-in AppArmor override: {STAGING_OVERRIDE_PATH}",
        )
        services, has_override = _parse_override_services(
            STAGING_OVERRIDE_PATH.read_text(encoding="utf-8")
        )
        self.assertEqual(
            set(services.keys()),
            EXPECTED_STAGING_DB_SERVICES,
            f"Staging override must cover exact 6 staging DB services. Got: {set(services.keys())}",
        )
        for svc, opts in services.items():
            self.assertEqual(
                opts,
                ["apparmor:alt-docker-postgres"],
                f"{svc} in {STAGING_OVERRIDE_PATH} must set apparmor:alt-docker-postgres",
            )
            self.assertTrue(
                has_override.get(svc, False),
                f"{svc} in {STAGING_OVERRIDE_PATH} must use !override on security_opt",
            )

    def test_base_compose_include_does_not_auto_select_apparmor_override(self) -> None:
        root_compose = (REPO_ROOT / "compose" / "compose.yaml").read_text(
            encoding="utf-8"
        )
        self.assertNotIn(
            "compose.apparmor.yaml",
            root_compose,
            "compose/compose.yaml must not automatically include compose.apparmor.yaml before host validation",
        )

    def test_runbook_documents_provisioning_validation_opt_in_and_rollback(
        self,
    ) -> None:
        self.assertTrue(
            RUNBOOK_PATH.is_file(),
            f"Missing runbook: {RUNBOOK_PATH}",
        )
        text = RUNBOOK_PATH.read_text(encoding="utf-8")
        for required_snippet in (
            "alt-docker-postgres",
            "scripts/install-apparmor-profile.sh",
            "scripts/check-apparmor-profile.sh",
            "scripts/probe-apparmor-postgres.sh",
            "compose/compose.apparmor.yaml",
            "compose/compose.staging.apparmor.yaml",
            "//&unconfined",
        ):
            self.assertIn(
                required_snippet,
                text,
                f"Runbook {RUNBOOK_PATH} must reference {required_snippet!r}",
            )


if __name__ == "__main__":
    unittest.main()
