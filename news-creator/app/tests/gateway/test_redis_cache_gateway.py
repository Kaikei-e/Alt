"""Tests for RedisCacheGateway, including an end-to-end proof that
CACHE_ENABLED=true actually results in cache reads/writes -- not just a
config value that main.py forgets to wire (HIGH finding, 2026-07-06 review).
"""

from unittest.mock import Mock
from uuid import uuid4

import pytest

from news_creator.domain.models import (
    LLMGenerateResponse,
    RecapClusterInput,
    RecapSummaryRequest,
    RepresentativeSentence,
)
from news_creator.gateway.redis_cache_gateway import NullCacheGateway, RedisCacheGateway
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase


class _FakeRedisClient:
    """In-memory stand-in for redis.asyncio.Redis -- only the boundary to
    the real `redis` package is faked; RedisCacheGateway's own get/set/init
    logic all runs for real against this fake."""

    def __init__(self):
        self.store: dict[str, str] = {}
        self.get_calls: list[str] = []
        self.set_calls: list[tuple[str, str, int | None]] = []

    async def ping(self) -> bool:
        return True

    async def get(self, key: str):
        self.get_calls.append(key)
        return self.store.get(key)

    async def set(self, key: str, value: str, ex=None) -> bool:
        self.set_calls.append((key, value, ex))
        self.store[key] = value
        return True

    async def close(self) -> None:
        return None


def _recap_config():
    config = Mock()
    config.cache_enabled = True
    config.cache_redis_url = "redis://cache-test:6379/0"
    config.cache_redis_password = None
    config.cache_ttl_seconds = 3600
    # RecapSummaryUsecase generation-path config knobs (mirrors
    # tests/usecase/test_recap_summary_usecase.py fixtures).
    config.summary_num_predict = 400
    config.recap_summary_num_predict = 400
    config.recap_min_avg_bullet_length = 0
    config.llm_temperature = 0.25
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000
    return config


def _recap_request() -> RecapSummaryRequest:
    # Two clusters / three representative sentences -- enough evidence to
    # clear RecapSummaryUsecase._should_bypass_llm's default thresholds, so
    # generate_summary() actually reaches the LLM (and cache) path instead of
    # taking the low-evidence shortcut.
    return RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="TechFusion announced the acquisition of Nova Labs for $1.2B."
                    ),
                    RepresentativeSentence(
                        text="Executives expect integration in March 2026."
                    ),
                ],
                top_terms=["acquisition", "AI", "Nova Labs"],
            ),
            RecapClusterInput(
                cluster_id=1,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Nova Labs is known for fast fine-tuning infrastructure."
                    ),
                ],
            ),
        ],
    )


def _llm_response() -> LLMGenerateResponse:
    return LLMGenerateResponse(
        response="""
        {
          "title": "AI業界の大型買収",
          "bullets": [
            "米TechFusion社は2025年11月7日、AIスタートアップNova Labsを買収したと発表した。",
            "統合完了は2026年3月を予定している。",
            "規制当局の承認は未取得だが世界シェア拡大を狙う。"
          ],
          "language": "ja"
        }
        """,
        model="gemma4-e4b-q4km",
        prompt_eval_count=512,
        eval_count=256,
        total_duration=1_750_000_000,
    )


@pytest.mark.asyncio
async def test_cache_enabled_results_in_real_reads_and_writes(monkeypatch):
    """With CACHE_ENABLED=true and a wired RedisCacheGateway, generating the
    same recap summary twice must (1) actually call redis GET/SET, and
    (2) serve the second call from cache instead of calling the LLM again.
    """
    fake_client = _FakeRedisClient()
    monkeypatch.setattr(
        "news_creator.gateway.redis_cache_gateway.redis.Redis.from_url",
        lambda *args, **kwargs: fake_client,
    )

    config = _recap_config()
    cache_gateway = RedisCacheGateway(config)
    await cache_gateway.initialize()
    assert cache_gateway._enabled is True, (
        "cache silently disabled itself in initialize()"
    )

    llm_provider = Mock()
    call_count = {"n": 0}

    async def counting_generate(*args, **kwargs):
        call_count["n"] += 1
        return _llm_response()

    llm_provider.generate = counting_generate

    usecase = RecapSummaryUsecase(
        config=config, llm_provider=llm_provider, cache=cache_gateway
    )
    request = _recap_request()

    first = await usecase.generate_summary(request)

    assert fake_client.get_calls, (
        "RedisCacheGateway.get() was never called against redis"
    )
    assert fake_client.set_calls, (
        "RedisCacheGateway.set() was never called against redis"
    )
    assert call_count["n"] == 1

    second = await usecase.generate_summary(request)

    # Cache hit: LLM must NOT be invoked again, and the response must come
    # from the cached value written on the first call.
    assert call_count["n"] == 1, "second call bypassed the cache and hit the LLM again"
    assert second.summary.title == first.summary.title
    assert second.summary.bullets == first.summary.bullets

    await cache_gateway.cleanup()


