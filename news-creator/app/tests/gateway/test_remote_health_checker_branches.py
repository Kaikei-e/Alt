"""Branch coverage tests for RemoteHealthChecker._probe_base_url."""

from __future__ import annotations

import pytest

from news_creator.config.distributed_be_config import RemoteTarget
from news_creator.gateway.remote_health_checker import RemoteHealthChecker


@pytest.mark.asyncio
async def test_probe_base_url_raises_when_session_not_initialized() -> None:
    target = RemoteTarget(
        base_url="http://router.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=8192,
    )
    checker = RemoteHealthChecker([target])

    with pytest.raises(RuntimeError, match="Session not initialized"):
        await checker._probe_base_url(target.base_url, [target])
