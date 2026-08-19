"""Security contract tests for F-001..F-011 (sync Python PKI)."""

# ruff: noqa: PTH110

from __future__ import annotations

import base64
import json
import os
import ssl
import tempfile
import threading
import time
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from cryptography import x509 as cx509
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec, ed25519, rsa
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from jwcrypto.jwa import default_max_pbkdf2_iterations
from jwcrypto.jwk import JWK
from prometheus_client import REGISTRY as DEFAULT_REGISTRY
from prometheus_client import generate_latest

from recap_subworker.app.infra.pki.certfile import CertFile
from recap_subworker.app.infra.pki.config import (
    MAX_RESPONSE_BYTES,
    MODE_ENABLED,
    CARejectedError,
    InsecureCAURLError,
    PasswordTooLargeError,
    PkiError,
    ProvisionerPageLimitError,
    RedirectError,
    ResponseTooLargeError,
    UnsupportedIssuerAlgorithm,
    load_config,
)
from recap_subworker.app.infra.pki.ctx import CancelledError, Ctx
from recap_subworker.app.infra.pki.filesafe import MAX_PASSWORD_BYTES
from recap_subworker.app.infra.pki.manager import Manager
from recap_subworker.app.infra.pki.metrics import PromObserver
from recap_subworker.app.infra.pki.native_issuer import (
    STEP_CA_PBES2_P2C,
    NativeStepCAIssuer,
    _verify_chain,
    decrypt_provisioner_jwk,
    encrypt_step_ca_provisioner_jwk,
)
from tests.unit.pki.helpers import self_signed_pem, write_password_file
from tests.unit.pki.test_manager import FakeIssuer
from tests.unit.pki.test_native_issuer import SUBJECT, FakeStepCA, _new_issuer

NB = datetime(2026, 8, 18, tzinfo=UTC)


def _enabled(monkeypatch: pytest.MonkeyPatch, subject: str = "recap-subworker") -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", MODE_ENABLED)
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.setenv("CERT_SUBJECT", subject)


