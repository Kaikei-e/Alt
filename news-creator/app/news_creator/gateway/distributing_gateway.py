"""Distributing Gateway — decorator over OllamaGateway for distributed BE chat dispatch."""

from __future__ import annotations

import asyncio
import contextvars
import logging
import math
from collections.abc import AsyncIterator
from contextlib import AsyncExitStack, asynccontextmanager
from dataclasses import dataclass
from typing import Any

from news_creator.config.distributed_be_config import RemoteTarget
from news_creator.domain.models import LLMGenerateResponse
from news_creator.gateway import dispatch_metrics
from news_creator.gateway.chat_messages import gemma_prompt_to_messages
from news_creator.gateway.payload_builder import prepare_raw_prompt
from news_creator.gateway.remote_chat_driver import (
    RemoteChatDriver,
    RemoteRequestRejectedError,
)
from news_creator.gateway.remote_health_checker import RemoteHealthChecker
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)

_TOKEN_BUDGET_SAFETY_MARGIN = 256


@dataclass
class _DeferredSlotState:
    stack: AsyncExitStack
    owner_task: asyncio.Task[Any] | None = None
    closed: bool = False
    local_acquired: bool = False
    local_cancel_event: Any | None = None
    local_task_id: str | None = None


_deferred_be_slot: contextvars.ContextVar[_DeferredSlotState | None] = (
    contextvars.ContextVar("_deferred_be_slot", default=None)
)


