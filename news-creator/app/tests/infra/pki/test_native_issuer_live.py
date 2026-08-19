"""Isolated live step-ca:0.30.2 gate. Default suite skips; never talks to Alt compose CA."""

from __future__ import annotations

import http.client
import json
import os
import shutil
import ssl
import subprocess
import time
from datetime import UTC, datetime, timedelta
from pathlib import Path
from urllib.parse import urlparse

import pytest
from cryptography import x509
from cryptography.hazmat.primitives.asymmetric import ec

from news_creator.infra.pki.certfile import CertFile
from news_creator.infra.pki.config import MODE_ENABLED, Config
from news_creator.infra.pki.ctx import Ctx
from news_creator.infra.pki.manager import Manager
from news_creator.infra.pki.native_issuer import NativeStepCAIssuer
from news_creator.infra.pki.state import State

LIVE_CA_IMAGE = "smallstep/step-ca:0.30.2"
LIVE_CA_CONTAINER = "alt-pki-native-itest-news-creator"
LIVE_CA_NETWORK = "alt-pki-native-itest-news-creator-net"
LIVE_CA_HOST_PORT = "19012"
LIVE_CA_PASSWORD = "itest-only"
LIVE_JWK_PASSWORD = "itest-jwk-news-creator"
LIVE_PROVISIONER = "pki-agent-news-creator"
LIVE_SUBJECT = "news-creator"


def _docker(*args: str, timeout: float = 30) -> str:
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


