"""Tests for main dependency wiring."""

from types import ModuleType, SimpleNamespace

import pytest


class _DummyGateway:
    def __init__(self, config):
        self.config = config
        self.driver = object()

    async def initialize(self) -> None:
        return None

    async def cleanup(self) -> None:
        return None


class _DummyWarmupService:
    def __init__(self, config, driver):
        self.config = config
        self.driver = driver

    async def warmup_models(self) -> None:
        return None


class _DummySummarizeUsecase:
    def __init__(self, config, llm_provider):
        self.config = config
        self.llm_provider = llm_provider


class _DummyRecapUsecase:
    def __init__(self, config, llm_provider, cache=None):
        self.config = config
        self.llm_provider = llm_provider
        self.cache = cache


class _DummyExpandUsecase:
    def __init__(self, config, llm_provider):
        self.config = config
        self.llm_provider = llm_provider


class _DummyRerankUsecase:
    def __init__(self):
        pass

    async def warmup(self) -> None:
        return None


class _DummyDistributedGateway:
    def __init__(
        self,
        local_gateway,
        health_checker,
        remote_driver,
        enabled=True,
        remote_model=None,
        model_overrides=None,
    ):
        self.local_gateway = local_gateway
        self.health_checker = health_checker
        self.remote_driver = remote_driver
        self.enabled = enabled
        self.remote_model = remote_model
        self.model_overrides = model_overrides or {}


class _DummyRemoteDriver:
    def __init__(self, timeout_seconds=None):
        self.timeout_seconds = timeout_seconds


class _DummyRemoteHealthChecker:
    def __init__(self, **kwargs):
        self.kwargs = kwargs


class _DummyConfig(SimpleNamespace):
    def __init__(self):
        super().__init__(
            distributed_be_enabled=True,
            distributed_be_remotes=["http://remote-a:11434"],
            distributed_be_health_interval_seconds=30,
            distributed_be_timeout_seconds=300,
            distributed_be_cooldown_seconds=60,
            distributed_be_remote_model="gemma4-e4b-q4km",
            distributed_be_model_overrides={"http://remote-a:11434": "gemma4-e4b-q4km"},
            cache_enabled=False,
            cache_redis_url="redis://localhost:6379/0",
            cache_ttl_seconds=3600,
        )


class _DummyCacheConfig(SimpleNamespace):
    """Minimal config for cache-wiring tests (no distributed BE noise)."""

    def __init__(self, cache_enabled: bool):
        super().__init__(
            distributed_be_enabled=False,
            distributed_be_remotes=[],
            cache_enabled=cache_enabled,
            cache_redis_url="redis://cache-test:6379/0",
            cache_ttl_seconds=3600,
        )


class _SpyCacheGateway:
    """Records lifecycle calls without touching a real Redis connection."""

    def __init__(self, config=None):
        self.config = config
        self.initialized = False
        self.cleaned_up = False

    async def initialize(self) -> None:
        self.initialized = True

    async def cleanup(self) -> None:
        self.cleaned_up = True

    async def get(self, key):
        return None

    async def set(self, key, value, ttl_seconds=None):
        return True

    async def delete(self, key):
        return False


def test_dependency_container_keeps_summarize_usecase_local(monkeypatch):
    """SummarizeUsecase must keep the local Ollama gateway even when distributed BE is enabled."""
    import sys

    import main as main_module

    monkeypatch.setattr(main_module, "NewsCreatorConfig", _DummyConfig)
    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)
    monkeypatch.setattr(main_module, "SummarizeUsecase", _DummySummarizeUsecase)
    monkeypatch.setattr(main_module, "RecapSummaryUsecase", _DummyRecapUsecase)
    monkeypatch.setattr(main_module, "ExpandQueryUsecase", _DummyExpandUsecase)
    monkeypatch.setattr(main_module, "RerankUsecase", _DummyRerankUsecase)

    distributing_mod = ModuleType("news_creator.gateway.distributing_gateway")
    distributing_mod.DistributingGateway = _DummyDistributedGateway
    remote_ollama_mod = ModuleType("news_creator.gateway.remote_ollama_driver")
    remote_ollama_mod.RemoteOllamaDriver = _DummyRemoteDriver
    remote_health_mod = ModuleType("news_creator.gateway.remote_health_checker")
    remote_health_mod.RemoteHealthChecker = _DummyRemoteHealthChecker
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.distributing_gateway", distributing_mod
    )
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.remote_ollama_driver", remote_ollama_mod
    )
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.remote_health_checker", remote_health_mod
    )

    container = main_module.DependencyContainer()

    assert container.summarize_usecase.llm_provider is container.llm_provider
    assert container.recap_summary_usecase.llm_provider is container.llm_provider
    assert container.expand_query_usecase.llm_provider is container.llm_provider


def _build_cache_wiring_container(monkeypatch, cache_enabled: bool):
    """Build a DependencyContainer with only the heavy Ollama-side classes
    stubbed out, keeping the real RecapSummaryUsecase / cache gateway classes
    so the actual wiring in main.py is exercised (not re-implemented in the
    test double)."""
    import main as main_module

    monkeypatch.setattr(
        main_module, "NewsCreatorConfig", lambda: _DummyCacheConfig(cache_enabled)
    )
    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)

    return main_module.DependencyContainer()


def test_dependency_container_wires_real_cache_when_cache_enabled_true(
    monkeypatch, caplog
):
    """CACHE_ENABLED=true must produce a real RedisCacheGateway wired into
    RecapSummaryUsecase -- not a silently-ignored config value (CLAUDE.md
    rule 8 / di-wiring.md)."""
    from news_creator.gateway.redis_cache_gateway import RedisCacheGateway

    with caplog.at_level("INFO"):
        container = _build_cache_wiring_container(monkeypatch, cache_enabled=True)

    assert isinstance(container.cache_gateway, RedisCacheGateway)
    assert container.recap_summary_usecase.cache is container.cache_gateway
    assert any("cache_enabled" in record.message for record in caplog.records)


def test_dependency_container_wires_null_cache_when_cache_enabled_false(
    monkeypatch, caplog
):
    """CACHE_ENABLED=false must wire an explicit NullCacheGateway (not bare
    None) so 'disabled' is a loud, explicit state rather than an absence."""
    from news_creator.gateway.redis_cache_gateway import NullCacheGateway

    with caplog.at_level("INFO"):
        container = _build_cache_wiring_container(monkeypatch, cache_enabled=False)

    assert isinstance(container.cache_gateway, NullCacheGateway)
    assert container.recap_summary_usecase.cache is container.cache_gateway
    assert any("cache_disabled" in record.message for record in caplog.records)


@pytest.mark.asyncio
async def test_dependency_container_initialize_and_cleanup_manage_cache_lifecycle(
    monkeypatch,
):
    """initialize()/cleanup() must drive the cache gateway's own lifecycle,
    not just construct it and forget it."""
    import main as main_module

    monkeypatch.setattr(
        main_module, "NewsCreatorConfig", lambda: _DummyCacheConfig(cache_enabled=True)
    )
    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)
    monkeypatch.setattr(main_module, "RerankUsecase", _DummyRerankUsecase)
    monkeypatch.setattr(main_module, "RedisCacheGateway", _SpyCacheGateway)

    container = main_module.DependencyContainer()
    spy_cache = container.cache_gateway
    assert isinstance(spy_cache, _SpyCacheGateway)

    await container.initialize()
    assert spy_cache.initialized is True

    await container.cleanup()
    assert spy_cache.cleaned_up is True