def test_empty_enrollment_fails_not_disabled(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT", "")
    with pytest.raises(PkiError, match="PKI_ENROLLMENT"):
        load_config("recap-subworker")


def test_enrollment_file_missing_fails(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", str(tmp_path / "missing"))
    with pytest.raises(PkiError, match="PKI_ENROLLMENT_FILE"):
        load_config("recap-subworker")


def test_enrollment_file_empty_fails(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    path = tmp_path / "empty"
    path.write_text(" \n", encoding="utf-8")
    path.chmod(0o600)
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", str(path))
    with pytest.raises(PkiError, match="PKI_ENROLLMENT_FILE"):
        load_config("recap-subworker")


def test_enrollment_file_empty_path_fails(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", "")
    with pytest.raises(PkiError, match="PKI_ENROLLMENT_FILE"):
        load_config("recap-subworker")


def test_enrollment_file_oversize_fails(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    path = tmp_path / "huge"
    path.write_bytes(b"enabled" + b"x" * (8 << 10))
    path.chmod(0o600)
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", str(path))
    with pytest.raises(PkiError):
        load_config("recap-subworker")


def test_rejects_http_and_schemeless_ca_url(monkeypatch: pytest.MonkeyPatch) -> None:
    _enabled(monkeypatch)
    for raw in ("http://step-ca:9000", "step-ca:9000", "https://"):
        monkeypatch.setenv("STEP_CA_URL", raw)
        with pytest.raises(InsecureCAURLError):
            load_config("recap-subworker")


def test_exact_provisioner_and_password_basename(monkeypatch: pytest.MonkeyPatch) -> None:
    _enabled(monkeypatch, "acolyte")
    monkeypatch.setenv("STEP_CA_PROVISIONER", "pki-agent-recap-subworker")
    monkeypatch.setenv(
        "STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/pki-agent-recap-subworker-jwk"
    )
    with pytest.raises(PkiError, match="must be exactly"):
        load_config("acolyte")


def test_rejects_wrong_password_basename(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    _enabled(monkeypatch)
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", str(tmp_path / "wrong-name"))
    with pytest.raises(PkiError, match="basename"):
        load_config("recap-subworker")


def test_temp_dir_allowed_when_basename_matches(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    _enabled(monkeypatch)
    path = tmp_path / "pki-agent-recap-subworker-jwk"
    monkeypatch.setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", str(path))
    cfg = load_config("recap-subworker")
    assert cfg.password_file == str(path)


def test_file_env_symlink_rejected(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    real = tmp_path / "real"
    real.write_text("enabled\n", encoding="utf-8")
    real.chmod(0o600)
    link = tmp_path / "link"
    link.symlink_to(real)
    monkeypatch.setenv("PKI_ENROLLMENT_FILE", str(link))
    with pytest.raises(PkiError):
        load_config("recap-subworker")


def _compact_jwe_with_p2c(p2c: object, extra: dict[str, object] | None = None) -> str:
    header = {"alg": "PBES2-HS256+A128KW", "enc": "A256GCM", "p2c": p2c}
    if extra:
        header.update(extra)
    b64 = (
        base64.urlsafe_b64encode(json.dumps(header, separators=(",", ":")).encode("utf-8"))
        .rstrip(b"=")
        .decode("ascii")
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
        except BaseException as err:
            errors.append(err)

    threads = [threading.Thread(target=_worker) for _ in range(8)]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()
    assert errors == []
    assert default_max_pbkdf2_iterations == before


def test_pbes2_restores_cap_on_error() -> None:
    before = default_max_pbkdf2_iterations
    key = JWK.generate(kty="EC", crv="P-256")
    compact = encrypt_step_ca_provisioner_jwk(key, "right")
    with pytest.raises(PkiError):
        decrypt_provisioner_jwk(compact, b"wrong")
    assert default_max_pbkdf2_iterations == before


def test_rejects_redirects(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "redirect-pw")
    ca.redirect_sign = 307
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(RedirectError):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_http_client_requires_https(tmp_path: Path) -> None:
    iss = NativeStepCAIssuer(
        ca_url="http://127.0.0.1:1",
        root_file=str(tmp_path / "missing.pem"),
        provisioner="pki-agent-recap-subworker",
        password_file=str(tmp_path / "pki-agent-recap-subworker-jwk"),
    )
    with pytest.raises(InsecureCAURLError):
        iss.issue(Ctx(), SUBJECT, [SUBJECT])


def test_rejects_tls12_only_ca(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "tls12-pw")
    ca.tls_max = ssl.TLSVersion.TLSv1_2
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(PkiError):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_ca_error_is_sentinel_without_body(tmp_path: Path) -> None:
    secret = "super-secret-jwk-password-and-ott"
    ca = FakeStepCA("pki-agent-recap-subworker", "sentinel-pw")
    ca.sign_status = 401
    ca.sign_body = json.dumps({"status": 401, "message": secret}).encode()
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(CARejectedError) as exc_info:
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
        assert secret not in str(exc_info.value)
    finally:
        ca.close()


def test_provisioner_page_cap(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "pages-pw")
    ca.endless_provisioners = True
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(ProvisionerPageLimitError):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_response_size_cap(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "huge-pw")
    ca.sign_status = 201
    ca.sign_body = b"A" * (MAX_RESPONSE_BYTES + 8)
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(ResponseTooLargeError):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_password_symlink_rejected(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "symlink-pw")
    url, root = ca.start(tmp_path)
    real = write_password_file(tmp_path, "pki-agent-recap-subworker-jwk", "symlink-pw")
    link = tmp_path / "link-jwk"
    link.symlink_to(real)
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(link), timeout=5
    )
    try:
        with pytest.raises(PkiError):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_password_size_cap(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "cap-pw")
    url, root = ca.start(tmp_path)
    path = write_password_file(
        tmp_path, "pki-agent-recap-subworker-jwk", "x" * (MAX_PASSWORD_BYTES + 1)
    )
    iss = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=ca.provisioner, password_file=str(path), timeout=5
    )
    try:
        with pytest.raises(PasswordTooLargeError):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_rejects_extra_dns(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "extra-dns")
    ca.mutate_sans = lambda names: [*names, cx509.DNSName("evil.example")]
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(PkiError, match="DNS SAN"):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_exact_ip_and_uri_sans(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "typed-san")
    iss = _new_issuer(ca, tmp_path)
    sans = ["127.0.0.1", "spiffe://alt.local/recap-subworker"]
    try:
        cert_pem, key_pem = iss.issue(Ctx(), SUBJECT, sans)
        leaf = cx509.load_pem_x509_certificate(cert_pem)
        san = leaf.extensions.get_extension_for_class(cx509.SubjectAlternativeName).value
        assert san.get_values_for_type(cx509.DNSName) == []
        assert str(san.get_values_for_type(cx509.IPAddress)[0]) == "127.0.0.1"
        assert san.get_values_for_type(cx509.UniformResourceIdentifier) == [
            "spiffe://alt.local/recap-subworker"
        ]
        assert key_pem
        usages = list(leaf.extensions.get_extension_for_class(cx509.ExtendedKeyUsage).value)
        assert ExtendedKeyUsageOID.SERVER_AUTH in usages
        assert ExtendedKeyUsageOID.CLIENT_AUTH in usages
    finally:
        ca.close()


def test_rejects_mismatched_leaf_key(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "key-mismatch")
    iss = _new_issuer(ca, tmp_path)
    try:
        cert_pem, _key_pem = iss.issue(Ctx(), SUBJECT, [SUBJECT])
        other = ec.generate_private_key(ec.SECP256R1())
        with pytest.raises(PkiError, match="public key"):
            iss._validate_and_encode(SUBJECT, [SUBJECT], {"crt": cert_pem.decode()}, other)
    finally:
        ca.close()


def test_certfile_rejects_symlink_dest_and_parent(tmp_path: Path) -> None:
    nb = datetime.now(tz=UTC)
    cert, key = self_signed_pem("recap-subworker", nb, nb + timedelta(hours=1))

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


def test_enroll_cancel_during_backoff_is_cancelled(tmp_path: Path) -> None:
    from recap_subworker.app.infra.pki.config import Config

    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))

    class _Fail:
        def issue(self, ctx: Ctx, subject: str, sans: list[str]) -> tuple[bytes, bytes]:
            del ctx, subject, sans
            raise PkiError("CA down")

    cfg = Config(
        mode=MODE_ENABLED,
        subject=SUBJECT,
        sans=(SUBJECT,),
        cert_path=str(files.cert_path),
        key_path=str(files.key_path),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-recap-subworker",
        password_file="/run/secrets/pki-agent-recap-subworker-jwk",
        retry_attempts=5,
        retry_backoff=30,
    )
    mgr = Manager(cfg, _Fail(), files)
    ctx = Ctx()
    done: list[BaseException | None] = []

    def _run() -> None:
        try:
            mgr.enroll(ctx)
            done.append(None)
        except BaseException as err:
            done.append(err)

    thread = threading.Thread(target=_run)
    thread.start()
    time.sleep(0.05)
    ctx.cancel()
    thread.join(timeout=5)
    assert thread.is_alive() is False
    assert isinstance(done[0], CancelledError)


def test_rekey_does_not_use_named_tmp(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "memfd-pw")
    iss = _new_issuer(ca, tmp_path)

    def _boom(*_args: object, **_kwargs: object) -> object:
        raise AssertionError("named tempfile must not be used for rekey mTLS")

    monkeypatch.setattr(tempfile, "mkstemp", _boom)
    try:
        cert_pem, key_pem = iss.issue(Ctx(), SUBJECT, [SUBJECT])
        new_cert, new_key = iss.rekey(Ctx(), cert_pem, key_pem, SUBJECT, [SUBJECT])
        assert new_cert != cert_pem
        assert new_key
    finally:
        ca.close()


def test_prom_observer_rejects_default_registry() -> None:
    with pytest.raises(TypeError, match="private"):
        PromObserver("recap-subworker", DEFAULT_REGISTRY)


def test_start_uses_private_registry_and_ops(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    import urllib.request

    from prometheus_client import CollectorRegistry

    from recap_subworker.app.infra.pki.config import Config
    from recap_subworker.app.infra.pki.start import start_with_observer

    monkeypatch.setenv("OPS_LISTEN", "127.0.0.1:0")

    cfg = Config(
        mode=MODE_ENABLED,
        subject="recap-subworker",
        sans=("recap-subworker",),
        cert_path=str(tmp_path / "svc-cert.pem"),
        key_path=str(tmp_path / "svc-key.pem"),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-recap-subworker",
        password_file=str(tmp_path / "pki-agent-recap-subworker-jwk"),
        retry_attempts=1,
        retry_backoff=0.001,
        tick_interval=3600,
    )

    class _Issuer:
        timeout = 15.0

        def issue(self, ctx: Ctx, subject: str, sans: list[str]) -> tuple[bytes, bytes]:
            del ctx, sans
            return self_signed_pem(
                subject,
                datetime.now(UTC) - timedelta(minutes=1),
                datetime.now(UTC) + timedelta(hours=24),
            )

        def close_idle_connections(self) -> None:
            return None

    registry = CollectorRegistry()
    handle = start_with_observer(
        cfg, _Issuer(), PromObserver(cfg.subject, registry), registry=registry
    )
    assert handle is not None
    assert handle.ops_addr is not None
    assert handle.registry is not DEFAULT_REGISTRY
    try:
        with urllib.request.urlopen(f"http://{handle.ops_addr}/health", timeout=2) as health:
            assert health.status == 200
            assert b"recap-subworker" in health.read()
        with urllib.request.urlopen(f"http://{handle.ops_addr}/metrics", timeout=2) as metrics:
            body = metrics.read().decode()
        assert "pki_enrollment_healthy" in body
        leaked = generate_latest(DEFAULT_REGISTRY).decode()
        assert 'subject="recap-subworker"' not in leaked or "pki_enrollment_healthy" not in leaked
    finally:
        handle.stop()


def test_stop_during_http_closes_connections(tmp_path: Path) -> None:
    ca = FakeStepCA("pki-agent-recap-subworker", "http-stop")
    ca.block_sign = threading.Event()
    iss = _new_issuer(ca, tmp_path)
    ctx = Ctx()
    done: list[BaseException | None] = []

    def _run() -> None:
        try:
            iss.issue(ctx, SUBJECT, [SUBJECT])
            done.append(None)
        except BaseException as err:
            done.append(err)

    thread = threading.Thread(target=_run)
    thread.start()
    time.sleep(0.2)
    ctx.cancel()
    iss.close_idle_connections()
    thread.join(timeout=5)
    assert thread.is_alive() is False
    assert done and isinstance(done[0], CancelledError)
    if ca.block_sign is not None:
        ca.block_sign.set()
    ca.close()


def test_stop_during_write_does_not_abandon(tmp_path: Path) -> None:
    from recap_subworker.app.infra.pki.config import Config

    files = CertFile(str(tmp_path / "b-cert.pem"), str(tmp_path / "b-key.pem"))
    started = threading.Event()
    orig = files.write

    def _slow(cert_pem: bytes, key_pem: bytes) -> None:
        started.set()
        time.sleep(0.3)
        orig(cert_pem, key_pem)

    files.write = _slow  # type: ignore[method-assign]
    cfg = Config(
        mode=MODE_ENABLED,
        subject="recap-subworker-write",
        sans=("recap-subworker-write",),
        cert_path=str(files.cert_path),
        key_path=str(files.key_path),
        ca_url="https://127.0.0.1:1",
        root_file=str(tmp_path / "root.pem"),
        provisioner="pki-agent-recap-subworker-write",
        password_file="/run/secrets/pki-agent-recap-subworker-write-jwk",
        retry_attempts=1,
        retry_backoff=0.001,
        tick_interval=3600,
    )
    mgr = Manager(
        cfg,
        FakeIssuer(
            not_before=datetime.now(UTC) - timedelta(minutes=1), lifetime=timedelta(hours=24)
        ),
        files,
    )
    ctx = Ctx()
    done: list[BaseException | None] = []

    def _enroll() -> None:
        try:
            mgr.enroll(ctx)
            done.append(None)
        except BaseException as err:
            done.append(err)

    thread = threading.Thread(target=_enroll)
    thread.start()
    assert started.wait(timeout=2)
    ctx.cancel()
    thread.join(timeout=5)
    assert thread.is_alive() is False
    assert done == [None]
    assert Path(files.cert_path).is_file()
    assert Path(files.key_path).is_file()


def test_pbes2_constant_is_step_ca_default() -> None:
    assert STEP_CA_PBES2_P2C == 600_000
    assert os.path.exists("/proc/self/fd")


def _leaf_and_ca(
    issuer_key: rsa.RSAPrivateKey | ec.EllipticCurvePrivateKey | ed25519.Ed25519PrivateKey,
    leaf_key: ec.EllipticCurvePrivateKey,
    alg: hashes.SHA256 | None,
    cn: str,
) -> tuple[cx509.Certificate, cx509.Certificate]:
    now = datetime.now(tz=UTC)
    ca_name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, f"{cn}-ca")])
    leaf_name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, cn)])
    ca_pub = issuer_key.public_key()
    ca_builder = (
        cx509.CertificateBuilder()
        .subject_name(ca_name)
        .issuer_name(ca_name)
        .public_key(ca_pub)
        .serial_number(1)
        .not_valid_before(now - timedelta(hours=1))
        .not_valid_after(now + timedelta(hours=24))
        .add_extension(cx509.BasicConstraints(ca=True, path_length=1), critical=True)
        .add_extension(
            cx509.KeyUsage(
                digital_signature=True,
                content_commitment=False,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=True,
                crl_sign=True,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .add_extension(cx509.SubjectKeyIdentifier.from_public_key(ca_pub), critical=False)
    )
    ca_cert = (
        ca_builder.sign(issuer_key, alg) if alg is not None else ca_builder.sign(issuer_key, None)
    )
    leaf_builder = (
        cx509.CertificateBuilder()
        .subject_name(leaf_name)
        .issuer_name(ca_name)
        .public_key(leaf_key.public_key())
        .serial_number(2)
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(hours=1))
        .add_extension(cx509.SubjectAlternativeName([cx509.DNSName(cn)]), critical=False)
        .add_extension(
            cx509.ExtendedKeyUsage(
                [ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH]
            ),
            critical=False,
        )
        .add_extension(
            cx509.KeyUsage(
                digital_signature=True,
                content_commitment=False,
                key_encipherment=True,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=False,
                crl_sign=False,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .add_extension(
            cx509.AuthorityKeyIdentifier.from_issuer_public_key(ca_pub),
            critical=False,
        )
        .add_extension(
            cx509.SubjectKeyIdentifier.from_public_key(leaf_key.public_key()), critical=False
        )
        .add_extension(cx509.BasicConstraints(ca=False, path_length=None), critical=True)
    )
    leaf = (
        leaf_builder.sign(issuer_key, alg)
        if alg is not None
        else leaf_builder.sign(issuer_key, None)
    )
    return leaf, ca_cert


def test_verify_chain_rsa_and_ecdsa_pass_policybuilder() -> None:
    leaf_key = ec.generate_private_key(ec.SECP256R1())
    rsa_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    ec_key = ec.generate_private_key(ec.SECP256R1())
    for issuer, alg, cn in (
        (rsa_key, hashes.SHA256(), "rsa.example"),
        (ec_key, hashes.SHA256(), "ecdsa.example"),
    ):
        leaf, ca_cert = _leaf_and_ca(issuer, leaf_key, alg, cn)
        _verify_chain(leaf, [ca_cert], [], [cn])


def test_ed25519_directly_signed_chain_cannot_bypass_policy() -> None:
    """Ed25519 issuers are fail-closed; signature-walk must not accept the leaf."""
    leaf_key = ec.generate_private_key(ec.SECP256R1())
    ed_key = ed25519.Ed25519PrivateKey.generate()
    leaf, ca_cert = _leaf_and_ca(ed_key, leaf_key, None, "ed25519.example")
    with pytest.raises(UnsupportedIssuerAlgorithm, match="unsupported issuer algorithm"):
        _verify_chain(leaf, [ca_cert], [], ["ed25519.example"])


def _ku_ca() -> cx509.KeyUsage:
    return cx509.KeyUsage(
        digital_signature=True,
        content_commitment=False,
        key_encipherment=False,
        data_encipherment=False,
        key_agreement=False,
        key_cert_sign=True,
        crl_sign=True,
        encipher_only=False,
        decipher_only=False,
    )


def _ku_ee() -> cx509.KeyUsage:
    return cx509.KeyUsage(
        digital_signature=True,
        content_commitment=False,
        key_encipherment=True,
        data_encipherment=False,
        key_agreement=False,
        key_cert_sign=False,
        crl_sign=False,
        encipher_only=False,
        decipher_only=False,
    )


def test_non_ca_issuer_is_rejected() -> None:
    now = datetime.now(tz=UTC)
    issuer_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    leaf_key = ec.generate_private_key(ec.SECP256R1())
    name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, "not-a-ca.example")])
    leaf_name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, "leaf.example")])
    issuer = (
        cx509.CertificateBuilder()
        .subject_name(name)
        .issuer_name(name)
        .public_key(issuer_key.public_key())
        .serial_number(1)
        .not_valid_before(now - timedelta(hours=1))
        .not_valid_after(now + timedelta(hours=24))
        .add_extension(cx509.BasicConstraints(ca=False, path_length=None), critical=True)
        .add_extension(_ku_ee(), critical=True)
        .add_extension(
            cx509.SubjectKeyIdentifier.from_public_key(issuer_key.public_key()), critical=False
        )
        .sign(issuer_key, hashes.SHA256())
    )
    leaf = (
        cx509.CertificateBuilder()
        .subject_name(leaf_name)
        .issuer_name(name)
        .public_key(leaf_key.public_key())
        .serial_number(2)
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(hours=1))
        .add_extension(
            cx509.SubjectAlternativeName([cx509.DNSName("leaf.example")]), critical=False
        )
        .add_extension(
            cx509.ExtendedKeyUsage(
                [ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH]
            ),
            critical=False,
        )
        .add_extension(_ku_ee(), critical=True)
        .add_extension(
            cx509.AuthorityKeyIdentifier.from_issuer_public_key(issuer_key.public_key()),
            critical=False,
        )
        .add_extension(
            cx509.SubjectKeyIdentifier.from_public_key(leaf_key.public_key()), critical=False
        )
        .add_extension(cx509.BasicConstraints(ca=False, path_length=None), critical=True)
        .sign(issuer_key, hashes.SHA256())
    )
    with pytest.raises(PkiError, match="does not verify"):
        _verify_chain(leaf, [issuer], [], ["leaf.example"])


