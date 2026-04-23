"""RED: sentences in a cluster with multiple distinct sources get a corroboration boost."""

from unittest.mock import Mock

from news_creator.domain.models import RepresentativeSentence
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase


def _make_usecase() -> RecapSummaryUsecase:
    return RecapSummaryUsecase(config=Mock(), llm_provider=Mock())


def test_multi_source_cluster_scores_higher_than_solo_cluster():
    usecase = _make_usecase()
    sent_a = RepresentativeSentence(
        text="TechFusion が Nova Labs の買収を発表した。",
        source_url="https://a-news.example.com/1",
    )
    sent_b = RepresentativeSentence(
        text="Nova Labs 買収が規制当局に報告された。",
        source_url="https://b-news.example.com/2",
    )

    solo_score = usecase._score_sentence_for_packing(sent_a, cluster_sentences=[sent_a])
    multi_score = usecase._score_sentence_for_packing(
        sent_a, cluster_sentences=[sent_a, sent_b]
    )

    assert multi_score > solo_score, (
        f"multi-source should boost: solo={solo_score}, multi={multi_score}"
    )


def test_same_source_twice_does_not_boost():
    usecase = _make_usecase()
    sent_a = RepresentativeSentence(
        text="イベントA が発表された。", source_url="https://same.example.com/1"
    )
    sent_dup = RepresentativeSentence(
        text="イベントA に関する追加情報。", source_url="https://same.example.com/2"
    )

    solo = usecase._score_sentence_for_packing(sent_a, [sent_a])
    same_domain_cluster = usecase._score_sentence_for_packing(sent_a, [sent_a, sent_dup])

    assert same_domain_cluster == solo, (
        "same-domain duplicates should not count as multi-source corroboration"
    )
