"""Isolated disposable step-ca:0.30.2. Never talks to Alt compose CA.

Run:
  PKI_NATIVE_LIVE_CA=1 uv run pytest tests/unit/infra/pki/test_live_ca.py -v --no-cov
Cleanup is guaranteed (60s budget).
"""

# ruff: noqa: ASYNC221, TRY300, PLR0915

from __future__ import annotations

import os
import shutil
import ssl
import subprocess
import threading
import time
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from cryptography import x509
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

from acolyte.infra.pki.certfile import CertFile
from acolyte.infra.pki.manager import Manager
from acolyte.infra.pki.native_issuer import NativeStepCAIssuer, _verify_chain

pytestmark = pytest.mark.skipif(
    os.environ.get("PKI_NATIVE_LIVE_CA") != "1",
    reason="set PKI_NATIVE_LIVE_CA=1 to run isolated disposable step-ca; never talks to Alt compose CA",
)

LIVE_IMAGE = "smallstep/step-ca:0.30.2"
LIVE_CONTAINER = "alt-pki-acolyte-itest"
LIVE_NETWORK = "alt-pki-acolyte-itest-net"
LIVE_HOST_PORT = "19010"
LIVE_CA_PASSWORD = "itest-only"
LIVE_JWK_PASSWORD = "itest-jwk-acolyte-orchestrator"
LIVE_PROVISIONER = "pki-agent-acolyte-orchestrator"
LIVE_SUBJECT = "acolyte-orchestrator"


def _docker(*args: str, timeout: float = 20) -> str:
    result = subprocess.run(
        ["docker", *args],
        check=False,
        capture_output=True,
        text=True,
        timeout=timeout,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stdout + result.stderr)
    return result.stdout + result.stderr


@pytest.mark.asyncio
async def test_native_issuer_disposable_live_ca(tmp_path: Path) -> None:
    if shutil.which("docker") is None:
        pytest.fail("docker is required for the live gate")

    subprocess.run(["docker", "rm", "-f", LIVE_CONTAINER], check=False, capture_output=True)
    subprocess.run(["docker", "network", "rm", LIVE_NETWORK], check=False, capture_output=True)

    def _cleanup() -> None:
        subprocess.run(["docker", "rm", "-f", LIVE_CONTAINER], check=False, capture_output=True)
        subprocess.run(["docker", "network", "rm", LIVE_NETWORK], check=False, capture_output=True)

    watchdog = threading.Timer(60, _cleanup)
    watchdog.daemon = True
    watchdog.start()
    try:
        _docker("network", "create", LIVE_NETWORK)
        _docker(
            "run",
            "--rm",
            "-d",
            "--name",
            LIVE_CONTAINER,
            "--network",
            LIVE_NETWORK,
            "-p",
            f"127.0.0.1:{LIVE_HOST_PORT}:9000",
            "-e",
            "DOCKER_STEPCA_INIT_NAME=alt-itest",
            "-e",
            "DOCKER_STEPCA_INIT_DNS_NAMES=localhost,127.0.0.1",
            "-e",
            f"DOCKER_STEPCA_INIT_PASSWORD={LIVE_CA_PASSWORD}",
            "-e",
            f"DOCKER_STEPCA_INIT_WITH_CA_URL=https://127.0.0.1:{LIVE_HOST_PORT}",
            LIVE_IMAGE,
        )
        root_file = tmp_path / "root_ca.crt"
        _wait_root(root_file)
        _add_provisioner()
        password_file = tmp_path / "pki-agent-acolyte-orchestrator-jwk"
        password_file.write_text(LIVE_JWK_PASSWORD + "\n", encoding="utf-8")
        password_file.chmod(0o400)

        ca_url = f"https://127.0.0.1:{LIVE_HOST_PORT}"
        iss = NativeStepCAIssuer(
            ca_url=ca_url,
            root_file=str(root_file),
            provisioner=LIVE_PROVISIONER,
            password_file=str(password_file),
            timeout_seconds=30,
        )
        cert_pem, key_pem = await iss.issue(LIVE_SUBJECT, [LIVE_SUBJECT])
        leaf1 = _must_live_leaf(cert_pem, key_pem, root_file)

        rekey_cert, rekey_key = await iss.rekey(cert_pem, key_pem, LIVE_SUBJECT, [LIVE_SUBJECT])
        leaf2 = _must_live_leaf(rekey_cert, rekey_key, root_file)
        assert leaf1.public_bytes(Encoding.DER) != leaf2.public_bytes(Encoding.DER)
        assert leaf1.public_key().public_bytes(
            Encoding.DER, PublicFormat.SubjectPublicKeyInfo
        ) != leaf2.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)

        files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
        files.write(rekey_cert, rekey_key)
        mgr = Manager(
            subject=LIVE_SUBJECT,
            sans=(LIVE_SUBJECT,),
            provisioner=LIVE_PROVISIONER,
            files=files,
            issuer=iss,
            renew_at_fraction=0.66,
        )
        near = leaf2.not_valid_before_utc + (leaf2.not_valid_after_utc - leaf2.not_valid_before_utc) * 3 / 4
        mgr._now = lambda: near
        state = await mgr.tick()
        assert state.value in {"fresh", "near_expiry"}
        after_rekey = files.cert_path.read_bytes()
        leaf3 = _must_live_leaf(after_rekey, files.key_path.read_bytes(), root_file)
        assert leaf2.public_bytes(Encoding.DER) != leaf3.public_bytes(Encoding.DER)

        expired = leaf3.not_valid_after_utc + timedelta(minutes=1)
        mgr._now = lambda: expired
        await mgr.tick()
        after_expire = files.cert_path.read_bytes()
        _must_live_leaf(after_expire, files.key_path.read_bytes(), root_file)
        assert after_expire != after_rekey

        files.cert_path.unlink()
        files.key_path.unlink()
        mgr._now = lambda: datetime.now(tz=UTC)
        await mgr.tick()
        after_missing = files.cert_path.read_bytes()
        _must_live_leaf(after_missing, files.key_path.read_bytes(), root_file)
    finally:
        watchdog.cancel()
        _cleanup()


