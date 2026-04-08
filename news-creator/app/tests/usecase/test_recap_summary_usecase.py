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
        model="gemma4-e4b-q4km",
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
        options=RecapSummaryOptions(max_bullets=3, temperature=0.6),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    assert response.job_id == request.job_id
    assert response.genre == "ai"
    assert response.summary.title == "AI業界の大型買収"
    assert len(response.summary.bullets) == 3
    assert response.summary.language == "ja"
    assert response.metadata.model == "gemma4-e4b-q4km"
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
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="business",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text="Sample sentence for testing.")
                ],
            )
        ],
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    # Invalid JSON triggers graceful fallback (not exception) — Issue 5 improvement
    response = await usecase.generate_summary(request)
    assert response.metadata.is_degraded is True
    assert response.metadata.model == "cluster-fallback"


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
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="science",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text="Example sentence.")
                ],
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
    config.ollama_request_concurrency = 2
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
        response=json.dumps(
            {
                "title": title,
                "bullets": [f"{genre} の要点1", f"{genre} の要点2"],
                "language": "ja",
            }
        ),
        model="gemma4-e4b-q4km",
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

    RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    # Pydantic validation should reject empty requests list
    with pytest.raises(ValueError):
        BatchRecapSummaryRequest(requests=[])


@pytest.mark.asyncio
async def test_generate_batch_summary_single_request():
    """Test batch processing with a single request."""
    config = _create_mock_config()
    llm_provider = AsyncMock()

    llm_provider.generate.return_value = _create_llm_response("テック要約", "tech")

    batch_request = BatchRecapSummaryRequest(requests=[_create_sample_request("tech")])

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
                cluster_counts[cluster.cluster_id] = (
                    cluster_counts.get(cluster.cluster_id, 0) + 1
                )

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
    from contextlib import asynccontextmanager

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
    config.ollama_request_concurrency = 2
    # Recursive reduce settings
    config.recursive_reduce_max_chars = 500  # Small for testing
    config.recursive_reduce_max_depth = 3

    llm_provider = Mock()

    # Track call count to verify multiple reduce calls
    generate_raw_call_count = 0

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        yield 0.0, None, None

    def mock_generate_raw(prompt, **kwargs):
        nonlocal generate_raw_call_count
        generate_raw_call_count += 1
        # Return smaller summaries on each call
        if generate_raw_call_count <= 4:  # Map phase (initial chunks)
            return LLMGenerateResponse(
                response=json.dumps(
                    {
                        "bullets": [
                            f"中間要約{generate_raw_call_count}-1 " + "詳細" * 50,
                            f"中間要約{generate_raw_call_count}-2 " + "詳細" * 50,
                        ],
                    }
                ),
                model="gemma4-e4b-q4km",
                prompt_eval_count=100,
                eval_count=50,
                total_duration=500_000_000,
            )
        else:  # Recursive reduce phase
            return LLMGenerateResponse(
                response=json.dumps(
                    {
                        "bullets": ["要約済み要点1", "要約済み要点2"],
                    }
                ),
                model="gemma4-e4b-q4km",
                prompt_eval_count=100,
                eval_count=50,
                total_duration=500_000_000,
            )

    # Final reduce goes through _generate_single_shot_summary which uses generate()
    def mock_generate(prompt, **kwargs):
        return LLMGenerateResponse(
            response=json.dumps(
                {
                    "title": "最終要約タイトル",
                    "bullets": ["最終要点1", "最終要点2"],
                    "language": "ja",
                }
            ),
            model="gemma4-e4b-q4km",
            prompt_eval_count=100,
            eval_count=50,
            total_duration=500_000_000,
        )

    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
    llm_provider.generate = AsyncMock(side_effect=mock_generate)

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
    # Map phase + reduce phase(s) via generate_raw
    assert generate_raw_call_count >= 2


