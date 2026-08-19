"""Native step-ca issuer against an in-process TLS fake CA (no step CLI)."""

# ruff: noqa: TRY003, PLC0415

from __future__ import annotations

import asyncio
import json
import ssl
import threading
import time
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from ipaddress import ip_address
from pathlib import Path
from typing import Any

import httpx
import pytest
from cryptography import x509
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.serialization import Encoding, NoEncryption, PrivateFormat
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from jwcrypto import jwt
from jwcrypto.jwk import JWK

from acolyte.infra.pki.config import PkiError, SharedProvisionerError
from acolyte.infra.pki.native_issuer import NativeStepCAIssuer, decrypt_provisioner_jwk, encrypt_step_ca_provisioner_jwk
from tests.unit.infra.pki.helpers import write_password_file

SUBJECT = "acolyte-orchestrator"
PROVISIONER = "pki-agent-acolyte-orchestrator"


def _encrypt_jwk(key: JWK, password: str) -> str:
    return encrypt_step_ca_provisioner_jwk(key, password)


def _generate_ca() -> tuple[x509.Certificate, ec.EllipticCurvePrivateKey]:
    key = ec.generate_private_key(ec.SECP256R1())
    name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "alt-test-step-ca")])
    tmpl = (
        x509.CertificateBuilder()
        .subject_name(name)
        .issuer_name(name)
        .public_key(key.public_key())
        .serial_number(1)
        .not_valid_before(datetime.now(tz=UTC) - timedelta(hours=1))
        .not_valid_after(datetime.now(tz=UTC) + timedelta(hours=24))
        .add_extension(x509.BasicConstraints(ca=True, path_length=1), critical=True)
        .add_extension(
            x509.KeyUsage(
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
        .add_extension(
            x509.SubjectAlternativeName([x509.DNSName("localhost"), x509.IPAddress(ip_address("127.0.0.1"))]),
            critical=False,
        )
        .add_extension(x509.SubjectKeyIdentifier.from_public_key(key.public_key()), critical=False)
    )
    cert = tmpl.sign(key, hashes.SHA256())
    return cert, key


class FakeStepCA:
    def __init__(self, provisioner: str, password: str) -> None:
        self.provisioner = provisioner
        self.password = password
        self.jwk = JWK.generate(kty="EC", crv="P-256", alg="ES256", use="sig")
        self.enc_key = _encrypt_jwk(self.jwk, password)
        self.ca_cert, self.ca_key = _generate_ca()
        self.seen: list[tuple[str, str, bytes]] = []
        self.used_jti: set[str] = set()
        self.sign_delay = 0.0
        self.block_sign: threading.Event | None = None
        self.malformed_sign = False
        self.mutate_leaf: Any = None
        self.mutate_sans: Any = None
        self.eku = [ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH]
        self.reject_expired = True
        self.reject_reuse = True
        self.redirect_sign = 0
        self.sign_status = 0
        self.sign_body = b""
        self.endless_provisioners = False
        self.tls_max: ssl.TLSVersion | None = None
        self._httpd: ThreadingHTTPServer | None = None
        self._thread: threading.Thread | None = None

    def start(self, tmp_path: Path) -> tuple[str, str]:
        ca = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, format: str, *args: object) -> None:
                _ = (format, args)

            def do_GET(self) -> None:
                ca._record(self)
                if self.path.split("?", 1)[0] == "/health":
                    self._json(200, {"status": "ok"})
                    return
                if self.path.split("?", 1)[0] == "/provisioners":
                    if ca.endless_provisioners:
                        self._json(200, {"provisioners": [], "nextCursor": "more"})
                        return
                    pub = json.loads(ca.jwk.export_public())
                    self._json(
                        200,
                        {
                            "provisioners": [
                                {
                                    "type": "JWK",
                                    "name": ca.provisioner,
                                    "key": pub,
                                    "encryptedKey": ca.enc_key,
                                }
                            ]
                        },
                    )
                    return
                self.send_error(404)

            def do_POST(self) -> None:
                ca._record(self)
                path = self.path.split("?", 1)[0]
                if path in {"/sign", "/1.0/sign"}:
                    if ca.redirect_sign:
                        self.send_response(ca.redirect_sign)
                        self.send_header("Location", "https://evil.example/sign")
                        self.end_headers()
                        return
                    ca._handle_sign(self)
                    return
                if path == "/rekey":
                    ca._handle_rekey(self)
                    return
                if path == "/renew":
                    self.send_error(401, "renew requires a valid client certificate")
                    return
                self.send_error(404)

            def _json(self, status: int, body: dict[str, object]) -> None:
                raw = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

        httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        ctx.minimum_version = ssl.TLSVersion.TLSv1_3
        if self.tls_max is not None:
            ctx.minimum_version = ssl.TLSVersion.TLSv1_2
            ctx.maximum_version = self.tls_max
        cert_path = tmp_path / "ca-tls.pem"
        key_path = tmp_path / "ca-tls.key"
        cert_path.write_bytes(self.ca_cert.public_bytes(Encoding.PEM))
        key_path.write_bytes(self.ca_key.private_bytes(Encoding.PEM, PrivateFormat.PKCS8, NoEncryption()))
        ctx.load_cert_chain(certfile=str(cert_path), keyfile=str(key_path))
        ctx.verify_mode = ssl.CERT_OPTIONAL
        ctx.load_verify_locations(cafile=str(cert_path))
        httpd.socket = ctx.wrap_socket(httpd.socket, server_side=True)
        self._httpd = httpd
        thread = threading.Thread(target=httpd.serve_forever, daemon=True)
        thread.start()
        self._thread = thread
        root_file = tmp_path / "root.pem"
        root_file.write_bytes(self.ca_cert.public_bytes(Encoding.PEM))
        port = int(httpd.server_address[1])
        return f"https://127.0.0.1:{port}", str(root_file)

    def close(self) -> None:
        if self.block_sign is not None:
            self.block_sign.set()
        if self._httpd is not None:
            stopper = threading.Thread(target=self._httpd.shutdown, daemon=True)
            stopper.start()
            stopper.join(timeout=1)
            self._httpd.server_close()
            self._httpd = None
        if self._thread is not None:
            self._thread.join(timeout=1)
            self._thread = None

    def _record(self, handler: BaseHTTPRequestHandler) -> None:
        length = int(handler.headers.get("Content-Length", "0"))
        body = handler.rfile.read(length) if length else b""
        handler.close_connection = False
        # stash body for POST handlers
        handler._pki_body = body  # type: ignore[attr-defined]
        self.seen.append((handler.command, handler.path.split("?", 1)[0], body))

    def last_sign(self) -> tuple[str, str, bytes]:
        for rec in reversed(self.seen):
            if rec[1] in {"/sign", "/1.0/sign"}:
                return rec
        return ("", "", b"")

    def last_rekey(self) -> tuple[str, str, bytes]:
        for rec in reversed(self.seen):
            if rec[1] == "/rekey":
                return rec
        return ("", "", b"")

    def _handle_sign(self, handler: BaseHTTPRequestHandler) -> None:
        if self.sign_delay:
            time.sleep(self.sign_delay)
        if self.block_sign is not None:
            self.block_sign.wait(timeout=10)
        if self.malformed_sign:
            handler.send_response(201)
            handler.send_header("Content-Type", "application/json")
            handler.end_headers()
            handler.wfile.write(b"{not-json")
            return
        if self.sign_body:
            status = self.sign_status or 201
            handler.send_response(status)
            handler.send_header("Content-Type", "application/json")
            handler.send_header("Content-Length", str(len(self.sign_body)))
            handler.end_headers()
            handler.wfile.write(self.sign_body)
            return
        body = getattr(handler, "_pki_body", b"")
        req = json.loads(body.decode())
        try:
            claims = self.verify_ott(req["ott"])
        except ValueError as err:
            handler.send_error(401, str(err))
            return
        csr = x509.load_pem_x509_csr(req["csr"].encode())
        if csr.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value != claims["sub"]:
            handler.send_error(400, "csr subject mismatch")
            return
        leaf = self._sign_csr(csr)
        self._write_sign(handler, leaf)

    def _handle_rekey(self, handler: BaseHTTPRequestHandler) -> None:
        ssl_sock = handler.connection
        try:
            peer_der = ssl_sock.getpeercert(binary_form=True)
        except (
            ValueError,
            AttributeError,
        ):
            peer_der = None
        if not peer_der:
            handler.send_error(400, "missing client certificate")
            return
        body = getattr(handler, "_pki_body", b"")
        req = json.loads(body.decode())
        csr = x509.load_pem_x509_csr(req["csr"].encode())
        leaf = self._sign_csr(csr)
        self._write_sign(handler, leaf)

    def _write_sign(self, handler: BaseHTTPRequestHandler, leaf: x509.Certificate) -> None:
        leaf_pem = leaf.public_bytes(Encoding.PEM).decode()
        ca_pem = self.ca_cert.public_bytes(Encoding.PEM).decode()
        raw = json.dumps({"crt": leaf_pem, "ca": ca_pem, "certChain": [leaf_pem, ca_pem]}).encode()
        handler.send_response(201)
        handler.send_header("Content-Type", "application/json")
        handler.send_header("Content-Length", str(len(raw)))
        handler.end_headers()
        handler.wfile.write(raw)

    def _sign_csr(self, csr: x509.CertificateSigningRequest) -> x509.Certificate:
        names: list[x509.GeneralName] = []
        try:
            ext = csr.extensions.get_extension_for_class(x509.SubjectAlternativeName)
            names = list(ext.value)
        except x509.ExtensionNotFound:
            names = [x509.DNSName(SUBJECT)]
        if self.mutate_sans is not None:
            names = list(self.mutate_sans(names))
        builder = (
            x509.CertificateBuilder()
            .subject_name(csr.subject)
            .issuer_name(self.ca_cert.subject)
            .public_key(csr.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(datetime.now(tz=UTC) - timedelta(minutes=1))
            .not_valid_after(datetime.now(tz=UTC) + timedelta(hours=1))
            .add_extension(x509.SubjectAlternativeName(names), critical=False)
            .add_extension(
                x509.ExtendedKeyUsage(list(self.eku)),
                critical=False,
            )
            .add_extension(
                x509.KeyUsage(
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
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .add_extension(x509.AuthorityKeyIdentifier.from_issuer_public_key(self.ca_key.public_key()), critical=False)
            .add_extension(x509.SubjectKeyIdentifier.from_public_key(csr.public_key()), critical=False)
        )
        if self.mutate_leaf is not None:
            builder = self.mutate_leaf(builder)
        return builder.sign(self.ca_key, hashes.SHA256())

    def verify_ott(self, ott: str) -> dict[str, Any]:
        token = jwt.JWT(key=self.jwk, jwt=ott)
        claims = json.loads(token.claims)
        if claims.get("iss") != self.provisioner:
            raise ValueError("wrong issuer")
        aud = claims.get("aud")
        auds = aud if isinstance(aud, list) else [aud]
        if not any("/1.0/sign" in str(item) for item in auds):
            raise ValueError("wrong audience")
        if self.reject_expired and int(claims["exp"]) < int(time.time()):
            raise ValueError("expired ott")
        jti = str(claims.get("jti") or "")
        if not jti:
            raise ValueError("missing jti")
        if self.reject_reuse and jti in self.used_jti:
            raise ValueError("reused ott")
        self.used_jti.add(jti)
        return claims


@pytest.fixture
def issuer(tmp_path: Path) -> Iterator[tuple[NativeStepCAIssuer, FakeStepCA]]:
    password = "subject-scoped-jwk-password"
    ca = FakeStepCA(PROVISIONER, password)
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "pki-agent-acolyte-orchestrator-jwk", password, 0o400)
    native = NativeStepCAIssuer(
        ca_url=url,
        root_file=root,
        provisioner=PROVISIONER,
        password_file=str(pw),
        timeout_seconds=30,
    )
    try:
        yield native, ca
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_issue_success(issuer: tuple[NativeStepCAIssuer, FakeStepCA]) -> None:
    native, ca = issuer
    cert_pem, key_pem = await native.issue(SUBJECT, [SUBJECT])
    assert cert_pem and key_pem
    method, path, body = ca.last_sign()
    assert method == "POST"
    assert path == "/sign"
    payload = json.loads(body)
    assert payload["ott"] and payload["csr"]
    token = jwt.JWT(key=ca.jwk, jwt=payload["ott"])
    claims = json.loads(token.claims)
    assert claims["iss"] == PROVISIONER
    assert claims["sub"] == SUBJECT
    assert "/1.0/sign" in str(claims["aud"])
    assert claims["sans"][0] == SUBJECT
    leaf = x509.load_pem_x509_certificate(cert_pem)
    assert leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value == SUBJECT


@pytest.mark.asyncio
async def test_mints_distinct_otts(issuer: tuple[NativeStepCAIssuer, FakeStepCA]) -> None:
    native, ca = issuer
    await native.issue(SUBJECT, [SUBJECT])
    await native.issue(SUBJECT, [SUBJECT])
    jtis: list[str] = []
    for _method, path, body in ca.seen:
        if path != "/sign":
            continue
        ott = json.loads(body)["ott"]
        token = jwt.JWT(key=ca.jwk, jwt=ott)
        jtis.append(json.loads(token.claims)["jti"])
    assert len(jtis) == 2
    assert jtis[0] and jtis[0] != jtis[1]


@pytest.mark.asyncio
async def test_reused_ott_rejected_by_ca(issuer: tuple[NativeStepCAIssuer, FakeStepCA], tmp_path: Path) -> None:
    native, ca = issuer
    await native.issue(SUBJECT, [SUBJECT])
    _method, _path, body = ca.last_sign()
    ctx = ssl.create_default_context(cafile=native.root_file)
    async with httpx.AsyncClient(verify=ctx, trust_env=False) as client:
        resp = await client.post(native.ca_url + "/sign", content=body, headers={"Content-Type": "application/json"})
    assert resp.status_code >= 400


@pytest.mark.asyncio
async def test_wrong_ca_root(issuer: tuple[NativeStepCAIssuer, FakeStepCA], tmp_path: Path) -> None:
    native, _ca = issuer
    other, _key = _generate_ca()
    wrong = tmp_path / "wrong.pem"
    wrong.write_bytes(other.public_bytes(Encoding.PEM))
    native.root_file = str(wrong)
    native._cred = None
    with pytest.raises(PkiError) as exc_info:
        await native.issue(SUBJECT, [SUBJECT])
    assert "insecure" not in str(exc_info.value).lower()


@pytest.mark.asyncio
async def test_wrong_subject_rejected(tmp_path: Path) -> None:
    password = "pw-sub"
    ca = FakeStepCA(PROVISIONER, password)

    def signed_wrong(csr: x509.CertificateSigningRequest) -> x509.Certificate:
        return (
            x509.CertificateBuilder()
            .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "evil")]))
            .issuer_name(ca.ca_cert.subject)
            .public_key(csr.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(datetime.now(tz=UTC) - timedelta(minutes=1))
            .not_valid_after(datetime.now(tz=UTC) + timedelta(hours=1))
            .add_extension(x509.SubjectAlternativeName([x509.DNSName("evil")]), critical=False)
            .add_extension(
                x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH]),
                critical=False,
            )
            .sign(ca.ca_key, hashes.SHA256())
        )

    ca._sign_csr = signed_wrong  # type: ignore[method-assign]
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "jwk", password)
    native = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=PROVISIONER, password_file=str(pw), timeout_seconds=5
    )
    try:
        with pytest.raises(PkiError, match="CN"):
            await native.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_wrong_san_rejected(tmp_path: Path) -> None:
    password = "pw-san"
    ca = FakeStepCA(PROVISIONER, password)

    def mutate(builder: x509.CertificateBuilder) -> x509.CertificateBuilder:
        return builder

    # Replace SAN by signing a different CSR-equivalent: mutate after builder is complete
    original = ca._sign_csr

    def signed_wrong(csr: x509.CertificateSigningRequest) -> x509.Certificate:
        return (
            x509.CertificateBuilder()
            .subject_name(csr.subject)
            .issuer_name(ca.ca_cert.subject)
            .public_key(csr.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(datetime.now(tz=UTC) - timedelta(minutes=1))
            .not_valid_after(datetime.now(tz=UTC) + timedelta(hours=1))
            .add_extension(
                x509.SubjectAlternativeName([x509.DNSName("not-the-requested-san")]),
                critical=False,
            )
            .add_extension(
                x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH]),
                critical=False,
            )
            .sign(ca.ca_key, hashes.SHA256())
        )

    ca._sign_csr = signed_wrong  # type: ignore[method-assign]
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "jwk", password)
    native = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=PROVISIONER, password_file=str(pw), timeout_seconds=5
    )
    try:
        with pytest.raises(PkiError, match="SAN"):
            await native.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()
        ca._sign_csr = original  # type: ignore[method-assign]


