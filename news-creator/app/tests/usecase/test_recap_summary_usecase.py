import pytest
from unittest.mock import AsyncMock, Mock
from uuid import uuid4

from news_creator.domain.models import (
    LLMGenerateResponse,
    RecapClusterInput,
    RecapSummaryOptions,
    RecapSummaryRequest,
)
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase


@pytest.mark.asyncio
async def test_generate_summary_success():
    config = Mock()
    config.summary_num_predict = 400
    config.llm_temperature = 0.25

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="""
        ```json
        {
          "title": "AI業界の大型買収",
          "bullets": [
            "米TechFusion社は2025年11月7日、AIスタートアップNova Labsを総額12億ドルで買収したと発表した。",
            "Nova Labsは生成AIモデルの高速最適化技術を持ち、買収後はTechFusionの研究開発拠点として運営される。",
            "規制当局の承認は未提示だが、TechFusionは統合完了を2026年3月と見込み、世界シェア拡大を狙う。"
          ],
          "language": "ja"
        }
        ```
        """,
        model="gemma3:4b",
        prompt_eval_count=512,
        eval_count=256,
        total_duration=1_750_000_000,
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="ai",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    "TechFusion announced the acquisition of Nova Labs for $1.2B.",
                    "Executives expect integration in March 2026.",
                ],
                top_terms=["acquisition", "AI", "Nova Labs"],
            ),
            RecapClusterInput(
                cluster_id=1,
                representative_sentences=[
                    "Nova Labs is known for fast fine-tuning infrastructure.",
                ],
            ),
        ],
        options=RecapSummaryOptions(max_bullets=3, temperature=0.6),
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    response = await usecase.generate_summary(request)

    assert response.job_id == request.job_id
    assert response.genre == "ai"
    assert response.summary.title == "AI業界の大型買収"
    assert len(response.summary.bullets) == 3
    assert response.summary.language == "ja"
    assert response.metadata.model == "gemma3:4b"
    assert response.metadata.temperature == pytest.approx(0.6)
    assert response.metadata.prompt_tokens == 512
    assert response.metadata.completion_tokens == 256
    assert response.metadata.processing_time_ms == 1750

    llm_provider.generate.assert_awaited_once()
    _, kwargs = llm_provider.generate.call_args
    assert kwargs["num_predict"] == config.summary_num_predict
    assert kwargs["options"] == {"temperature": 0.6}


@pytest.mark.asyncio
async def test_generate_summary_invalid_json_raises_runtime_error():
    config = Mock()
    config.summary_num_predict = 300
    config.llm_temperature = 0.2

    llm_provider = AsyncMock()
    llm_provider.generate.return_value = LLMGenerateResponse(
        response="### no json payload ###",
        model="gemma3:4b",
    )

    request = RecapSummaryRequest(
        job_id=uuid4(),
        genre="business",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=["Sample sentence for testing."]
            )
        ],
    )

    usecase = RecapSummaryUsecase(config=config, llm_provider=llm_provider)

    with pytest.raises(RuntimeError):
        await usecase.generate_summary(request)

