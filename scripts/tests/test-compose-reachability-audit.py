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
assert spec is not None and spec.loader is not None
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
    def test_caller_without_a_shared_network_is_flagged(self):
        self.assertEqual(
            triples(INTERNAL_ONLY),
            [("rag-orchestrator", "EMBEDDER_EXTERNAL", "knowledge-embedder-local")],
        )

    def test_caller_sharing_a_network_is_clean(self):
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
        self.assertEqual(triples(cfg), [])

    def test_list_form_environment_is_read(self):
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
        self.assertEqual(triples(cfg), [("caller", "BACKEND_URL", "backend")])

    def test_unknown_and_loopback_hosts_are_not_compose_services(self):
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
        self.assertEqual(triples(cfg), [])

    def test_values_without_a_scheme_are_not_urls(self):
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
        self.assertEqual(triples(cfg), [])

    def test_every_url_in_a_comma_separated_list_is_checked(self):
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
        self.assertEqual(triples(cfg), [("caller", "PEERS", "far")])

    def test_userinfo_does_not_hide_the_host(self):
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
        self.assertEqual(triples(cfg), [("caller", "REDIS_URL", "redis-streams")])

    def test_alias_resolves_only_on_the_network_that_declares_it(self):
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
        self.assertEqual(triples(cfg), [("caller", "PRIVATE", "back-alias")])

    def test_container_name_resolves_like_the_service_name(self):
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
        self.assertEqual(triples(cfg), [("caller", "TARGET", "target-container")])

    def test_network_mode_service_shares_the_parent_networks(self):
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
        self.assertEqual(triples(cfg), [("sidecar", "FAR", "far")])

    def test_network_mode_host_resolves_no_service_names(self):
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
        self.assertEqual(triples(cfg), [("caller", "TARGET", "target")])

    def test_a_callee_inside_another_netns_has_no_name_of_its_own(self):
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
        self.assertEqual(triples(cfg), [("caller", "SIDECAR", "sidecar")])

    def test_service_without_networks_joins_default(self):
        cfg = load(
            """
services:
  caller:
    environment:
      TARGET: http://target:80
  target: {}
"""
        )
        self.assertEqual(triples(cfg), [])

    def test_networks_compare_by_resolved_name(self):
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
        self.assertEqual(triples(cfg), [])

    def test_extra_hosts_entry_is_not_a_dns_lookup(self):
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
        self.assertEqual(triples(cfg), [])

    def test_findings_are_sorted_and_unique(self):
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
        self.assertEqual(
            triples(cfg),
            [("alpha", "A", "far"), ("zeta", "A", "far"), ("zeta", "B", "far")],
        )


class RenderTests(unittest.TestCase):
    def render_argv(self, *args) -> list[str]:
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout=json.dumps({"services": {}}), stderr="")
        with mock.patch.object(audit.subprocess, "run", return_value=completed) as run:
            self.assertEqual(audit.render(pathlib.Path("compose/compose.yaml"), pathlib.Path(".env.template"), *args), {"services": {}})
        return run.call_args.args[0]

    def test_render_uses_the_env_file_and_the_deploy_profiles(self):
        argv = self.render_argv()
        self.assertEqual(argv[:2], ["docker", "compose"])
        self.assertEqual(argv[argv.index("--env-file") + 1], ".env.template")
        self.assertEqual(argv[argv.index("-f") + 1], "compose/compose.yaml")
        self.assertNotIn("--profile", argv)
        self.assertEqual(argv[-3:], ["config", "--format", "json"])

    def test_render_adds_requested_profiles(self):
        argv = self.render_argv(["tts", "perf"])
        self.assertEqual([argv[i + 1] for i, arg in enumerate(argv) if arg == "--profile"], ["tts", "perf"])
        self.assertEqual(argv[-3:], ["config", "--format", "json"])

    def test_uninterpolated_render_keeps_env_file_out_of_environment(self):
        argv = self.render_argv(None, False)
        self.assertEqual(argv[-4:], ["config", "--no-interpolate", "--format", "json"])

    def test_render_failure_raises_with_the_exit_code(self):
        completed = subprocess.CompletedProcess(args=[], returncode=15, stdout="", stderr="env file .env not found")
        with mock.patch.object(audit.subprocess, "run", return_value=completed):
            with self.assertRaises(audit.RenderError) as raised:
                audit.render(pathlib.Path("compose/compose.yaml"), pathlib.Path(".env.template"))
        self.assertEqual(raised.exception.returncode, 15)


class OriginTests(unittest.TestCase):
    def test_declared_keys_read_both_environment_forms(self):
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
        self.assertEqual(audit.declared_keys(raw), {"listed": {"A", "B"}, "mapped": {"C"}, "bare": set()})

    def test_split_separates_declared_from_env_file_variables(self):
        found = [audit.Finding("svc", "DECLARED", "h"), audit.Finding("svc", "INJECTED", "h")]
        own, shared = audit.split_by_origin(found, {"svc": {"DECLARED"}})
        self.assertEqual(own, [audit.Finding("svc", "DECLARED", "h")])
        self.assertEqual(shared, [audit.Finding("svc", "INJECTED", "h")])


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
        with mock.patch.object(audit, "render", side_effect=renders), redirect_stdout(out), redirect_stderr(err):
            code = audit.main([])
        return code, out.getvalue() + err.getvalue()

    def test_declared_variable_fails_while_env_file_variable_is_reported(self):
        code, output = self.run_main(ENV_FILE_RENDERED, ENV_FILE_RAW)
        self.assertEqual(code, 1)
        self.assertIn("reader\tEMBED_URL\tbackend", output)
        self.assertIn("SHARED_URL\tbackend\t2", output)
        self.assertNotIn("http://", output)

    def test_env_file_only_findings_do_not_fail(self):
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
        self.assertEqual(code, 0)
        self.assertIn("EMBED_URL\tbackend\t1", output)
        self.assertIn("SHARED_URL\tbackend\t2", output)

    def test_findings_exit_one_and_print_only_service_variable_host(self):
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
        self.assertEqual(code, 1)
        self.assertIn("caller", output)
        self.assertIn("DATABASE_URL", output)
        self.assertIn("db", output)
        for leaked in ("s3cret-value", "postgres://", "5432", "sslmode"):
            self.assertNotIn(leaked, output)

    def test_clean_config_exits_zero(self):
        code, _ = self.run_main({"services": {"a": {"networks": ["n"]}}})
        self.assertEqual(code, 0)

    def test_render_failure_exits_two(self):
        out, err = io.StringIO(), io.StringIO()
        with mock.patch.object(audit, "render", side_effect=audit.RenderError("boom", 15)), redirect_stdout(out), redirect_stderr(err):
            self.assertEqual(audit.main([]), 2)


if __name__ == "__main__":
    unittest.main()
