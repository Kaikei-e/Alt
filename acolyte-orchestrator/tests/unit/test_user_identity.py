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

from acolyte.config.settings import Settings
from acolyte.infra.user_identity import (
    BACKEND_TOKEN_HEADER,
    UserIdentityInterceptor,
    current_user_id,
    get_acting_user_id,
    resolve_backend_token_secret,
)


def _make_token(  # noqa: PLR0913 — each knob is the payload field one negative test tampers with
    user_id: str,
    secret: bytes,
    issuer: str = "auth-hub",
    audience: str = "alt-backend",
    expires_in: int = 3600,
    alg: str = "HS256",
) -> str:
    now = int(time.time())
    payload = {
        "sub": user_id,
        "iss": issuer,
        "aud": audience,
        "iat": now,
        "exp": now + expires_in,
    }
    return jwt.encode(payload, secret, algorithm=alg)


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


@pytest.mark.asyncio
async def test_valid_token_sets_acting_user() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    uid = uuid4()
    token = _make_token(str(uid), secret)
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")

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
    secret = b"test-secret-key-for-jwt-signing-12345"
    uid = uuid4()
    token = _make_token(str(uid), secret, expires_in=-100)
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("CreateReport", token)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "ok"

    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(call_next, MagicMock(), ctx)

    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_wrong_audience_raises_unauthenticated() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    uid = uuid4()
    token = _make_token(str(uid), secret, audience="wrong-service")
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("CreateReport", token)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "ok"

    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(call_next, MagicMock(), ctx)

    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_wrong_issuer_raises_unauthenticated() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    uid = uuid4()
    token = _make_token(str(uid), secret, issuer="evil-issuer")
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("CreateReport", token)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "ok"

    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(call_next, MagicMock(), ctx)

    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_missing_token_raises_unauthenticated() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("CreateReport", token=None)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "ok"

    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(call_next, MagicMock(), ctx)

    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_tampered_token_signature_raises_unauthenticated() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    other_secret = b"different-secret-key-67890"
    uid = uuid4()
    token = _make_token(str(uid), other_secret)
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("CreateReport", token)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "ok"

    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(call_next, MagicMock(), ctx)

    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_invalid_uuid_subject_raises_unauthenticated() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    token = _make_token("not-a-valid-uuid", secret)
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("CreateReport", token)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "ok"

    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(call_next, MagicMock(), ctx)

    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_health_check_bypasses_token_verification() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("HealthCheck", token=None)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "health_ok"

    res = await interceptor.intercept_unary(call_next, MagicMock(), ctx)
    assert res == "health_ok"


def test_missing_secret_file_raises_at_startup(tmp_path: Path) -> None:
    non_existent = str(tmp_path / "does_not_exist_secret.txt")
    settings = Settings(
        backend_token_secret_file=non_existent,
        backend_token_verification="enabled",
    )
    with pytest.raises(RuntimeError) as exc_info:
        resolve_backend_token_secret(settings)

    assert "missing or unreadable" in str(exc_info.value).lower()


def test_empty_secret_file_raises_at_startup(tmp_path: Path) -> None:
    empty_file = tmp_path / "empty_secret.txt"
    empty_file.write_text("")
    settings = Settings(
        backend_token_secret_file=str(empty_file),
        backend_token_verification="enabled",
    )
    with pytest.raises(RuntimeError) as exc_info:
        resolve_backend_token_secret(settings)

    assert "empty" in str(exc_info.value).lower()


def test_disabled_verification_returns_none() -> None:
    settings = Settings(
        backend_token_secret_file="",
        backend_token_verification="disabled",
    )
    secret = resolve_backend_token_secret(settings)
    assert secret is None


@pytest.mark.asyncio
async def test_alg_none_token_raises_unauthenticated() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    uid = uuid4()
    now = int(time.time())
    payload = {
        "sub": str(uid),
        "iss": "auth-hub",
        "aud": "alt-backend",
        "iat": now,
        "exp": now + 3600,
    }
    token = jwt.encode(payload, key="", algorithm="none")
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("CreateReport", token)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "ok"

    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(call_next, MagicMock(), ctx)

    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_algorithm_confusion_raises_unauthenticated() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    uid = uuid4()
    token = _make_token(str(uid), secret, alg="HS384")
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("CreateReport", token)

    async def call_next(req: object, req_ctx: RequestContext) -> str:
        return "ok"

    with pytest.raises(ConnectError) as exc_info:
        await interceptor.intercept_unary(call_next, MagicMock(), ctx)

    assert exc_info.value.code == Code.UNAUTHENTICATED


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


def test_missing_secret_and_dev_user_id_raises_value_error() -> None:
    with pytest.raises(ValueError, match="Either JWT secret or dev_user_id must be provided"):
        UserIdentityInterceptor()


@pytest.mark.asyncio
async def test_server_stream_interceptor_sets_acting_user() -> None:
    secret = b"test-secret-key-for-jwt-signing-12345"
    uid = uuid4()
    token = _make_token(str(uid), secret)
    interceptor = UserIdentityInterceptor(secret, issuer="auth-hub", audience="alt-backend")
    ctx = _make_ctx("StreamRunProgress", token)

    async def call_next(req: object, req_ctx: RequestContext) -> AsyncIterator[str]:
        assert get_acting_user_id(req_ctx) == uid
        yield "event-1"
        yield "event-2"

    events = [ev async for ev in interceptor.intercept_server_stream(call_next, MagicMock(), ctx)]
    assert events == ["event-1", "event-2"]
