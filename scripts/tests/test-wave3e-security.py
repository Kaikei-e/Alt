#!/usr/bin/env python3
"""Wave 3-E Security Remediations Test Suite (SEC-03 through SEC-10)."""

from __future__ import annotations

import importlib.util
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

        for rel_path in [
            REPO_ROOT / "scripts" / "workflow-safety-check.py",
            REPO_ROOT / ".github" / "workflows" / "scripts" / "workflow-safety-check.py",
        ]:
            assert rel_path.is_file(), f"Missing {rel_path}"
            spec = importlib.util.spec_from_file_location("workflow_safety_check", rel_path)
            assert spec is not None, f"Failed to load spec for {rel_path}"
            assert spec.loader is not None, f"Missing loader for {rel_path}"
            mod = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(mod)
            regex = getattr(mod, "_DANGEROUS_CONTEXT_RE", None)
            assert regex is not None, f"_DANGEROUS_CONTEXT_RE missing in {rel_path}"
            for sample in [
                "github.event.workflow_run.head_sha",
                "github.event.discussion.title",
                "github.event.pages[0].page_name",
                "github.event.commits[0].message",
            ]:
                assert regex.search(sample) is not None, f"Failed to detect {sample}"

    def test_sec07_sec09_sec10_scripts_hardening(self) -> None:
        """Scripts enforce loopback vLLM bind, mktemp unit file, umask 077, curl stdin auth, and localhost OLLAMA_ORIGINS."""
        vllm_sh = (REPO_ROOT / "scripts" / "deploy-vllm-aix.sh").read_text(encoding="utf-8")
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

        setup_sh = (REPO_ROOT / "setup.sh").read_text(encoding="utf-8")
        assert (
            "http://localhost,https://localhost,http://127.0.0.1,https://127.0.0.1" in setup_sh
        ), setup_sh
        assert "<string>*</string>" not in setup_sh, setup_sh


if __name__ == "__main__":
    unittest.main()
