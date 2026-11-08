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


@pytest.mark.asyncio
async def test_lifespan_stops_and_awaits_stream_consumers_before_closing_shared_clients(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """On shutdown, lifespan must signal both stream consumers to stop and await
    their threads before closing TLS/PKI handles and cleaning up tag_service.
    """
    import asyncio
    import threading

    import tag_generator.service as service_mod
    import tag_generator.stream_consumer as stream_consumer_mod
    from tag_generator.stream_consumer import ConsumerConfig, StreamConsumer

    order: list[str] = []
    both_started = threading.Event()
    started_streams: list[str] = []
    created_consumers: list[StreamConsumer] = []

    class _FakeService:
        def __init__(self, _config: object = None) -> None:
            pass

        def run_service(self) -> None:
            pass

    class _FakeConsumer(StreamConsumer):
        async def start(self) -> None:
            started_streams.append(self.config.stream_key)
            if len(started_streams) >= 2:
                both_started.set()
            while not self._shutdown:
                await asyncio.sleep(0.005)
            await asyncio.sleep(0.02)
            order.append(f"consumer_awaited:{self.config.stream_key}")

        def stop(self) -> None:
            order.append(f"consumer_stop:{self.config.stream_key}")
            super().stop()

    def _make_consumer(config: ConsumerConfig, handler: object) -> _FakeConsumer:
        inst = _FakeConsumer(config, handler)  # type: ignore[arg-type]
        created_consumers.append(inst)
        return inst

    class _FakePki:
        async def aclose(self) -> None:
            order.append("pki_stop")

    async def _fake_enroll(_service_name: str) -> _FakePki:
        return _FakePki()

    class _FakeRuntime:
        async def aclose(self) -> None:
            order.append("tls_stop")

    async def _fake_tls(_app: object, _bind: object) -> _FakeRuntime:
        return _FakeRuntime()

    async def _fake_cleanup() -> None:
        order.append("tag_service_cleanup")

    monkeypatch.setattr(service_mod, "TagGeneratorService", _FakeService)
    monkeypatch.setattr(
        stream_consumer_mod.ConsumerConfig,
        "from_env",
        classmethod(lambda cls: ConsumerConfig(enabled=True, stream_key="alt:events:articles")),
    )
    monkeypatch.setattr(
        stream_consumer_mod.ConsumerConfig,
        "tags_stream_from_env",
        classmethod(lambda cls: ConsumerConfig(enabled=True, stream_key="alt:events:tags")),
    )
    monkeypatch.setattr(stream_consumer_mod, "StreamConsumer", _make_consumer)
    monkeypatch.setattr(auth_service, "start_enrollment", _fake_enroll)
    monkeypatch.setattr(auth_service, "start_inbound_tls_listener", _fake_tls)
    monkeypatch.setattr(auth_service.tag_service, "initialize", AsyncMock())
    monkeypatch.setattr(auth_service.tag_service, "cleanup", _fake_cleanup)

    try:
        async with auth_service.app.router.lifespan_context(auth_service.app):
            started = await asyncio.to_thread(both_started.wait, 2.0)
            assert started, "both stream consumers should start during lifespan"

        assert len(created_consumers) == 2
        assert all(c.is_stopped for c in created_consumers)
        assert "consumer_stop:alt:events:articles" in order
        assert "consumer_stop:alt:events:tags" in order
        assert "consumer_awaited:alt:events:articles" in order
        assert "consumer_awaited:alt:events:tags" in order
        assert order.index("consumer_awaited:alt:events:articles") < order.index("tls_stop")
        assert order.index("consumer_awaited:alt:events:tags") < order.index("tls_stop")
        assert order.index("tls_stop") < order.index("pki_stop")
        assert order.index("pki_stop") < order.index("tag_service_cleanup")
    finally:
        for c in created_consumers:
            StreamConsumer.stop(c)