@pytest.mark.asyncio
async def test_recursive_reduce_respects_max_depth():
    """Test that recursive reduce stops at max recursion depth."""
    from contextlib import asynccontextmanager

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
    config.ollama_request_concurrency = 2
    config.recursive_reduce_max_chars = 50  # Very small to force recursion
    config.recursive_reduce_max_depth = 2  # Limited depth

    llm_provider = Mock()

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        yield 0.0, None, None

    large_response = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "要約タイトル",
                "bullets": ["長い要点" * 20],  # Large output
                "language": "ja",
            }
        ),
        model="gemma4-e4b-q4km",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=500_000_000,
    )

    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = AsyncMock(return_value=large_response)
    llm_provider.generate = AsyncMock(return_value=large_response)

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
    from contextlib import asynccontextmanager

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
    config.ollama_request_concurrency = 2
    config.recursive_reduce_max_chars = 10000  # Large enough to skip recursion
    config.recursive_reduce_max_depth = 3

    llm_provider = Mock()

    generate_raw_call_count = 0

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        yield 0.0, None, None

    def mock_generate_raw(prompt, **kwargs):
        nonlocal generate_raw_call_count
        generate_raw_call_count += 1
        # Map phase: return small intermediate summaries
        return LLMGenerateResponse(
            response=json.dumps(
                {
                    "bullets": ["短い要点1", "短い要点2"],
                }
            ),
            model="gemma4-e4b-q4km",
        )

    # Final reduce goes through _generate_single_shot_summary (uses generate())
    def mock_generate(prompt, **kwargs):
        return LLMGenerateResponse(
            response=json.dumps(
                {"title": "最終要約", "bullets": ["最終要点1"], "language": "ja"}
            ),
            model="gemma4-e4b-q4km",
        )

    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
    llm_provider.generate = AsyncMock(side_effect=mock_generate)

    clusters = [
        RecapClusterInput(
            cluster_id=0,
            representative_sentences=[RepresentativeSentence(text="短いテスト文。")],
        ),
        RecapClusterInput(
            cluster_id=1,
            representative_sentences=[
                RepresentativeSentence(text="もう一つの短い文。")
            ],
        ),
        RecapClusterInput(
            cluster_id=2,
            representative_sentences=[RepresentativeSentence(text="三つ目の短い文。")],
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
    assert generate_raw_call_count <= 3


# ============================================================================
# Hierarchical BE Path Tests — map/reduce must use hold_slot+generate_raw
# ============================================================================


def _make_recap_config_for_hierarchical():
    """Config that triggers hierarchical summarization."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100  # Very low → always hierarchical
    config.hierarchical_threshold_clusters = 2  # Low → trigger on 3+ clusters
    config.hierarchical_chunk_max_chars = 2000
    config.hierarchical_chunk_overlap_ratio = 0.15
    config.ollama_request_concurrency = 2
    config.recursive_reduce_max_chars = 10000
    config.recursive_reduce_max_depth = 3
    return config


def _make_hierarchical_clusters(count=4):
    """Create clusters that exceed hierarchical threshold."""
    return [
        RecapClusterInput(
            cluster_id=i,
            representative_sentences=[
                RepresentativeSentence(
                    text=f"クラスタ{i}の代表文。テスト用の内容です。" * 5
                )
            ],
            top_terms=[f"term{i}"],
        )
        for i in range(count)
    ]


@pytest.mark.asyncio
async def test_hierarchical_map_phase_uses_hold_slot_generate_raw():
    """Map phase in recap hierarchical summarization must use hold_slot+generate_raw,
    not generate() (local-only), to avoid starving local GPU.

    Note: The final reduce step goes through _generate_single_shot_summary which uses
    generate() — this is acceptable because the final reduce input is small (combined bullets).
    Only the map and recursive reduce phases need BE dispatch.
    """
    from contextlib import asynccontextmanager

    config = _make_recap_config_for_hierarchical()
    llm_provider = Mock()

    hold_slot_calls = 0
    generate_raw_calls = 0

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        nonlocal hold_slot_calls
        hold_slot_calls += 1
        yield 0.0, None, None

    async def mock_generate_raw(prompt, **kwargs):
        nonlocal generate_raw_calls
        generate_raw_calls += 1
        return LLMGenerateResponse(
            response=json.dumps(
                {
                    "bullets": [f"要点{generate_raw_calls}"],
                }
            ),
            model="gemma4-e4b-q4km",
            prompt_eval_count=100,
            eval_count=50,
            total_duration=500_000_000,
        )

    # generate() is used by _generate_single_shot_summary for the final reduce
    async def mock_generate(prompt, **kwargs):
        return LLMGenerateResponse(
            response=json.dumps(
                {
                    "title": "最終要約",
                    "bullets": ["最終要点1", "最終要点2"],
                    "language": "ja",
                }
            ),
            model="gemma4-e4b-q4km",
            prompt_eval_count=100,
            eval_count=50,
            total_duration=500_000_000,
        )

    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
    llm_provider.generate = AsyncMock(side_effect=mock_generate)

    clusters = _make_hierarchical_clusters(4)
    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="tech",
        clusters=clusters,
        options=RecapSummaryOptions(max_bullets=3),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    assert hold_slot_calls >= 1, "hold_slot must be called for hierarchical map chunks"
    assert generate_raw_calls >= 1, (
        "generate_raw must be used for hierarchical map chunks"
    )
    assert response.summary is not None


@pytest.mark.asyncio
async def test_hierarchical_3days_map_phase_uses_3days_prompt_contract():
    """Hierarchical map phase should preserve window_days=3 and use the 3days contract."""
    from contextlib import asynccontextmanager

    config = _make_recap_config_for_hierarchical()
    llm_provider = Mock()

    captured_prompts = []

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        yield 0.0, None, None

    async def mock_generate_raw(prompt, **kwargs):
        captured_prompts.append(prompt)
        return LLMGenerateResponse(
            response=json.dumps(
                {
                    "bullets": ["主要な変化が確認された。", "追加の更新が確認された。"],
                    "language": "ja",
                }
            ),
            model="gemma4-e4b-q4km",
        )

    async def mock_generate(prompt, **kwargs):
        return LLMGenerateResponse(
            response=json.dumps(
                {
                    "title": "最終要約",
                    "bullets": [
                        "主要企業の動きが変化し、市場の競争環境に影響した [1]",
                        "規制側の更新も重なり、今後の導入見通しが変わった [2]",
                    ],
                    "language": "ja",
                    "references": [
                        {
                            "id": 1,
                            "url": "https://example.com/1",
                            "domain": "example.com",
                        },
                        {
                            "id": 2,
                            "url": "https://example.com/2",
                            "domain": "example.com",
                        },
                    ],
                }
            ),
            model="gemma4-e4b-q4km",
        )

    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
    llm_provider.generate = AsyncMock(side_effect=mock_generate)

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="tech",
        clusters=_make_hierarchical_clusters(4),
        options=RecapSummaryOptions(max_bullets=3, temperature=0.0),
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    await usecase.generate_summary(request)

    assert captured_prompts, "Expected at least one hierarchical map prompt"
    first_prompt = captured_prompts[0]
    assert first_prompt.startswith("<|turn>system\n")
    assert "直近3日間" in first_prompt
    assert "language" in first_prompt and '"ja"' in first_prompt


@pytest.mark.asyncio
async def test_hierarchical_reduce_group_uses_hold_slot_generate_raw():
    """Recursive reduce groups must also use hold_slot+generate_raw."""
    from contextlib import asynccontextmanager

    config = _make_recap_config_for_hierarchical()
    # Use small chunk max to force multiple chunks (map produces multiple summaries)
    config.hierarchical_chunk_max_chars = 300
    config.recursive_reduce_max_chars = 50  # Very small → force recursive reduce
    config.recursive_reduce_max_depth = 2
    llm_provider = Mock()

    generate_raw_calls = 0

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        yield 0.0, None, None

    async def mock_generate_raw(prompt, **kwargs):
        nonlocal generate_raw_calls
        generate_raw_calls += 1
        return LLMGenerateResponse(
            response=json.dumps(
                {
                    "bullets": [f"要点{generate_raw_calls}" + "詳細" * 20],
                }
            ),
            model="gemma4-e4b-q4km",
            prompt_eval_count=100,
            eval_count=50,
            total_duration=500_000_000,
        )

    # generate() is used by _generate_single_shot_summary for the final reduce
    async def mock_generate(prompt, **kwargs):
        return LLMGenerateResponse(
            response=json.dumps(
                {"title": "最終要約", "bullets": ["最終要点"], "language": "ja"}
            ),
            model="gemma4-e4b-q4km",
            prompt_eval_count=100,
            eval_count=50,
            total_duration=500_000_000,
        )

    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
    llm_provider.generate = AsyncMock(side_effect=mock_generate)

    # Use many clusters with long text to ensure multiple chunks
    clusters = [
        RecapClusterInput(
            cluster_id=i,
            representative_sentences=[
                RepresentativeSentence(text=f"クラスタ{i}の代表文。" * 10)
            ],
            top_terms=[f"term{i}"],
        )
        for i in range(10)
    ]
    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="tech",
        clusters=clusters,
        options=RecapSummaryOptions(max_bullets=3),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    # Map phase (multiple chunks) + recursive reduce = multiple generate_raw calls
    assert generate_raw_calls >= 2, (
        f"Expected at least 2 generate_raw calls (map + reduce phases), got {generate_raw_calls}"
    )
    assert response.summary is not None


# ============================================================================
# Issue 2: Prompt Split Tests (window_days → 3days/7days template selection)
# ============================================================================


@pytest.mark.asyncio
async def test_selects_3days_prompt_when_window_is_3():
    """window_days=3 → 3days-specific prompt template is used."""
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
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AI最新動向",
                "bullets": [
                    "TechFusion は 4月5日に Nova Labs 買収を発表し、統合後の推論基盤共通化を打ち出した。買収総額は12億ドルで、企業向け AI 競争を加速させる可能性がある [1]",
                    "Google は 4月6日に新モデルの API 提供時期を公開し、企業導入の前倒しを促した。価格改定も重なり、主要クラウド各社の競争は強まっている [2]",
                ],
                "language": "ja",
                "references": [
                    {"id": 1, "url": "https://a.com", "domain": "a.com"},
                    {"id": 2, "url": "https://b.com", "domain": "b.com"},
                ],
            }
        ),
        model="gemma4-e4b-q4km",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=1_000_000_000,
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Sample sentence for testing.",
                        source_url="https://example.com/1",
                        article_id="art1",
                    ),
                    RepresentativeSentence(
                        text="Another sample sentence for testing.",
                        source_url="https://example.com/2",
                        article_id="art2",
                    ),
                    RepresentativeSentence(
                        text="Third sample sentence for testing.",
                        source_url="https://example.com/3",
                        article_id="art3",
                    ),
                    RepresentativeSentence(
                        text="Fourth sample sentence for testing.",
                        source_url="https://example.com/4",
                        article_id="art4",
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    await usecase.generate_summary(request)

    # Verify that the prompt passed to LLM contains 3days-specific markers
    _, kwargs = llm_provider.generate.call_args
    # The generate() first arg is prompt (positional)
    call_args = llm_provider.generate.call_args
    prompt = call_args.args[0] if call_args.args else call_args.kwargs.get("prompt", "")
    assert "変化" in prompt or "変わった" in prompt, (
        "3days prompt should prioritize changes, but prompt does not contain change-focused markers"
    )


@pytest.mark.asyncio
async def test_3days_prompt_uses_gemma_turn_format():
    """3days recap prompts should be wrapped for Gemma raw prompting."""
    config = _create_mock_config()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AI最新動向",
                "bullets": [
                    "主要企業が新機能を公開し、競争環境が変化した [1]",
                    "規制当局も新方針を示し、今後の導入に影響する [2]",
                ],
                "language": "ja",
                "references": [
                    {"id": 1, "url": "https://example.com/1", "domain": "example.com"},
                    {"id": 2, "url": "https://example.com/2", "domain": "example.com"},
                ],
            }
        ),
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Sample sentence one.",
                        source_url="https://example.com/1",
                        article_id="art1",
                    ),
                    RepresentativeSentence(
                        text="Sample sentence two.",
                        source_url="https://example.com/2",
                        article_id="art2",
                    ),
                    RepresentativeSentence(
                        text="Sample sentence three.",
                        source_url="https://example.com/3",
                        article_id="art3",
                    ),
                    RepresentativeSentence(
                        text="Sample sentence four.",
                        source_url="https://example.com/4",
                        article_id="art4",
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    await usecase.generate_summary(request)

    call_args = llm_provider.generate.call_args
    prompt = call_args.args[0] if call_args.args else call_args.kwargs.get("prompt", "")
    assert prompt.startswith("<|turn>system\n")
    assert "<|turn>user\n" in prompt
    assert prompt.endswith("<|turn>model\n")


@pytest.mark.asyncio
async def test_selects_7days_prompt_when_window_is_none():
    """window_days=None (default) → existing 7days prompt template is used."""
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
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AI最新動向",
                "bullets": ["テスト要約"],
                "language": "ja",
            }
        ),
        model="gemma4-e4b-q4km",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=1_000_000_000,
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text="Sample sentence for testing.")
                ],
            )
        ],
        # window_days not set → defaults to None → 7days behavior
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    await usecase.generate_summary(request)

    # 7days prompt should contain the existing deep-dive markers
    call_args = llm_provider.generate.call_args
    prompt = call_args.args[0] if call_args.args else call_args.kwargs.get("prompt", "")
    # The existing prompt says "3〜7 個" for bullets
    assert "3〜7" in prompt, (
        "7days prompt should contain '3〜7' bullet count spec from existing template"
    )


@pytest.mark.asyncio
async def test_3days_max_bullets_default_is_7():
    """window_days=3 with no explicit max_bullets → default is 7 (not 15)."""
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
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "テスト",
                "bullets": [
                    "要点1: 主要企業が新製品投入や価格改定を進め、市場の競争環境がこの3日で大きく変化したことが確認された。今後の導入拡大にも影響する可能性が高い",
                    "要点2: 主要企業が新製品投入や価格改定を進め、市場の競争環境がこの3日で大きく変化したことが確認された。今後の導入拡大にも影響する可能性が高い",
                    "要点3: 主要企業が新製品投入や価格改定を進め、市場の競争環境がこの3日で大きく変化したことが確認された。今後の導入拡大にも影響する可能性が高い",
                    "要点4: 主要企業が新製品投入や価格改定を進め、市場の競争環境がこの3日で大きく変化したことが確認された。今後の導入拡大にも影響する可能性が高い",
                    "要点5: 主要企業が新製品投入や価格改定を進め、市場の競争環境がこの3日で大きく変化したことが確認された。今後の導入拡大にも影響する可能性が高い",
                    "要点6: 主要企業が新製品投入や価格改定を進め、市場の競争環境がこの3日で大きく変化したことが確認された。今後の導入拡大にも影響する可能性が高い",
                    "要点7: 主要企業が新製品投入や価格改定を進め、市場の競争環境がこの3日で大きく変化したことが確認された。今後の導入拡大にも影響する可能性が高い",
                    "要点8: 主要企業が新製品投入や価格改定を進め、市場の競争環境がこの3日で大きく変化したことが確認された。今後の導入拡大にも影響する可能性が高い",
                ],
                "language": "ja",
            }
        ),
        model="gemma4-e4b-q4km",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=1_000_000_000,
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Sample one.",
                        source_url="https://example.com/1",
                        article_id="art1",
                    ),
                    RepresentativeSentence(
                        text="Sample two.",
                        source_url="https://example.com/2",
                        article_id="art2",
                    ),
                    RepresentativeSentence(
                        text="Sample three.",
                        source_url="https://example.com/3",
                        article_id="art3",
                    ),
                    RepresentativeSentence(
                        text="Sample four.",
                        source_url="https://example.com/4",
                        article_id="art4",
                    ),
                ],
            )
        ],
        window_days=3,
        # No options → max_bullets should default to 7 for 3days
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    # 8 bullets in LLM response but max_bullets=7 → trimmed to 7
    assert len(response.summary.bullets) <= 7


@pytest.mark.asyncio
async def test_3days_prompt_contains_contract_examples_and_forbidden_patterns():
    """3days prompt should inline the contract, examples, and invalid-pattern guidance."""
    config = _create_mock_config()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AI最新動向",
                "bullets": [
                    "TechFusion は 4月5日に Nova Labs 買収を発表し、統合後の製品戦略を示した [1]",
                    "Google は 新モデルを公開し、API 提供時期も明示した [2]",
                ],
                "language": "ja",
                "references": [
                    {"id": 1, "url": "https://example.com/1", "domain": "example.com"},
                    {"id": 2, "url": "https://example.com/2", "domain": "example.com"},
                ],
            }
        ),
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="TechFusion bought Nova Labs for $1.2B.",
                        source_url="https://example.com/1",
                        article_id="art1",
                    ),
                    RepresentativeSentence(
                        text="Google launched a new model and shared the API timeline.",
                        source_url="https://example.com/2",
                        article_id="art2",
                    ),
                    RepresentativeSentence(
                        text="Japanese regulators published new AI guidance.",
                        source_url="https://example.com/3",
                        article_id="art3",
                    ),
                    RepresentativeSentence(
                        text="AWS lowered pricing for inference workloads.",
                        source_url="https://example.com/4",
                        article_id="art4",
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    await usecase.generate_summary(request)

    call_args = llm_provider.generate.call_args
    prompt = call_args.args[0] if call_args.args else call_args.kwargs.get("prompt", "")
    assert "良い例" in prompt
    assert "不正な例" in prompt
    assert '"references"' in prompt
    assert "... [1]" in prompt


@pytest.mark.asyncio
async def test_3days_strict_validation_rejects_english_title_and_missing_references():
    """3days strict validation should reject English-only title and uncited bullets."""
    config = _create_mock_config()
    config.max_repetition_retries = 0
    config.recap_summary_repair_attempts = 0

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AI updates",
                "bullets": [
                    "主要企業が新機能を公開し、市場の競争環境が変化した。",
                    "規制当局も新方針を示し、今後の導入見通しが変わった。",
                ],
                "language": "ja",
                "references": [],
            }
        ),
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Sample one.",
                        source_url="https://example.com/1",
                        article_id="art1",
                    ),
                    RepresentativeSentence(
                        text="Sample two.",
                        source_url="https://example.com/2",
                        article_id="art2",
                    ),
                    RepresentativeSentence(
                        text="Sample three.",
                        source_url="https://example.com/3",
                        article_id="art3",
                    ),
                    RepresentativeSentence(
                        text="Sample four.",
                        source_url="https://example.com/4",
                        article_id="art4",
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    assert response.metadata.is_degraded is True
    assert response.summary.language == "ja"


# ============================================================================
# Issue 3: Reduce Quality Tests
# ============================================================================


@pytest.mark.asyncio
async def test_reduce_group_uses_structured_prompt():
    """Reduce prompt should contain deduplication and reference preservation rules."""
    from contextlib import asynccontextmanager

    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25

    llm_provider = Mock()

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        yield 0.0, None, None

    captured_prompts = []

    async def mock_generate_raw(prompt, **kwargs):
        captured_prompts.append(prompt)
        return LLMGenerateResponse(
            response=json.dumps(
                {
                    "bullets": ["統合された要約 [1]"],
                    "language": "ja",
                }
            ),
            model="gemma4-e4b-q4km",
        )

    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = mock_generate_raw

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    from news_creator.domain.models import IntermediateSummary

    group = [
        IntermediateSummary(
            bullets=["TechFusionがNova Labsを買収 [1]", "Google新モデル発表 [2]"],
            language="ja",
        ),
        IntermediateSummary(
            bullets=["TechFusion買収でAI業界再編 [1]", "日銀金利変更 [3]"],
            language="ja",
        ),
    ]

    await usecase._reduce_group(group, Mock(job_id="test", genre="ai"), {}, {})

    assert len(captured_prompts) == 1
    reduce_prompt = captured_prompts[0]
    # Reduce prompt should contain structured rules (Issue 3 requirement)
    assert "重複" in reduce_prompt or "統合" in reduce_prompt, (
        "Reduce prompt should contain deduplication rules"
    )
    assert "参照" in reduce_prompt or "[n]" in reduce_prompt, (
        "Reduce prompt should mention reference preservation"
    )


@pytest.mark.asyncio
async def test_metadata_includes_reduce_depth():
    """Response metadata should include reduce_depth when hierarchical path is used."""
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
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "テスト",
                "bullets": ["要約"],
                "language": "ja",
            }
        ),
        model="gemma4-e4b-q4km",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=1_000_000_000,
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[RepresentativeSentence(text="Sample.")],
            )
        ],
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    # reduce_depth should exist in metadata (0 for single-shot)
    assert hasattr(response.metadata, "reduce_depth")
    assert response.metadata.reduce_depth == 0


# ============================================================================
# Issue 5: Fallback / Degraded Mode Tests
# ============================================================================


@pytest.mark.asyncio
async def test_fallback_includes_degraded_metadata():
    """When LLM returns invalid JSON on all retries, response has is_degraded=True."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 0  # No retries → immediate fallback
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000

    llm_provider = AsyncMock()
    # Return invalid JSON to trigger fallback
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="This is not valid JSON at all",
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Sample sentence.",
                        source_url="https://example.com/1",
                        article_id="art1",
                        is_centroid=True,
                    ),
                ],
            )
        ],
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    # Should not raise — should fall back gracefully
    response = await usecase.generate_summary(request)

    assert response.metadata.is_degraded is True
    assert response.metadata.degradation_reason is not None
    assert len(response.metadata.degradation_reason) > 0


