#!/usr/bin/env python3
"""PKI regression test verifying staging CA key usage and strict TLS verification.

Exercises the staging PKI mint script (e2e/_lib/mint-staging-pki.sh) to ensure
the generated CA includes critical basicConstraints=CA:TRUE and
critical keyUsage=keyCertSign,cRLSign as required by RFC 5280 and Python 3.14+
strict verification (VERIFY_X509_STRICT).
"""

import os
import shutil
import ssl
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
MINT_SCRIPT = ROOT / "e2e/_lib/mint-staging-pki.sh"

# In Python 3.14+, VERIFY_X509_STRICT is default.
# For local Python < 3.14, explicitly set X509_V_FLAG_X509_STRICT (0x20).
STRICT_FLAG = getattr(
    ssl.VerifyFlags, "VERIFY_X509_STRICT", getattr(ssl, "VERIFY_X509_STRICT", 0x20)
)


def _drive_bio_handshake(
    client_ssl: ssl.SSLObject,
    server_ssl: ssl.SSLObject,
    client_in: ssl.MemoryBIO,
    client_out: ssl.MemoryBIO,
    server_in: ssl.MemoryBIO,
    server_out: ssl.MemoryBIO,
    max_rounds: int = 50,
) -> bool:
    """Drive in-memory TLS handshake between client and server SSLObjects."""
    client_done = False
    server_done = False
    for _ in range(max_rounds):
        if not client_done:
            try:
                client_ssl.do_handshake()
                client_done = True
            except ssl.SSLWantReadError:
                pass

        client_data = client_out.read()
        if client_data:
            server_in.write(client_data)

        if not server_done:
            try:
                server_ssl.do_handshake()
                server_done = True
            except ssl.SSLWantReadError:
                pass

        server_data = server_out.read()
        if server_data:
            client_in.write(server_data)

        if client_done and server_done:
            return True
        if not client_data and not server_data:
            break
    return False


def simulate_tls_handshake(
    ca_file: Path,
    server_cert: Path,
    server_key: Path,
    server_hostname: str = "alt-data-hub",
    client_cert: Path | None = None,
    client_key: Path | None = None,
    require_client_auth: bool = False,
    strict_verification: bool = True,
) -> bool:
    """Simulate TLS / mTLS handshake with strict verification."""
    # Configure client context
    client_ctx = ssl.create_default_context(
        purpose=ssl.Purpose.SERVER_AUTH,
        cafile=str(ca_file),
    )
    if strict_verification:
        client_ctx.verify_flags |= STRICT_FLAG
    client_ctx.check_hostname = True

    if client_cert and client_key:
        client_ctx.load_cert_chain(str(client_cert), str(client_key))

    # Configure server context
    server_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    server_ctx.load_cert_chain(str(server_cert), str(server_key))

    if require_client_auth:
        server_ctx.verify_mode = ssl.CERT_REQUIRED
        server_ctx.load_verify_locations(cafile=str(ca_file))
        if strict_verification:
            server_ctx.verify_flags |= STRICT_FLAG

    # In-memory BIO pairs
    client_in = ssl.MemoryBIO()
    client_out = ssl.MemoryBIO()
    server_in = ssl.MemoryBIO()
    server_out = ssl.MemoryBIO()

    client_ssl = client_ctx.wrap_bio(
        client_in,
        client_out,
        server_side=False,
        server_hostname=server_hostname,
    )
    server_ssl = server_ctx.wrap_bio(
        server_in,
        server_out,
        server_side=True,
    )

    return _drive_bio_handshake(
        client_ssl, server_ssl, client_in, client_out, server_in, server_out
    )


