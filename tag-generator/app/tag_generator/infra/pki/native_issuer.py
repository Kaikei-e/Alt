"""Production step-ca client: cryptography + jwcrypto. No step CLI."""

# ruff: noqa: TRY003, PLR2004, ANN401

from __future__ import annotations

import asyncio
import base64
import ctypes
import hashlib
import ipaddress
import json
import os
import secrets
import ssl
import threading
from collections.abc import Sequence
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from email.utils import parseaddr
from typing import Any
from urllib.parse import urlparse, urlunparse

import httpx
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.serialization import Encoding, NoEncryption, PrivateFormat, PublicFormat
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from cryptography.x509.verification import PolicyBuilder, Store, VerificationError
from jwcrypto import jwa, jwe, jwk, jwt
from jwcrypto.common import JWException
from jwcrypto.jwk import JWK

from tag_generator.infra.pki.config import (
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
    require_https,
)
from tag_generator.infra.pki.filesafe import MAX_PASSWORD_BYTES, MAX_ROOT_PEM_BYTES, read_regular_no_follow

_DEFAULT_ISSUE_TIMEOUT = 15.0
_OTT_LIFETIME = timedelta(minutes=5)
STEP_CA_PBES2_P2C = 600_000
MAX_JWE_COMPACT_BYTES = 8 << 10
MAX_JWK_JSON_BYTES = 8 << 10
_PBES2_ALG = "PBES2-HS256+A128KW"
_PBES2_ENC = "A256GCM"
_PBES2_LOCK = threading.Lock()
_ALLOWED_JWE_HEADER_KEYS = frozenset({"alg", "enc", "p2c", "p2s", "kid", "typ", "cty"})


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
        *,
        ca_url: str,
        root_file: str,
        provisioner: str,
        password_file: str,
        timeout_seconds: float = _DEFAULT_ISSUE_TIMEOUT,
    ) -> None:
        self.ca_url = ca_url.rstrip("/")
        self.root_file = root_file
        self.provisioner = provisioner
        self.password_file = password_file
        self.timeout_seconds = timeout_seconds if timeout_seconds > 0 else _DEFAULT_ISSUE_TIMEOUT
        self._cred: _ProvisionerCred | None = None
        self._lock = asyncio.Lock()

    async def issue(self, subject: str, sans: Sequence[str]) -> tuple[bytes, bytes]:
        self._guard_provisioner()
        require_https(self.ca_url)
        names = list(sans) if sans else [subject]
        try:
            async with asyncio.timeout(self.timeout_seconds):
                cred = await self._credentials()
                ott = _mint_ott(cred, subject, names)
                csr_pem, key = _create_csr(subject, names)
                async with self._client() as client:
                    resp = await _post_json(client, _join_url(self.ca_url, "/sign"), {"csr": csr_pem, "ott": ott})
                return self._validate_and_encode(subject, names, resp, key)
        except TimeoutError as err:
            task = asyncio.current_task()
            if task is not None and task.cancelling():
                raise
            raise PkiError("pki: sign: timed out") from err
        except asyncio.CancelledError:
            raise

    async def rekey(
        self,
        cert_pem: bytes,
        key_pem: bytes,
        subject: str,
        sans: Sequence[str],
    ) -> tuple[bytes, bytes]:
        self._guard_provisioner()
        require_https(self.ca_url)
        names = list(sans) if sans else [subject]
        try:
            async with asyncio.timeout(self.timeout_seconds):
                csr_pem, key = _create_csr(subject, names)
                async with self._client(client_cert=(cert_pem, key_pem)) as client:
                    resp = await _post_json(client, _join_url(self.ca_url, "/rekey"), {"csr": csr_pem})
                return self._validate_and_encode(subject, names, resp, key)
        except TimeoutError as err:
            task = asyncio.current_task()
            if task is not None and task.cancelling():
                raise
            raise PkiError("pki: rekey: timed out") from err
        except asyncio.CancelledError:
            raise

    def _guard_provisioner(self) -> None:
        if self.provisioner in {"pki-agent", ""}:
            raise SharedProvisionerError(
                f"pki: shared provisioner pki-agent is forbidden for in-process enrollment (got {self.provisioner!r})"
            )
        if "step_ca_root_password" in self.password_file:
            raise SharedRootSecretError(
                f"pki: provisioner password file must not be the shared step_ca_root_password secret (got {self.password_file!r})"
            )

    async def _credentials(self) -> _ProvisionerCred:
        async with self._lock:
            if self._cred is not None:
                return self._cred
            password = _read_provisioner_password(self.password_file)
            try:
                async with self._client() as client:
                    fingerprint = await _root_fingerprint(client, self.ca_url, self.root_file)
                    loaded = await _load_provisioner_jwk(client, self.ca_url, self.provisioner, password)
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

    def _ssl_context(self, client_cert: tuple[bytes, bytes] | None = None) -> ssl.SSLContext:
        try:
            data = read_regular_no_follow(self.root_file, MAX_ROOT_PEM_BYTES)
        except FileNotFoundError as err:
            raise PkiError("pki: read CA root") from err
        except PkiError as err:
            raise PkiError("pki: read CA root") from err
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        ctx.minimum_version = ssl.TLSVersion.TLSv1_3
        ctx.check_hostname = True
        ctx.verify_mode = ssl.CERT_REQUIRED
        try:
            ctx.load_verify_locations(cadata=data.decode("ascii"))
        except (ValueError, ssl.SSLError, UnicodeDecodeError) as err:
            raise PkiError(f"pki: parse CA root {self.root_file!r}: no certificates") from err
        if client_cert is not None:
            _load_client_chain(ctx, client_cert[0], client_cert[1])
        return ctx

    def _client(self, client_cert: tuple[bytes, bytes] | None = None) -> httpx.AsyncClient:
        require_https(self.ca_url)
        return httpx.AsyncClient(
            verify=self._ssl_context(client_cert),
            timeout=self.timeout_seconds,
            trust_env=False,
            follow_redirects=False,
        )

    def _validate_and_encode(
        self,
        subject: str,
        sans: Sequence[str],
        resp: dict[str, Any],
        key: ec.EllipticCurvePrivateKey,
    ) -> tuple[bytes, bytes]:
        crt = str(resp.get("crt") or "").strip()
        if not crt:
            raise PkiError("pki: empty sign response")
        try:
            leaf = x509.load_pem_x509_certificate(crt.encode("utf-8"))
        except ValueError as err:
            raise PkiError("pki: parse issued cert") from err
        cn = leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)
        cn_value = cn[0].value if cn else ""
        if cn_value != subject:
            raise PkiError(f"pki: issued CN {cn_value!r} does not match subject {subject!r}")
        _assert_sans(leaf, sans)
        _public_key_matches(leaf, key)
        try:
            root_pem = read_regular_no_follow(self.root_file, MAX_ROOT_PEM_BYTES)
            roots = x509.load_pem_x509_certificates(root_pem)
        except (OSError, ValueError, PkiError) as err:
            raise PkiError("pki: load pinned CA root") from err
        if not roots:
            raise PkiError(f"pki: parse CA root {self.root_file!r}: no certificates")
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
                raise PkiError("pki: parse issued chain") from err
            if parsed.public_bytes(Encoding.DER) != leaf.public_bytes(Encoding.DER):
                intermediates.append(parsed)
        _verify_chain(leaf, roots, intermediates, sans)
        return _encode_issued(leaf, intermediates, key)


