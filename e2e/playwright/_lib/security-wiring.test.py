"""Offline checks of staging startup requirements; never starts Compose."""

import unittest
import json
import os
import subprocess
import tempfile
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[3]


def environment(service):
    values = service.get("environment", {})
    if isinstance(values, dict):
        return values
    return dict(value.split("=", 1) for value in values if "=" in value)


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


if __name__ == "__main__":
    unittest.main()