@pytest.mark.asyncio
async def test_malformed_response(tmp_path: Path) -> None:
    password = "pw-malformed"
    ca = FakeStepCA(PROVISIONER, password)
    ca.malformed_sign = True
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "jwk", password)
    native = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=PROVISIONER, password_file=str(pw), timeout_seconds=5
    )
    try:
        with pytest.raises(PkiError):
            await native.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_timeout(tmp_path: Path) -> None:
    password = "pw-timeout"
    ca = FakeStepCA(PROVISIONER, password)
    ca.sign_delay = 30.0
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "jwk", password)
    native = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=PROVISIONER, password_file=str(pw), timeout_seconds=3
    )
    started = time.monotonic()
    try:
        with pytest.raises(PkiError, match="timed out"):
            await native.issue(SUBJECT, [SUBJECT])
        assert time.monotonic() - started < 15
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_canceled(tmp_path: Path) -> None:
    password = "pw-cancel"
    ca = FakeStepCA(PROVISIONER, password)
    ca.block_sign = threading.Event()
    url, root = ca.start(tmp_path)
    pw = write_password_file(tmp_path, "jwk", password)
    native = NativeStepCAIssuer(
        ca_url=url, root_file=root, provisioner=PROVISIONER, password_file=str(pw), timeout_seconds=30
    )

    async def _issue() -> None:
        await native.issue(SUBJECT, [SUBJECT])

    task = asyncio.create_task(_issue())
    await asyncio.sleep(0.2)
    task.cancel()
    try:
        with pytest.raises(asyncio.CancelledError):
            await task
    finally:
        ca.block_sign.set()
        ca.close()


