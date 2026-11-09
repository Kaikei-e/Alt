"""Tests for Gemma 4 raw prompt templating guard and Morning Letter output validation."""

from contextlib import asynccontextmanager
import json
from unittest.mock import AsyncMock, Mock, patch
from uuid import uuid4

import pytest

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    CardGenerateRequest,
    CardItemInput,
    ConversationMessage,
    LLMGenerateResponse,
    MorningLetterGroupInput,
    MorningLetterRecapInput,
    MorningLetterRequest,
    RecapClusterInput,
    RecapSummaryOptions,
    RecapSummaryRequest,
    RepresentativeSentence,
)
from news_creator.gateway.ollama_gateway import OllamaGateway
from news_creator.gateway.payload_builder import (
    GeneratePayload,
    PayloadBuilder,
    prepare_raw_prompt,
)
from news_creator.usecase.expand_query_usecase import ExpandQueryUsecase
from news_creator.usecase.morning_letter_usecase import MorningLetterUsecase
from news_creator.usecase.recap_card_usecase import RecapCardUsecase
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase
from news_creator.usecase.summarize_usecase import SummarizeUsecase


def _assert_gemma_raw_prompt(prompt: str) -> None:
    assert isinstance(prompt, str)
    assert prompt.startswith("<|turn>"), (
        f"Prompt must start with <|turn>: {prompt[:60]!r}"
    )
    assert prompt.endswith("<|turn>model\n"), (
        f"Prompt must end with <|turn>model\\n: {prompt[-60:]!r}"
    )
    assert "<turn|>" in prompt


def _make_morning_letter_config() -> Mock:
    config = Mock()
    config.summary_num_predict = 2048
    config.llm_temperature = 0.1
    config.llm_repeat_penalty = 1.1
    config.get_llm_options.return_value = {
        "temperature": 0.1,
        "repeat_penalty": 1.1,
    }
    return config


def _make_morning_letter_request() -> MorningLetterRequest:
    return MorningLetterRequest(
        target_date="2026-04-07",
        edition_timezone="Asia/Tokyo",
        recap_summaries=[
            MorningLetterRecapInput(
                genre="ai",
                title="AI業界の大型買収と新モデル公開",
                bullets=[
                    "米TechFusion社はAIスタートアップNova Labsを12億ドルで買収した [1]",
                    "Googleは次世代マルチモーダルモデルGemini 3.0を正式発表した [2]",
                ],
                window_days=3,
            ),
            MorningLetterRecapInput(
                genre="finance",
                title="日銀の金融政策変更",
                bullets=[
                    "日本銀行は政策金利を0.25%引き上げる決定を発表した [3]",
                ],
                window_days=3,
            ),
        ],
        overnight_groups=[
            MorningLetterGroupInput(
                group_id=uuid4(),
                articles=[
                    RepresentativeSentence(
                        text="TechFusion社がNova Labsを12億ドルで買収と発表。",
                        source_url="https://techfusion.com/news",
                        article_id="art1",
                        is_centroid=True,
                    ),
                ],
            ),
            MorningLetterGroupInput(
                group_id=uuid4(),
                articles=[
                    RepresentativeSentence(
                        text="日本銀行が政策金利を0.25%引き上げる決定を発表。",
                        source_url="https://example.com/finance",
                        article_id="art2",
                        is_centroid=True,
                    ),
                ],
            ),
        ],
    )


def _make_valid_morning_letter_llm_response() -> LLMGenerateResponse:
    content = {
        "schema_version": 1,
        "lead": "本日のトップはTechFusion社によるNova Labsの大型買収と日銀の利上げ決定です。",
        "sections": [
            {
                "key": "top3",
                "title": "本日のトップ3",
                "bullets": [
                    "TechFusion社がNova Labsを12億ドルで買収と発表 [1]",
                    "Googleが次世代モデルGemini 3.0を公開 [2]",
                    "日銀が政策金利を0.25%引き上げ [3]",
                ],
                "genre": None,
            },
            {
                "key": "what_changed",
                "title": "昨日からの変化",
                "bullets": [
                    "AIスタートアップへの大型M&Aが一段と加速した [1]",
                ],
                "genre": None,
            },
            {
                "key": "by_genre:ai",
                "title": "AI",
                "bullets": [
                    "生成AI基盤の統合とモデル競争が新段階に入った [1]",
                ],
                "genre": "ai",
            },
        ],
        "source_recap_window_days": 3,
    }
    return LLMGenerateResponse(
        response=json.dumps(content, ensure_ascii=False),
        model="gemma4-e4b-12k",
        prompt_eval_count=800,
        eval_count=400,
        total_duration=2_000_000_000,
    )


