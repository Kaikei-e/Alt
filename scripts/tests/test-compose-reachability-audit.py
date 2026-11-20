#!/usr/bin/env python3
"""Tests for scripts/compose-reachability-audit.py.

Run:
    python3 scripts/tests/test-compose-reachability-audit.py
"""

from __future__ import annotations

import importlib.util
import io
import json
import pathlib
import subprocess
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest import mock

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "compose-reachability-audit.py"

spec = importlib.util.spec_from_file_location("reachability_audit", SCRIPT)
assert spec is not None
assert spec.loader is not None
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


def load(text: str) -> dict:
    return yaml.safe_load(text)


# The production failure shape: a caller on the shared network holding a URL
# for a backend that lives only on an internal network behind a proxy.
INTERNAL_ONLY = load(
    """
services:
  rag-orchestrator:
    networks: [alt-network]
    environment:
      EMBEDDER_EXTERNAL: http://knowledge-embedder-local:11434
      SEARCH_INDEXER_URL: http://search-indexer:9300
  embedding-proxy:
    networks: [alt-network, embedding-raw-network]
  knowledge-embedder-local:
    networks: [embedding-raw-network]
  search-indexer:
    networks: [alt-network]
networks:
  alt-network: {}
  embedding-raw-network:
    internal: true
"""
)


def triples(cfg: dict) -> list[tuple[str, str, str]]:
    return [(f.service, f.variable, f.host) for f in audit.findings(cfg)]


class FindingsTests(unittest.TestCase):
    def test_caller_without_a_shared_network_is_flagged(self) -> None:
        got = triples(INTERNAL_ONLY)
        assert got == [("rag-orchestrator", "EMBEDDER_EXTERNAL", "knowledge-embedder-local")], got

    def test_caller_sharing_a_network_is_clean(self) -> None:
        cfg = load(
            """
services:
  rag-orchestrator:
    networks: [alt-network]
    environment:
      EMBEDDER_EXTERNAL: http://embedding-proxy:11436
  embedding-proxy:
    networks: [alt-network, embedding-raw-network]
"""
        )
        got = triples(cfg)
        assert got == [], got

    def test_list_form_environment_is_read(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      - BACKEND_URL=http://backend:80
  backend:
    networks: [back]
"""
        )
        got = triples(cfg)
        assert got == [("caller", "BACKEND_URL", "backend")], got

    def test_unknown_and_loopback_hosts_are_not_compose_services(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      LOOPBACK: http://127.0.0.1:9
      EXTERNAL: https://example.com/path
      BLANK: ""
      UNSET: null
"""
        )
        got = triples(cfg)
        assert got == [], got

    def test_values_without_a_scheme_are_not_urls(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      DB_HOST: backend
      ADDR: backend:5432
  backend:
    networks: [back]
"""
        )
        got = triples(cfg)
        assert got == [], got

    def test_every_url_in_a_comma_separated_list_is_checked(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      PEERS: http://near:1,http://far:2
  near:
    networks: [front]
  far:
    networks: [back]
"""
        )
        got = triples(cfg)
        assert got == [("caller", "PEERS", "far")], got

    def test_userinfo_does_not_hide_the_host(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      REDIS_URL: redis://streams@redis-streams:6379
  redis-streams:
    networks: [back]
"""
        )
        got = triples(cfg)
        assert got == [("caller", "REDIS_URL", "redis-streams")], got

    def test_alias_resolves_only_on_the_network_that_declares_it(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      SHARED: http://front-alias
      PRIVATE: http://back-alias
  target:
    networks:
      front:
        aliases: [front-alias]
      back:
        aliases: [back-alias]
"""
        )
        got = triples(cfg)
        assert got == [("caller", "PRIVATE", "back-alias")], got

    def test_container_name_resolves_like_the_service_name(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      TARGET: http://target-container:80
  target:
    container_name: target-container
    networks: [back]
"""
        )
        got = triples(cfg)
        assert got == [("caller", "TARGET", "target-container")], got

    def test_network_mode_service_shares_the_parent_networks(self) -> None:
        cfg = load(
            """
services:
  parent:
    networks: [front]
  sidecar:
    network_mode: service:parent
    environment:
      OK: http://near:1
      FAR: http://far:2
  near:
    networks: [front]
  far:
    networks: [back]
"""
        )
        got = triples(cfg)
        assert got == [("sidecar", "FAR", "far")], got

    def test_network_mode_host_resolves_no_service_names(self) -> None:
        cfg = load(
            """
services:
  caller:
    network_mode: host
    environment:
      TARGET: http://target:80
  target:
    networks: [front]
"""
        )
        got = triples(cfg)
        assert got == [("caller", "TARGET", "target")], got

    def test_a_callee_inside_another_netns_has_no_name_of_its_own(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      PARENT: http://parent:80
      SIDECAR: http://sidecar:81
  parent:
    networks: [front]
  sidecar:
    network_mode: service:parent
"""
        )
        got = triples(cfg)
        assert got == [("caller", "SIDECAR", "sidecar")], got

    def test_service_without_networks_joins_default(self) -> None:
        cfg = load(
            """
services:
  caller:
    environment:
      TARGET: http://target:80
  target: {}
"""
        )
        got = triples(cfg)
        assert got == [], got

    def test_networks_compare_by_resolved_name(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [shared-a]
    environment:
      TARGET: http://target:80
  target:
    networks: [shared-b]
networks:
  shared-a:
    name: alt_shared
  shared-b:
    name: alt_shared
"""
        )
        got = triples(cfg)
        assert got == [], got

    def test_extra_hosts_entry_is_not_a_dns_lookup(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    extra_hosts:
      - target=10.0.0.5
    environment:
      TARGET: http://target:80
  target:
    networks: [back]
"""
        )
        got = triples(cfg)
        assert got == [], got

    def test_findings_are_sorted_and_unique(self) -> None:
        cfg = load(
            """
services:
  zeta:
    networks: [front]
    environment:
      B: http://far:1
      A: http://far:1
  alpha:
    networks: [front]
    environment:
      A: http://far:1,http://far:2
  far:
    networks: [back]
"""
        )
        got = triples(cfg)
        assert got == [("alpha", "A", "far"), ("zeta", "A", "far"), ("zeta", "B", "far")], got