def test_path_length_zero_rejects_intermediate() -> None:
    now = datetime.now(tz=UTC)
    root_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    int_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    leaf_key = ec.generate_private_key(ec.SECP256R1())
    root_name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, "root-pl0.example")])
    int_name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, "int.example")])
    leaf_name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, "leaf-pl.example")])
    root = (
        cx509.CertificateBuilder()
        .subject_name(root_name)
        .issuer_name(root_name)
        .public_key(root_key.public_key())
        .serial_number(1)
        .not_valid_before(now - timedelta(hours=1))
        .not_valid_after(now + timedelta(hours=24))
        .add_extension(cx509.BasicConstraints(ca=True, path_length=0), critical=True)
        .add_extension(_ku_ca(), critical=True)
        .add_extension(
            cx509.SubjectKeyIdentifier.from_public_key(root_key.public_key()), critical=False
        )
        .sign(root_key, hashes.SHA256())
    )
    intermediate = (
        cx509.CertificateBuilder()
        .subject_name(int_name)
        .issuer_name(root_name)
        .public_key(int_key.public_key())
        .serial_number(2)
        .not_valid_before(now - timedelta(hours=1))
        .not_valid_after(now + timedelta(hours=24))
        .add_extension(cx509.BasicConstraints(ca=True, path_length=0), critical=True)
        .add_extension(_ku_ca(), critical=True)
        .add_extension(
            cx509.AuthorityKeyIdentifier.from_issuer_public_key(root_key.public_key()),
            critical=False,
        )
        .add_extension(
            cx509.SubjectKeyIdentifier.from_public_key(int_key.public_key()), critical=False
        )
        .sign(root_key, hashes.SHA256())
    )
    leaf = (
        cx509.CertificateBuilder()
        .subject_name(leaf_name)
        .issuer_name(int_name)
        .public_key(leaf_key.public_key())
        .serial_number(3)
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(hours=1))
        .add_extension(
            cx509.SubjectAlternativeName([cx509.DNSName("leaf-pl.example")]), critical=False
        )
        .add_extension(
            cx509.ExtendedKeyUsage(
                [ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH]
            ),
            critical=False,
        )
        .add_extension(_ku_ee(), critical=True)
        .add_extension(
            cx509.AuthorityKeyIdentifier.from_issuer_public_key(int_key.public_key()),
            critical=False,
        )
        .add_extension(
            cx509.SubjectKeyIdentifier.from_public_key(leaf_key.public_key()), critical=False
        )
        .add_extension(cx509.BasicConstraints(ca=False, path_length=None), critical=True)
        .sign(int_key, hashes.SHA256())
    )
    with pytest.raises(PkiError, match="does not verify"):
        _verify_chain(leaf, [root], [intermediate], ["leaf-pl.example"])