@pytest.mark.asyncio
async def test_recap_summary_single_shot_prompt_is_gemma_templated():
    config = Mock()
    config.model_name = "gemma4-e4b-q4km"
    config.summary_num_predict = 400
    config.recap_summary_num_predict = 400
    config.recap_min_avg_bullet_length = 0
    config.llm_temperature = 0.25
    config.max_repetition_retries = 1
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AI業界の大型買収",
                "bullets": [
                    "米TechFusion社はAIスタートアップNova Labsを12億ドルで買収した。"
                ],
                "language": "ja",
            },
            ensure_ascii=False,
        ),
        model="gemma4-e4b-q4km",
        prompt_eval_count=200,
        eval_count=100,
        total_duration=500_000_000,
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(
                        text="TechFusion announced the acquisition of Nova Labs."
                    )
                ],
                top_terms=["AI", "Nova Labs"],
            )
        ],
        options=RecapSummaryOptions(max_bullets=3, temperature=0.2),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)
    await usecase.generate_summary(request)

    llm_provider.generate.assert_awaited_once()
    sent_prompt = llm_provider.generate.call_args.args[0]
    _assert_gemma_raw_prompt(sent_prompt)


@pytest.mark.asyncio
async def test_recap_summary_hierarchical_prompts_are_gemma_templated():
    config = Mock()
    config.model_name = "gemma4-e4b-q4km"
    config.summary_num_predict = 400
    config.recap_summary_num_predict = 400
    config.recap_min_avg_bullet_length = 0
    config.llm_temperature = 0.25
    config.max_repetition_retries = 1
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100
    config.hierarchical_threshold_clusters = 2
    config.hierarchical_chunk_max_chars = 300
    config.hierarchical_chunk_overlap_ratio = 0.15
    config.ollama_request_concurrency = 2
    config.recursive_reduce_max_chars = 50
    config.recursive_reduce_max_depth = 2

    captured_prompts: list[str] = []

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        yield 0.0, None, None

    async def mock_generate_raw(prompt: str, **kwargs):
        captured_prompts.append(prompt)
        return LLMGenerateResponse(
            response=json.dumps(
                {"bullets": [f"中間要点{len(captured_prompts)}" + "詳細" * 15]},
                ensure_ascii=False,
            ),
            model="gemma4-e4b-q4km",
            prompt_eval_count=100,
            eval_count=50,
            total_duration=200_000_000,
        )

    async def mock_generate(prompt: str, **kwargs):
        captured_prompts.append(prompt)
        return LLMGenerateResponse(
            response=json.dumps(
                {"title": "統合要約", "bullets": ["最終要点"], "language": "ja"},
                ensure_ascii=False,
            ),
            model="gemma4-e4b-q4km",
            prompt_eval_count=100,
            eval_count=50,
            total_duration=200_000_000,
        )

    llm_provider = Mock()
    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
    llm_provider.generate = AsyncMock(side_effect=mock_generate)

    clusters = [
        RecapClusterInput(
            cluster_id=i,
            representative_sentences=[
                RepresentativeSentence(text=f"クラスタ{i}の代表文です。" * 10)
            ],
            top_terms=[f"term{i}"],
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
    await usecase.generate_summary(request)

    assert len(captured_prompts) >= 3
    for prompt in captured_prompts:
        _assert_gemma_raw_prompt(prompt)


@pytest.mark.asyncio
async def test_recap_card_prompt_is_gemma_templated():
    class _DummyCache:
        async def get(self, key: str) -> str | None:
            return None

        async def set(
            self, key: str, value: str, ttl_seconds: int | None = None
        ) -> bool:
            return True

    config = Mock()
    config.model_name = "gemma4-e4b-12k"
    config.llm = Mock()
    config.llm.recap_card_max_new_tokens = 700
    config.llm.recap_card_temperature = 0.2
    config.llm.model_name = "gemma4-e4b-12k"
    config.llm.recap_ja_ratio_threshold = 0.6

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=(
            "【見出し】\nソラリス社、分散ログ基盤の次期版を公開\n"
            "【何が起きた】\n"
            "ソラリス社はログ収集エンジン「パルス」のバージョン3.0を正式公開した。[1]\n"
            "メモリ使用量が従来比で40%削減され、毎秒10万件の転送に対応した。[1]\n"
            "また、外部クラウドへの暗号化バックアップ機能が標準化された。[2]\n"
            "【なぜ重要】\n運用インフラのサーバ費用が年間約25%削減される。[1]\n"
            "【出典】\n[1] [2]\n"
        ),
        model="gemma4-e4b-12k",
        prompt_eval_count=120,
        eval_count=60,
        total_duration=300_000_000,
    )

    usecase = RecapCardUsecase(
        config=config,
        llm_provider=llm_provider,
        cache=_DummyCache(),
    )
    request = CardGenerateRequest(
        job_id=uuid4(),
        candidate_id=uuid4(),
        prompt_version="recap_card.v1",
        items=[
            CardItemInput(
                n=1,
                feed_id=uuid4(),
                title="テスト記事1",
                host="example.com",
                url="https://example.com/1",
                pub_date="2026-09-21T00:00:00Z",
                lede="ソラリス社が新基盤を発表した。",
            ),
            CardItemInput(
                n=2,
                feed_id=uuid4(),
                title="テスト記事2",
                host="example.org",
                url="https://example.org/2",
                pub_date="2026-09-21T00:00:00Z",
                lede="新基盤により暗号化機能が標準化された。",
            ),
        ],
    )
    await usecase.generate_card(request)

    llm_provider.generate.assert_awaited_once()
    sent_prompt = llm_provider.generate.call_args.kwargs["prompt"]
    _assert_gemma_raw_prompt(sent_prompt)


@pytest.mark.asyncio
async def test_expand_query_prompts_are_gemma_templated():
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="AI技術の最新動向\nAI technology trends",
        model="gemma4-e4b-12k",
        prompt_eval_count=100,
        eval_count=30,
        total_duration=200_000_000,
    )

    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)
    await usecase.expand_query(
        query="AI技術のトレンド",
        japanese_count=1,
        english_count=1,
    )
    await usecase.expand_query(
        query="その課題は？",
        japanese_count=1,
        english_count=1,
        conversation_history=[
            ConversationMessage(role="user", content="AI技術のトレンドを教えて"),
            ConversationMessage(role="assistant", content="生成AIの普及が進んでいます"),
        ],
    )

    assert llm_provider.generate.await_count == 2
    for call in llm_provider.generate.call_args_list:
        sent_prompt = call.args[0]
        _assert_gemma_raw_prompt(sent_prompt)


