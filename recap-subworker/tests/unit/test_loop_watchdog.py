"""Unit tests for the GIL-independent event-loop watchdog."""

from __future__ import annotations

import asyncio

import pytest
from pydantic import ValidationError

from recap_subworker.infra.config import Settings
from recap_subworker.infra.loop_watchdog import LoopWatchdog


@pytest.mark.asyncio
async def test_watchdog_rearms_on_schedule() -> None:
    arm_calls: list[float] = []
    cancel_calls: list[str] = []
    sleep_calls: list[float] = []

    def fake_arm(timeout: float) -> None:
        arm_calls.append(timeout)

    def fake_cancel() -> None:
        cancel_calls.append("cancel")

    async def fake_sleep(seconds: float) -> None:
        sleep_calls.append(seconds)
        await asyncio.sleep(0)

    watchdog = LoopWatchdog(
        timeout_seconds=60.0,
        arm_fn=fake_arm,
        cancel_fn=fake_cancel,
        sleep_fn=fake_sleep,
    )
    watchdog.start()

    await asyncio.sleep(0.01)
    await watchdog.stop()

    assert len(arm_calls) >= 2, "arm must be called initially and after sleep"
    assert arm_calls[0] == 60.0
    assert sleep_calls[0] == 15.0, "re-arm interval must be timeout / 4"


@pytest.mark.asyncio
async def test_watchdog_cancels_on_shutdown() -> None:
    cancel_calls: list[str] = []

    watchdog = LoopWatchdog(
        timeout_seconds=60.0,
        arm_fn=lambda t: None,
        cancel_fn=lambda: cancel_calls.append("cancel"),
        sleep_fn=lambda s: asyncio.sleep(100),
    )
    watchdog.start()
    await asyncio.sleep(0.001)
    await watchdog.stop()

    assert "cancel" in cancel_calls, "cancel_fn must be called on shutdown"


@pytest.mark.asyncio
async def test_watchdog_stopping_loop_means_no_further_rearm() -> None:
    arm_calls: list[float] = []
    sleep_event = asyncio.Event()

    async def controlled_sleep(seconds: float) -> None:
        await sleep_event.wait()

    watchdog = LoopWatchdog(
        timeout_seconds=60.0,
        arm_fn=lambda t: arm_calls.append(t),
        cancel_fn=lambda: None,
        sleep_fn=controlled_sleep,
    )
    watchdog.start()
    await asyncio.sleep(0.001)
    assert len(arm_calls) == 1, "arm called once initially"

    await watchdog.stop()
    sleep_event.set()
    await asyncio.sleep(0.005)

    assert len(arm_calls) == 1, "no further re-arms should occur after watchdog is stopped"


def test_watchdog_settings_default() -> None:
    s = Settings()
    assert s.loop_watchdog_timeout_seconds == 360
    assert s.hdbscan_timeout_seconds == 300
    assert s.loop_watchdog_timeout_seconds > s.hdbscan_timeout_seconds


def test_watchdog_settings_valid_range() -> None:
    s = Settings(loop_watchdog_timeout_seconds=360)
    assert s.loop_watchdog_timeout_seconds == 360

    s_min = Settings(loop_watchdog_timeout_seconds=31, hdbscan_timeout_seconds=30)
    assert s_min.loop_watchdog_timeout_seconds == 31

    s_max = Settings(loop_watchdog_timeout_seconds=900)
    assert s_max.loop_watchdog_timeout_seconds == 900


def test_watchdog_settings_out_of_range_fails(monkeypatch: pytest.MonkeyPatch) -> None:
    with pytest.raises(ValidationError):
        Settings(**{"loop_watchdog_timeout_seconds": 29})

    with pytest.raises(ValidationError):
        Settings(**{"loop_watchdog_timeout_seconds": 901})

    monkeypatch.setenv("RECAP_SUBWORKER_LOOP_WATCHDOG_TIMEOUT_SECONDS", "20")
    with pytest.raises(ValidationError):
        Settings()


def test_watchdog_settings_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("RECAP_SUBWORKER_LOOP_WATCHDOG_TIMEOUT_SECONDS", "400")
    s = Settings()
    assert s.loop_watchdog_timeout_seconds == 400


