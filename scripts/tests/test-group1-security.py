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

import hashlib
import os
import re
import stat
import subprocess
import tempfile
import unittest
import urllib.parse
from pathlib import Path
from typing import Any, ClassVar

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
OWNER_RW_FILE_MODE = 0o600


class TestGroup1Security(unittest.TestCase):
    def setUp(self) -> None:
        self.compose_core = self._load_yaml(REPO_ROOT / "compose" / "core.yaml")
        self.compose_auth = self._load_yaml(REPO_ROOT / "compose" / "auth.yaml")
        self.compose_mq = self._load_yaml(REPO_ROOT / "compose" / "mq.yaml")
        self.compose_ai = self._load_yaml(REPO_ROOT / "compose" / "ai.yaml")
        self.compose_workers = self._load_yaml(REPO_ROOT / "compose" / "workers.yaml")
        self.compose_augur = self._load_yaml(REPO_ROOT / "compose.augur.yaml")
        self.compose_db = self._load_yaml(REPO_ROOT / "compose" / "db.yaml")
        self.compose_base = self._load_yaml(REPO_ROOT / "compose" / "base.yaml")

    def _load_yaml(self, path: Path) -> dict[str, Any]:
        with path.open(encoding="utf-8") as f:
            return yaml.safe_load(f)

    # -------------------------------------------------------------------------
    # A02: Plecto Proxy Mounts
    # -------------------------------------------------------------------------
    def test_a02_plecto_mounts(self) -> None:
        """Plecto mounts ONLY manifest.toml, dev-key.pub, artifacts/stale-chunk-heal, and public trust CA."""
        plecto_service = self.compose_core["services"]["plecto-proxy"]
        volumes = plecto_service.get("volumes", [])

        # Must not mount the whole ../plecto directory
        for v in volumes:
            if isinstance(v, str):
                assert v != "../plecto:/etc/plecto:ro", v
                assert not v.startswith("../plecto:/"), v
                # Must not mount private key
                assert "dev-key:" not in v, v
                assert "dev-key.pem" not in v, v
            elif isinstance(v, dict):
                src = str(v.get("source", ""))
                # Forbid mounting whole ../plecto directory regardless of target
                assert src != "../plecto", src
                assert src != "../plecto/", src
                assert not src.startswith("../plecto:/"), src
                # Must not mount private key
                assert "dev-key:" not in src, src
                assert "dev-key.pem" not in src, src

        # Expected mounts: either legacy short string or long-form bind dict with read_only=True
        manifest_mount = next(
            (
                v
                for v in volumes
                if (
                    v == "../plecto/manifest.toml:/etc/plecto/manifest.toml:ro"
                    or (isinstance(v, dict) and v.get("target") == "/etc/plecto/manifest.toml")
                )
            ),
            None,
        )
        assert manifest_mount is not None, "Expected manifest.toml mount in plecto-proxy"
        if isinstance(manifest_mount, dict):
            assert manifest_mount.get("type") == "bind", manifest_mount
            assert manifest_mount.get("source") == "../plecto/manifest.toml", manifest_mount
            assert manifest_mount.get("read_only") is True, manifest_mount
            assert not manifest_mount.get("bind", {}).get("create_host_path", True), manifest_mount

        key_mount = next(
            (
                v
                for v in volumes
                if (
                    v == "../plecto/.plecto/dev-key.pub:/etc/plecto/.plecto/dev-key.pub:ro"
                    or (
                        isinstance(v, dict) and v.get("target") == "/etc/plecto/.plecto/dev-key.pub"
                    )
                )
            ),
            None,
        )
        assert key_mount is not None, "Expected dev-key.pub mount in plecto-proxy"
        if isinstance(key_mount, dict):
            assert key_mount.get("type") == "bind", key_mount
            assert (
                key_mount.get("source")
                == "${PLECTO_PUBLIC_KEY_HOST_PATH:-/var/lib/alt-plecto/dev-key.pub}"
            ), key_mount
            assert key_mount.get("read_only") is True, key_mount
            assert not key_mount.get("bind", {}).get("create_host_path", True), key_mount

        artifacts_mount = next(
            (
                v
                for v in volumes
                if (
                    v
                    == "../plecto/artifacts/stale-chunk-heal:/etc/plecto/artifacts/stale-chunk-heal:ro"
                    or (
                        isinstance(v, dict)
                        and v.get("target") == "/etc/plecto/artifacts/stale-chunk-heal"
                    )
                )
            ),
            None,
        )
        assert artifacts_mount is not None, (
            "Expected artifacts/stale-chunk-heal mount in plecto-proxy"
        )
        if isinstance(artifacts_mount, dict):
            assert artifacts_mount.get("type") == "bind", artifacts_mount
            assert (
                artifacts_mount.get("source")
                == "${PLECTO_ARTIFACTS_HOST_PATH:-/var/lib/alt-plecto/artifacts/stale-chunk-heal}"
            ), artifacts_mount
            assert artifacts_mount.get("read_only") is True, artifacts_mount
            assert not artifacts_mount.get("bind", {}).get("create_host_path", True), (
                artifacts_mount
            )

        assert "pki_trust_bundle:/trust:ro" in volumes, (
            "Expected volume mount pki_trust_bundle:/trust:ro in plecto-proxy"
        )

        # Verify artifacts directory structure on disk if staged in checkout or operator path
        artifacts_source = Path(
            os.environ.get(
                "PLECTO_ARTIFACTS_HOST_PATH", "/var/lib/alt-plecto/artifacts/stale-chunk-heal"
            )
        )
        for candidate in [
            REPO_ROOT / "plecto" / "artifacts" / "stale-chunk-heal",
            artifacts_source,
        ]:
            if candidate.is_dir():
                assert (candidate / "blobs").is_dir(), f"{candidate}/blobs directory missing"
                assert (candidate / "index.json").is_file(), f"{candidate}/index.json missing"
                assert (candidate / "oci-layout").is_file(), f"{candidate}/oci-layout missing"
                break

    # -------------------------------------------------------------------------
    # A03: Kratos Entrypoint & Canonical Template
    # -------------------------------------------------------------------------
    def test_a03_kratos_template_and_compose_wiring(self) -> None:
        """kratos_template.yml is authority, compose uses it and mounts all secrets."""
        template_content = (REPO_ROOT / "kratos" / "kratos_template.yml").read_text(
            encoding="utf-8"
        )
        assert "${KRATOS_COOKIE_SECRET}" in template_content, template_content
        assert "${KRATOS_CIPHER_SECRET}" in template_content, template_content
        assert "0123456789abcdef0123456789abcdef" not in template_content, template_content

        # kratos-migrate in compose/auth.yaml
        km = self.compose_auth["services"]["kratos-migrate"]
        assert "--config" in km["command"], km["command"]
        cfg_idx = km["command"].index("--config")
        assert km["command"][cfg_idx + 1] == "/etc/config/kratos/kratos_template.yml", km["command"]
        assert "kratos_db_password" in km["secrets"], km["secrets"]
        assert "kratos_cookie_secret" in km["secrets"], km["secrets"]
        assert "kratos_cipher_secret" in km["secrets"], km["secrets"]

        # kratos in compose/auth.yaml
        ks = self.compose_auth["services"]["kratos"]
        assert "--config" in ks["command"], ks["command"]
        cfg_idx = ks["command"].index("--config")
        assert ks["command"][cfg_idx + 1] == "/etc/config/kratos/kratos_template.yml", ks["command"]

    def _assert_kratos_argv_and_yaml(
        self,
        entrypoint_path: Path,
        fake_kratos: Path,
        template: Path,
        tmp: Path,
        env: dict[str, str],
    ) -> list[str]:
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
        res = subprocess.run(cmd, env=env, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
        output = res.stdout
        assert "ARG:[serve]" in output, output
        assert "ARG:[all]" in output, output
        assert "ARG:[--config]" in output, output
        assert f"ARG:[{tmp / 'out_kratos.yml'}]" in output, output
        assert "ARG:[argument with spaces]" in output, output
        assert "ARG:[--custom-flag=hello world]" in output, output

        # Check generated YAML parsed structure and exact STR types
        out_yaml = (tmp / "out_kratos.yml").read_text()
        parsed_yaml = yaml.safe_load(out_yaml)
        assert isinstance(parsed_yaml["secrets"]["cookie"], list), parsed_yaml["secrets"]["cookie"]
        assert isinstance(parsed_yaml["secrets"]["cookie"][0], str), parsed_yaml["secrets"][
            "cookie"
        ]
        assert parsed_yaml["secrets"]["cookie"][0] == "cookie_secret_1234567890_abcdefghij", (
            parsed_yaml["secrets"]["cookie"]
        )
        assert isinstance(parsed_yaml["secrets"]["cipher"], list), parsed_yaml["secrets"]["cipher"]
        assert isinstance(parsed_yaml["secrets"]["cipher"][0], str), parsed_yaml["secrets"][
            "cipher"
        ]
        assert parsed_yaml["secrets"]["cipher"][0] == "cipher_secret_4567890123_abcdefghij", (
            parsed_yaml["secrets"]["cipher"]
        )
        assert parsed_yaml["session"]["cookie"]["domain"] == ".example.com", parsed_yaml["session"]

        # Check parsed DSN
        parsed_dsn = urllib.parse.urlsplit(parsed_yaml["dsn"])
        assert urllib.parse.unquote(parsed_dsn.username) == "kratos_user", parsed_dsn
        assert urllib.parse.unquote(parsed_dsn.password) == "db_secret_pass", parsed_dsn

        # Check config file mode is 0600
        file_mode = stat.S_IMODE((tmp / "out_kratos.yml").stat().st_mode)
        assert file_mode == OWNER_RW_FILE_MODE, oct(file_mode)

        # 2. Test both forms: --config=...
        cmd2 = [
            "/bin/sh",
            str(entrypoint_path),
            str(fake_kratos),
            "migrate",
            f"--config={template}",
        ]
        res2 = subprocess.run(cmd2, env=env, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
        assert f"ARG:[--config={tmp / 'out_kratos.yml'}]" in res2.stdout, res2.stdout
        return cmd

    def _assert_kratos_secret_errors(
        self,
        entrypoint_path: Path,
        fake_kratos: Path,
        cmd: list[str],
        sec_dir: Path,
        env: dict[str, str],
    ) -> None:
        # 3. Test dangling --config rejected
        cmd_dangling = ["/bin/sh", str(entrypoint_path), str(fake_kratos), "serve", "--config"]
        res_dangling = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_dangling, env=env, capture_output=True, text=True, check=False
        )
        assert res_dangling.returncode != 0, res_dangling.stdout
        assert "missing argument for --config" in res_dangling.stderr, res_dangling.stderr

        # 4. Test missing secret failure & sanitized error (no secret leaked)
        env_missing = env.copy()
        env_missing["KRATOS_COOKIE_SECRET_FILE"] = str(sec_dir / "nonexistent")
        res_missing = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd, env=env_missing, capture_output=True, text=True, check=False
        )
        assert res_missing.returncode != 0, res_missing.stdout
        assert "is missing" in res_missing.stderr, res_missing.stderr

        # 5. Test empty secret file failure
        empty_file = sec_dir / "empty"
        empty_file.write_text("")
        env_empty = env.copy()
        env_empty["KRATOS_COOKIE_SECRET_FILE"] = str(empty_file)
        res_empty = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd, env=env_empty, capture_output=True, text=True, check=False
        )
        assert res_empty.returncode != 0, res_empty.stdout
        assert "is empty" in res_empty.stderr, res_empty.stderr

        # 6. Test newline-only secret file failure (critical requirement!)
        newline_file = sec_dir / "newline_only"
        newline_file.write_text("\n\n\r\n")
        env_newline = env.copy()
        env_newline["KRATOS_COOKIE_SECRET_FILE"] = str(newline_file)
        res_newline = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd, env=env_newline, capture_output=True, text=True, check=False
        )
        assert res_newline.returncode != 0, res_newline.stdout
        assert "is empty" in res_newline.stderr, res_newline.stderr

    def _assert_kratos_secret_encodings(
        self,
        cmd: list[str],
        sec_dir: Path,
        tmp: Path,
        env: dict[str, str],
    ) -> None:
        # 7. Test cookie fixture 'a: b...' is parsed as STR, not mapping/dict
        mapping_cookie_val = "a: b_padding_to_32_characters_long_secret"
        (sec_dir / "mapping_cookie").write_text(mapping_cookie_val + "\n")
        env_map = env.copy()
        env_map["KRATOS_COOKIE_SECRET_FILE"] = str(sec_dir / "mapping_cookie")
        subprocess.run(cmd, env=env_map, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
        out_map_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
        assert isinstance(out_map_yaml["secrets"]["cookie"][0], str), out_map_yaml["secrets"]
        assert out_map_yaml["secrets"]["cookie"][0] == mapping_cookie_val, out_map_yaml["secrets"]

        # 8. Test DB password fixture with special URI characters 'a@b/c?#d' & user with special chars
        (sec_dir / "special_db_pass").write_text("a@b/c?#d\n")
        env_special_db = env.copy()
        env_special_db["KRATOS_DB_PASSWORD_FILE"] = str(sec_dir / "special_db_pass")
        env_special_db["KRATOS_DB_USER"] = "user:spec"
        subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd, env=env_special_db, capture_output=True, text=True, check=True
        )
        out_special_db_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
        parsed_special_dsn = urllib.parse.urlsplit(out_special_db_yaml["dsn"])
        assert urllib.parse.unquote(parsed_special_dsn.username) == "user:spec", parsed_special_dsn
        assert urllib.parse.unquote(parsed_special_dsn.password) == "a@b/c?#d", parsed_special_dsn

        # 9. Test safe substitution with quotes, backslashes, colons, &, |, and non-ASCII UTF-8
        (sec_dir / "complex_cookie").write_text(r'páss"word\with: special & symbols|test')
        env_complex = env.copy()
        env_complex["KRATOS_COOKIE_SECRET_FILE"] = str(sec_dir / "complex_cookie")
        subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd, env=env_complex, capture_output=True, text=True, check=True
        )
        out_complex_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
        assert isinstance(out_complex_yaml["secrets"]["cookie"][0], str), out_complex_yaml[
            "secrets"
        ]
        assert (
            out_complex_yaml["secrets"]["cookie"][0] == r'páss"word\with: special & symbols|test'
        ), out_complex_yaml["secrets"]

        # 10. Test runtime public/dev env parameters substitution
        env_dev = env.copy()
        env_dev["KRATOS_PUBLIC_URL"] = "http://localhost/ory"
        env_dev["KRATOS_DEFAULT_BROWSER_RETURN_URL"] = "http://localhost:4173/sv/"
        subprocess.run(cmd, env=env_dev, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
        out_dev_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
        assert out_dev_yaml["serve"]["public"]["base_url"] == "http://localhost/ory", out_dev_yaml
        assert (
            out_dev_yaml["selfservice"]["default_browser_return_url"] == "http://localhost:4173/sv/"
        ), out_dev_yaml

    def _assert_kratos_template_and_cookie_domain(
        self,
        cmd: list[str],
        template: Path,
        tmp: Path,
        env: dict[str, str],
    ) -> None:
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
        subprocess.run(cmd, env=env, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
        out_yaml_2 = (tmp / "out_kratos.yml").read_text()
        assert "updated_version: 2" in out_yaml_2, out_yaml_2

        # 12. Test KRATOS_COOKIE_DOMAIN set to explicitly empty string renders host-only cookie (domain: "")
        env_empty_cookie = env.copy()
        env_empty_cookie["KRATOS_COOKIE_DOMAIN"] = ""
        subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd, env=env_empty_cookie, capture_output=True, text=True, check=True
        )
        out_empty_cookie_yaml = yaml.safe_load((tmp / "out_kratos.yml").read_text())
        assert out_empty_cookie_yaml["session"]["cookie"]["domain"] == "", out_empty_cookie_yaml

        # 13. Test KRATOS_COOKIE_DOMAIN unset fails when the template renders it
        env_unset_cookie = env.copy()
        env_unset_cookie.pop("KRATOS_COOKIE_DOMAIN", None)
        res_unset_cookie = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd, env=env_unset_cookie, capture_output=True, text=True, check=False
        )
        assert res_unset_cookie.returncode != 0, res_unset_cookie.stdout
        assert "KRATOS_COOKIE_DOMAIN" in res_unset_cookie.stderr, res_unset_cookie.stderr
        assert "ARG:[" not in res_unset_cookie.stdout, res_unset_cookie.stdout

        # 14. Test a template without the placeholder needs no cookie domain
        no_domain_template = tmp / "no_domain_template.yml"
        no_domain_template.write_text(
            "dsn: ${DSN}\nsecrets:\n  cookie:\n    - ${KRATOS_COOKIE_SECRET}\n"
        )
        env_no_domain = env_unset_cookie.copy()
        env_no_domain["KRATOS_TEMPLATE_FILE"] = str(no_domain_template)
        subprocess.run(cmd, env=env_no_domain, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test

    def test_a03_kratos_entrypoint_behavioral_fixtures(self) -> None:
        """Execute kratos/entrypoint.sh in isolated temp environments via /bin/sh."""
        entrypoint_path = REPO_ROOT / "kratos" / "entrypoint.sh"

        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            sec_dir = tmp / "secrets"
            sec_dir.mkdir()
            (sec_dir / "db_pass").write_text("db_secret_pass\n")
            (sec_dir / "cookie_sec").write_text("cookie_secret_1234567890_abcdefghij\n")
            (sec_dir / "cipher_sec").write_text("cipher_secret_4567890123_abcdefghij\n")

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
                '#!/bin/sh\nfor arg in "$@"; do\n  printf \'ARG:[%s]\\n\' "$arg"\ndone\n'
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

            cmd = self._assert_kratos_argv_and_yaml(
                entrypoint_path, fake_kratos, template, tmp, env
            )
            self._assert_kratos_secret_errors(entrypoint_path, fake_kratos, cmd, sec_dir, env)
            self._assert_kratos_secret_encodings(cmd, sec_dir, tmp, env)
            self._assert_kratos_template_and_cookie_domain(cmd, template, tmp, env)

    def _assert_kratos_return_urls_and_cors_valid(
        self,
        cmd: list[str],
        base_env: dict[str, str],
        tmp: Path,
    ) -> None:
        # (a) allowed_return_urls equals ordered list from KRATOS_ALLOWED_RETURN_URL_1..3
        # and contains no unpassed example.com/localhost entries
        with self.subTest("(a) allowed_return_urls ordered from env without unpassed entries"):
            env_a = base_env.copy()
            env_a["KRATOS_ALLOWED_RETURN_URL_1"] = "https://example.com/custom/return1"
            env_a["KRATOS_ALLOWED_RETURN_URL_2"] = "http://localhost:4173/custom/return2"
            env_a["KRATOS_ALLOWED_RETURN_URL_3"] = "https://example.com/custom/return3"
            env_a["KRATOS_CORS_ALLOWED_ORIGIN_1"] = "http://localhost:4173"
            subprocess.run(cmd, env=env_a, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
            out_yaml_a = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            expected_urls = [
                "https://example.com/custom/return1",
                "http://localhost:4173/custom/return2",
                "https://example.com/custom/return3",
            ]
            assert out_yaml_a["selfservice"]["allowed_return_urls"] == expected_urls, out_yaml_a[
                "selfservice"
            ]["allowed_return_urls"]
            for unpassed in [
                "https://example.com/",
                "https://example.com/home",
                "https://example.com/desktop/home",
                "https://example.com/auth/login",
                "http://localhost/",
                "http://localhost/home",
                "http://localhost/desktop/home",
                "http://localhost/auth/login",
                "http://localhost:4173/sv/",
                "http://localhost:4173/sv/home",
                "http://localhost:4173/sv/auth/login",
            ]:
                assert unpassed not in out_yaml_a["selfservice"]["allowed_return_urls"], out_yaml_a[
                    "selfservice"
                ]["allowed_return_urls"]

        # (b) cors.allowed_origins rendered from KRATOS_CORS_ALLOWED_ORIGIN_1..2, and [] when none passed
        with self.subTest("(b) cors.allowed_origins from env and [] when none passed"):
            env_b1 = base_env.copy()
            env_b1["KRATOS_ALLOWED_RETURN_URL_1"] = "https://example.com/custom/return1"
            env_b1["KRATOS_CORS_ALLOWED_ORIGIN_1"] = "http://localhost:4173"
            env_b1["KRATOS_CORS_ALLOWED_ORIGIN_2"] = "https://example.com"
            subprocess.run(cmd, env=env_b1, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
            out_yaml_b1 = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            assert out_yaml_b1["serve"]["public"]["cors"]["allowed_origins"] == [
                "http://localhost:4173",
                "https://example.com",
            ], out_yaml_b1["serve"]["public"]["cors"]["allowed_origins"]

            # Unset CORS origins renders []
            env_b2 = base_env.copy()
            env_b2["KRATOS_ALLOWED_RETURN_URL_1"] = "https://example.com/custom/return1"
            subprocess.run(cmd, env=env_b2, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
            out_yaml_b2 = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            assert out_yaml_b2["serve"]["public"]["cors"]["allowed_origins"] == [], out_yaml_b2[
                "serve"
            ]["public"]["cors"]["allowed_origins"]

            # Empty string CORS origin renders []
            env_b3 = base_env.copy()
            env_b3["KRATOS_ALLOWED_RETURN_URL_1"] = "https://example.com/custom/return1"
            env_b3["KRATOS_CORS_ALLOWED_ORIGIN_1"] = ""
            subprocess.run(cmd, env=env_b3, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
            out_yaml_b3 = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            assert out_yaml_b3["serve"]["public"]["cors"]["allowed_origins"] == [], out_yaml_b3[
                "serve"
            ]["public"]["cors"]["allowed_origins"]

    def _assert_kratos_return_urls_and_cors_invalid(
        self,
        cmd: list[str],
        base_env: dict[str, str],
    ) -> None:
        # (c) missing KRATOS_ALLOWED_RETURN_URL_1 exits non-zero with variable name on stderr
        with self.subTest(
            "(c) missing KRATOS_ALLOWED_RETURN_URL_1 exits non-zero with var name on stderr"
        ):
            env_c1 = base_env.copy()
            env_c1.pop("KRATOS_ALLOWED_RETURN_URL_1", None)
            res_c1 = subprocess.run(  # noqa: S603 - fixed argv built in this test
                cmd, env=env_c1, capture_output=True, text=True, check=False
            )
            assert res_c1.returncode != 0, res_c1.stdout
            assert "KRATOS_ALLOWED_RETURN_URL_1" in res_c1.stderr, res_c1.stderr

            env_c2 = base_env.copy()
            env_c2["KRATOS_ALLOWED_RETURN_URL_1"] = ""
            res_c2 = subprocess.run(  # noqa: S603 - fixed argv built in this test
                cmd, env=env_c2, capture_output=True, text=True, check=False
            )
            assert res_c2.returncode != 0, res_c2.stdout
            assert "KRATOS_ALLOWED_RETURN_URL_1" in res_c2.stderr, res_c2.stderr

        # (d) non-absolute value such as example.com/ exits non-zero
        with self.subTest("(d) non-absolute value exits non-zero"):
            env_d1 = base_env.copy()
            env_d1["KRATOS_ALLOWED_RETURN_URL_1"] = "example.com/"
            res_d1 = subprocess.run(  # noqa: S603 - fixed argv built in this test
                cmd, env=env_d1, capture_output=True, text=True, check=False
            )
            assert res_d1.returncode != 0, res_d1.stdout
            assert "KRATOS_ALLOWED_RETURN_URL_1" in res_d1.stderr, res_d1.stderr

            env_d2 = base_env.copy()
            env_d2["KRATOS_ALLOWED_RETURN_URL_1"] = "https://example.com/valid"
            env_d2["KRATOS_ALLOWED_RETURN_URL_2"] = "localhost:4173"
            res_d2 = subprocess.run(  # noqa: S603 - fixed argv built in this test
                cmd, env=env_d2, capture_output=True, text=True, check=False
            )
            assert res_d2.returncode != 0, res_d2.stdout
            assert "KRATOS_ALLOWED_RETURN_URL_2" in res_d2.stderr, res_d2.stderr

            env_d3 = base_env.copy()
            env_d3["KRATOS_ALLOWED_RETURN_URL_1"] = "https://example.com/valid"
            env_d3["KRATOS_CORS_ALLOWED_ORIGIN_1"] = "example.com"
            res_d3 = subprocess.run(  # noqa: S603 - fixed argv built in this test
                cmd, env=env_d3, capture_output=True, text=True, check=False
            )
            assert res_d3.returncode != 0, res_d3.stdout
            assert "KRATOS_CORS_ALLOWED_ORIGIN_1" in res_d3.stderr, res_d3.stderr

    def _assert_kratos_return_urls_escaping_and_contiguous(
        self,
        cmd: list[str],
        base_env: dict[str, str],
        tmp: Path,
    ) -> None:
        # (e) value containing a double quote or backslash is rendered as valid YAML string
        with self.subTest("(e) double quote or backslash is rendered as valid YAML string"):
            env_e = base_env.copy()
            complex_return = r'https://example.com/path"with\special&chars|test'
            complex_cors = r'https://example.com/origin"with\backslash'
            env_e["KRATOS_ALLOWED_RETURN_URL_1"] = complex_return
            env_e["KRATOS_CORS_ALLOWED_ORIGIN_1"] = complex_cors
            subprocess.run(cmd, env=env_e, capture_output=True, text=True, check=True)  # noqa: S603 - fixed argv built in this test
            out_yaml_e = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            assert out_yaml_e["selfservice"]["allowed_return_urls"][0] == complex_return, (
                out_yaml_e["selfservice"]["allowed_return_urls"]
            )
            assert out_yaml_e["serve"]["public"]["cors"]["allowed_origins"][0] == complex_cors, (
                out_yaml_e["serve"]["public"]["cors"]["allowed_origins"]
            )

        # Contiguous collection stops at first unset/empty
        with self.subTest("contiguous collection stops at first unset/empty"):
            env_cont = base_env.copy()
            env_cont["KRATOS_ALLOWED_RETURN_URL_1"] = "https://example.com/1"
            env_cont["KRATOS_ALLOWED_RETURN_URL_2"] = "https://example.com/2"
            env_cont["KRATOS_ALLOWED_RETURN_URL_3"] = ""
            env_cont["KRATOS_ALLOWED_RETURN_URL_4"] = "https://example.com/4"
            subprocess.run(  # noqa: S603 - fixed argv built in this test
                cmd, env=env_cont, capture_output=True, text=True, check=True
            )
            out_yaml_cont = yaml.safe_load((tmp / "out_kratos.yml").read_text())
            assert out_yaml_cont["selfservice"]["allowed_return_urls"] == [
                "https://example.com/1",
                "https://example.com/2",
            ], out_yaml_cont["selfservice"]["allowed_return_urls"]

    def test_a03_kratos_allowed_return_urls_and_cors(self) -> None:
        """kratos/entrypoint.sh renders allowed_return_urls and CORS origins from env and validates URLs."""
        entrypoint_path = REPO_ROOT / "kratos" / "entrypoint.sh"
        canonical_template = REPO_ROOT / "kratos" / "kratos_template.yml"

        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            sec_dir = tmp / "secrets"
            sec_dir.mkdir()
            (sec_dir / "db_pass").write_text("db_secret_pass\n")
            (sec_dir / "cookie_sec").write_text("cookie_secret_1234567890_abcdefghij\n")
            (sec_dir / "cipher_sec").write_text("cipher_secret_4567890123_abcdefghij\n")

            fake_kratos = tmp / "fake_kratos"
            fake_kratos.write_text(
                '#!/bin/sh\nfor arg in "$@"; do\n  printf \'ARG:[%s]\\n\' "$arg"\ndone\n'
            )
            fake_kratos.chmod(0o755)

            base_env = {
                "PATH": f"{tmp}:" + os.environ.get("PATH", ""),
                "KRATOS_DB_PASSWORD_FILE": str(sec_dir / "db_pass"),
                "KRATOS_COOKIE_SECRET_FILE": str(sec_dir / "cookie_sec"),
                "KRATOS_CIPHER_SECRET_FILE": str(sec_dir / "cipher_sec"),
                "KRATOS_TEMPLATE_FILE": str(canonical_template),
                "KRATOS_CONFIG_FILE": str(tmp / "out_kratos.yml"),
                "KRATOS_COOKIE_DOMAIN": ".example.com",
            }
            cmd = [
                "/bin/sh",
                str(entrypoint_path),
                str(fake_kratos),
                "serve",
            ]

            self._assert_kratos_return_urls_and_cors_valid(cmd, base_env, tmp)
            self._assert_kratos_return_urls_and_cors_invalid(cmd, base_env)
            self._assert_kratos_return_urls_escaping_and_contiguous(cmd, base_env, tmp)

    # -------------------------------------------------------------------------
    # A04: Frontend HTTPS & AuthHub mTLS Configuration
    # -------------------------------------------------------------------------
    def test_a04_frontend_and_auth_hub_contracts(self) -> None:
        """alt-frontend-sv uses https://auth-hub:8443 and NODE_EXTRA_CA_CERTS; auth-hub restricts plaintext."""
        # Frontend in compose/core.yaml
        fe = self.compose_core["services"]["alt-frontend-sv"]
        env_list = fe.get("environment", [])
        assert (
            "AUTH_HUB_INTERNAL_URL=${AUTH_HUB_INTERNAL_URL:-https://auth-hub:8443}" in env_list
        ), env_list
        assert "NODE_EXTRA_CA_CERTS=/trust/ca-bundle.pem" in env_list, env_list
        assert "pki_trust_bundle:/trust:ro" in fe.get("volumes", []), fe.get("volumes", [])

        # Backend & DataHub in compose/core.yaml
        be = self.compose_core["services"]["alt-backend"]
        assert "AUTH_HUB_URL=https://auth-hub:9443" in be.get("environment", []), be.get(
            "environment", []
        )

        dh = self.compose_core["services"]["alt-data-hub"]
        assert dh["environment"]["AUTH_HUB_URL"] == "https://auth-hub:9443", dh["environment"]

        # auth-hub in compose/auth.yaml
        ah = self.compose_auth["services"]["auth-hub"]
        ah_env = ah.get("environment", [])
        assert "FRONTEND_TLS_LISTEN=true" in ah_env, ah_env
        assert "FRONTEND_TLS_PORT=8443" in ah_env, ah_env
        assert "FRONTEND_TLS_CERT_FILE=/certs/svc-cert.pem" in ah_env, ah_env
        assert "FRONTEND_TLS_KEY_FILE=/certs/svc-key.pem" in ah_env, ah_env

        # Peer allowlist has search-indexer and knowledge-sovereign, no unused BFF
        allowed_peers_entry = next(e for e in ah_env if e.startswith("MTLS_ALLOWED_PEERS="))
        assert "search-indexer" in allowed_peers_entry, allowed_peers_entry
        assert "knowledge-sovereign" in allowed_peers_entry, allowed_peers_entry
        assert "alt-butterfly-facade" not in allowed_peers_entry, allowed_peers_entry

    # -------------------------------------------------------------------------
    # B04: Augur Ollama 127.0.0.1 Binding
    # -------------------------------------------------------------------------
    def test_b04_augur_ports_bound_to_loopback(self) -> None:
        """Augur Ollama services must bind strictly to 127.0.0.1, not 0.0.0.0."""
        augur_ports = self.compose_augur["services"]["knowledge-augur"]["ports"]
        assert "127.0.0.1:11435:11434" in augur_ports, augur_ports
        assert "11435:11434" not in augur_ports, augur_ports

        embedder_ports = self.compose_augur["services"]["knowledge-embedder"]["ports"]
        assert "127.0.0.1:11436:11434" in embedder_ports, embedder_ports
        assert "11436:11434" not in embedder_ports, embedder_ports

    # -------------------------------------------------------------------------
    # B05: Named Redis Roles & ACL Loader
    # -------------------------------------------------------------------------
    def _assert_redis_streams_and_cache_roles(
        self,
        entrypoint_path: Path,
        tmp: Path,
        env: dict[str, str],
    ) -> list[str]:
        # 1. Streams role test
        cmd_streams = [
            "/bin/sh",
            str(entrypoint_path),
            "streams",
            "--maxmemory",
            "1gb",
        ]
        res_streams = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env, capture_output=True, text=True, check=True
        )
        assert f"--aclfile {tmp / 'test_users.acl'} --maxmemory 1gb" in res_streams.stdout, (
            res_streams.stdout
        )

        acl_content = (tmp / "test_users.acl").read_text()
        assert "user default off -@all" in acl_content, acl_content

        producer_hash = hashlib.sha256(b"producer_secret_pass").hexdigest()
        consumer_hash = hashlib.sha256(b"consumer_secret_pass").hexdigest()
        limiter_hash = hashlib.sha256(b"limiter_secret_pass").hexdigest()
        assert (
            f"user streams_producer on #{producer_hash} resetkeys ~alt:events:* ~alt:replies:tags:* -@all +hello +client|setinfo +client|setname +ping +xadd +xgroup|create +xinfo|stream +xinfo|groups +xtrim +scan (~alt:replies:tags:* +xread +del +expire +ttl)"
            in acl_content
        ), acl_content
        assert (
            f"user streams_consumer on #{consumer_hash} resetkeys ~alt:events:* ~alt:replies:tags:* -@all +hello +client|setinfo +client|setname +ping +xreadgroup +xack +xautoclaim +xpending +xgroup|create +xinfo|groups (~alt:events:*:dlq +xadd) (~alt:replies:tags:* +xadd)"
            in acl_content
        ), acl_content
        assert "user streams on" not in acl_content, acl_content
        assert (
            f"user limiter on #{limiter_hash} resetkeys ~host_rate_limiter:v1:* -@all +hello +client|setinfo +client|setname +ping +set +pttl +select"
            in acl_content
        ), acl_content
        assert "producer_secret_pass" not in acl_content, acl_content
        assert "consumer_secret_pass" not in acl_content, acl_content
        assert "limiter_secret_pass" not in acl_content, acl_content

        # ACL file permissions must be 0600
        file_mode = stat.S_IMODE((tmp / "test_users.acl").stat().st_mode)
        assert file_mode == OWNER_RW_FILE_MODE, oct(file_mode)

        # 2. Cache role test
        cmd_cache = [
            "/bin/sh",
            str(entrypoint_path),
            "cache",
            "--maxmemory",
            "256mb",
        ]
        res_cache = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_cache, env=env, capture_output=True, text=True, check=True
        )
        assert f"--aclfile {tmp / 'test_users.acl'} --maxmemory 256mb" in res_cache.stdout, (
            res_cache.stdout
        )

        acl_cache = (tmp / "test_users.acl").read_text()
        assert "user default off -@all" in acl_cache, acl_cache
        cache_hash = hashlib.sha256(b"cache_secret_pass").hexdigest()
        assert (
            f"user cache on #{cache_hash} resetkeys ~recap_card:* ~recap:summary:* -@all +hello +client|setinfo +client|setname +ping +get +set +del"
            in acl_cache
        ), acl_cache
        assert "cache_secret_pass" not in acl_cache, acl_cache
        assert "user streams" not in acl_cache, acl_cache
        return cmd_streams

    def _assert_redis_missing_and_consumer_secrets(
        self,
        cmd_streams: list[str],
        sec_dir: Path,
        tmp: Path,
        env: dict[str, str],
    ) -> None:
        # 3. Missing secret fails
        env_err = env.copy()
        env_err["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "nonexistent")
        res_err = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_err, capture_output=True, text=True, check=False
        )
        assert res_err.returncode != 0, res_err.stdout
        assert "is missing" in res_err.stderr, res_err.stderr

        # 3b. Identical producer and consumer secrets must be denied
        (sec_dir / "consumer_same").write_text("producer_secret_pass\n")
        env_same = env.copy()
        env_same["REDIS_STREAMS_CONSUMER_PASSWORD_FILE"] = str(sec_dir / "consumer_same")
        res_same = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_same, capture_output=True, text=True, check=False
        )
        assert res_same.returncode != 0, res_same.stdout
        assert "must differ" in res_same.stderr, res_same.stderr

        # 3c. The consumer secret goes through the same validation as the producer's
        (sec_dir / "consumer_nopass").write_text("fixture nopass\n")
        (sec_dir / "consumer_tab").write_text("tab\tinjection\n")
        (sec_dir / "consumer_ctrl").write_bytes(b"ctrl\x01pass\n")
        (sec_dir / "consumer_blank").write_text("   \t  \n")
        for bad in (
            "nonexistent",
            "consumer_nopass",
            "consumer_tab",
            "consumer_ctrl",
            "consumer_blank",
        ):
            env_bad = env.copy()
            env_bad["REDIS_STREAMS_CONSUMER_PASSWORD_FILE"] = str(sec_dir / bad)
            res_bad = subprocess.run(  # noqa: S603 - fixed argv built in this test
                cmd_streams, env=env_bad, capture_output=True, text=True, check=False
            )
            assert res_bad.returncode != 0, f"consumer secret {bad} must be rejected"
            assert "fixture nopass" not in res_bad.stderr, res_bad.stderr

        # 4. Malicious space fixture 'fixture nopass' must be denied (exit non-zero, no nopass directive in ACL)
        (sec_dir / "malicious_nopass").write_text("fixture nopass\n")
        env_nopass = env.copy()
        env_nopass["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_nopass")
        res_nopass = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_nopass, capture_output=True, text=True, check=False
        )
        assert res_nopass.returncode != 0, res_nopass.stdout
        assert "fixture nopass" not in res_nopass.stderr, res_nopass.stderr
        # Ensure ACL file does not contain nopass directive
        if (tmp / "test_users.acl").exists():
            acl_after = (tmp / "test_users.acl").read_text()
            assert "nopass" not in acl_after, acl_after

    def _assert_redis_injection_and_utf8_rejections(
        self,
        cmd_streams: list[str],
        sec_dir: Path,
        env: dict[str, str],
    ) -> None:
        # 5. Tab character injection must be denied
        (sec_dir / "malicious_tab").write_text("tab\tinjection\n")
        env_tab = env.copy()
        env_tab["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_tab")
        res_tab = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_tab, capture_output=True, text=True, check=False
        )
        assert res_tab.returncode != 0, res_tab.stdout
        assert "tab\tinjection" not in res_tab.stderr, res_tab.stderr

        # 6. Control character injection must be denied
        (sec_dir / "malicious_ctrl").write_bytes(b"ctrl\x01pass\n")
        env_ctrl = env.copy()
        env_ctrl["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_ctrl")
        res_ctrl = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_ctrl, capture_output=True, text=True, check=False
        )
        assert res_ctrl.returncode != 0, res_ctrl.stdout

        # 7. Malformed UTF-8 must be denied
        (sec_dir / "malicious_utf8").write_bytes(b"\xff\xfe\xfd\n")
        env_utf8 = env.copy()
        env_utf8["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_utf8")
        res_utf8 = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_utf8, capture_output=True, text=True, check=False
        )
        assert res_utf8.returncode != 0, res_utf8.stdout

        # 8. Blank / whitespace-only secret must be denied
        (sec_dir / "malicious_blank").write_text("   \t  \n")
        env_blank = env.copy()
        env_blank["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_blank")
        res_blank = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_blank, capture_output=True, text=True, check=False
        )
        assert res_blank.returncode != 0, res_blank.stdout

        # 9. Canonical UTF-8 scalar ranges: overlong / surrogate / out-of-Unicode MUST be denied
        # 9a. Overlong 3-byte sequence E0 80 80
        (sec_dir / "malicious_overlong").write_bytes(b"\xe0\x80\x80\n")
        env_overlong = env.copy()
        env_overlong["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_overlong")
        res_overlong = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_overlong, capture_output=True, text=True, check=False
        )
        assert res_overlong.returncode != 0, "E0 80 80 overlong must be rejected"
        assert "malicious_overlong" not in res_overlong.stderr, res_overlong.stderr
        assert "\xe0\x80\x80" not in res_overlong.stderr, res_overlong.stderr

        # 9b. UTF-16 surrogate ED A0 80
        (sec_dir / "malicious_surrogate").write_bytes(b"\xed\xa0\x80\n")
        env_surrogate = env.copy()
        env_surrogate["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_surrogate")
        res_surrogate = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_surrogate, capture_output=True, text=True, check=False
        )
        assert res_surrogate.returncode != 0, "ED A0 80 surrogate must be rejected"
        assert "malicious_surrogate" not in res_surrogate.stderr, res_surrogate.stderr
        assert "\xed\xa0\x80" not in res_surrogate.stderr, res_surrogate.stderr

        # 9c. Out of Unicode F4 90 80 80
        (sec_dir / "malicious_out_of_unicode").write_bytes(b"\xf4\x90\x80\x80\n")
        env_oou = env.copy()
        env_oou["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_out_of_unicode")
        res_oou = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_oou, capture_output=True, text=True, check=False
        )
        assert res_oou.returncode != 0, "F4 90 80 80 out-of-Unicode must be rejected"
        assert "malicious_out_of_unicode" not in res_oou.stderr, res_oou.stderr
        assert "\xf4\x90\x80\x80" not in res_oou.stderr, res_oou.stderr

        # 9d. Overlong 4-byte sequence F0 80 80 80
        (sec_dir / "malicious_overlong_4").write_bytes(b"\xf0\x80\x80\x80\n")
        env_ol4 = env.copy()
        env_ol4["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "malicious_overlong_4")
        res_ol4 = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_ol4, capture_output=True, text=True, check=False
        )
        assert res_ol4.returncode != 0, "F0 80 80 80 overlong must be rejected"

    def _assert_redis_valid_utf8_and_multiline(
        self,
        cmd_streams: list[str],
        sec_dir: Path,
        tmp: Path,
        env: dict[str, str],
    ) -> None:
        # 10. Valid non-ASCII UTF-8 sequences MUST be accepted with exact raw byte sha256 hash
        # 10a. Valid non-ASCII text with German umlauts and Japanese kanji under UTF-8 locale
        valid_utf8_pw = "p\xc3\xa1ss_日本語_123"
        (sec_dir / "valid_utf8").write_text(valid_utf8_pw + "\n", encoding="utf-8")
        env_valid_utf8 = env.copy()
        env_valid_utf8["LC_ALL"] = "C.UTF-8"
        env_valid_utf8["LANG"] = "C.UTF-8"
        env_valid_utf8["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "valid_utf8")
        subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_valid_utf8, capture_output=True, text=True, check=True
        )
        valid_utf8_hash = hashlib.sha256(valid_utf8_pw.encode("utf-8")).hexdigest()
        acl_valid_utf8 = (tmp / "test_users.acl").read_text()
        assert f"user streams_producer on #{valid_utf8_hash}" in acl_valid_utf8, acl_valid_utf8
        assert valid_utf8_pw not in acl_valid_utf8, acl_valid_utf8

        # 10b. Valid boundary UTF-8 sequences: U+0800 (E0 A0 80), U+D7FF (ED 9F BF), U+10FFFF (F4 8F BF BF), emoji (F0 9F 8E 89)
        valid_boundary_pw = b"\xe0\xa0\x80_\xed\x9f\xbf_\xf4\x8f\xbf\xbf_\xf0\x9f\x8e\x89"
        (sec_dir / "valid_boundary").write_bytes(valid_boundary_pw + b"\n")
        env_valid_boundary = env.copy()
        env_valid_boundary["LC_ALL"] = "C.UTF-8"
        env_valid_boundary["LANG"] = "C.UTF-8"
        env_valid_boundary["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "valid_boundary")
        subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_valid_boundary, capture_output=True, text=True, check=True
        )
        valid_boundary_hash = hashlib.sha256(valid_boundary_pw).hexdigest()
        acl_valid_boundary = (tmp / "test_users.acl").read_text()
        assert f"user streams_producer on #{valid_boundary_hash}" in acl_valid_boundary, (
            acl_valid_boundary
        )

        # 10c. Native Japanese + emoji combined secret under UTF-8 locale ensures exact raw UTF-8 SHA256 hash in ACL
        valid_jp_emoji_pw = "東京_🗼_富士山_🗻_2026!secret"
        (sec_dir / "valid_jp_emoji").write_text(valid_jp_emoji_pw + "\r\n", encoding="utf-8")
        env_valid_jp_emoji = env.copy()
        env_valid_jp_emoji["LC_ALL"] = "C.UTF-8"
        env_valid_jp_emoji["LANG"] = "C.UTF-8"
        env_valid_jp_emoji["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(sec_dir / "valid_jp_emoji")
        subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_valid_jp_emoji, capture_output=True, text=True, check=True
        )
        valid_jp_emoji_hash = hashlib.sha256(valid_jp_emoji_pw.encode("utf-8")).hexdigest()
        acl_valid_jp_emoji = (tmp / "test_users.acl").read_text()
        assert f"user streams_producer on #{valid_jp_emoji_hash}" in acl_valid_jp_emoji, (
            acl_valid_jp_emoji
        )
        assert valid_jp_emoji_pw not in acl_valid_jp_emoji, acl_valid_jp_emoji

        # 10d. trimONEallowedtrailinglineend: multiple trailing newlines must be rejected
        (sec_dir / "malicious_multiline_end").write_text("validpass\n\n", encoding="utf-8")
        env_multiline = env.copy()
        env_multiline["REDIS_STREAMS_PRODUCER_PASSWORD_FILE"] = str(
            sec_dir / "malicious_multiline_end"
        )
        res_multiline = subprocess.run(  # noqa: S603 - fixed argv built in this test
            cmd_streams, env=env_multiline, capture_output=True, text=True, check=False
        )
        assert res_multiline.returncode != 0, "multiple trailing newlines must be rejected"

    def test_b05_redis_acl_loader_behavioral_fixtures(self) -> None:
        """Execute docker/redis/entrypoint.sh in isolated temp environments via /bin/sh."""
        entrypoint_path = REPO_ROOT / "docker" / "redis" / "entrypoint.sh"

        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            sec_dir = tmp / "secrets"
            sec_dir.mkdir()
            (sec_dir / "producer_pass").write_text("producer_secret_pass\n")
            (sec_dir / "consumer_pass").write_text("consumer_secret_pass\n")
            (sec_dir / "limiter_pass").write_text("limiter_secret_pass\n")
            (sec_dir / "cache_pass").write_text("cache_secret_pass\n")

            fake_redis = tmp / "fake_redis_server"
            fake_redis.write_text("#!/bin/sh\nprintf 'REDIS_ARGS:[%s]\\n' \"$*\"\n")
            fake_redis.chmod(0o755)
            # Replace redis-server with fake binary inside script execution by symlinking
            (tmp / "redis-server").symlink_to(fake_redis)

            env = {
                "PATH": f"{tmp}:" + os.environ.get("PATH", ""),
                "REDIS_STREAMS_PRODUCER_PASSWORD_FILE": str(sec_dir / "producer_pass"),
                "REDIS_STREAMS_CONSUMER_PASSWORD_FILE": str(sec_dir / "consumer_pass"),
                "REDIS_STREAMS_RESET_USER": "disabled",
                "REDIS_LIMITER_PASSWORD_FILE": str(sec_dir / "limiter_pass"),
                "REDIS_CACHE_PASSWORD_FILE": str(sec_dir / "cache_pass"),
                "REDIS_ACL_FILE": str(tmp / "test_users.acl"),
            }

            cmd_streams = self._assert_redis_streams_and_cache_roles(entrypoint_path, tmp, env)
            self._assert_redis_missing_and_consumer_secrets(cmd_streams, sec_dir, tmp, env)
            self._assert_redis_injection_and_utf8_rejections(cmd_streams, sec_dir, env)
            self._assert_redis_valid_utf8_and_multiline(cmd_streams, sec_dir, tmp, env)

    # -------------------------------------------------------------------------
    # Dev Helper start-dev.sh Contracts
    # -------------------------------------------------------------------------
    def test_start_dev_script_contracts(self) -> None:
        """start-dev.sh must use compose/compose.yaml and compose/compose.dev.yaml, and not generate kratos.yml."""
        start_dev_content = (REPO_ROOT / "for-development" / "start-dev.sh").read_text(
            encoding="utf-8"
        )
        assert "-f compose.yaml -f compose.dev.yaml" not in start_dev_content, start_dev_content
        assert "-f compose/compose.yaml -f compose/compose.dev.yaml" in start_dev_content, (
            start_dev_content
        )
        assert "kratos/kratos.yml" not in start_dev_content, start_dev_content

    def test_b05_compose_callers_and_healthchecks(self) -> None:
        """Verify all Redis callers and healthchecks use named roles and separate secrets."""
        # redis-streams
        rs = self.compose_mq["services"]["redis-streams"]
        assert rs["entrypoint"] == ["/entrypoint.sh", "streams"], rs["entrypoint"]
        assert "redis_streams_producer_password" in rs["secrets"], rs["secrets"]
        assert "redis_streams_consumer_password" in rs["secrets"], rs["secrets"]
        assert "redis_limiter_password" in rs["secrets"], rs["secrets"]
        assert "--user streams_producer" in rs["healthcheck"]["test"][1], rs["healthcheck"]

        # mq-hub
        mq = self.compose_mq["services"]["mq-hub"]
        assert "REDIS_URL=redis://streams_producer@redis-streams:6379" in mq["environment"], mq[
            "environment"
        ]
        assert (
            "REDIS_PASSWORD_FILE=/run/secrets/redis_streams_producer_password" in mq["environment"]
        ), mq["environment"]
        assert "redis_streams_producer_password" in mq["secrets"], mq["secrets"]
        assert "redis_streams_consumer_password" not in mq["secrets"], mq["secrets"]

        # redis-cache
        rc = self.compose_ai["services"]["redis-cache"]
        assert rc["entrypoint"] == ["/entrypoint.sh", "cache"], rc["entrypoint"]
        assert "redis_cache_password" in rc["secrets"], rc["secrets"]
        assert "--user cache" in rc["healthcheck"]["test"][1], rc["healthcheck"]

        # news-creator
        nc = self.compose_ai["services"]["news-creator"]
        assert "CACHE_REDIS_URL=redis://cache@redis-cache:6379/0" in nc["environment"], nc[
            "environment"
        ]
        assert "REDIS_PASSWORD_FILE=/run/secrets/redis_cache_password" in nc["environment"], nc[
            "environment"
        ]
        assert "redis_cache_password" in nc["secrets"], nc["secrets"]

        # pre-processor
        pp = self.compose_ai["services"]["pre-processor"]
        assert (
            "REDIS_STREAMS_URL=redis://streams_consumer@redis-streams:6379" in pp["environment"]
        ), pp["environment"]
        assert (
            "REDIS_PASSWORD_FILE=/run/secrets/redis_streams_consumer_password" in pp["environment"]
        ), pp["environment"]
        assert "redis_streams_consumer_password" in pp["secrets"], pp["secrets"]
        assert "redis_streams_producer_password" not in pp["secrets"], pp["secrets"]

        # search-indexer
        si = self.compose_workers["services"]["search-indexer"]
        assert (
            "REDIS_STREAMS_URL=redis://streams_consumer@redis-streams:6379" in si["environment"]
        ), si["environment"]
        assert (
            "REDIS_PASSWORD_FILE=/run/secrets/redis_streams_consumer_password" in si["environment"]
        ), si["environment"]
        assert "redis_streams_consumer_password" in si["secrets"], si["secrets"]
        assert "redis_streams_producer_password" not in si["secrets"], si["secrets"]

        # tag-generator
        tg = self.compose_workers["services"]["tag-generator"]
        assert (
            "REDIS_STREAMS_URL=redis://streams_consumer@redis-streams:6379" in tg["environment"]
        ), tg["environment"]
        assert (
            "REDIS_PASSWORD_FILE=/run/secrets/redis_streams_consumer_password" in tg["environment"]
        ), tg["environment"]
        assert "redis_streams_consumer_password" in tg["secrets"], tg["secrets"]
        assert "redis_streams_producer_password" not in tg["secrets"], tg["secrets"]

        # backend limiter
        be = self.compose_core["services"]["alt-backend"]
        assert (
            "HOST_RATE_LIMITER_REDIS_URL=redis://limiter@redis-streams:6379/3" in be["environment"]
        ), be["environment"]
        assert (
            "HOST_RATE_LIMITER_REDIS_PASSWORD_FILE=/run/secrets/redis_limiter_password"
            in be["environment"]
        ), be["environment"]
        assert "redis_limiter_password" in be["secrets"], be["secrets"]

        # harvester limiter
        hv = self.compose_core["services"]["alt-harvester"]
        assert (
            hv["environment"]["HOST_RATE_LIMITER_REDIS_URL"]
            == "redis://limiter@redis-streams:6379/3"
        ), hv["environment"]
        assert (
            hv["environment"]["HOST_RATE_LIMITER_REDIS_PASSWORD_FILE"]
            == "/run/secrets/redis_limiter_password"  # noqa: S105 - test fixture value
        ), hv["environment"]
        assert "redis_limiter_password" in hv["secrets"], hv["secrets"]

    # -------------------------------------------------------------------------
    # ClickHouse Direct Backup & db.yaml
    # -------------------------------------------------------------------------
    def test_clickhouse_direct_backup_config(self) -> None:
        """ClickHouse in compose/db.yaml configures backup_disk and mounts /backups/clickhouse."""
        ch = self.compose_db["services"]["clickhouse"]
        volumes = ch.get("volumes", [])
        backup_mount = next(
            (
                v
                for v in volumes
                if (
                    v == "../backups/clickhouse:/backups/clickhouse"
                    or (isinstance(v, dict) and v.get("target") == "/backups/clickhouse")
                )
            ),
            None,
        )
        assert backup_mount is not None, (
            "Missing /backups/clickhouse volume mount in clickhouse service"
        )
        if isinstance(backup_mount, dict):
            assert backup_mount.get("type") == "bind", backup_mount
            assert (
                backup_mount.get("source")
                == "${CLICKHOUSE_BACKUP_HOST_PATH:-/var/lib/alt-clickhouse-backups}"
            ), backup_mount
            # Writable backup directory
            assert not backup_mount.get("read_only", False), backup_mount
            assert not backup_mount.get("bind", {}).get("create_host_path", True), backup_mount

        configs = ch.get("configs", [])
        backup_cfg = [c for c in configs if c.get("source") == "clickhouse_backup_disk"]
        assert len(backup_cfg) > 0, (
            "Missing clickhouse_backup_disk config mount in clickhouse service"
        )
        assert backup_cfg[0]["target"] == "/etc/clickhouse-server/config.d/backup_disk.xml", (
            backup_cfg[0]
        )

        # Top-level configs definition
        top_configs = self.compose_db.get("configs", {})
        assert "clickhouse_backup_disk" in top_configs, top_configs
        assert (
            top_configs["clickhouse_backup_disk"]["file"] == "../clickhouse/config/backup_disk.xml"
        ), top_configs["clickhouse_backup_disk"]

    # -------------------------------------------------------------------------
    # Base Secret Declarations
    # -------------------------------------------------------------------------
    @staticmethod
    def _environment(service: dict[str, Any]) -> dict[str, Any]:
        env = service.get("environment", {})
        if isinstance(env, dict):
            return env
        return dict(item.split("=", 1) for item in env if "=" in item)

    def test_mqhub_broker_and_enabled_clients_share_generated_secret(self) -> None:
        """Every enabled broker client must authenticate with the broker's file secret."""
        secret_name = "mqhub_auth_token"  # noqa: S105 - test fixture value
        token_path = f"/run/secrets/{secret_name}"
        assert self.compose_base["secrets"].get(secret_name) == {
            "file": "../secrets/mqhub_auth_token.txt"
        }, self.compose_base["secrets"].get(secret_name)
        services: dict[str, dict[str, Any]] = {}
        for document in (self.compose_core, self.compose_ai, self.compose_workers, self.compose_mq):
            services.update(document["services"])
        enabled = {
            name
            for name, service in services.items()
            if str(self._environment(service).get("MQHUB_ENABLED", "false")).lower() == "true"
        }
        assert enabled == {"alt-backend", "alt-harvester", "alt-data-hub"}, enabled
        for name in enabled | {"mq-hub"}:
            with self.subTest(service=name):
                service = services[name]
                environment = self._environment(service)
                assert environment.get("MQHUB_AUTH_TOKEN_FILE") == token_path, environment
                assert "MQHUB_AUTH_TOKEN" not in environment, environment
                assert secret_name in service.get("secrets", []), service.get("secrets", [])

    def test_search_introspection_uses_own_leaf_and_auth_hub_mtls_route(self) -> None:
        """Search's mandatory introspection URL must use the authorized mTLS listener."""
        service = self.compose_workers["services"]["search-indexer"]
        env = self._environment(service)
        assert (
            env.get("USER_JWT_INTROSPECTION_URL")
            == "https://auth-hub:9443/internal/token/introspect"
        ), env.get("USER_JWT_INTROSPECTION_URL")
        assert env["CERT_SUBJECT"] == "search-indexer", env["CERT_SUBJECT"]
        assert env["MTLS_CERT_FILE"] == "/certs/svc-cert.pem", env["MTLS_CERT_FILE"]
        assert env["MTLS_KEY_FILE"] == "/certs/svc-key.pem", env["MTLS_KEY_FILE"]
        assert env["MTLS_CA_FILE"] == "/trust/ca-bundle.pem", env["MTLS_CA_FILE"]
        assert "search_indexer_certs:/certs" in service["volumes"], service["volumes"]
        assert "pki_trust_bundle:/trust:ro" in service["volumes"], service["volumes"]
        auth_env = self._environment(self.compose_auth["services"]["auth-hub"])
        assert auth_env["MTLS_PORT"] == "9443", auth_env["MTLS_PORT"]
        peers = auth_env["MTLS_ALLOWED_PEERS"].split(":-", 1)[1].removesuffix("}").split(",")
        assert "search-indexer" in peers, peers

    def test_search_loads_mounted_inference_token_for_embedder_api_key(self) -> None:
        """A mounted token without its loader environment key yields an empty embedder API key."""
        service = self.compose_workers["services"]["search-indexer"]
        assert (
            self._environment(service).get("INFERENCE_SERVICE_TOKEN_FILE")
            == "/run/secrets/inference_service_token"
        ), self._environment(service)
        assert "inference_service_token" in service["secrets"], service["secrets"]

    def test_dev_stacks_never_use_real_token_keys(self) -> None:
        """Dev stacks mint and verify tokens with their own volume-held key pair, never the real secrets/."""
        for name in ("dev.yaml", "frontend-dev.yaml"):
            data = self._load_yaml(REPO_ROOT / "compose" / name)
            text = (REPO_ROOT / "compose" / name).read_text()
            assert "../secrets/" not in text, f"{name} must not read or write ../secrets/"
            for svc_name, svc in data["services"].items():
                for sec in svc.get("secrets", []) or []:
                    sec_name = sec if isinstance(sec, str) else sec.get("source")
                    assert sec_name not in (
                        "backend_token_signing_key",
                        "backend_token_verify_key",
                    ), f"{name}:{svc_name} mounts the real token secret {sec_name}"
                for env in svc.get("environment", []) or []:
                    if isinstance(env, str) and env.split("=", 1)[0] in (
                        "BACKEND_TOKEN_SIGNING_KEY_FILE",
                        "BACKEND_TOKEN_VERIFY_KEY_FILE",
                    ):
                        assert env.split("=", 1)[-1].startswith("/secrets/dev_"), (
                            f"{name}:{svc_name} token key must come from the dev secrets volume: {env}"
                        )

    def _load_yaml_lenient(self, path: Path) -> dict[str, Any]:
        """Compose-only tags (!override, !reset) load as their plain node value."""

        class Loader(yaml.SafeLoader):
            pass

        Loader.add_multi_constructor(
            "!",
            lambda loader, suffix, node: (
                loader.construct_sequence(node)
                if isinstance(node, yaml.SequenceNode)
                else loader.construct_mapping(node)
                if isinstance(node, yaml.MappingNode)
                else loader.construct_scalar(node)
            ),
        )
        with path.open(encoding="utf-8") as f:
            return yaml.load(f, Loader=Loader)  # noqa: S506 - Loader subclasses yaml.SafeLoader for Compose tags

    def _all_stack_services(self) -> list[tuple[str, str, dict[str, Any]]]:
        out: list[tuple[str, str, dict[str, Any]]] = []
        for path in sorted((REPO_ROOT / "compose").glob("*.yaml")):
            data = self._load_yaml_lenient(path)
            for name, svc in (data.get("services") or {}).items():
                out.append((path.name, name, svc))
        return out

    def test_cap_dropped_services_have_capable_root_hooks(self) -> None:
        """A pre_start hook inherits the service's cap_drop, so a root chown hook must re-add its capabilities."""
        needed = {"CHOWN", "DAC_OVERRIDE", "FOWNER"}
        checked = 0
        for fname, name, svc in self._all_stack_services():
            if "ALL" not in (svc.get("cap_drop") or []):
                continue
            for key in ("pre_start", "post_start", "pre_stop", "post_stop"):
                for hook in svc.get(key) or []:
                    if str(hook.get("user", "")).split(":")[0] not in ("0", "root"):
                        continue
                    checked += 1
                    assert needed <= set(hook.get("cap_add") or []), (
                        f"{fname}:{name} {key} runs as root under cap_drop ALL without {sorted(needed)}"
                    )
        assert checked > 0, checked

    def test_backend_token_key_distribution(self) -> None:
        """Signing key: identity gateway only. Verify key: user-facing backend, BFF, acolyte; never knowledge-sovereign."""

        def secret_names(svc: dict[str, Any]) -> list[str | None]:
            return [
                x if isinstance(x, str) else x.get("source") for x in (svc.get("secrets") or [])
            ]

        def env_map(svc: dict[str, Any]) -> dict[str, str]:
            env = svc.get("environment") or []
            if isinstance(env, dict):
                return {k: str(v) for k, v in env.items()}
            return dict(e.split("=", 1) for e in env if "=" in e)

        skip = ("dev.yaml", "frontend-dev.yaml", "compose.dev.yaml")
        prod = [
            p
            for p in sorted((REPO_ROOT / "compose").glob("*.yaml"))
            if "staging" not in p.name and p.name not in skip
        ]
        signers, verifiers = [], []
        for path in prod:
            for name, svc in (self._load_yaml_lenient(path).get("services") or {}).items():
                names = secret_names(svc)
                if "backend_token_signing_key" in names:
                    signers.append(name)
                    assert (
                        env_map(svc).get("BACKEND_TOKEN_SIGNING_KEY_FILE")
                        == "/run/secrets/backend_token_signing_key"
                    ), env_map(svc)
                if "backend_token_verify_key" in names:
                    verifiers.append(name)
                    assert (
                        env_map(svc).get("BACKEND_TOKEN_VERIFY_KEY_FILE")
                        == "/run/secrets/backend_token_verify_key"
                    ), env_map(svc)
        assert signers == ["auth-hub"], signers
        assert sorted(verifiers) == [
            "acolyte-orchestrator",
            "alt-backend",
            "alt-butterfly-facade",
        ], verifiers
        for name in ("dev.yaml", "frontend-dev.yaml"):
            for svc in (
                self._load_yaml_lenient(REPO_ROOT / "compose" / name).get("services") or {}
            ).values():
                assert "backend_token_signing_key" not in secret_names(svc), secret_names(svc)
        sov = self._load_yaml(REPO_ROOT / "compose" / "sovereign.yaml")["services"][
            "knowledge-sovereign"
        ]
        assert "backend_token_verify_key" not in secret_names(sov), secret_names(sov)
        assert "BACKEND_TOKEN_VERIFY_KEY_FILE" not in env_map(sov), env_map(sov)
        assert "USER_JWT_INTROSPECTION_URL" in env_map(sov), env_map(sov)

    def test_trusted_proxy_cidrs_wiring(self) -> None:
        """alt-backend requires an operator value; auth-hub trusts no forwarder unless told otherwise."""
        holders = {}
        for fname, name, svc in self._all_stack_services():
            if fname in ("dev.yaml", "compose.dev.yaml") or "staging" in fname:
                continue
            env = svc.get("environment") or []
            items = env if isinstance(env, list) else [f"{k}={v}" for k, v in env.items()]
            for e in items:
                if str(e).startswith("TRUSTED_PROXY_CIDRS="):
                    holders[name] = str(e).split("=", 1)[1]
            assert not any(str(e).startswith("NOTIFICATION_USER_ID") for e in items), (
                f"{name} still sets NOTIFICATION_USER_ID"
            )
        assert sorted(holders) == ["alt-backend", "auth-hub"], holders
        assert holders["alt-backend"].startswith("${TRUSTED_PROXY_CIDRS:?"), holders["alt-backend"]
        assert holders["auth-hub"] == "${AUTH_HUB_TRUSTED_PROXY_CIDRS:-none}", holders["auth-hub"]

    # Repo-built services in the production stack set that deliberately keep default capabilities.
    UNHARDENED: ClassVar[dict[str, str]] = {
        "dashboard": "GPU device reservation, not verifiable without starting it",
        "recap-subworker": "GPU device reservation",
        "knowledge-embedder-local": "image runs as root and drops privilege with gosu; nvidia runtime",
        "news-creator-backend": "entrypoint re-execs from root with chown/setpriv; nvidia runtime",
        "rag-db": "upstream postgres entrypoint (gosu) with AppArmor profile",
        "recap-db": "upstream postgres entrypoint (gosu) with AppArmor profile",
        "restic-backup": "image has no USER directive and reads host data as root",
    }

    def test_repo_built_services_drop_capabilities_unless_allow_listed(self) -> None:
        """Covers profile-gated services too: the YAML is read directly, not through a default render."""
        included = self._load_yaml(REPO_ROOT / "compose" / "compose.yaml")["include"]
        seen_unhardened = set()
        for fname, name, svc in self._all_stack_services():
            if fname not in included or not svc.get("build"):
                continue
            hardened = "ALL" in (svc.get("cap_drop") or []) and "no-new-privileges:true" in (
                svc.get("security_opt") or []
            )
            if name in self.UNHARDENED:
                seen_unhardened.add(name)
                assert not hardened, f"{name} is hardened now; remove it from UNHARDENED"
            else:
                assert hardened, f"{fname}:{name} is repo-built but keeps default capabilities"
        assert seen_unhardened == set(self.UNHARDENED), seen_unhardened

    def test_knowledge_sovereign_auth_modes_and_app_env(self) -> None:
        def env_map(svc: dict[str, Any]) -> dict[str, str]:
            env = svc.get("environment") or []
            if isinstance(env, dict):
                return {k: str(v) for k, v in env.items()}
            return dict(e.split("=", 1) for e in env if "=" in e)

        main = self._load_yaml_lenient(REPO_ROOT / "compose" / "sovereign.yaml")["services"][
            "knowledge-sovereign"
        ]
        assert "env_file" not in main, main
        env = env_map(main)
        assert env.get("ADMIN_AUTH") != "disabled", env
        assert env.get("EVENT_AUTH") != "disabled", env
        checked = 0
        for fname, name, svc in self._all_stack_services():
            if name != "knowledge-sovereign":
                continue
            env = env_map(svc)
            if "disabled" in (env.get("ADMIN_AUTH"), env.get("EVENT_AUTH")):
                checked += 1
                assert env.get("APP_ENV") in ("development", "test", "staging"), f"{fname}:{name}"
        assert checked > 0, checked

    def test_production_redis_streams_has_no_reset_user(self) -> None:
        svc = self._load_yaml_lenient(REPO_ROOT / "compose" / "mq.yaml")["services"][
            "redis-streams"
        ]
        env = svc.get("environment") or {}
        env = env if isinstance(env, dict) else dict(e.split("=", 1) for e in env if "=" in e)
        assert env.get("REDIS_STREAMS_RESET_USER") == "disabled", env
        names = [x if isinstance(x, str) else x.get("source") for x in (svc.get("secrets") or [])]
        assert "redis_streams_reset_password" not in names, names
        for fname, name, other in self._all_stack_services():
            if fname != "compose.staging.yaml":
                assert "redis_streams_reset_password" not in str(other.get("secrets", [])), (
                    f"{fname}:{name}"
                )

    def test_registration_closed_by_default(self) -> None:
        text = (REPO_ROOT / "kratos" / "kratos_template.yml").read_text()
        m = re.search(r"^    registration:\n      enabled: (\w+)", text, re.M)
        assert m is not None, text
        assert m.group(1) == "false", m.group(1)
        kratos = self.compose_auth["services"]["kratos"]["environment"]
        assert (
            "SELFSERVICE_FLOWS_REGISTRATION_ENABLED=${KRATOS_REGISTRATION_ENABLED:-false}" in kratos
        ), kratos

    def test_base_yaml_secrets_declarations(self) -> None:
        """compose/base.yaml declares new named role passwords and tokens."""
        secrets = self.compose_base.get("secrets", {})
        expected_new_secrets = [
            "redis_streams_producer_password",
            "redis_streams_consumer_password",
            "redis_cache_password",
            "redis_limiter_password",
            "rask_ingest_token",
            "inference_service_token",
            "k6_api_token",
        ]
        for s in expected_new_secrets:
            assert s in secrets, f"compose/base.yaml missing secret {s}"
            assert secrets[s]["file"] == f"../secrets/{s}.txt", secrets[s]


if __name__ == "__main__":
    unittest.main()
