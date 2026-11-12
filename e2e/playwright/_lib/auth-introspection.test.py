"""Offline tests of the actual bounded staging JWT verifier; no listener."""
import base64
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import unittest.mock

spec = importlib.util.spec_from_file_location("fixture_auth", Path(__file__).parents[1] / "_fixtures/auth-introspection.py")
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)


def generate_key(directory):
    """Throwaway Ed25519 pair; returns (private key path, raw public key)."""
    private = Path(directory) / "key.pem"
    subprocess.run(["openssl", "genpkey", "-algorithm", "ED25519", "-out", private], check=True)
    public = subprocess.run(["openssl", "pkey", "-in", private, "-pubout"], check=True, capture_output=True).stdout
    return private, fixture.load_public_key(public)


KEYDIR = tempfile.TemporaryDirectory()
PRIVATE, KEY = generate_key(KEYDIR.name)
OTHER_DIR = tempfile.TemporaryDirectory()
OTHER_PRIVATE, _ = generate_key(OTHER_DIR.name)
USER = "11111111-1111-4111-a111-111111111111"
TENANT = "22222222-2222-4222-a222-222222222222"


def sign(claims, header=None, key=PRIVATE):
    encode = lambda value: base64.urlsafe_b64encode(json.dumps(value).encode()).decode().rstrip("=")
    data = f"{encode(header if header is not None else {'alg': 'EdDSA'})}.{encode(claims)}"
    with tempfile.NamedTemporaryFile() as message:
        message.write(data.encode())
        message.flush()
        raw = subprocess.run(["openssl", "pkeyutl", "-sign", "-inkey", key, "-rawin", "-in", message.name], check=True, capture_output=True).stdout
    return data + "." + base64.urlsafe_b64encode(raw).decode().rstrip("=")


class FixtureAuthTest(unittest.TestCase):
    def claims(self, **changes):
        return {"sub": USER, "tenant_id": TENANT, "exp": 1300, "iss": "alt-staging-auth-hub", "aud": "alt-backend", **changes}

    def test_accepts_valid_owner_and_retains_tenant(self):
        self.assertEqual(fixture.introspect(sign(self.claims()), KEY, 1000), {"active": True, "sub": USER, "tenant_id": TENANT, "exp": 1300})

    def test_rejects_bad_claims_and_types(self):
        for changes in [{"exp": 1000}, {"exp": 1301}, {"exp": True}, {"sub": 12}, {"sub": "not-a-uuid"}, {"tenant_id": None}, {"tenant_id": "00000000-0000-0000-0000-000000000000"}, {"iss": "other"}, {"aud": "other"}]:
            with self.subTest(changes=changes):
                self.assertFalse(fixture.introspect(sign(self.claims(**changes)), KEY, 1000)["active"])

    def test_missing_claims_and_non_object_json_fail_closed(self):
        for name in ["sub", "tenant_id", "exp"]:
            claims = self.claims()
            del claims[name]
            self.assertFalse(fixture.introspect(sign(claims), KEY, 1000)["active"])
        for value in [[], 123, "text", None]:
            self.assertFalse(fixture.introspect(sign(value), KEY, 1000)["active"])
            self.assertFalse(fixture.introspect(sign(self.claims(), header=value if value is not None else []), KEY, 1000)["active"])

    def test_bad_alg_signature_encoding_and_compact_shape_fail_closed(self):
        for token in ["", "a.b", "a.b.c.d", "!a.b.c", sign(self.claims(), header={"alg": "none"}), sign(self.claims(), header={"alg": "HS256"}), sign(self.claims(), key=OTHER_PRIVATE)]:
            self.assertFalse(fixture.introspect(token, KEY, 1000)["active"])


class MalleabilityAndEncodingTest(unittest.TestCase):
    # y = 2**255 - 1 is >= p, so it is not a canonical point encoding.
    BAD_POINT = b"\xff" * 31 + b"\x7f"

    def parts(self):
        claims = {"sub": USER, "tenant_id": TENANT, "exp": 1300, "iss": "alt-staging-auth-hub", "aud": "alt-backend"}
        header, payload, signature = sign(claims).split(".")
        decode = lambda value: base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))
        return header, payload, decode(signature)

    def token(self, header, payload, signature):
        return ".".join([header, payload, base64.urlsafe_b64encode(signature).decode().rstrip("=")])

    def test_signature_with_group_order_added_to_s_is_rejected(self):
        header, payload, signature = self.parts()
        s = int.from_bytes(signature[32:], "little")
        mauled = signature[:32] + (s + fixture._Q).to_bytes(32, "little")
        self.assertTrue(fixture.introspect(self.token(header, payload, signature), KEY, 1000)["active"])
        self.assertFalse(fixture.introspect(self.token(header, payload, mauled), KEY, 1000)["active"])

    def test_undecodable_r_is_rejected(self):
        header, payload, signature = self.parts()
        broken = self.BAD_POINT + signature[32:]
        self.assertFalse(fixture.introspect(self.token(header, payload, broken), KEY, 1000)["active"])

    def test_undecodable_public_key_is_rejected(self):
        header, payload, signature = self.parts()
        self.assertFalse(fixture.introspect(self.token(header, payload, signature), self.BAD_POINT, 1000)["active"])
        self.assertFalse(fixture.ed25519_verify(self.BAD_POINT, b"message", signature))


