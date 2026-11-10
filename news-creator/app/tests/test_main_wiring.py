"""Tests for main dependency wiring."""

import asyncio
from types import ModuleType, SimpleNamespace

import pytest


class _DummyGateway:
    def __init__(self, config):
        self.config = config
        self.driver = object()
        self.initialize_count = 0
        self.cleanup_count = 0

    async def initialize(self) -> None:
        self.initialize_count += 1

    async def cleanup(self) -> None:
        self.cleanup_count += 1


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


class _DummyRecapCardUsecase:
    def __init__(self, config, llm_provider, cache=None):
        self.config = config
        self.llm_provider = llm_provider
        self.cache = cache


class _DummyExpandUsecase:
    def __init__(self, config, llm_provider):
        self.config = config
        self.llm_provider = llm_provider


class _DummyPlanQueryUsecase:
    def __init__(self, config, llm_provider):
        self.config = config
        self.llm_provider = llm_provider


class _DummyMorningLetterUsecase:
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
    ):
        self.local_gateway = local_gateway
        self.health_checker = health_checker
        self.remote_driver = remote_driver
        self.enabled = enabled

    async def initialize(self) -> None:
        await self.local_gateway.initialize()
        if self.enabled:
            await self.remote_driver.initialize()
            await self.health_checker.start()

    async def cleanup(self) -> None:
        await self.local_gateway.cleanup()
        if self.enabled:
            await self.health_checker.stop()
            await self.remote_driver.cleanup()


class _DummyRemoteChatDriver:
    def __init__(self, timeout_seconds: int, reasoning_effort: str | None = None):
        self.timeout_seconds = timeout_seconds
        self.reasoning_effort = reasoning_effort
        self.initialize_count = 0
        self.cleanup_count = 0

    async def initialize(self) -> None:
        self.initialize_count += 1

    async def cleanup(self) -> None:
        self.cleanup_count += 1


class _DummyRemoteHealthChecker:
    def __init__(
        self,
        targets,
        interval_seconds: int = 30,
        cooldown_seconds: int = 60,
        timeout_seconds: int = 10,
        unhealthy_threshold: int = 2,
    ):
        self.targets = targets
        self.interval_seconds = interval_seconds
        self.cooldown_seconds = cooldown_seconds
        self.timeout_seconds = timeout_seconds
        self.unhealthy_threshold = unhealthy_threshold
        self.start_count = 0
        self.stop_count = 0

    async def start(self) -> None:
        self.start_count += 1

    async def stop(self) -> None:
        self.stop_count += 1


@pytest.fixture(autouse=True)
def _setup_main_wiring_env(tmp_path, monkeypatch):
    """Isolate telemetry and ensure Redis authentication fail-fast contract is satisfied."""
    secret_file = tmp_path / "redis_secret.txt"
    secret_file.write_text("test-wiring-redis-password")
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(secret_file))
    monkeypatch.setenv("OTEL_ENABLED", "false")
    import main  # noqa: F401