def _anonymous_fd(name: str) -> int:
    """Anonymous inode for PEM material. memfd only — never a named /tmp path."""
    create = getattr(os, "memfd_create", None)
    if callable(create):
        fd = create(name, getattr(os, "MFD_CLOEXEC", 1))
        if not isinstance(fd, int) or fd < 0:
            raise PkiError("pki: anonymous secret fd unavailable")
        return fd
    libc = ctypes.CDLL("libc.so.6", use_errno=True)
    libc.memfd_create.argtypes = [ctypes.c_char_p, ctypes.c_uint]
    libc.memfd_create.restype = ctypes.c_int
    fd = libc.memfd_create(name.encode("ascii", "replace"), 1)
    if fd >= 0:
        return fd
    err = ctypes.get_errno()
    raise PkiError("pki: anonymous secret fd unavailable") from OSError(err, os.strerror(err))


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
        "exp": int((now + _OTT_LIFETIME).timestamp()),
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
    if 300 <= status < 400:
        return RedirectError("pki: refusing HTTP redirect to step-ca")
    if 400 <= status < 500:
        return CARejectedError(f"pki: CA rejected the request (status {status})")
    if status >= 500:
        return CAUnavailableError(f"pki: CA unavailable (status {status})")
    return PkiError(f"pki: CA rejected the request (status {status})")


