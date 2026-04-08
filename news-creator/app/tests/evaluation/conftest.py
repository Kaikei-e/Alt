"""Shared fixtures for evaluation tests."""

from pathlib import Path

import pytest

from news_creator.domain.models import (
    RecapSummary,
    Reference,
)


FIXTURES_DIR = Path(__file__).parent / "fixtures"


def _make_summary_with_refs() -> RecapSummary:
    """Create a well-formed summary with proper reference alignment."""
    return RecapSummary(
        title="AI業界の最新動向",
        bullets=[
            "米TechFusion社は2025年11月7日、AIスタートアップNova Labsを総額12億ドル（約1,800億円）で買収すると発表した。Nova Labsは生成AIモデルの高速推論技術で知られ、買収後はTechFusionのAI研究開発拠点として統合される見込み。TechFusionは過去2年で3件のAI企業買収を実施しており、今回の買収でAIインフラ分野のシェアを15%から22%に引き上げることを目指す。統合完了は2026年3月を予定している [1]",
            "Googleは2025年11月6日、次世代言語モデル「Gemini 3.0」を発表し、マルチモーダル推論能力が従来比で40%向上したと公表した。特に日本語を含む多言語処理の精度が大幅に改善され、医療・法務分野での実用化を視野に入れている。同時にAPIの料金を平均30%引き下げ、開発者コミュニティの拡大を狙う。発表翌日にはAzureとの統合も明らかにされた [2]",
        ],
        language="ja",
        references=[
            Reference(
                id=1,
                url="https://techfusion.com/nova-labs",
                domain="techfusion.com",
                article_id="art1",
            ),
            Reference(
                id=2,
                url="https://blog.google/gemini3",
                domain="blog.google",
                article_id="art2",
            ),
        ],
    )


def _make_summary_broken_refs() -> RecapSummary:
    """Summary with broken reference alignment: dangling [3], unused ref id=2."""
    return RecapSummary(
        title="テスト要約",
        bullets=[
            "テスト文その1。参照あり [1]",
            "テスト文その2。存在しない参照 [3]",
        ],
        language="ja",
        references=[
            Reference(id=1, url="https://a.com/1", domain="a.com"),
            Reference(id=2, url="https://b.com/2", domain="b.com"),  # unused
        ],
    )


def _make_summary_no_refs() -> RecapSummary:
    """Summary with no references at all."""
    return RecapSummary(
        title="参照なし要約",
        bullets=["テスト文。参照マーカーなし。"],
        language="ja",
    )


def _make_redundant_summary() -> RecapSummary:
    """Summary with highly redundant bullets."""
    return RecapSummary(
        title="冗長テスト",
        bullets=[
            "TechFusion社がNova Labsを12億ドルで買収した。AI分野の大型M&Aとなる。",
            "TechFusion社によるNova Labsの12億ドル買収が発表された。AI業界で注目されている。",
            "全く別のトピック。日銀が金利を0.25%引き上げ、市場は円高に振れた。",
        ],
        language="ja",
    )


def _make_short_bullets_summary() -> RecapSummary:
    """Summary with bullets that are too short (< 400 chars)."""
    return RecapSummary(
        title="短い要約",
        bullets=[
            "短い文。",
            "これも短い。",
        ],
        language="ja",
    )


@pytest.fixture
def good_summary():
    return _make_summary_with_refs()


@pytest.fixture
def broken_refs_summary():
    return _make_summary_broken_refs()


@pytest.fixture
def no_refs_summary():
    return _make_summary_no_refs()


@pytest.fixture
def redundant_summary():
    return _make_redundant_summary()


@pytest.fixture
def short_summary():
    return _make_short_bullets_summary()
