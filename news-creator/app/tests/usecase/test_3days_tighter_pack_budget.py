"""RED: 3days requests use a tighter MAX_CLUSTER_SECTION_LENGTH than 7days."""

from unittest.mock import Mock
from uuid import uuid4

from news_creator.domain.models import (
    RecapClusterInput,
    RecapSummaryRequest,
    RepresentativeSentence,
)
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase


def _make_usecase() -> RecapSummaryUsecase:
    return RecapSummaryUsecase(config=Mock(), llm_provider=Mock())


def _make_request(window_days: int) -> RecapSummaryRequest:
    return RecapSummaryRequest(
        job_id=uuid4(),
        genre="consumer_tech",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text="sample sentence.")
                ],
            )
        ],
        window_days=window_days,
    )


def test_3days_budget_is_smaller_than_7days():
    usecase = _make_usecase()
    budget_3d = usecase._max_cluster_section_length(_make_request(window_days=3))
    budget_7d = usecase._max_cluster_section_length(_make_request(window_days=7))

    assert budget_3d < budget_7d, (
        f"3days budget should be tighter: 3d={budget_3d}, 7d={budget_7d}"
    )


def test_3days_budget_is_8000_chars():
    usecase = _make_usecase()
    budget = usecase._max_cluster_section_length(_make_request(window_days=3))
    assert budget == 8_000


def test_7days_budget_is_12000_chars():
    usecase = _make_usecase()
    budget = usecase._max_cluster_section_length(_make_request(window_days=7))
    assert budget == 12_000
