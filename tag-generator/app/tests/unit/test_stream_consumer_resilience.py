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
