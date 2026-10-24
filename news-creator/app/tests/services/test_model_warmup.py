"""Unit tests for ModelWarmupService."""

import logging
from unittest.mock import AsyncMock, Mock

import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.driver.ollama_driver import OllamaDriver
from news_creator.services.model_warmup import ModelWarmupService


def _make_config(
    *,
    warmup_enabled: bool = True,
    warmup_keep_alive_minutes: int = 30,
    model_routing_enabled: bool = True,
    model_name: str = "gemma4-e4b-q4km",
    model_8k_name: str = "gemma4-e4b-12k",
) -> Mock:
    config = Mock(spec=NewsCreatorConfig)
    config.warmup_enabled = warmup_enabled
    config.warmup_keep_alive_minutes = warmup_keep_alive_minutes
    config.model_routing_enabled = model_routing_enabled
    config.model_name = model_name
    config.model_8k_name = model_8k_name
    return config


class TestModelWarmupService:
    """Specification tests for ModelWarmupService."""

    @pytest.mark.asyncio
    async def test_warmup_disabled_skips_driver_calls(self):
        """When warmup_enabled=False, warmup_models() returns immediately without calling driver."""
        config = _make_config(warmup_enabled=False)
        driver = Mock(spec=OllamaDriver)
        driver.generate = AsyncMock()

        service = ModelWarmupService(config=config, driver=driver)
        await service.warmup_models()

        driver.generate.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_model_routing_enabled_warms_primary_8k_model_with_clamped_keep_alive(
        self,
    ):
        """When model_routing_enabled=True, warms model_8k_name with keep_alive clamped to >= 1440m."""
        config = _make_config(
            warmup_enabled=True,
            warmup_keep_alive_minutes=30,
            model_routing_enabled=True,
            model_8k_name="gemma4-e4b-12k",
        )
        driver = Mock(spec=OllamaDriver)
        driver.generate = AsyncMock(return_value={"response": "pong", "done": True})

        service = ModelWarmupService(config=config, driver=driver)
        await service.warmup_models()

        driver.generate.assert_awaited_once_with(
            {
                "model": "gemma4-e4b-12k",
                "prompt": "ping",
                "stream": False,
                "keep_alive": "1440m",
                "options": {"num_predict": 1},
            }
        )

    @pytest.mark.asyncio
    async def test_model_routing_disabled_warms_default_model_name(self):
        """When model_routing_enabled=False, warms config.model_name."""
        config = _make_config(
            warmup_enabled=True,
            warmup_keep_alive_minutes=2880,
            model_routing_enabled=False,
            model_name="gemma4-e4b-q4km",
        )
        driver = Mock(spec=OllamaDriver)
        driver.generate = AsyncMock(return_value={"response": "pong", "done": True})

        service = ModelWarmupService(config=config, driver=driver)
        await service.warmup_models()

        driver.generate.assert_awaited_once_with(
            {
                "model": "gemma4-e4b-q4km",
                "prompt": "ping",
                "stream": False,
                "keep_alive": "2880m",
                "options": {"num_predict": 1},
            }
        )

    @pytest.mark.asyncio
    async def test_driver_error_during_warmup_is_logged_and_does_not_raise(
        self, caplog
    ):
        """Driver exceptions during warmup are logged and swallowed so startup continues."""
        config = _make_config(
            warmup_enabled=True,
            model_routing_enabled=True,
            model_8k_name="gemma4-e4b-12k",
        )
        driver = Mock(spec=OllamaDriver)
        driver.generate = AsyncMock(
            side_effect=RuntimeError("Ollama backend not ready")
        )

        service = ModelWarmupService(config=config, driver=driver)

        with caplog.at_level(logging.WARNING):
            # Must not raise
            await service.warmup_models()

        driver.generate.assert_awaited_once()
        assert any(
            "Failed to warm up model gemma4-e4b-12k" in rec.message
            for rec in caplog.records
        )

    @pytest.mark.asyncio
    async def test_warmup_response_missing_response_field_logs_warning(self, caplog):
        """If driver returns a dict without 'response', logs warning and does not raise."""
        config = _make_config(warmup_enabled=True, model_routing_enabled=True)
        driver = Mock(spec=OllamaDriver)
        driver.generate = AsyncMock(return_value={"done": True})

        service = ModelWarmupService(config=config, driver=driver)

        with caplog.at_level(logging.WARNING):
            await service.warmup_models()

        assert any(
            "warmup response missing 'response' field" in rec.message
            for rec in caplog.records
        )
