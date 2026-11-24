#!/usr/bin/env python3
"""Wave 3-E Security Remediations Test Suite (SEC-03 through SEC-10)."""

from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent.parent


class TestWave3ESecurity(unittest.TestCase):
    def test_sec03_logging_yaml_no_docker_containers_mount(self) -> None:
        """compose/logging.yaml must not mount /var/lib/docker/containers in x-rask-forwarder."""
        logging_yaml = yaml.safe_load(
            (REPO_ROOT / "compose" / "logging.yaml").read_text(encoding="utf-8")
        )
        fwd = logging_yaml.get("x-rask-forwarder", {})
        volumes = fwd.get("volumes", [])
        for vol in volumes:
            assert "/var/lib/docker/containers" not in str(vol), f"unexpected mount: {vol!r}"

    def test_sec08_dockerfiles_nonroot_and_scram_auth(self) -> None:
        """inference-proxy and otel-relay run as non-root; pg_hba.conf and pgbouncer use scram-sha-256."""
        inf_df = (REPO_ROOT / "docker" / "inference-proxy" / "Dockerfile").read_text(
            encoding="utf-8"
        )
        assert "FROM gcr.io/distroless/static-debian12:nonroot" in inf_df, inf_df
        assert "USER nonroot:nonroot" in inf_df, inf_df

        otel_df = (REPO_ROOT / "docker" / "otel-relay" / "Dockerfile").read_text(encoding="utf-8")
        assert "USER 65532:65532" in otel_df, otel_df

        pg_hba = (REPO_ROOT / "docker" / "postgres" / "pg_hba.conf").read_text(encoding="utf-8")
        for line in pg_hba.splitlines():
            stripped = line.strip()
            if stripped.startswith("host"):
                assert "scram-sha-256" in stripped, stripped
                assert "md5" not in stripped, stripped

        pgbouncer = (REPO_ROOT / "compose" / "pgbouncer.yaml").read_text(encoding="utf-8")
        assert "AUTH_TYPE=scram-sha-256" in pgbouncer, pgbouncer
        assert "AUTH_TYPE=md5" not in pgbouncer, pgbouncer
        assert "AUTH_TYPE=plain" not in pgbouncer, pgbouncer

    def test_sec04_sec05_sec06_workflows_and_safety_check(self) -> None:
        """Workflows enforce security gates and workflow-safety-check detects dangerous contexts."""
        py_wf = (REPO_ROOT / ".github" / "workflows" / "reusable-test-python.yaml").read_text(
            encoding="utf-8"
        )
        assert "--exit-zero" not in py_wf, "reusable-test-python.yaml still contains --exit-zero"

        go_wf = (REPO_ROOT / ".github" / "workflows" / "reusable-go-quality-gates.yaml").read_text(
            encoding="utf-8"
        )
        assert "-no-fail" not in go_wf, "reusable-go-quality-gates.yaml still contains -no-fail"

        deno_wf = (REPO_ROOT / ".github" / "workflows" / "reusable-test-deno.yaml").read_text(
            encoding="utf-8"
        )
        assert "|| true" not in deno_wf, "reusable-test-deno.yaml still contains || true"

        proto_wf = (REPO_ROOT / ".github" / "workflows" / "proto-contract.yaml").read_text(
            encoding="utf-8"
        )
        for line in proto_wf.splitlines():
            if "buf breaking" in line:
                assert "|| true" not in line, line

        alt_harness = REPO_ROOT / "scripts" / "altharness" / "alt-harness"
        assert alt_harness.is_file(), f"Missing {alt_harness}"

        def run_workflow_check(repo_root: Path) -> subprocess.CompletedProcess[str]:
            return subprocess.run(  # noqa: S603 - fixed repo-local alt-harness binary and arguments
                [str(alt_harness), "workflow", "--repo-root", str(repo_root)],
                capture_output=True,
                text=True,
                check=False,
            )

        # Real repo workflows must pass alt-harness workflow cleanly.
        real_res = run_workflow_check(REPO_ROOT)
        assert real_res.returncode == 0, (
            f"alt-harness workflow failed on real repo:\nSTDOUT:\n{real_res.stdout}\nSTDERR:\n{real_res.stderr}"
        )

        # Fixture 1: dangerous attacker-controlled github.event.* expressions in run: bodies.
        with tempfile.TemporaryDirectory(prefix="alt-wave3e-wf-") as tmp_str:
            tmp_root = Path(tmp_str)
            wf_dir = tmp_root / ".github" / "workflows"
            wf_dir.mkdir(parents=True, exist_ok=True)
            dangerous_samples = [
                "github.event.workflow_run.head_sha",
                "github.event.discussion.title",
                "github.event.pages[0].page_name",
                "github.event.commits[0].message",
            ]
            steps_yaml = "".join(
                f"      - run: echo ${{{{ {sample} }}}}\n" for sample in dangerous_samples
            )
            (wf_dir / "dangerous.yaml").write_text(
                "on: [push]\n"
                "permissions:\n"
                "  contents: read\n"
                "jobs:\n"
                "  check:\n"
                "    runs-on: ubuntu-latest\n"
                "    timeout-minutes: 10\n"
                "    steps:\n" + steps_yaml,
                encoding="utf-8",
            )
            res_danger = run_workflow_check(tmp_root)
            combined_danger = res_danger.stdout + res_danger.stderr
            assert res_danger.returncode != 0, (
                f"Expected failure on dangerous contexts: {combined_danger}"
            )
            for sample in dangerous_samples:
                assert sample in combined_danger, (
                    f"Failed to detect dangerous context {sample!r} in:\n{combined_danger}"
                )

            # Fixture 2: Rule 12 lint swallow (./scripts/lint/run.sh repo || true).
            (wf_dir / "dangerous.yaml").unlink()
            (wf_dir / "swallow.yaml").write_text(
                "on: [push]\n"
                "permissions:\n"
                "  contents: read\n"
                "jobs:\n"
                "  lint:\n"
                "    runs-on: ubuntu-latest\n"
                "    timeout-minutes: 10\n"
                "    steps:\n"
                "      - run: ./scripts/lint/run.sh repo || true\n",
                encoding="utf-8",
            )
            res_r12 = run_workflow_check(tmp_root)
            combined_r12 = res_r12.stdout + res_r12.stderr
            assert res_r12.returncode != 0, f"Rule 12 failed to flag lint swallow: {combined_r12}"
            assert "swallow" in combined_r12.lower(), (
                f"Rule 12 output missing swallow diagnostic:\n{combined_r12}"
            )

            # Fixture 3: Rule 13 direct linter invocation (ruff check .).
            (wf_dir / "swallow.yaml").unlink()
            (wf_dir / "direct-lint.yaml").write_text(
                "on: [push]\n"
                "permissions:\n"
                "  contents: read\n"
                "jobs:\n"
                "  check:\n"
                "    runs-on: ubuntu-latest\n"
                "    timeout-minutes: 10\n"
                "    steps:\n"
                "      - run: ruff check .\n",
                encoding="utf-8",
            )
            res_r13 = run_workflow_check(tmp_root)
            combined_r13 = res_r13.stdout + res_r13.stderr
            assert res_r13.returncode != 0, f"Rule 13 failed to flag direct linter: {combined_r13}"
            assert "ruff" in combined_r13, (
                f"Rule 13 output missing ruff diagnostic:\n{combined_r13}"
            )
            assert "scripts/lint/run.sh" in combined_r13, (
                f"Rule 13 output missing scripts/lint/run.sh diagnostic:\n{combined_r13}"
            )

    def test_sec07_sec09_sec10_scripts_hardening(self) -> None:
        """Scripts enforce loopback vLLM bind, mktemp unit file, umask 077, curl stdin auth, and localhost OLLAMA_ORIGINS."""
        vllm_path = REPO_ROOT / "scripts" / "deploy-vllm-aix.sh"
        if vllm_path.is_file():
            vllm_sh = vllm_path.read_text(encoding="utf-8")
            assert "${VLLM_BIND_HOST:-127.0.0.1}" in vllm_sh, vllm_sh
            assert "--host 0.0.0.0" not in vllm_sh, vllm_sh
            assert "mktemp" in vllm_sh, vllm_sh
            assert "/tmp/vllm.service " not in vllm_sh, vllm_sh  # noqa: S108 - test fixture value
            assert "> /tmp/vllm.service" not in vllm_sh, vllm_sh

        backup_pg = (REPO_ROOT / "backup-postgres-docker.sh").read_text(encoding="utf-8")
        assert "umask 077" in backup_pg, backup_pg

        backup_all = (REPO_ROOT / "scripts" / "backup" / "backup-all.sh").read_text(
            encoding="utf-8"
        )
        assert "umask 077" in backup_all, backup_all

        pact_sh = (REPO_ROOT / "scripts" / "pact-check.sh").read_text(encoding="utf-8")
        assert "/tmp/alt-pact-broker.env" not in pact_sh, pact_sh  # noqa: S108 - test fixture value
        assert "/tmp/alt_pact_broker_password.txt" not in pact_sh, pact_sh  # noqa: S108 - test fixture value
        assert '-u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}"' not in pact_sh, pact_sh
        assert ("-K -" in pact_sh) or ("--config -" in pact_sh) or ("--netrc-file" in pact_sh), (
            pact_sh
        )

        setup_path = REPO_ROOT / "setup.sh"
        if setup_path.is_file():
            setup_sh = setup_path.read_text(encoding="utf-8")
            assert (
                "http://localhost,https://localhost,http://127.0.0.1,https://127.0.0.1" in setup_sh
            ), setup_sh
            assert "<string>*</string>" not in setup_sh, setup_sh


if __name__ == "__main__":
    unittest.main()
