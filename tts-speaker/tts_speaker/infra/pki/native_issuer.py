"""Native step-ca client: official JOSE + local key/CSR. No step CLI."""

from __future__ import annotations

import base64
import contextlib
import ctypes
import hashlib
import http.client
import ipaddress
import json
import os
import secrets
import ssl
import threading
import time
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from email.utils import parseaddr
from typing import Any
from urllib.parse import urlparse, urlunparse

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.serialization import (
    Encoding,
    NoEncryption,
    PrivateFormat,
    PublicFormat,
)
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from cryptography.x509.verification import PolicyBuilder, Store, VerificationError
from jwcrypto import jwa, jwe, jwk, jwt
from jwcrypto.common import JWException
from jwcrypto.jwk import JWK

from tts_speaker.infra.pki.config import (
    MAX_PROVISIONER_PAGES,
    MAX_RESPONSE_BYTES,
    CARejectedError,
    CAUnavailableError,
    PasswordTooLargeError,
    PkiError,
    ProvisionerPageLimitError,
    RedirectError,
    ResponseTooLargeError,
    SharedProvisionerError,
    SharedRootSecretError,
    UnsupportedIssuerAlgorithmError,
    require_https,
)
from tts_speaker.infra.pki.ctx import CancelledError, Ctx
from tts_speaker.infra.pki.filesafe import (
    MAX_PASSWORD_BYTES,
    MAX_ROOT_PEM_BYTES,
    read_regular_no_follow,
)

DEFAULT_ISSUE_TIMEOUT = 15.0
OTT_LIFETIME = timedelta(minutes=5)
STEP_CA_PBES2_P2C = 600_000
MAX_JWE_COMPACT_BYTES = 8 << 10
MAX_JWK_JSON_BYTES = 8 << 10
_PBES2_ALG = "PBES2-HS256+A128KW"
_PBES2_ENC = "A256GCM"
_PBES2_LOCK = threading.Lock()
_ALLOWED_JWE_HEADER_KEYS = frozenset({"alg", "enc", "p2c", "p2s", "kid", "typ", "cty"})
_HTTP_REDIRECT_MIN = 300
_HTTP_CLIENT_ERROR_MIN = 400
_HTTP_SERVER_ERROR_MIN = 500
_JWE_COMPACT_PARTS = 5


@dataclass
class _ProvisionerCred:
    name: str
    jwk: JWK
    fingerprint: str
    audience: str