@pytest.mark.asyncio
async def test_summarize_single_hierarchical_and_stream_prompts_are_gemma_templated():
    config = Mock(spec=NewsCreatorConfig)
    config.max_repetition_retries = 1
    config.repetition_threshold = 0.3
    config.summary_num_predict = 500
    config.summary_temperature = 0.5
    config.llm_repeat_penalty = 1.15
    config.llm_num_ctx = 8192
    config.hierarchical_single_article_threshold = 500
    config.hierarchical_single_article_chunk_size = 200
    config.hierarchical_token_budget_percent = 75

    captured_prompts: list[str] = []

    @asynccontextmanager
    async def mock_hold_slot(is_high_priority=False):
        yield 0.0, None, None

    async def mock_generate_raw(prompt: str, **kwargs):
        captured_prompts.append(prompt)
        return LLMGenerateResponse(
            response="これは日本語のテスト要約文です。十分な長さを持つ要約テキストを返します。",
            model="gemma4-e4b-12k",
            done=True,
            done_reason="stop",
            prompt_eval_count=100,
            eval_count=40,
            total_duration=200_000_000,
        )

    async def mock_generate_stream(prompt: str, **kwargs):
        captured_prompts.append(prompt)

        async def _gen():
            yield LLMGenerateResponse(
                response="ストリーム要約チャンク",
                model="gemma4-e4b-12k",
                done=True,
            )

        return _gen()

    llm_provider = Mock()
    llm_provider.hold_slot = mock_hold_slot
    llm_provider.generate_raw = AsyncMock(side_effect=mock_generate_raw)
    llm_provider.generate = AsyncMock(side_effect=mock_generate_stream)

    usecase = SummarizeUsecase(config=config, llm_provider=llm_provider)

    # 1. Single-article summary path (200 chars < 500 threshold)
    await usecase.generate_summary("art-single", "あ" * 200)

    # 2. Hierarchical summary path (700 chars > 500 threshold)
    await usecase.generate_summary("art-hier", "い" * 700)

    # 3. Streaming summary path
    stream = usecase.generate_summary_stream("art-stream", "う" * 200)
    async for _ in stream:
        pass

    assert len(captured_prompts) >= 4
    for prompt in captured_prompts:
        _assert_gemma_raw_prompt(prompt)


