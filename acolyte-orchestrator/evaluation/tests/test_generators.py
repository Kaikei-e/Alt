"""Tests for the recorded-fixture generator and its slug function."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from evaluation.dataset import EvalCase
from evaluation.generators.recorded_fixture import RecordedFixtureGenerator, slugify


def test_slugify_ascii_topic_returns_dashed_lowercase():
    assert slugify("2026 Q1 AI chip market outlook") == "2026-q1-ai-chip-market-outlook"


def test_slugify_japanese_topic_returns_sha_prefixed_slug():
    slug = slugify("電気自動車バッテリー技術")
    assert slug.startswith("topic-")
    assert len(slug) == len("topic-") + 12


def test_slugify_strips_leading_trailing_dashes():
    assert slugify("-- abc !! ") == "abc"


def test_recorded_fixture_reads_json(tmp_path: Path):
    fixtures = tmp_path / "fix"
    fixtures.mkdir()
    path = fixtures / "topic.json"
    path.write_text(
        json.dumps(
            {
                "body": "body",
                "source_map": {"S1": {"source_id": "a"}},
                "articles_by_id": {"a": {"language": "en"}},
                "evidence_by_short_id": {"S1": "quote"},
            }
        )
    )
    gen = RecordedFixtureGenerator(fixtures)
    body, sm, ab, ev = gen(EvalCase(topic="topic", query_lang="en", gold_source_ids=frozenset()))
    assert body == "body"
    assert sm == {"S1": {"source_id": "a"}}
    assert ab == {"a": {"language": "en"}}
    assert ev == {"S1": "quote"}


def test_recorded_fixture_missing_file_raises(tmp_path: Path):
    gen = RecordedFixtureGenerator(tmp_path)
    with pytest.raises(FileNotFoundError):
        gen(EvalCase(topic="not-there", query_lang="en", gold_source_ids=frozenset()))
