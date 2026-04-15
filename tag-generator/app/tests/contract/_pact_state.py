"""Shared provider-state registry helper for Pact verification.

Pact v3 sends a JSON body to the configured state handler endpoint:

    {"state": "<name>", "params": {...}, "action": "setup"|"teardown"}

This helper centralises the dispatch so each provider declares a single
``StateRegistry`` mapping state names to async setup actions. Unknown
states are logged as warnings instead of being silently ignored.
"""

from __future__ import annotations

import logging
from collections.abc import Awaitable, Callable
from typing import Any

from pydantic import BaseModel, ConfigDict

logger = logging.getLogger(__name__)

StateAction = Callable[[dict[str, Any]], Awaitable[None]]
StateRegistry = dict[str, StateAction]


class ProviderStateRequest(BaseModel):
    model_config = ConfigDict(extra="allow")

    state: str = ""
    params: dict[str, Any] = {}
    action: str = "setup"


async def dispatch(registry: StateRegistry, payload: dict[str, Any]) -> None:
    """Dispatch a provider-state payload to a registry entry.

    Teardown actions are accepted and ignored (verifier configures
    ``teardown=False``). Unknown states warn so missing registrations
    surface during test runs.
    """
    req = ProviderStateRequest.model_validate(payload)
    if req.action == "teardown":
        return
    action = registry.get(req.state)
    if action is None:
        logger.warning("Unknown provider state: %s", req.state)
        return
    await action(req.params)
