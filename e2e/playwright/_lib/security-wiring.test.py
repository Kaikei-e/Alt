"""Offline checks of staging startup requirements; never starts Compose."""

import unittest
import json
import os
import re
import subprocess
import tempfile
from pathlib import Path
from urllib.parse import urlparse

import yaml

ROOT = Path(__file__).resolve().parents[3]
TEST_CREDENTIALS = ROOT / "e2e/playwright/_fixtures/test-credentials"
INFERENCE_TOKEN_PATH = "/run/secrets/inference_service_token"


def environment(service):
    values = service.get("environment", {})
    if isinstance(values, dict):
        return values
    return dict(value.split("=", 1) for value in values if "=" in value)


def suite_endpoints(suite):
    """The `suite_endpoint NAME "default"` pairs a run.sh declares."""
    text = (ROOT / "e2e/playwright" / suite / "run.sh").read_text()
    return dict(re.findall(r'^suite_endpoint\s+(\w+)\s+"([^"]*)"', text, re.MULTILINE))


class SecurityWiringTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.compose = yaml.safe_load((ROOT / "compose/compose.staging.yaml").read_text())
        cls.services = cls.compose["services"]

    def test_enabled_go_telemetry_has_mounted_token(self):
        for name in ("search-indexer", "mq-hub", "alt-backend", "alt-harvester", "alt-data-hub", "rag-orchestrator", "auth-hub"):
            with self.subTest(service=name):
                svc = self.services[name]
                self.assertEqual(environment(svc).get("RASK_INGEST_TOKEN_FILE"), "/run/secrets/rask_ingest_token")
                self.assertIn("rask_ingest_token", svc.get("secrets", []))

    def test_enabled_mq_clients_have_the_same_file_contract(self):
        for name in ("mq-hub", "alt-harvester", "alt-data-hub"):
            with self.subTest(service=name):
                svc = self.services[name]
                self.assertEqual(environment(svc).get("MQHUB_AUTH_TOKEN_FILE"), "/run/secrets/mqhub_auth_token")
                self.assertIn("mqhub_auth_token", svc.get("secrets", []))

    def test_kratos_entrypoints_have_db_secret(self):
        for name in ("kratos", "auth-hub-db-migrator"):
            with self.subTest(service=name):
                svc = self.services[name]
                self.assertEqual(environment(svc).get("KRATOS_DB_PASSWORD_FILE"), "/run/secrets/kratos_db_password")
                self.assertIn("kratos_db_password", svc.get("secrets", []))

    def test_auth_hub_uses_bounded_ttls_and_frontend_https(self):
        env = environment(self.services["auth-hub"])
        self.assertEqual(env.get("FRONTEND_TLS_LISTEN"), "true")
        self.assertEqual(env.get("CACHE_TTL"), "60s")
        self.assertEqual(env.get("BACKEND_TOKEN_TTL"), "5m")
        self.assertNotEqual(env.get("AUTH_HUB_DEV_PLAINTEXT"), "true")

    def test_kratos_explicitly_selects_existing_staging_template(self):
        for name in ("kratos", "auth-hub-db-migrator"):
            with self.subTest(service=name):
                self.assertEqual(environment(self.services[name]).get("KRATOS_TEMPLATE_FILE"), "/etc/config/kratos/kratos.yml")

    def test_real_kratos_entrypoint_renders_selected_template_without_running_kratos(self):
        # Invoke the actual entrypoint with a local argv-reporting executable;
        # no Kratos process, listener, Docker or DB is started.
        with tempfile.TemporaryDirectory(prefix="alt-e2e-kratos-") as tmp:
            output = Path(tmp) / "config.yml"
            env = dict(os.environ)
            env.update({
                "KRATOS_TEMPLATE_FILE": str(ROOT / "e2e/fixtures/auth-hub/kratos/kratos.yml"),
                "KRATOS_CONFIG_FILE": str(output),
                "KRATOS_DB_PASSWORD_FILE": str(ROOT / "e2e/playwright/_fixtures/test-credentials/kratos_db_password.txt"),
                "KRATOS_COOKIE_SECRET_FILE": str(ROOT / "e2e/fixtures/staging-secrets/auth_hub_kratos_cookie_secret.txt"),
                "KRATOS_CIPHER_SECRET_FILE": str(ROOT / "e2e/fixtures/staging-secrets/auth_hub_kratos_cipher_secret.txt"),
                "KRATOS_DB_HOST": "auth-hub-db",
            })
            result = subprocess.run(["sh", str(ROOT / "kratos/entrypoint.sh"), "python3", "-c", "import json,sys;print(json.dumps(sys.argv[1:]))", "--config", "/old/config.yml"], env=env, capture_output=True, text=True, timeout=10, check=True)
            self.assertEqual(json.loads(result.stdout), ["--config", str(output)])
            config = yaml.safe_load(output.read_text())
            self.assertNotIn("${", output.read_text())
            self.assertTrue(config["dsn"].startswith("postgres://kratos_user:"))
            self.assertIn("@auth-hub-db:5432/kratos?", config["dsn"])
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)

    def test_python_business_listeners_require_peer_identity(self):
        for name, flag in (("news-creator", "INBOUND_MTLS"), ("tag-generator", "INBOUND_TLS_ENABLED")):
            with self.subTest(service=name):
                env = environment(self.services[name])
                self.assertEqual(env.get(flag), "true")
                self.assertEqual(env.get("PEER_IDENTITY_STRICT"), "true")
                self.assertIn("alt-backend", env.get("MTLS_ALLOWED_PEERS", "").split(","))

    def test_migrators_have_writable_cache(self):
        for name, svc in self.services.items():
            if "migrator" in name and name != "auth-hub-db-migrator":
                with self.subTest(service=name):
                    self.assertEqual(environment(svc).get("HOME"), "/tmp")
                    self.assertEqual(environment(svc).get("XDG_CACHE_HOME"), "/tmp/cache")

    def test_search_introspection_uses_verified_https(self):
        env = environment(self.services["search-indexer"])
        self.assertEqual(env.get("USER_JWT_INTROSPECTION_URL"), "https://auth-introspection:9443/internal/token/introspect")
        for name in ("MTLS_CERT_FILE", "MTLS_KEY_FILE", "MTLS_CA_FILE"):
            self.assertTrue(env.get(name), name)

    def test_rag_orchestrator_has_inference_token_wiring(self):
        svc = self.services["rag-orchestrator"]
        env = environment(svc)
        self.assertEqual(env.get("INFERENCE_SERVICE_TOKEN_FILE"), "/run/secrets/inference_service_token")
        self.assertIn("inference_service_token", svc.get("secrets", []))
        secret_file = ROOT / "compose" / self.compose["secrets"]["inference_service_token"]["file"]
        self.assertTrue(secret_file.exists(), f"secret file {secret_file} must exist")
        token = secret_file.read_text().strip()
        self.assertGreaterEqual(len(token), 16)

    def test_rag_orchestrator_stub_upstreams_satisfy_its_startup_contract(self):
        env = environment(self.services["rag-orchestrator"])
        # rag-orchestrator refuses a plaintext SEARCH_INDEXER_URL at startup, even
        # one this slice never dials.
        self.assertEqual(urlparse(env.get("SEARCH_INDEXER_URL", "")).scheme, "https")
        # One inference token covers embedder, rerank and Augur.
        self.assertNotIn("RERANK_INFERENCE_TOKEN_FILE", env)

    def test_preprocessor_upstream_uses_verified_https_and_server_name(self):
        for name in ("alt-backend", "alt-harvester"):
            with self.subTest(service=name):
                env = environment(self.services[name])
                self.assertTrue(env.get("PRE_PROCESSOR_URL", "").startswith("https://"))
                self.assertTrue(env.get("PRE_PROCESSOR_CONNECT_URL", "").startswith("https://"))
                self.assertEqual(env.get("PRE_PROCESSOR_MTLS_SERVER_NAME"), "auth-hub")
        auth_env = environment(self.services["auth-introspection"])
        self.assertEqual(auth_env.get("STAGING_FORWARD_PREPROCESSOR"), "true")

    def test_alt_backend_has_sovereign_operator_token_wiring(self):
        svc = self.services["alt-backend"]
        env = environment(svc)
        self.assertEqual(env.get("SOVEREIGN_OPERATOR_TOKEN_FILE"), "/run/secrets/sovereign_operator_token")
        self.assertIn("sovereign_operator_token", svc.get("secrets", []))
        secret_file = ROOT / "compose" / self.compose["secrets"]["sovereign_operator_token"]["file"]
        self.assertTrue(secret_file.exists(), f"secret file {secret_file} must exist")
        token = secret_file.read_text().strip()
        self.assertGreaterEqual(len(token), 24)

    def test_alt_backend_has_operator_token_wiring(self):
        svc = self.services["alt-backend"]
        env = environment(svc)
        self.assertNotEqual(env.get("OPERATOR_AUTH"), "disabled", "OPERATOR_AUTH=disabled must not be set when privileged sovereign operator is enabled")
        self.assertEqual(env.get("OPERATOR_TOKEN_FILE"), "/run/secrets/backend_operator_token")
        self.assertIn("backend_operator_token", svc.get("secrets", []))
        self.assertIn("backend_operator_token", self.compose.get("secrets", {}))
        secret_file = ROOT / "compose" / self.compose["secrets"]["backend_operator_token"]["file"]
        self.assertTrue(secret_file.exists(), f"secret file {secret_file} must exist")
        token = secret_file.read_text().strip()
        self.assertGreaterEqual(len(token), 24)

    def test_acolyte_orchestrator_search_indexer_mtls_wiring(self):
        svc = self.services["acolyte-orchestrator"]
        env = environment(svc)
        self.assertEqual(env.get("SEARCH_INDEXER_URL"), "https://search-indexer:9443")
        self.assertEqual(env.get("MTLS_ENFORCE"), "true")
        self.assertEqual(env.get("MTLS_CERT_FILE"), "/certs/acolyte-orchestrator.pem")
        self.assertEqual(env.get("MTLS_KEY_FILE"), "/certs/acolyte-orchestrator-key.pem")
        self.assertEqual(env.get("MTLS_CA_FILE"), "/trust/ca-bundle.pem")
        volumes = svc.get("volumes", [])
        volume_targets = [v.split(":")[-2] for v in volumes if ":" in v]
        self.assertIn("/certs", volume_targets)
        self.assertIn("/trust", volume_targets)