class DistributingGateway(LLMProviderPort):
    """LLMProviderPort decorator that dispatches BE requests to OpenAI-compatible remote targets."""

    def __init__(
        self,
        local_gateway: LLMProviderPort,
        health_checker: RemoteHealthChecker,
        remote_driver: RemoteChatDriver,
        enabled: bool = True,
    ):
        self._local = local_gateway
        self._health_checker = health_checker
        self._remote_driver = remote_driver
        self._enabled = enabled

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

    async def chat_generate(
        self, payload: dict[str, Any], *, priority: str = "high"
    ) -> dict[str, Any]:
        return await self._local.chat_generate(payload, priority=priority)

    async def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        stream: bool = False,
        keep_alive: int | str | None = None,
        format: str | dict[str, Any] | None = None,
        options: dict[str, Any] | None = None,
        priority: str = "low",
    ) -> LLMGenerateResponse | AsyncIterator[LLMGenerateResponse]:
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

    @asynccontextmanager
    async def hold_slot(self, is_high_priority: bool = False):
        if is_high_priority or not self._enabled:
            token = _deferred_be_slot.set(None)
            try:
                async with self._local.hold_slot(is_high_priority) as slot:
                    yield slot
            finally:
                _deferred_be_slot.reset(token)
            return

        async with AsyncExitStack() as stack:
            state = _DeferredSlotState(
                stack=stack,
                owner_task=asyncio.current_task(),
            )
            token = _deferred_be_slot.set(state)
            try:
                yield 0.0, None, None
            finally:
                state.closed = True
                _deferred_be_slot.reset(token)

    async def generate_raw(
        self,
        prompt: str,
        *,
        cancel_event: Any | None = None,
        task_id: str | None = None,
        model: str | None = None,
        num_predict: int | None = None,
        keep_alive: int | str | None = None,
        format: str | dict[str, Any] | None = None,
        options: dict[str, Any] | None = None,
    ) -> LLMGenerateResponse:
        slot_state = _deferred_be_slot.get(None)
        if slot_state is None:
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

        if slot_state.closed or slot_state.owner_task is not asyncio.current_task():
            raise RuntimeError(
                "Deferred BE slot must only be used by its owning task while open"
            )

        if format is not None:
            raise ValueError(
                "Structured output format is not supported on remote BE chat dispatch"
            )

        prompt = prepare_raw_prompt(prompt)

        if not slot_state.local_acquired:
            messages = gemma_prompt_to_messages(prompt)
            merged_options = self._merge_options(
                num_predict=num_predict, options=options
            )
            max_tokens = int(merged_options.get("num_predict", 1200))
            temperature = float(merged_options.get("temperature", 0.7))
            top_p = float(merged_options.get("top_p", 0.85))

            utf8_bytes = sum(len(m["content"].encode("utf-8")) for m in messages)
            estimated_prompt_tokens = math.ceil(utf8_bytes / 3)
            required_ctx = (
                estimated_prompt_tokens + max_tokens + _TOKEN_BUDGET_SAFETY_MARGIN
            )

            tried: set[RemoteTarget] = set()
            last_failed_target: RemoteTarget | None = None
            last_failure_reason = "error"

            def _fits(target: RemoteTarget) -> bool:
                return target not in tried and required_ctx <= target.num_ctx

            while True:
                target = self._health_checker.acquire_idle_target(_fits)
                if target is None:
                    if last_failed_target is not None:
                        dispatch_metrics.record_fallback(
                            from_remote_url=last_failed_target.base_url,
                            to="local",
                            reason=(
                                "rejected"
                                if last_failure_reason == "rejected"
                                else "exhausted"
                            ),
                        )
                    break

                if last_failed_target is not None:
                    dispatch_metrics.record_fallback(
                        from_remote_url=last_failed_target.base_url,
                        to="next_remote",
                        reason=last_failure_reason,
                    )

                try:
                    async with dispatch_metrics.dispatch_context(
                        remote_url=target.base_url,
                        model=target.model,
                    ) as obs:
                        response = await self._remote_driver.chat(
                            target,
                            messages,
                            max_tokens=max_tokens,
                            temperature=temperature,
                            top_p=top_p,
                        )
                        obs.set_outcome("success")
                    self._health_checker.mark_success(target)
                    logger.info(
                        "Remote BE chat dispatch succeeded (served_model=%s)",
                        response.model,
                        extra={
                            "dispatch_target": "remote",
                            "remote_url": target.base_url,
                            "model": target.model,
                            "served_model": response.model,
                        },
                    )
                    return response
                except RemoteRequestRejectedError:
                    tried.add(target)
                    last_failed_target = target
                    last_failure_reason = "rejected"
                    self._health_checker.release_target(target)
                except RuntimeError:
                    tried.add(target)
                    last_failed_target = target
                    last_failure_reason = "error"
                    self._health_checker.mark_failure(target)
                except BaseException:
                    self._health_checker.release_target(target)
                    raise

            (
                _,
                slot_state.local_cancel_event,
                slot_state.local_task_id,
            ) = await slot_state.stack.enter_async_context(
                self._local.hold_slot(is_high_priority=False)
            )
            slot_state.local_acquired = True

        return await self._local.generate_raw(
            prompt,
            cancel_event=slot_state.local_cancel_event,
            task_id=slot_state.local_task_id,
            model=model,
            num_predict=num_predict,
            keep_alive=keep_alive,
            format=format,
            options=options,
        )

    def queue_status(self) -> dict[str, Any]:
        local_status = self._local.queue_status()
        local_status["remotes"] = self._health_checker.status()
        local_status["distributed_be_enabled"] = self._enabled
        return local_status

    async def list_models(self) -> list[dict[str, Any]]:
        return await self._local.list_models()

    def _merge_options(
        self,
        *,
        num_predict: int | None,
        options: dict[str, Any] | None,
    ) -> dict[str, Any]:
        config = getattr(self._local, "config", None)
        if config is not None:
            llm_options = dict(config.get_llm_options())
            if options:
                filtered = {k: v for k, v in options.items() if k != "num_ctx"}
                llm_options.update(filtered)
            if num_predict is not None:
                llm_options["num_predict"] = num_predict
            return llm_options

        llm_options = dict(options or {})
        if num_predict is not None:
            llm_options["num_predict"] = num_predict
        return llm_options