class NativeStepCAIssuer:
    """Mint a leaf via GET /provisioners + POST /sign (OTT) and POST /rekey (mTLS)."""

    def __init__(
        self,
        ca_url: str,
        root_file: str,
        provisioner: str,
        password_file: str,
        timeout: float = DEFAULT_ISSUE_TIMEOUT,
    ) -> None:
        self.ca_url = ca_url.rstrip("/")
        self.root_file = root_file
        self.provisioner = provisioner
        self.password_file = password_file
        self.timeout = timeout if timeout > 0 else DEFAULT_ISSUE_TIMEOUT
        self._lock = threading.Lock()
        self._cred: _ProvisionerCred | None = None
        self._http = _CancelableHTTP()

    def close_idle_connections(self) -> None:
        self._http.close_all()

    def issue(self, ctx: Ctx, subject: str, sans: list[str]) -> tuple[bytes, bytes]:
        ctx.raise_if_cancelled()
        self._guard_provisioner()
        require_https(self.ca_url)
        names = list(sans) if sans else [subject]
        cred = self._credentials(ctx)
        ott = _mint_ott(cred, subject, names)
        csr_pem, key = _create_csr(subject, names)
        try:
            resp = self._post_json(ctx, "/sign", {"csr": csr_pem, "ott": ott}, client_cert=None)
        except CancelledError as exc:
            msg = "pki: sign: canceled"
            raise CancelledError(msg) from exc
        except (OSError, PkiError, http.client.HTTPException) as exc:
            ctx.raise_if_cancelled()
            if isinstance(exc, PkiError):
                raise
            msg = f"pki: sign: {exc}"
            raise PkiError(msg) from exc
        return self._validate_and_encode(subject, names, resp, key)

    def rekey(self, ctx: Ctx, cert_pem: bytes, key_pem: bytes, subject: str, sans: list[str]) -> tuple[bytes, bytes]:
        ctx.raise_if_cancelled()
        self._guard_provisioner()
        require_https(self.ca_url)
        names = list(sans) if sans else [subject]
        csr_pem, key = _create_csr(subject, names)
        try:
            resp = self._post_json(ctx, "/rekey", {"csr": csr_pem}, client_cert=(cert_pem, key_pem))
        except CancelledError as exc:
            msg = "pki: rekey: canceled"
            raise CancelledError(msg) from exc
        except (OSError, PkiError, http.client.HTTPException) as exc:
            ctx.raise_if_cancelled()
            if isinstance(exc, PkiError):
                raise
            msg = f"pki: rekey: {exc}"
            raise PkiError(msg) from exc
        return self._validate_and_encode(subject, names, resp, key)

    def _guard_provisioner(self) -> None:
        if self.provisioner in {"pki-agent", ""}:
            msg = f"pki: shared provisioner pki-agent is forbidden for in-process enrollment (got {self.provisioner!r})"
            raise SharedProvisionerError(msg)
        if "step_ca_root_password" in self.password_file:
            msg = (
                "pki: provisioner password file must not be the shared step_ca_root_password secret "
                f"(got {self.password_file!r})"
            )
            raise SharedRootSecretError(msg)

    def _request_timeout(self, ctx: Ctx) -> float:
        duration = self.timeout if self.timeout > 0 else DEFAULT_ISSUE_TIMEOUT
        remain = ctx.remaining()
        if remain is None:
            return duration
        if remain <= 0:
            return 0.001
        return min(duration, remain)

    def _credentials(self, ctx: Ctx) -> _ProvisionerCred:
        ctx.raise_if_cancelled()
        with self._lock:
            if self._cred is not None:
                return self._cred
            password = _read_provisioner_password(self.password_file)
            try:
                ssl_ctx = self._ssl_context()
                fingerprint = self._root_fingerprint(ctx, ssl_ctx)
                loaded = self._load_provisioner_jwk(ctx, ssl_ctx, password)
            finally:
                for i in range(len(password)):
                    password[i] = 0
            self._cred = _ProvisionerCred(
                name=self.provisioner,
                jwk=loaded,
                fingerprint=fingerprint,
                audience=_join_url(self.ca_url, "/1.0/sign"),
            )
            return self._cred

    def _root_fingerprint(self, ctx: Ctx, ssl_ctx: ssl.SSLContext) -> str:
        status, _body, _chain = self._http.request(
            ctx,
            ssl_ctx,
            self.ca_url,
            _HTTPRequest(method="GET", path="/health", timeout=self._request_timeout(ctx)),
        )
        if status >= _HTTP_REDIRECT_MIN:
            raise _classify_ca_status(status)
        try:
            certs = x509.load_pem_x509_certificates(read_regular_no_follow(self.root_file, MAX_ROOT_PEM_BYTES))
        except (OSError, ValueError, PkiError, FileNotFoundError) as err:
            msg = "pki: health: missing verified TLS chain"
            raise PkiError(msg) from err
        if not certs:
            msg = "pki: health: empty verified TLS chain"
            raise PkiError(msg)
        return hashlib.sha256(certs[-1].public_bytes(Encoding.DER)).hexdigest()

    def _load_provisioner_jwk(self, ctx: Ctx, ssl_ctx: ssl.SSLContext, password: bytearray) -> JWK:
        cursor = ""
        last_decrypt: PkiError | None = None
        for _page in range(MAX_PROVISIONER_PAGES):
            path = "/provisioners?limit=100"
            if cursor:
                path += f"&cursor={cursor}"
            status, body, _chain = self._http.request(
                ctx,
                ssl_ctx,
                self.ca_url,
                _HTTPRequest(method="GET", path=path, timeout=self._request_timeout(ctx)),
            )
            if status >= _HTTP_REDIRECT_MIN:
                raise _classify_ca_status(status)
            try:
                payload = json.loads(body)
            except json.JSONDecodeError as err:
                msg = "pki: decode provisioners"
                raise PkiError(msg) from err
            for item in payload.get("provisioners") or []:
                if str(item.get("type", "")).upper() != "JWK":
                    continue
                if item.get("name") != self.provisioner:
                    continue
                encrypted = item.get("encryptedKey") or ""
                if not encrypted:
                    continue
                try:
                    return decrypt_provisioner_jwk(str(encrypted), bytes(password))
                except PkiError as err:
                    last_decrypt = err
                    continue
            cursor = str(payload.get("nextCursor") or "")
            if not cursor:
                if last_decrypt is not None:
                    raise last_decrypt
                msg = f"pki: jwk provisioner {self.provisioner!r} not found (or password is wrong)"
                raise PkiError(msg)
        msg = "pki: provisioner listing exceeded page cap"
        raise ProvisionerPageLimitError(msg)

    def _post_json(
        self,
        ctx: Ctx,
        path: str,
        payload: dict[str, str],
        client_cert: tuple[bytes, bytes] | None,
    ) -> dict[str, Any]:
        ssl_ctx = self._ssl_context(client_cert)
        raw = json.dumps(payload).encode("utf-8")
        status, body, _chain = self._http.request(
            ctx,
            ssl_ctx,
            self.ca_url,
            _HTTPRequest(
                method="POST",
                path=path,
                body=raw,
                headers={"Content-Type": "application/json"},
                timeout=self._request_timeout(ctx),
            ),
        )
        if status >= _HTTP_REDIRECT_MIN:
            raise _classify_ca_status(status)
        try:
            parsed = json.loads(body)
        except json.JSONDecodeError as err:
            msg = "pki: decode sign response"
            raise PkiError(msg) from err
        if not isinstance(parsed, dict):
            msg = "pki: decode sign response"
            raise PkiError(msg)
        return parsed

    def _ssl_context(self, client_cert: tuple[bytes, bytes] | None = None) -> ssl.SSLContext:
        try:
            data = read_regular_no_follow(self.root_file, MAX_ROOT_PEM_BYTES)
        except FileNotFoundError as err:
            msg = "pki: read CA root"
            raise PkiError(msg) from err
        except PkiError as err:
            msg = "pki: read CA root"
            raise PkiError(msg) from err
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        ctx.minimum_version = ssl.TLSVersion.TLSv1_3
        ctx.check_hostname = True
        ctx.verify_mode = ssl.CERT_REQUIRED
        try:
            ctx.load_verify_locations(cadata=data.decode("ascii"))
        except (ValueError, ssl.SSLError, UnicodeDecodeError) as err:
            msg = f"pki: parse CA root {self.root_file!r}: no certificates"
            raise PkiError(msg) from err
        if client_cert is not None:
            _load_client_chain(ctx, client_cert[0], client_cert[1])
        return ctx

    def _validate_and_encode(
        self,
        subject: str,
        sans: Sequence[str],
        resp: dict[str, Any],
        key: ec.EllipticCurvePrivateKey,
    ) -> tuple[bytes, bytes]:
        crt = str(resp.get("crt") or "").strip()
        if not crt:
            msg = "pki: empty sign response"
            raise PkiError(msg)
        try:
            leaf = x509.load_pem_x509_certificate(crt.encode("utf-8"))
        except ValueError as err:
            msg = "pki: parse issued cert"
            raise PkiError(msg) from err
        cn = leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)
        cn_value = cn[0].value if cn else ""
        if cn_value != subject:
            msg = f"pki: issued CN {cn_value!r} does not match subject {subject!r}"
            raise PkiError(msg)
        _assert_sans(leaf, sans)
        _public_key_matches(leaf, key)
        try:
            root_pem = read_regular_no_follow(self.root_file, MAX_ROOT_PEM_BYTES)
            roots = x509.load_pem_x509_certificates(root_pem)
        except (OSError, ValueError, PkiError) as err:
            msg = "pki: load pinned CA root"
            raise PkiError(msg) from err
        if not roots:
            msg = f"pki: parse CA root {self.root_file!r}: no certificates"
            raise PkiError(msg)
        intermediates: list[x509.Certificate] = []
        chain_pems: list[str] = []
        raw_chain = resp.get("certChain") or []
        if isinstance(raw_chain, list):
            chain_pems.extend(str(item) for item in raw_chain if item)
        ca_pem = str(resp.get("ca") or "").strip()
        if ca_pem:
            chain_pems.append(ca_pem)
        for pem_str in chain_pems:
            try:
                parsed = x509.load_pem_x509_certificate(pem_str.encode("utf-8"))
            except ValueError as err:
                msg = "pki: parse issued chain"
                raise PkiError(msg) from err
            if parsed.public_bytes(Encoding.DER) != leaf.public_bytes(Encoding.DER):
                intermediates.append(parsed)
        _verify_chain(leaf, roots, intermediates, sans)
        return _encode_issued(leaf, intermediates, key)


