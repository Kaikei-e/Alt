"""RED: numeric-fact sentences (金額/日付/%) get boosted pack score."""

from unittest.mock import Mock

from news_creator.domain.models import RepresentativeSentence
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase


def _make_usecase() -> RecapSummaryUsecase:
    return RecapSummaryUsecase(config=Mock(), llm_provider=Mock())


def test_numeric_fact_sentence_outranks_plain():
    usecase = _make_usecase()
    plain = RepresentativeSentence(text="テック企業が新しい方針を発表した。")
    numeric = RepresentativeSentence(text="売上高は 35% 増の 2,400 億円に達した。")

    score_plain = usecase._score_sentence_for_packing(plain, cluster_sentences=[plain])
    score_numeric = usecase._score_sentence_for_packing(
        numeric, cluster_sentences=[numeric]
    )

    assert score_numeric > score_plain, (
        f"numeric fact should rank higher: plain={score_plain}, numeric={score_numeric}"
    )


def test_percent_only_sentence_still_boosted():
    usecase = _make_usecase()
    plain = RepresentativeSentence(text="The market changed in some way today.")
    pct = RepresentativeSentence(text="The market fell 4.2% today.")
    assert usecase._score_sentence_for_packing(
        pct, [pct]
    ) > usecase._score_sentence_for_packing(plain, [plain])


def test_date_only_sentence_still_boosted():
    usecase = _make_usecase()
    plain = RepresentativeSentence(text="An announcement was made recently.")
    dated = RepresentativeSentence(text="On 2026-04-22 the acquisition was announced.")
    assert usecase._score_sentence_for_packing(
        dated, [dated]
    ) > usecase._score_sentence_for_packing(plain, [plain])