@pytest.mark.asyncio
async def test_fallback_preserves_references():
    """Fallback response should include references from cluster source_urls."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 0
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="not json",
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="TechFusion bought Nova Labs for $1.2B.",
                        source_url="https://techfusion.com/news",
                        article_id="art1",
                        is_centroid=True,
                    ),
                ],
            ),
            RecapClusterInput(
                cluster_id=1,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Google launched Gemini 3.0.",
                        source_url="https://blog.google/gemini",
                        article_id="art2",
                    ),
                ],
            ),
        ],
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    # Fallback should preserve references
    assert response.summary.references is not None
    assert len(response.summary.references) >= 1
    urls = [ref.url for ref in response.summary.references]
    assert "https://techfusion.com/news" in urls


@pytest.mark.asyncio
async def test_3days_fallback_wraps_english_source_sentences_in_japanese():
    """3days degraded fallback should remain Japanese even when source sentences are English."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 0
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000
    config.recap_ja_ratio_threshold = 0.6

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="not json",
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="TechFusion bought Nova Labs for $1.2B and will integrate the platform in March 2026.",
                        source_url="https://example.com/techfusion-news",
                        article_id="art1",
                        is_centroid=True,
                    ),
                    RepresentativeSentence(
                        text="Google launched a new API pricing tier for enterprise inference workloads.",
                        source_url="https://example.com/google-gemini",
                        article_id="art2",
                    ),
                ],
                top_terms=["TechFusion", "Nova Labs"],
            ),
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    assert response.metadata.is_degraded is True
    assert response.summary.language == "ja"
    # English sentences should get a Japanese genre prefix (not the old template)
    assert "【" in response.summary.bullets[0]
    assert "TechFusion" in response.summary.bullets[0]
    assert response.summary.references is not None
    assert response.summary.references[0].url == "https://example.com/techfusion-news"