@dataclass(frozen=True, slots=True)
class _HTTPRequest:
    method: str
    path: str
    body: bytes | None = None
    headers: dict[str, str] | None = None
    timeout: float = DEFAULT_ISSUE_TIMEOUT


class _CancelableHTTP:
    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._conns: list[http.client.HTTPSConnection] = []

    def close_all(self) -> None:
        with self._lock:
            conns = list(self._conns)
        for conn in conns:
            with contextlib.suppress(OSError):
                conn.close()

    def request(
        self,
        ctx: Ctx,
        ssl_ctx: ssl.SSLContext,
        base: str,
        req: _HTTPRequest,
    ) -> tuple[int, bytes, list[bytes]]:
        ctx.raise_if_cancelled()
        require_https(base)
        parsed = urlparse(base)
        host = parsed.hostname or ""
        if not host:
            msg = "pki: STEP_CA_URL missing host"
            raise PkiError(msg)
        target = (host, parsed.port or 443)
        deadline = time.monotonic() + req.timeout
        last_exc: BaseException | None = None
        while time.monotonic() < deadline:
            ctx.raise_if_cancelled()
            result, last_exc = self._try_once(ctx, ssl_ctx, target, req, deadline)
            if result is not None:
                return result
        ctx.raise_if_cancelled()
        if last_exc is not None:
            raise last_exc
        msg = "pki: http timed out"
        raise TimeoutError(msg)

    def _try_once(
        self,
        ctx: Ctx,
        ssl_ctx: ssl.SSLContext,
        target: tuple[str, int],
        req: _HTTPRequest,
        deadline: float,
    ) -> tuple[tuple[int, bytes, list[bytes]] | None, BaseException | None]:
        slice_timeout = min(0.2, max(0.05, deadline - time.monotonic()))
        conn = http.client.HTTPSConnection(target[0], target[1], context=ssl_ctx, timeout=slice_timeout)
        with self._lock:
            self._conns.append(conn)
        try:
            conn.request(req.method, req.path, body=req.body, headers=req.headers or {})
            resp = conn.getresponse()
            if _HTTP_REDIRECT_MIN <= resp.status < _HTTP_CLIENT_ERROR_MIN:
                return None, self._raise_redirect()
            data = _read_capped(resp)
            chain = self._extract_verified_chain(conn)
        except (RedirectError, ResponseTooLargeError):
            raise
        except TimeoutError as exc:
            ctx.raise_if_cancelled()
            return None, exc
        except ssl.SSLError as exc:
            if ctx.cancelled():
                msg = "canceled"
                raise CancelledError(msg) from exc
            msg = "pki: ca request"
            raise PkiError(msg) from exc
        except (OSError, http.client.HTTPException) as exc:
            if ctx.cancelled():
                msg = "canceled"
                raise CancelledError(msg) from exc
            if time.monotonic() >= deadline:
                msg = "pki: ca request"
                raise PkiError(msg) from exc
            ctx.raise_if_cancelled()
            return None, exc
        else:
            return (resp.status, data, chain), None
        finally:
            with contextlib.suppress(OSError):
                conn.close()
            with self._lock:
                if conn in self._conns:
                    self._conns.remove(conn)

    @staticmethod
    def _raise_redirect() -> RedirectError:
        msg = "pki: refusing HTTP redirect to step-ca"
        raise RedirectError(msg)

    @staticmethod
    def _extract_verified_chain(conn: http.client.HTTPSConnection) -> list[bytes]:
        chain: list[bytes] = []
        sock = conn.sock
        if sock is None:
            return chain
        raw_chain = sock.get_verified_chain()
        if raw_chain:
            for item in raw_chain:
                if isinstance(item, str):
                    chain.append(ssl.PEM_cert_to_DER_cert(item))
                else:
                    chain.append(item)
        elif peer := sock.getpeercert(binary_form=True):
            chain.append(peer)
        return chain


