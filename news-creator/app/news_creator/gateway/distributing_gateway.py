"""Distributing Gateway — decorator over OllamaGateway for distributed BE dispatch.

When enabled, BE (batch) hold_slot() requests are intercepted and dispatched
to a healthy remote Ollama instance, bypassing the local semaphore entirely.
RT (streaming/high-priority) requests always pass through to the local gateway.

When disabled or when all remotes are unhealthy, behaviour is identical to
using the local OllamaGateway directly.
"""

import contextvars
import logging
from contextlib import asynccontextmanager
from typing import Any, AsyncIterator, Dict, List, Optional, Union

from news_creator.domain.models import LLMGenerateResponse
from news_creator.gateway import dispatch_metrics
from news_creator.gateway.remote_health_checker import RemoteHealthChecker
from news_creator.gateway.remote_ollama_driver import RemoteOllamaDriver
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)

# Per-coroutine tracking of the remote reservation selected during hold_slot()
_dispatch_state_var: contextvars.ContextVar[Optional[Dict[str, Any]]] = (
    contextvars.ContextVar("_dispatch_state_var", default=None)
)


class DistributingGateway(LLMProviderPort):
    """LLMProviderPort decorator that distributes BE requests to remote Ollama instances."""

    def __init__(
        self,
        local_gateway: LLMProviderPort,
        health_checker: RemoteHealthChecker,
        remote_driver: RemoteOllamaDriver,
        enabled: bool = True,
        remote_model: Optional[str] = None,
        model_overrides: Optional[Dict[str, str]] = None,
    ):
        self._local = local_gateway
        self._health_checker = health_checker
        self._remote_driver = remote_driver
        self._enabled = enabled
        self._remote_model = remote_model
        self._model_overrides = model_overrides or {}

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    async def initialize(self) -> None:
        await self._local.initialize()
        if self._enabled:
            await self._remote_driver.initialize()
            await self._health_checker.start()
            logger.info("DistributingGateway initialized (distributed BE ON)")
        else:
            logger.info("DistributingGateway initialized (distributed BE OFF)")

    async def cleanup(self) -> None:
        await self._local.cleanup()
        if self._enabled:
            await self._health_checker.stop()
            await self._remote_driver.cleanup()
            logger.info("DistributingGateway cleaned up")

    # ------------------------------------------------------------------
    # chat_generate() — always local (used for plan-query, morning letter)
    # ------------------------------------------------------------------

    async def chat_generate(self, payload: Dict[str, Any]) -> Dict[str, Any]:
        return await self._local.chat_generate(payload)

    # ------------------------------------------------------------------
    # generate() — always local (handles streaming, model routing, etc.)
    # ------------------------------------------------------------------

    async def generate(
        self,
        prompt: str,
        *,
        model: Optional[str] = None,
        num_predict: Optional[int] = None,
        stream: bool = False,
        keep_alive: Optional[Union[int, str]] = None,
        format: Optional[Union[str, Dict[str, Any]]] = None,
        options: Optional[Dict[str, Any]] = None,
        priority: str = "low",
    ) -> Union[LLMGenerateResponse, AsyncIterator[LLMGenerateResponse]]:
        return await self._local.generate(
            prompt,
            model=model,
            num_predict=num_predict,
            stream=stream,
            keep_alive=keep_alive,
            format=format,
            options=options,
            priority=priority,
        )

    # ------------------------------------------------------------------
    # hold_slot() — intercept BE to dispatch to remote
    # ------------------------------------------------------------------

    @asynccontextmanager
    async def hold_slot(self, is_high_priority: bool = False):
        # RT or feature OFF → local
        if is_high_priority or not self._enabled:
            async with self._local.hold_slot(is_high_priority) as slot:
                yield slot
            return

        # BE with dispatch enabled → try remote
        remote_url = self._health_checker.acquire_idle_remote()

        if remote_url is None:
            # All remotes down → local fallback
            logger.debug(
                "No idle healthy remotes, falling back to local",
                extra={
                    "dispatch_target": "local",
                    "fallback_reason": "all_remotes_busy_or_unhealthy",
                },
            )
            async with self._local.hold_slot(is_high_priority) as slot:
                yield slot
            return

        # Remote available → no local semaphore acquisition
        logger.info(
            "Dispatching BE to remote",
            extra={
                "dispatch_target": "remote",
                "remote_url": remote_url,
            },
        )
        dispatch_state = {
            "remote_url": remote_url,
            "released": False,
        }
        token = _dispatch_state_var.set(dispatch_state)
        try:
            yield 0.0, None, None
        finally:
            state = _dispatch_state_var.get(None)
            if state is not None and not state["released"]:
                self._health_checker.release_remote(state["remote_url"])
            _dispatch_state_var.reset(token)

    # ------------------------------------------------------------------
    # generate_raw() — route based on contextvars set by hold_slot
    # ------------------------------------------------------------------

    async def generate_raw(
        self,
        prompt: str,
        *,
        cancel_event: Optional[Any] = None,
        task_id: Optional[str] = None,
        model: Optional[str] = None,
        num_predict: Optional[int] = None,
        keep_alive: Optional[Union[int, str]] = None,
        format: Optional[Union[str, Dict[str, Any]]] = None,
        options: Optional[Dict[str, Any]] = None,
    ) -> LLMGenerateResponse:
        dispatch_state = _dispatch_state_var.get(None)
        remote_url = (
            dispatch_state["remote_url"] if dispatch_state is not None else None
        )

        if remote_url is not None:
            tried: set[str] = set()
            candidate_url: Optional[str] = remote_url

            while candidate_url is not None:
                payload = self._build_payload(
                    prompt,
                    target_url=candidate_url,
                    model=model,
                    num_predict=num_predict,
                    keep_alive=keep_alive,
                    format=format,
                    options=options,
                )
                logger.info(
                    "generate_raw dispatching to remote",
                    extra={
                        "dispatch_target": "remote",
                        "remote_url": candidate_url,
                        "model": payload.get("model"),
                    },
                )
                try:
                    async with dispatch_metrics.dispatch_context(
                        remote_url=candidate_url,
                        model=str(payload.get("model") or ""),
                    ) as obs:
                        response = await self._remote_driver.generate(
                            base_url=candidate_url,
                            payload=payload,
                        )
                        obs.set_outcome("success")
                    self._health_checker.mark_success(candidate_url)
                    if (
                        dispatch_state is not None
                        and dispatch_state["remote_url"] == candidate_url
                    ):
                        dispatch_state["released"] = True
                    return response
                except RuntimeError:
                    failed_url = candidate_url
                    tried.add(candidate_url)
                    self._health_checker.mark_failure(candidate_url)
                    if (
                        dispatch_state is not None
                        and dispatch_state["remote_url"] == candidate_url
                    ):
                        dispatch_state["released"] = True
                    next_candidates = self._health_checker.get_healthy_remotes(
                        exclude=tried
                    )
                    if not next_candidates:
                        dispatch_metrics.record_fallback(
                            from_remote_url=failed_url,
                            to="local",
                            reason="exhausted",
                        )
                        logger.warning(
                            "All healthy remotes exhausted during generate_raw; falling back to local",
                            extra={
                                "dispatch_target": "local",
                                "fallback_reason": "remote_generation_failed",
                                "tried_remotes": sorted(tried),
                            },
                        )
                        break
                    dispatch_metrics.record_fallback(
                        from_remote_url=failed_url,
                        to="next_remote",
                        reason="error",
                    )
                    candidate_url = next_candidates[0]
                    replacement_state = {
                        "remote_url": candidate_url,
                        "released": False,
                    }
                    _dispatch_state_var.set(replacement_state)
                    dispatch_state = replacement_state
                    logger.warning(
                        "Remote generation failed; retrying next healthy remote",
                        extra={
                            "failed_remote_url": failed_url,
                            "next_remote_url": candidate_url,
                        },
                    )

        # Local path
        return await self._local.generate_raw(
            prompt,
            cancel_event=cancel_event,
            task_id=task_id,
            model=model,
            num_predict=num_predict,
            keep_alive=keep_alive,
            format=format,
            options=options,
        )

    # ------------------------------------------------------------------
    # Status / observability
    # ------------------------------------------------------------------

    def queue_status(self) -> Dict[str, Any]:
        """Return queue status including remote health state."""
        local_status = {}
        if hasattr(self._local, "_semaphore"):
            local_status = self._local._semaphore.queue_status()
        local_status["remotes"] = self._health_checker.status()
        local_status["distributed_be_enabled"] = self._enabled
        return local_status

    async def list_models(self) -> List[Dict[str, Any]]:
        """Delegate to local gateway."""
        return await self._local.list_models()

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _build_payload(
        self,
        prompt: str,
        *,
        target_url: Optional[str] = None,
        model: Optional[str] = None,
        num_predict: Optional[int] = None,
        keep_alive: Optional[Union[int, str]] = None,
        format: Optional[Union[str, Dict[str, Any]]] = None,
        options: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Build an Ollama-compatible payload for the remote driver."""
        # Per-remote model override takes priority
        override_model = self._model_overrides.get(target_url) if target_url else None
        # Reuse local gateway's config for model name and options
        config = getattr(self._local, "config", None)
        if config is not None:
            effective_model = (
                model or override_model or self._remote_model or config.model_name
            )
            llm_options = config.get_llm_options()
            if options:
                filtered = {k: v for k, v in options.items() if k != "num_ctx"}
                llm_options.update(filtered)
            if num_predict is not None:
                llm_options["num_predict"] = num_predict
            final_keep_alive = (
                keep_alive
                if keep_alive is not None
                else config.get_keep_alive_for_model(effective_model)
            )
        else:
            effective_model = (
                model or override_model or self._remote_model or "gemma4-e4b-q4km"
            )
            llm_options = options or {}
            if num_predict is not None:
                llm_options["num_predict"] = num_predict
            final_keep_alive = keep_alive or -1

        payload: Dict[str, Any] = {
            "model": effective_model,
            "prompt": prompt.strip(),
            "stream": False,
            "raw": True,
            "keep_alive": final_keep_alive,
            "options": llm_options,
        }
        if format is not None:
            payload["format"] = format
        return payload
