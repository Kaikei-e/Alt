"""Consumer-group creation must survive a temporarily unwritable Redis.

`_ensure_consumer_group` runs before the retrying consume loop, so it was the
one step on the startup path with no retry around it. When redis-streams sat at
maxmemory under noeviction, `XGROUP CREATE` came back with an OOM
`ResponseError`, the thread died, and `/health` latched at 503 for 23 hours with
no way back short of recreating the container.
"""

import asyncio
import json
from typing import Any, cast

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


@pytest.mark.parametrize("block_timeout_ms", [1000, 5000, 30000])
def test_socket_timeout_margin_is_real_and_bounded_at_any_block_timeout(
    monkeypatch: pytest.MonkeyPatch, block_timeout_ms: int
) -> None:
    """The socket deadline must clear block_timeout_ms by enough to absorb
    network/scheduling jitter (a sub-second margin re-creates the intermittent
    idle TimeoutError noise this fix removed) while staying close enough that a
    genuinely hung Redis still surfaces within tens of seconds of the block
    window, not hours (a ms-as-seconds slip makes it ~83 minutes). Parametrized
    across the tags cadence (1s), the articles default (5s), and an operator
    raising CONSUMER_BLOCK_TIMEOUT_MS (30s) -- a hardcoded socket timeout
    passes at the defaults and reintroduces the outage at the last one.
    """
    captured_kwargs: dict[str, Any] = {}

    def fake_from_url(url: str, **kwargs: Any) -> _FakeClientForStart:
        captured_kwargs.update(kwargs)
        return _FakeClientForStart()

    monkeypatch.setattr(redis, "from_url", fake_from_url)

    config = ConsumerConfig(enabled=True, block_timeout_ms=block_timeout_ms)
    consumer = StreamConsumer(config, handler=EventHandler())
    consumer.stop()

    asyncio.run(consumer.start())

    margin = captured_kwargs["socket_timeout"] - block_timeout_ms / 1000
    assert margin >= 1.0, "margin must exceed worst-case reply round-trip jitter"
    assert margin <= 60.0, "a hung Redis must surface soon after the block window"


class _BlockRecordingClient:
    """Fake for a full start(): records what XREADGROUP is actually given."""

    def __init__(self, stop: Any) -> None:
        self._stop = stop
        self.xreadgroup_kwargs: dict[str, Any] | None = None

    async def xgroup_create(self, *args: Any, **kwargs: Any) -> None:
        del args, kwargs

    async def xreadgroup(self, **kwargs: Any) -> list[Any]:
        self.xreadgroup_kwargs = kwargs
        self._stop()
        return []

    async def xautoclaim(self, **kwargs: Any) -> tuple[str, list[Any], int]:
        del kwargs
        return ("0-0", [], 0)

    async def close(self) -> None:
        pass


