"""Consumer-group creation must survive a temporarily unwritable Redis.

`_ensure_consumer_group` runs before the retrying consume loop, so it was the
one step on the startup path with no retry around it. When redis-streams sat at
maxmemory under noeviction, `XGROUP CREATE` came back with an OOM
`ResponseError`, the thread died, and `/health` latched at 503 for 23 hours with
no way back short of recreating the container.
"""

import asyncio
from typing import cast

import pytest
import redis.asyncio as redis

from tag_generator.stream_consumer import (
    ConsumerConfig,
    EventHandler,
    StreamConsumer,
    is_group_already_exists,
    is_transient_response_error,
)


class _FakeClient:
    """Records `xgroup_create` calls and replays a scripted outcome per call."""

    def __init__(self, outcomes: list[Exception | None]) -> None:
        self.outcomes = list(outcomes)
        self.calls = 0

    async def xgroup_create(self, *args: object, **kwargs: object) -> None:
        del args, kwargs
        self.calls += 1
        outcome = self.outcomes.pop(0) if self.outcomes else None
        if outcome is not None:
            raise outcome


def _consumer(outcomes: list[Exception | None]) -> tuple[StreamConsumer, _FakeClient]:
    consumer = StreamConsumer(ConsumerConfig(enabled=True), handler=EventHandler())
    client = _FakeClient(outcomes)
    consumer.client = cast(redis.Redis, client)
    return consumer, client


OOM = redis.ResponseError("OOM command not allowed when used memory > 'maxmemory'.")


@pytest.mark.parametrize(
    ("message", "transient"),
    [
        ("OOM command not allowed when used memory > 'maxmemory'.", True),
        ("LOADING Redis is loading the dataset in memory", True),
        ("MASTERDOWN Link with MASTER is down", True),
        ("READONLY You can't write against a read only replica.", True),
        ("BUSYGROUP Consumer Group name already exists", False),
        ("ERR syntax error", False),
        ("WRONGTYPE Operation against a key holding the wrong kind of value", False),
    ],
)
def test_transient_classification(message: str, transient: bool) -> None:
    assert is_transient_response_error(redis.ResponseError(message)) is transient


def test_busygroup_is_recognised_as_already_existing() -> None:
    assert is_group_already_exists(redis.ResponseError("BUSYGROUP already exists"))
    assert not is_group_already_exists(OOM)


def test_retries_until_redis_accepts_the_write() -> None:
    consumer, client = _consumer([OOM, OOM, None])

    asyncio.run(consumer._ensure_consumer_group())

    assert client.calls == 3


def test_existing_group_is_not_retried() -> None:
    consumer, client = _consumer([redis.ResponseError("BUSYGROUP already exists")])

    asyncio.run(consumer._ensure_consumer_group())

    assert client.calls == 1


def test_permanent_error_fails_immediately() -> None:
    """A real bug must not be retried into a slow startup."""
    consumer, client = _consumer([redis.ResponseError("ERR syntax error")])

    with pytest.raises(redis.ResponseError):
        asyncio.run(consumer._ensure_consumer_group())

    assert client.calls == 1


def test_gives_up_after_exhausting_attempts() -> None:
    """Retrying forever would hide a Redis that is never coming back."""
    consumer, client = _consumer([OOM] * 50)

    with pytest.raises(redis.ResponseError):
        asyncio.run(consumer._ensure_consumer_group())

    assert 1 < client.calls < 50


class _FakeClientForStart:
    """Minimal client covering the calls `start()` makes before creating tasks."""

    async def xgroup_create(self, *args: object, **kwargs: object) -> None:
        del args, kwargs

    async def close(self) -> None:
        pass


def test_start_gives_the_redis_client_a_socket_timeout_above_block_timeout(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """A blocking XREADGROUP only bounds the *server's* wait; the empty reply
    still has to make a network round trip back. If the client's own
    socket-level read deadline is not comfortably larger than block_timeout_ms,
    the client times itself out before that reply lands and every idle poll
    raises `redis.exceptions.TimeoutError` instead of returning cleanly --
    reproduced live against redis-streams: block=5000ms with the redis-py
    default socket_timeout (5s) raises `TimeoutError` after 5.01s every time,
    while block=1000ms returns `[]` after 1.02s.
    """
    captured_kwargs: dict[str, object] = {}

    def fake_from_url(url: str, **kwargs: object) -> _FakeClientForStart:
        captured_kwargs["url"] = url
        captured_kwargs.update(kwargs)
        return _FakeClientForStart()

    monkeypatch.setattr(redis, "from_url", fake_from_url)

    config = ConsumerConfig(enabled=True, block_timeout_ms=5000)
    consumer = StreamConsumer(config, handler=EventHandler())
    consumer.stop()  # loops exit on their first `while not self._shutdown` check

    asyncio.run(consumer.start())

    socket_timeout = captured_kwargs.get("socket_timeout")
    assert isinstance(socket_timeout, (int, float)), "socket_timeout must be passed explicitly"
    assert socket_timeout > config.block_timeout_ms / 1000