@pytest.mark.skipif(
    os.getenv("PKI_NATIVE_LIVE_CA") != "1",
    reason="set PKI_NATIVE_LIVE_CA=1 to run isolated disposable step-ca; never talks to Alt compose CA",
)
def test_native_step_ca_issuer_disposable_live_ca(tmp_path: Path) -> None:
    """PKI_NATIVE_LIVE_CA=1 pytest tests/unit/pki/test_native_issuer_live.py -q  (60s, always cleanup)."""
    if shutil.which("docker") is None:
        pytest.fail("docker is required for the live gate")
    deadline = time.monotonic() + 50
    try:
        subprocess.run(
            ["docker", "rm", "-f", LIVE_CA_CONTAINER], check=False, capture_output=True
        )
        subprocess.run(
            ["docker", "network", "rm", LIVE_CA_NETWORK],
            check=False,
            capture_output=True,
        )
        _docker("network", "create", LIVE_CA_NETWORK)
        _docker(
            "run",
            "--rm",
            "-d",
            "--name",
            LIVE_CA_CONTAINER,
            "--network",
            LIVE_CA_NETWORK,
            "-p",
            f"127.0.0.1:{LIVE_CA_HOST_PORT}:9000",
            "-e",
            "DOCKER_STEPCA_INIT_NAME=alt-itest",
            "-e",
            "DOCKER_STEPCA_INIT_DNS_NAMES=localhost,127.0.0.1",
            "-e",
            f"DOCKER_STEPCA_INIT_PASSWORD={LIVE_CA_PASSWORD}",
            "-e",
            f"DOCKER_STEPCA_INIT_WITH_CA_URL=https://127.0.0.1:{LIVE_CA_HOST_PORT}",
            LIVE_CA_IMAGE,
        )
        root_file = tmp_path / "root_ca.crt"
        _wait_live_ca_root(root_file, deadline)
        _add_live_jwk_provisioner(deadline)
        password_file = tmp_path / "pki-agent-news-creator-jwk"
        password_file.write_text(LIVE_JWK_PASSWORD + "\n")
        password_file.chmod(0o400)
        ca_url = f"https://127.0.0.1:{LIVE_CA_HOST_PORT}"
        _wait_provisioner_http(ca_url, root_file, deadline)
        iss = NativeStepCAIssuer(
            ca_url=ca_url,
            root_file=str(root_file),
            provisioner=LIVE_PROVISIONER,
            password_file=str(password_file),
            timeout=10,
        )
        ctx = Ctx(timeout=max(1.0, deadline - time.monotonic()))
        cert_pem, key_pem = iss.issue(ctx, LIVE_SUBJECT, [LIVE_SUBJECT])
        leaf1 = _must_live_leaf(cert_pem, key_pem, root_file)
        rekey_cert, rekey_key = iss.rekey(
            ctx, cert_pem, key_pem, LIVE_SUBJECT, [LIVE_SUBJECT]
        )
        leaf2 = _must_live_leaf(rekey_cert, rekey_key, root_file)
        assert leaf1.public_bytes(
            encoding=__import__(
                "cryptography"
            ).hazmat.primitives.serialization.Encoding.DER
        ) != leaf2.public_bytes(
            encoding=__import__(
                "cryptography"
            ).hazmat.primitives.serialization.Encoding.DER
        )
        assert not _public_key_equal(leaf1, leaf2)

        files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
        now_holder = {"t": datetime.now(UTC)}
        mgr = Manager(
            Config(
                mode=MODE_ENABLED,
                subject=LIVE_SUBJECT,
                sans=(LIVE_SUBJECT,),
                cert_path=str(files.cert_path),
                key_path=str(files.key_path),
                ca_url=ca_url,
                root_file=str(root_file),
                provisioner=LIVE_PROVISIONER,
                password_file=str(password_file),
                renew_at_fraction=0.66,
            ),
            iss,
            files,
            now=lambda: now_holder["t"],
        )
        files.write(rekey_cert, rekey_key)
        near = (
            leaf2.not_valid_before_utc
            + (leaf2.not_valid_after_utc - leaf2.not_valid_before_utc) * 3 / 4
        )
        now_holder["t"] = near
        state = mgr.tick(ctx)
        assert state in {State.FRESH, State.NEAR_EXPIRY}
        after_rekey = Path(files.cert_path).read_bytes()
        leaf3 = _must_live_leaf(
            after_rekey, Path(files.key_path).read_bytes(), root_file
        )
        assert leaf2.public_bytes(
            __import__("cryptography").hazmat.primitives.serialization.Encoding.DER
        ) != leaf3.public_bytes(
            __import__("cryptography").hazmat.primitives.serialization.Encoding.DER
        )

        now_holder["t"] = leaf3.not_valid_after_utc + timedelta(minutes=1)
        mgr.tick(ctx)
        after_expire = Path(files.cert_path).read_bytes()
        _must_live_leaf(after_expire, Path(files.key_path).read_bytes(), root_file)
        assert after_expire != leaf3.public_bytes(
            __import__("cryptography").hazmat.primitives.serialization.Encoding.PEM
        )

        Path(files.cert_path).unlink()
        Path(files.key_path).unlink()
        now_holder["t"] = datetime.now(UTC)
        mgr.tick(ctx)
        after_missing = Path(files.cert_path).read_bytes()
        _must_live_leaf(after_missing, Path(files.key_path).read_bytes(), root_file)
    finally:
        subprocess.run(
            ["docker", "rm", "-f", LIVE_CA_CONTAINER], check=False, capture_output=True
        )
        subprocess.run(
            ["docker", "network", "rm", LIVE_CA_NETWORK],
            check=False,
            capture_output=True,
        )