def test_socket_deadline_clears_the_block_the_server_is_actually_given(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """The invariant lives between two call sites: the socket_timeout handed to
    from_url and the `block` handed to XREADGROUP. Asserting each against the
    config would let them drift apart (e.g. a hardcoded block), so this pins
    the deadline against the block value the server is actually sent.
    """
    client_by_url: dict[str, _BlockRecordingClient] = {}
    captured_kwargs: dict[str, Any] = {}

    config = ConsumerConfig(enabled=True, block_timeout_ms=5000, reclaim_interval_seconds=0.01)
    consumer = StreamConsumer(config, handler=EventHandler())
    client = _BlockRecordingClient(consumer.stop)

    def fake_from_url(url: str, **kwargs: Any) -> _BlockRecordingClient:
        captured_kwargs.update(kwargs)
        client_by_url[url] = client
        return client

    monkeypatch.setattr(redis, "from_url", fake_from_url)

    asyncio.run(consumer.start())

    assert client.xreadgroup_kwargs is not None, "consume loop never polled"
    block_ms = client.xreadgroup_kwargs["block"]
    assert captured_kwargs["socket_timeout"] > block_ms / 1000


class _ScriptedReadClient:
    """Replays scripted XREADGROUP batches and records XACK calls."""

    def __init__(self, batches: list[list[Any]]) -> None:
        self.batches = list(batches)
        self.acks: list[tuple[str, str, str]] = []

    async def xreadgroup(self, **kwargs: Any) -> list[Any]:
        del kwargs
        return self.batches.pop(0) if self.batches else []

    async def xack(self, stream: str, group: str, message_id: str) -> None:
        self.acks.append((stream, group, message_id))


class _RecordingHandler(EventHandler):
    def __init__(self, error: Exception | None = None) -> None:
        self.events: list[Any] = []
        self._error = error

    async def handle_event(self, event: Any) -> None:
        self.events.append(event)
        if self._error is not None:
            raise self._error


def _delivery(message_id: str) -> list[Any]:
    fields = {
        "event_id": f"evt-{message_id}",
        "event_type": "ArticleCreated",
        "source": "pre-processor",
        "payload": json.dumps({"article_id": "a1"}),
    }
    return [("alt:events:articles", [(message_id, fields)])]


def test_delivered_message_is_handled_then_acked() -> None:
    """An idle poll (empty read) must be a no-op, and a delivered message must
    reach the handler and only then be XACKed -- the guard between the two is
    exactly the branch a blocking read that times out client-side never got to
    take during the outage.
    """
    config = ConsumerConfig(enabled=True)
    handler = _RecordingHandler()
    consumer = StreamConsumer(config, handler)
    client = _ScriptedReadClient([[], _delivery("1-0")])
    consumer.client = cast(redis.Redis, client)

    asyncio.run(consumer._read_and_process())  # idle poll
    assert handler.events == []
    assert client.acks == []

    asyncio.run(consumer._read_and_process())  # real delivery
    assert [e.event_id for e in handler.events] == ["evt-1-0"]
    assert client.acks == [(config.stream_key, config.group_name, "1-0")]


def test_failed_message_is_left_pending_not_acked() -> None:
    """At-least-once: a handler failure must leave the entry in the PEL for the
    reclaim loop. ACKing it anyway silently drops the event (at-most-once).
    """
    config = ConsumerConfig(enabled=True)
    handler = _RecordingHandler(error=RuntimeError("model not loaded"))
    consumer = StreamConsumer(config, handler)
    client = _ScriptedReadClient([_delivery("2-0")])
    consumer.client = cast(redis.Redis, client)

    asyncio.run(consumer._read_and_process())

    assert len(handler.events) == 1, "the handler must still be attempted"
    assert client.acks == []


class _ClaimedClient:
    """Serves one reclaimed entry and records XACK calls."""

    def __init__(self, times_delivered: int = 1) -> None:
        self.acks: list[tuple[str, str, str]] = []
        self._times_delivered = times_delivered

    async def xpending_range(self, *args: Any, **kwargs: Any) -> list[Any]:
        del args, kwargs
        return [{"times_delivered": self._times_delivered}]

    async def xack(self, stream: str, group: str, message_id: str) -> None:
        self.acks.append((stream, group, message_id))


def test_reclaimed_message_is_acked_only_after_the_handler_succeeds() -> None:
    """The reclaim path carries the same at-least-once obligation as the
    delivery path, and it is the one that exists specifically so a crashed
    consumer's entries are not lost. ACKing a reclaimed entry before or
    regardless of the handler drops exactly the messages the loop was added
    to rescue.
    """
    config = ConsumerConfig(enabled=True)
    handler = _RecordingHandler(error=RuntimeError("model not loaded"))
    consumer = StreamConsumer(config, handler)
    client = _ClaimedClient()
    consumer.client = cast(redis.Redis, client)

    _stream, entries = _delivery("3-0")[0]
    asyncio.run(consumer._process_claimed_messages(entries))

    assert len(handler.events) == 1, "the handler must still be attempted"
    assert client.acks == [], "a failed reclaim must leave the entry in the PEL"


def test_reclaimed_message_is_acked_once_the_handler_succeeds() -> None:
    """The companion to the above: a reclaimed entry that processes cleanly
    must leave the PEL, or the reclaim loop re-serves it forever.
    """
    config = ConsumerConfig(enabled=True)
    handler = _RecordingHandler()
    consumer = StreamConsumer(config, handler)
    client = _ClaimedClient()
    consumer.client = cast(redis.Redis, client)

    _stream, entries = _delivery("4-0")[0]
    asyncio.run(consumer._process_claimed_messages(entries))

    assert len(handler.events) == 1
    assert client.acks == [(config.stream_key, config.group_name, "4-0")]


def test_consume_loop_survives_an_error_and_backs_off_once(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """A raising poll (e.g. TimeoutError from a genuinely hung Redis) must not
    kill the loop, and must back off before the next poll -- without the
    backoff, a persistent error becomes a hot spin flooding the log, which is
    the noise signature this incident was diagnosed by. An idle empty poll must
    NOT back off, so exactly one sleep is expected here.
    """
    delays: list[float] = []

    async def fake_sleep(seconds: float) -> None:
        delays.append(seconds)

    monkeypatch.setattr(asyncio, "sleep", fake_sleep)

    config = ConsumerConfig(enabled=True)
    consumer = StreamConsumer(config, handler=EventHandler())
    calls = {"n": 0}

    class _FlakyClient:
        async def xreadgroup(self, **kwargs: Any) -> list[Any]:
            del kwargs
            calls["n"] += 1
            if calls["n"] == 1:
                raise redis.TimeoutError("Timeout reading from socket")
            consumer.stop()
            return []

    consumer.client = cast(redis.Redis, _FlakyClient())

    asyncio.run(consumer._consume_loop())

    assert calls["n"] == 2, "the loop must poll again after an error"
    assert delays == [1], "exactly one backoff: after the error, not after the idle poll"
