"""Tests for OllamaGateway."""

import asyncio
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from recap_evaluator.config import Settings
from recap_evaluator.gateway.ollama_gateway import (
    BatchGEvalResult,
    GEvalResult,
    OllamaGateway,
)


@pytest.fixture
def mock_settings() -> Settings:
    return Settings(
        recap_db_dsn="postgres://test:test@localhost/test",
        ollama_url="http://localhost:11434",
        ollama_model="test-model",
        ollama_timeout=10,
        ollama_concurrency=2,
    )


@pytest.fixture
def mock_client():
    return AsyncMock(spec=httpx.AsyncClient)


@pytest.fixture
def gateway(mock_client, mock_settings):
    return OllamaGateway(mock_client, mock_settings)


class TestGEvalResult:
    def test_average_score(self):
        result = GEvalResult(coherence=4.0, consistency=3.0, fluency=5.0, relevance=4.0)
        assert result.average_score == 4.0

    def test_error_result(self):
        result = GEvalResult(
            coherence=0, consistency=0, fluency=0, relevance=0, error="timeout"
        )
        assert result.error == "timeout"
        assert result.average_score == 0.0


class TestBatchGEvalResult:
    def test_empty_batch(self):
        batch = BatchGEvalResult()
        assert batch.count == 0
        assert batch.success_count == 0
        assert batch.avg_overall == 0.0

    def test_aggregates_results(self):
        batch = BatchGEvalResult(
            results=[
                GEvalResult(coherence=4.0, consistency=4.0, fluency=4.0, relevance=4.0),
                GEvalResult(coherence=3.0, consistency=3.0, fluency=3.0, relevance=3.0),
            ]
        )
        assert batch.count == 2
        assert batch.success_count == 2
        assert batch.avg_coherence == 3.5

    def test_ignores_errors_in_averages(self):
        batch = BatchGEvalResult(
            results=[
                GEvalResult(coherence=4.0, consistency=4.0, fluency=4.0, relevance=4.0),
                GEvalResult(
                    coherence=0, consistency=0, fluency=0, relevance=0, error="fail"
                ),
            ]
        )
        assert batch.success_count == 1
        assert batch.avg_coherence == 4.0