@pytest.mark.asyncio
async def test_morning_letter_jinja_prompt_is_gemma_templated():
    config = _make_morning_letter_config()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = _make_valid_morning_letter_llm_response()

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    assert usecase.template is not None

    await usecase.generate_letter(_make_morning_letter_request())

    llm_provider.generate.assert_awaited_once()
    sent_prompt = llm_provider.generate.call_args.args[0]
    _assert_gemma_raw_prompt(sent_prompt)
    assert "<|turn>system\n" in sent_prompt
    assert "<|turn>user\n" in sent_prompt
    assert (
        sent_prompt.index("<|turn>system\n")
        < sent_prompt.index("<|turn>user\n")
        < sent_prompt.index("<|turn>model\n")
    )


@pytest.mark.asyncio
async def test_morning_letter_inline_prompt_is_gemma_templated():
    config = _make_morning_letter_config()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = _make_valid_morning_letter_llm_response()

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    usecase.template = None

    await usecase.generate_letter(_make_morning_letter_request())

    llm_provider.generate.assert_awaited_once()
    sent_prompt = llm_provider.generate.call_args.args[0]
    _assert_gemma_raw_prompt(sent_prompt)
    assert "<|turn>system\n" in sent_prompt
    assert "<|turn>user\n" in sent_prompt
    assert (
        sent_prompt.index("<|turn>system\n")
        < sent_prompt.index("<|turn>user\n")
        < sent_prompt.index("<|turn>model\n")
    )


@pytest.mark.parametrize(
    "untemplated_prompt",
    [
        "Plain untemplated prompt",
        "System instruction\n<|turn>model\n",
        "<|turn>user\nHello world<turn|>",
        "<|turn>model\n",
        "",
        "   ",
    ],
)
def test_prepare_raw_prompt_and_payload_builder_reject_untemplated_prompt(
    untemplated_prompt: str,
):
    from news_creator.domain.errors import UntemplatedRawPromptError

    with pytest.raises(UntemplatedRawPromptError):
        prepare_raw_prompt(untemplated_prompt)

    builder = PayloadBuilder()
    with pytest.raises(UntemplatedRawPromptError):
        builder.build(
            prompt=untemplated_prompt,
            model="gemma4-e4b-12k",
            options={},
            keep_alive="24h",
            raw=True,
        )

    with pytest.raises(UntemplatedRawPromptError):
        GeneratePayload(
            model="gemma4-e4b-12k",
            prompt=untemplated_prompt,
            options={},
            keep_alive="24h",
            stream=False,
            raw=True,
        )


