"""Branch coverage tests for MorningLetterUsecase._validate_letter_content."""

from __future__ import annotations

from news_creator.domain.models import MorningLetterContent, MorningLetterSection
from news_creator.usecase.morning_letter_usecase import MorningLetterUsecase


def test_morning_letter_validate_content_rejects_placeholder_title_and_narrative() -> (
    None
):
    valid_bullet = "TechFusion社がNova Labsを買収と発表 [1]"
    bad_title = MorningLetterContent(
        schema_version=1,
        lead="本日のトップニュースです。",
        sections=[
            MorningLetterSection(
                key="top3",
                title="...",
                bullets=[valid_bullet],
            )
        ],
    )
    res_title = MorningLetterUsecase._validate_letter_content(bad_title)
    assert res_title is not None
    assert res_title[0] == "placeholder_output"

    bad_narrative = MorningLetterContent(
        schema_version=1,
        lead="本日のトップニュースです。",
        sections=[
            MorningLetterSection(
                key="top3",
                title="主要トピック",
                bullets=[valid_bullet],
                narrative="...",
            )
        ],
    )
    res_narrative = MorningLetterUsecase._validate_letter_content(bad_narrative)
    assert res_narrative is not None
    assert res_narrative[0] == "placeholder_output"


def test_morning_letter_validate_content_rejects_empty_sections_and_bullets() -> None:
    empty_sections = MorningLetterContent.model_construct(
        schema_version=1,
        lead="本日のトップニュースです。",
        sections=[],
    )
    res_sections = MorningLetterUsecase._validate_letter_content(empty_sections)
    assert res_sections == (
        "placeholder_output",
        "No sections in Morning Letter content",
    )

    empty_bullets_section = MorningLetterSection.model_construct(
        key="top3",
        title="主要トピック",
        bullets=[],
        genre=None,
        narrative=None,
    )
    empty_bullets = MorningLetterContent.model_construct(
        schema_version=1,
        lead="本日のトップニュースです。",
        sections=[empty_bullets_section],
    )
    res_bullets = MorningLetterUsecase._validate_letter_content(empty_bullets)
    assert res_bullets == ("placeholder_output", "Empty bullets in section top3")