def test_watchdog_settings_removed_alias_not_used(monkeypatch: pytest.MonkeyPatch) -> None:
    """RECAP_LOOP_WATCHDOG_TIMEOUT_SECONDS alias was removed and must not be recognized."""
    monkeypatch.delenv("RECAP_SUBWORKER_LOOP_WATCHDOG_TIMEOUT_SECONDS", raising=False)
    monkeypatch.setenv("RECAP_LOOP_WATCHDOG_TIMEOUT_SECONDS", "450")
    s = Settings()
    assert s.loop_watchdog_timeout_seconds == 360, "removed alias must not configure the setting"


def test_watchdog_timeout_must_exceed_hdbscan_timeout() -> None:
    """Watchdog timeout must exceed HDBSCAN timeout so it does not preempt fallback."""
    with pytest.raises(ValidationError, match="preempt the HDBSCAN timeout fallback"):
        Settings(loop_watchdog_timeout_seconds=300, hdbscan_timeout_seconds=300)

    with pytest.raises(ValidationError, match="preempt the HDBSCAN timeout fallback"):
        Settings(loop_watchdog_timeout_seconds=200, hdbscan_timeout_seconds=300)

    s = Settings(loop_watchdog_timeout_seconds=360, hdbscan_timeout_seconds=300)
    assert s.loop_watchdog_timeout_seconds == 360
    assert s.hdbscan_timeout_seconds == 300


@pytest.mark.asyncio
async def test_watchdog_arm_failure_does_not_cancel_and_emits_critical_log() -> None:
    from structlog.testing import capture_logs

    cancel_calls: list[str] = []
    arm_count = 0

    def failing_arm(timeout: float) -> None:
        nonlocal arm_count
        arm_count += 1
        if arm_count > 1:
            raise RuntimeError("re-arm timer failed")

    def fake_cancel() -> None:
        cancel_calls.append("cancel")

    async def fake_sleep(seconds: float) -> None:
        await asyncio.sleep(0)

    with capture_logs() as cap_logs:
        watchdog = LoopWatchdog(
            timeout_seconds=60.0,
            arm_fn=failing_arm,
            cancel_fn=fake_cancel,
            sleep_fn=fake_sleep,
        )
        watchdog.start()
        await asyncio.sleep(0.01)

        # cancel must NOT be called when arm fails
        assert cancel_calls == [], "cancel_fn must not be called when arm raises"

        # critical log must be emitted
        critical_logs = [log for log in cap_logs if log.get("log_level") == "critical"]
        assert len(critical_logs) >= 1, "critical log must be emitted on watchdog task failure"

        with pytest.raises(RuntimeError, match="re-arm timer failed"):
            await watchdog.stop()

        assert cancel_calls == [], (
            "cancel_fn must still not be called after stop() propagates task exception"
        )


@pytest.mark.asyncio
async def test_watchdog_normal_stop_calls_cancel() -> None:
    cancel_calls: list[str] = []

    watchdog = LoopWatchdog(
        timeout_seconds=60.0,
        arm_fn=lambda t: None,
        cancel_fn=lambda: cancel_calls.append("cancel"),
    )
    watchdog.start()
    await asyncio.sleep(0.01)
    await watchdog.stop()

    assert cancel_calls == ["cancel"], "cancel_fn must be called on normal stop"


@pytest.mark.asyncio
async def test_watchdog_first_arm_raising_makes_start_raise() -> None:
    cancel_calls: list[str] = []

    def failing_arm(timeout: float) -> None:
        raise RuntimeError("first arm failed")

    watchdog = LoopWatchdog(
        timeout_seconds=60.0,
        arm_fn=failing_arm,
        cancel_fn=lambda: cancel_calls.append("cancel"),
    )
    with pytest.raises(RuntimeError, match="first arm failed"):
        watchdog.start()

    assert cancel_calls == [], "cancel_fn must not be called when first arm raises"
    assert watchdog._task is None, "task must not be created if first arm fails"


@pytest.mark.asyncio
async def test_watchdog_cancellation_cancels_timer_and_reraises() -> None:
    cancel_calls: list[str] = []

    watchdog = LoopWatchdog(
        timeout_seconds=60.0,
        arm_fn=lambda t: None,
        cancel_fn=lambda: cancel_calls.append("cancel"),
        sleep_fn=lambda s: asyncio.sleep(100),
    )
    watchdog.start()
    task = watchdog._task
    assert task is not None
    await asyncio.sleep(0.001)
    await watchdog.stop()

    assert cancel_calls == ["cancel"], "cancel_fn must be called on cancellation"
    assert task.cancelled(), "watchdog task must re-raise CancelledError and end in cancelled state"
