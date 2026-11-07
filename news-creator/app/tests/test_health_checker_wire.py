"""Wire-level tests for RemoteHealthChecker GET /v1/models."""

from unittest.mock import AsyncMock, patch

import pytest

from news_creator.gateway.remote_health_checker import RemoteHealthChecker


@pytest.mark.asyncio
async def test_remote_health_checker_gets_v1_models_with_no_auth_and_no_redirect():
    """RemoteHealthChecker probes GET {base_url}/v1/models with allow_redirects=False and no Authorization header."""
    from news_creator.config.distributed_be_config import RemoteTarget

    target = RemoteTarget(
        base_url="http://router.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )
    checker = RemoteHealthChecker(
        targets=[target],
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
    )

    with patch("aiohttp.ClientSession.get") as mock_get:
        mock_response = AsyncMock()
        mock_response.status = 200
        mock_response.text = AsyncMock(
            return_value='{"object": "list", "data": [{"id": "provider-a/model-a:latest"}]}'
        )
        mock_response.__aenter__.return_value = mock_response
        mock_get.return_value = mock_response

        await checker.start()
        try:
            mock_get.assert_called_once()
            args, kwargs = mock_get.call_args
            assert args[0] == "http://router.example:8080/v1/models"
            assert kwargs.get("allow_redirects") is False
            assert checker._session is not None
            assert "Authorization" not in checker._session.headers
        finally:
            await checker.stop()
