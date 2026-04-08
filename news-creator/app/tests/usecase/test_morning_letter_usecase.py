"""Tests for MorningLetterUsecase — Morning Letter document generation."""

import json
import pytest
from unittest.mock import AsyncMock, Mock
from uuid import uuid4

from news_creator.domain.models import (
    LLMGenerateResponse,
    MorningLetterGroupInput,
    MorningLetterRecapInput,
    MorningLetterRequest,
    RepresentativeSentence,
)
from news_creator.usecase.morning_letter_usecase import MorningLetterUsecase


def _make_config():
    config = Mock()
    config.summary_num_predict = 2048
    config.llm_temperature = 0.1
    config.llm_repeat_penalty = 1.1
    return config


def _make_recap_input(genre: str = "ai") -> MorningLetterRecapInput:
    return MorningLetterRecapInput(
        genre=genre,
        title=f"{genre}の最新動向",
        bullets=[f"{genre}に関する重要な変化 [1]", f"{genre}の新技術が発表された [2]"],
        window_days=3,
    )


def _make_overnight_group() -> MorningLetterGroupInput:
    return MorningLetterGroupInput(
        group_id=uuid4(),
        articles=[
            RepresentativeSentence(
                text="TechFusion社がNova Labsを12億ドルで買収と発表。",
                source_url="https://techfusion.com/news",
                article_id="art1",
                is_centroid=True,
            ),
        ],
    )


def _make_llm_response_full() -> LLMGenerateResponse:
    """LLM response for full (non-degraded) Morning Letter."""
    content = {
        "schema_version": 1,
        "lead": "本日のトップ: TechFusion社のNova Labs買収が確定",
        "sections": [
            {
                "key": "top3",
                "title": "Top 3 Stories",
                "bullets": [
                    "TechFusion社がNova Labsを12億ドルで買収 [1]",
                    "Google Gemini 3.0を発表 [2]",
                    "日銀が金利を0.25%引き上げ [3]",
                ],
            },
            {
                "key": "what_changed",
                "title": "What Changed",
                "bullets": ["AI業界でM&A加速 [1]"],
            },
            {
                "key": "by_genre:ai",
                "title": "AI",
                "genre": "ai",
                "bullets": ["AIスタートアップの買収が活発化 [1]"],
            },
        ],
        "generated_at": "2026-04-07T06:00:00Z",
        "source_recap_window_days": 3,
    }
    return LLMGenerateResponse(
        response=json.dumps(content, ensure_ascii=False),
        model="gemma4-e4b-q4km",
        prompt_eval_count=500,
        eval_count=800,
        total_duration=5_000_000_000,
    )


def _make_llm_response_degraded() -> LLMGenerateResponse:
    """LLM response for degraded Morning Letter (no what_changed section)."""
    content = {
        "schema_version": 1,
        "lead": "本日のニュース概要",
        "sections": [
            {
                "key": "top3",
                "title": "Top 3 Stories",
                "bullets": [
                    "TechFusion社の買収発表 [1]",
                    "新しいAIモデルのリリース [2]",
                    "市場動向の変化 [3]",
                ],
            },
        ],
        "generated_at": "2026-04-07T06:00:00Z",
        "source_recap_window_days": None,
    }
    return LLMGenerateResponse(
        response=json.dumps(content, ensure_ascii=False),
        model="gemma4-e4b-q4km",
        prompt_eval_count=300,
        eval_count=400,
        total_duration=3_000_000_000,
    )


# ============================================================================
# Success Cases
# ============================================================================


@pytest.mark.asyncio
async def test_generate_letter_success():
    """Full letter with recap + overnight → all sections present."""
    config = _make_config()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = _make_llm_response_full()

    request = MorningLetterRequest(
        target_date="2026-04-07",
        edition_timezone="Asia/Tokyo",
        recap_summaries=[_make_recap_input("ai"), _make_recap_input("business")],
        overnight_groups=[_make_overnight_group()],
    )

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(request)

    assert response.target_date == "2026-04-07"
    assert response.edition_timezone == "Asia/Tokyo"
    assert response.content.lead is not None
    assert len(response.content.sections) >= 1
    assert response.content.schema_version == 1
    assert response.metadata.model == "gemma4-e4b-q4km"


# ============================================================================
# Degraded Mode
# ============================================================================


@pytest.mark.asyncio
async def test_generate_letter_degraded():
    """No recap summaries → degraded letter without what_changed section."""
    config = _make_config()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = _make_llm_response_degraded()

    request = MorningLetterRequest(
        target_date="2026-04-07",
        overnight_groups=[_make_overnight_group()],
        recap_summaries=None,  # No recap available
    )

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(request)

    assert response.metadata.is_degraded is True
    assert response.metadata.degradation_reason is not None
    # what_changed section should not be present
    section_keys = [s.key for s in response.content.sections]
    assert "what_changed" not in section_keys


# ============================================================================
# Section Key Contract
# ============================================================================


@pytest.mark.asyncio
async def test_generate_letter_preserves_section_key_contract():
    """Only allowed section keys (lead, top3, what_changed, by_genre:*) pass validation."""
    config = _make_config()
    llm_provider = AsyncMock()

    # LLM returns a response with an invalid section key
    bad_content = {
        "schema_version": 1,
        "lead": "テスト",
        "sections": [
            {"key": "top3", "title": "T", "bullets": ["b"]},
            {"key": "INVALID_KEY", "title": "Bad", "bullets": ["x"]},
        ],
        "generated_at": "2026-04-07T06:00:00Z",
    }
    llm_provider.generate.return_value = LLMGenerateResponse(
        response=json.dumps(bad_content, ensure_ascii=False),
        model="gemma4-e4b-q4km",
    )

    request = MorningLetterRequest(
        target_date="2026-04-07",
        overnight_groups=[_make_overnight_group()],
    )

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(request)

    # Invalid keys should be filtered out
    section_keys = [s.key for s in response.content.sections]
    assert "INVALID_KEY" not in section_keys
    assert "top3" in section_keys


# ============================================================================
# Extractive Fallback (LLM failure)
# ============================================================================


@pytest.mark.asyncio
async def test_generate_letter_llm_failure_uses_extractive_fallback():
    """When LLM returns invalid response, use deterministic extractive fallback."""
    config = _make_config()
    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="This is not valid JSON",
        model="gemma4-e4b-q4km",
    )

    request = MorningLetterRequest(
        target_date="2026-04-07",
        recap_summaries=[_make_recap_input("ai")],
        overnight_groups=[_make_overnight_group()],
    )

    usecase = MorningLetterUsecase(config=config, llm_provider=llm_provider)
    response = await usecase.generate_letter(request)

    # Should not raise — uses extractive fallback
    assert response.metadata.is_degraded is True
    assert response.metadata.model == "extractive-fallback"
    assert response.content.lead is not None
    assert len(response.content.sections) >= 1
