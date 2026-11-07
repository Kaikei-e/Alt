"""Wire-level tests for RemoteHealthChecker GET /v1/models."""

import json
from unittest.mock import AsyncMock, MagicMock, patch

from aiohttp import web
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

    body_text = '{"object": "list", "data": [{"id": "provider-a/model-a:latest"}]}'
    body_bytes = body_text.encode("utf-8")

    with patch("aiohttp.ClientSession.get") as mock_get:
        mock_response = AsyncMock()
        mock_response.status = 200
        mock_response.headers = {"Content-Length": str(len(body_bytes))}
        mock_response.content_length = len(body_bytes)
        mock_response.text = AsyncMock(return_value=body_text)
        mock_response.read = AsyncMock(return_value=body_bytes)
        mock_response.content = MagicMock()
        mock_response.content.read = AsyncMock(return_value=body_bytes)
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


@pytest.mark.asyncio
async def test_remote_health_checker_wire_rejects_models_body_just_over_1_mib():
    """S1: Wire server returning a /v1/models body just over 1 MiB leaves the target unhealthy."""
    from news_creator.config.distributed_be_config import RemoteTarget

    one_mib = 1 * 1024 * 1024
    padding = "p" * (one_mib + 64)
    oversized_payload = json.dumps(
        {
            "object": "list",
            "data": [
                {"id": "provider-a/model-a:latest"},
                {"id": padding},
            ],
        }
    ).encode("utf-8")
    assert len(oversized_payload) > one_mib

    async def handle_v1_models(_request):
        return web.Response(
            status=200,
            body=oversized_payload,
            content_type="application/json",
        )

    app = web.Application()
    app.router.add_get("/v1/models", handle_v1_models)
    runner = web.AppRunner(app)
    await runner.setup()
    site = web.TCPSite(runner, "127.0.0.1", 0)
    await site.start()
    try:
        port = site._server.sockets[0].getsockname()[1]
        target = RemoteTarget(
            base_url=f"http://127.0.0.1:{port}",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        )
        checker = RemoteHealthChecker(
            targets=[target],
            interval_seconds=30,
            cooldown_seconds=60,
            timeout_seconds=5,
        )
        await checker.start()
        try:
            assert checker.status()[0]["healthy"] is False
        finally:
            await checker.stop()
    finally:
        await runner.cleanup()