def test_payload_builder_requires_options_and_keep_alive():
    builder = PayloadBuilder()
    templated = "<|turn>user\nHello<turn|>\n<|turn>model\n"
    with pytest.raises(TypeError):
        builder.build(prompt=templated, model="gemma4-e4b-12k")  # type: ignore[call-arg]
    with pytest.raises(TypeError):
        GeneratePayload(
            model="gemma4-e4b-12k",
            prompt=templated,
            options={},
        )  # type: ignore[call-arg]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "untemplated_prompt",
    [
        "Plain untemplated prompt",
        "Prefix before turn <|turn>user\nHello<turn|>\n<|turn>model\n",
        "<|turn>user\nHello<turn|>",
        "<|turn>model\n",
    ],
)
async def test_ollama_gateway_generate_and_generate_raw_reject_untemplated_prompt(
    untemplated_prompt: str,
):
    from news_creator.domain.errors import UntemplatedRawPromptError

    config = Mock(spec=NewsCreatorConfig)
    config.llm_service_url = "http://localhost:11434"
    config.model_name = "test-model"
    config.llm_timeout_seconds = 60
    config.llm_keep_alive = -1
    config.llm = Mock(inference_service_token=None)
    config.ollama_request_concurrency = 1
    config.oom_detection_enabled = False
    config.model_routing_enabled = False
    config.scheduling_rt_reserved_slots = 1
    config.scheduling_aging_threshold_seconds = 60.0
    config.scheduling_aging_boost = 0.5
    config.scheduling_preemption_enabled = True
    config.scheduling_preemption_wait_threshold_seconds = 2.0
    config.scheduling_priority_promotion_threshold_seconds = 600.0
    config.scheduling_guaranteed_be_ratio = 5
    config.max_queue_depth = 0
    config.scheduling_rt_mode = "fifo"
    config.scheduling_aging_sweep_interval_releases = 8
    config.llm_num_ctx = 4096
    config.is_base_model_name = Mock(return_value=False)
    config.is_bucket_model_name = Mock(return_value=False)
    config.get_keep_alive_for_model = Mock(return_value=-1)
    config.get_llm_options.return_value = {
        "num_ctx": 4096,
        "num_predict": 500,
        "temperature": 0.2,
        "num_gpu": 99,
    }

    driver = AsyncMock()
    driver.initialize = AsyncMock()
    driver.cleanup = AsyncMock()
    driver.generate = AsyncMock()

    with patch("news_creator.gateway.ollama_gateway.OllamaDriver", return_value=driver):
        gateway = OllamaGateway(config)
        await gateway.initialize()
        try:
            with pytest.raises(UntemplatedRawPromptError):
                await gateway.generate(untemplated_prompt)

            with pytest.raises(UntemplatedRawPromptError):
                await gateway.generate(untemplated_prompt, stream=True)

            with pytest.raises(UntemplatedRawPromptError):
                await gateway.generate_raw(untemplated_prompt)

            driver.generate.assert_not_called()
        finally:
            await gateway.cleanup()


@pytest.mark.asyncio
async def test_morning_letter_retries_once_on_placeholder_and_succeeds():
    config = _make_morning_letter_config()
    placeholder_content = {
        "schema_version": 1,
        "lead": "...",
        "sections": [
            {
                "key": "top3",
                "title": "...",
                "bullets": ["..."],
                "genre": None,
            }
        ],
        "source_recap_window_days": 3,
    }
    placeholder_response = LLMGenerateResponse(
        response=json.dumps(placeholder_content, ensure_ascii=False),
        model="gemma4-e4b-12k",
        prompt_eval_count=500,
        eval_count=40,
        total_duration=500_000_000,
    )
    valid_response = _make_valid_morning_letter_llm_response()

    llm_provider = AsyncMock()
    llm_provider.generate = AsyncMock(
        side_effect=[placeholder_response, valid_response]
    )

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(_make_morning_letter_request())

    assert llm_provider.generate.await_count == 2
    assert response.metadata.is_degraded is False
    assert response.metadata.degradation_reason is None
    assert (
        response.content.lead
        == "本日のトップはTechFusion社によるNova Labsの大型買収と日銀の利上げ決定です。"
    )


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "bad_lead,bad_bullet",
    [
        ("...", "TechFusion社がNova Labsを12億ドルで買収と発表 [1]"),
        ("……", "TechFusion社がNova Labsを12億ドルで買収と発表 [1]"),
        ("・・・", "TechFusion社がNova Labsを12億ドルで買収と発表 [1]"),
        (
            "本日のトップはTechFusion社によるNova Labsの大型買収です。",
            "...",
        ),
        (
            "本日のトップはTechFusion社によるNova Labsの大型買収です。",
            "……",
        ),
        (
            "本日のトップはTechFusion社によるNova Labsの大型買収です。",
            "・・・",
        ),
        (
            "本日のトップはTechFusion社によるNova Labsの大型買収です。",
            "ーーーーーー",
        ),
        (
            "本日のトップはTechFusion社によるNova Labsの大型買収です。",
            "——————",
        ),
        (
            "本日のトップはTechFusion社によるNova Labsの大型買収です。",
            "――――――",
        ),
        (
            "本日のトップはTechFusion社によるNova Labsの大型買収です。",
            "‥‥‥‥‥",
        ),
        (
            "短い",
            "TechFusion社がNova Labsを12億ドルで買収と発表 [1]",
        ),
        (
            "本日のトップはTechFusion社によるNova Labsの大型買収です。",
            "短文",
        ),
    ],
)
async def test_morning_letter_placeholder_or_short_output_falls_back_after_retry(
    bad_lead: str, bad_bullet: str
):
    config = _make_morning_letter_config()
    bad_content = {
        "schema_version": 1,
        "lead": bad_lead,
        "sections": [
            {
                "key": "top3",
                "title": "本日のトップ3",
                "bullets": [
                    bad_bullet,
                    "Googleが次世代モデルGemini 3.0を公開 [2]",
                ],
                "genre": None,
            }
        ],
        "source_recap_window_days": 3,
    }
    bad_response = LLMGenerateResponse(
        response=json.dumps(bad_content, ensure_ascii=False),
        model="gemma4-e4b-12k",
        prompt_eval_count=500,
        eval_count=50,
        total_duration=500_000_000,
    )

    llm_provider = AsyncMock()
    llm_provider.generate = AsyncMock(side_effect=[bad_response, bad_response])

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(_make_morning_letter_request())

    assert llm_provider.generate.await_count == 2
    assert response.metadata.is_degraded is True
    assert response.metadata.degradation_reason == "placeholder_output"
    assert response.content.lead != bad_lead
    assert "..." not in response.content.lead