@pytest.mark.asyncio
async def test_password_file_errors(tmp_path: Path) -> None:
    password = "pw-files"
    ca = FakeStepCA(PROVISIONER, password)
    url, root = ca.start(tmp_path)
    try:
        cases = {
            "missing": tmp_path / "nope",
            "empty": write_password_file(tmp_path, "empty", "", 0o400),
            "directory": tmp_path,
            "world-writable": write_password_file(tmp_path, "open", "secret", 0o666),
        }
        for path in cases.values():
            native = NativeStepCAIssuer(
                ca_url=url,
                root_file=root,
                provisioner=PROVISIONER,
                password_file=str(path),
                timeout_seconds=2,
            )
            with pytest.raises(PkiError):
                await native.issue(SUBJECT, [SUBJECT])
    finally:
        ca.close()


@pytest.mark.asyncio
async def test_does_not_log_secrets(issuer: tuple[NativeStepCAIssuer, FakeStepCA], tmp_path: Path) -> None:
    native, _ca = issuer
    await native.issue(SUBJECT, [SUBJECT])
    native.password_file = str(tmp_path / "missing")
    native._cred = None
    with pytest.raises(PkiError) as exc_info:
        await native.issue(SUBJECT, [SUBJECT])
    assert "subject-scoped-jwk-password" not in str(exc_info.value)