class StagingPkiRegressionTests(unittest.TestCase):
    def setUp(self):
        self.tmp_dir = tempfile.mkdtemp(prefix="pki-test-", dir="/tmp")

    def tearDown(self):
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    def _mint_legacy_pki(self, target_dir: str):
        """Mint staging PKI using the unpatched/legacy CA generation (missing keyUsage)."""
        os.makedirs(target_dir, exist_ok=True)
        # Legacy CA minting command (without -addext basicConstraints and keyUsage)
        subprocess.run(
            [
                "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                "-days", "1", "-sha256", "-subj", "/CN=alt-e2e-ca",
                "-keyout", f"{target_dir}/ca-key.pem",
                "-out", f"{target_dir}/ca.pem",
            ],
            check=True,
            capture_output=True,
            timeout=10,
        )

        # Helper to mint leaves under the legacy CA
        leaf_specs = [
            {"common_name": "alt-data-hub", "extended_key_usage": "serverAuth"},
            {"common_name": "alt-backend", "extended_key_usage": "clientAuth"},
        ]
        for spec in leaf_specs:
            name = spec["common_name"]
            eku = spec["extended_key_usage"]
            csr = f"{target_dir}/{name}.csr"
            subprocess.run(
                [
                    "openssl", "req", "-newkey", "rsa:2048", "-nodes",
                    "-subj", f"/CN={name}",
                    "-keyout", f"{target_dir}/{name}-key.pem",
                    "-out", csr,
                ],
                check=True,
                capture_output=True,
                timeout=10,
            )
            ext_content = (
                f"subjectAltName=DNS:{name}\n"
                f"extendedKeyUsage={eku}\n"
                f"basicConstraints=CA:FALSE\n"
            )
            ext_file = f"{target_dir}/{name}.ext"
            Path(ext_file).write_text(ext_content)
            subprocess.run(
                [
                    "openssl", "x509", "-req", "-days", "1", "-sha256",
                    "-in", csr,
                    "-CA", f"{target_dir}/ca.pem",
                    "-CAkey", f"{target_dir}/ca-key.pem",
                    "-CAcreateserial",
                    "-extfile", ext_file,
                    "-out", f"{target_dir}/{name}.pem",
                ],
                check=True,
                capture_output=True,
                timeout=10,
            )
            os.remove(csr)
            os.remove(ext_file)

    def _mint_with_script(self, target_dir: str, server: str, *clients: str):
        """Mint PKI using the actual e2e/_lib/mint-staging-pki.sh script."""
        result = subprocess.run(
            [
                "bash",
                "-c",
                'source "$1" && shift && mint_staging_pki "$@"',
                "staging-pki-test",
                str(MINT_SCRIPT),
                target_dir,
                server,
                *clients,
            ],
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode != 0:
            self.fail(f"mint_staging_pki failed (rc={result.returncode}):\n{result.stderr}")

    def test_legacy_ca_fails_strict_verification_reproducing_ci_failure(self):
        """RED: Legacy CA lacks keyUsage extension; strict verification fails as in CI."""
        legacy_dir = os.path.join(self.tmp_dir, "legacy")
        self._mint_legacy_pki(legacy_dir)

        ca_pem = Path(legacy_dir) / "ca.pem"
        server_pem = Path(legacy_dir) / "alt-data-hub.pem"
        server_key = Path(legacy_dir) / "alt-data-hub-key.pem"

        # Verify that Python strict TLS verification raises SSLCertVerificationError
        # matching: 'CA cert does not include key usage extension'
        with self.assertRaises(ssl.SSLCertVerificationError) as ctx:
            simulate_tls_handshake(
                ca_file=ca_pem,
                server_cert=server_pem,
                server_key=server_key,
                server_hostname="alt-data-hub",
                strict_verification=True,
            )
        self.assertIn("CA cert does not include key usage extension", str(ctx.exception))

    def test_fixed_script_passes_strict_tls_and_mtls_verification(self):
        """GREEN: Patched mint script produces CA with required usage; TLS and mTLS succeed."""
        fixed_dir = os.path.join(self.tmp_dir, "fixed")
        self._mint_with_script(fixed_dir, "alt-data-hub", "alt-backend")

        ca_pem = Path(fixed_dir) / "ca.pem"
        server_pem = Path(fixed_dir) / "svc-cert.pem"
        server_key = Path(fixed_dir) / "svc-key.pem"
        client_pem = Path(fixed_dir) / "alt-backend.pem"
        client_key = Path(fixed_dir) / "alt-backend-key.pem"

        # 1. Server-auth TLS handshake with strict verification succeeds
        success = simulate_tls_handshake(
            ca_file=ca_pem,
            server_cert=server_pem,
            server_key=server_key,
            server_hostname="alt-data-hub",
            strict_verification=True,
        )
        self.assertTrue(success, "Server TLS handshake should succeed with strict verification")

        # 2. Mutual TLS (mTLS) handshake with strict verification succeeds
        mtls_success = simulate_tls_handshake(
            ca_file=ca_pem,
            server_cert=server_pem,
            server_key=server_key,
            server_hostname="alt-data-hub",
            client_cert=client_pem,
            client_key=client_key,
            require_client_auth=True,
            strict_verification=True,
        )
        self.assertTrue(mtls_success, "mTLS handshake should succeed with strict verification")

    def test_ca_has_explicit_critical_extensions(self):
        """Verify CA certificate has explicit critical basicConstraints and keyUsage."""
        fixed_dir = os.path.join(self.tmp_dir, "fixed")
        self._mint_with_script(fixed_dir, "alt-data-hub", "alt-backend")

        ca_pem = Path(fixed_dir) / "ca.pem"
        res = subprocess.run(
            ["openssl", "x509", "-in", str(ca_pem), "-text", "-noout"],
            capture_output=True,
            text=True,
            check=True,
            timeout=10,
        )
        out = res.stdout

        # basicConstraints must be CA:TRUE and critical
        self.assertIn("X509v3 Basic Constraints: critical", out)
        self.assertIn("CA:TRUE", out)

        # keyUsage must include Certificate Sign, CRL Sign and be critical
        self.assertIn("X509v3 Key Usage: critical", out)
        self.assertIn("Certificate Sign", out)
        self.assertIn("CRL Sign", out)

    def test_server_and_client_leaf_roles_and_identities(self):
        """Verify serverAuth and clientAuth purposes and leaf identities are preserved."""
        fixed_dir = os.path.join(self.tmp_dir, "fixed")
        self._mint_with_script(fixed_dir, "alt-data-hub", "alt-backend")

        ca_pem = Path(fixed_dir) / "ca.pem"
        server_pem = Path(fixed_dir) / "svc-cert.pem"
        server_key = Path(fixed_dir) / "svc-key.pem"
        client_pem = Path(fixed_dir) / "alt-backend.pem"
        client_key = Path(fixed_dir) / "alt-backend-key.pem"

        # Verify leaf purposes via openssl x509 -purpose
        srv_purpose = subprocess.run(
            ["openssl", "x509", "-in", str(server_pem), "-purpose", "-noout"],
            capture_output=True,
            text=True,
            check=True,
            timeout=10,
        ).stdout
        self.assertIn("SSL server : Yes", srv_purpose)
        self.assertIn("SSL client : No", srv_purpose)

        cli_purpose = subprocess.run(
            ["openssl", "x509", "-in", str(client_pem), "-purpose", "-noout"],
            capture_output=True,
            text=True,
            check=True,
            timeout=10,
        ).stdout
        self.assertIn("SSL client : Yes", cli_purpose)
        self.assertIn("SSL server : No", cli_purpose)

        # Verify hostnames: wrong hostname fails verification
        with self.assertRaises((ssl.SSLCertVerificationError, ssl.CertificateError)):
            simulate_tls_handshake(
                ca_file=ca_pem,
                server_cert=server_pem,
                server_key=server_key,
                server_hostname="wrong-host",
                strict_verification=True,
            )

        # Verify purpose constraint: clientAuth leaf cannot act as TLS server
        with self.assertRaises(ssl.SSLError):
            simulate_tls_handshake(
                ca_file=ca_pem,
                server_cert=client_pem,
                server_key=client_key,
                server_hostname="alt-backend",
                strict_verification=True,
            )


if __name__ == "__main__":
    unittest.main()