def _read_capped(resp: http.client.HTTPResponse) -> bytes:
    cl = resp.getheader("Content-Length")
    if cl is not None:
        try:
            if int(cl) > MAX_RESPONSE_BYTES:
                msg = "pki: CA response exceeded size cap"
                raise ResponseTooLargeError(msg)
        except ValueError:
            pass
    buf = bytearray()
    while True:
        chunk = resp.read(65536)
        if not chunk:
            break
        buf.extend(chunk)
        if len(buf) > MAX_RESPONSE_BYTES:
            msg = "pki: CA response exceeded size cap"
            raise ResponseTooLargeError(msg)
    return bytes(buf)


def _anonymous_fd(name: str) -> int:
    """Anonymous inode for PEM material. memfd only — never a named /tmp path."""
    create = getattr(os, "memfd_create", None)
    if callable(create):
        fd = create(name, getattr(os, "MFD_CLOEXEC", 1))
        if not isinstance(fd, int) or fd < 0:
            msg = "pki: anonymous secret fd unavailable"
            raise PkiError(msg)
        return fd
    libc = ctypes.CDLL("libc.so.6", use_errno=True)
    libc.memfd_create.argtypes = [ctypes.c_char_p, ctypes.c_uint]
    libc.memfd_create.restype = ctypes.c_int
    fd = libc.memfd_create(name.encode("ascii", "replace"), 1)
    if fd >= 0:
        return fd
    err = ctypes.get_errno()
    msg = "pki: anonymous secret fd unavailable"
    raise PkiError(msg) from OSError(err, os.strerror(err))


