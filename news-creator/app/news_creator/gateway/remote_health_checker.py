"""Background health checker for remote Ollama instances.

Probes each remote with GET /api/tags at configurable intervals.
Marks remotes as healthy only if the required model is present.
Unhealthy remotes are re-probed after a cooldown period.
"""

import asyncio
import json
import logging
import time
from typing import Any, Dict, List, Optional

import aiohttp

logger = logging.getLogger(__name__)


class RemoteHealthChecker:
    """Periodic health checker for remote Ollama instances."""

    def __init__(
        self,
        remotes: List[str],
        required_model: str,
        interval_seconds: int = 30,
        cooldown_seconds: int = 60,
        timeout_seconds: int = 10,
        model_overrides: Optional[Dict[str, str]] = None,
    ):
        self._remotes = remotes
        self._required_model = required_model
        self._model_overrides = model_overrides or {}
        self._interval_seconds = interval_seconds
        self._cooldown_seconds = cooldown_seconds
        self._timeout_seconds = timeout_seconds
        self._session: Optional[aiohttp.ClientSession] = None
        self._task: Optional[asyncio.Task] = None

        # Per-remote state
        self._states: Dict[str, Dict[str, Any]] = {}
        for url in remotes:
            self._states[url] = {
                "healthy": False,
                "busy": False,
                "last_checked": 0.0,
                "last_healthy": 0.0,
                "last_assigned": 0.0,
                "last_completed": 0.0,
                "in_flight_count": 0,
                "consecutive_failures": 0,
            }

    def acquire_idle_remote(self) -> Optional[str]:
        """Reserve the next idle healthy remote, or None if all are busy/unhealthy."""
        idle_healthy = [
            url
            for url in self._remotes
            if self._states[url]["healthy"] and not self._states[url]["busy"]
        ]
        if not idle_healthy:
            return None

        # Prefer the remote that has been idle longest.
        remote_url = min(
            idle_healthy,
            key=lambda url: (
                self._states[url]["last_completed"] > 0.0,
                self._states[url]["last_completed"],
                self._states[url]["last_assigned"],
                self._remotes.index(url),
            ),
        )
        state = self._states[remote_url]
        state["busy"] = True
        state["in_flight_count"] += 1
        state["last_assigned"] = time.monotonic()
        return remote_url

    def release_remote(self, url: str) -> None:
        """Release a previously reserved remote without affecting health."""
        if url not in self._states:
            return
        state = self._states[url]
        state["busy"] = False
        state["in_flight_count"] = max(0, state["in_flight_count"] - 1)

    def mark_success(self, url: str) -> None:
        """Release a remote after successful generation."""
        if url not in self._states:
            return
        self.release_remote(url)
        self._states[url]["last_completed"] = time.monotonic()

    def get_healthy_remotes(self, exclude: Optional[set[str]] = None) -> List[str]:
        """Return healthy idle remotes in priority order, excluding any specified URLs."""
        excluded = exclude or set()
        return [
            url
            for url in self._remotes
            if (
                self._states[url]["healthy"]
                and not self._states[url]["busy"]
                and url not in excluded
            )
        ]

    def mark_failure(self, url: str) -> None:
        """Immediately mark a remote unhealthy after a dispatch failure."""
        if url not in self._states:
            return
        self.release_remote(url)
        state = self._states[url]
        state["healthy"] = False
        state["consecutive_failures"] += 1
        state["last_checked"] = time.monotonic()

    def status(self) -> List[Dict[str, Any]]:
        """Return state of all remotes for health endpoint."""
        result = []
        for url in self._remotes:
            state = self._states[url]
            result.append(
                {
                    "url": url,
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

    def _should_probe(self, url: str) -> bool:
        """Determine if a remote should be probed now."""
        state = self._states[url]
        now = time.monotonic()
        elapsed = now - state["last_checked"]

        if state["healthy"]:
            return elapsed >= self._interval_seconds
        else:
            return elapsed >= self._cooldown_seconds

    async def _probe(self, url: str) -> None:
        """Probe a single remote and update its state."""
        state = self._states[url]
        state["last_checked"] = time.monotonic()

        try:
            tags_url = f"{url.rstrip('/')}/api/tags"
            assert self._session is not None, "Session not initialized"
            async with self._session.get(tags_url) as response:
                if response.status != 200:
                    state["healthy"] = False
                    state["consecutive_failures"] += 1
                    logger.warning(
                        "Remote health check failed: HTTP %d from %s",
                        response.status,
                        url,
                        extra={"remote_url": url, "status": response.status},
                    )
                    return

                text = await response.text()
                data = json.loads(text)
                models = data.get("models", [])
                model_names = [m.get("name", "") for m in models]

                required = self._model_overrides.get(url, self._required_model)
                # Match model name with or without :latest tag suffix
                if any(
                    name == required or name.startswith(f"{required}:")
                    for name in model_names
                ):
                    state["healthy"] = True
                    state["last_healthy"] = time.monotonic()
                    state["consecutive_failures"] = 0
                    logger.debug(
                        "Remote %s healthy (model %s present)",
                        url,
                        required,
                    )
                else:
                    state["healthy"] = False
                    state["consecutive_failures"] += 1
                    logger.warning(
                        "Remote %s missing required model %s (has: %s)",
                        url,
                        required,
                        model_names,
                        extra={"remote_url": url, "models": model_names},
                    )

        except (aiohttp.ClientError, json.JSONDecodeError, Exception) as err:
            state["healthy"] = False
            state["consecutive_failures"] += 1
            logger.warning(
                "Remote health check error for %s: %s",
                url,
                err,
                extra={
                    "remote_url": url,
                    "error_type": type(err).__name__,
                },
            )

    async def _check_all(self) -> None:
        """Probe all remotes that are due for checking."""
        for url in self._remotes:
            if self._should_probe(url):
                await self._probe(url)

    async def _loop(self) -> None:
        """Background loop that periodically checks all remotes."""
        try:
            # Initial probe of all remotes
            await self._check_all()
            while True:
                await asyncio.sleep(self._interval_seconds)
                await self._check_all()
        except asyncio.CancelledError:
            logger.info("Remote health checker background task cancelled")

    async def start(self) -> None:
        """Start the background health check loop."""
        if not self._remotes:
            logger.info("No remotes configured; health checker not started")
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
                "remotes": self._remotes,
                "interval": self._interval_seconds,
                "cooldown": self._cooldown_seconds,
                "required_model": self._required_model,
            },
        )

    async def stop(self) -> None:
        """Stop the background health check loop and clean up."""
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