class _DummyCacheConfig(SimpleNamespace):
    """Minimal config for cache-wiring tests (no distributed BE noise)."""

    def __init__(
        self,
        cache_enabled: bool,
        cache_redis_password: str | None = "test-wiring-redis-password",
    ):
        super().__init__(
            distributed_be_enabled=False,
            distributed_be_targets=[],
            distributed_be_reasoning_effort=None,
            cache_enabled=cache_enabled,
            cache_redis_url="redis://cache-test:6379/0",
            cache_redis_password=cache_redis_password,
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


def test_dependency_container_routes_only_summarize_usecase_through_distributing_gateway(
    monkeypatch, caplog
):
    """When enabled, only SummarizeUsecase receives DistributingGateway; all other usecases receive local ollama_gateway."""
    import sys

    import main as main_module
    from news_creator.config.distributed_be_config import RemoteTarget

    targets = [
        RemoteTarget(
            base_url="http://router.example:8080",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        )
    ]

    class _EnabledConfig(SimpleNamespace):
        def __init__(self):
            super().__init__(
                distributed_be_enabled=True,
                distributed_be_targets=targets,
                distributed_be_reasoning_effort="low",
                distributed_be_health_interval_seconds=15,
                distributed_be_timeout_seconds=120,
                distributed_be_cooldown_seconds=45,
                distributed_be_unhealthy_threshold=2,
                cache_enabled=False,
                cache_redis_url="redis://localhost:6379/0",
                cache_redis_password=None,
                cache_ttl_seconds=3600,
            )

    monkeypatch.setattr(main_module, "NewsCreatorConfig", _EnabledConfig)
    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)
    monkeypatch.setattr(main_module, "SummarizeUsecase", _DummySummarizeUsecase)
    monkeypatch.setattr(main_module, "RecapSummaryUsecase", _DummyRecapUsecase)
    monkeypatch.setattr(main_module, "RecapCardUsecase", _DummyRecapCardUsecase)
    monkeypatch.setattr(main_module, "ExpandQueryUsecase", _DummyExpandUsecase)
    monkeypatch.setattr(main_module, "PlanQueryUsecase", _DummyPlanQueryUsecase)
    monkeypatch.setattr(main_module, "MorningLetterUsecase", _DummyMorningLetterUsecase)
    monkeypatch.setattr(main_module, "RerankUsecase", _DummyRerankUsecase)

    distributing_mod = ModuleType("news_creator.gateway.distributing_gateway")
    distributing_mod.DistributingGateway = _DummyDistributedGateway
    remote_chat_mod = ModuleType("news_creator.gateway.remote_chat_driver")
    remote_chat_mod.RemoteChatDriver = _DummyRemoteChatDriver
    remote_health_mod = ModuleType("news_creator.gateway.remote_health_checker")
    remote_health_mod.RemoteHealthChecker = _DummyRemoteHealthChecker
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.distributing_gateway", distributing_mod
    )
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.remote_chat_driver", remote_chat_mod
    )
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.remote_health_checker", remote_health_mod
    )

    with caplog.at_level("INFO"):
        container = main_module.DependencyContainer()

    dist_gw = container.summarize_usecase.llm_provider
    assert isinstance(dist_gw, _DummyDistributedGateway)
    assert dist_gw.local_gateway is container.ollama_gateway
    assert dist_gw.remote_driver.timeout_seconds == 120
    assert dist_gw.remote_driver.reasoning_effort == "low"
    assert dist_gw.health_checker.targets == targets
    assert dist_gw.health_checker.interval_seconds == 15
    assert dist_gw.health_checker.cooldown_seconds == 45
    assert dist_gw.health_checker.timeout_seconds == 15

    # All other LLM usecases must receive self.ollama_gateway directly
    assert container.recap_summary_usecase.llm_provider is container.ollama_gateway
    assert container.recap_card_usecase.llm_provider is container.ollama_gateway
    assert container.expand_query_usecase.llm_provider is container.ollama_gateway
    assert container.plan_query_usecase.llm_provider is container.ollama_gateway
    assert container.morning_letter_usecase.llm_provider is container.ollama_gateway

    enabled_logs = [
        r for r in caplog.records if "distributed_be_enabled" in r.getMessage()
    ]
    disabled_logs = [
        r for r in caplog.records if "distributed_be_disabled" in r.getMessage()
    ]
    assert len(enabled_logs) == 1
    assert len(disabled_logs) == 0


def test_dependency_container_logs_distributed_be_disabled_when_off(
    monkeypatch, caplog
):
    """When disabled, startup logs distributed_be_disabled and wires ollama_gateway everywhere."""
    import main as main_module

    monkeypatch.setattr(
        main_module,
        "NewsCreatorConfig",
        lambda: _DummyCacheConfig(cache_enabled=False),
    )
    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)

    with caplog.at_level("INFO"):
        container = main_module.DependencyContainer()

    assert container.summarize_usecase.llm_provider is container.ollama_gateway
    assert container.recap_summary_usecase.llm_provider is container.ollama_gateway
    assert container.recap_card_usecase.llm_provider is container.ollama_gateway
    assert container.expand_query_usecase.llm_provider is container.ollama_gateway
    assert container.plan_query_usecase.llm_provider is container.ollama_gateway
    assert container.morning_letter_usecase.llm_provider is container.ollama_gateway

    enabled_logs = [
        r for r in caplog.records if "distributed_be_enabled" in r.getMessage()
    ]
    disabled_logs = [
        r for r in caplog.records if "distributed_be_disabled" in r.getMessage()
    ]
    assert len(enabled_logs) == 0
    assert len(disabled_logs) == 1


