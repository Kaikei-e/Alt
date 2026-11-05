import hashlib
import os
import stat
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_SCRIPT = REPO_ROOT / "scripts" / "ci-install-compose.sh"


def _extract_option_value(args: list[str], flag: str) -> str | None:
    for index, token in enumerate(args):
        if token == flag and index + 1 < len(args):
            return args[index + 1]
        if token.startswith(f"{flag}="):
            return token.split("=", 1)[1]
    return None


def _has_short_flag(args: list[str], short_flag: str) -> bool:
    for token in args:
        if token.startswith("-") and not token.startswith("--") and short_flag in token[1:]:
            return True
    return False


class CiInstallComposeTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.home_dir = self.root / "home"
        self.bin_dir = self.root / "bin"
        self.state_dir = self.root / "state"
        self.home_dir.mkdir(parents=True, exist_ok=True)
        self.bin_dir.mkdir(parents=True, exist_ok=True)
        self.state_dir.mkdir(parents=True, exist_ok=True)

        self.script_path = Path(
            os.environ.get("CI_INSTALL_COMPOSE_SCRIPT", str(DEFAULT_SCRIPT))
        )
        self.payload = b"#!/bin/sh\necho fake-compose-binary\n"
        self.payload_sha256 = hashlib.sha256(self.payload).hexdigest()

        self._write_executable(
            self.bin_dir / "uname",
            """#!/usr/bin/env bash
            echo "${FAKE_UNAME_ARCH:-x86_64}"
            """,
        )
        self._write_executable(
            self.bin_dir / "curl",
            """#!/usr/bin/env bash
            set -euo pipefail
            printf '%s\\n' "$@" > "${FAKE_STATE_DIR}/curl_args.txt"
            if [ "${FAKE_CURL_EXIT_CODE:-0}" -ne 0 ]; then
              exit "${FAKE_CURL_EXIT_CODE}"
            fi
            out=""
            while [ "$#" -gt 0 ]; do
              case "$1" in
                -o|--output)
                  out="$2"
                  shift 2
                  ;;
                *)
                  shift
                  ;;
              esac
            done
            if [ -n "$out" ]; then
              cp "${FAKE_CURL_PAYLOAD_FILE}" "$out"
            fi
            """,
        )
        self._write_executable(
            self.bin_dir / "docker",
            """#!/usr/bin/env bash
            set -euo pipefail
            printf '%s\\n' "$@" > "${FAKE_STATE_DIR}/docker_args.txt"
            echo "Docker Compose version v5.5.0"
            """,
        )

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def _write_executable(self, path: Path, content: str) -> None:
        path.write_text(textwrap.dedent(content).lstrip(), encoding="utf-8")
        path.chmod(path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

    def _run_script(
        self,
        *,
        payload: bytes | None = None,
        expected_sha256: str | None = None,
        curl_exit_code: int = 0,
        uname_arch: str = "x86_64",
    ) -> subprocess.CompletedProcess[str]:
        payload_file = self.state_dir / "payload.bin"
        payload_file.write_bytes(self.payload if payload is None else payload)

        env = os.environ.copy()
        env["HOME"] = str(self.home_dir)
        env["PATH"] = f"{self.bin_dir}:{env.get('PATH', '')}"
        env["FAKE_STATE_DIR"] = str(self.state_dir)
        env["FAKE_CURL_PAYLOAD_FILE"] = str(payload_file)
        env["FAKE_CURL_EXIT_CODE"] = str(curl_exit_code)
        env["FAKE_UNAME_ARCH"] = uname_arch
        env["COMPOSE_SHA256"] = (
            self.payload_sha256 if expected_sha256 is None else expected_sha256
        )
        env.pop("COMPOSE_VERSION", None)

        return subprocess.run(
            ["bash", str(self.script_path)],
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_curl_invoked_with_bounded_retry_and_timeout_options(self) -> None:
        result = self._run_script()
        self.assertEqual(
            result.returncode,
            0,
            msg=f"Expected exit 0, got {result.returncode}: {result.stderr}",
        )

        curl_args_file = self.state_dir / "curl_args.txt"
        self.assertTrue(curl_args_file.is_file(), "curl was not invoked")
        curl_args = curl_args_file.read_text(encoding="utf-8").splitlines()

        self.assertTrue(
            _has_short_flag(curl_args, "f") or "--fail" in curl_args,
            f"curl must retain -f/--fail: {curl_args}",
        )
        self.assertTrue(
            _has_short_flag(curl_args, "L") or "--location" in curl_args,
            f"curl must retain -L/--location redirect handling: {curl_args}",
        )

        retry = _extract_option_value(curl_args, "--retry")
        retry_delay = _extract_option_value(curl_args, "--retry-delay")
        connect_timeout = _extract_option_value(curl_args, "--connect-timeout")
        max_time = _extract_option_value(curl_args, "--max-time")
        retry_max_time = _extract_option_value(curl_args, "--retry-max-time")

        self.assertIsNotNone(retry, f"Missing --retry in {curl_args}")
        self.assertIsNotNone(retry_delay, f"Missing --retry-delay in {curl_args}")
        self.assertIsNotNone(
            connect_timeout, f"Missing --connect-timeout in {curl_args}"
        )
        self.assertIsNotNone(max_time, f"Missing --max-time in {curl_args}")
        self.assertIsNotNone(retry_max_time, f"Missing --retry-max-time in {curl_args}")

        self.assertGreaterEqual(int(retry), 3)
        self.assertGreaterEqual(int(retry_delay), 1)
        self.assertGreater(int(connect_timeout), 0)
        self.assertGreater(int(max_time), 0)
        self.assertGreaterEqual(int(retry_max_time), int(max_time))

        expected_plugin = self.home_dir / ".docker" / "cli-plugins" / "docker-compose"
        self.assertEqual(_extract_option_value(curl_args, "-o"), str(expected_plugin))
        self.assertIn(
            "https://github.com/docker/compose/releases/download/v5.5.0/docker-compose-linux-x86_64",
            curl_args,
        )
        self.assertTrue(expected_plugin.is_file())
        self.assertTrue(os.access(expected_plugin, os.X_OK))

        docker_args_file = self.state_dir / "docker_args.txt"
        self.assertTrue(docker_args_file.is_file(), "docker was not invoked")
        self.assertEqual(
            docker_args_file.read_text(encoding="utf-8").splitlines(),
            ["compose", "version"],
        )

    def test_checksum_mismatch_fails_before_chmod_and_docker(self) -> None:
        result = self._run_script(payload=b"corrupted-binary-payload")
        self.assertNotEqual(result.returncode, 0)

        expected_plugin = self.home_dir / ".docker" / "cli-plugins" / "docker-compose"
        self.assertTrue(expected_plugin.is_file())
        self.assertFalse(
            os.access(expected_plugin, os.X_OK),
            "Plugin binary must not be marked executable when SHA-256 verification fails",
        )
        self.assertFalse(
            (self.state_dir / "docker_args.txt").exists(),
            "docker compose version must not run after checksum failure",
        )

    def test_curl_exit_22_propagates_failure_without_running_docker(self) -> None:
        result = self._run_script(curl_exit_code=22)
        self.assertEqual(result.returncode, 22)
        self.assertFalse(
            (self.state_dir / "docker_args.txt").exists(),
            "docker compose version must not run when curl fails",
        )

    def test_unsupported_architecture_exits_nonzero_before_curl(self) -> None:
        result = self._run_script(uname_arch="aarch64")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(
            (self.state_dir / "curl_args.txt").exists(),
            "curl must not be called on non-x86_64 architectures",
        )


if __name__ == "__main__":
    unittest.main()