class RenderTests(unittest.TestCase):
    def render_argv(self, *args: object) -> list[str]:
        completed = subprocess.CompletedProcess(
            args=[], returncode=0, stdout=json.dumps({"services": {}}), stderr=""
        )
        with mock.patch.object(audit.subprocess, "run", return_value=completed) as run:
            rendered = audit.render(
                pathlib.Path("compose/compose.yaml"), pathlib.Path(".env.template"), *args
            )
            assert rendered == {"services": {}}, rendered
        return run.call_args.args[0]

    def test_render_uses_the_env_file_and_the_deploy_profiles(self) -> None:
        argv = self.render_argv()
        assert argv[:2] == ["docker", "compose"], argv
        assert argv[argv.index("--env-file") + 1] == ".env.template", argv
        assert argv[argv.index("-f") + 1] == "compose/compose.yaml", argv
        assert "--profile" not in argv, argv
        assert argv[-3:] == ["config", "--format", "json"], argv

    def test_render_adds_requested_profiles(self) -> None:
        argv = self.render_argv(["tts", "perf"])
        profiles = [argv[i + 1] for i, arg in enumerate(argv) if arg == "--profile"]
        assert profiles == ["tts", "perf"], profiles
        assert argv[-3:] == ["config", "--format", "json"], argv

    def test_uninterpolated_render_keeps_env_file_out_of_environment(self) -> None:
        argv = self.render_argv(None, False)
        assert argv[-4:] == ["config", "--no-interpolate", "--format", "json"], argv

    def test_render_passes_repo_root_as_cwd(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[], returncode=0, stdout=json.dumps({"services": {}}), stderr=""
        )
        stage = pathlib.Path("/tmp/stage-root")  # noqa: S108 - test fixture value
        with mock.patch.object(audit.subprocess, "run", return_value=completed) as run:
            audit.render(
                stage / "compose" / "compose.yaml",
                stage / ".env",
                repo_root=stage,
            )
        cwd = run.call_args.kwargs.get("cwd")
        assert cwd == stage, cwd

    def test_render_failure_raises_with_the_exit_code(self) -> None:
        expected_rc = 15
        completed = subprocess.CompletedProcess(
            args=[], returncode=expected_rc, stdout="", stderr="env file .env not found"
        )
        with (
            mock.patch.object(audit.subprocess, "run", return_value=completed),
            self.assertRaises(audit.RenderError) as raised,  # noqa: PT027 - unittest runner; pytest is not a dependency of this file
        ):
            audit.render(pathlib.Path("compose/compose.yaml"), pathlib.Path(".env.template"))
        assert raised.exception.returncode == expected_rc, raised.exception.returncode