def test_unwraps_step_ca_pbes2_iteration_count() -> None:
    from jwcrypto.jwa import default_max_pbkdf2_iterations as current_max

    before = current_max
    key = JWK.generate(kty="EC", crv="P-256")
    password = "subject-scoped-jwk-password"
    compact = encrypt_step_ca_provisioner_jwk(key, password)
    loaded = decrypt_provisioner_jwk(compact, password.encode())
    assert loaded.get("kty") == "EC"
    from jwcrypto.jwa import default_max_pbkdf2_iterations as after

    assert after == before
    assert after < 600_000


@pytest.mark.asyncio
async def test_rejects_shared_provisioner() -> None:
    native = NativeStepCAIssuer(
        ca_url="https://127.0.0.1:1",
        root_file="/nonexistent/root.pem",
        provisioner="pki-agent",
        password_file="/run/secrets/pki-agent-acolyte-orchestrator-jwk",
    )
    with pytest.raises(SharedProvisionerError):
        await native.issue(SUBJECT, [SUBJECT])


@pytest.mark.asyncio
async def test_rekey_uses_client_cert(issuer: tuple[NativeStepCAIssuer, FakeStepCA]) -> None:
    native, ca = issuer
    cert_pem, key_pem = await native.issue(SUBJECT, [SUBJECT])
    new_cert, new_key = await native.rekey(cert_pem, key_pem, SUBJECT, [SUBJECT])
    method, path, _body = ca.last_rekey()
    assert method == "POST"
    assert path == "/rekey"
    assert new_cert != cert_pem
    assert new_key