@pytest.mark.asyncio
async def test_fallback_selects_centroids_first():
    """Fallback should prefer centroid sentences over non-centroid."""
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25
    config.max_repetition_retries = 0
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="not json",
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Non-centroid secondary info.", is_centroid=False
                    ),
                    RepresentativeSentence(
                        text="CENTROID: Main topic here.", is_centroid=True
                    ),
                ],
            ),
        ],
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    # First bullet should be the centroid sentence
    assert "CENTROID" in response.summary.bullets[0]


@pytest.mark.asyncio
async def test_degraded_bullet_uses_raw_sentence_for_japanese():
    """Japanese representative sentence should be used directly without template wrapping."""
    config = _create_mock_config()
    config.max_repetition_retries = 0

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="not json", model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai_data",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Googleは新たなAIモデルを発表し、従来比30%の速度向上を実現した。",
                        source_url="https://example.com/1",
                        article_id="art1",
                        is_centroid=True,
                    ),
                ],
                top_terms=["AI", "model"],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    bullet = response.summary.bullets[0]
    # Japanese sentence should NOT be wrapped in the mechanical template
    assert "に関する更新が確認された" not in bullet
    assert "中心情報として示されており" not in bullet
    # The actual sentence content should be present
    assert "Googleは新たなAIモデルを発表" in bullet