@pytest.mark.asyncio
async def test_dependency_container_initializes_and_cleans_up_local_gateway_exactly_once(
    monkeypatch,
):
    """When distributed BE is enabled, local ollama_gateway is initialized and cleaned up exactly once."""
    import sys

    import main as main_module
    from news_creator.config.distributed_be_config import RemoteTarget

    targets = [
        RemoteTarget(
            base_url="http://router.example:8080",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        )
    ]

    class _EnabledConfig(SimpleNamespace):
        def __init__(self):
            super().__init__(
                distributed_be_enabled=True,
                distributed_be_targets=targets,
                distributed_be_reasoning_effort=None,
                distributed_be_health_interval_seconds=30,
                distributed_be_timeout_seconds=120,
                distributed_be_cooldown_seconds=60,
                distributed_be_unhealthy_threshold=2,
                cache_enabled=False,
                cache_redis_url="redis://localhost:6379/0",
                cache_redis_password=None,
                cache_ttl_seconds=3600,
            )

    monkeypatch.setattr(main_module, "NewsCreatorConfig", _EnabledConfig)
    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)
    monkeypatch.setattr(main_module, "RerankUsecase", _DummyRerankUsecase)

    distributing_mod = ModuleType("news_creator.gateway.distributing_gateway")
    distributing_mod.DistributingGateway = _DummyDistributedGateway
    remote_chat_mod = ModuleType("news_creator.gateway.remote_chat_driver")
    remote_chat_mod.RemoteChatDriver = _DummyRemoteChatDriver
    remote_health_mod = ModuleType("news_creator.gateway.remote_health_checker")
    remote_health_mod.RemoteHealthChecker = _DummyRemoteHealthChecker
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.distributing_gateway", distributing_mod
    )
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.remote_chat_driver", remote_chat_mod
    )
    monkeypatch.setitem(
        sys.modules, "news_creator.gateway.remote_health_checker", remote_health_mod
    )

    container = main_module.DependencyContainer()
    await container.initialize()
    assert container.ollama_gateway.initialize_count == 1

    await container.cleanup()
    assert container.ollama_gateway.cleanup_count == 1


@pytest.mark.asyncio
async def test_dependency_container_real_distributing_gateway_lifecycle_initializes_and_cleans_up_local_once(
    monkeypatch,
):
    """T1: With DISTRIBUTED_BE_ENABLED=true and the real DistributingGateway, local gateway initialize/cleanup each run once."""
    from unittest.mock import AsyncMock, MagicMock

    import main as main_module
    from news_creator.gateway.distributing_gateway import DistributingGateway
    from news_creator.gateway.remote_chat_driver import RemoteChatDriver
    from news_creator.gateway.remote_health_checker import RemoteHealthChecker

    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv(
        "DISTRIBUTED_BE_TARGETS",
        "http://router.example:8080|provider-a/model-a:latest|32768",
    )
    monkeypatch.setenv("INFERENCE_AUTH", "disabled")
    monkeypatch.setenv("CACHE_ENABLED", "false")

    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)
    monkeypatch.setattr(main_module, "RerankUsecase", _DummyRerankUsecase)

    def _fake_session(*_args, **_kwargs):
        sess = MagicMock()
        sess.closed = False
        sess.headers = {}
        sess.close = AsyncMock()
        return sess

    monkeypatch.setattr(
        "news_creator.gateway.remote_chat_driver.aiohttp.ClientSession",
        _fake_session,
    )
    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.aiohttp.ClientSession",
        _fake_session,
    )
    monkeypatch.setattr(
        RemoteHealthChecker,
        "_check_all",
        AsyncMock(),
    )

    container = main_module.DependencyContainer()
    assert isinstance(container.llm_provider, DistributingGateway)
    assert isinstance(container.llm_provider._remote_driver, RemoteChatDriver)
    assert isinstance(container.llm_provider._health_checker, RemoteHealthChecker)

    await container.initialize()
    assert container.ollama_gateway.initialize_count == 1

    await container.cleanup()
    assert container.ollama_gateway.cleanup_count == 1