class RedisAclParityTests(unittest.TestCase):
    """Staging redis-streams must enforce the same ACL users production does.

    With `--requirepass` and the default user, staging accepted the legacy
    single-argument AUTH and every command on every key, so a client that
    production's ACL rejects still passed E2E.
    """

    @classmethod
    def setUpClass(cls):
        cls.compose = yaml.safe_load((ROOT / "compose/compose.staging.yaml").read_text())
        cls.services = cls.compose["services"]
        cls.production = yaml.safe_load((ROOT / "compose/mq.yaml").read_text())["services"]["redis-streams"]

    def test_redis_streams_runs_the_production_acl_entrypoint(self):
        svc = self.services["redis-streams"]
        self.assertEqual(svc.get("entrypoint"), self.production["entrypoint"])
        self.assertEqual(svc.get("user"), "redis")
        mounts = {c["target"]: c for c in svc.get("configs", []) if isinstance(c, dict)}
        self.assertEqual(mounts.get("/entrypoint.sh", {}).get("source"), "redis_entrypoint")
        source = ROOT / "compose" / self.compose["configs"]["redis_entrypoint"]["file"]
        self.assertEqual(source.resolve(), (ROOT / "docker/redis/entrypoint.sh").resolve())
        self.assertNotIn("--requirepass", json.dumps(svc))

    def test_entrypoint_source_is_executable(self):
        # Compose ignores `configs[].mode` for file-backed configs and bind-mounts
        # the file as it is on disk, so exec needs the bit in git itself.
        source = (ROOT / "compose" / self.compose["configs"]["redis_entrypoint"]["file"]).resolve()
        self.assertTrue(
            os.access(source, os.X_OK),
            f"{source.relative_to(ROOT)} is not executable; `git update-index --chmod=+x` it "
            "or redis-streams fails with exec permission denied in staging and production",
        )

    def test_redis_streams_mounts_both_acl_passwords(self):
        svc = self.services["redis-streams"]
        for secret in ("redis_streams_password", "redis_limiter_password"):
            with self.subTest(secret=secret):
                self.assertIn(secret, svc.get("secrets", []))

    def test_redis_streams_healthcheck_authenticates_as_the_streams_user(self):
        probe = " ".join(self.services["redis-streams"]["healthcheck"]["test"])
        self.assertIn("--user streams", probe)
        self.assertIn("/run/secrets/redis_streams_password", probe)

    def test_redis_clients_authenticate_as_the_streams_acl_user(self):
        for name, variable in (("mq-hub", "REDIS_URL"), ("tag-generator", "REDIS_STREAMS_URL")):
            with self.subTest(service=name):
                svc = self.services[name]
                env = environment(svc)
                url = urlparse(env.get(variable, ""))
                self.assertEqual((url.scheme, url.username, url.hostname, url.port), ("redis", "streams", "redis-streams", 6379))
                self.assertIsNone(url.password, "the password belongs in REDIS_PASSWORD_FILE, not the URL")
                self.assertEqual(env.get("REDIS_PASSWORD_FILE"), "/run/secrets/redis_streams_password")
                self.assertIn("redis_streams_password", svc.get("secrets", []))
                self.assertNotIn("redis_password", svc.get("secrets", []))

    def test_acl_passwords_are_synthetic_fixtures_the_entrypoint_accepts(self):
        self.assertNotIn("redis_password", self.compose["secrets"])
        for secret in ("redis_streams_password", "redis_limiter_password"):
            with self.subTest(secret=secret):
                path = (ROOT / "compose" / self.compose["secrets"][secret]["file"]).resolve()
                self.assertEqual(path.parent, TEST_CREDENTIALS.resolve())
                value = path.read_bytes().rstrip(b"\r\n")
                self.assertTrue(value.startswith(b"staging-only-"))
                self.assertGreaterEqual(len(value), 24)
                # docker/redis/entrypoint.sh rejects whitespace and control bytes.
                self.assertTrue(all(32 < byte < 127 for byte in value))

    def test_mq_hub_suite_resets_streams_as_the_acl_user(self):
        endpoints = suite_endpoints("mq-hub")
        url = urlparse(endpoints.get("REDIS_URL", ""))
        self.assertEqual((url.username, url.hostname, url.port), ("streams", "redis-streams", 6379))
        self.assertEqual(
            endpoints.get("REDIS_PASSWORD_FILE"),
            "$ROOT/e2e/playwright/_fixtures/test-credentials/redis_streams_password.txt",
        )