@pytest.mark.asyncio
async def test_morning_letter_duplicate_bullets_across_sections_falls_back_after_retry():
    config = _make_morning_letter_config()
    duplicate_bullet = "TechFusion社がNova Labsを12億ドルで買収と発表 [1]"
    bad_content = {
        "schema_version": 1,
        "lead": "本日のトップはTechFusion社によるNova Labsの大型買収と日銀の利上げ決定です。",
        "sections": [
            {
                "key": "top3",
                "title": "本日のトップ3",
                "bullets": [
                    duplicate_bullet,
                    "Googleが次世代モデルGemini 3.0を公開 [2]",
                ],
                "genre": None,
            },
            {
                "key": "what_changed",
                "title": "昨日からの変化",
                "bullets": [
                    duplicate_bullet,
                ],
                "genre": None,
            },
        ],
        "source_recap_window_days": 3,
    }
    bad_response = LLMGenerateResponse(
        response=json.dumps(bad_content, ensure_ascii=False),
        model="gemma4-e4b-12k",
        prompt_eval_count=500,
        eval_count=100,
        total_duration=500_000_000,
    )

    llm_provider = AsyncMock()
    llm_provider.generate = AsyncMock(side_effect=[bad_response, bad_response])

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(_make_morning_letter_request())

    assert llm_provider.generate.await_count == 2
    assert response.metadata.is_degraded is True
    assert response.metadata.degradation_reason == "duplicate_bullets"


@pytest.mark.asyncio
async def test_morning_letter_low_japanese_ratio_falls_back_after_retry():
    config = _make_morning_letter_config()
    english_content = {
        "schema_version": 1,
        "lead": "Top news today: TechFusion completed the acquisition of Nova Labs for 1.2B USD.",
        "sections": [
            {
                "key": "top3",
                "title": "Top Stories",
                "bullets": [
                    "TechFusion acquired AI startup Nova Labs for 1.2 billion dollars [1]",
                    "Google officially launched Gemini 3.0 multimodal model [2]",
                    "Bank of Japan raised policy interest rate by 25 basis points [3]",
                ],
                "genre": None,
            },
            {
                "key": "what_changed",
                "title": "What Changed",
                "bullets": [
                    "Consolidation in the artificial intelligence sector accelerated [1]",
                ],
                "genre": None,
            },
        ],
        "source_recap_window_days": 3,
    }
    bad_response = LLMGenerateResponse(
        response=json.dumps(english_content, ensure_ascii=False),
        model="gemma4-e4b-12k",
        prompt_eval_count=500,
        eval_count=120,
        total_duration=500_000_000,
    )

    llm_provider = AsyncMock()
    llm_provider.generate = AsyncMock(side_effect=[bad_response, bad_response])

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(_make_morning_letter_request())

    assert llm_provider.generate.await_count == 2
    assert response.metadata.is_degraded is True
    assert response.metadata.degradation_reason == "low_japanese_ratio"


