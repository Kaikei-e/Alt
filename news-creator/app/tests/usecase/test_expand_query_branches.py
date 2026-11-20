"""Branch coverage tests for ExpandQueryUsecase non-response error handling."""

from __future__ import annotations

from collections.abc import AsyncIterator
from unittest.mock import AsyncMock, Mock

import pytest

from news_creator.domain.models import LLMGenerateResponse
from news_creator.usecase.expand_query_usecase import ExpandQueryUsecase


async def _empty_stream() -> AsyncIterator[LLMGenerateResponse]:
    if False:
        yield LLMGenerateResponse(response="", model="unused", done=True)


@pytest.mark.asyncio
async def test_expand_query_unexpected_stream_return_raises_runtime_error() -> None:
    config = Mock()
    llm_provider = AsyncMock()
    llm_provider.generate = AsyncMock(return_value=_empty_stream())
    usecase = ExpandQueryUsecase(config=config, llm_provider=llm_provider)

    with pytest.raises(RuntimeError, match="Query expansion failed"):
        await usecase.expand_query(query="AI news", japanese_count=1, english_count=1)