async def _read_capped(resp: httpx.Response) -> bytes:
    buf = bytearray()
    async for chunk in resp.aiter_bytes():
        buf.extend(chunk)
        if len(buf) > MAX_RESPONSE_BYTES:
            raise ResponseTooLargeError("pki: CA response exceeded size cap")
    return bytes(buf)


async def _request_json(client: httpx.AsyncClient, method: str, url: str, **kwargs: Any) -> tuple[int, bytes]:
    try:
        req = client.build_request(method, url, **kwargs)
        resp = await client.send(req, stream=True)
    except httpx.TimeoutException as err:
        raise PkiError("pki: ca request: timed out") from err
    except httpx.HTTPError as err:
        raise PkiError("pki: ca request") from err
    try:
        if 300 <= resp.status_code < 400:
            raise RedirectError("pki: refusing HTTP redirect to step-ca")
        body = await _read_capped(resp)
    finally:
        await resp.aclose()
    return resp.status_code, body


async def _root_fingerprint(client: httpx.AsyncClient, ca_url: str, root_file: str) -> str:
    status, _body = await _request_json(client, "GET", _join_url(ca_url, "/health"))
    if status >= 400:
        raise _classify_ca_status(status)
    try:
        certs = x509.load_pem_x509_certificates(read_regular_no_follow(root_file, MAX_ROOT_PEM_BYTES))
    except (OSError, ValueError, PkiError, FileNotFoundError) as err:
        raise PkiError("pki: health: missing verified TLS chain") from err
    if not certs:
        raise PkiError("pki: health: empty verified TLS chain")
    return hashlib.sha256(certs[-1].public_bytes(Encoding.DER)).hexdigest()


async def _load_provisioner_jwk(client: httpx.AsyncClient, ca_url: str, name: str, password: bytearray) -> JWK:
    cursor = ""
    last_decrypt: PkiError | None = None
    for _page in range(MAX_PROVISIONER_PAGES):
        params: dict[str, str] = {"limit": "100"}
        if cursor:
            params["cursor"] = cursor
        status, body = await _request_json(client, "GET", _join_url(ca_url, "/provisioners"), params=params)
        if status >= 400:
            raise _classify_ca_status(status)
        try:
            payload = json.loads(body)
        except json.JSONDecodeError as err:
            raise PkiError("pki: decode provisioners") from err
        for item in payload.get("provisioners") or []:
            if str(item.get("type", "")).upper() != "JWK":
                continue
            if item.get("name") != name:
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
            raise PkiError(f"pki: jwk provisioner {name!r} not found (or password is wrong)")
    raise ProvisionerPageLimitError("pki: provisioner listing exceeded page cap")


def _parse_jwe_protected_header(encrypted_key: str) -> dict[str, Any]:
    if len(encrypted_key.encode("utf-8")) > MAX_JWE_COMPACT_BYTES:
        raise PkiError("pki: provisioner JWE exceeded size cap")
    parts = encrypted_key.split(".")
    if len(parts) != 5:
        raise PkiError("pki: malformed provisioner jwe")
    raw = parts[0].encode("ascii")
    padded = raw + b"=" * ((4 - len(raw) % 4) % 4)
    try:
        header = json.loads(base64.urlsafe_b64decode(padded))
    except (ValueError, json.JSONDecodeError) as err:
        raise PkiError("pki: malformed provisioner jwe header") from err
    if not isinstance(header, dict):
        raise PkiError("pki: malformed provisioner jwe header")
    return header


def _with_step_ca_pbes2_budget[T](fn: Any) -> Any:
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
        raise PkiError("pki: unexpected provisioner JWE zip")
    unknown = set(header) - _ALLOWED_JWE_HEADER_KEYS
    if unknown:
        raise PkiError(f"pki: unexpected provisioner JWE header {sorted(unknown)}")
    if header.get("alg") != _PBES2_ALG or header.get("enc") != _PBES2_ENC:
        raise PkiError("pki: unexpected provisioner JWE alg/enc")
    if header.get("p2c") != STEP_CA_PBES2_P2C:
        raise PkiError("pki: unexpected provisioner JWE p2c")

    def _decrypt() -> JWK:
        token = jwe.JWE()
        token.deserialize(encrypted_key)
        token.decrypt(jwk.JWK.from_password(password.decode("utf-8")))
        payload = token.payload
        if len(payload) > MAX_JWK_JSON_BYTES:
            raise PkiError("pki: decrypted provisioner JWK exceeded size cap")
        return JWK.from_json(payload.decode("utf-8"))

    try:
        return _with_step_ca_pbes2_budget(_decrypt)
    except PkiError:
        raise
    except (JWException, UnicodeDecodeError, ValueError) as err:
        raise PkiError("pki: decrypt provisioner jwk") from err