class TestOllamaGateway:
    def test_parse_valid_json(self, gateway):
        text = '{"coherence": 4, "consistency": 3, "fluency": 5, "relevance": 4}'
        result = gateway._parse_geval_response(text)
        assert result.coherence == 4.0
        assert result.fluency == 5.0
        assert result.error is None

    def test_parse_json_with_surrounding_text(self, gateway):
        text = 'Here is my evaluation: {"coherence": 4, "consistency": 3, "fluency": 5, "relevance": 4} Done.'
        result = gateway._parse_geval_response(text)
        assert result.coherence == 4.0

    def test_parse_no_json(self, gateway):
        result = gateway._parse_geval_response("No JSON here")
        assert result.error == "No JSON found in response"
        assert result.coherence == 0

    def test_parse_invalid_json(self, gateway):
        result = gateway._parse_geval_response("{invalid json}")
        assert result.error is not None

    async def test_evaluate_summary_success(self, gateway, mock_client):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "response": '{"coherence": 4, "consistency": 4, "fluency": 5, "relevance": 4}'
        }
        mock_response.raise_for_status = MagicMock()
        mock_client.post.return_value = mock_response

        result = await gateway.evaluate_summary("source text", "summary text")

        assert result.coherence == 4.0
        assert result.error is None

    async def test_evaluate_summary_timeout(self, gateway, mock_client):
        mock_client.post.side_effect = httpx.TimeoutException("timeout")

        result = await gateway.evaluate_summary("source", "summary")

        assert result.error == "Request timeout"
        assert result.coherence == 0

    async def test_evaluate_batch_concurrent(self, gateway, mock_client):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "response": '{"coherence": 4, "consistency": 4, "fluency": 4, "relevance": 4}'
        }
        mock_response.raise_for_status = MagicMock()
        mock_client.post.return_value = mock_response

        items = [("src1", "sum1"), ("src2", "sum2"), ("src3", "sum3")]
        result = await gateway.evaluate_batch(items)

        assert result.count == 3
        assert result.success_count == 3
        assert mock_client.post.call_count == 3

    async def test_evaluate_batch_concurrency_is_bounded_by_semaphore(
        self, gateway, mock_client
    ):
        """mock_settings pins ollama_concurrency=2. With 4 items in flight,
        no more than 2 requests may be in-flight at once — a regression that
        drops or widens the semaphore would let all 4 hit Ollama at once and
        overwhelm the single-model server (ADR context: concurrency exists
        specifically to bound Ollama load).

        Uses asyncio.Event handshakes (no wall-clock sleeps) so the
        assertion is deterministic instead of timing-dependent.
        """
        concurrency_limit = 2
        active = 0
        max_active = 0
        entered_count = 0
        all_slots_full = asyncio.Event()
        release = asyncio.Event()

        async def fake_post(*args, **kwargs):
            nonlocal active, max_active, entered_count
            active += 1
            entered_count += 1
            max_active = max(max_active, active)
            if entered_count == concurrency_limit:
                all_slots_full.set()
            await release.wait()
            active -= 1
            response = MagicMock()
            response.json.return_value = {
                "response": (
                    '{"coherence": 4, "consistency": 4, '
                    '"fluency": 4, "relevance": 4}'
                )
            }
            response.raise_for_status = MagicMock()
            return response

        mock_client.post.side_effect = fake_post
        items = [(f"src{i}", f"sum{i}") for i in range(4)]

        batch_task = asyncio.create_task(gateway.evaluate_batch(items))

        await all_slots_full.wait()
        # Exactly `concurrency_limit` requests got in — the other two are
        # still blocked on the semaphore, not on the mocked HTTP call.
        assert active == concurrency_limit
        assert entered_count == concurrency_limit

        release.set()
        result = await batch_task

        assert max_active == concurrency_limit
        assert result.count == 4
        assert result.success_count == 4

    async def test_health_check_success(self, gateway, mock_client):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "models": [{"name": "test-model:latest"}]
        }
        mock_response.raise_for_status = MagicMock()
        mock_client.get.return_value = mock_response

        assert await gateway.health_check() is True

    async def test_health_check_failure(self, gateway, mock_client):
        mock_client.get.side_effect = httpx.ConnectError("connection refused")

        assert await gateway.health_check() is False


class TestScoreReadability:
    """OllamaGateway.score_readability — used by ReadabilityEvaluator.

    Previously this method did not exist at all, so every call raised
    AttributeError, silently caught and defaulted to 0.0 (CLAUDE.md rule 8).
    """

    async def test_score_readability_returns_real_score(self, gateway, mock_client):
        mock_response = MagicMock()
        mock_response.json.return_value = {"response": '{"score": 4.5}'}
        mock_response.raise_for_status = MagicMock()
        mock_client.post.return_value = mock_response

        score = await gateway.score_readability("本日の主要な変化: AI導入で業務自動化。")

        assert score == pytest.approx(4.5)
        mock_client.post.assert_called_once()

    async def test_score_readability_raises_on_unparseable_response(
        self, gateway, mock_client
    ):
        mock_response = MagicMock()
        mock_response.json.return_value = {"response": "not json at all"}
        mock_response.raise_for_status = MagicMock()
        mock_client.post.return_value = mock_response

        with pytest.raises(ValueError, match="No JSON found"):
            await gateway.score_readability("要約")

    async def test_score_readability_propagates_http_errors(
        self, gateway, mock_client
    ):
        mock_client.post.side_effect = httpx.TimeoutException("timeout")

        with pytest.raises(httpx.TimeoutException):
            await gateway.score_readability("要約")
