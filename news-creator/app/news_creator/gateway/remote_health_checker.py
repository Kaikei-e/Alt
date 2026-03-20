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
        self._rr_index = 0
        for url in remotes:
            self._states[url] = {
                "healthy": False,
                "last_checked": 0.0,
                "last_healthy": 0.0,
                "consecutive_failures": 0,
            }

    def get_healthy_remote(self) -> Optional[str]:
        """Return the next healthy remote via round-robin, or None."""
        healthy = [url for url in self._remotes if self._states[url]["healthy"]]
        if not healthy:
            return None
        idx = self._rr_index % len(healthy)
        self._rr_index += 1
        return healthy[idx]

    def get_healthy_remotes(self, exclude: Optional[set[str]] = None) -> List[str]:
        """Return healthy remotes in priority order, excluding any specified URLs."""
        excluded = exclude or set()
        return [
            url for url in self._remotes
            if self._states[url]["healthy"] and url not in excluded
        ]

    def mark_failure(self, url: str) -> None:
        """Immediately mark a remote unhealthy after a dispatch failure."""
        if url not in self._states:
            return
        state = self._states[url]
        state["healthy"] = False
        state["consecutive_failures"] += 1
        state["last_checked"] = time.monotonic()

    def status(self) -> List[Dict[str, Any]]:
        """Return state of all remotes for health endpoint."""
        result = []
        for url in self._remotes:
            state = self._states[url]
            result.append({
                "url": url,
                "healthy": state["healthy"],
                "last_checked": state["last_checked"],
                "last_healthy": state["last_healthy"],
                "consecutive_failures": state["consecutive_failures"],
            })
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
                if required in model_names:
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
