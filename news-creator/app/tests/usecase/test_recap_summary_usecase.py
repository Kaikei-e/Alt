import json
import pytest
from unittest.mock import AsyncMock, Mock
from uuid import uuid4

from news_creator.domain.models import (
    BatchRecapSummaryRequest,
    LLMGenerateResponse,
    RecapClusterInput,
    RecapSummaryOptions,
    RecapSummaryRequest,
    RepresentativeSentence,
)
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase


@pytest.mark.asyncio
async def test_generate_summary_success():
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000

    llm_provider = AsyncMock()
    # Structured outputs return raw JSON without code blocks usually, but we strip them anyway
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="""
        {
          "title": "AI業界の大型買収",
          "bullets": [
            "米TechFusion社は2025年11月7日、AIスタートアップNova Labsを総額12億ドルで買収したと発表した。",
            "Nova Labsは生成AIモデルの高速最適化技術を持ち、買収後はTechFusionの研究開発拠点として運営される。",
            "規制当局の承認は未提示だが、TechFusionは統合完了を2026年3月と見込み、世界シェア拡大を狙う。"
          ],
          "language": "ja"
        }
        """,
        model="gemma3:4b",
        prompt_eval_count=512,
        eval_count=256,
        total_duration=1_750_000_000,
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text="TechFusion announced the acquisition of Nova Labs for $1.2B."),
                    RepresentativeSentence(text="Executives expect integration in March 2026."),
                ],
                top_terms=["acquisition", "AI", "Nova Labs"],
            ),
            RecapClusterInput(
                cluster_id=1,
                representative_sentences=[
                    RepresentativeSentence(text="Nova Labs is known for fast fine-tuning infrastructure."),
                ],
            ),
        ],
        options=RecapSummaryOptions(max_bullets=3, temperature=0.6),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    assert response.job_id == request.job_id
    assert response.genre == "ai"
    assert response.summary.title == "AI業界の大型買収"
    assert len(response.summary.bullets) == 3
    assert response.summary.language == "ja"
    assert response.metadata.model == "gemma3:4b"
    assert response.metadata.temperature == pytest.approx(0.6)
    assert response.metadata.prompt_tokens == 512
    assert response.metadata.completion_tokens == 256
    assert response.metadata.processing_time_ms == 1750

    llm_provider.generate.assert_awaited_once()
    _, kwargs = llm_provider.generate.call_args
    assert kwargs["num_predict"] == config.summary_num_predict
    # Options should include temperature + repeat_penalty
    assert kwargs["options"]["temperature"] == 0.6
    assert kwargs["options"]["repeat_penalty"] == 1.1
    # Check that format is a dict (JSON Schema)
    assert isinstance(kwargs["format"], dict)


@pytest.mark.asyncio
async def test_generate_summary_raises_error_when_invalid_json():
    config = Mock()
    config.summary_num_predict = 300
    config.llm_temperature = 0.2
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000

    llm_provider = AsyncMock()
    # Return invalid JSON to trigger error
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="""
        レポート：主要な出来事
        - 経済の回復が進展
        """,
        model="gemma3:4b",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="business",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[RepresentativeSentence(text="Sample sentence for testing.")]
            )
        ],
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    with pytest.raises(RuntimeError):
        await usecase.generate_summary(request)


@pytest.mark.asyncio
async def test_generate_summary_trims_excess_bullets():
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.5
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 0.7
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000

    bullets = [f"要点{i}" for i in range(1, 13)]

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "要約",
                "bullets": bullets,
                "language": "ja",
            }
        ),
        model="gemma3:4b",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="science",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[RepresentativeSentence(text="Example sentence.")],
            )
        ],
        options=RecapSummaryOptions(max_bullets=8, temperature=0.3),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    assert len(response.summary.bullets) == 8


# ============================================================================
# Batch Processing Tests
# ============================================================================


def _create_mock_config():
    """Create a mock config for testing."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000
    return config


def _create_sample_request(genre: str) -> RecapSummaryRequest:
    """Create a sample recap summary request for testing."""
    return RecapSummaryRequest(
        job_id=uuid4(),
        genre=genre,
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text=f"Sample sentence for {genre}.")
                ],
                top_terms=[genre, "test"],
            )
        ],
        options=RecapSummaryOptions(max_bullets=3),
    )


def _create_llm_response(title: str, genre: str) -> LLMGenerateResponse:
    """Create a mock LLM response."""
    return LLMGenerateResponse(
        response=json.dumps({
            "title": title,
            "bullets": [f"{genre} の要点1", f"{genre} の要点2"],
            "language": "ja"
        }),
        model="gemma3:4b",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=1_000_000_000,
    )


@pytest.mark.asyncio
async def test_generate_batch_summary_success():
    """Test batch processing with multiple successful requests."""
    config = _create_mock_config()
    llm_provider = AsyncMock()

    # Return different responses for different genres
    llm_provider.generate.side_effect = [
        _create_llm_response("テック要約", "tech"),
        _create_llm_response("政治要約", "politics"),
    ]

    batch_request = BatchRecapSummaryRequest(
        requests=[
            _create_sample_request("tech"),
            _create_sample_request("politics"),
        ]
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_batch_summary(batch_request)

    assert len(response.responses) == 2
    assert len(response.errors) == 0
    assert response.responses[0].genre == "tech"
    assert response.responses[1].genre == "politics"
    assert llm_provider.generate.call_count == 2


@pytest.mark.asyncio
async def test_generate_batch_summary_partial_failure():
    """Test batch processing with some failed requests."""
    config = _create_mock_config()
    llm_provider = AsyncMock()

    # First request succeeds, second fails
    llm_provider.generate.side_effect = [
        _create_llm_response("テック要約", "tech"),
        RuntimeError("LLM service unavailable"),
    ]

    batch_request = BatchRecapSummaryRequest(
        requests=[
            _create_sample_request("tech"),
            _create_sample_request("politics"),
        ]
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_batch_summary(batch_request)

    assert len(response.responses) == 1
    assert len(response.errors) == 1
    assert response.responses[0].genre == "tech"
    assert response.errors[0].genre == "politics"
    assert "LLM service unavailable" in response.errors[0].error


@pytest.mark.asyncio
async def test_generate_batch_summary_all_fail():
    """Test batch processing when all requests fail."""
    config = _create_mock_config()
    llm_provider = AsyncMock()

    llm_provider.generate.side_effect = RuntimeError("LLM service unavailable")

    batch_request = BatchRecapSummaryRequest(
        requests=[
            _create_sample_request("tech"),
            _create_sample_request("politics"),
        ]
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_batch_summary(batch_request)

    assert len(response.responses) == 0
    assert len(response.errors) == 2


@pytest.mark.asyncio
async def test_generate_batch_summary_empty_requests():
    """Test batch processing with empty requests list should raise error."""
    config = _create_mock_config()
    llm_provider = AsyncMock()

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    # Pydantic validation should reject empty requests list
    with pytest.raises(ValueError):
        BatchRecapSummaryRequest(requests=[])


@pytest.mark.asyncio
async def test_generate_batch_summary_single_request():
    """Test batch processing with a single request."""
    config = _create_mock_config()
    llm_provider = AsyncMock()

    llm_provider.generate.return_value = _create_llm_response("テック要約", "tech")

    batch_request = BatchRecapSummaryRequest(
        requests=[_create_sample_request("tech")]
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_batch_summary(batch_request)

    assert len(response.responses) == 1
    assert len(response.errors) == 0
    assert response.responses[0].genre == "tech"