@pytest.mark.asyncio
async def test_degraded_bullet_adds_genre_prefix_for_english():
    """English representative sentence should get a Japanese genre label prefix."""
    config = _create_mock_config()
    config.max_repetition_retries = 0

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="not json", model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="cybersecurity",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="A critical vulnerability was found in OpenSSL affecting millions of servers.",
                        source_url="https://example.com/1",
                        article_id="art1",
                        is_centroid=True,
                    ),
                ],
                top_terms=["vulnerability", "OpenSSL"],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    bullet = response.summary.bullets[0]
    # Should NOT use the old template
    assert "に関する更新が確認された" not in bullet
    # Should have a Japanese genre prefix for English content
    assert "サイバーセキュリティ" in bullet or "cybersecurity" in bullet.lower()
    # The original content should be present
    assert "OpenSSL" in bullet


@pytest.mark.asyncio
async def test_degraded_title_uses_japanese_genre_name():
    """Degraded titles should use Japanese genre name instead of raw English slug."""
    config = _create_mock_config()
    config.max_repetition_retries = 0

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="not json", model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="software_dev",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="New framework released.",
                        source_url="https://example.com/1",
                        article_id="art1",
                        is_centroid=True,
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    # Title should NOT contain the raw slug "software dev"
    assert "software dev" not in response.summary.title.lower()
    # Should contain Japanese genre name
    assert "ソフトウェア開発" in response.summary.title


