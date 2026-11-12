"""Bounded mTLS AuthHub test provider. Only EdDSA staging JWTs are accepted."""

import base64
import hashlib
import hmac
import json
import os
import re
import ssl
import time
import urllib.error
import urllib.request
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


_P = 2**255 - 19
_D = -121665 * pow(121666, _P - 2, _P) % _P
_Q = 2**252 + 27742317777372353535851937790883648493
_SQRT_M1 = pow(2, (_P - 1) // 4, _P)
_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")


def _recover_x(y, sign):
    if y >= _P:
        return None
    x2 = (y * y - 1) * pow(_D * y * y + 1, _P - 2, _P) % _P
    if x2 == 0:
        return None if sign else 0
    x = pow(x2, (_P + 3) // 8, _P)
    if (x * x - x2) % _P:
        x = x * _SQRT_M1 % _P
    if (x * x - x2) % _P:
        return None
    return _P - x if (x & 1) != sign else x


def _add(a, b):
    p1 = (a[1] - a[0]) * (b[1] - b[0]) % _P
    p2 = (a[1] + a[0]) * (b[1] + b[0]) % _P
    c = 2 * a[3] * b[3] * _D % _P
    d = 2 * a[2] * b[2] % _P
    e, f, g, h = p2 - p1, d - c, d + c, p2 + p1
    return (e * f % _P, g * h % _P, f * g % _P, e * h % _P)


def _mul(scalar, point):
    result = (0, 1, 1, 0)
    while scalar > 0:
        if scalar & 1:
            result = _add(result, point)
        point = _add(point, point)
        scalar >>= 1
    return result


def _decompress(raw):
    y = int.from_bytes(raw, "little")
    x = _recover_x(y & ((1 << 255) - 1), y >> 255)
    if x is None:
        return None
    y &= (1 << 255) - 1
    return (x, y, 1, x * y % _P)


_BASE_Y = 4 * pow(5, _P - 2, _P) % _P
_BASE = (_recover_x(_BASE_Y, 0), _BASE_Y, 1, _recover_x(_BASE_Y, 0) * _BASE_Y % _P)


def ed25519_verify(public_key, message, signature):
    """RFC 8032 section 6 verification; the stdlib has no Ed25519."""
    if len(public_key) != 32 or len(signature) != 64:
        return False
    a, r = _decompress(public_key), _decompress(signature[:32])
    s = int.from_bytes(signature[32:], "little")
    if a is None or r is None or s >= _Q:
        return False
    h = int.from_bytes(hashlib.sha512(signature[:32] + public_key + message).digest(), "little") % _Q
    left, right = _mul(s, _BASE), _add(r, _mul(h, a))
    return (left[0] * right[2] - right[0] * left[2]) % _P == 0 and (left[1] * right[2] - right[1] * left[2]) % _P == 0


def load_public_key(pem):
    """Raw 32-byte key from an Ed25519 SPKI PEM."""
    der = base64.b64decode("".join(line for line in pem.decode().splitlines() if not line.startswith("-----")))
    if len(der) != len(_SPKI_PREFIX) + 32 or not der.startswith(_SPKI_PREFIX):
        raise ValueError("not an Ed25519 SPKI public key")
    return der[len(_SPKI_PREFIX):]


def introspect(token, key, now, issuer="alt-staging-auth-hub"):
    try:
        header, payload, signature = token.split(".")
        if not all(re.fullmatch(r"[A-Za-z0-9_-]+", part) for part in (header, payload, signature)):
            return {"active": False}
        decode = lambda value: base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))
        protected = json.loads(decode(header))
        if not isinstance(protected, dict) or protected.get("alg") != "EdDSA":
            return {"active": False}
        if not ed25519_verify(key, f"{header}.{payload}".encode(), decode(signature)):
            return {"active": False}
        claims = json.loads(decode(payload))
        if not isinstance(claims, dict):
            return {"active": False}
        sub, tenant, exp = claims["sub"], claims["tenant_id"], claims["exp"]
        if not isinstance(sub, str) or not isinstance(tenant, str):
            return {"active": False}
        if str(uuid.UUID(sub)) != sub or str(uuid.UUID(tenant)) != tenant:
            return {"active": False}
        if uuid.UUID(sub).int == 0 or uuid.UUID(tenant).int == 0:
            return {"active": False}
        if not isinstance(exp, int) or isinstance(exp, bool) or not now < exp <= now + 300:
            return {"active": False}
        if claims.get("iss") != issuer or claims.get("aud") != "alt-backend":
            return {"active": False}
        return {"active": True, "sub": sub, "tenant_id": tenant, "exp": exp}
    except (ValueError, TypeError, KeyError, UnicodeError):
        return {"active": False}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        # JWTs and private bearer headers never enter fixture logs.
        pass

    def reply(self, status, body):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def peer(self):
        for rdn in self.connection.getpeercert().get("subject", ()):
            for name, value in rdn:
                if name == "commonName":
                    return value
        return ""

    def forward_upstream(self, method="POST"):
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if not 0 <= length <= 65536:
                return self.reply(400, {})
            body = self.rfile.read(length) if length > 0 else (b"" if method == "POST" else None)
            headers = {}
            for h in ("Content-Type", "X-Alt-Backend-Token", "Connect-Protocol-Version"):
                if h in self.headers:
                    headers[h] = self.headers[h]
            if "Content-Type" not in headers and method == "POST":
                headers["Content-Type"] = "application/json"
            request = urllib.request.Request(
                "http://alt-backend-deps-stub" + self.path,
                data=body,
                headers=headers,
                method=method,
            )
            with urllib.request.urlopen(request, timeout=5) as response:
                res_body = response.read(65537)
                self.send_response(response.status)
                for k, v in response.headers.items():
                    if k.lower() in {"content-type", "connect-protocol-version"}:
                        self.send_header(k, v)
                self.send_header("Content-Length", str(len(res_body)))
                self.end_headers()
                self.wfile.write(res_body)
                return
        except urllib.error.HTTPError as err:
            res_body = err.read(65537)
            self.send_response(err.code)
            for k, v in err.headers.items():
                if k.lower() in {"content-type", "connect-protocol-version"}:
                    self.send_header(k, v)
            self.send_header("Content-Length", str(len(res_body)))
            self.end_headers()
            self.wfile.write(res_body)
            return
        except (ValueError, OSError):
            return self.reply(502, {})

    def do_POST(self):
        is_preprocessor = os.environ.get("STAGING_FORWARD_PREPROCESSOR") == "true" and (
            self.path.startswith("/services.preprocessor.v2.PreProcessorService/") or self.path.startswith("/api/v1/summarize")
        )
        is_search = self.path in {"/services.search.v2.SearchService/SearchArticles", "/services.search.v2.SearchService/SearchRecaps"}
        if (is_search and self.peer() == "alt-backend") or (is_preprocessor and self.peer() in {"alt-backend", "alt-harvester"}):
            return self.forward_upstream(method="POST")

        if self.path != "/internal/token/introspect":
            return self.reply(404, {})
        if self.peer() != "search-indexer":
            return self.reply(403, {})
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if not 0 < length <= 8192:
                return self.reply(400, {})
            token = json.loads(self.rfile.read(length))["token"]
            if not isinstance(token, str):
                return self.reply(400, {})
        except (ValueError, KeyError, TypeError):
            return self.reply(400, {})
        result = introspect(token, self.server.verify_key, int(time.time()))
        if not result["active"]:
            result = introspect(token, self.server.verify_key, int(time.time()), "auth-hub")
        return self.reply(200, result)

    def do_GET(self):
        if os.environ.get("STAGING_FORWARD_PREPROCESSOR") == "true" and self.path.startswith("/api/v1/summarize") and self.peer() in {"alt-backend", "alt-harvester"}:
            return self.forward_upstream(method="GET")

        if self.path != "/internal/system-user":
            return self.reply(404, {})
        if self.peer() not in {"alt-backend", "alt-data-hub"}:
            return self.reply(403, {})
        if not hmac.compare_digest(self.headers.get("X-Internal-Auth", ""), os.environ["STAGING_INTERNAL_AUTH"]):
            return self.reply(403, {})
        return self.reply(200, {"user_id": "00000000-0000-4000-a000-000000000001"})


def main():
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.minimum_version = ssl.TLSVersion.TLSv1_3
    context.verify_mode = ssl.CERT_REQUIRED
    context.load_cert_chain("/certs/auth-hub.pem", "/certs/auth-hub-key.pem")
    context.load_verify_locations("/certs/ca-bundle.pem")
    server = ThreadingHTTPServer(("0.0.0.0", 9443), Handler)
    server.verify_key = load_public_key(Path(os.environ["BACKEND_TOKEN_VERIFY_KEY_FILE"]).read_bytes())
    server.socket = context.wrap_socket(server.socket, server_side=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
