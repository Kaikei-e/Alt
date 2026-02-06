"""Unit tests for ServiceContainer composition root."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from recap_subworker.app.container import ServiceContainer
from recap_subworker.infra.config import Settings


class TestServiceContainerForTesting:
    def test_for_testing_injects_embedder(self):
        settings = Settings(model_id="fake")
        fake_emb = MagicMock()
        container = ServiceContainer.for_testing(settings, embedder=fake_emb)
        assert container.embedder is fake_emb

    def test_for_testing_injects_clusterer(self):
        settings = Settings(model_id="fake")
        fake_clust = MagicMock()
        container = ServiceContainer.for_testing(settings, clusterer=fake_clust)
        assert container.clusterer_gateway is fake_clust

    def test_for_testing_injects_pipeline(self):
        settings = Settings(model_id="fake")
        fake_pipe = MagicMock()
        container = ServiceContainer.for_testing(settings, pipeline=fake_pipe)
        assert container.pipeline is fake_pipe

    def test_for_testing_injects_run_manager(self):
        settings = Settings(model_id="fake")
        fake_rm = MagicMock()
        container = ServiceContainer.for_testing(settings, run_manager=fake_rm)
        assert container.run_manager is fake_rm


class TestServiceContainerShutdown:
    @pytest.mark.asyncio
    async def test_shutdown_when_nothing_initialized(self):
        settings = Settings(model_id="fake")
        container = ServiceContainer(settings)
        await container.shutdown()  # Should not raise

    @pytest.mark.asyncio
    async def test_shutdown_calls_run_manager_shutdown(self):
        settings = Settings(model_id="fake")
        mock_rm = AsyncMock()
        container = ServiceContainer.for_testing(settings, run_manager=mock_rm)
        await container.shutdown()
        mock_rm.shutdown.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_shutdown_handles_run_manager_error(self):
        settings = Settings(model_id="fake")
        mock_rm = AsyncMock()
        mock_rm.shutdown.side_effect = RuntimeError("boom")
        container = ServiceContainer.for_testing(settings, run_manager=mock_rm)
        await container.shutdown()  # Should not raise
