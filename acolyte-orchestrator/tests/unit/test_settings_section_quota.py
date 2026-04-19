"""Tests for Settings.get_language_quota section/report routing."""

from __future__ import annotations

import json

from acolyte.config.settings import Settings


def _s(cfg: dict | None = None, quota_en: float = 0.2) -> Settings:
    return Settings(
        acolyte_db_dsn="postgresql://x",  # minimal required field
        language_quota_en=quota_en,
        section_language_quota_json=json.dumps(cfg or {}),
    )


def test_no_config_falls_back_to_language_quota_en():
    settings = Settings(acolyte_db_dsn="x", language_quota_en=0.25)
    assert settings.get_language_quota() == {"en": 0.25}


def test_empty_json_falls_back_to_language_quota_en():
    settings = _s({}, quota_en=0.3)
    assert settings.get_language_quota() == {"en": 0.3}


def test_default_entry_wins_over_language_quota_en():
    settings = _s({"_default": {"en": 0.4}}, quota_en=0.2)
    assert settings.get_language_quota() == {"en": 0.4}


def test_section_specific_override_for_allowed_role():
    settings = _s(
        {
            "weekly_briefing:analysis": {"en": 0.5},
            "_default": {"en": 0.2},
        }
    )
    assert settings.get_language_quota("analysis", "weekly_briefing") == {"en": 0.5}


def test_unknown_section_role_falls_back_to_default():
    settings = _s({"_default": {"en": 0.3}, "weekly_briefing:hacked": {"en": 0.99}})
    # "hacked" is not in the allowlist
    assert settings.get_language_quota("hacked", "weekly_briefing") == {"en": 0.3}


def test_unknown_report_type_falls_back_to_default():
    settings = _s({"_default": {"en": 0.3}, "evil_type:analysis": {"en": 0.99}})
    assert settings.get_language_quota("analysis", "evil_type") == {"en": 0.3}


def test_returns_fresh_dict_each_call():
    settings = _s({"_default": {"en": 0.3}})
    a = settings.get_language_quota()
    b = settings.get_language_quota()
    assert a == b
    assert a is not b


def test_invalid_json_falls_back_to_language_quota_en():
    settings = Settings(
        acolyte_db_dsn="x",
        language_quota_en=0.2,
        section_language_quota_json="{this is not json",
    )
    assert settings.get_language_quota("analysis", "weekly_briefing") == {"en": 0.2}


def test_non_dict_root_falls_back_to_language_quota_en():
    settings = Settings(
        acolyte_db_dsn="x",
        language_quota_en=0.2,
        section_language_quota_json=json.dumps(["not", "an", "object"]),
    )
    assert settings.get_language_quota("analysis", "weekly_briefing") == {"en": 0.2}


def test_market_analysis_japan_zeroes_en_quota():
    settings = _s(
        {
            "market_analysis_japan:analysis": {"en": 0.0},
            "_default": {"en": 0.2},
        }
    )
    assert settings.get_language_quota("analysis", "market_analysis_japan") == {"en": 0.0}