def _load_client_chain(ctx: ssl.SSLContext, cert_pem: bytes, key_pem: bytes) -> None:
    cert_fd = -1
    key_fd = -1
    try:
        cert_fd = _anonymous_fd("pki-mtls-cert")
        key_fd = _anonymous_fd("pki-mtls-key")
        os.write(cert_fd, cert_pem)
        os.write(key_fd, key_pem)
        os.lseek(cert_fd, 0, os.SEEK_SET)
        os.lseek(key_fd, 0, os.SEEK_SET)
        ctx.load_cert_chain(certfile=f"/proc/self/fd/{cert_fd}", keyfile=f"/proc/self/fd/{key_fd}")
    finally:
        if cert_fd >= 0:
            os.close(cert_fd)
        if key_fd >= 0:
            os.close(key_fd)


def _mint_ott(cred: _ProvisionerCred, subject: str, sans: Sequence[str]) -> str:
    now = datetime.now(tz=UTC)
    claims: dict[str, Any] = {
        "iss": cred.name,
        "sub": subject,
        "aud": cred.audience,
        "iat": int(now.timestamp()),
        "nbf": int(now.timestamp()),
        "exp": int((now + OTT_LIFETIME).timestamp()),
        "jti": secrets.token_hex(64),
        "sans": list(sans),
    }
    if cred.fingerprint:
        claims["sha"] = cred.fingerprint
    header: dict[str, str] = {"alg": "ES256", "typ": "JWT"}
    kid = cred.jwk.get("kid")
    if isinstance(kid, str) and kid:
        header["kid"] = kid
    token = jwt.JWT(header=header, claims=claims)
    token.make_signed_token(cred.jwk)
    return token.serialize()


