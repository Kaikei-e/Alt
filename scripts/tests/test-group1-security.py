#!/usr/bin/env python3
"""Group 1 Security Fixes Integration and Behavioral Test Suite.

Covers:
- A02: Plecto Proxy Mounts (ONLY manifest, dev-key.pub, OCI directory with blobs/index/oci-layout, trust CA)
- A03: Kratos Canonical Template Authority, Entrypoint Secret Substitution, Argv Preservation, Error Sanitization
- A04: Frontend HTTPS 8443, NODE_EXTRA_CA_CERTS, and AuthHub mTLS 9443 Peer Introspection Allowlist
- B04: Augur Ollama Port Binding (127.0.0.1:11435 and 127.0.0.1:11436)
- B05: Named Redis Roles (streams, cache, limiter), ACL Loader, Healthchecks, Client URLs & Secrets
- ClickHouse Native Backup Configuration (backup_disk.xml, /backups/clickhouse volume, DB rask_logs)
- Secrets Declaration & altctl Discovery Alignment
"""

import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path
import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent.parent


class TestGroup1Security(unittest.TestCase):
    def setUp(self):
        self.compose_core = self._load_yaml(REPO_ROOT / "compose" / "core.yaml")
        self.compose_auth = self._load_yaml(REPO_ROOT / "compose" / "auth.yaml")
        self.compose_mq = self._load_yaml(REPO_ROOT / "compose" / "mq.yaml")
        self.compose_ai = self._load_yaml(REPO_ROOT / "compose" / "ai.yaml")
        self.compose_workers = self._load_yaml(REPO_ROOT / "compose" / "workers.yaml")
        self.compose_augur = self._load_yaml(REPO_ROOT / "compose.augur.yaml")
        self.compose_db = self._load_yaml(REPO_ROOT / "compose" / "db.yaml")
        self.compose_base = self._load_yaml(REPO_ROOT / "compose" / "base.yaml")

    def _load_yaml(self, path: Path):
        with open(path, "r", encoding="utf-8") as f:
            return yaml.safe_load(f)

    # -------------------------------------------------------------------------
    # A02: Plecto Proxy Mounts
    # -------------------------------------------------------------------------
    def test_a02_plecto_mounts(self):
        """Plecto mounts ONLY manifest.toml, dev-key.pub, artifacts/stale-chunk-heal, and public trust CA."""
        plecto_service = self.compose_core["services"]["plecto-proxy"]
        volumes = plecto_service.get("volumes", [])

        # Must not mount the whole ../plecto directory
        for v in volumes:
            if isinstance(v, str):
                self.assertNotEqual(v, "../plecto:/etc/plecto:ro")
                self.assertFalse(v.startswith("../plecto:/"))
                # Must not mount private key
                self.assertNotIn("dev-key:", v)
                self.assertNotIn("dev-key.pem", v)
            elif isinstance(v, dict):
                src = str(v.get("source", ""))
                # Forbid mounting whole ../plecto directory regardless of target
                self.assertNotEqual(src, "../plecto")
                self.assertNotEqual(src, "../plecto/")
                self.assertFalse(src.startswith("../plecto:/"))
                # Must not mount private key
                self.assertNotIn("dev-key:", src)
                self.assertNotIn("dev-key.pem", src)

        # Expected mounts: either legacy short string or long-form bind dict with read_only=True
        manifest_mount = next(
            (
                v for v in volumes
                if (v == "../plecto/manifest.toml:/etc/plecto/manifest.toml:ro" or
                    (isinstance(v, dict) and v.get("target") == "/etc/plecto/manifest.toml"))
            ),
            None,
        )
        self.assertIsNotNone(manifest_mount, "Expected manifest.toml mount in plecto-proxy")
        if isinstance(manifest_mount, dict):
            self.assertEqual(manifest_mount.get("type"), "bind")
            self.assertEqual(manifest_mount.get("source"), "../plecto/manifest.toml")
            self.assertIs(manifest_mount.get("read_only"), True)
            self.assertFalse(manifest_mount.get("bind", {}).get("create_host_path", True))

        key_mount = next(
            (
                v for v in volumes
                if (v == "../plecto/.plecto/dev-key.pub:/etc/plecto/.plecto/dev-key.pub:ro" or
                    (isinstance(v, dict) and v.get("target") == "/etc/plecto/.plecto/dev-key.pub"))
            ),
            None,
        )
        self.assertIsNotNone(key_mount, "Expected dev-key.pub mount in plecto-proxy")
        if isinstance(key_mount, dict):
            self.assertEqual(key_mount.get("type"), "bind")
            self.assertEqual(
                key_mount.get("source"),
                "${PLECTO_PUBLIC_KEY_HOST_PATH:-/var/lib/alt-plecto/dev-key.pub}",
            )
            self.assertIs(key_mount.get("read_only"), True)
            self.assertFalse(key_mount.get("bind", {}).get("create_host_path", True))

        artifacts_mount = next(
            (
                v for v in volumes
                if (v == "../plecto/artifacts/stale-chunk-heal:/etc/plecto/artifacts/stale-chunk-heal:ro" or
                    (isinstance(v, dict) and v.get("target") == "/etc/plecto/artifacts/stale-chunk-heal"))
            ),
            None,
        )
        self.assertIsNotNone(artifacts_mount, "Expected artifacts/stale-chunk-heal mount in plecto-proxy")
        if isinstance(artifacts_mount, dict):
            self.assertEqual(artifacts_mount.get("type"), "bind")
            self.assertEqual(
                artifacts_mount.get("source"),
                "${PLECTO_ARTIFACTS_HOST_PATH:-/var/lib/alt-plecto/artifacts/stale-chunk-heal}",
            )
            self.assertIs(artifacts_mount.get("read_only"), True)
            self.assertFalse(artifacts_mount.get("bind", {}).get("create_host_path", True))

        self.assertIn("pki_trust_bundle:/trust:ro", volumes, "Expected volume mount pki_trust_bundle:/trust:ro in plecto-proxy")

        # Verify artifacts directory structure on disk if staged in checkout or operator path
        artifacts_source = Path(os.environ.get("PLECTO_ARTIFACTS_HOST_PATH", "/var/lib/alt-plecto/artifacts/stale-chunk-heal"))
        for candidate in [REPO_ROOT / "plecto" / "artifacts" / "stale-chunk-heal", artifacts_source]:
            if candidate.is_dir():
                self.assertTrue((candidate / "blobs").is_dir(), f"{candidate}/blobs directory missing")
                self.assertTrue((candidate / "index.json").is_file(), f"{candidate}/index.json missing")
                self.assertTrue((candidate / "oci-layout").is_file(), f"{candidate}/oci-layout missing")
                break

    # -------------------------------------------------------------------------
    # A03: Kratos Entrypoint & Canonical Template
    # -------------------------------------------------------------------------
    def test_a03_kratos_template_and_compose_wiring(self):
        """kratos_template.yml is authority, compose uses it and mounts all secrets."""
        template_content = (REPO_ROOT / "kratos" / "kratos_template.yml").read_text(encoding="utf-8")
        self.assertIn("${KRATOS_COOKIE_SECRET}", template_content)
        self.assertIn("${KRATOS_CIPHER_SECRET}", template_content)
        self.assertNotIn("0123456789abcdef0123456789abcdef", template_content)

        # kratos-migrate in compose/auth.yaml
        km = self.compose_auth["services"]["kratos-migrate"]
        self.assertIn("--config", km["command"])
        cfg_idx = km["command"].index("--config")
        self.assertEqual(km["command"][cfg_idx + 1], "/etc/config/kratos/kratos_template.yml")
        self.assertIn("kratos_db_password", km["secrets"])
        self.assertIn("kratos_cookie_secret", km["secrets"])
        self.assertIn("kratos_cipher_secret", km["secrets"])

        # kratos in compose/auth.yaml
        ks = self.compose_auth["services"]["kratos"]
        self.assertIn("--config", ks["command"])
        cfg_idx = ks["command"].index("--config")
        self.assertEqual(ks["command"][cfg_idx + 1], "/etc/config/kratos/kratos_template.yml")

    def test_a03_kratos_entrypoint_behavioral_fixtures(self):
        """Execute kratos/entrypoint.sh in isolated temp environments via /bin/sh."""
        import urllib.parse
        entrypoint_path = REPO_ROOT / "kratos" / "entrypoint.sh"

        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            sec_dir = tmp / "secrets"
            sec_dir.mkdir()
            (sec_dir / "db_pass").write_text("db_secret_pass\n")
            (sec_dir / "cookie_sec").write_text("cookie_secret_123\n")
            (sec_dir / "cipher_sec").write_text("cipher_secret_456\n")

            template = tmp / "kratos_template.yml"
            template.write_text(
                "version: v1.2.0\n"
                "dsn: ${DSN}\n"
                "serve:\n"
                "  public:\n"
                "    base_url: ${KRATOS_PUBLIC_URL}\n"
                "selfservice:\n"
                "  default_browser_return_url: ${KRATOS_DEFAULT_BROWSER_RETURN_URL}\n"
                "secrets:\n"
                "  cookie:\n"
                "    - ${KRATOS_COOKIE_SECRET}\n"
                "  cipher:\n"
                "    - ${KRATOS_CIPHER_SECRET}\n"
                "session:\n"
                "  cookie:\n"
                "    domain: ${KRATOS_COOKIE_DOMAIN}\n"
            )

            fake_kratos = tmp / "fake_kratos"
            fake_kratos.write_text(
                "#!/bin/sh\n"
                "for arg in \"$@\"; do\n"
                "  printf 'ARG:[%s]\\n' \"$arg\"\n"
                "done\n"
            )
            fake_kratos.chmod(0o755)

            env = {
                "PATH": f"{tmp}:" + os.environ.get("PATH", ""),
                "KRATOS_DB_PASSWORD_FILE": str(sec_dir / "db_pass"),
                "KRATOS_COOKIE_SECRET_FILE": str(sec_dir / "cookie_sec"),
                "KRATOS_CIPHER_SECRET_FILE": str(sec_dir / "cipher_sec"),
                "KRATOS_TEMPLATE_FILE": str(template),
                "KRATOS_CONFIG_FILE": str(tmp / "out_kratos.yml"),
                "KRATOS_COOKIE_DOMAIN": ".example.com",
            }

            # 1. Test argv preservation (spaces, flags, metacharacters) and --config replacement
            cmd = [
                "/bin/sh",
                str(entrypoint_path),
                str(fake_kratos),
                "serve",
                "all",
                "--config",
                "/etc/config/kratos/kratos_template.yml",
                "argument with spaces",
                "--custom-flag=hello world",
            ]
            res = subprocess.run(cmd, env=env, capture_output=True, text=True, check=True)
            output = res.stdout
            self.assertIn("ARG:[serve]", output)
            self.assertIn("ARG:[all]", output)
            self.assertIn("ARG:[--config]", output)
            self.assertIn(f"ARG:[{tmp / 'out_kratos.yml'}]", output)
            self.assertIn("ARG:[argument with spaces]", output)
            self.assertIn("ARG:[--custom-flag=hello world]", output)

            # Check generated YAML parsed structure and exact STR types
            out_yaml = (tmp / "out_kratos.yml").read_text()
            parsed_yaml = yaml.safe_load(out_yaml)
            self.assertIsInstance(parsed_yaml["secrets"]["cookie"], list)
            self.assertIsInstance(parsed_yaml["secrets"]["cookie"][0], str)
            self.assertEqual(parsed_yaml["secrets"]["cookie"][0], "cookie_secret_123")
            self.assertIsInstance(parsed_yaml["secrets"]["cipher"], list)
            self.assertIsInstance(parsed_yaml["secrets"]["cipher"][0], str)
            self.assertEqual(parsed_yaml["secrets"]["cipher"][0], "cipher_secret_456")
            self.assertEqual(parsed_yaml["session"]["cookie"]["domain"], ".example.com")

            # Check parsed DSN
            parsed_dsn = urllib.parse.urlsplit(parsed_yaml["dsn"])
            self.assertEqual(urllib.parse.unquote(parsed_dsn.username), "kratos_user")
            self.assertEqual(urllib.parse.unquote(parsed_dsn.password), "db_secret_pass")

            # Check config file mode is 0600
            file_mode = stat.S_IMODE((tmp / "out_kratos.yml").stat().st_mode)
            self.assertEqual(file_mode, 0o600)

            # 2. Test both forms: --config=...
            cmd2 = [
                "/bin/sh",
                str(entrypoint_path),
                str(fake_kratos),
                "migrate",
                f"--config={template}",
            ]
            res2 = subprocess.run(cmd2, env=env, capture_output=True, text=True, check=True)
            self.assertIn(f"ARG:[--config={tmp / 'out_kratos.yml'}]", res2.stdout)

            # 3. Test dangling --config rejected
            cmd_dangling = ["/bin/sh", str(entrypoint_path), str(fake_kratos), "serve", "--config"]
            res_dangling = subprocess.run(cmd_dangling, env=env, capture_output=True, text=True)
            self.assertNotEqual(res_dangling.returncode, 0)
            self.assertIn("missing argument for --config", res_dangling.stderr)

            # 4. Test missing secret failure & sanitized error (no secret leaked)
            env_missing = env.copy()
            env_missing["KRATOS_COOKIE_SECRET_FILE"] = str(sec_dir / "nonexistent")
            res_missing = subprocess.run(cmd, env=env_missing, capture_output=True, text=True)
            self.assertNotEqual(res_missing.returncode, 0)
            self.assertIn("is missing", res_missing.stderr)

            # 5. Test empty secret file failure
            empty_file = sec_dir / "empty"
            empty_file.write_text("")
            env_empty = env.copy()
            env_empty["KRATOS_COOKIE_SECRET_FILE"] = str(empty_file)
            res_empty = subprocess.run(cmd, env=env_empty, capture_output=True, text=True)
            self.assertNotEqual(res_empty.returncode, 0)
            self.assertIn("is empty", res_empty.stderr)

            # 6. Test newline-only secret file failure (critical requirement!)
            newline_file = sec_dir / "newline_only"
            newline_file.write_text("\n\n\r\n")
            env_newline = env.copy()
            env_newline["KRATOS_COOKIE_SECRET_FILE"] = str(newline_file)
            res_newline = subprocess.run(cmd, env=env_newline, capture_output=True, text=True)
            self.assertNotEqual(res_newline.returncode, 0)
            self.assertIn("is empty", res_newline.stderr)

            # 7. Test cookie fixture 'a: b' is parsed as STR, not mapping/dict
            (sec_dir / "mapping_cookie").write_text("a: b\n")
            env_map = env.copy()
            env_map["KRATOS_COOKIE_SECRET_FILE"] = str(sec_dir / "mapping_cookie")
            res_map = subprocess.run(cmd, env=env_map, capture_output=True, text=True, check=True)
            out_map_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            self.assertIsInstance(out_map_yaml["secrets"]["cookie"][0], str)
            self.assertEqual(out_map_yaml["secrets"]["cookie"][0], "a: b")

            # 8. Test DB password fixture with special URI characters 'a@b/c?#d' & user with special chars
            (sec_dir / "special_db_pass").write_text("a@b/c?#d\n")
            env_special_db = env.copy()
            env_special_db["KRATOS_DB_PASSWORD_FILE"] = str(sec_dir / "special_db_pass")
            env_special_db["KRATOS_DB_USER"] = "user:spec"
            res_special_db = subprocess.run(cmd, env=env_special_db, capture_output=True, text=True, check=True)
            out_special_db_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            parsed_special_dsn = urllib.parse.urlsplit(out_special_db_yaml["dsn"])
            self.assertEqual(urllib.parse.unquote(parsed_special_dsn.username), "user:spec")
            self.assertEqual(urllib.parse.unquote(parsed_special_dsn.password), "a@b/c?#d")

            # 9. Test safe substitution with quotes, backslashes, colons, &, |, and non-ASCII UTF-8
            (sec_dir / "complex_cookie").write_text(r'páss"word\with: special & symbols|test')
            env_complex = env.copy()
            env_complex["KRATOS_COOKIE_SECRET_FILE"] = str(sec_dir / "complex_cookie")
            res_complex = subprocess.run(cmd, env=env_complex, capture_output=True, text=True, check=True)
            out_complex_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            self.assertIsInstance(out_complex_yaml["secrets"]["cookie"][0], str)
            self.assertEqual(out_complex_yaml["secrets"]["cookie"][0], r'páss"word\with: special & symbols|test')

            # 10. Test runtime public/dev env parameters substitution
            env_dev = env.copy()
            env_dev["KRATOS_PUBLIC_URL"] = "http://localhost/ory"
            env_dev["KRATOS_DEFAULT_BROWSER_RETURN_URL"] = "http://localhost:4173/sv/"
            res_dev = subprocess.run(cmd, env=env_dev, capture_output=True, text=True, check=True)
            out_dev_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            self.assertEqual(out_dev_yaml["serve"]["public"]["base_url"], "http://localhost/ory")
            self.assertEqual(out_dev_yaml["selfservice"]["default_browser_return_url"], "http://localhost:4173/sv/")

            # 11. Test template modification on second invocation regenerates config
            template.write_text(
                "updated_version: 2\n"
                "secrets:\n"
                "  cookie:\n"
                "    - ${KRATOS_COOKIE_SECRET}\n"
                "  cipher:\n"
                "    - ${KRATOS_CIPHER_SECRET}\n"
                "session:\n"
                "  cookie:\n"
                "    domain: ${KRATOS_COOKIE_DOMAIN}\n"
            )
            subprocess.run(cmd, env=env, capture_output=True, text=True, check=True)
            out_yaml_2 = (tmp / "out_kratos.yml").read_text()
            self.assertIn("updated_version: 2", out_yaml_2)

            # 12. Test KRATOS_COOKIE_DOMAIN set to explicitly empty string renders host-only cookie (domain: "")
            env_empty_cookie = env.copy()
            env_empty_cookie["KRATOS_COOKIE_DOMAIN"] = ""
            res_empty_cookie = subprocess.run(cmd, env=env_empty_cookie, capture_output=True, text=True, check=True)
            out_empty_cookie_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            self.assertEqual(out_empty_cookie_yaml["session"]["cookie"]["domain"], "")

            # 13. Test KRATOS_COOKIE_DOMAIN unset fails when the template renders it
            env_unset_cookie = env.copy()
            env_unset_cookie.pop("KRATOS_COOKIE_DOMAIN", None)
            res_unset_cookie = subprocess.run(cmd, env=env_unset_cookie, capture_output=True, text=True)
            self.assertNotEqual(res_unset_cookie.returncode, 0)
            self.assertIn("KRATOS_COOKIE_DOMAIN", res_unset_cookie.stderr)
            self.assertNotIn("ARG:[", res_unset_cookie.stdout)

            # 14. Test a template without the placeholder needs no cookie domain
            no_domain_template = tmp / "no_domain_template.yml"
            no_domain_template.write_text(
                "dsn: ${DSN}\n"
                "secrets:\n"
                "  cookie:\n"
                "    - ${KRATOS_COOKIE_SECRET}\n"
            )
            env_no_domain = env_unset_cookie.copy()
            env_no_domain["KRATOS_TEMPLATE_FILE"] = str(no_domain_template)
            subprocess.run(cmd, env=env_no_domain, capture_output=True, text=True, check=True)

    # -------------------------------------------------------------------------
    # A04: Frontend HTTPS & AuthHub mTLS Configuration
    # -------------------------------------------------------------------------
    def test_a04_frontend_and_auth_hub_contracts(self):
        """alt-frontend-sv uses https://auth-hub:8443 and NODE_EXTRA_CA_CERTS; auth-hub restricts plaintext."""
        # Frontend in compose/core.yaml
        fe = self.compose_core["services"]["alt-frontend-sv"]
        env_list = fe.get("environment", [])
        self.assertIn("AUTH_HUB_INTERNAL_URL=${AUTH_HUB_INTERNAL_URL:-https://auth-hub:8443}", env_list)
        self.assertIn("NODE_EXTRA_CA_CERTS=/trust/ca-bundle.pem", env_list)
        self.assertIn("pki_trust_bundle:/trust:ro", fe.get("volumes", []))

        # Backend & DataHub in compose/core.yaml
        be = self.compose_core["services"]["alt-backend"]
        self.assertIn("AUTH_HUB_URL=https://auth-hub:9443", be.get("environment", []))

        dh = self.compose_core["services"]["alt-data-hub"]
        self.assertEqual(dh["environment"]["AUTH_HUB_URL"], "https://auth-hub:9443")

        # auth-hub in compose/auth.yaml
        ah = self.compose_auth["services"]["auth-hub"]
        ah_env = ah.get("environment", [])
        self.assertIn("FRONTEND_TLS_LISTEN=true", ah_env)
        self.assertIn("FRONTEND_TLS_PORT=8443", ah_env)
        self.assertIn("FRONTEND_TLS_CERT_FILE=/certs/svc-cert.pem", ah_env)
        self.assertIn("FRONTEND_TLS_KEY_FILE=/certs/svc-key.pem", ah_env)

        # Peer allowlist has search-indexer and knowledge-sovereign, no unused BFF
        allowed_peers_entry = [e for e in ah_env if e.startswith("MTLS_ALLOWED_PEERS=")][0]
        self.assertIn("search-indexer", allowed_peers_entry)
        self.assertIn("knowledge-sovereign", allowed_peers_entry)
        self.assertNotIn("alt-butterfly-facade", allowed_peers_entry)

    # -------------------------------------------------------------------------
    # B04: Augur Ollama 127.0.0.1 Binding
    # -------------------------------------------------------------------------
    def test_b04_augur_ports_bound_to_loopback(self):
        """Augur Ollama services must bind strictly to 127.0.0.1, not 0.0.0.0."""
        augur_ports = self.compose_augur["services"]["knowledge-augur"]["ports"]
        self.assertIn("127.0.0.1:11435:11434", augur_ports)
        self.assertNotIn("11435:11434", augur_ports)

        embedder_ports = self.compose_augur["services"]["knowledge-embedder"]["ports"]
        self.assertIn("127.0.0.1:11436:11434", embedder_ports)
        self.assertNotIn("11436:11434", embedder_ports)

    # -------------------------------------------------------------------------
    # B05: Named Redis Roles & ACL Loader
    # -------------------------------------------------------------------------
    def test_b05_redis_acl_loader_behavioral_fixtures(self):
        """Execute docker/redis/entrypoint.sh in isolated temp environments via /bin/sh."""
        import hashlib
        entrypoint_path = REPO_ROOT / "docker" / "redis" / "entrypoint.sh"

        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            sec_dir = tmp / "secrets"
            sec_dir.mkdir()
            (sec_dir / "streams_pass").write_text("streams_secret_pass\n")
            (sec_dir / "limiter_pass").write_text("limiter_secret_pass\n")
            (sec_dir / "cache_pass").write_text("cache_secret_pass\n")

            fake_redis = tmp / "fake_redis_server"
            fake_redis.write_text(
                "#!/bin/sh\n"
                "printf 'REDIS_ARGS:[%s]\\n' \"$*\"\n"
            )
            fake_redis.chmod(0o755)

            env = {
                "PATH": f"{tmp}:" + os.environ.get("PATH", ""),
                "REDIS_STREAMS_PASSWORD_FILE": str(sec_dir / "streams_pass"),
                "REDIS_LIMITER_PASSWORD_FILE": str(sec_dir / "limiter_pass"),
                "REDIS_CACHE_PASSWORD_FILE": str(sec_dir / "cache_pass"),
                "REDIS_ACL_FILE": str(tmp / "test_users.acl"),
            }

            # 1. Streams role test
            cmd_streams = [
                "/bin/sh",
                str(entrypoint_path),
                "streams",
                "--maxmemory",
                "1gb",
            ]
            # Replace redis-server with fake binary inside script execution by symlinking
            (tmp / "redis-server").symlink_to(fake_redis)

            res_streams = subprocess.run(cmd_streams, env=env, capture_output=True, text=True, check=True)
            self.assertIn(f"--aclfile {tmp / 'test_users.acl'} --maxmemory 1gb", res_streams.stdout)

            acl_content = (tmp / "test_users.acl").read_text()
            self.assertIn("user default off -@all", acl_content)

            streams_hash = hashlib.sha256(b"streams_secret_pass").hexdigest()
            limiter_hash = hashlib.sha256(b"limiter_secret_pass").hexdigest()
            self.assertIn(f"user streams on #{streams_hash} resetkeys ~alt:events:* ~alt:replies:tags:* -@all +hello +client|setinfo +client|setname +ping +xadd +xread +xreadgroup +xack +xautoclaim +xpending +xgroup|create +xinfo|stream +xinfo|groups +xtrim +expire +ttl +scan +del", acl_content)
            self.assertIn(f"user limiter on #{limiter_hash} resetkeys ~host_rate_limiter:v1:* -@all +hello +client|setinfo +client|setname +ping +set +pttl +select", acl_content)
            self.assertNotIn("streams_secret_pass", acl_content)
            self.assertNotIn("limiter_secret_pass", acl_content)

            # ACL file permissions must be 0600
            file_mode = stat.S_IMODE((tmp / "test_users.acl").stat().st_mode)
            self.assertEqual(file_mode, 0o600)

            # 2. Cache role test
            cmd_cache = [
                "/bin/sh",
                str(entrypoint_path),
                "cache",
                "--maxmemory",
                "256mb",
            ]
            res_cache = subprocess.run(cmd_cache, env=env, capture_output=True, text=True, check=True)
            self.assertIn(f"--aclfile {tmp / 'test_users.acl'} --maxmemory 256mb", res_cache.stdout)

            acl_cache = (tmp / "test_users.acl").read_text()
            self.assertIn("user default off -@all", acl_cache)
            cache_hash = hashlib.sha256(b"cache_secret_pass").hexdigest()
            self.assertIn(f"user cache on #{cache_hash} resetkeys ~recap_card:* ~recap:summary:* -@all +hello +client|setinfo +client|setname +ping +get +set +del", acl_cache)
            self.assertNotIn("cache_secret_pass", acl_cache)
            self.assertNotIn("user streams", acl_cache)

            # 3. Missing secret fails
            env_err = env.copy()
            env_err["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "nonexistent")
            res_err = subprocess.run(cmd_streams, env=env_err, capture_output=True, text=True)
            self.assertNotEqual(res_err.returncode, 0)
            self.assertIn("is missing", res_err.stderr)

            # 4. Malicious space fixture 'fixture nopass' must be denied (exit non-zero, no nopass directive in ACL)
            (sec_dir / "malicious_nopass").write_text("fixture nopass\n")
            env_nopass = env.copy()
            env_nopass["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_nopass")
            res_nopass = subprocess.run(cmd_streams, env=env_nopass, capture_output=True, text=True)
            self.assertNotEqual(res_nopass.returncode, 0)
            self.assertNotIn("fixture nopass", res_nopass.stderr)
            # Ensure ACL file does not contain nopass directive
            if (tmp / "test_users.acl").exists():
                self.assertNotIn("nopass", (tmp / "test_users.acl").read_text())

            # 5. Tab character injection must be denied
            (sec_dir / "malicious_tab").write_text("tab\tinjection\n")
            env_tab = env.copy()
            env_tab["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_tab")
            res_tab = subprocess.run(cmd_streams, env=env_tab, capture_output=True, text=True)
            self.assertNotEqual(res_tab.returncode, 0)
            self.assertNotIn("tab\tinjection", res_tab.stderr)

            # 6. Control character injection must be denied
            (sec_dir / "malicious_ctrl").write_bytes(b"ctrl\x01pass\n")
            env_ctrl = env.copy()
            env_ctrl["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_ctrl")
            res_ctrl = subprocess.run(cmd_streams, env=env_ctrl, capture_output=True, text=True)
            self.assertNotEqual(res_ctrl.returncode, 0)

            # 7. Malformed UTF-8 must be denied
            (sec_dir / "malicious_utf8").write_bytes(b"\xff\xfe\xfd\n")
            env_utf8 = env.copy()
            env_utf8["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_utf8")
            res_utf8 = subprocess.run(cmd_streams, env=env_utf8, capture_output=True, text=True)
            self.assertNotEqual(res_utf8.returncode, 0)

            # 8. Blank / whitespace-only secret must be denied
            (sec_dir / "malicious_blank").write_text("   \t  \n")
            env_blank = env.copy()
            env_blank["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_blank")
            res_blank = subprocess.run(cmd_streams, env=env_blank, capture_output=True, text=True)
            self.assertNotEqual(res_blank.returncode, 0)

            # 9. Canonical UTF-8 scalar ranges: overlong / surrogate / out-of-Unicode MUST be denied
            # 9a. Overlong 3-byte sequence E0 80 80
            (sec_dir / "malicious_overlong").write_bytes(b"\xe0\x80\x80\n")
            env_overlong = env.copy()
            env_overlong["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_overlong")
            res_overlong = subprocess.run(cmd_streams, env=env_overlong, capture_output=True, text=True)
            self.assertNotEqual(res_overlong.returncode, 0, "E0 80 80 overlong must be rejected")
            self.assertNotIn("malicious_overlong", res_overlong.stderr)
            self.assertNotIn("\xe0\x80\x80", res_overlong.stderr)

            # 9b. UTF-16 surrogate ED A0 80
            (sec_dir / "malicious_surrogate").write_bytes(b"\xed\xa0\x80\n")
            env_surrogate = env.copy()
            env_surrogate["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_surrogate")
            res_surrogate = subprocess.run(cmd_streams, env=env_surrogate, capture_output=True, text=True)
            self.assertNotEqual(res_surrogate.returncode, 0, "ED A0 80 surrogate must be rejected")
            self.assertNotIn("malicious_surrogate", res_surrogate.stderr)
            self.assertNotIn("\xed\xa0\x80", res_surrogate.stderr)

            # 9c. Out of Unicode F4 90 80 80
            (sec_dir / "malicious_out_of_unicode").write_bytes(b"\xf4\x90\x80\x80\n")
            env_oou = env.copy()
            env_oou["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_out_of_unicode")
            res_oou = subprocess.run(cmd_streams, env=env_oou, capture_output=True, text=True)
            self.assertNotEqual(res_oou.returncode, 0, "F4 90 80 80 out-of-Unicode must be rejected")
            self.assertNotIn("malicious_out_of_unicode", res_oou.stderr)
            self.assertNotIn("\xf4\x90\x80\x80", res_oou.stderr)

            # 9d. Overlong 4-byte sequence F0 80 80 80
            (sec_dir / "malicious_overlong_4").write_bytes(b"\xf0\x80\x80\x80\n")
            env_ol4 = env.copy()
            env_ol4["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_overlong_4")
            res_ol4 = subprocess.run(cmd_streams, env=env_ol4, capture_output=True, text=True)
            self.assertNotEqual(res_ol4.returncode, 0, "F0 80 80 80 overlong must be rejected")

            # 10. Valid non-ASCII UTF-8 sequences MUST be accepted with exact raw byte sha256 hash
            # 10a. Valid non-ASCII text with German umlauts and Japanese kanji under UTF-8 locale
            valid_utf8_pw = "p\xc3\xa1ss_日本語_123"
            (sec_dir / "valid_utf8").write_text(valid_utf8_pw + "\n", encoding="utf-8")
            env_valid_utf8 = env.copy()
            env_valid_utf8["LC_ALL"] = "C.UTF-8"
            env_valid_utf8["LANG"] = "C.UTF-8"
            env_valid_utf8["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "valid_utf8")
            res_valid_utf8 = subprocess.run(cmd_streams, env=env_valid_utf8, capture_output=True, text=True, check=True)
            valid_utf8_hash = hashlib.sha256(valid_utf8_pw.encode("utf-8")).hexdigest()
            acl_valid_utf8 = (tmp / "test_users.acl").read_text()
            self.assertIn(f"user streams on #{valid_utf8_hash}", acl_valid_utf8)
            self.assertNotIn(valid_utf8_pw, acl_valid_utf8)

            # 10b. Valid boundary UTF-8 sequences: U+0800 (E0 A0 80), U+D7FF (ED 9F BF), U+10FFFF (F4 8F BF BF), emoji (F0 9F 8E 89)
            valid_boundary_pw = b"\xe0\xa0\x80_\xed\x9f\xbf_\xf4\x8f\xbf\xbf_\xf0\x9f\x8e\x89"
            (sec_dir / "valid_boundary").write_bytes(valid_boundary_pw + b"\n")
            env_valid_boundary = env.copy()
            env_valid_boundary["LC_ALL"] = "C.UTF-8"
            env_valid_boundary["LANG"] = "C.UTF-8"
            env_valid_boundary["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "valid_boundary")
            res_valid_boundary = subprocess.run(cmd_streams, env=env_valid_boundary, capture_output=True, text=True, check=True)
            valid_boundary_hash = hashlib.sha256(valid_boundary_pw).hexdigest()
            acl_valid_boundary = (tmp / "test_users.acl").read_text()
            self.assertIn(f"user streams on #{valid_boundary_hash}", acl_valid_boundary)

            # 10c. Native Japanese + emoji combined secret under UTF-8 locale ensures exact raw UTF-8 SHA256 hash in ACL
            valid_jp_emoji_pw = "東京_🗼_富士山_🗻_2026!secret"
            (sec_dir / "valid_jp_emoji").write_text(valid_jp_emoji_pw + "\r\n", encoding="utf-8")
            env_valid_jp_emoji = env.copy()
            env_valid_jp_emoji["LC_ALL"] = "C.UTF-8"
            env_valid_jp_emoji["LANG"] = "C.UTF-8"
            env_valid_jp_emoji["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "valid_jp_emoji")
            res_valid_jp_emoji = subprocess.run(cmd_streams, env=env_valid_jp_emoji, capture_output=True, text=True, check=True)
            valid_jp_emoji_hash = hashlib.sha256(valid_jp_emoji_pw.encode("utf-8")).hexdigest()
            acl_valid_jp_emoji = (tmp / "test_users.acl").read_text()
            self.assertIn(f"user streams on #{valid_jp_emoji_hash}", acl_valid_jp_emoji)
            self.assertNotIn(valid_jp_emoji_pw, acl_valid_jp_emoji)

            # 10d. trimONEallowedtrailinglineend: multiple trailing newlines must be rejected
            (sec_dir / "malicious_multiline_end").write_text("validpass\n\n", encoding="utf-8")
            env_multiline = env.copy()
            env_multiline["REDIS_STREAMS_PASSWORD_FILE"] = str(sec_dir / "malicious_multiline_end")
            res_multiline = subprocess.run(cmd_streams, env=env_multiline, capture_output=True, text=True)
            self.assertNotEqual(res_multiline.returncode, 0, "multiple trailing newlines must be rejected")

    # -------------------------------------------------------------------------
    # Dev Helper start-dev.sh Contracts
    # -------------------------------------------------------------------------
    def test_start_dev_script_contracts(self):
        """start-dev.sh must use compose/compose.yaml and compose/compose.dev.yaml, and not generate kratos.yml."""
        start_dev_content = (REPO_ROOT / "for-development" / "start-dev.sh").read_text(encoding="utf-8")
        self.assertNotIn("-f compose.yaml -f compose.dev.yaml", start_dev_content)
        self.assertIn("-f compose/compose.yaml -f compose/compose.dev.yaml", start_dev_content)
        self.assertNotIn("kratos/kratos.yml", start_dev_content)

    def test_b05_compose_callers_and_healthchecks(self):
        """Verify all Redis callers and healthchecks use named roles and separate secrets."""
        # redis-streams
        rs = self.compose_mq["services"]["redis-streams"]
        self.assertEqual(rs["entrypoint"], ["/entrypoint.sh", "streams"])
        self.assertIn("redis_streams_password", rs["secrets"])
        self.assertIn("redis_limiter_password", rs["secrets"])
        self.assertIn("--user streams", rs["healthcheck"]["test"][1])

        # mq-hub
        mq = self.compose_mq["services"]["mq-hub"]
        self.assertIn("REDIS_URL=redis://streams@redis-streams:6379", mq["environment"])
        self.assertIn("REDIS_PASSWORD_FILE=/run/secrets/redis_streams_password", mq["environment"])
        self.assertIn("redis_streams_password", mq["secrets"])

        # redis-cache
        rc = self.compose_ai["services"]["redis-cache"]
        self.assertEqual(rc["entrypoint"], ["/entrypoint.sh", "cache"])
        self.assertIn("redis_cache_password", rc["secrets"])
        self.assertIn("--user cache", rc["healthcheck"]["test"][1])

        # news-creator
        nc = self.compose_ai["services"]["news-creator"]
        self.assertIn("CACHE_REDIS_URL=redis://cache@redis-cache:6379/0", nc["environment"])
        self.assertIn("REDIS_PASSWORD_FILE=/run/secrets/redis_cache_password", nc["environment"])
        self.assertIn("redis_cache_password", nc["secrets"])

        # pre-processor
        pp = self.compose_ai["services"]["pre-processor"]
        self.assertIn("REDIS_STREAMS_URL=redis://streams@redis-streams:6379", pp["environment"])
        self.assertIn("REDIS_PASSWORD_FILE=/run/secrets/redis_streams_password", pp["environment"])
        self.assertIn("redis_streams_password", pp["secrets"])

        # search-indexer
        si = self.compose_workers["services"]["search-indexer"]
        self.assertIn("REDIS_STREAMS_URL=redis://streams@redis-streams:6379", si["environment"])
        self.assertIn("REDIS_PASSWORD_FILE=/run/secrets/redis_streams_password", si["environment"])
        self.assertIn("redis_streams_password", si["secrets"])

        # tag-generator
        tg = self.compose_workers["services"]["tag-generator"]
        self.assertIn("REDIS_STREAMS_URL=redis://streams@redis-streams:6379", tg["environment"])
        self.assertIn("REDIS_PASSWORD_FILE=/run/secrets/redis_streams_password", tg["environment"])
        self.assertIn("redis_streams_password", tg["secrets"])

        # backend limiter
        be = self.compose_core["services"]["alt-backend"]
        self.assertIn("HOST_RATE_LIMITER_REDIS_URL=redis://limiter@redis-streams:6379/3", be["environment"])
        self.assertIn("HOST_RATE_LIMITER_REDIS_PASSWORD_FILE=/run/secrets/redis_limiter_password", be["environment"])
        self.assertIn("redis_limiter_password", be["secrets"])

        # harvester limiter
        hv = self.compose_core["services"]["alt-harvester"]
        self.assertEqual(hv["environment"]["HOST_RATE_LIMITER_REDIS_URL"], "redis://limiter@redis-streams:6379/3")
        self.assertEqual(hv["environment"]["HOST_RATE_LIMITER_REDIS_PASSWORD_FILE"], "/run/secrets/redis_limiter_password")
        self.assertIn("redis_limiter_password", hv["secrets"])

    # -------------------------------------------------------------------------
    # ClickHouse Direct Backup & db.yaml
    # -------------------------------------------------------------------------
    def test_clickhouse_direct_backup_config(self):
        """ClickHouse in compose/db.yaml configures backup_disk and mounts /backups/clickhouse."""
        ch = self.compose_db["services"]["clickhouse"]
        volumes = ch.get("volumes", [])
        backup_mount = next(
            (
                v for v in volumes
                if (v == "../backups/clickhouse:/backups/clickhouse" or
                    (isinstance(v, dict) and v.get("target") == "/backups/clickhouse"))
            ),
            None,
        )
        self.assertIsNotNone(backup_mount, "Missing /backups/clickhouse volume mount in clickhouse service")
        if isinstance(backup_mount, dict):
            self.assertEqual(backup_mount.get("type"), "bind")
            self.assertEqual(
                backup_mount.get("source"),
                "${CLICKHOUSE_BACKUP_HOST_PATH:-/var/lib/alt-clickhouse-backups}",
            )
            # Writable backup directory
            self.assertFalse(backup_mount.get("read_only", False))
            self.assertFalse(backup_mount.get("bind", {}).get("create_host_path", True))

        configs = ch.get("configs", [])
        backup_cfg = [c for c in configs if c.get("source") == "clickhouse_backup_disk"]
        self.assertTrue(len(backup_cfg) > 0, "Missing clickhouse_backup_disk config mount in clickhouse service")
        self.assertEqual(backup_cfg[0]["target"], "/etc/clickhouse-server/config.d/backup_disk.xml")

        # Top-level configs definition
        top_configs = self.compose_db.get("configs", {})
        self.assertIn("clickhouse_backup_disk", top_configs)
        self.assertEqual(top_configs["clickhouse_backup_disk"]["file"], "../clickhouse/config/backup_disk.xml")

    # -------------------------------------------------------------------------
    # Base Secret Declarations
    # -------------------------------------------------------------------------
    @staticmethod
    def _environment(service):
        env = service.get("environment", {})
        if isinstance(env, dict):
            return env
        return dict(item.split("=", 1) for item in env if "=" in item)

    def test_mqhub_broker_and_enabled_clients_share_generated_secret(self):
        """Every enabled broker client must authenticate with the broker's file secret."""
        secret_name = "mqhub_auth_token"
        token_path = f"/run/secrets/{secret_name}"
        self.assertEqual(
            self.compose_base["secrets"].get(secret_name),
            {"file": "../secrets/mqhub_auth_token.txt"},
        )
        services = {}
        for document in (self.compose_core, self.compose_ai, self.compose_workers, self.compose_mq):
            services.update(document["services"])
        enabled = {
            name for name, service in services.items()
            if str(self._environment(service).get("MQHUB_ENABLED", "false")).lower() == "true"
        }
        self.assertEqual(enabled, {"alt-backend", "alt-harvester", "alt-data-hub"})
        for name in enabled | {"mq-hub"}:
            with self.subTest(service=name):
                service = services[name]
                environment = self._environment(service)
                self.assertEqual(environment.get("MQHUB_AUTH_TOKEN_FILE"), token_path)
                self.assertNotIn("MQHUB_AUTH_TOKEN", environment)
                self.assertIn(secret_name, service.get("secrets", []))

    def test_search_introspection_uses_own_leaf_and_auth_hub_mtls_route(self):
        """Search's mandatory introspection URL must use the authorized mTLS listener."""
        service = self.compose_workers["services"]["search-indexer"]
        env = self._environment(service)
        self.assertEqual(
            env.get("USER_JWT_INTROSPECTION_URL"),
            "https://auth-hub:9443/internal/token/introspect",
        )
        self.assertEqual(env["CERT_SUBJECT"], "search-indexer")
        self.assertEqual(env["MTLS_CERT_FILE"], "/certs/svc-cert.pem")
        self.assertEqual(env["MTLS_KEY_FILE"], "/certs/svc-key.pem")
        self.assertEqual(env["MTLS_CA_FILE"], "/trust/ca-bundle.pem")
        self.assertIn("search_indexer_certs:/certs", service["volumes"])
        self.assertIn("pki_trust_bundle:/trust:ro", service["volumes"])
        auth_env = self._environment(self.compose_auth["services"]["auth-hub"])
        self.assertEqual(auth_env["MTLS_PORT"], "9443")
        peers = auth_env["MTLS_ALLOWED_PEERS"].split(":-", 1)[1].removesuffix("}").split(",")
        self.assertIn("search-indexer", peers)

    def test_search_loads_mounted_inference_token_for_embedder_api_key(self):
        """A mounted token without its loader environment key yields an empty embedder API key."""
        service = self.compose_workers["services"]["search-indexer"]
        self.assertEqual(
            self._environment(service).get("INFERENCE_SERVICE_TOKEN_FILE"),
            "/run/secrets/inference_service_token",
        )
        self.assertIn("inference_service_token", service["secrets"])

    def test_base_yaml_secrets_declarations(self):
        """compose/base.yaml declares new named role passwords and tokens."""
        secrets = self.compose_base.get("secrets", {})
        expected_new_secrets = [
            "redis_streams_password",
            "redis_cache_password",
            "redis_limiter_password",
            "rask_ingest_token",
            "inference_service_token",
            "k6_api_token",
        ]
        for s in expected_new_secrets:
            self.assertIn(s, secrets, f"compose/base.yaml missing secret {s}")
            self.assertEqual(secrets[s]["file"], f"../secrets/{s}.txt")


if __name__ == "__main__":
    unittest.main()
