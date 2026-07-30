"""A crashed consumer thread must come back on its own.

`run_consumer` used to be a one-shot: `asyncio.run(consumer.start())` inside a
`try`, and any exception set a module-level `_consumers_healthy = False` that
nothing ever reset. One transient Redis error therefore took stream-driven tag
generation down permanently and pinned `/health` at 503 until the container was
recreated.
"""

from collections.abc import Callable

from tag_generator.stream_consumer import supervise


class _Recorder:
    def __init__(self) -> None:
        self.health: dict[str, bool] = {}
        self.delays: list[float] = []

    def on_health(self, name: str, healthy: bool) -> None:
        self.health[name] = healthy

    def sleep(self, seconds: float) -> None:
        self.delays.append(seconds)


def _flaky(failures: int) -> tuple[list[int], Callable[[], None]]:
    """A runnable that raises `failures` times, then returns cleanly."""
    calls: list[int] = []

    def run() -> None:
        calls.append(len(calls) + 1)
        if len(calls) <= failures:
            raise ConnectionError("redis went away")

    return calls, run


def test_restarts_until_the_consumer_stays_up() -> None:
    calls, run = _flaky(failures=2)
    rec = _Recorder()

    supervise("articles", run, on_health=rec.on_health, sleep=rec.sleep, max_attempts=10)

    assert len(calls) == 3
    assert rec.health["articles"] is True


def test_clean_return_is_a_shutdown_not_a_crash() -> None:
    calls, run = _flaky(failures=0)
    rec = _Recorder()

    supervise("articles", run, on_health=rec.on_health, sleep=rec.sleep, max_attempts=10)

    assert len(calls) == 1
    assert not rec.delays


def test_reports_unhealthy_while_down() -> None:
    """Health must be observable during the outage, not only after recovery."""
    seen: list[bool] = []

    def run() -> None:
        raise ConnectionError("redis went away")

    def on_health(_name: str, healthy: bool) -> None:
        seen.append(healthy)

    supervise("articles", run, on_health=on_health, sleep=lambda _s: None, max_attempts=3)

    assert seen[-1] is False
    assert False in seen


def test_gives_up_only_after_the_attempt_budget() -> None:
    calls: list[int] = []

    def run() -> None:
        calls.append(1)
        raise ConnectionError("redis went away")

    rec = _Recorder()
    supervise("articles", run, on_health=rec.on_health, sleep=rec.sleep, max_attempts=4)

    assert len(calls) == 4
    assert rec.health["articles"] is False


def test_backoff_grows_and_is_capped() -> None:
    def run() -> None:
        raise ConnectionError("redis went away")

    rec = _Recorder()
    supervise(
        "articles",
        run,
        on_health=rec.on_health,
        sleep=rec.sleep,
        max_attempts=8,
        backoff_base_seconds=1.0,
        backoff_cap_seconds=4.0,
    )

    assert rec.delays == [1.0, 2.0, 4.0, 4.0, 4.0, 4.0, 4.0]