class InferenceTokenWiringTests(unittest.TestCase):
    """Every staging caller of an inference proxy carries the service bearer."""

    CALLERS = ("rag-orchestrator", "news-creator", "search-indexer")

    @classmethod
    def setUpClass(cls):
        cls.compose = yaml.safe_load((ROOT / "compose/compose.staging.yaml").read_text())
        cls.services = cls.compose["services"]

    def test_callers_mount_the_inference_token(self):
        for name in self.CALLERS:
            with self.subTest(service=name):
                svc = self.services[name]
                env = environment(svc)
                self.assertEqual(env.get("INFERENCE_SERVICE_TOKEN_FILE"), INFERENCE_TOKEN_PATH)
                self.assertNotIn("INFERENCE_AUTH", env)
                self.assertIn("inference_service_token", svc.get("secrets", []))

    def test_inference_token_is_a_synthetic_fixture(self):
        path = (ROOT / "compose" / self.compose["secrets"]["inference_service_token"]["file"]).resolve()
        self.assertEqual(path.parent, TEST_CREDENTIALS.resolve())
        self.assertTrue(path.read_text().startswith("staging-only-"))

    def test_news_creator_reaches_its_llm_through_the_generation_proxy(self):
        env = environment(self.services["news-creator"])
        url = urlparse(env.get("LLM_SERVICE_URL", ""))
        self.assertEqual((url.hostname, url.port), ("generation-proxy", 11436))
        depends = self.services["news-creator"].get("depends_on", {})
        self.assertEqual(depends.get("generation-proxy", {}).get("condition"), "service_healthy")

    def test_generation_proxy_fronts_the_ollama_stub_with_the_token(self):
        proxy = self.services["generation-proxy"]
        self.assertIn("news-creator", proxy.get("profiles", []))
        self.assertEqual(Path(proxy["build"]["context"]), Path("../docker/inference-proxy"))
        command = proxy.get("command", [])
        args = dict(zip(command[::2], command[1::2]))
        self.assertEqual(args.get("--target"), "http://news-creator-ollama-stub:11435")
        self.assertEqual(args.get("--port"), "11436")
        self.assertEqual(args.get("--token-file"), INFERENCE_TOKEN_PATH)
        self.assertIn("inference_service_token", proxy.get("secrets", []))
        self.assertEqual(proxy["healthcheck"]["test"], ["CMD", "/inference-proxy", "healthcheck", "-port", "11436"])

    def test_news_creator_suite_probes_the_proxy_with_the_mounted_token(self):
        endpoints = suite_endpoints("news-creator")
        self.assertEqual(endpoints.get("GENERATION_PROXY_URL"), "http://generation-proxy:11436")
        self.assertEqual(
            endpoints.get("INFERENCE_SERVICE_TOKEN_FILE"),
            "$ROOT/e2e/playwright/_fixtures/test-credentials/inference_service_token.txt",
        )


if __name__ == "__main__":
    unittest.main()