def split_sans(
    sans: Sequence[str],
) -> tuple[list[str], list[ipaddress.IPv4Address | ipaddress.IPv6Address], list[str], list[str]]:
    dns: list[str] = []
    ips: list[ipaddress.IPv4Address | ipaddress.IPv6Address] = []
    emails: list[str] = []
    uris: list[str] = []
    for name in sans:
        try:
            ips.append(ipaddress.ip_address(name))
            continue
        except ValueError:
            pass
        if "://" in name:
            uris.append(name)
            continue
        _, parsed_email = parseaddr(name)
        if "@" in name and parsed_email == name:
            emails.append(name)
            continue
        dns.append(name)
    return dns, ips, emails, uris


def _create_csr(subject: str, sans: Sequence[str]) -> tuple[str, ec.EllipticCurvePrivateKey]:
    key = ec.generate_private_key(ec.SECP256R1())
    dns, ips, emails, uris = split_sans(sans)
    general_names: list[x509.GeneralName] = []
    general_names.extend(x509.DNSName(item) for item in dns)
    general_names.extend(x509.IPAddress(item) for item in ips)
    general_names.extend(x509.RFC822Name(item) for item in emails)
    general_names.extend(x509.UniformResourceIdentifier(item) for item in uris)
    builder = x509.CertificateSigningRequestBuilder().subject_name(
        x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, subject)])
    )
    if general_names:
        builder = builder.add_extension(x509.SubjectAlternativeName(general_names), critical=False)
    csr = builder.sign(key, hashes.SHA256())
    return csr.public_bytes(Encoding.PEM).decode("utf-8"), key


def _join_url(base: str, path: str) -> str:
    parsed = urlparse(base)
    if not parsed.scheme:
        return base.rstrip("/") + path
    return urlunparse(parsed._replace(path=path, query="", fragment=""))


def _classify_ca_status(status: int) -> PkiError:
    if _HTTP_REDIRECT_MIN <= status < _HTTP_CLIENT_ERROR_MIN:
        msg = "pki: refusing HTTP redirect to step-ca"
        return RedirectError(msg)
    if _HTTP_CLIENT_ERROR_MIN <= status < _HTTP_SERVER_ERROR_MIN:
        msg = f"pki: CA rejected the request (status {status})"
        return CARejectedError(msg)
    if status >= _HTTP_SERVER_ERROR_MIN:
        msg = f"pki: CA unavailable (status {status})"
        return CAUnavailableError(msg)
    msg = f"pki: CA rejected the request (status {status})"
    return PkiError(msg)


def _parse_jwe_protected_header(encrypted_key: str) -> dict[str, Any]:
    if len(encrypted_key.encode("utf-8")) > MAX_JWE_COMPACT_BYTES:
        msg = "pki: provisioner JWE exceeded size cap"
        raise PkiError(msg)
    parts = encrypted_key.split(".")
    if len(parts) != _JWE_COMPACT_PARTS:
        msg = "pki: malformed provisioner jwe"
        raise PkiError(msg)
    raw = parts[0].encode("ascii")
    padded = raw + b"=" * ((4 - len(raw) % 4) % 4)
    try:
        header = json.loads(base64.urlsafe_b64decode(padded))
    except (ValueError, json.JSONDecodeError) as err:
        msg = "pki: malformed provisioner jwe header"
        raise PkiError(msg) from err
    if not isinstance(header, dict):
        msg = "pki: malformed provisioner jwe header"
        raise PkiError(msg)
    return header


