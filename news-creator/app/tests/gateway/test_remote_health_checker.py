"""Tests for RemoteHealthChecker."""

import asyncio
import json
import time
import pytest
from unittest.mock import AsyncMock, MagicMock, patch
import aiohttp

from news_creator.gateway.remote_health_checker import RemoteHealthChecker


@pytest.fixture
def remotes():
    return [
        "http://remote-a:11434",
        "http://remote-b:11434",
        "http://remote-c:11436",
    ]


@pytest.fixture
def checker(remotes):
    return RemoteHealthChecker(
        remotes=remotes,
        required_model="gemma3:4b-it-qat",
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
    )


def test_acquire_idle_remote_prefers_first_never_completed_remote(checker, remotes):
    """When all remotes are healthy and unused, the first remote is selected."""
    for url in remotes:
        checker._states[url]["healthy"] = True

    result = checker.acquire_idle_remote()
    assert result == remotes[0]
    assert checker._states[remotes[0]]["busy"] is True
    assert checker._states[remotes[0]]["in_flight_count"] == 1


def test_acquire_idle_remote_skips_unhealthy_and_busy(checker, remotes):
    """Only healthy idle remotes are candidates for selection."""
    for url in remotes:
        checker._states[url]["healthy"] = True
    checker._states[remotes[0]]["busy"] = True
    checker._states[remotes[0]]["in_flight_count"] = 1
    checker._states[remotes[1]]["healthy"] = False

    result = checker.acquire_idle_remote()
    assert result == remotes[2]


def test_all_remotes_unavailable_returns_none(checker, remotes):
    """When all remotes are unhealthy or busy, returns None."""
    for url in remotes:
        checker._states[url]["healthy"] = False

    result = checker.acquire_idle_remote()
    assert result is None


def test_cooldown_prevents_probe_spam(checker, remotes):
    """An unhealthy remote is not probed again until cooldown expires."""
    checker._states[remotes[0]]["healthy"] = False
    checker._states[remotes[0]]["last_checked"] = time.monotonic()

    assert checker._should_probe(remotes[0]) is False


def test_probe_allowed_after_cooldown(checker, remotes):
    """After cooldown expires, the remote should be probed again."""
    checker._states[remotes[0]]["healthy"] = False
    checker._states[remotes[0]]["last_checked"] = time.monotonic() - 120

    assert checker._should_probe(remotes[0]) is True


def test_healthy_remote_probed_at_interval(checker, remotes):
    """Healthy remote is probed at normal interval, not cooldown."""
    checker._states[remotes[0]]["healthy"] = True
    checker._states[remotes[0]]["last_checked"] = time.monotonic() - 35

    assert checker._should_probe(remotes[0]) is True


def test_healthy_remote_not_probed_before_interval(checker, remotes):
    """Healthy remote is not probed before interval expires."""
    checker._states[remotes[0]]["healthy"] = True
    checker._states[remotes[0]]["last_checked"] = time.monotonic() - 10

    assert checker._should_probe(remotes[0]) is False


@pytest.mark.asyncio
async def test_probe_marks_healthy_when_model_present(checker, remotes):
    """Probe marks remote as healthy when required model is in /api/tags."""
    tags_response = {"models": [{"name": "gemma3:4b-it-qat"}]}
    mock_response = MagicMock()
    mock_response.status = 200
    mock_response.text = AsyncMock(return_value=json.dumps(tags_response))
    mock_response.__aenter__ = AsyncMock(return_value=mock_response)
    mock_response.__aexit__ = AsyncMock(return_value=False)

    mock_session = MagicMock(spec=aiohttp.ClientSession)
    mock_session.closed = False
    mock_session.get = MagicMock(return_value=mock_response)
    checker._session = mock_session

    await checker._probe(remotes[0])

    assert checker._states[remotes[0]]["healthy"] is True
    assert checker._states[remotes[0]]["consecutive_failures"] == 0


@pytest.mark.asyncio
async def test_probe_marks_unhealthy_when_model_missing(checker, remotes):
    """Probe marks remote as unhealthy when required model is NOT in /api/tags."""
    tags_response = {"models": [{"name": "some-other-model"}]}
    mock_response = MagicMock()
    mock_response.status = 200
    mock_response.text = AsyncMock(return_value=json.dumps(tags_response))
    mock_response.__aenter__ = AsyncMock(return_value=mock_response)
    mock_response.__aexit__ = AsyncMock(return_value=False)

    mock_session = MagicMock(spec=aiohttp.ClientSession)
    mock_session.closed = False
    mock_session.get = MagicMock(return_value=mock_response)
    checker._session = mock_session

    await checker._probe(remotes[0])

    assert checker._states[remotes[0]]["healthy"] is False


@pytest.mark.asyncio
async def test_probe_marks_unhealthy_on_connection_error(checker, remotes):
    """Probe marks remote as unhealthy on connection error."""
    mock_session = MagicMock(spec=aiohttp.ClientSession)
    mock_session.closed = False
    mock_session.get = MagicMock(
        side_effect=aiohttp.ClientConnectorError(
            connection_key=MagicMock(), os_error=OSError("Connection refused")
        )
    )
    checker._session = mock_session

    await checker._probe(remotes[0])

    assert checker._states[remotes[0]]["healthy"] is False
    assert checker._states[remotes[0]]["consecutive_failures"] == 1


