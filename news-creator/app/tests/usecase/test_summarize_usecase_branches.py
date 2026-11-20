"""Branch characterization tests for SummarizeUsecase before complexity split."""

import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Self
from unittest.mock import AsyncMock, Mock

import pytest

import news_creator.usecase.summarize_usecase as uc_mod
from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import LLMGenerateResponse
from news_creator.usecase.summarize_usecase import SummarizeUsecase

_VALID_TEXT_LENGTH = 200
_SHORT_CLEANED_LENGTH = 40
_MAX_CONTENT_LENGTH = 60_000
_TRUNCATED_OVERFLOW_LENGTH = 65_000
_HUGE_THRESHOLD = 200_000
_SMALL_CONTEXT_WINDOW = 500
_SMALL_NUM_PREDICT = 50
_LARGE_CONTEXT_WINDOW = 20_000
_HIGH_BUDGET_PERCENT = 99
_CTX_FOR_90_WARN = 500
_BUDGET_PCT_NO_HIERARCHICAL = 1_000
_TINY_CTX_FOR_TRUNCATED_WARN = 200
_TRUNCATED_WARN_CONTENT_LEN = 1_500
_TINY_NUM_PREDICT = 10
_DEFAULT_PROMPT_TOKENS = 100
_DEFAULT_COMPLETION_TOKENS = 50
_DEFAULT_TOTAL_DURATION_NS = 1_000_000_000
_MULTIPLE_STREAM_TOKENS = 6
_REPEATED_PHRASE_MULTIPLIER = 30
_HIGH_REPETITION_SCORE = 0.95
_MIN_EXPECTED_NONEMPTY_RATIO = 0.0


def _make_config(
    *,
    max_repetition_retries: int = 2,
    llm_num_ctx: int = _LARGE_CONTEXT_WINDOW,
    summary_num_predict: int = 500,
    hierarchical_single_article_threshold: int = 20_000,
    hierarchical_token_budget_percent: int = _HIGH_BUDGET_PERCENT,
) -> Mock:
    config = Mock(spec=NewsCreatorConfig)
    config.max_repetition_retries = max_repetition_retries
    config.repetition_threshold = 0.3
    config.summary_num_predict = summary_num_predict
    config.summary_temperature = 0.5
    config.llm_repeat_penalty = 1.15
    config.llm_num_ctx = llm_num_ctx
    config.hierarchical_single_article_threshold = hierarchical_single_article_threshold
    config.hierarchical_single_article_chunk_size = 6_000
    config.hierarchical_token_budget_percent = hierarchical_token_budget_percent
    return config


def _make_llm_response(text: str) -> LLMGenerateResponse:
    return LLMGenerateResponse(
        response=text,
        model="test-model",
        done=True,
        done_reason="stop",
        prompt_eval_count=_DEFAULT_PROMPT_TOKENS,
        eval_count=_DEFAULT_COMPLETION_TOKENS,
        total_duration=_DEFAULT_TOTAL_DURATION_NS,
    )


def _make_hold_slot_provider(responses: list[LLMGenerateResponse]) -> Mock:
    provider = Mock()
    idx = 0

    @asynccontextmanager
    async def _hold_slot(
        *, is_high_priority: bool = False
    ) -> AsyncIterator[tuple[float, None, str]]:
        _ = is_high_priority
        yield (0.0, None, "task-branch-test")

    async def _generate_raw(
        _prompt: str,
        *,
        cancel_event: object = None,
        task_id: str | None = None,
        num_predict: int | None = None,
        options: dict[str, object] | None = None,
    ) -> LLMGenerateResponse:
        nonlocal idx
        _ = (cancel_event, task_id, num_predict, options)
        response = responses[idx]
        idx += 1
        return response

    provider.hold_slot = _hold_slot
    provider.generate_raw = AsyncMock(side_effect=_generate_raw)
    return provider


class _IteratorWithoutAclose:
    """AsyncIterator implementing only __aiter__/__anext__ without aclose."""

    def __init__(self, items: list[LLMGenerateResponse]) -> None:
        self._items = items
        self._index = 0

    def __aiter__(self) -> Self:
        return self

    async def __anext__(self) -> LLMGenerateResponse:
        if self._index >= len(self._items):
            raise StopAsyncIteration
        item = self._items[self._index]
        self._index += 1
        return item


