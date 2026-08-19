"""Native issuer against an in-process fake step-ca (no real CA, no step CLI)."""

from __future__ import annotations

import json
import ssl
import threading
import time
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

import pytest
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from jwcrypto import jwk, jwt

from news_creator.infra.pki.config import PkiError, SharedProvisionerError
from news_creator.infra.pki.ctx import CancelledError, Ctx
from news_creator.infra.pki.native_issuer import (
    NativeStepCAIssuer,
    decrypt_provisioner_jwk,
    encrypt_step_ca_provisioner_jwk,
)

SUBJECT = "news-creator"
PROVISIONER = "pki-agent-news-creator"


class RecordedReq:
    def __init__(self, method: str, path: str, body: bytes) -> None:
        self.method = method
        self.path = path
        self.body = body


def _encrypt_jwk(key: jwk.JWK, password: bytes | str) -> str:
    secret = password.decode() if isinstance(password, bytes) else password
    return encrypt_step_ca_provisioner_jwk(key, secret)


def _generate_test_ca() -> tuple[x509.Certificate, ec.EllipticCurvePrivateKey]:
    key = ec.generate_private_key(ec.SECP256R1())
    name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "alt-test-step-ca")])
    cert = (
        x509.CertificateBuilder()
        .subject_name(name)
        .issuer_name(name)
        .public_key(key.public_key())
        .serial_number(1)
        .not_valid_before(datetime.now(UTC) - timedelta(hours=1))
        .not_valid_after(datetime.now(UTC) + timedelta(hours=24))
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
            x509.SubjectAlternativeName(
                [
                    x509.DNSName("localhost"),
                    x509.IPAddress(__import__("ipaddress").ip_address("127.0.0.1")),
                ]
            ),
            critical=False,
        )
        .add_extension(
            x509.SubjectKeyIdentifier.from_public_key(key.public_key()), critical=False
        )
        .sign(key, hashes.SHA256())
    )
    return cert, key