def _with_step_ca_pbes2_budget[T](fn: Callable[[], T]) -> T:
    with _PBES2_LOCK:
        previous = jwa.default_max_pbkdf2_iterations
        jwa.default_max_pbkdf2_iterations = max(previous, STEP_CA_PBES2_P2C)
        try:
            return fn()
        finally:
            jwa.default_max_pbkdf2_iterations = previous


def decrypt_provisioner_jwk(encrypted_key: str, password: bytes) -> JWK:
    header = _parse_jwe_protected_header(encrypted_key)
    if "zip" in header:
        msg = "pki: unexpected provisioner JWE zip"
        raise PkiError(msg)
    unknown = set(header) - _ALLOWED_JWE_HEADER_KEYS
    if unknown:
        msg = f"pki: unexpected provisioner JWE header {sorted(unknown)}"
        raise PkiError(msg)
    if header.get("alg") != _PBES2_ALG or header.get("enc") != _PBES2_ENC:
        msg = "pki: unexpected provisioner JWE alg/enc"
        raise PkiError(msg)
    if header.get("p2c") != STEP_CA_PBES2_P2C:
        msg = "pki: unexpected provisioner JWE p2c"
        raise PkiError(msg)

    def _decrypt() -> JWK:
        token = jwe.JWE()
        token.deserialize(encrypted_key)
        token.decrypt(jwk.JWK.from_password(password.decode("utf-8")))
        payload = token.payload
        if len(payload) > MAX_JWK_JSON_BYTES:
            msg = "pki: decrypted provisioner JWK exceeded size cap"
            raise PkiError(msg)
        return JWK.from_json(payload.decode("utf-8"))

    try:
        return _with_step_ca_pbes2_budget(_decrypt)
    except PkiError:
        raise
    except (JWException, UnicodeDecodeError, ValueError) as err:
        msg = "pki: decrypt provisioner jwk"
        raise PkiError(msg) from err


def encrypt_step_ca_provisioner_jwk(key: JWK, password: str) -> str:
    """Test/helper: mint a compact JWE with exact step-ca p2c=600000."""

    def _encrypt() -> str:
        protected = json.dumps({"alg": _PBES2_ALG, "enc": _PBES2_ENC, "p2c": STEP_CA_PBES2_P2C})
        token = jwe.JWE(key.export_private().encode("utf-8"), protected)
        token.add_recipient(jwk.JWK.from_password(password))
        compact = str(token.serialize(compact=True))
        if len(compact.encode("utf-8")) > MAX_JWE_COMPACT_BYTES:
            msg = "pki: provisioner JWE exceeded size cap"
            raise PkiError(msg)
        return compact

    return _with_step_ca_pbes2_budget(_encrypt)


_decrypt_provisioner_jwk = decrypt_provisioner_jwk


def _assert_sans(cert: x509.Certificate, sans: Sequence[str]) -> None:
    want_dns, want_ip, want_email, want_uri = split_sans(sans)
    try:
        ext = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName)
        have = ext.value
    except x509.ExtensionNotFound as err:
        if not (want_dns or want_ip or want_email or want_uri):
            return
        msg = "pki: issued cert missing SAN"
        raise PkiError(msg) from err
    got_dns = [item.lower() for item in have.get_values_for_type(x509.DNSName)]
    got_ip = list(have.get_values_for_type(x509.IPAddress))
    got_email = [item.lower() for item in have.get_values_for_type(x509.RFC822Name)]
    got_uri = list(have.get_values_for_type(x509.UniformResourceIdentifier))
    if sorted(got_dns) != sorted(item.lower() for item in want_dns):
        msg = f"pki: issued DNS SAN set mismatch: got {got_dns!r} want {list(want_dns)!r}"
        raise PkiError(msg)
    if {str(g) for g in got_ip} != {str(w) for w in want_ip}:
        msg = "pki: issued IP SAN set mismatch"
        raise PkiError(msg)
    if sorted(got_email) != sorted(item.lower() for item in want_email):
        msg = "pki: issued email SAN set mismatch"
        raise PkiError(msg)
    if sorted(got_uri) != sorted(want_uri):
        msg = "pki: issued URI SAN set mismatch"
        raise PkiError(msg)