class TestGenerateSummaryBranches:
    """Characterization tests for generate_summary uncovered branches."""

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("article_id", "content", "expected_msg"),
        [
            ("   ", "A" * _VALID_TEXT_LENGTH, "article_id cannot be empty"),
            ("art-empty-content", "   \n  ", "content cannot be empty"),
        ],
    )
    async def test_rejects_empty_inputs(
        self, article_id: str, content: str, expected_msg: str
    ) -> None:
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=Mock())
        with pytest.raises(ValueError, match=expected_msg):
            await usecase.generate_summary(article_id, content)

    @pytest.mark.asyncio
    async def test_logs_warning_when_html_removed(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        html_body = (
            "<html><body><p>"
            + ("これはHTML除去後のテスト記事本文です。" * 10)
            + "</p></body></html>"
        )
        provider = _make_hold_slot_provider(
            [_make_llm_response("テスト要約。十分な長さの要約結果です。")]
        )
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=provider)

        with caplog.at_level(logging.WARNING):
            summary, _metadata = await usecase.generate_summary("art-html-1", html_body)

        assert "テスト要約" in summary
        assert "HTML detected and removed from article content" in caplog.messages

    @pytest.mark.asyncio
    async def test_rejects_content_too_short_after_html_cleaning(self) -> None:
        short_html = "<p>" + ("A" * _SHORT_CLEANED_LENGTH) + "</p>"
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=Mock())

        with pytest.raises(ValueError, match="Content is empty or too short"):
            await usecase.generate_summary("art-short-html", short_html)

    @pytest.mark.asyncio
    async def test_truncates_content_above_60k_when_hierarchical_threshold_higher(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        long_content = "あ" * _TRUNCATED_OVERFLOW_LENGTH
        config = _make_config(
            hierarchical_single_article_threshold=_HUGE_THRESHOLD,
            llm_num_ctx=_LARGE_CONTEXT_WINDOW,
        )
        provider = _make_hold_slot_provider(
            [_make_llm_response("長い記事を要約した有効なテキスト出力です。")]
        )
        usecase = SummarizeUsecase(config=config, llm_provider=provider)

        with caplog.at_level(logging.WARNING):
            summary, _ = await usecase.generate_summary("art-trunc-60k", long_content)

        assert "長い記事" in summary
        assert "Input content truncated to fit context window" in caplog.messages
        sent_prompt = provider.generate_raw.call_args.args[0]
        assert len(sent_prompt) < _TRUNCATED_OVERFLOW_LENGTH

    @pytest.mark.asyncio
    async def test_warns_when_prompt_close_to_context_window_without_hierarchical_fallback(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        config = _make_config(
            llm_num_ctx=_CTX_FOR_90_WARN,
            summary_num_predict=_TINY_NUM_PREDICT,
            hierarchical_token_budget_percent=_BUDGET_PCT_NO_HIERARCHICAL,
        )
        provider = _make_hold_slot_provider(
            [_make_llm_response("コンテキスト上限近くの警告テスト要約結果です。")]
        )
        usecase = SummarizeUsecase(config=config, llm_provider=provider)

        with caplog.at_level(logging.WARNING):
            await usecase.generate_summary(
                "art-90pct", "B" * _TRUNCATED_WARN_CONTENT_LEN
            )

        assert "Prompt may be close to context window limit" in caplog.messages

    @pytest.mark.asyncio
    async def test_logs_error_and_proceeds_when_repetition_persists_after_max_retries(
        self,
        monkeypatch: pytest.MonkeyPatch,
        caplog: pytest.LogCaptureFixture,
    ) -> None:
        repeated_text = (
            "繰り返される要約テスト文字列です。" * _REPEATED_PHRASE_MULTIPLIER
        )
        monkeypatch.setattr(
            uc_mod,
            "detect_repetition",
            lambda _text, threshold=0.3: (True, _HIGH_REPETITION_SCORE, ["repeat"]),
        )
        config = _make_config(max_repetition_retries=1)
        provider = _make_hold_slot_provider(
            [
                _make_llm_response(repeated_text),
                _make_llm_response(repeated_text),
            ]
        )
        usecase = SummarizeUsecase(config=config, llm_provider=provider)

        with caplog.at_level(logging.ERROR):
            summary, _ = await usecase.generate_summary(
                "art-rep-exhausted", "C" * _VALID_TEXT_LENGTH
            )

        assert summary
        assert (
            "Repetition still detected after all retries, using summary anyway"
            in caplog.messages
        )

    @pytest.mark.asyncio
    async def test_single_empty_response_on_zero_retries_falls_through_to_empty_error(
        self,
    ) -> None:
        config = _make_config(max_repetition_retries=0)
        provider = _make_hold_slot_provider([_make_llm_response("   ")])
        usecase = SummarizeUsecase(config=config, llm_provider=provider)

        with pytest.raises(
            RuntimeError, match="LLM returned an empty summary after cleaning"
        ):
            await usecase.generate_summary(
                "art-empty-zero-retry", "D" * _VALID_TEXT_LENGTH
            )

    @pytest.mark.asyncio
    async def test_empty_after_cleaning_with_truncated_prompt_includes_warning(
        self,
    ) -> None:
        config = _make_config(
            max_repetition_retries=0,
            llm_num_ctx=_TINY_CTX_FOR_TRUNCATED_WARN,
            summary_num_predict=_TINY_NUM_PREDICT,
            hierarchical_token_budget_percent=_BUDGET_PCT_NO_HIERARCHICAL,
        )
        provider = _make_hold_slot_provider(
            [_make_llm_response("<|channel>thought\ninternal only<channel|>")]
        )
        usecase = SummarizeUsecase(config=config, llm_provider=provider)

        with pytest.raises(
            RuntimeError, match=r"WARNING: Prompt may have been truncated"
        ):
            await usecase.generate_summary("art-trunc-warn", "H" * _VALID_TEXT_LENGTH)

    @pytest.mark.asyncio
    async def test_abnormal_prompt_size_logs_error(
        self,
        monkeypatch: pytest.MonkeyPatch,
        caplog: pytest.LogCaptureFixture,
    ) -> None:
        amplified_prompt = "増幅テキスト。" * 20_000
        monkeypatch.setattr(
            uc_mod, "sanitize_for_prompt", lambda _text: amplified_prompt
        )
        config = _make_config(
            llm_num_ctx=100_000,
            summary_num_predict=500,
            hierarchical_token_budget_percent=100,
        )
        provider = _make_hold_slot_provider(
            [_make_llm_response("異常プロンプトサイズ検知後の要約結果です。")]
        )
        usecase = SummarizeUsecase(config=config, llm_provider=provider)

        with caplog.at_level(logging.ERROR):
            await usecase.generate_summary(
                "art-abnormal-prompt", "E" * _VALID_TEXT_LENGTH
            )

        assert "ABNORMAL PROMPT SIZE DETECTED in summarize_usecase" in caplog.messages
        assert (
            "Prompt size is much larger than content - possible repetition or amplification"
            in caplog.messages
        )


class TestGenerateSummaryStreamBranches:
    """Characterization tests for generate_summary_stream uncovered branches."""

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("article_id", "content", "expected_msg"),
        [
            ("", "A" * _VALID_TEXT_LENGTH, "article_id cannot be empty"),
            ("art-stream-empty", "   ", "content cannot be empty"),
        ],
    )
    async def test_stream_rejects_empty_inputs(
        self, article_id: str, content: str, expected_msg: str
    ) -> None:
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=Mock())
        with pytest.raises(ValueError, match=expected_msg):
            async for _chunk in usecase.generate_summary_stream(article_id, content):
                pass

    @pytest.mark.asyncio
    async def test_stream_truncates_content_over_60k_and_flushes_remaining_buffer(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        huge_content = "あ" * _TRUNCATED_OVERFLOW_LENGTH

        async def _stream() -> AsyncIterator[LLMGenerateResponse]:
            tokens = ["one ", "", "two ", "three ", "four ", "five ", "[unclosed"]
            for idx, tok in enumerate(tokens):
                yield LLMGenerateResponse(
                    response=tok,
                    model="test-model",
                    done=idx == len(tokens) - 1,
                )

        provider = Mock()
        provider.generate = AsyncMock(return_value=_stream())
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=provider)

        with caplog.at_level(logging.WARNING):
            emitted = [
                tok
                async for tok in usecase.generate_summary_stream(
                    "art-stream-trunc", huge_content
                )
            ]

        assert len("".join(emitted)) > _MIN_EXPECTED_NONEMPTY_RATIO
        assert "one " in emitted
        assert "five " in emitted
        assert any("unclosed" in tok for tok in emitted)
        assert "Content truncated for streaming" in caplog.messages

    @pytest.mark.asyncio
    async def test_stream_warns_when_no_data_yielded_and_accepts_iterator_without_aclose(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        items = [
            LLMGenerateResponse(response="", model="test-model", done=False),
            LLMGenerateResponse(response="", model="test-model", done=True),
        ]
        provider = Mock()
        provider.generate = AsyncMock(return_value=_IteratorWithoutAclose(items))
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=provider)

        with caplog.at_level(logging.WARNING):
            emitted = [
                tok
                async for tok in usecase.generate_summary_stream(
                    "art-stream-nodata", "F" * _VALID_TEXT_LENGTH
                )
            ]

        assert emitted == []
        assert "Stream completed but no data was yielded" in caplog.messages

    @pytest.mark.asyncio
    async def test_stream_wraps_type_error_when_provider_returns_non_iterator(
        self,
    ) -> None:
        provider = Mock()
        provider.generate = AsyncMock(return_value=_make_llm_response("not-a-stream"))
        usecase = SummarizeUsecase(config=_make_config(), llm_provider=provider)

        with pytest.raises(RuntimeError, match="Expected AsyncIterator for streaming"):
            async for _tok in usecase.generate_summary_stream(
                "art-stream-noniter", "G" * _VALID_TEXT_LENGTH
            ):
                pass
