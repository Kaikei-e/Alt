#!/usr/bin/env python3
"""Wave 3-E Security Remediations Test Suite (SEC-03 through SEC-10)."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import unittest
import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent.parent


class TestWave3ESecurity(unittest.TestCase):
    def test_sec03_logging_yaml_no_docker_containers_mount(self):
        """compose/logging.yaml must not mount /var/lib/docker/containers in x-rask-forwarder."""
        logging_yaml = yaml.safe_load((REPO_ROOT / "compose" / "logging.yaml").read_text(encoding="utf-8"))
        fwd = logging_yaml.get("x-rask-forwarder", {})
        volumes = fwd.get("volumes", [])
        for vol in volumes:
            self.assertNotIn("/var/lib/docker/containers", str(vol))

    def test_sec08_dockerfiles_nonroot_and_scram_auth(self):
        """inference-proxy and otel-relay run as non-root; pg_hba.conf and pgbouncer use scram-sha-256."""
        inf_df = (REPO_ROOT / "docker" / "inference-proxy" / "Dockerfile").read_text(encoding="utf-8")
        self.assertIn("FROM gcr.io/distroless/static-debian12:nonroot", inf_df)
        self.assertIn("USER nonroot:nonroot", inf_df)

        otel_df = (REPO_ROOT / "docker" / "otel-relay" / "Dockerfile").read_text(encoding="utf-8")
        self.assertIn("USER 65532:65532", otel_df)

        pg_hba = (REPO_ROOT / "docker" / "postgres" / "pg_hba.conf").read_text(encoding="utf-8")
        for line in pg_hba.splitlines():
            stripped = line.strip()
            if stripped.startswith("host"):
                self.assertIn("scram-sha-256", stripped)
                self.assertNotIn("md5", stripped)

        pgbouncer = (REPO_ROOT / "compose" / "pgbouncer.yaml").read_text(encoding="utf-8")
        self.assertIn("AUTH_TYPE=scram-sha-256", pgbouncer)
        self.assertNotIn("AUTH_TYPE=md5", pgbouncer)
        self.assertNotIn("AUTH_TYPE=plain", pgbouncer)

    def test_sec04_sec05_sec06_workflows_and_safety_check(self):
        """Workflows enforce security gates and workflow-safety-check detects dangerous contexts."""
        py_wf = (REPO_ROOT / ".github" / "workflows" / "reusable-test-python.yaml").read_text(encoding="utf-8")
        self.assertNotIn("--exit-zero", py_wf)

        go_wf = (REPO_ROOT / ".github" / "workflows" / "reusable-go-quality-gates.yaml").read_text(encoding="utf-8")
        self.assertNotIn("-no-fail", go_wf)

        deno_wf = (REPO_ROOT / ".github" / "workflows" / "reusable-test-deno.yaml").read_text(encoding="utf-8")
        self.assertNotIn("|| true", deno_wf)

        proto_wf = (REPO_ROOT / ".github" / "workflows" / "proto-contract.yaml").read_text(encoding="utf-8")
        for line in proto_wf.splitlines():
            if "buf breaking" in line:
                self.assertNotIn("|| true", line)

        for rel_path in [
            REPO_ROOT / "scripts" / "workflow-safety-check.py",
            REPO_ROOT / ".github" / "workflows" / "scripts" / "workflow-safety-check.py",
        ]:
            self.assertTrue(rel_path.is_file(), f"Missing {rel_path}")
            spec = importlib.util.spec_from_file_location("workflow_safety_check", rel_path)
            mod = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(mod)
            regex = getattr(mod, "_DANGEROUS_CONTEXT_RE", None)
            self.assertIsNotNone(regex, f"_DANGEROUS_CONTEXT_RE missing in {rel_path}")
            for sample in [
                "github.event.workflow_run.head_sha",
                "github.event.discussion.title",
                "github.event.pages[0].page_name",
                "github.event.commits[0].message",
            ]:
                self.assertIsNotNone(regex.search(sample), f"Failed to detect {sample}")

    def test_sec07_sec09_sec10_scripts_hardening(self):
        """Scripts enforce loopback vLLM bind, mktemp unit file, umask 077, curl stdin auth, and localhost OLLAMA_ORIGINS."""
        vllm_sh = (REPO_ROOT / "scripts" / "deploy-vllm-aix.sh").read_text(encoding="utf-8")
        self.assertIn("${VLLM_BIND_HOST:-127.0.0.1}", vllm_sh)
        self.assertNotIn("--host 0.0.0.0", vllm_sh)
        self.assertIn("mktemp", vllm_sh)
        self.assertNotIn("/tmp/vllm.service ", vllm_sh)
        self.assertNotIn("> /tmp/vllm.service", vllm_sh)

        backup_pg = (REPO_ROOT / "backup-postgres-docker.sh").read_text(encoding="utf-8")
        self.assertIn("umask 077", backup_pg)

        backup_all = (REPO_ROOT / "scripts" / "backup" / "backup-all.sh").read_text(encoding="utf-8")
        self.assertIn("umask 077", backup_all)

        pact_sh = (REPO_ROOT / "scripts" / "pact-check.sh").read_text(encoding="utf-8")
        self.assertNotIn("/tmp/alt-pact-broker.env", pact_sh)
        self.assertNotIn("/tmp/alt_pact_broker_password.txt", pact_sh)
        self.assertNotIn('-u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}"', pact_sh)
        self.assertTrue(("-K -" in pact_sh) or ("--config -" in pact_sh) or ("--netrc-file" in pact_sh))

        setup_sh = (REPO_ROOT / "setup.sh").read_text(encoding="utf-8")
        self.assertIn("http://localhost,https://localhost,http://127.0.0.1,https://127.0.0.1", setup_sh)
        self.assertNotIn("<string>*</string>", setup_sh)


if __name__ == "__main__":
    unittest.main()