def _public_key_matches(leaf: x509.Certificate, key: ec.EllipticCurvePrivateKey) -> None:
    leaf_spki = leaf.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)
    csr_spki = key.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)
    if leaf_spki != csr_spki:
        msg = "pki: issued leaf public key does not match CSR key"
        raise PkiError(msg)


def _verify_chain(
    leaf: x509.Certificate,
    roots: Sequence[x509.Certificate],
    intermediates: Sequence[x509.Certificate],
    sans: Sequence[str],
) -> None:
    try:
        eku = leaf.extensions.get_extension_for_class(x509.ExtendedKeyUsage)
        usages = list(eku.value)
    except x509.ExtensionNotFound as err:
        msg = "pki: issued chain does not verify against pinned CA root"
        raise PkiError(msg) from err
    if ExtendedKeyUsageOID.SERVER_AUTH not in usages or ExtendedKeyUsageOID.CLIENT_AUTH not in usages:
        msg = "pki: issued chain does not verify against pinned CA root"
        raise PkiError(msg)
    builder = PolicyBuilder().store(Store(list(roots)))
    dns, ips, _, _ = split_sans(sans)
    try:
        if dns:
            builder.build_server_verifier(x509.DNSName(dns[0])).verify(leaf, list(intermediates))
        elif ips:
            builder.build_server_verifier(x509.IPAddress(ips[0])).verify(leaf, list(intermediates))
        else:
            builder.build_client_verifier().verify(leaf, list(intermediates))
    except (VerificationError, ValueError, TypeError) as err:
        err_text = str(err)
        if "Forbidden public key algorithm" in err_text or "Ed25519" in err_text:
            msg = "pki: unsupported issuer algorithm"
            raise UnsupportedIssuerAlgorithmError(msg) from err
        msg = "pki: issued chain does not verify against pinned CA root"
        raise PkiError(msg) from err


def _encode_issued(
    leaf: x509.Certificate,
    chain: Sequence[x509.Certificate],
    key: ec.EllipticCurvePrivateKey,
) -> tuple[bytes, bytes]:
    parts = [leaf.public_bytes(Encoding.PEM)]
    seen = {leaf.public_bytes(Encoding.DER)}
    for cert in chain:
        der = cert.public_bytes(Encoding.DER)
        if der in seen:
            continue
        seen.add(der)
        parts.append(cert.public_bytes(Encoding.PEM))
    key_pem = key.private_bytes(Encoding.PEM, PrivateFormat.PKCS8, NoEncryption())
    cert_pem = b"".join(parts)
    serialization.load_pem_private_key(key_pem, password=None)
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    _load_client_chain(ctx, cert_pem, key_pem)
    return cert_pem, key_pem


def _read_provisioner_password(path: str) -> bytearray:
    try:
        raw = read_regular_no_follow(path, MAX_PASSWORD_BYTES)
    except FileNotFoundError as err:
        msg = "pki: provisioner password file"
        raise PkiError(msg) from err
    except PkiError as err:
        if "exceeds" in str(err):
            msg = "pki: provisioner password file exceeded size cap"
            raise PasswordTooLargeError(msg) from err
        msg = "pki: provisioner password file"
        raise PkiError(msg) from err
    password = bytearray(raw.strip())
    if not password:
        msg = f"pki: provisioner password file {path!r} is empty"
        raise PkiError(msg)
    return password