class FakeStepCA:
    def __init__(self, provisioner: str, password: bytes | str) -> None:
        self.provisioner = provisioner
        self.password = password if isinstance(password, bytes) else password.encode()
        self.jwk = jwk.JWK.generate(kty="EC", crv="P-256")
        self.jwk["alg"] = "ES256"
        self.jwk["use"] = "sig"
        self.enc_key = _encrypt_jwk(self.jwk, self.password)
        self.ca_cert, self.ca_key = _generate_test_ca()
        self.used_jti: set[str] = set()
        self.seen: list[RecordedReq] = []
        self.mu = threading.Lock()
        self.sign_delay = 0.0
        self.block_sign: threading.Event | None = None
        self.malformed_sign = False
        self.mutate_leaf: Any = None
        self.mutate_sans: Any = None
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
                del format, args

            def do_GET(self) -> None:
                path = self.path.split("?", 1)[0]
                ca._record(self)
                if path == "/health":
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.end_headers()
                    self.wfile.write(b'{"status":"ok"}')
                    return
                if path == "/provisioners":
                    if ca.endless_provisioners:
                        body = json.dumps(
                            {"provisioners": [], "nextCursor": "more"}
                        ).encode()
                    else:
                        pub = json.loads(ca.jwk.export_public())
                        body = json.dumps(
                            {
                                "provisioners": [
                                    {
                                        "type": "JWK",
                                        "name": ca.provisioner,
                                        "key": pub,
                                        "encryptedKey": ca.enc_key,
                                    }
                                ]
                            }
                        ).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                self.send_error(404)

            def do_POST(self) -> None:
                path = self.path.split("?", 1)[0]
                ca._record(self)
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
                    self.send_error(
                        401,
                        "renew requires a valid client certificate; expired leaves must re-enroll",
                    )
                    return
                self.send_error(404)

        httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        ctx.minimum_version = ssl.TLSVersion.TLSv1_3
        if self.tls_max is not None:
            ctx.minimum_version = ssl.TLSVersion.TLSv1_2
            ctx.maximum_version = self.tls_max
        cert_pem = self.ca_cert.public_bytes(serialization.Encoding.PEM)
        key_pem = self.ca_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.TraditionalOpenSSL,
            encryption_algorithm=serialization.NoEncryption(),
        )
        cert_file = tmp_path / "ca-server.pem"
        key_file = tmp_path / "ca-server.key"
        cert_file.write_bytes(cert_pem)
        key_file.write_bytes(key_pem)
        ctx.load_cert_chain(str(cert_file), str(key_file))
        ctx.verify_mode = ssl.CERT_OPTIONAL
        ctx.load_verify_locations(cadata=cert_pem.decode("ascii"))
        httpd.socket = ctx.wrap_socket(httpd.socket, server_side=True)
        thread = threading.Thread(target=httpd.serve_forever, daemon=True)
        thread.start()
        self._httpd = httpd
        self._thread = thread
        port = httpd.server_address[1]
        root_file = tmp_path / "root.pem"
        root_file.write_bytes(cert_pem)
        return f"https://127.0.0.1:{port}", str(root_file)

    def close(self) -> None:
        if self._httpd is not None:
            self._httpd.shutdown()
            self._httpd.server_close()

    def _record(self, handler: BaseHTTPRequestHandler) -> None:
        length = int(handler.headers.get("Content-Length") or 0)
        body = handler.rfile.read(length) if length else b""
        handler._body = body  # type: ignore[attr-defined]
        with self.mu:
            self.seen.append(
                RecordedReq(handler.command, handler.path.split("?", 1)[0], body)
            )

    def last_sign(self) -> RecordedReq | None:
        with self.mu:
            for rec in reversed(self.seen):
                if rec.path in {"/sign", "/1.0/sign"}:
                    return rec
        return None

    def last_rekey(self) -> RecordedReq | None:
        with self.mu:
            for rec in reversed(self.seen):
                if rec.path == "/rekey":
                    return rec
        return None

    def _handle_sign(self, handler: BaseHTTPRequestHandler) -> None:
        if self.sign_delay:
            time.sleep(self.sign_delay)
        if self.block_sign is not None:
            self.block_sign.wait(timeout=30)
        if self.malformed_sign:
            handler.send_response(201)
            handler.end_headers()
            handler.wfile.write(b"{not-json")
            return
        if self.sign_body:
            handler.send_response(self.sign_status or 201)
            handler.send_header("Content-Type", "application/json")
            handler.send_header("Content-Length", str(len(self.sign_body)))
            handler.end_headers()
            handler.wfile.write(self.sign_body)
            return
        req = json.loads(handler._body.decode())  # type: ignore[attr-defined]
        try:
            claims = self.verify_ott(req["ott"])
        except ValueError as exc:
            _write_ca_error(handler, 401, f"ott: {exc}")
            return
        csr = x509.load_pem_x509_csr(req["csr"].encode())
        cn = csr.subject.get_attributes_for_oid(NameOID.COMMON_NAME)
        cn_val = cn[0].value if cn else ""
        if cn_val != claims["sub"]:
            _write_ca_error(handler, 400, "csr subject mismatch")
            return
        leaf = self._sign_csr(csr, self.mutate_leaf)
        self._write_sign_response(handler, leaf)

    def _handle_rekey(self, handler: BaseHTTPRequestHandler) -> None:
        sock = handler.connection
        peer = sock.getpeercert(binary_form=True) if sock is not None else None
        if not peer:
            handler.send_error(400, "missing client certificate")
            return
        peer_cert = x509.load_der_x509_certificate(peer)
        if datetime.now(UTC) > peer_cert.not_valid_after_utc:
            handler.send_error(401, "expired client certificate")
            return
        req = json.loads(handler._body.decode())  # type: ignore[attr-defined]
        csr = x509.load_pem_x509_csr(req["csr"].encode())
        leaf = self._sign_csr(csr, None)
        self._write_sign_response(handler, leaf)

    def verify_ott(self, ott: str) -> dict[str, Any]:
        pub = jwk.JWK.from_json(self.jwk.export_public())
        try:
            token = jwt.JWT(key=pub, jwt=ott)
        except (ValueError, jwt.JWTExpired, jwt.JWTMissingKey) as exc:
            raise ValueError(str(exc)) from exc
        claims = json.loads(token.claims)
        if claims.get("iss") != self.provisioner:
            raise ValueError("wrong issuer")
        aud = claims.get("aud")
        auds = aud if isinstance(aud, list) else [aud]
        if not any("/1.0/sign" in str(a) for a in auds):
            raise ValueError("wrong audience")
        exp = claims.get("exp")
        if (
            self.reject_expired
            and exp is not None
            and datetime.now(UTC).timestamp() > float(exp)
        ):
            raise ValueError("expired ott")
        jti = claims.get("jti")
        if not jti:
            raise ValueError("missing jti")
        with self.mu:
            if self.reject_reuse and jti in self.used_jti:
                raise ValueError("reused ott")
            self.used_jti.add(str(jti))
        return claims

    def _sign_csr(
        self, csr: x509.CertificateSigningRequest, mutate: Any
    ) -> x509.Certificate:
        try:
            san = csr.extensions.get_extension_for_class(x509.SubjectAlternativeName)
            dns = list(san.value)
        except x509.ExtensionNotFound:
            cn = csr.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value
            dns = [x509.DNSName(cn if isinstance(cn, str) else cn.decode("utf-8"))]
        if self.mutate_sans is not None:
            dns = list(self.mutate_sans(dns))
        builder = (
            x509.CertificateBuilder()
            .subject_name(csr.subject)
            .issuer_name(self.ca_cert.subject)
            .public_key(csr.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(datetime.now(UTC) - timedelta(minutes=1))
            .not_valid_after(datetime.now(UTC) + timedelta(hours=1))
            .add_extension(x509.SubjectAlternativeName(dns), critical=False)
            .add_extension(
                x509.ExtendedKeyUsage(
                    [ExtendedKeyUsageOID.SERVER_AUTH, ExtendedKeyUsageOID.CLIENT_AUTH]
                ),
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
            .add_extension(
                x509.BasicConstraints(ca=False, path_length=None), critical=True
            )
            .add_extension(
                x509.AuthorityKeyIdentifier.from_issuer_public_key(
                    self.ca_key.public_key()
                ),
                critical=False,
            )
            .add_extension(
                x509.SubjectKeyIdentifier.from_public_key(csr.public_key()),
                critical=False,
            )
        )
        # mutate_leaf in Go mutates the x509.Certificate template including Subject/DNSNames.
        # We rebuild after an optional callback on a proxy object.
        leaf = builder.sign(self.ca_key, hashes.SHA256())
        if mutate is not None:
            tmpl = _MutateCert(leaf)
            mutate(tmpl)
            builder = (
                x509.CertificateBuilder()
                .subject_name(
                    x509.Name(
                        [x509.NameAttribute(NameOID.COMMON_NAME, tmpl.common_name)]
                    )
                )
                .issuer_name(self.ca_cert.subject)
                .public_key(csr.public_key())
                .serial_number(x509.random_serial_number())
                .not_valid_before(datetime.now(UTC) - timedelta(minutes=1))
                .not_valid_after(datetime.now(UTC) + timedelta(hours=1))
                .add_extension(
                    x509.SubjectAlternativeName(
                        [x509.DNSName(n) for n in tmpl.dns_names]
                    ),
                    critical=False,
                )
                .add_extension(
                    x509.ExtendedKeyUsage(
                        [
                            ExtendedKeyUsageOID.SERVER_AUTH,
                            ExtendedKeyUsageOID.CLIENT_AUTH,
                        ]
                    ),
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
                .add_extension(
                    x509.BasicConstraints(ca=False, path_length=None), critical=True
                )
                .add_extension(
                    x509.AuthorityKeyIdentifier.from_issuer_public_key(
                        self.ca_key.public_key()
                    ),
                    critical=False,
                )
                .add_extension(
                    x509.SubjectKeyIdentifier.from_public_key(csr.public_key()),
                    critical=False,
                )
            )
            leaf = builder.sign(self.ca_key, hashes.SHA256())
        return leaf

    def _write_sign_response(
        self, handler: BaseHTTPRequestHandler, leaf: x509.Certificate
    ) -> None:
        leaf_pem = leaf.public_bytes(serialization.Encoding.PEM).decode()
        ca_pem = self.ca_cert.public_bytes(serialization.Encoding.PEM).decode()
        body = json.dumps(
            {"crt": leaf_pem, "ca": ca_pem, "certChain": [leaf_pem, ca_pem]}
        ).encode()
        handler.send_response(201)
        handler.send_header("Content-Type", "application/json")
        handler.end_headers()
        handler.wfile.write(body)


class _MutateCert:
    def __init__(self, leaf: x509.Certificate) -> None:
        cn = leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)
        self.common_name = str(cn[0].value) if cn else ""
        try:
            ext = leaf.extensions.get_extension_for_class(x509.SubjectAlternativeName)
            self.dns_names = list(ext.value.get_values_for_type(x509.DNSName))
        except x509.ExtensionNotFound:
            self.dns_names = [self.common_name]


def _write_ca_error(handler: BaseHTTPRequestHandler, status: int, msg: str) -> None:
    body = json.dumps({"status": status, "message": msg}).encode()
    handler.send_response(status)
    handler.send_header("Content-Type", "application/json")
    handler.end_headers()
    handler.wfile.write(body)


def _write_password_file(tmp_path: Path, name: str, password: str, mode: int) -> str:
    path = tmp_path / name
    path.write_bytes((password + "\n").encode())
    path.chmod(mode)
    return str(path)


def _new_issuer(ca: FakeStepCA, tmp_path: Path) -> NativeStepCAIssuer:
    url, root = ca.start(tmp_path)
    pw = _write_password_file(
        tmp_path, "pki-agent-news-creator-jwk", ca.password.decode(), 0o400
    )
    return NativeStepCAIssuer(
        ca_url=url,
        root_file=root,
        provisioner=ca.provisioner,
        password_file=pw,
        timeout=10,
    )


@pytest.fixture
def ca_and_issuer(tmp_path: Path) -> Iterator[tuple[FakeStepCA, NativeStepCAIssuer]]:
    ca = FakeStepCA(PROVISIONER, b"subject-scoped-jwk-password")
    iss = _new_issuer(ca, tmp_path)
    yield ca, iss
    ca.close()


def test_issue_success(ca_and_issuer: tuple[FakeStepCA, NativeStepCAIssuer]) -> None:
    ca, iss = ca_and_issuer
    cert_pem, key_pem = iss.issue(Ctx(), SUBJECT, [SUBJECT])
    assert cert_pem and key_pem
    import tempfile

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

    got = ca.last_sign()
    assert got is not None
    assert got.method == "POST"
    assert got.path == "/sign"
    body = json.loads(got.body)
    assert body["ott"] and body["csr"]
    pub = jwk.JWK.from_json(ca.jwk.export_public())
    token = jwt.JWT(key=pub, jwt=body["ott"])
    claims = json.loads(token.claims)
    assert claims["iss"] == PROVISIONER
    assert claims["sub"] == SUBJECT
    aud = claims["aud"]
    auds = aud if isinstance(aud, list) else [aud]
    assert any("/1.0/sign" in str(a) for a in auds)
    assert claims["sans"][0] == SUBJECT
    leaf = x509.load_pem_x509_certificate(cert_pem)
    cn = leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value
    assert cn == SUBJECT


def test_mints_distinct_otts(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-distinct")
    iss = _new_issuer(ca, tmp_path)
    try:
        iss.issue(Ctx(), SUBJECT, [SUBJECT])
        iss.issue(Ctx(), SUBJECT, [SUBJECT])
        jtis: list[str] = []
        pub = jwk.JWK.from_json(ca.jwk.export_public())
        with ca.mu:
            for rec in ca.seen:
                if rec.path != "/sign":
                    continue
                ott = json.loads(rec.body)["ott"]
                token = jwt.JWT(key=pub, jwt=ott)
                jtis.append(json.loads(token.claims)["jti"])
        assert len(jtis) == 2 and jtis[0] and jtis[0] != jtis[1]
    finally:
        ca.close()


def test_reused_ott_rejected_by_ca(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-reuse")
    iss = _new_issuer(ca, tmp_path)
    try:
        iss.issue(Ctx(), SUBJECT, [SUBJECT])
        first = ca.last_sign()
        assert first is not None
        ctx = ssl.create_default_context(cafile=iss.root_file)
        import http.client
        from urllib.parse import urlparse

        parsed = urlparse(iss.ca_url)
        host = parsed.hostname
        assert host is not None
        conn = http.client.HTTPSConnection(host, parsed.port, context=ctx, timeout=5)
        conn.request(
            "POST",
            "/sign",
            body=first.body,
            headers={"Content-Type": "application/json"},
        )
        resp = conn.getresponse()
        assert resp.status >= 400
        conn.close()
    finally:
        ca.close()


def test_expired_ott_rejected_by_ca(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-exp")
    iss = _new_issuer(ca, tmp_path)
    try:
        now = datetime.now(UTC) - timedelta(hours=1)
        header = {"alg": "ES256", "kid": ca.jwk.get("kid") or "", "typ": "JWT"}
        claims = {
            "iss": ca.provisioner,
            "sub": SUBJECT,
            "aud": iss.ca_url + "/1.0/sign",
            "sans": [SUBJECT],
            "jti": "expired-jti",
            "nbf": int(now.timestamp()),
            "iat": int(now.timestamp()),
            "exp": int((now + timedelta(minutes=1)).timestamp()),
        }
        token = jwt.JWT(header=header, claims=claims)
        token.make_signed_token(ca.jwk)
        from news_creator.infra.pki.native_issuer import _create_csr

        csr_pem, _key = _create_csr(SUBJECT, [SUBJECT])
        body = json.dumps({"csr": csr_pem, "ott": token.serialize()}).encode()
        import http.client
        from urllib.parse import urlparse

        parsed = urlparse(iss.ca_url)
        ctx = ssl.create_default_context(cafile=iss.root_file)
        host = parsed.hostname
        assert host is not None
        conn = http.client.HTTPSConnection(host, parsed.port, context=ctx, timeout=5)
        conn.request(
            "POST", "/sign", body=body, headers={"Content-Type": "application/json"}
        )
        resp = conn.getresponse()
        assert resp.status >= 400
        conn.close()
    finally:
        ca.close()


def test_wrong_ca_root(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-root")
    iss = _new_issuer(ca, tmp_path)
    try:
        other, _ = _generate_test_ca()
        wrong = tmp_path / "wrong.pem"
        wrong.write_bytes(other.public_bytes(serialization.Encoding.PEM))
        iss.root_file = str(wrong)
        with pytest.raises(
            (OSError, RuntimeError, ssl.SSLError, ValueError)
        ) as excinfo:
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
        assert "insecure" not in str(excinfo.value).lower()
    finally:
        ca.close()


def test_wrong_subject_rejected(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-sub")

    def mutate(c: _MutateCert) -> None:
        c.common_name = "evil"
        c.dns_names = ["evil"]

    ca.mutate_leaf = mutate
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(PkiError, match="CN"):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_wrong_san_rejected(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-san")

    def mutate(c: _MutateCert) -> None:
        c.dns_names = ["not-the-requested-san"]

    ca.mutate_leaf = mutate
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(PkiError, match="SAN"):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_malformed_response(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-malformed")
    ca.malformed_sign = True
    iss = _new_issuer(ca, tmp_path)
    try:
        with pytest.raises(PkiError):
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
    finally:
        ca.close()


def test_timeout(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-timeout")
    ca.sign_delay = 2.0
    iss = _new_issuer(ca, tmp_path)
    iss.timeout = 0.05
    try:
        start = time.monotonic()
        with pytest.raises(
            (TimeoutError, OSError, RuntimeError, CancelledError, PkiError)
        ):
            iss.issue(Ctx(timeout=1), SUBJECT, [SUBJECT])
        assert time.monotonic() - start < 1.5
    finally:
        ca.close()


def test_canceled(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-cancel")
    ca.block_sign = threading.Event()
    iss = _new_issuer(ca, tmp_path)
    ctx = Ctx()
    done: list[BaseException | None] = []

    def run() -> None:
        try:
            iss.issue(ctx, SUBJECT, [SUBJECT])
            done.append(None)
        except BaseException as exc:
            done.append(exc)

    try:
        thread = threading.Thread(target=run)
        thread.start()
        time.sleep(0.2)
        ctx.cancel()
        thread.join(timeout=5)
        assert thread.is_alive() is False
        err = done[0]
        assert err is not None
        assert isinstance(err, CancelledError) or "cancel" in str(err).lower()
    finally:
        if ca.block_sign is not None:
            ca.block_sign.set()
        ca.close()


def test_password_file_errors(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-files")
    url, root = ca.start(tmp_path)
    try:
        cases = {
            "missing": str(tmp_path / "nope"),
            "empty": _write_password_file(tmp_path, "empty", "", 0o400),
            "directory": str(tmp_path / "dir"),
            "world-writable": _write_password_file(tmp_path, "open", "secret", 0o666),
        }
        (tmp_path / "dir").mkdir(exist_ok=True)
        for name, path in cases.items():
            iss = NativeStepCAIssuer(
                ca_url=url,
                root_file=root,
                provisioner=PROVISIONER,
                password_file=path,
                timeout=1,
            )
            with pytest.raises((OSError, RuntimeError, ValueError)):
                iss.issue(Ctx(), SUBJECT, [SUBJECT])
            del name
    finally:
        ca.close()


def test_does_not_log_secrets(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"super-secret-jwk-password")
    iss = _new_issuer(ca, tmp_path)
    try:
        iss.issue(Ctx(), SUBJECT, [SUBJECT])
        iss.password_file = str(tmp_path / "missing")
        iss._cred = None
        with pytest.raises((OSError, RuntimeError, ValueError)) as excinfo:
            iss.issue(Ctx(), SUBJECT, [SUBJECT])
        assert "super-secret-jwk-password" not in str(excinfo.value)
    finally:
        ca.close()


def test_rejects_shared_provisioner() -> None:
    iss = NativeStepCAIssuer(
        ca_url="https://127.0.0.1:1",
        root_file="/dev/null",
        provisioner="pki-agent",
        password_file="/run/secrets/pki-agent-news-creator-jwk",
    )
    with pytest.raises(SharedProvisionerError):
        iss.issue(Ctx(), SUBJECT, [SUBJECT])


def test_rekey_uses_client_cert(tmp_path: Path) -> None:
    ca = FakeStepCA(PROVISIONER, b"pw-rekey")
    iss = _new_issuer(ca, tmp_path)
    try:
        cert_pem, key_pem = iss.issue(Ctx(), SUBJECT, [SUBJECT])
        new_cert, new_key = iss.rekey(Ctx(), cert_pem, key_pem, SUBJECT, [SUBJECT])
        import tempfile

        with (
            tempfile.NamedTemporaryFile(delete=False) as cf,
            tempfile.NamedTemporaryFile(delete=False) as kf,
        ):
            cf.write(new_cert)
            kf.write(new_key)
            cname, kname = cf.name, kf.name
        try:
            ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER).load_cert_chain(cname, kname)
        finally:
            Path(cname).unlink(missing_ok=True)
            Path(kname).unlink(missing_ok=True)
        got = ca.last_rekey()
        assert got is not None
        assert got.method == "POST"
        assert got.path == "/rekey"
        assert cert_pem != new_cert
    finally:
        ca.close()


def test_decrypt_provisioner_jwk_password_and_bytes() -> None:
    key = jwk.JWK.generate(kty="EC", crv="P-256")
    key["alg"] = "ES256"
    password = b"itest-jwk-news-creator"
    got = decrypt_provisioner_jwk(_encrypt_jwk(key, password), password)
    assert (
        json.loads(got.export_private())["d"] == json.loads(key.export_private())["d"]
    )


def test_decrypt_provisioner_jwk_wrong_password() -> None:
    key = jwk.JWK.generate(kty="EC", crv="P-256")
    enc = _encrypt_jwk(key, b"right-password")
    with pytest.raises(PkiError, match="decrypt provisioner jwk"):
        decrypt_provisioner_jwk(enc, b"wrong-password")


def test_unwraps_step_ca_pbes2_iteration_count() -> None:
    """step-ca encrypts provisioner JWKs with p2c=600000; the global cap must not stick."""
    from jwcrypto.jwa import default_max_pbkdf2_iterations

    before = default_max_pbkdf2_iterations
    key = jwk.JWK.generate(kty="EC", crv="P-256")
    password = b"itest-jwk-news-creator"
    loaded = decrypt_provisioner_jwk(_encrypt_jwk(key, password), password)
    assert loaded.get("kty") == "EC"
    assert default_max_pbkdf2_iterations == before
    header = json.dumps({"alg": "PBES2-HS256+A128KW", "enc": "A256GCM", "p2c": 20_000})
    b64 = __import__("base64").urlsafe_b64encode(header.encode()).rstrip(b"=").decode()
    with pytest.raises(PkiError, match="p2c"):
        decrypt_provisioner_jwk(f"{b64}.e30.e30.e30.e30", password)
    assert default_max_pbkdf2_iterations == before