@pytest.mark.asyncio
async def test_background_check_updates_state(checker, remotes):
    """Background check loop probes and updates remote state."""
    tags_response = {"models": [{"name": "gemma3:4b-it-qat"}]}
    mock_response = MagicMock()
    mock_response.status = 200
    mock_response.text = AsyncMock(return_value=json.dumps(tags_response))
    mock_response.__aenter__ = AsyncMock(return_value=mock_response)
    mock_response.__aexit__ = AsyncMock(return_value=False)

    mock_session = MagicMock(spec=aiohttp.ClientSession)
    mock_session.closed = False
    mock_session.get = MagicMock(return_value=mock_response)
    checker._session = mock_session

    # Force all remotes to be due for probe
    for url in remotes:
        checker._states[url]["last_checked"] = 0

    await checker._check_all()

    for url in remotes:
        assert checker._states[url]["healthy"] is True


def test_status_returns_all_remote_states(checker, remotes):
    """status() returns a list with state of every remote."""
    checker._states[remotes[0]]["healthy"] = True
    checker._states[remotes[1]]["healthy"] = False
    checker._states[remotes[2]]["healthy"] = True
    checker._states[remotes[2]]["busy"] = True
    checker._states[remotes[2]]["in_flight_count"] = 1

    status = checker.status()

    assert len(status) == 3
    assert status[0]["url"] == remotes[0]
    assert status[0]["healthy"] is True
    assert status[1]["url"] == remotes[1]
    assert status[1]["healthy"] is False
    assert status[2]["url"] == remotes[2]
    assert status[2]["healthy"] is True
    assert status[2]["busy"] is True
    assert status[2]["in_flight_count"] == 1


def test_no_remotes_returns_none():
    """Empty remote list always returns None."""
    checker = RemoteHealthChecker(
        remotes=[],
        required_model="gemma3:4b-it-qat",
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
    )
    assert checker.acquire_idle_remote() is None
    assert checker.status() == []


def test_get_healthy_remotes_excludes_busy_and_requested_urls(checker, remotes):
    """Healthy idle remotes should be returned in order, excluding requested URLs."""
    for url in remotes:
        checker._states[url]["healthy"] = True
    checker._states[remotes[1]]["busy"] = True

    assert checker.get_healthy_remotes(exclude={remotes[0]}) == [remotes[2]]


def test_mark_failure_immediately_marks_remote_unhealthy(checker, remotes):
    """A generation failure should immediately demote the remote."""
    checker._states[remotes[0]]["healthy"] = True
    checker._states[remotes[0]]["busy"] = True
    checker._states[remotes[0]]["in_flight_count"] = 1

    checker.mark_failure(remotes[0])

    assert checker._states[remotes[0]]["healthy"] is False
    assert checker._states[remotes[0]]["busy"] is False
    assert checker._states[remotes[0]]["in_flight_count"] == 0
    assert checker._states[remotes[0]]["consecutive_failures"] == 1


def test_acquire_idle_remote_uses_completion_order(checker, remotes):
    """The remote that has been idle longest should be selected first."""
    for url in remotes:
        checker._states[url]["healthy"] = True
    checker._states[remotes[0]]["last_completed"] = 30.0
    checker._states[remotes[1]]["last_completed"] = 10.0
    checker._states[remotes[2]]["last_completed"] = 20.0

    assert checker.acquire_idle_remote() == remotes[1]


def test_release_remote_makes_remote_available_again(checker, remotes):
    """A released remote returns to the idle candidate pool."""
    for url in remotes:
        checker._states[url]["healthy"] = True

    chosen = checker.acquire_idle_remote()
    assert chosen == remotes[0]

    checker.release_remote(chosen)
    assert checker._states[chosen]["busy"] is False
    assert checker._states[chosen]["in_flight_count"] == 0

    next_candidates = checker.get_healthy_remotes()
    assert remotes[0] in next_candidates


def test_mark_success_updates_completion_time_and_releases_remote(checker, remotes):
    """Successful completion should clear busy state and update completion time."""
    checker._states[remotes[0]]["healthy"] = True
    checker._states[remotes[0]]["busy"] = True
    checker._states[remotes[0]]["in_flight_count"] = 1

    before = time.monotonic()
    checker.mark_success(remotes[0])

    assert checker._states[remotes[0]]["busy"] is False
    assert checker._states[remotes[0]]["in_flight_count"] == 0
    assert checker._states[remotes[0]]["last_completed"] >= before


def test_acquire_idle_remote_returns_none_when_all_busy(checker, remotes):
    """No remote should be selected when every healthy remote is busy."""
    checker._states[remotes[0]]["healthy"] = True
    checker._states[remotes[0]]["busy"] = True
    checker._states[remotes[2]]["healthy"] = True
    checker._states[remotes[2]]["busy"] = True

    assert checker.acquire_idle_remote() is None


@pytest.mark.asyncio
async def test_start_performs_initial_probe_before_returning(checker, remotes):
    """start() should perform an initial probe so remotes are available immediately after startup."""
    checker._check_all = AsyncMock()

    await checker.start()

    checker._check_all.assert_awaited_once()
    await checker.stop()