def test_dependency_container_fails_fast_when_distributed_be_enabled_with_empty_targets(
    monkeypatch,
):
    """DISTRIBUTED_BE_ENABLED=true with empty DISTRIBUTED_BE_TARGETS fails container construction."""
    import main as main_module

    monkeypatch.setenv("DISTRIBUTED_BE_ENABLED", "true")
    monkeypatch.setenv("DISTRIBUTED_BE_TARGETS", "")

    with pytest.raises(ValueError):
        main_module.DependencyContainer()


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
    rule 8 / di-wiring.md), and must not log cache_enabled before connecting."""
    from news_creator.gateway.redis_cache_gateway import RedisCacheGateway

    with caplog.at_level("INFO"):
        container = _build_cache_wiring_container(monkeypatch, cache_enabled=True)

    assert isinstance(container.cache_gateway, RedisCacheGateway)
    assert container.recap_summary_usecase.cache is container.cache_gateway
    assert container.recap_card_usecase.cache is container.cache_gateway
    assert not any("cache_enabled" in record.message for record in caplog.records)


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
    assert container.recap_card_usecase.cache is container.cache_gateway
    assert any("cache_disabled" in record.message for record in caplog.records)


@pytest.mark.asyncio
async def test_dependency_container_initialize_and_cleanup_manage_cache_lifecycle(
    monkeypatch, caplog
):
    """initialize()/cleanup() must drive the cache gateway's own lifecycle,
    and log cache_enabled only after initialize() succeeds."""
    import main as main_module

    monkeypatch.setattr(
        main_module, "NewsCreatorConfig", lambda: _DummyCacheConfig(cache_enabled=True)
    )
    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)
    monkeypatch.setattr(main_module, "RerankUsecase", _DummyRerankUsecase)
    monkeypatch.setattr(main_module, "RedisCacheGateway", _SpyCacheGateway)

    with caplog.at_level("INFO"):
        container = main_module.DependencyContainer()
        assert not any("cache_enabled" in record.message for record in caplog.records)

        spy_cache = container.cache_gateway
        assert isinstance(spy_cache, _SpyCacheGateway)

        await container.initialize()
        assert spy_cache.initialized is True
        assert any("cache_enabled" in record.message for record in caplog.records)

    await container.cleanup()
    assert spy_cache.cleaned_up is True


@pytest.mark.asyncio
async def test_dependency_container_does_not_log_cache_enabled_when_connect_fails(
    monkeypatch, caplog
):
    """When cache_gateway.initialize() fails, container.initialize() must raise
    and never emit cache_enabled."""
    import main as main_module

    class _FailingCacheGateway(_SpyCacheGateway):
        async def initialize(self) -> None:
            raise ConnectionError("Connection refused")

    monkeypatch.setattr(
        main_module, "NewsCreatorConfig", lambda: _DummyCacheConfig(cache_enabled=True)
    )
    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)
    monkeypatch.setattr(main_module, "RerankUsecase", _DummyRerankUsecase)
    monkeypatch.setattr(main_module, "RedisCacheGateway", _FailingCacheGateway)

    with caplog.at_level("INFO"):
        container = main_module.DependencyContainer()
        with pytest.raises(ConnectionError, match="Connection refused"):
            await container.initialize()

    assert not any("cache_enabled" in record.message for record in caplog.records)


@pytest.mark.asyncio
async def test_lifespan_starts_and_cancels_event_loop_lag_probe(monkeypatch):
    """lifespan() must start the event-loop-lag probe as a background task
    at startup and cancel it cleanly at shutdown -- not leave it dangling,
    and not simply forget to start it."""
    import main as main_module

    class _DummyContainer:
        def __init__(self):
            self.config = SimpleNamespace(event_loop_lag_warn_ms=500)

        async def initialize(self) -> None:
            return None

        async def cleanup(self) -> None:
            return None

    monkeypatch.setattr(main_module, "container", _DummyContainer())
    monkeypatch.setattr(main_module, "otel_shutdown", lambda: None)

    probe_started = asyncio.Event()
    probe_cancelled = asyncio.Event()
    received_threshold = None

    async def fake_probe(*, warn_threshold_ms):
        nonlocal received_threshold
        received_threshold = warn_threshold_ms
        probe_started.set()
        try:
            await asyncio.sleep(100)
        except asyncio.CancelledError:
            probe_cancelled.set()
            raise

    monkeypatch.setattr(main_module, "run_event_loop_lag_probe", fake_probe)

    async with main_module.lifespan(main_module.app):
        await asyncio.wait_for(probe_started.wait(), timeout=1.0)

    assert received_threshold == 500
    assert probe_cancelled.is_set(), (
        "lifespan must cancel the lag probe task on shutdown, not leave it dangling"
    )


def test_dependency_container_fails_fast_when_redis_auth_unconfigured(monkeypatch):
    """DependencyContainer startup must fail fast if Redis auth is unconfigured."""
    import main as main_module

    monkeypatch.delenv("REDIS_PASSWORD_FILE", raising=False)
    monkeypatch.delenv("REDIS_AUTH", raising=False)

    with pytest.raises(RuntimeError, match="redis authentication requires"):
        main_module.DependencyContainer()


def test_dependency_container_fails_fast_when_inference_auth_unconfigured(monkeypatch):
    """Startup must fail when the generation-proxy bearer is neither set nor disabled."""
    import main as main_module

    monkeypatch.delenv("INFERENCE_SERVICE_TOKEN_FILE", raising=False)
    monkeypatch.delenv("INFERENCE_AUTH", raising=False)

    with pytest.raises(ValueError, match="INFERENCE_SERVICE_TOKEN_FILE"):
        main_module.DependencyContainer()


@pytest.mark.asyncio
async def test_dependency_container_drivers_send_resolved_inference_token(
    monkeypatch, tmp_path
):
    """The container's real Ollama drivers carry the bearer read from the token file."""
    import main as main_module

    token_file = tmp_path / "inference.token"
    token_file.write_text("container-wired-token\n", encoding="utf-8")
    monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(token_file))
    monkeypatch.delenv("INFERENCE_AUTH", raising=False)

    container = main_module.DependencyContainer()
    gateway = container.ollama_gateway
    await gateway.initialize()
    try:
        expected = "Bearer container-wired-token"
        assert gateway.driver.session.headers["Authorization"] == expected
        assert gateway.stream_driver.session.headers["Authorization"] == expected
    finally:
        await gateway.cleanup()


