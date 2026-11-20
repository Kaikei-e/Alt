"""Branch coverage tests for RecapSummaryUsecase single-shot and payload sanitization."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, Mock
from uuid import uuid4

import pytest

from news_creator.domain.models import (
    LLMGenerateResponse,
    RecapClusterInput,
    RecapSummaryOptions,
    RecapSummaryRequest,
    RepresentativeSentence,
)
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase

MAX_REPETITION_RETRIES = 2
MIN_AVG_BULLET_LENGTH = 50


def _make_config() -> Mock:
    cfg = Mock()
    cfg.model_name = "gemma4-e4b-12k"
    cfg.summary_num_predict = 400
    cfg.recap_summary_num_predict = 400
    cfg.recap_min_avg_bullet_length = 0
    cfg.llm_temperature = 0.25
    cfg.max_repetition_retries = MAX_REPETITION_RETRIES
    cfg.llm_repeat_penalty = 1.0
    cfg.repetition_threshold = 0.3
    cfg.hierarchical_threshold_chars = 100_000
    cfg.hierarchical_threshold_clusters = 50
    cfg.hierarchical_chunk_max_chars = 20_000
    cfg.recap_summary_repair_attempts = 1
    cfg.recap_ja_ratio_threshold = 0.6
    return cfg


def _make_request(*, window_days: int | None = 7) -> RecapSummaryRequest:
    return RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        window_days=window_days,
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text="AI技術に関する重要な発表があった。")
                ],
                top_terms=["AI"],
            )
        ],
        options=RecapSummaryOptions(max_bullets=3, temperature=0.2),
    )


def test_sanitize_summary_payload_edge_cases() -> None:
    usecase = RecapSummaryUsecase(config=_make_config(), llm_provider=AsyncMock())

    nested = usecase._sanitize_summary_payload(
        {
            "summary": {
                "title": "```json",
                "bullets": ["最初の要点です。ここで区切るテキストです。"],
                "language": "",
                "references": [
                    "not-a-dict",
                    {"id": 0, "url": "https://a.example/1"},
                    {
                        "id": 1,
                        "url": "https://a.example/post",
                        "domain": "",
                        "source_url": "https://src.example/p",
                    },
                    {"id": 2, "url": "", "domain": ""},
                    {"id": 3, "url": "bare.domain", "domain": "bare.domain"},
                    {"id": 4, "url": "javascript:alert(1)", "domain": "bad"},
                ],
                "source_url": "https://top.example/article",
            }
        },
        max_bullets=3,
        strict_contract=False,
    )
    assert nested["title"] == "最初の要点です。ここで区切るテキストです。"
    assert nested["language"] == "ja"
    assert nested["source_url"] == "https://top.example/article"
    assert len(nested["references"]) == 1
    assert nested["references"][0]["domain"] == "a.example"
    assert nested["references"][0]["source_url"] == "https://src.example/p"

    empty_non_strict = usecase._sanitize_summary_payload(
        {"title": "", "bullets": [], "references": "not-a-list"},
        max_bullets=3,
        strict_contract=False,
    )
    assert empty_non_strict["title"] == "主要トピックのまとめ"
    assert empty_non_strict["bullets"] == ["主要トピックのまとめ"]
    assert "references" not in empty_non_strict

    non_list_bullets = usecase._sanitize_summary_payload(
        {"title": "見出し", "bullets": "not-a-list"},
        max_bullets=3,
        strict_contract=False,
    )
    assert non_list_bullets["bullets"] == ["見出し"]

    empty_strict = usecase._sanitize_summary_payload(
        {"title": None, "bullets": [], "language": None},
        max_bullets=3,
        strict_contract=True,
    )
    assert empty_strict["title"] == ""
    assert empty_strict["bullets"] == []
    assert empty_strict["language"] == ""


@pytest.mark.asyncio
async def test_single_shot_retries_repetition_and_short_bullets_then_falls_back() -> (
    None
):
    cfg = _make_config()
    cfg.recap_min_avg_bullet_length = MIN_AVG_BULLET_LENGTH
    llm_provider = AsyncMock()
    repeated_raw = LLMGenerateResponse(
        response="foo foo foo bar bar bar",
        model="gemma4",
    )
    short_bullets = LLMGenerateResponse(
        response=json.dumps(
            {
                "title": "AI動向",
                "bullets": ["短い [1]"],
                "language": "ja",
                "references": [
                    {"id": 1, "url": "https://ex.com/1", "domain": "ex.com"}
                ],
            },
            ensure_ascii=False,
        ),
        model="gemma4",
    )
    invalid_json = LLMGenerateResponse(
        response="not valid json at all",
        model="gemma4",
    )
    llm_provider.generate = AsyncMock(
        side_effect=[repeated_raw, short_bullets, invalid_json]
    )

    usecase = RecapSummaryUsecase(config=cfg, llm_provider=llm_provider)
    resp = await usecase._generate_single_shot_summary(
        _make_request(window_days=7), max_bullets=3, temperature_override=None
    )

    assert resp.metadata.model == "cluster-fallback"
    assert llm_provider.generate.await_count == MAX_REPETITION_RETRIES + 1
