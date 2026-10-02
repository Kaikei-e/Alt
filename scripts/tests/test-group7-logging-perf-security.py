#!/usr/bin/env python3
"""Group 7 (Logging & Perf Stack) Security Audit & Behavioral Test Suite.

Audits:
- D01: docker-socket-proxy-ro native binary healthcheck (/readonly-proxy healthcheck, no wget),
  non-root user (65534:984), cap_drop: [ALL], read_only: true, isolated network (logging-docker-proxy: internal).
- Forwarders: all 16 forwarders use tcp://docker-socket-proxy-ro:2375, none mount raw docker.sock,
  all mount rask_ingest_token secret.
- Rask OTLP network: plecto-otlp-collector is internal: true, joined by rask-log-aggregator.
- Compose secret inheritance: no conflicting 'external: true' definitions in logging.yaml or perf.yaml
  (both inherit file-backed secrets from base.yaml).
- D04: K6 AuthHub JWT authentication, no master HMAC secrets, feed-read-3000vu does not import generateJWT,
  all JS files pass syntax check ('node --check'), and auth_test.js passes all negative/positive claim checks.
- cAdvisor contract: documents runtime pending status for container recreation under non-root / proxy contract.
"""

import os
from pathlib import Path
import subprocess
import unittest
import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent.parent


