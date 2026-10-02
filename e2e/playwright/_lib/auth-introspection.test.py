"""Offline tests of the actual bounded staging JWT verifier; no listener."""
import base64
import hashlib
import hmac
import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("fixture_auth", Path(__file__).parents[1] / "_fixtures/auth-introspection.py")
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)
KEY = b"public-test-key"
USER = "11111111-1111-4111-a111-111111111111"
TENANT = "22222222-2222-4222-a222-222222222222"


def sign(claims, header=None, key=KEY):
    encode = lambda value: base64.urlsafe_b64encode(json.dumps(value).encode()).decode().rstrip("=")
    data = f"{encode(header if header is not None else {'alg': 'HS256'})}.{encode(claims)}"
    return data + "." + base64.urlsafe_b64encode(hmac.new(key, data.encode(), hashlib.sha256).digest()).decode().rstrip("=")


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
        for token in ["", "a.b", "a.b.c.d", "!a.b.c", sign(self.claims(), header={"alg": "none"}), sign(self.claims(), key=b"other")]:
            self.assertFalse(fixture.introspect(token, KEY, 1000)["active"])


if __name__ == "__main__":
    unittest.main()