def _wait_live_ca_root(dest: Path, deadline: float) -> None:
    last = ""
    while time.monotonic() < deadline:
        probe = subprocess.run(
            [
                "docker",
                "exec",
                LIVE_CA_CONTAINER,
                "test",
                "-f",
                "/home/step/certs/root_ca.crt",
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        if probe.returncode == 0:
            subprocess.run(
                [
                    "docker",
                    "cp",
                    f"{LIVE_CA_CONTAINER}:/home/step/certs/root_ca.crt",
                    str(dest),
                ],
                check=True,
                capture_output=True,
            )
            return
        last = probe.stderr
        time.sleep(0.2)
    logs = subprocess.run(
        ["docker", "logs", LIVE_CA_CONTAINER],
        check=False,
        capture_output=True,
        text=True,
    )
    pytest.fail(f"CA root did not appear: {last}\nlogs:\n{logs.stdout}{logs.stderr}")


def _add_live_jwk_provisioner(deadline: float) -> None:
    script = f"""set -euo pipefail
printf '%s\\n' {LIVE_JWK_PASSWORD!r} > /tmp/pki-agent-news-creator-jwk
chmod 400 /tmp/pki-agent-news-creator-jwk
step ca provisioner add {LIVE_PROVISIONER!r} --type JWK --create \\
  --password-file /tmp/pki-agent-news-creator-jwk \\
  --ca-config /home/step/config/ca.json
kill -HUP 1
"""
    subprocess.run(
        ["docker", "exec", LIVE_CA_CONTAINER, "sh", "-c", script],
        check=True,
        capture_output=True,
        text=True,
        timeout=20,
    )
    while time.monotonic() < deadline:
        listed = subprocess.run(
            [
                "docker",
                "exec",
                LIVE_CA_CONTAINER,
                "step",
                "ca",
                "provisioner",
                "list",
                "--ca-url",
                "https://localhost:9000",
                "--root",
                "/home/step/certs/root_ca.crt",
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        if listed.returncode == 0 and LIVE_PROVISIONER in listed.stdout:
            return
        time.sleep(0.25)
    pytest.fail(f"provisioner {LIVE_PROVISIONER} not served after SIGHUP")


def _wait_provisioner_http(ca_url: str, root_file: Path, deadline: float) -> None:
    last = ""
    while time.monotonic() < deadline:
        names: list[str] = []
        try:
            ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
            ctx.minimum_version = ssl.TLSVersion.TLSv1_2
            ctx.load_verify_locations(cafile=str(root_file))
            parsed = urlparse(ca_url)
            for path in ("/provisioners?limit=100", "/1.0/provisioners?limit=100"):
                conn = http.client.HTTPSConnection(
                    parsed.hostname, parsed.port, context=ctx, timeout=2
                )
                try:
                    conn.request("GET", path)
                    resp = conn.getresponse()
                    body = resp.read()
                finally:
                    conn.close()
                if resp.status == 404:
                    continue
                last = f"status={resp.status} bytes={len(body)}"
                if resp.status >= 400:
                    continue
                payload = json.loads(body.decode("utf-8"))
                for item in payload.get("provisioners") or []:
                    name = str(item.get("name") or "")
                    names.append(name)
                    if name == LIVE_PROVISIONER and item.get("encryptedKey"):
                        return
            last = f"{last} names={names}"
        except (OSError, ssl.SSLError, ValueError, json.JSONDecodeError) as exc:
            last = str(exc)
        time.sleep(0.25)
    pytest.fail(
        f"provisioner {LIVE_PROVISIONER} missing encryptedKey on HTTP API: {last}"
    )


def _must_live_leaf(
    cert_pem: bytes, key_pem: bytes, root_file: Path
) -> x509.Certificate:
    import tempfile

    from cryptography.hazmat.primitives import serialization

    with (
        tempfile.NamedTemporaryFile(delete=False) as cf,
        tempfile.NamedTemporaryFile(delete=False) as kf,
    ):
        cf.write(cert_pem)
        kf.write(key_pem)
        cname, kname = cf.name, kf.name
    try:
        ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER).load_cert_chain(cname, kname)
    finally:
        Path(cname).unlink(missing_ok=True)
        Path(kname).unlink(missing_ok=True)
    leaf = x509.load_pem_x509_certificate(cert_pem)
    cn = leaf.subject.get_attributes_for_oid(x509.oid.NameOID.COMMON_NAME)[0].value
    assert cn == LIVE_SUBJECT
    from news_creator.infra.pki.native_issuer import _assert_sans, _verify_chain

    _assert_sans(leaf, [LIVE_SUBJECT])
    root = x509.load_pem_x509_certificate(root_file.read_bytes())
    rest = cert_pem
    intermediates: list[x509.Certificate] = []
    while b"BEGIN CERTIFICATE" in rest:
        cert = x509.load_pem_x509_certificate(rest)
        der = cert.public_bytes(serialization.Encoding.DER)
        if der != leaf.public_bytes(serialization.Encoding.DER):
            intermediates.append(cert)
        idx = rest.find(b"-----END CERTIFICATE-----")
        rest = rest[idx + len(b"-----END CERTIFICATE-----") :]
    _verify_chain(leaf, [root], intermediates, [LIVE_SUBJECT])
    return leaf


def _public_key_equal(a: x509.Certificate, b: x509.Certificate) -> bool:
    ak = a.public_key()
    bk = b.public_key()
    if not isinstance(ak, ec.EllipticCurvePublicKey) or not isinstance(
        bk, ec.EllipticCurvePublicKey
    ):
        return False
    return ak.public_numbers() == bk.public_numbers()