def test_name_constraints_reject_leaf() -> None:
    now = datetime.now(tz=UTC)
    root_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    leaf_key = ec.generate_private_key(ec.SECP256R1())
    root_name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, "constrained-ca.example")])
    leaf_name = cx509.Name([cx509.NameAttribute(NameOID.COMMON_NAME, "evil.example")])
    root = (
        cx509.CertificateBuilder()
        .subject_name(root_name)
        .issuer_name(root_name)
        .public_key(root_key.public_key())
        .serial_number(1)
        .not_valid_before(now - timedelta(hours=1))
        .not_valid_after(now + timedelta(hours=24))
        .add_extension(cx509.BasicConstraints(ca=True, path_length=0), critical=True)
        .add_extension(_ku_ca(), critical=True)
        .add_extension(
            cx509.NameConstraints(
                permitted_subtrees=[cx509.DNSName(".allowed.example")],
                excluded_subtrees=None,
            ),
            critical=True,
        )
        .add_extension(
            cx509.SubjectKeyIdentifier.from_public_key(root_key.public_key()), critical=False
        )
        .sign(root_key, hashes.SHA256())
    )
    leaf = (
        cx509.CertificateBuilder()
        .subject_name(leaf_name)
        .issuer_name(root_name)
        .public_key(leaf_key.public_key())
        .serial_number(2)
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(hours=1))
        .add_extension(
            cx509.SubjectAlternativeName([cx509.DNSName("evil.example")]), critical=False
        )
        .add_extension(
            cx509.ExtendedKeyUsage(
                [ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH]
            ),
            critical=False,
        )
        .add_extension(_ku_ee(), critical=True)
        .add_extension(
            cx509.AuthorityKeyIdentifier.from_issuer_public_key(root_key.public_key()),
            critical=False,
        )
        .add_extension(
            cx509.SubjectKeyIdentifier.from_public_key(leaf_key.public_key()), critical=False
        )
        .add_extension(cx509.BasicConstraints(ca=False, path_length=None), critical=True)
        .sign(root_key, hashes.SHA256())
    )
    with pytest.raises(PkiError, match="does not verify"):
        _verify_chain(leaf, [root], [], ["evil.example"])
