"""Unit tests for user identity verification and interceptor."""

from __future__ import annotations

import time
from collections.abc import AsyncIterator
from pathlib import Path
from unittest.mock import MagicMock
from uuid import uuid4

import jwt
import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError
from connectrpc.method import IdempotencyLevel, MethodInfo
from connectrpc.request import Headers, RequestContext
from cryptography.exceptions import UnsupportedAlgorithm
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey, Ed25519PublicKey
from cryptography.hazmat.primitives.asymmetric.rsa import generate_private_key

from acolyte.config.settings import Settings
from acolyte.infra.user_identity import (
    BACKEND_TOKEN_HEADER,
    UserIdentityInterceptor,
    current_user_id,
    get_acting_user_id,
)
from tests.backend_token import public_key_pem, sign_backend_token, write_public_key


def _interceptor(private_key: Ed25519PrivateKey) -> UserIdentityInterceptor:
    return UserIdentityInterceptor(private_key.public_key(), issuer="auth-hub", audience="alt-backend")


def _make_ctx(method_name: str = "CreateReport", token: str | None = None) -> RequestContext:
    headers = Headers()
    if token is not None:
        headers[BACKEND_TOKEN_HEADER] = token

    method_info = MethodInfo(
        name=method_name,
        service_name="alt.acolyte.v1.AcolyteService",
        input=MagicMock(),
        output=MagicMock(),
        idempotency_level=IdempotencyLevel.UNKNOWN,
    )
    return RequestContext(
        method=method_info,
        http_method="POST",
        request_headers=headers,
    )


async def _ok(req: object, req_ctx: RequestContext) -> str:
    return "ok"


async def _assert_unauthenticated(interceptor: UserIdentityInterceptor, token: str | None) -> None:
    ctx = _make_ctx("CreateReport", token)
    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(_ok, MagicMock(), ctx)
    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_valid_token_sets_acting_user() -> None:
    key = Ed25519PrivateKey.generate()
    uid = uuid4()
    token = sign_backend_token(key, uid)
    interceptor = _interceptor(key)

    ctx = _make_ctx("CreateReport", token)
    captured_uid = None

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        nonlocal captured_uid
        captured_uid = get_acting_user_id(req_ctx)
        assert getattr(req_ctx, "user_id", None) == uid
        assert current_user_id.get() == uid
        return "ok"

    res = await interceptor.intercept_unary(call_next, MagicMock(), ctx)
    assert res == "ok"
    assert captured_uid == uid


@pytest.mark.asyncio
async def test_expired_token_raises_unauthenticated() -> None:
    key = Ed25519PrivateKey.generate()
    await _assert_unauthenticated(_interceptor(key), sign_backend_token(key, uuid4(), expires_in=-100))


@pytest.mark.asyncio
async def test_wrong_audience_raises_unauthenticated() -> None:
    key = Ed25519PrivateKey.generate()
    await _assert_unauthenticated(_interceptor(key), sign_backend_token(key, uuid4(), audience="wrong-service"))


@pytest.mark.asyncio
async def test_wrong_issuer_raises_unauthenticated() -> None:
    key = Ed25519PrivateKey.generate()
    await _assert_unauthenticated(_interceptor(key), sign_backend_token(key, uuid4(), issuer="evil-issuer"))


@pytest.mark.asyncio
async def test_missing_token_raises_unauthenticated() -> None:
    await _assert_unauthenticated(_interceptor(Ed25519PrivateKey.generate()), None)


@pytest.mark.asyncio
async def test_token_signed_by_a_different_key_raises_unauthenticated() -> None:
    await _assert_unauthenticated(
        _interceptor(Ed25519PrivateKey.generate()),
        sign_backend_token(Ed25519PrivateKey.generate(), uuid4()),
    )


@pytest.mark.asyncio
async def test_invalid_uuid_subject_raises_unauthenticated() -> None:
    key = Ed25519PrivateKey.generate()
    await _assert_unauthenticated(_interceptor(key), sign_backend_token(key, "not-a-valid-uuid"))


@pytest.mark.asyncio
async def test_health_check_bypasses_token_verification() -> None:
    interceptor = _interceptor(Ed25519PrivateKey.generate())
    ctx = _make_ctx("HealthCheck", token=None)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "health_ok"

    res = await interceptor.intercept_unary(call_next, MagicMock(), ctx)
    assert res == "health_ok"


@pytest.mark.asyncio
async def test_hs256_token_signed_with_the_public_key_bytes_is_rejected() -> None:
    key = Ed25519PrivateKey.generate()
    now = int(time.time())
    claims = {"sub": str(uuid4()), "iss": "auth-hub", "aud": "alt-backend", "iat": now, "exp": now + 3600}
    token = jwt.encode(
        claims,
        key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw),
        algorithm="HS256",
    )
    await _assert_unauthenticated(_interceptor(key), token)