def test_dependency_container_wires_resolved_redis_password(monkeypatch, tmp_path):
    """DependencyContainer must pass resolved Redis password to CachePort when CACHE_ENABLED=true."""
    import main as main_module
    from news_creator.gateway.redis_cache_gateway import RedisCacheGateway

    secret_file = tmp_path / "secret.txt"
    secret_file.write_text("custom-auth-password")
    monkeypatch.setenv("REDIS_PASSWORD_FILE", str(secret_file))
    monkeypatch.setenv("CACHE_ENABLED", "true")
    monkeypatch.delenv("REDIS_AUTH", raising=False)

    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)

    container = main_module.DependencyContainer()

    assert container.config.cache_redis_password == "custom-auth-password"
    assert isinstance(container.cache_gateway, RedisCacheGateway)
    assert container.cache_gateway.config.cache_redis_password == "custom-auth-password"


def test_dependency_container_accepts_explicit_redis_auth_disabled(monkeypatch):
    """DependencyContainer allows startup with None password when REDIS_AUTH=disabled."""
    import main as main_module
    from news_creator.gateway.redis_cache_gateway import NullCacheGateway

    monkeypatch.delenv("REDIS_PASSWORD_FILE", raising=False)
    monkeypatch.setenv("REDIS_AUTH", "disabled")
    monkeypatch.setenv("CACHE_ENABLED", "false")

    monkeypatch.setattr(main_module, "OllamaGateway", _DummyGateway)
    monkeypatch.setattr(main_module, "ModelWarmupService", _DummyWarmupService)

    container = main_module.DependencyContainer()

    assert container.config.cache_redis_password is None
    assert isinstance(container.cache_gateway, NullCacheGateway)


def test_metrics_endpoint_returns_200_without_redirect():
    """GET /metrics (without trailing slash) must return 200 directly without a 307 redirect."""
    from fastapi.testclient import TestClient
    import main as main_module

    client = TestClient(main_module.app)
    response = client.get("/metrics", follow_redirects=False)

    assert response.status_code == 200
    assert "text/plain" in response.headers.get("content-type", "")


def test_instrument_fastapi_excludes_health_metrics_and_asgi_send_receive_spans(
    monkeypatch,
):
    """instrument_fastapi must exclude health/metrics/api/tags URLs and ASGI receive/send spans."""
    from unittest.mock import MagicMock
    from news_creator import otel as otel_module

    monkeypatch.setenv("OTEL_ENABLED", "true")
    mock_instrument = MagicMock()
    monkeypatch.setattr(
        otel_module.FastAPIInstrumentor, "instrument_app", mock_instrument
    )

    dummy_app = object()
    otel_module.instrument_fastapi(dummy_app)

    mock_instrument.assert_called_once_with(
        dummy_app,
        excluded_urls="health,metrics,api/tags",
        exclude_spans=["receive", "send"],
    )
