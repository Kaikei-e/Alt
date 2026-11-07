"""Background health checker for OpenAI-compatible remote LLM targets."""

from __future__ import annotations

import asyncio
import json
import logging
import time
from collections.abc import Callable
from typing import Any

import aiohttp

from news_creator.config.distributed_be_config import RemoteTarget
from news_creator.gateway import dispatch_metrics

logger = logging.getLogger(__name__)


class RemoteHealthChecker:
    """Periodic `/v1/models` health checker and in-flight slot tracker for remote targets."""

    MAX_COOLDOWN_SECONDS = 900

    def __init__(
        self,
        targets: list[RemoteTarget],
        interval_seconds: int = 30,
        cooldown_seconds: int = 60,
        timeout_seconds: int = 10,
        *,
        max_cooldown_seconds: int = MAX_COOLDOWN_SECONDS,
    ):
        self._targets = list(targets)
        self._interval_seconds = interval_seconds
        self._cooldown_seconds = cooldown_seconds
        self._max_cooldown_seconds = max(cooldown_seconds, max_cooldown_seconds)
        self._timeout_seconds = timeout_seconds
        self._session: aiohttp.ClientSession | None = None
        self._task: asyncio.Task[None] | None = None

        self._states: dict[RemoteTarget, dict[str, Any]] = {}
        self._targets_by_base_url: dict[str, list[RemoteTarget]] = {}
        for target in self._targets:
            self._states[target] = {
                "healthy": False,
                "busy": False,
                "last_checked": 0.0,
                "last_healthy": 0.0,
                "last_assigned": 0.0,
                "last_completed": 0.0,
                "in_flight_count": 0,
                "consecutive_failures": 0,
                "logged_unhealthy": False,
            }
            self._targets_by_base_url.setdefault(target.base_url, []).append(target)

    def acquire_idle_target(
        self, fits: Callable[[RemoteTarget], bool]
    ) -> RemoteTarget | None:
        """Reserve the longest-idle healthy target where `fits(target)` is True."""
        idle_fitting = [
            target
            for target in self._targets
            if (
                self._states[target]["healthy"]
                and not self._states[target]["busy"]
                and self._states[target]["in_flight_count"] == 0
                and fits(target)
            )
        ]
        if not idle_fitting:
            return None

        chosen = min(
            idle_fitting,
            key=lambda target: (
                self._states[target]["last_completed"] > 0.0,
                self._states[target]["last_completed"],
                self._targets.index(target),
            ),
        )
        state = self._states[chosen]
        state["busy"] = True
        state["in_flight_count"] += 1
        state["last_assigned"] = time.monotonic()
        return chosen

    def release_target(self, target: RemoteTarget) -> None:
        """Release a previously reserved target without affecting health."""
        state = self._states.get(target)
        if state is None:
            return
        state["busy"] = False
        state["in_flight_count"] = max(0, state["in_flight_count"] - 1)

    def mark_success(self, target: RemoteTarget) -> None:
        """Release a target after successful generation and record completion time."""
        state = self._states.get(target)
        if state is None:
            return
        self.release_target(target)
        state["last_completed"] = time.monotonic()

    def mark_failure(self, target: RemoteTarget) -> None:
        """Immediately mark a target unhealthy after a dispatch failure."""
        state = self._states.get(target)
        if state is None:
            return
        self.release_target(target)
        state["healthy"] = False
        state["consecutive_failures"] += 1
        state["last_checked"] = time.monotonic()
        dispatch_metrics.record_cooldown(remote_url=target.base_url, reason="error")

    def status(self) -> list[dict[str, Any]]:
        """Return state of all targets for health and queue status endpoints."""
        result: list[dict[str, Any]] = []
        for target in self._targets:
            state = self._states[target]
            result.append(
                {
                    "url": target.base_url,
                    "base_url": target.base_url,
                    "model": target.model,
                    "num_ctx": target.num_ctx,
                    "healthy": state["healthy"],
                    "busy": state["busy"],
                    "last_checked": state["last_checked"],
                    "last_healthy": state["last_healthy"],
                    "last_assigned": state["last_assigned"],
                    "last_completed": state["last_completed"],
                    "in_flight_count": state["in_flight_count"],
                    "consecutive_failures": state["consecutive_failures"],
                }
            )
        return result

    def _effective_cooldown(self, target: RemoteTarget) -> float:
        failures = max(1, int(self._states[target].get("consecutive_failures", 0)))
        backoff = self._cooldown_seconds * (2 ** (failures - 1))
        return float(min(backoff, self._max_cooldown_seconds))

    def _should_probe_target(self, target: RemoteTarget, now: float) -> bool:
        state = self._states[target]
        elapsed = now - state["last_checked"]
        if state["healthy"]:
            return elapsed >= self._interval_seconds
        return elapsed >= self._effective_cooldown(target)

    def _mark_target_unhealthy(
        self, target: RemoteTarget, now: float, *, always_warn: bool = False
    ) -> Callable[..., None]:
        state = self._states[target]
        state["healthy"] = False
        state["consecutive_failures"] += 1
        state["last_checked"] = now
        if always_warn:
            state["logged_unhealthy"] = True
            return logger.warning
        log_fn = logger.debug if state.get("logged_unhealthy") else logger.warning
        state["logged_unhealthy"] = True
        return log_fn

    async def _probe_base_url(self, base_url: str, targets: list[RemoteTarget]) -> None:
        now = time.monotonic()
        models_url = f"{base_url.rstrip('/')}/v1/models"

        try:
            if self._session is None:
                raise RuntimeError("Session not initialized")

            async with self._session.get(models_url, allow_redirects=False) as response:
                if response.status != 200:
                    for target in targets:
                        log_fn = self._mark_target_unhealthy(target, now)
                        log_fn(
                            "Remote health check failed: HTTP %d from %s",
                            response.status,
                            base_url,
                            extra={
                                "remote_url": base_url,
                                "model": target.model,
                                "status": response.status,
                            },
                        )
                    return

                text = await response.text()
                try:
                    payload = json.loads(text)
                except json.JSONDecodeError as err:
                    for target in targets:
                        self._mark_target_unhealthy(target, now, always_warn=True)
                    logger.warning(
                        "Remote health check received non-JSON 200 response from %s: %s",
                        base_url,
                        err,
                        extra={"remote_url": base_url},
                    )
                    return

                data_list = payload.get("data") if isinstance(payload, dict) else None
                if not isinstance(data_list, list):
                    for target in targets:
                        self._mark_target_unhealthy(target, now, always_warn=True)
                    logger.warning(
                        "Remote health check response from %s missing 'data' list",
                        base_url,
                        extra={"remote_url": base_url},
                    )
                    return

                available_ids = {
                    item.get("id")
                    for item in data_list
                    if isinstance(item, dict) and isinstance(item.get("id"), str)
                }

                for target in targets:
                    state = self._states[target]
                    state["last_checked"] = now
                    if target.model in available_ids:
                        had_failures = state.get(
                            "consecutive_failures", 0
                        ) > 0 or state.get("logged_unhealthy", False)
                        state["healthy"] = True
                        state["last_healthy"] = now
                        state["consecutive_failures"] = 0
                        state["logged_unhealthy"] = False
                        if had_failures:
                            logger.info(
                                "Remote target %s (%s) recovered and is healthy",
                                base_url,
                                target.model,
                            )
                        else:
                            logger.debug(
                                "Remote target %s (%s) healthy",
                                base_url,
                                target.model,
                            )
                    else:
                        log_fn = self._mark_target_unhealthy(target, now)
                        log_fn(
                            "Remote %s missing required model %s (has: %s)",
                            base_url,
                            target.model,
                            sorted(available_ids),
                            extra={
                                "remote_url": base_url,
                                "model": target.model,
                                "models": sorted(available_ids),
                            },
                        )

        except (aiohttp.ClientError, asyncio.TimeoutError) as err:
            for target in targets:
                log_fn = self._mark_target_unhealthy(target, now)
                log_fn(
                    "Remote health check error for %s: %s",
                    base_url,
                    err,
                    extra={
                        "remote_url": base_url,
                        "model": target.model,
                        "error_type": type(err).__name__,
                    },
                )

    async def _check_all(self) -> None:
        now = time.monotonic()
        for base_url, targets in self._targets_by_base_url.items():
            if any(self._should_probe_target(t, now) for t in targets):
                await self._probe_base_url(base_url, targets)

    async def _loop(self) -> None:
        try:
            while True:
                await asyncio.sleep(self._interval_seconds)
                await self._check_all()
        except asyncio.CancelledError:
            logger.info("Remote health checker background task cancelled")

    async def start(self) -> None:
        if not self._targets:
            logger.info("No remote targets configured; health checker not started")
            return

        timeout = aiohttp.ClientTimeout(
            total=self._timeout_seconds,
            connect=5,
        )
        self._session = aiohttp.ClientSession(timeout=timeout)
        await self._check_all()
        self._task = asyncio.create_task(self._loop())
        logger.info(
            "Remote health checker started",
            extra={
                "targets": [t.key for t in self._targets],
                "interval": self._interval_seconds,
                "cooldown": self._cooldown_seconds,
            },
        )

    async def stop(self) -> None:
        if self._task is not None:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None

        if self._session is not None and not self._session.closed:
            await self._session.close()
            self._session = None

        logger.info("Remote health checker stopped")