def encrypt_step_ca_provisioner_jwk(key: JWK, password: str) -> str:
    """Test/helper: mint a compact JWE with exact step-ca p2c=600000."""

    def _encrypt() -> str:
        protected = json.dumps({"alg": _PBES2_ALG, "enc": _PBES2_ENC, "p2c": STEP_CA_PBES2_P2C})
        token = jwe.JWE(key.export_private().encode("utf-8"), protected)
        token.add_recipient(jwk.JWK.from_password(password))
        compact = str(token.serialize(compact=True))
        if len(compact.encode("utf-8")) > MAX_JWE_COMPACT_BYTES:
            raise PkiError("pki: provisioner JWE exceeded size cap")
        return compact

    return _with_step_ca_pbes2_budget(_encrypt)


async def _post_json(client: httpx.AsyncClient, endpoint: str, payload: dict[str, str]) -> dict[str, Any]:
    status, body = await _request_json(client, "POST", endpoint, json=payload)
    if status >= 400:
        raise _classify_ca_status(status)
    try:
        parsed = json.loads(body)
    except json.JSONDecodeError as err:
        raise PkiError("pki: decode sign response") from err
    if not isinstance(parsed, dict):
        raise PkiError("pki: decode sign response")
    return parsed


def _assert_sans(cert: x509.Certificate, sans: Sequence[str]) -> None:
    want_dns, want_ip, want_email, want_uri = split_sans(sans)
    try:
        ext = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName)
        have = ext.value
    except x509.ExtensionNotFound as err:
        if not (want_dns or want_ip or want_email or want_uri):
            return
        raise PkiError("pki: issued cert missing SAN") from err
    got_dns = [item.lower() for item in have.get_values_for_type(x509.DNSName)]
    got_ip = list(have.get_values_for_type(x509.IPAddress))
    got_email = [item.lower() for item in have.get_values_for_type(x509.RFC822Name)]
    got_uri = list(have.get_values_for_type(x509.UniformResourceIdentifier))
    if sorted(got_dns) != sorted(item.lower() for item in want_dns):
        raise PkiError(f"pki: issued DNS SAN set mismatch: got {got_dns!r} want {list(want_dns)!r}")
    if {str(g) for g in got_ip} != {str(w) for w in want_ip}:
        raise PkiError("pki: issued IP SAN set mismatch")
    if sorted(got_email) != sorted(item.lower() for item in want_email):
        raise PkiError("pki: issued email SAN set mismatch")
    if sorted(got_uri) != sorted(want_uri):
        raise PkiError("pki: issued URI SAN set mismatch")


def _public_key_matches(leaf: x509.Certificate, key: ec.EllipticCurvePrivateKey) -> None:
    leaf_spki = leaf.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)
    csr_spki = key.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)
    if leaf_spki != csr_spki:
        raise PkiError("pki: issued leaf public key does not match CSR key")


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
        raise PkiError("pki: issued leaf must have both serverAuth and clientAuth EKUs") from err
    if ExtendedKeyUsageOID.SERVER_AUTH not in usages or ExtendedKeyUsageOID.CLIENT_AUTH not in usages:
        raise PkiError("pki: issued leaf must have both serverAuth and clientAuth EKUs")
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
        raise PkiError("pki: issued chain does not verify against pinned CA root") from err


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
        raise PkiError("pki: provisioner password file") from err
    except PkiError as err:
        if "exceeds" in str(err):
            raise PasswordTooLargeError("pki: provisioner password file exceeded size cap") from err
        raise PkiError("pki: provisioner password file") from err
    password = bytearray(raw.strip())
    if not password:
        raise PkiError(f"pki: provisioner password file {path!r} is empty")
    return password