@pytest.mark.asyncio
async def test_fallback_filters_short_sentences():
    """Fallback should exclude very short sentences (<20 chars)."""
    config = _create_mock_config()
    config.max_repetition_retries = 0

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="not json", model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai_data",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Short.",  # Too short - should be excluded
                        source_url="https://example.com/1",
                        article_id="art1",
                        is_centroid=True,
                    ),
                    RepresentativeSentence(
                        text="Google released a major update to their AI infrastructure affecting millions of developers worldwide.",
                        source_url="https://example.com/2",
                        article_id="art2",
                    ),
                ],
                top_terms=["AI", "Google"],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    # The short sentence should not appear in any bullet
    for bullet in response.summary.bullets:
        assert "Short." not in bullet


@pytest.mark.asyncio
async def test_generate_summary_bypasses_llm_for_low_evidence_3days():
    """Low-evidence 3days requests should use deterministic extractive output."""
    config = _create_mock_config()
    config.recap_min_source_articles_for_llm = 3
    config.recap_min_representative_sentences_for_llm = 4

    llm_provider = AsyncMock()

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="TechFusion announced a small update.",
                        source_url="https://example.com/1",
                        article_id="art1",
                        is_centroid=True,
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    llm_provider.generate.assert_not_awaited()
    assert response.metadata.is_degraded is True
    assert response.metadata.model == "low-evidence-extractive"
    assert response.metadata.degradation_reason == "low_evidence_extractive"
    assert response.summary.references is not None


