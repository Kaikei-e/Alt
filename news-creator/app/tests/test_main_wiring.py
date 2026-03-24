"""Tests for main dependency wiring."""

from types import ModuleType, SimpleNamespace


class _DummyGateway:
    def __init__(self, config):
        self.config = config
        self.driver = object()


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


class _DummyDistributedGateway:
    def __init__(self, local_gateway, health_checker, remote_driver, enabled=True, remote_model=None, model_overrides=None):
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
            distributed_be_remote_model="gemma3:4b-it-qat",
            distributed_be_model_overrides={"http://remote-a:11434": "gemma3:4b-it-qat"},
        )


def test_dependency_container_keeps_summarize_usecase_local(monkeypatch):
    """SummarizeUsecase must keep the local Ollama gateway even when distributed BE is enabled."""
    import sys

    monkeypatch.setenv("SERVICE_SECRET", "test-secret")
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
    monkeypatch.setitem(sys.modules, "news_creator.gateway.distributing_gateway", distributing_mod)
    monkeypatch.setitem(sys.modules, "news_creator.gateway.remote_ollama_driver", remote_ollama_mod)
    monkeypatch.setitem(sys.modules, "news_creator.gateway.remote_health_checker", remote_health_mod)

    container = main_module.DependencyContainer()

    assert container.summarize_usecase.llm_provider is container.llm_provider
    assert container.recap_summary_usecase.llm_provider is container.llm_provider
    assert container.expand_query_usecase.llm_provider is container.llm_provider
