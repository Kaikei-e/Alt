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


# ============================================================================
# Chunk Splitting and Overlap Tests
# ============================================================================


def test_split_clusters_into_chunks_basic():
    """Test basic chunk splitting without overlap."""
    config = Mock()
    config.summary_num_predict = 400
    config.hierarchical_chunk_max_chars = 500  # Small for testing
    config.hierarchical_chunk_overlap_ratio = 0.0  # No overlap

    llm_provider = Mock()
    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    # Create clusters with known sizes
    clusters = [
        RecapClusterInput(
            cluster_id=i,
            representative_sentences=[
                RepresentativeSentence(text="A" * 100)  # 100 chars each
            ],
        )
        for i in range(5)
    ]

    chunks = usecase._split_clusters_into_chunks(clusters)

    # Each cluster is ~300 chars (100 + 200 overhead), so with 500 max, we get ~1 per chunk
    assert len(chunks) >= 2  # Should split into multiple chunks
    # All clusters should be represented
    all_cluster_ids = set()
    for chunk in chunks:
        for cluster in chunk:
            all_cluster_ids.add(cluster.cluster_id)
    assert len(all_cluster_ids) == 5


def test_split_clusters_into_chunks_with_overlap():
    """Test chunk splitting with overlap for context preservation."""
    config = Mock()
    config.hierarchical_chunk_max_chars = 600  # Small for testing
    config.hierarchical_chunk_overlap_ratio = 0.50  # 50% overlap for testing

    llm_provider = Mock()
    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    # Create clusters with known sizes
    clusters = [
        RecapClusterInput(
            cluster_id=i,
            representative_sentences=[
                RepresentativeSentence(text="A" * 100)  # 100 chars each
            ],
        )
        for i in range(6)
    ]

    chunks = usecase._split_clusters_into_chunks(clusters)

    # With overlap, some clusters should appear in multiple chunks
    if len(chunks) > 1:
        # Count cluster appearances
        cluster_counts = {}
        for chunk in chunks:
            for cluster in chunk:
                cluster_counts[cluster.cluster_id] = cluster_counts.get(cluster.cluster_id, 0) + 1

        # With 50% overlap, clusters near chunk boundaries should appear twice
        has_overlap = any(count > 1 for count in cluster_counts.values())
        # This is expected when overlap is enabled and we have multiple chunks
        assert has_overlap or len(chunks) == 1


def test_split_clusters_into_chunks_empty():
    """Test chunk splitting with empty clusters list."""
    config = Mock()
    config.hierarchical_chunk_max_chars = 1000
    config.hierarchical_chunk_overlap_ratio = 0.15

    llm_provider = Mock()
    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    chunks = usecase._split_clusters_into_chunks([])

    assert chunks == []


def test_split_clusters_into_chunks_single_large_cluster():
    """Test chunk splitting when a single cluster exceeds max_chars."""
    config = Mock()
    config.hierarchical_chunk_max_chars = 100  # Very small
    config.hierarchical_chunk_overlap_ratio = 0.15

    llm_provider = Mock()
    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    clusters = [
        RecapClusterInput(
            cluster_id=0,
            representative_sentences=[
                RepresentativeSentence(text="A" * 500)  # Large cluster
            ],
        )
    ]

    chunks = usecase._split_clusters_into_chunks(clusters)

    # Should have at least one chunk with the cluster
    assert len(chunks) >= 1
    assert clusters[0] in chunks[0]


# ============================================================================
# Recursive Reduce Tests (12K-only mode)
# ============================================================================