@pytest.mark.asyncio
async def test_morning_letter_accepts_fullwidth_alphanumeric_short_bullet_and_excludes_citations_from_ratio():
    from news_creator.usecase.morning_letter_usecase import MIN_BULLET_LENGTH

    assert MIN_BULLET_LENGTH == 4

    config = _make_morning_letter_config()
    content = {
        "schema_version": 1,
        "lead": "ＡＩ２０２６年の主要ニュースまとめ",
        "sections": [
            {
                "key": "top3",
                "title": "ＡＩ２０２６",
                "bullets": [
                    "円安が加速 [2]",
                    "AIが普及 [1][2][3][4][5][6][7][8]",
                ],
                "genre": None,
            }
        ],
        "source_recap_window_days": 3,
    }
    valid_response = LLMGenerateResponse(
        response=json.dumps(content, ensure_ascii=False),
        model="gemma4-e4b-12k",
        prompt_eval_count=400,
        eval_count=80,
        total_duration=400_000_000,
    )

    llm_provider = AsyncMock()
    llm_provider.generate = AsyncMock(return_value=valid_response)

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(_make_morning_letter_request())

    llm_provider.generate.assert_awaited_once()
    assert response.metadata.is_degraded is False
    assert response.metadata.degradation_reason is None
    assert response.content.lead == "ＡＩ２０２６年の主要ニュースまとめ"


def test_morning_letter_prompt_includes_narrative_guidance_in_jinja_and_inline():
    config = _make_morning_letter_config()
    usecase = MorningLetterUsecase(config=config, llm_provider=AsyncMock())
    req = _make_morning_letter_request()

    jinja_prompt = usecase._build_prompt(req, is_degraded=False)
    assert "narrative" in jinja_prompt

    usecase.template = None
    inline_prompt = usecase._build_prompt(req, is_degraded=False)
    assert "narrative" in inline_prompt


@pytest.mark.asyncio
async def test_morning_letter_retry_parse_failure_preserves_first_attempt_validation_reason():
    config = _make_morning_letter_config()
    placeholder_content = {
        "schema_version": 1,
        "lead": "...",
        "sections": [
            {
                "key": "top3",
                "title": "...",
                "bullets": ["..."],
                "genre": None,
            }
        ],
        "source_recap_window_days": 3,
    }
    attempt1_response = LLMGenerateResponse(
        response=json.dumps(placeholder_content, ensure_ascii=False),
        model="gemma4-e4b-12k",
        prompt_eval_count=400,
        eval_count=30,
        total_duration=300_000_000,
    )
    attempt2_unparseable = LLMGenerateResponse(
        response="not valid json at all",
        model="gemma4-e4b-12k",
        prompt_eval_count=400,
        eval_count=10,
        total_duration=200_000_000,
    )

    llm_provider = AsyncMock()
    llm_provider.generate = AsyncMock(
        side_effect=[attempt1_response, attempt2_unparseable]
    )

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(_make_morning_letter_request())

    assert llm_provider.generate.await_count == 2
    assert response.metadata.is_degraded is True
    assert response.metadata.degradation_reason == "placeholder_output"


@pytest.mark.asyncio
async def test_morning_letter_omits_generated_at_from_schema_prompt_and_response():
    from news_creator.domain.models import MorningLetterContent

    schema = MorningLetterContent.model_json_schema()
    assert "generated_at" not in schema.get("properties", {})

    config = _make_morning_letter_config()
    llm_provider = AsyncMock()
    llm_provider.generate = AsyncMock(
        return_value=_make_valid_morning_letter_llm_response()
    )
    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    req = _make_morning_letter_request()

    assert "generated_at" not in usecase._build_prompt(req, is_degraded=False)
    usecase.template = None
    assert "generated_at" not in usecase._build_prompt(req, is_degraded=False)

    response = await usecase.generate_letter(req)
    dumped = response.model_dump()
    assert dumped["target_date"] == "2026-04-07"
    assert dumped["edition_timezone"] == "Asia/Tokyo"
    assert "generated_at" not in dumped["content"]

    fallback_content = usecase._build_extractive_fallback(req)
    assert "generated_at" not in fallback_content.model_dump()