@pytest.mark.asyncio
async def test_generate_summary_repairs_placeholder_and_non_japanese_output():
    """Semantic contract violations should trigger one repair attempt before fallback."""
    config = _create_mock_config()
    config.recap_summary_repair_attempts = 1
    config.recap_ja_ratio_threshold = 0.6

    llm_provider = AsyncMock()
    llm_provider.generate.side_effect = [
        LLMGenerateResponse(
            response=json.dumps(
                {
                    "title": "AI Updates",
                    "bullets": ["... [1]"],
                    "language": "en",
                    "references": [
                        {
                            "id": 1,
                            "url": "https://example.com/1",
                            "domain": "example.com",
                        }
                    ],
                }
            ),
            model="gemma4-e4b-q4km",
        ),
        LLMGenerateResponse(
            response=json.dumps(
                {
                    "title": "AI業界の重要更新",
                    "bullets": [
                        "TechFusion は 4月5日に Nova Labs の買収を発表し、統合後に推論基盤を共通化する方針を示した。買収額は12億ドルで、生成AI向け最適化技術の獲得が狙いとみられる [1]",
                        "Google は 4月6日に新モデルの API 提供時期を公表し、企業導入の前倒しを促した。競合各社も価格改定を進めており、市場の競争は一段と激しくなっている [2]",
                    ],
                    "language": "ja",
                    "references": [
                        {
                            "id": 1,
                            "url": "https://example.com/1",
                            "domain": "example.com",
                        },
                        {
                            "id": 2,
                            "url": "https://example.com/2",
                            "domain": "example.com",
                        },
                    ],
                }
            ),
            model="gemma4-e4b-q4km",
        ),
    ]

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="TechFusion announced the Nova Labs acquisition.",
                        source_url="https://example.com/1",
                        article_id="art1",
                    ),
                    RepresentativeSentence(
                        text="Google shared an API launch timeline for the new model.",
                        source_url="https://example.com/2",
                        article_id="art2",
                    ),
                    RepresentativeSentence(
                        text="Regulators published AI safety guidance.",
                        source_url="https://example.com/3",
                        article_id="art3",
                    ),
                    RepresentativeSentence(
                        text="AWS cut inference prices for enterprise workloads.",
                        source_url="https://example.com/4",
                        article_id="art4",
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    assert llm_provider.generate.await_count == 2
    assert response.metadata.is_degraded is False
    assert response.summary.language == "ja"
    assert response.summary.title == "AI業界の重要更新"


@pytest.mark.asyncio
async def test_generate_summary_repairs_reference_mismatch():
    """Broken [n] citations should trigger repair instead of accepting the payload."""
    config = _create_mock_config()
    config.recap_summary_repair_attempts = 1

    llm_provider = AsyncMock()
    llm_provider.generate.side_effect = [
        LLMGenerateResponse(
            response=json.dumps(
                {
                    "title": "AI業界の更新",
                    "bullets": [
                        "TechFusion は Nova Labs の買収を発表し、統合完了時期も示した [2]",
                        "Google は 新モデルの API 提供時期を公開した [3]",
                    ],
                    "language": "ja",
                    "references": [
                        {
                            "id": 1,
                            "url": "https://example.com/1",
                            "domain": "example.com",
                        }
                    ],
                }
            ),
            model="gemma4-e4b-q4km",
        ),
        LLMGenerateResponse(
            response=json.dumps(
                {
                    "title": "AI業界の更新",
                    "bullets": [
                        "TechFusion は Nova Labs の買収を発表し、統合完了時期も示した。買収額は12億ドルで、生成AI向け最適化技術を取り込む狙いがある [1]",
                        "Google は 新モデルの API 提供時期を公開し、企業導入の前倒しを促した。価格改定も重なり市場競争は一段と激しくなっている [2]",
                    ],
                    "language": "ja",
                    "references": [
                        {
                            "id": 1,
                            "url": "https://example.com/1",
                            "domain": "example.com",
                        },
                        {
                            "id": 2,
                            "url": "https://example.com/2",
                            "domain": "example.com",
                        },
                    ],
                }
            ),
            model="gemma4-e4b-q4km",
        ),
    ]

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="TechFusion announced the Nova Labs acquisition.",
                        source_url="https://example.com/1",
                        article_id="art1",
                    ),
                    RepresentativeSentence(
                        text="Google shared an API launch timeline for the new model.",
                        source_url="https://example.com/2",
                        article_id="art2",
                    ),
                    RepresentativeSentence(
                        text="Regulators published AI safety guidance.",
                        source_url="https://example.com/3",
                        article_id="art3",
                    ),
                    RepresentativeSentence(
                        text="AWS cut inference prices for enterprise workloads.",
                        source_url="https://example.com/4",
                        article_id="art4",
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    assert llm_provider.generate.await_count == 2
    assert response.metadata.is_degraded is False
    assert response.summary.references is not None
    assert [ref.id for ref in response.summary.references] == [1, 2]


@pytest.mark.asyncio
async def test_should_not_bypass_llm_with_2_sentences_from_1_source():
    """2 representative sentences from 1 source should NOT trigger LLM bypass."""
    config = _create_mock_config()
    # Use defaults — should be lenient enough for 2 sentences / 1 source
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AIモデルの最新動向",
                "bullets": [
                    "Googleは新たなAIモデルを発表し、従来モデルと比較して推論速度が大幅に向上した。同モデルは複数のベンチマークで最高精度を達成している [1]",
                    "同モデルの精度は95%に達しており、産業応用への展開が期待されている。特にヘルスケアおよび金融分野での導入が検討されている [1]",
                ],
                "language": "ja",
                "references": [
                    {"id": 1, "url": "https://example.com/1", "domain": "example.com", "article_id": "art1"}
                ],
            }
        ),
        model="gemma4-e4b-q4km",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=500_000_000,
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai_data",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Google released new AI model.",
                        source_url="https://example.com/1",
                        article_id="art1",
                        is_centroid=True,
                    ),
                    RepresentativeSentence(
                        text="The model achieves 95% accuracy.",
                        source_url="https://example.com/1",
                        article_id="art1",
                    ),
                ],
                top_terms=["ai", "model"],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    # LLM should have been called, not bypassed
    llm_provider.generate.assert_awaited()
    assert response.metadata.is_degraded is False