@pytest.mark.asyncio
async def test_recursive_reduce_with_large_intermediate_summaries():
    """Test that large intermediate summaries trigger recursive reduce."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100  # Trigger hierarchical for this test
    config.hierarchical_threshold_clusters = 3
    config.hierarchical_chunk_max_chars = 500
    config.hierarchical_chunk_overlap_ratio = 0.15
    # Recursive reduce settings
    config.recursive_reduce_max_chars = 500  # Small for testing
    config.recursive_reduce_max_depth = 3

    llm_provider = AsyncMock()

    # Track call count to verify multiple reduce calls
    call_count = 0

    def mock_generate(*args, **kwargs):
        nonlocal call_count
        call_count += 1
        # Return smaller summaries on each call
        if call_count <= 4:  # Map phase (initial chunks)
            return LLMGenerateResponse(
                response=json.dumps({
                    "bullets": [f"中間要約{call_count}-1 " + "詳細" * 50, f"中間要約{call_count}-2 " + "詳細" * 50],
                }),
                model="gemma3:4b",
                prompt_eval_count=100,
                eval_count=50,
                total_duration=500_000_000,
            )
        else:  # Reduce phase
            return LLMGenerateResponse(
                response=json.dumps({
                    "title": "最終要約タイトル",
                    "bullets": ["最終要点1", "最終要点2"],
                    "language": "ja"
                }),
                model="gemma3:4b",
                prompt_eval_count=100,
                eval_count=50,
                total_duration=500_000_000,
            )

    llm_provider.generate.side_effect = mock_generate

    # Create many clusters to trigger hierarchical summarization
    clusters = [
        RecapClusterInput(
            cluster_id=i,
            representative_sentences=[
                RepresentativeSentence(text=f"クラスタ{i}の代表文。" + "内容" * 20)
            ],
            top_terms=[f"term{i}"],
        )
        for i in range(10)
    ]

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="tech",
        clusters=clusters,
        options=RecapSummaryOptions(max_bullets=5),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    # Should have completed successfully
    assert response.summary.title is not None
    assert len(response.summary.bullets) >= 1
    # Map phase + reduce phase(s)
    assert call_count >= 2


@pytest.mark.asyncio
async def test_recursive_reduce_respects_max_depth():
    """Test that recursive reduce stops at max recursion depth."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100
    config.hierarchical_threshold_clusters = 2
    config.hierarchical_chunk_max_chars = 300
    config.hierarchical_chunk_overlap_ratio = 0.15
    config.recursive_reduce_max_chars = 50  # Very small to force recursion
    config.recursive_reduce_max_depth = 2  # Limited depth

    llm_provider = AsyncMock()

    # Always return large intermediate summaries
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps({
            "title": "要約タイトル",
            "bullets": ["長い要点" * 20],  # Large output
            "language": "ja"
        }),
        model="gemma3:4b",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=500_000_000,
    )

    clusters = [
        RecapClusterInput(
            cluster_id=i,
            representative_sentences=[
                RepresentativeSentence(text=f"クラスタ{i}の代表文。" + "内容" * 10)
            ],
        )
        for i in range(6)
    ]

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="tech",
        clusters=clusters,
        options=RecapSummaryOptions(max_bullets=3),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    # Should complete despite large intermediate summaries (max depth reached)
    assert response.summary is not None


@pytest.mark.asyncio
async def test_small_intermediate_summaries_skip_recursive_reduce():
    """Test that small intermediate summaries go directly to final reduce."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100
    config.hierarchical_threshold_clusters = 2
    config.hierarchical_chunk_max_chars = 2000
    config.hierarchical_chunk_overlap_ratio = 0.15
    config.recursive_reduce_max_chars = 10000  # Large enough to skip recursion
    config.recursive_reduce_max_depth = 3

    llm_provider = AsyncMock()

    call_count = 0

    def mock_generate(*args, **kwargs):
        nonlocal call_count
        call_count += 1
        if call_count == 1:  # Map phase
            return LLMGenerateResponse(
                response=json.dumps({
                    "bullets": ["短い要点1", "短い要点2"],
                }),
                model="gemma3:4b",
            )
        else:  # Final reduce
            return LLMGenerateResponse(
                response=json.dumps({
                    "title": "最終要約",
                    "bullets": ["最終要点1"],
                    "language": "ja"
                }),
                model="gemma3:4b",
            )

    llm_provider.generate.side_effect = mock_generate

    clusters = [
        RecapClusterInput(
            cluster_id=0,
            representative_sentences=[
                RepresentativeSentence(text="短いテスト文。")
            ],
        ),
        RecapClusterInput(
            cluster_id=1,
            representative_sentences=[
                RepresentativeSentence(text="もう一つの短い文。")
            ],
        ),
        RecapClusterInput(
            cluster_id=2,
            representative_sentences=[
                RepresentativeSentence(text="三つ目の短い文。")
            ],
        ),
    ]

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="tech",
        clusters=clusters,
        options=RecapSummaryOptions(max_bullets=3),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    # Should complete with minimal calls (map + single reduce)
    assert response.summary.title == "最終要約"
    # 3 clusters with 2000 max chars should fit in 1-2 chunks
    assert call_count <= 3