@pytest.mark.asyncio
async def test_cache_initialize_forwards_resolved_password(monkeypatch):
    """The password resolved from REDIS_PASSWORD_FILE must reach
    redis.Redis.from_url, not just the URL -- otherwise the client silently
    connects unauthenticated."""
    fake_client = _FakeRedisClient()
    captured: dict[str, object] = {}

    def fake_from_url(url, **kwargs):
        captured["url"] = url
        captured.update(kwargs)
        return fake_client

    monkeypatch.setattr(
        "news_creator.gateway.redis_cache_gateway.redis.Redis.from_url",
        fake_from_url,
    )

    config = _recap_config()
    config.cache_redis_password = "hunter2"
    cache_gateway = RedisCacheGateway(config)
    await cache_gateway.initialize()

    assert captured["password"] == "hunter2"
    await cache_gateway.cleanup()


@pytest.mark.asyncio
async def test_cache_authentication_failure_is_a_loud_startup_error(monkeypatch):
    """NOAUTH/WRONGPASS must not be swallowed into "cache will be disabled" --
    that would hide a wrong or missing REDIS_PASSWORD_FILE behind a warning
    log instead of the fail-fast startup error CLAUDE.md rule 9 requires."""
    from redis.exceptions import AuthenticationError

    class _AuthFailingClient:
        async def ping(self):
            raise AuthenticationError("WRONGPASS invalid username-password pair")

        async def close(self):
            return None

    monkeypatch.setattr(
        "news_creator.gateway.redis_cache_gateway.redis.Redis.from_url",
        lambda *args, **kwargs: _AuthFailingClient(),
    )

    config = _recap_config()
    cache_gateway = RedisCacheGateway(config)

    with pytest.raises(AuthenticationError):
        await cache_gateway.initialize()


@pytest.mark.asyncio
async def test_cache_unreachable_connection_still_soft_disables(monkeypatch):
    """A genuinely unreachable cache (not an auth failure) keeps the existing
    soft-disable behaviour -- news-creator must not refuse to start just
    because Redis has not come up yet."""

    class _UnreachableClient:
        async def ping(self):
            raise ConnectionError("Connection refused")

        async def close(self):
            return None

    monkeypatch.setattr(
        "news_creator.gateway.redis_cache_gateway.redis.Redis.from_url",
        lambda *args, **kwargs: _UnreachableClient(),
    )

    config = _recap_config()
    cache_gateway = RedisCacheGateway(config)
    await cache_gateway.initialize()

    assert cache_gateway._enabled is False
    assert cache_gateway._client is None


@pytest.mark.asyncio
async def test_cache_disabled_never_touches_redis(monkeypatch):
    """NullCacheGateway must be a real no-op: no redis calls, cache never
    short-circuits the LLM call."""
    fake_client = _FakeRedisClient()
    monkeypatch.setattr(
        "news_creator.gateway.redis_cache_gateway.redis.Redis.from_url",
        lambda *args, **kwargs: fake_client,
    )

    config = _recap_config()
    cache_gateway = NullCacheGateway()

    llm_provider = Mock()
    call_count = {"n": 0}

    async def counting_generate(*args, **kwargs):
        call_count["n"] += 1
        return _llm_response()

    llm_provider.generate = counting_generate

    usecase = RecapSummaryUsecase(
        config=config, llm_provider=llm_provider, cache=cache_gateway
    )
    request = _recap_request()

    await usecase.generate_summary(request)
    await usecase.generate_summary(request)

    assert call_count["n"] == 2, "NullCacheGateway unexpectedly caused a cache hit"
    assert not fake_client.get_calls
    assert not fake_client.set_calls