@pytest.mark.asyncio
async def test_should_bypass_llm_with_only_1_sentence():
    """Only 1 representative sentence should still trigger LLM bypass."""
    config = _create_mock_config()
    llm_provider = AsyncMock()

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai_data",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Single sentence only.",
                        source_url="https://example.com/1",
                        article_id="art1",
                        is_centroid=True,
                    ),
                ],
                top_terms=["ai"],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_summary(request)

    llm_provider.generate.assert_not_awaited()
    assert response.metadata.is_degraded is True
    assert response.metadata.degradation_reason == "low_evidence_extractive"


@pytest.mark.asyncio
async def test_3days_prompt_includes_anti_hallucination_section():
    """3days prompt should contain anti-hallucination instructions."""
    config = _create_mock_config()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AI最新アップデート",
                "bullets": [
                    "Googleは新しいAIモデルを4月5日にリリースし、推論速度が従来比30%向上した。同社は企業向けAPI料金も引き下げた [1]",
                    "日本政府はAI安全性に関するガイドラインを公開し、生成AIの責任範囲を明文化した [2]",
                ],
                "language": "ja",
                "references": [
                    {"id": 1, "url": "https://example.com/1", "domain": "example.com", "article_id": "art1"},
                    {"id": 2, "url": "https://example.com/2", "domain": "example.com", "article_id": "art2"},
                ],
            }
        ),
        model="gemma4-e4b-q4km",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="Google launched new model.", source_url="https://example.com/1", article_id="art1",
                    ),
                    RepresentativeSentence(
                        text="Japan published AI guidelines.", source_url="https://example.com/2", article_id="art2",
                    ),
                ],
            )
        ],
        window_days=3,
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    await usecase.generate_summary(request)

    call_args = llm_provider.generate.call_args
    prompt = call_args.args[0] if call_args.args else call_args.kwargs.get("prompt", "")
    # Anti-hallucination section should be present
    assert "禁止事項" in prompt
    assert "異なるクラスタの情報を1つのbulletに混合しない" in prompt


def test_3days_prompt_has_event_extraction_step():
    """3days prompt bullet construction should mention event-based extraction."""
    config = _create_mock_config()
    llm_provider = Mock()
    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text="Sample text for prompt test."),
                ],
            )
        ],
        window_days=3,
    )
    prompt = usecase._build_prompt(request, max_bullets=4)
    assert "イベント単位" in prompt
