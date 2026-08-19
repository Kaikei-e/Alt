"""Security contract tests for F-001..F-011 (Python PKI)."""

# ruff: noqa: TRY003, PLC0415, PTH110

from __future__ import annotations

import asyncio
import base64
import json
import os
import ssl
import tempfile
import threading
from collections.abc import Sequence
from datetime import UTC, datetime, timedelta
from pathlib import Path

import httpx
import pytest
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509 import ObjectIdentifier
from cryptography.x509.oid import ExtendedKeyUsageOID
from jwcrypto.jwa import default_max_pbkdf2_iterations
from jwcrypto.jwk import JWK
from prometheus_client import REGISTRY as DEFAULT_REGISTRY
from prometheus_client import generate_latest

from tag_generator.infra.pki.certfile import CertFile
from tag_generator.infra.pki.config import (
    MAX_RESPONSE_BYTES,
    MODE_ENABLED,
    CARejectedError,
    InsecureCAURLError,
    PasswordTooLargeError,
    PkiError,
    ProvisionerPageLimitError,
    RedirectError,
    ResponseTooLargeError,
    load_config,
)
from tag_generator.infra.pki.filesafe import MAX_PASSWORD_BYTES
from tag_generator.infra.pki.manager import Manager
from tag_generator.infra.pki.metrics import PromObserver
from tag_generator.infra.pki.native_issuer import (
    STEP_CA_PBES2_P2C,
    NativeStepCAIssuer,
    decrypt_provisioner_jwk,
    encrypt_step_ca_provisioner_jwk,
)
from tag_generator.infra.pki.start import start_enrollment
from tests.unit.infra.pki.helpers import self_signed_pem, write_password_file
from tests.unit.infra.pki.test_native_issuer import SUBJECT, FakeStepCA

NB = datetime(2026, 8, 18, tzinfo=UTC)


def _enabled(monkeypatch: pytest.MonkeyPatch, subject: str = "tag-generator") -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("CERT_SUBJECT", subject)


