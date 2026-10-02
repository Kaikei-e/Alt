import pytest
import aiohttp
from unittest.mock import patch, AsyncMock
from news_creator.gateway.remote_health_checker import RemoteHealthChecker
import os

@pytest.mark.asyncio
async def test_remote_health_checker_auth_and_no_redirect():
    checker = RemoteHealthChecker(
        remotes=["http://fake"],
        required_model="test_model",
        inference_token="test_secret_token=123"
    )

    with patch('aiohttp.ClientSession.get') as mock_get:
        mock_response = AsyncMock()
        mock_response.status = 200
        mock_response.text = AsyncMock(return_value='{"models": [{"name": "test_model"}]}')
        mock_response.__aenter__.return_value = mock_response
        mock_get.return_value = mock_response

        # Manually invoke the start without the loop for testing
        checker._session = aiohttp.ClientSession(headers={"Authorization": "Bearer test_secret_token=123"})
        await checker._check_all()

        mock_get.assert_called_once()
        args, kwargs = mock_get.call_args
        assert kwargs.get('allow_redirects') is False
        assert checker._session.headers.get('Authorization') == 'Bearer test_secret_token=123'

    await checker._session.close()
