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


class PkiEnrollmentModeTests(unittest.TestCase):
    """Staging runs no step-ca, so no service may enroll, and a binary that
    refuses an unset PKI_ENROLLMENT must be given an explicit opt-out."""

    @classmethod
    def setUpClass(cls):
        cls.services = yaml.safe_load((ROOT / "compose/compose.staging.yaml").read_text())["services"]

    def test_knowledge_sovereign_opts_out_of_enrollment_explicitly(self):
        # knowledge-sovereign exits at startup when PKI_ENROLLMENT is unset.
        env = environment(self.services["knowledge-sovereign"])
        self.assertEqual(env.get("PKI_ENROLLMENT"), "disabled")
        self.assertNotIn("PKI_ENROLLMENT_FILE", env)

    def test_no_service_enrolls_without_a_step_ca(self):
        self.assertNotIn("step-ca", self.services)
        for name, svc in self.services.items():
            with self.subTest(service=name):
                self.assertNotEqual(environment(svc).get("PKI_ENROLLMENT"), "enabled")

    def test_recap_evaluator_does_not_enforce_mtls_without_enrollment(self):
        # recap-evaluator refuses MTLS_ENFORCE=true unless it enrolls, and
        # enrolling is impossible here.
        svc = self.services.get("recap-evaluator")
        if svc is None:
            self.skipTest("recap-evaluator is not part of the staging stack")
        env = environment(svc)
        self.assertEqual(env.get("PKI_ENROLLMENT"), "disabled")
        self.assertNotEqual(env.get("MTLS_ENFORCE", "").strip().lower(), "true")


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

    def test_redis_streams_mounts_both_stream_passwords_and_the_limiter_password(self):
        svc = self.services["redis-streams"]
        for secret in ("redis_streams_producer_password", "redis_streams_consumer_password", "redis_limiter_password"):
            with self.subTest(secret=secret):
                self.assertIn(secret, svc.get("secrets", []))
        self.assertNotIn("redis_streams_password", svc.get("secrets", []))

    def test_redis_streams_healthcheck_authenticates_as_the_producer_user(self):
        probe = " ".join(self.services["redis-streams"]["healthcheck"]["test"])
        self.assertIn("--user streams_producer", probe)
        self.assertIn("/run/secrets/redis_streams_producer_password", probe)

    def _client(self, svc, variable, user, password_secret, other_secret):
        env = environment(svc)
        url = urlparse(env.get(variable, ""))
        self.assertEqual((url.scheme, url.username, url.hostname, url.port), ("redis", user, "redis-streams", 6379))
        self.assertIsNone(url.password, "the password belongs in REDIS_PASSWORD_FILE, not the URL")
        self.assertEqual(env.get("REDIS_PASSWORD_FILE"), f"/run/secrets/{password_secret}")
        self.assertIn(password_secret, svc.get("secrets", []))
        self.assertNotIn(other_secret, svc.get("secrets", []))
        self.assertNotIn("redis_streams_password", svc.get("secrets", []))
        self.assertNotIn("redis_password", svc.get("secrets", []))

    def test_mq_hub_authenticates_as_the_producer(self):
        self._client(self.services["mq-hub"], "REDIS_URL", "streams_producer",
                     "redis_streams_producer_password", "redis_streams_consumer_password")

    def test_stream_consumers_authenticate_as_the_consumer(self):
        # Staging runs tag-generator only; the other consumers are checked against
        # the production files they are defined in.
        self._client(self.services["tag-generator"], "REDIS_STREAMS_URL", "streams_consumer",
                     "redis_streams_consumer_password", "redis_streams_producer_password")
        for path, name in (("compose/ai.yaml", "pre-processor"), ("compose/workers.yaml", "search-indexer"),
                           ("compose/workers.yaml", "tag-generator")):
            with self.subTest(service=name, file=path):
                svc = yaml.safe_load((ROOT / path).read_text())["services"][name]
                self._client(svc, "REDIS_STREAMS_URL", "streams_consumer",
                             "redis_streams_consumer_password", "redis_streams_producer_password")
        mq = yaml.safe_load((ROOT / "compose/mq.yaml").read_text())["services"]["mq-hub"]
        self._client(mq, "REDIS_URL", "streams_producer",
                     "redis_streams_producer_password", "redis_streams_consumer_password")

    def test_acl_passwords_are_synthetic_fixtures_the_entrypoint_accepts(self):
        self.assertNotIn("redis_password", self.compose["secrets"])
        self.assertNotIn("redis_streams_password", self.compose["secrets"])
        values = {}
        for secret in ("redis_streams_producer_password", "redis_streams_consumer_password", "redis_limiter_password"):
            with self.subTest(secret=secret):
                path = (ROOT / "compose" / self.compose["secrets"][secret]["file"]).resolve()
                self.assertEqual(path.parent, TEST_CREDENTIALS.resolve())
                value = path.read_bytes().rstrip(b"\r\n")
                values[secret] = value
                self.assertTrue(value.startswith(b"staging-only-"))
                self.assertGreaterEqual(len(value), 24)
                # docker/redis/entrypoint.sh rejects whitespace and control bytes.
                self.assertTrue(all(32 < byte < 127 for byte in value))
        # The entrypoint refuses identical producer and consumer secrets.
        self.assertNotEqual(values["redis_streams_producer_password"], values["redis_streams_consumer_password"])
        self.assertFalse((TEST_CREDENTIALS / "redis_streams_password.txt").exists())

    def test_host_rate_limiters_authenticate_as_the_limiter_acl_user(self):
        # Production (compose/core.yaml) points both internet-facing binaries at
        # the `limiter` user on DB 3. With REDIS_AUTH=disabled and no URL, staging
        # ran them in local mode and never sent that user a command.
        core = yaml.safe_load((ROOT / "compose/core.yaml").read_text())["services"]
        for name in ("alt-backend", "alt-harvester"):
            with self.subTest(service=name):
                svc = self.services[name]
                env = environment(svc)
                url = urlparse(env.get("HOST_RATE_LIMITER_REDIS_URL", ""))
                self.assertEqual(
                    (url.scheme, url.username, url.hostname, url.port, url.path),
                    ("redis", "limiter", "redis-streams", 6379, "/3"),
                )
                self.assertIsNone(url.password, "the password belongs in the _PASSWORD_FILE, not the URL")
                self.assertEqual(
                    env.get("HOST_RATE_LIMITER_REDIS_URL"),
                    environment(core[name]).get("HOST_RATE_LIMITER_REDIS_URL"),
                )
                self.assertEqual(env.get("HOST_RATE_LIMITER_REDIS_PASSWORD_FILE"), "/run/secrets/redis_limiter_password")
                self.assertIn("redis_limiter_password", svc.get("secrets", []))
                self.assertNotIn("REDIS_AUTH", env, "no Redis client in this binary reads REDIS_AUTH once the limiter has its own file")

    def test_host_rate_limiter_suites_bring_up_redis_streams(self):
        for name in ("alt-backend", "alt-harvester"):
            with self.subTest(suite=name):
                self.assertIn(name, self.services["redis-streams"].get("profiles", []))
                depends = self.services[name].get("depends_on", {})
                self.assertEqual(depends.get("redis-streams", {}).get("condition"), "service_healthy")
                text = (ROOT / "e2e/playwright" / name / "run.sh").read_text()
                block = re.search(r"^suite_up((?:[^\n]*\\\n)*[^\n]*)$", text, re.MULTILINE)
                self.assertIsNotNone(block, "run.sh has no suite_up call")
                self.assertIn("redis-streams", block.group(1).replace("\\", " ").split())

    def test_redis_streams_reset_user_is_enabled_only_in_staging(self):
        svc = self.services["redis-streams"]
        self.assertEqual(environment(svc).get("REDIS_STREAMS_RESET_USER"), "enabled")
        self.assertEqual(environment(svc).get("REDIS_STREAMS_RESET_PASSWORD_FILE"), "/run/secrets/redis_streams_reset_password")
        self.assertIn("redis_streams_reset_password", svc.get("secrets", []))
        for name in ("mq-hub", "tag-generator"):
            with self.subTest(service=name):
                self.assertNotIn("redis_streams_reset_password", self.services[name].get("secrets", []))
        prod = yaml.safe_load((ROOT / "compose/mq.yaml").read_text())
        prod_svc = prod["services"]["redis-streams"]
        self.assertEqual(environment(prod_svc).get("REDIS_STREAMS_RESET_USER"), "disabled")
        self.assertNotIn("redis_streams_reset_password", prod_svc.get("secrets", []))
        self.assertNotIn("redis_streams_reset_password", json.dumps(prod))

    def test_mq_hub_suite_resets_streams_as_the_reset_user(self):
        endpoints = suite_endpoints("mq-hub")
        url = urlparse(endpoints.get("REDIS_URL", ""))
        self.assertEqual((url.username, url.hostname, url.port), ("streams_reset", "redis-streams", 6379))
        self.assertEqual(
            endpoints.get("REDIS_PASSWORD_FILE"),
            "$ROOT/e2e/playwright/_fixtures/test-credentials/redis_streams_reset_password.txt",
        )

    def test_all_redis_fixture_secrets_differ(self):
        names = ("redis_streams_producer_password", "redis_streams_consumer_password",
                 "redis_streams_reset_password", "redis_limiter_password")
        values = []
        for secret in names:
            path = (ROOT / "compose" / self.compose["secrets"][secret]["file"]).resolve()
            self.assertEqual(path.parent, TEST_CREDENTIALS.resolve())
            value = path.read_bytes().rstrip(b"\r\n")
            self.assertTrue(value.startswith(b"staging-only-"))
            self.assertTrue(all(32 < byte < 127 for byte in value))
            values.append(value)
        self.assertEqual(len(set(values)), len(values))


