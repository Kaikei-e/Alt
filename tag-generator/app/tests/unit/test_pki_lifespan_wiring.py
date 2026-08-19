"""Enrollment starts before the in-process :9443 listener consumes certs."""

from __future__ import annotations

from unittest.mock import AsyncMock

import pytest

import auth_service


@pytest.mark.asyncio
async def test_lifespan_starts_enrollment_before_inbound_tls(monkeypatch: pytest.MonkeyPatch) -> None:
    order: list[str] = []

    class _FakePki:
        async def aclose(self) -> None:
            order.append("pki_stop")

    async def _fake_enroll(service_name: str) -> _FakePki:
        assert service_name == "tag-generator"
        order.append("pki_start")
        return _FakePki()

    class _FakeRuntime:
        async def aclose(self) -> None:
            order.append("tls_stop")

    async def _fake_tls(_app: object, _bind: object) -> _FakeRuntime:
        order.append("tls_start")
        return _FakeRuntime()

    monkeypatch.setattr(auth_service, "start_enrollment", _fake_enroll)
    monkeypatch.setattr(auth_service, "start_inbound_tls_listener", _fake_tls)
    monkeypatch.setattr(auth_service.tag_service, "initialize", AsyncMock())
    monkeypatch.setattr(auth_service.tag_service, "cleanup", AsyncMock())
    monkeypatch.setattr(auth_service, "_run_background_tag_generation", lambda: None)

    async with auth_service.app.router.lifespan_context(auth_service.app):
        assert order[:2] == ["pki_start", "tls_start"]

    assert "tls_stop" in order
    assert "pki_stop" in order
    assert order.index("pki_start") < order.index("tls_start")
    assert order.index("tls_stop") < order.index("pki_stop")