def test_empty_enrollment_fails_not_disabled(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", "")
    with pytest.raises(PkiError, match="PKI_ENROLLMENT"):
        load_config("tag-generator")


def test_enrollment_file_missing_fails(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", str(tmp_path / "missing"))
    with pytest.raises(PkiError, match="PKI_ENROLLMENT_FILE"):
        load_config("tag-generator")


def test_enrollment_file_empty_fails(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    path = tmp_path / "empty"
    path.write_text(" \n", encoding="utf-8")
    path.chmod(0o600)
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", str(path))
    with pytest.raises(PkiError, match="PKI_ENROLLMENT_FILE"):
        load_config("tag-generator")


def test_enrollment_file_empty_path_fails(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", "")
    with pytest.raises(PkiError, match="PKI_ENROLLMENT_FILE"):
        load_config("tag-generator")


def test_rejects_http_and_schemeless_ca_url(monkeypatch: pytest.MonkeyPatch) -> None:
    _enabled(monkeypatch)
    for raw in ("http://step-ca:9000", "step-ca:9000", "https://"):
        monkeypatch.setenv("STEP_CA_URL", raw)
        with pytest.raises(InsecureCAURLError):
            load_config("tag-generator")


def test_exact_provisioner_and_password_basename(monkeypatch: pytest.MonkeyPatch) -> None:
    _enabled(monkeypatch, "acolyte")
    monkeypatch.setenv("STEP_CA_PROVISIONER", "pki-agent-tag-generator")
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-tag-generator-jwk")
    with pytest.raises(PkiError, match="must be exactly"):
        load_config("acolyte")


def test_rejects_wrong_password_basename(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    _enabled(monkeypatch)
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", str(tmp_path / "wrong-name"))
    with pytest.raises(PkiError, match="basename"):
        load_config("tag-generator")


def test_temp_dir_allowed_when_basename_matches(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    _enabled(monkeypatch)
    path = tmp_path / "pki-agent-tag-generator-jwk"
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", str(path))
    cfg = load_config("tag-generator")
    assert cfg.password_file == str(path)


def test_file_env_symlink_rejected(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    real = tmp_path / "real"
    real.write_text("enabled\n", encoding="utf-8")
    real.chmod(0o600)
    link = tmp_path / "link"
    link.symlink_to(real)
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", str(link))
    with pytest.raises(PkiError):
        load_config("tag-generator")


def _compact_jwe_with_p2c(p2c: object, extra: dict[str, object] | None = None) -> str:
    header: dict[str, object] = {"alg": "PBES2-HS256+A128KW", "enc": "A256GCM", "p2c": p2c}
    if extra:
        header.update(extra)
    b64 = (
        base64.urlsafe_b64encode(json.dumps(header, separators=(",", ":")).encode("utf-8")).rstrip(b"=").decode("ascii")
    )
    return f"{b64}.e30.e30.e30.e30"


def test_pbes2_rejects_wrong_p2c_before_kdf() -> None:
    before = default_max_pbkdf2_iterations
    for p2c in (20_000, 599_999, 600_001, 1_000_000, "600000"):
        with pytest.raises(PkiError, match="p2c"):
            decrypt_provisioner_jwk(_compact_jwe_with_p2c(p2c), b"pw")
        assert default_max_pbkdf2_iterations == before


def test_pbes2_rejects_zip_and_unknown_header() -> None:
    before = default_max_pbkdf2_iterations
    with pytest.raises(PkiError, match="zip"):
        decrypt_provisioner_jwk(_compact_jwe_with_p2c(STEP_CA_PBES2_P2C, {"zip": "DEF"}), b"pw")
    with pytest.raises(PkiError, match="header"):
        decrypt_provisioner_jwk(_compact_jwe_with_p2c(STEP_CA_PBES2_P2C, {"foo": "x"}), b"pw")
    assert default_max_pbkdf2_iterations == before


def test_pbes2_rejects_malformed_and_oversized_header() -> None:
    before = default_max_pbkdf2_iterations
    with pytest.raises(PkiError, match="malformed"):
        decrypt_provisioner_jwk("not-a-jwe", b"pw")
    huge = "A" * ((8 << 10) + 8)
    with pytest.raises(PkiError, match="size cap"):
        decrypt_provisioner_jwk(huge, b"pw")
    assert default_max_pbkdf2_iterations == before


def test_pbes2_lock_restores_cap_under_concurrency() -> None:
    key = JWK.generate(kty="EC", crv="P-256")
    password = "concurrent-pw"
    compact = encrypt_step_ca_provisioner_jwk(key, password)
    before = default_max_pbkdf2_iterations
    errors: list[BaseException] = []

    def _worker() -> None:
        try:
            decrypt_provisioner_jwk(compact, password.encode())
        except BaseException as err:  # noqa: BLE001
            errors.append(err)

    threads = [threading.Thread(target=_worker) for _ in range(8)]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()
    assert errors == []
    assert default_max_pbkdf2_iterations == before


@pytest.mark.asyncio
async def test_rejects_redirects(tmp_path: Path) -> None:
    password = "redirect-pw"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    ca.redirect_sign = 307
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    try:
        with pytest.raises(RedirectError):
            await iss.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_http_client_requires_https(tmp_path: Path) -> None:
    iss = NativeStepCAIssuer(
        ca_url="http://127.0.0.1:1",
        root_file=str(tmp_path / "missing.pem"),
        provisioner="pki-agent-tag-generator",
        password_file=str(tmp_path / "pki-agent-tag-generator-jwk"),
    )
    with pytest.raises(InsecureCAURLError):
        await iss.issue(SUBJECT, [SUBJECT])


@pytest.mark.asyncio
async def test_rejects_tls12_only_ca(tmp_path: Path) -> None:
    password = "tls12-pw"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    ca.tls_max = ssl.TLSVersion.TLSv1_2
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    try:
        with pytest.raises(PkiError):
            await iss.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_ca_error_is_sentinel_without_body(tmp_path: Path) -> None:
    password = "sentinel-pw"
    secret = "super-secret-jwk-password-and-ott"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    ca.sign_status = 401
    ca.sign_body = json.dumps({"status": 401, "message": secret}).encode()
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    try:
        with pytest.raises(CARejectedError) as exc_info:
            await iss.issue(SUBJECT, [SUBJECT])
        assert secret not in str(exc_info.value)
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_provisioner_page_cap(tmp_path: Path) -> None:
    password = "pages-pw"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    ca.endless_provisioners = True
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    try:
        with pytest.raises(ProvisionerPageLimitError):
            await iss.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_response_size_cap(tmp_path: Path) -> None:
    password = "huge-pw"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    ca.sign_status = 201
    ca.sign_body = b"A" * (MAX_RESPONSE_BYTES + 8)
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    try:
        with pytest.raises(ResponseTooLargeError):
            await iss.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_password_symlink_rejected(tmp_path: Path) -> None:
    password = "symlink-pw"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    url, root = ca.start(tmp_path)
    real = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    link = tmp_path / "link-jwk"
    link.symlink_to(real)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(link), timeout_seconds=30
    )
    try:
        with pytest.raises(PkiError):
            await iss.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_password_size_cap(tmp_path: Path) -> None:
    password = "cap-pw"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    url, root = ca.start(tmp_path)
    path = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", "x" * (MAX_PASSWORD_BYTES + 1))
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(path), timeout_seconds=30
    )
    try:
        with pytest.raises(PasswordTooLargeError):
            await iss.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("eku",),
    [
        ([ExtendedKeyUsageOID.SERVER_AUTH],),
        ([ExtendedKeyUsageOID.CLIENT_AUTH],),
    ],
)
async def test_rejects_one_sided_eku(tmp_path: Path, eku: list[ObjectIdentifier]) -> None:
    password = "one-sided-eku"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    ca.eku = eku
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    try:
        with pytest.raises(PkiError, match="serverAuth and clientAuth"):
            await iss.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_rejects_extra_dns(tmp_path: Path) -> None:
    from cryptography import x509 as cx509

    password = "extra-dns"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    ca.mutate_sans = lambda names: [*names, cx509.DNSName("evil.example")]
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    try:
        with pytest.raises(PkiError, match="DNS SAN"):
            await iss.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_exact_ip_and_uri_sans(tmp_path: Path) -> None:
    from cryptography import x509 as cx509

    password = "typed-san"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    sans = ["127.0.0.1", "spiffe://alt.local/tag-generator"]
    try:
        cert_pem, key_pem = await iss.issue(SUBJECT, sans)
        leaf = cx509.load_pem_x509_certificate(cert_pem)
        san = leaf.extensions.get_extension_for_class(cx509.SubjectAlternativeName).value
        assert san.get_values_for_type(cx509.DNSName) == []
        assert str(san.get_values_for_type(cx509.IPAddress)[0]) == "127.0.0.1"
        assert san.get_values_for_type(cx509.UniformResourceIdentifier) == ["spiffe://alt.local/tag-generator"]
        assert key_pem
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_rejects_mismatched_leaf_key(tmp_path: Path) -> None:
    password = "key-mismatch"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )
    try:
        cert_pem, _key_pem = await iss.issue(SUBJECT, [SUBJECT])
        other = ec.generate_private_key(ec.SECP256R1())
        with pytest.raises(PkiError, match="public key"):
            iss._validate_and_encode(SUBJECT, [SUBJECT], {"crt": cert_pem.decode()}, other)
    finally:
        ca.close()


def test_certfile_rejects_symlink_dest_and_parent(tmp_path: Path) -> None:
    nb = datetime.now(tz=UTC)
    cert, key = self_signed_pem("tag-generator", nb, nb + timedelta(hours=1))

    target = tmp_path / "target.pem"
    target.write_bytes(b"nope")
    dest = tmp_path / "svc-cert.pem"
    dest.symlink_to(target)
    files = CertFile(str(dest), str(tmp_path / "svc-key.pem"))
    with pytest.raises(PkiError, match="symlink"):
        files.write(cert, key)

    real_dir = tmp_path / "real"
    real_dir.mkdir()
    link_dir = tmp_path / "link"
    link_dir.symlink_to(real_dir)
    files = CertFile(str(link_dir / "svc-cert.pem"), str(link_dir / "svc-key.pem"))
    with pytest.raises(PkiError, match="symlink"):
        files.write(cert, key)


@pytest.mark.asyncio
async def test_enroll_cancel_during_backoff_is_cancelled(tmp_path: Path) -> None:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))

    class _Fail:
        async def issue(self, subject: str, sans: Sequence[str]) -> tuple[bytes, bytes]:
            _ = (subject, sans)
            raise PkiError("CA down")

    mgr = Manager(
        subject=SUBJECT,
        sans=(SUBJECT,),
        provisioner="pki-agent-tag-generator",
        files=files,
        issuer=_Fail(),
        retry_attempts=5,
        retry_backoff_seconds=30,
    )
    task = asyncio.create_task(mgr.enroll())
    await asyncio.sleep(0.05)
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task


@pytest.mark.asyncio
async def test_rekey_does_not_use_named_tmp(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    password = "memfd-pw"
    ca = FakeStepCA("pki-agent-tag-generator", password)
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-tag-generator-jwk", password)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(pw), timeout_seconds=30
    )

    def _boom(*_args: object, **_kwargs: object) -> object:
        raise AssertionError("named tempfile must not be used for rekey mTLS")

    monkeypatch.setattr(tempfile, "mkstemp", _boom)
    try:
        cert_pem, key_pem = await iss.issue(SUBJECT, [SUBJECT])
        new_cert, new_key = await iss.rekey(cert_pem, key_pem, SUBJECT, [SUBJECT])
        assert new_cert != cert_pem
        assert new_key
    finally:
        ca.close()


def test_prom_observer_rejects_default_registry() -> None:
    with pytest.raises(TypeError, match="private"):
        PromObserver("tag-generator", DEFAULT_REGISTRY)


@pytest.mark.asyncio
async def test_start_uses_private_registry_and_ops(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_TLS_ENABLED", "true")
    monkeypatch.setenv("CERT_SUBJECT", "tag-generator")
    monkeypatch.setenv("CERT_PATH", str(tmp_path / "svc-cert.pem"))
    monkeypatch.setenv("KEY_PATH", str(tmp_path / "svc-key.pem"))
    monkeypatch.setenv("STEP_CA_URL", "https://127.0.0.1:1")
    monkeypatch.setenv("STEP_CA_ROOT_FILE", str(tmp_path / "root.pem"))
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", str(tmp_path / "pki-agent-tag-generator-jwk"))
    monkeypatch.setenv("OPS_LISTEN", "127.0.0.1:0")

    class _Issuer:
        async def issue(self, subject: str, sans: Sequence[str]) -> tuple[bytes, bytes]:
            _ = sans
            now = datetime.now(tz=UTC)
            return self_signed_pem(subject, now - timedelta(minutes=1), now + timedelta(hours=24))

    handle = await start_enrollment("tag-generator", issuer=_Issuer())
    assert handle is not None
    assert handle.ops_addr is not None
    assert handle.registry is not None
    assert handle.registry is not DEFAULT_REGISTRY
    try:
        async with httpx.AsyncClient() as client:
            health = await client.get(f"http://{handle.ops_addr}/health")
            metrics = await client.get(f"http://{handle.ops_addr}/metrics")
        assert health.status_code == 200
        assert "tag-generator" in health.text
        assert metrics.status_code == 200
        assert "pki_enrollment_healthy" in metrics.text
        leaked = generate_latest(DEFAULT_REGISTRY).decode()
        assert 'subject="tag-generator"' not in leaked or "pki_enrollment_healthy" not in leaked
    finally:
        await handle.aclose()


def test_pbes2_constant_is_step_ca_default() -> None:
    assert STEP_CA_PBES2_P2C == 600_000
    assert os.path.exists("/proc/self/fd")