class ContainerHardeningParityTests(unittest.TestCase):
    """Staging runs the production capability set, so a helper that suddenly
    needs a capability fails E2E instead of the first production restart."""

    PRODUCTION = (("redis-streams", "compose/mq.yaml"), ("generation-proxy", "compose/ai.yaml"))
    STAGING_POSTGRES_SERVICES = (
        "knowledge-sovereign-db",
        "rag-db",
        "recap-db",
        "acolyte-db",
        "alt-backend-db",
        "auth-hub-db",
    )
    PRODUCTION_POSTGRES_TARGETS = (
        ("compose/db.yaml", ("db", "pre-processor-db")),
        ("compose/auth.yaml", ("kratos-db",)),
        ("compose/acolyte.yaml", ("acolyte-db",)),
        ("compose/pact.yaml", ("pact-db",)),
        ("compose/rag.yaml", ("rag-db",)),
        ("compose/recap.yaml", ("recap-db",)),
        ("compose/sovereign.yaml", ("knowledge-sovereign-db",)),
    )

    @classmethod
    def setUpClass(cls):
        cls.services = yaml.safe_load((ROOT / "compose/compose.staging.yaml").read_text())["services"]

    def test_staging_drops_capabilities_like_production(self):
        for name, source in self.PRODUCTION:
            with self.subTest(service=name):
                production = yaml.safe_load((ROOT / source).read_text())["services"][name]
                svc = self.services[name]
                self.assertEqual(svc.get("cap_drop"), ["ALL"])
                self.assertNotIn("cap_add", svc)
                self.assertIn("no-new-privileges:true", svc.get("security_opt", []))
                self.assertEqual(svc.get("cap_drop"), production.get("cap_drop"))
                self.assertEqual(svc.get("security_opt"), production.get("security_opt"))

    def test_staging_postgres_uses_default_apparmor_while_production_stays_confined(self):
        # GitHub-hosted E2E runners (ubuntu-latest) do not have the custom host
        # profile /etc/apparmor.d/alt-docker-postgres loaded, so hardcoding
        # apparmor:alt-docker-postgres in compose.staging.yaml causes runc init
        # to fail with ENOENT on proc/thread-self/attr/apparmor/exec.
        # Staging DBs must rely on Docker's built-in docker-default confinement
        # (never apparmor:unconfined), while compose.staging.apparmor.yaml
        # provides the opt-in override for Ubuntu 26.04 hosts and production
        # compose files keep apparmor:alt-docker-postgres enforced.
        for name in self.STAGING_POSTGRES_SERVICES:
            with self.subTest(staging_service=name):
                sec_opts = self.services[name].get("security_opt", [])
                self.assertNotIn("apparmor:alt-docker-postgres", sec_opts)
                self.assertNotIn("apparmor:unconfined", sec_opts)

        staging_override_text = (ROOT / "compose/compose.staging.apparmor.yaml").read_text()
        for name in self.STAGING_POSTGRES_SERVICES:
            with self.subTest(staging_override=name):
                self.assertIn(f"  {name}:", staging_override_text)
        self.assertIn("apparmor:alt-docker-postgres", staging_override_text)

        for rel_path, db_names in self.PRODUCTION_POSTGRES_TARGETS:
            prod_services = yaml.safe_load((ROOT / rel_path).read_text())["services"]
            for db_name in db_names:
                with self.subTest(production_file=rel_path, service=db_name):
                    self.assertEqual(
                        prod_services[db_name].get("security_opt"),
                        ["apparmor:alt-docker-postgres"],
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