class TestGroup7LoggingPerfSecurity(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.logging_path = REPO_ROOT / "compose" / "logging.yaml"
        cls.perf_path = REPO_ROOT / "compose" / "perf.yaml"
        cls.base_path = REPO_ROOT / "compose" / "base.yaml"

        with open(cls.logging_path, "r", encoding="utf-8") as f:
            cls.logging_compose = yaml.safe_load(f)

        with open(cls.perf_path, "r", encoding="utf-8") as f:
            cls.perf_compose = yaml.safe_load(f)

        with open(cls.base_path, "r", encoding="utf-8") as f:
            cls.base_compose = yaml.safe_load(f)

    # -------------------------------------------------------------------------
    # D01: Readonly Proxy & Logging Stack Audit
    # -------------------------------------------------------------------------
    def test_d01_readonly_proxy_configuration(self):
        """docker-socket-proxy-ro must use binary healthcheck, unprivileged user, cap_drop ALL, and read_only."""
        services = self.logging_compose["services"]
        self.assertIn("docker-socket-proxy-ro", services)
        proxy = services["docker-socket-proxy-ro"]

        # Healthcheck must use native binary, NOT wget (since scratch container lacks wget)
        hc = proxy.get("healthcheck", {})
        test_cmd = hc.get("test", [])
        self.assertEqual(
            test_cmd,
            ["CMD", "/readonly-proxy", "healthcheck"],
            "Healthcheck must call /readonly-proxy healthcheck directly",
        )
        self.assertNotIn("wget", " ".join(test_cmd))

        # Security hardening
        self.assertEqual(proxy.get("user"), "65534:984", "Must run as non-root user with docker group GID 984")
        self.assertEqual(proxy.get("cap_drop"), ["ALL"], "Must drop all Linux capabilities")
        self.assertTrue(proxy.get("read_only"), "Root filesystem must be read_only")
        self.assertIn("no-new-privileges:true", proxy.get("security_opt", []))

        # Volumes: ONLY docker.sock mounted read-only
        volumes = proxy.get("volumes", [])
        self.assertEqual(len(volumes), 1)
        v = volumes[0]
        self.assertEqual(v.get("source"), "/var/run/docker.sock")
        self.assertEqual(v.get("target"), "/var/run/docker.sock")
        self.assertTrue(v.get("read_only"))

        # Network: ONLY logging-docker-proxy (internal)
        networks = proxy.get("networks", [])
        self.assertEqual(networks, ["logging-docker-proxy"])

    def test_d01_all_16_forwarders_use_proxy_no_raw_socks(self):
        """All 16 forwarders must use DOCKER_HOST via proxy and never mount raw docker.sock."""
        # Check shared forwarder definitions
        shared_env = self.logging_compose.get("x-rask-forwarder-env", {})
        self.assertEqual(
            shared_env.get("DOCKER_HOST"),
            "tcp://docker-socket-proxy-ro:2375",
            "Forwarder default DOCKER_HOST must point to docker-socket-proxy-ro:2375",
        )
        self.assertEqual(
            shared_env.get("RASK_INGEST_TOKEN_FILE"),
            "/run/secrets/rask_ingest_token",
        )

        shared_fwd = self.logging_compose.get("x-rask-forwarder", {})
        self.assertIn("rask_ingest_token", shared_fwd.get("secrets", []))
        self.assertIn("logging-docker-proxy", shared_fwd.get("networks", []))

        # Ensure no forwarder mounts /var/run/docker.sock
        for vol in shared_fwd.get("volumes", []):
            self.assertNotIn("docker.sock", str(vol))

        # Identify all forwarder services (services ending in -logs)
        services = self.logging_compose["services"]
        forwarder_names = [name for name in services if name.endswith("-logs")]
        self.assertEqual(len(forwarder_names), 16, f"Expected 16 forwarders, found {len(forwarder_names)}: {forwarder_names}")

        for name in forwarder_names:
            svc = services[name]
            # Must inherit shared forwarder
            for vol in svc.get("volumes", []):
                self.assertNotIn("docker.sock", str(vol), f"Forwarder {name} must not mount docker.sock")

    def test_d01_rask_aggregator_and_networks(self):
        """rask-log-aggregator joins plecto-otlp-collector and has RASK_INGEST_TOKEN_FILE."""
        services = self.logging_compose["services"]
        aggregator = services["rask-log-aggregator"]

        self.assertIn("alt-network", aggregator.get("networks", []))
        self.assertIn("plecto-otlp-collector", aggregator.get("networks", []))
        self.assertIn("rask_ingest_token", aggregator.get("secrets", []))
        self.assertEqual(
            aggregator.get("environment", {}).get("RASK_INGEST_TOKEN_FILE"),
            "/run/secrets/rask_ingest_token",
        )

        # Networks declaration
        declared_networks = self.logging_compose.get("networks", {})
        self.assertTrue(declared_networks.get("logging-docker-proxy", {}).get("internal"))
        self.assertTrue(declared_networks.get("plecto-otlp-collector", {}).get("internal"))

    def test_d01_secrets_reconciliation_logging(self):
        """logging.yaml must not redeclare external: true for rask_ingest_token (inherited from base)."""
        top_secrets = self.logging_compose.get("secrets")
        if top_secrets:
            self.assertNotIn(
                "rask_ingest_token",
                top_secrets,
                "rask_ingest_token must not be declared external:true in logging.yaml when base.yaml provides it",
            )
        # Verify base.yaml provides file-backed secret
        base_secrets = self.base_compose.get("secrets", {})
        self.assertIn("rask_ingest_token", base_secrets)
        self.assertEqual(base_secrets["rask_ingest_token"].get("file"), "../secrets/rask_ingest_token.txt")

    # -------------------------------------------------------------------------
    # D04: Perf Stack & K6 AuthHub Token Audit
    # -------------------------------------------------------------------------
    def test_d04_perf_compose_configuration(self):
        """perf.yaml must not conflict with base.yaml k6_api_token, k6 runs unprivileged."""
        top_secrets = self.perf_compose.get("secrets")
        if top_secrets:
            self.assertNotIn(
                "k6_api_token",
                top_secrets,
                "k6_api_token must not be declared external:true in perf.yaml when base.yaml provides it",
            )

        base_secrets = self.base_compose.get("secrets", {})
        self.assertIn("k6_api_token", base_secrets)
        self.assertEqual(base_secrets["k6_api_token"].get("file"), "../secrets/k6_api_token.txt")

        # k6 service
        k6 = self.perf_compose["services"]["k6"]
        self.assertIn("k6_api_token", k6.get("secrets", []))
        self.assertIn("K6_API_TOKEN_FILE=/run/secrets/k6_api_token", k6.get("environment", []))
        self.assertNotEqual(k6.get("user"), "0:0", "k6 must not run as root 0:0")
        self.assertNotIn("SYS_ADMIN", k6.get("cap_add", []))

    def test_d04_feed_read_3000vu_no_generate_jwt(self):
        """feed-read-3000vu.js must NOT import or call generateJWT, must use getAuthHeaders."""
        scenario_file = REPO_ROOT / "alt-perf" / "k6" / "scenarios" / "feed-read-3000vu.js"
        content = scenario_file.read_text(encoding="utf-8")
        self.assertNotIn("generateJWT", content, "feed-read-3000vu.js must not reference generateJWT")
        self.assertIn("getAuthHeaders", content, "feed-read-3000vu.js must import getAuthHeaders")
        self.assertNotIn("backendTokenSecret", content, "feed-read-3000vu.js must not reference backendTokenSecret")

    def test_d04_all_js_syntax_validation(self):
        """All JavaScript files in alt-perf/k6 must pass node --check syntax validation."""
        k6_dir = REPO_ROOT / "alt-perf" / "k6"
        js_files = list(k6_dir.glob("**/*.js"))
        self.assertGreater(len(js_files), 5, "Expected several JS files in alt-perf/k6")

        for js_file in js_files:
            res = subprocess.run(["node", "--check", str(js_file)], capture_output=True, text=True)
            self.assertEqual(
                res.returncode,
                0,
                f"Syntax check failed for {js_file.name}:\n{res.stderr}",
            )

    def test_d04_auth_test_suite_passes(self):
        """Execute alt-perf/k6/tests/auth_test.js to verify AuthHub claim validation."""
        test_script = REPO_ROOT / "alt-perf" / "k6" / "tests" / "auth_test.js"
        res = subprocess.run(["node", str(test_script)], capture_output=True, text=True)
        self.assertEqual(
            res.returncode,
            0,
            f"auth_test.js failed:\n{res.stdout}\n{res.stderr}",
        )
        self.assertIn("All AuthHub JWT tests passed!", res.stdout)

    # -------------------------------------------------------------------------
    # cAdvisor Security Contract Audit
    # -------------------------------------------------------------------------
    def test_cadvisor_source_contract(self):
        """cAdvisor in observability.yaml enforces non-root UID/GID, cap_drop ALL, no SYSLOG grant, no whole host root or raw docker aliases, and uses tcp proxy."""
        obs_path = REPO_ROOT / "compose" / "observability.yaml"
        with open(obs_path, "r", encoding="utf-8") as f:
            obs_compose = yaml.safe_load(f)

        cadvisor = obs_compose["services"]["cadvisor"]
        # Explicit non-root UID/GID, no Docker GID supplementary group
        user = cadvisor.get("user")
        self.assertIsNotNone(user, "cAdvisor must explicitly define non-root user")
        self.assertNotEqual(user, "0:0", "cAdvisor must not run as root 0:0")
        uid, gid = user.split(":")
        self.assertNotEqual(uid, "0", "UID must be non-zero non-root")
        self.assertNotEqual(gid, "0", "GID must be non-zero non-root")
        self.assertNotEqual(gid, "984", "Must not grant Docker GID 984 supplementary group to cAdvisor")

        # Must drop ALL capabilities and remove SYSLOG/SYS_ADMIN grants
        self.assertEqual(cadvisor.get("cap_drop"), ["ALL"], "cAdvisor must drop ALL capabilities")
        cap_add = cadvisor.get("cap_add", [])
        self.assertNotIn("SYSLOG", cap_add, "SYSLOG grant must be removed from cAdvisor")
        self.assertNotIn("SYS_ADMIN", cap_add, "SYS_ADMIN must not be present in cAdvisor")
        self.assertFalse(cadvisor.get("privileged", False), "cAdvisor must not be privileged")

        # Docker API socket proxy
        self.assertEqual(cadvisor.get("command"), ["--docker=tcp://docker-socket-proxy-ro:2375"])
        self.assertIn("logging-docker-proxy", cadvisor.get("networks", []))

        # Check volumes: no whole host root mount, no raw docker socket or indirect aliases
        volumes = cadvisor.get("volumes", [])
        for vol in volumes:
            src = vol.split(":")[0] if isinstance(vol, str) else vol.get("source", "")
            tgt = vol.split(":")[1] if isinstance(vol, str) else vol.get("target", "")
            self.assertNotEqual(src, "/", "Whole host root mount (/) is forbidden in cAdvisor")
            self.assertNotEqual(tgt, "/rootfs", "Whole host rootfs target mount (/rootfs) is forbidden in cAdvisor")
            self.assertNotIn("docker.sock", str(vol), "docker.sock or alias must not be mounted in cAdvisor")
            self.assertFalse(src.startswith("/rootfs"), "Indirect /rootfs path aliases forbidden in cAdvisor")
            self.assertFalse(src.startswith("/var/run"), "Raw /var/run path forbidden in cAdvisor")
            self.assertFalse(src.startswith("/run"), "Raw /run path forbidden in cAdvisor")

        # Devices: /dev/kmsg must not be mounted
        devices = cadvisor.get("devices", [])
        for dev in devices:
            self.assertNotIn("kmsg", str(dev), "/dev/kmsg device must be removed from cAdvisor")


if __name__ == "__main__":
    unittest.main()