class OriginTests(unittest.TestCase):
    def test_declared_keys_read_both_environment_forms(self) -> None:
        raw = load(
            """
services:
  listed:
    environment:
      - A=${A:-http://x}
      - B
  mapped:
    environment:
      C: http://y
  bare: {}
"""
        )
        got = audit.declared_keys(raw)
        assert got == {"listed": {"A", "B"}, "mapped": {"C"}, "bare": set()}, got

    def test_split_separates_declared_from_env_file_variables(self) -> None:
        found = [audit.Finding("svc", "DECLARED", "h"), audit.Finding("svc", "INJECTED", "h")]
        own, shared = audit.split_by_origin(found, {"svc": {"DECLARED"}})
        assert own == [audit.Finding("svc", "DECLARED", "h")], own
        assert shared == [audit.Finding("svc", "INJECTED", "h")], shared


# What `docker compose config` renders when two callers carry env_file: the
# interpolated view merges the file into `environment`, the uninterpolated view
# does not.
ENV_FILE_RENDERED = load(
    """
services:
  reader:
    networks: [front]
    environment:
      EMBED_URL: http://backend:11434
      SHARED_URL: http://backend:11434
  bystander:
    networks: [front]
    environment:
      SHARED_URL: http://backend:11434
  backend:
    networks: [back]
"""
)
ENV_FILE_RAW = load(
    """
services:
  reader:
    networks: [front]
    environment:
      EMBED_URL: ${EMBED_URL:-http://proxy:11436}
  bystander:
    networks: [front]
  backend:
    networks: [back]
"""
)


class MainTests(unittest.TestCase):
    def run_main(self, cfg: dict, raw: dict | None = None) -> tuple[int, str]:
        out, err = io.StringIO(), io.StringIO()
        renders = [cfg, cfg if raw is None else raw]
        with (
            mock.patch.object(audit, "render", side_effect=renders),
            redirect_stdout(out),
            redirect_stderr(err),
        ):
            code = audit.main([])
        return code, out.getvalue() + err.getvalue()

    def test_declared_variable_fails_while_env_file_variable_is_reported(self) -> None:
        code, output = self.run_main(ENV_FILE_RENDERED, ENV_FILE_RAW)
        assert code == 1, output
        assert "reader\tEMBED_URL\tbackend" in output, output
        assert "SHARED_URL\tbackend\t2" in output, output
        assert "http://" not in output, output

    def test_env_file_only_findings_do_not_fail(self) -> None:
        raw = load(
            """
services:
  reader:
    networks: [front]
  bystander:
    networks: [front]
  backend:
    networks: [back]
"""
        )
        code, output = self.run_main(ENV_FILE_RENDERED, raw)
        assert code == 0, output
        assert "EMBED_URL\tbackend\t1" in output, output
        assert "SHARED_URL\tbackend\t2" in output, output

    def test_findings_exit_one_and_print_only_service_variable_host(self) -> None:
        cfg = load(
            """
services:
  caller:
    networks: [front]
    environment:
      DATABASE_URL: postgres://app:s3cret-value@db:5432/app?sslmode=disable
  db:
    networks: [back]
"""
        )
        code, output = self.run_main(cfg)
        assert code == 1, output
        assert "caller" in output, output
        assert "DATABASE_URL" in output, output
        assert "db" in output, output
        for leaked in ("s3cret-value", "postgres://", "5432", "sslmode"):
            assert leaked not in output, output

    def test_clean_config_exits_zero(self) -> None:
        code, output = self.run_main({"services": {"a": {"networks": ["n"]}}})
        assert code == 0, output

    def test_render_failure_exits_two(self) -> None:
        render_error_exit_code = 2
        out, err = io.StringIO(), io.StringIO()
        with (
            mock.patch.object(audit, "render", side_effect=audit.RenderError("boom", 15)),
            redirect_stdout(out),
            redirect_stderr(err),
        ):
            rc = audit.main([])
            assert rc == render_error_exit_code, rc


if __name__ == "__main__":
    unittest.main()
