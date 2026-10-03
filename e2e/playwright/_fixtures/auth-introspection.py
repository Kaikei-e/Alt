"""Bounded mTLS AuthHub test provider. Only public staging JWTs are accepted."""

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


def introspect(token, key, now, issuer="alt-staging-auth-hub"):
    try:
        header, payload, signature = token.split(".")
        if not all(re.fullmatch(r"[A-Za-z0-9_-]+", part) for part in (header, payload, signature)):
            return {"active": False}
        decode = lambda value: base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))
        protected = json.loads(decode(header))
        if not isinstance(protected, dict) or protected.get("alg") != "HS256":
            return {"active": False}
        expected = hmac.new(key, f"{header}.{payload}".encode(), hashlib.sha256).digest()
        if not hmac.compare_digest(expected, decode(signature)):
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
        result = introspect(token, self.server.fixture_key, int(time.time()))
        if not result["active"]:
            result = introspect(token, self.server.acolyte_key, int(time.time()), "auth-hub")
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
    server.fixture_key = Path(os.environ["STAGING_JWT_KEY_FILE"]).read_bytes().strip()
    server.acolyte_key = Path("/run/secrets/backend_token_secret").read_bytes().strip()
    if not server.fixture_key or not server.acolyte_key:
        raise ValueError("staging JWT key is empty")
    server.socket = context.wrap_socket(server.socket, server_side=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