@pytest.mark.asyncio
async def test_alg_none_token_raises_unauthenticated() -> None:
    now = int(time.time())
    claims = {"sub": str(uuid4()), "iss": "auth-hub", "aud": "alt-backend", "iat": now, "exp": now + 3600}
    token = jwt.encode(claims, key="", algorithm="none")
    await _assert_unauthenticated(_interceptor(Ed25519PrivateKey.generate()), token)


@pytest.mark.asyncio
async def test_dev_user_id_authenticates_without_token() -> None:
    dev_uid = uuid4()
    interceptor = UserIdentityInterceptor(dev_user_id=dev_uid)
    ctx = _make_ctx("CreateReport", token=None)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        assert get_acting_user_id(req_ctx) == dev_uid
        return "ok"

    res = await interceptor.intercept_unary(call_next, MagicMock(), ctx)
    assert res == "ok"


def test_missing_verify_key_and_dev_user_id_raises_value_error() -> None:
    with pytest.raises(ValueError, match="Either verify_key or dev_user_id must be provided"):
        UserIdentityInterceptor()


@pytest.mark.asyncio
async def test_server_stream_interceptor_sets_acting_user() -> None:
    key = Ed25519PrivateKey.generate()
    uid = uuid4()
    interceptor = _interceptor(key)
    ctx = _make_ctx("StreamRunProgress", sign_backend_token(key, uid))

    async def call_next(req: object, req_ctx: RequestContext) -> AsyncIterator[str]:
        assert get_acting_user_id(req_ctx) == uid
        yield "event-1"
        yield "event-2"

    events = [ev async for ev in interceptor.intercept_server_stream(call_next, MagicMock(), ctx)]
    assert events == ["event-1", "event-2"]


def test_valid_verify_key_file_loads_an_ed25519_public_key(tmp_path: Path) -> None:
    key = Ed25519PrivateKey.generate()
    path = write_public_key(key, tmp_path / "verify.pem")
    settings = Settings(backend_token_verify_key_file=str(path), backend_token_verification="enabled")

    loaded = settings.resolve_backend_token_verify_key()

    assert isinstance(loaded, Ed25519PublicKey)
    assert public_key_pem_of(loaded) == public_key_pem(key)


def public_key_pem_of(public_key: Ed25519PublicKey) -> bytes:
    return public_key.public_bytes(serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo)


def test_missing_verify_key_file_raises_at_startup(tmp_path: Path) -> None:
    settings = Settings(
        backend_token_verify_key_file=str(tmp_path / "does_not_exist.pem"),
        backend_token_verification="enabled",
    )
    with pytest.raises(RuntimeError, match="missing or unreadable"):
        settings.resolve_backend_token_verify_key()


def test_empty_verify_key_path_raises_at_startup() -> None:
    settings = Settings(backend_token_verify_key_file="", backend_token_verification="enabled")
    with pytest.raises(RuntimeError, match="BACKEND_TOKEN_VERIFY_KEY_FILE"):
        settings.resolve_backend_token_verify_key()


def test_garbage_verify_key_file_raises_at_startup(tmp_path: Path) -> None:
    path = tmp_path / "garbage.pem"
    path.write_text("not a pem")
    settings = Settings(backend_token_verify_key_file=str(path), backend_token_verification="enabled")
    with pytest.raises(RuntimeError, match="Ed25519"):
        settings.resolve_backend_token_verify_key()


def test_non_ed25519_verify_key_file_raises_at_startup(tmp_path: Path) -> None:
    rsa_public = generate_private_key(public_exponent=65537, key_size=2048).public_key()
    path = tmp_path / "rsa.pem"
    path.write_bytes(
        rsa_public.public_bytes(serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo)
    )
    settings = Settings(backend_token_verify_key_file=str(path), backend_token_verification="enabled")
    with pytest.raises(RuntimeError, match="Ed25519"):
        settings.resolve_backend_token_verify_key()


def test_private_key_file_is_not_accepted_as_a_verify_key(tmp_path: Path) -> None:
    path = tmp_path / "private.pem"
    path.write_bytes(
        Ed25519PrivateKey.generate().private_bytes(
            serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()
        )
    )
    settings = Settings(backend_token_verify_key_file=str(path), backend_token_verification="enabled")
    with pytest.raises(RuntimeError, match="Ed25519"):
        settings.resolve_backend_token_verify_key()


def test_unsupported_key_type_in_verify_key_file_raises_a_startup_error(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    path = write_public_key(Ed25519PrivateKey.generate(), tmp_path / "verify.pem")

    def _unsupported(_data: bytes) -> object:
        msg = "unsupported key type"
        raise UnsupportedAlgorithm(msg)

    monkeypatch.setattr("acolyte.config.settings.serialization.load_pem_public_key", _unsupported)
    settings = Settings(backend_token_verify_key_file=str(path), backend_token_verification="enabled")
    with pytest.raises(RuntimeError, match="Ed25519"):
        settings.resolve_backend_token_verify_key()


def test_disabled_verification_returns_none() -> None:
    settings = Settings(backend_token_verify_key_file="", backend_token_verification="disabled")
    assert settings.resolve_backend_token_verify_key() is None