class HandlerForwardingTest(unittest.TestCase):
    def make_handler(self, path, method="POST", headers=None, body=b"", peer_name="alt-backend"):
        h = fixture.Handler.__new__(fixture.Handler)
        h.path = path
        h.command = method
        h.headers = headers or {"Content-Length": str(len(body)), "Content-Type": "application/json"}
        h.rfile = io.BytesIO(body)
        h.wfile = io.BytesIO()
        h._headers_buffer = []
        h.peer = lambda: peer_name
        h.send_response = unittest.mock.MagicMock()
        h.send_header = unittest.mock.MagicMock()
        h.end_headers = unittest.mock.MagicMock()
        return h

    def test_empty_post_retains_post_method(self):
        h = self.make_handler(
            "/services.search.v2.SearchService/SearchArticles",
            method="POST",
            headers={"Content-Length": "0"},
            body=b"",
            peer_name="alt-backend",
        )
        with unittest.mock.patch("urllib.request.urlopen") as mock_urlopen:
            mock_resp = unittest.mock.MagicMock()
            mock_resp.status = 200
            mock_resp.read.return_value = b"{}"
            mock_resp.headers = {"Content-Type": "application/json"}
            mock_urlopen.return_value.__enter__.return_value = mock_resp
            h.do_POST()
            self.assertTrue(mock_urlopen.called)
            req = mock_urlopen.call_args[0][0]
            self.assertEqual(req.get_method(), "POST")

    def test_wrong_search_peer_rejected(self):
        h = self.make_handler(
            "/services.search.v2.SearchService/SearchArticles",
            method="POST",
            body=b"{}",
            peer_name="alt-harvester",
        )
        with unittest.mock.patch("urllib.request.urlopen") as mock_urlopen:
            h.do_POST()
            self.assertFalse(mock_urlopen.called)
            h.send_response.assert_called_with(404)

    def test_preprocessor_allowed_and_denied_peers(self):
        with unittest.mock.patch.dict("os.environ", {"STAGING_FORWARD_PREPROCESSOR": "true"}):
            # Allowed peers: alt-backend and alt-harvester
            for peer in ("alt-backend", "alt-harvester"):
                h = self.make_handler("/api/v1/summarize", method="POST", body=b"{}", peer_name=peer)
                with unittest.mock.patch("urllib.request.urlopen") as mock_urlopen:
                    mock_resp = unittest.mock.MagicMock()
                    mock_resp.status = 200
                    mock_resp.read.return_value = b"{}"
                    mock_resp.headers = {"Content-Type": "application/json"}
                    mock_urlopen.return_value.__enter__.return_value = mock_resp
                    h.do_POST()
                    self.assertTrue(mock_urlopen.called, f"expected {peer} to be forwarded")
            # Denied peer: unauthorized-client
            h_denied = self.make_handler("/api/v1/summarize", method="POST", body=b"{}", peer_name="unauthorized-client")
            with unittest.mock.patch("urllib.request.urlopen") as mock_urlopen:
                h_denied.do_POST()
                self.assertFalse(mock_urlopen.called)
                h_denied.send_response.assert_called_with(404)

    def test_upstream_http_error_response_preservation(self):
        with unittest.mock.patch.dict("os.environ", {"STAGING_FORWARD_PREPROCESSOR": "true"}):
            h = self.make_handler("/api/v1/summarize", method="POST", body=b"{}", peer_name="alt-backend")
            import urllib.error
            error = urllib.error.HTTPError(
                url="http://alt-backend-deps-stub/api/v1/summarize",
                code=422,
                msg="Unprocessable Entity",
                hdrs={"Content-Type": "application/json"},
                fp=io.BytesIO(b'{"detail":"invalid input"}'),
            )
            with unittest.mock.patch("urllib.request.urlopen", side_effect=error):
                h.do_POST()
                h.send_response.assert_called_with(422)
                self.assertEqual(h.wfile.getvalue(), b'{"detail":"invalid input"}')


if __name__ == "__main__":
    unittest.main()