def _wait_root(dest: Path) -> None:
    deadline = time.monotonic() + 20
    last = ""
    while time.monotonic() < deadline:
        try:
            _docker("exec", LIVE_CONTAINER, "test", "-f", "/home/step/certs/root_ca.crt")
            _docker("cp", f"{LIVE_CONTAINER}:/home/step/certs/root_ca.crt", str(dest))
            return
        except RuntimeError as err:
            last = str(err)
            time.sleep(0.2)
    logs = ""
    try:
        logs = _docker("logs", LIVE_CONTAINER)
    except RuntimeError:
        logs = ""
    pytest.fail(f"CA root did not appear: {last}\nlogs:\n{logs}")


def _add_provisioner() -> None:
    script = f"""set -euo pipefail
printf '%s\\n' {LIVE_JWK_PASSWORD!r} > /tmp/pki-agent-acolyte-orchestrator-jwk
chmod 400 /tmp/pki-agent-acolyte-orchestrator-jwk
step ca provisioner add {LIVE_PROVISIONER!r} --type JWK --create \\
  --password-file /tmp/pki-agent-acolyte-orchestrator-jwk \\
  --ca-config /home/step/config/ca.json
kill -HUP 1
"""
    _docker("exec", LIVE_CONTAINER, "sh", "-c", script, timeout=30)
    deadline = time.monotonic() + 8
    last = ""
    while time.monotonic() < deadline:
        try:
            out = _docker(
                "exec",
                LIVE_CONTAINER,
                "step",
                "ca",
                "provisioner",
                "list",
                "--ca-url",
                "https://localhost:9000",
                "--root",
                "/home/step/certs/root_ca.crt",
            )
            if LIVE_PROVISIONER in out:
                return
            last = out
        except RuntimeError as err:
            last = str(err)
        time.sleep(0.25)
    pytest.fail(f"provisioner {LIVE_PROVISIONER} not served after SIGHUP: {last}")


def _must_live_leaf(cert_pem: bytes, key_pem: bytes, root_file: Path) -> x509.Certificate:
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    cert_path = root_file.parent / "leaf-check-cert.pem"
    key_path = root_file.parent / "leaf-check-key.pem"
    cert_path.write_bytes(cert_pem)
    key_path.write_bytes(key_pem)
    ctx.load_cert_chain(str(cert_path), str(key_path))
    leaf = x509.load_pem_x509_certificate(cert_pem)
    cn = leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value
    assert cn == LIVE_SUBJECT
    san = leaf.extensions.get_extension_for_class(x509.SubjectAlternativeName)
    assert LIVE_SUBJECT in san.value.get_values_for_type(x509.DNSName)
    roots = x509.load_pem_x509_certificates(root_file.read_bytes())
    intermediates = [
        c
        for c in x509.load_pem_x509_certificates(cert_pem)
        if c.public_bytes(Encoding.DER) != leaf.public_bytes(Encoding.DER)
    ]
    _verify_chain(leaf, roots, intermediates, [LIVE_SUBJECT])
    eku = leaf.extensions.get_extension_for_class(x509.ExtendedKeyUsage)
    assert ExtendedKeyUsageOID.SERVER_AUTH in eku.value
    assert ExtendedKeyUsageOID.CLIENT_AUTH in eku.value
    return leaf
