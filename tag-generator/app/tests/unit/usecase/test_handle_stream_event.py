"""Tests for HandleStreamEventUsecase."""

from unittest.mock import AsyncMock, MagicMock

import pytest

from tag_generator.domain.models import Tag, TagExtractionResult


def _make_result(article_id: str = "a-1") -> TagExtractionResult:
    return TagExtractionResult(
        article_id=article_id,
        tags=[Tag(name="ml", confidence=0.9)],
        language="en",
        inference_ms=42.0,
        overall_confidence=0.85,
    )


@pytest.mark.asyncio
class TestHandleStreamEventUsecase:
    async def test_generates_tags(self):
        from tag_generator.usecase.handle_stream_event import HandleStreamEventUsecase

        mock_extract = MagicMock()
        mock_extract.execute.return_value = _make_result()

        usecase = HandleStreamEventUsecase(extract_usecase=mock_extract)
        result = await usecase.handle_tag_generation_request(
            article_id="a-1",
            title="Title",
            content="Content",
            feed_id="f-1",
        )

        assert isinstance(result, TagExtractionResult)
        assert result.tag_names == ["ml"]

    async def test_publishes_reply_when_reply_to_set(self):
        from tag_generator.usecase.handle_stream_event import HandleStreamEventUsecase

        mock_extract = MagicMock()
        mock_extract.execute.return_value = _make_result()
        mock_publisher = AsyncMock()

        usecase = HandleStreamEventUsecase(
            extract_usecase=mock_extract,
            event_publisher=mock_publisher,
        )
        await usecase.handle_tag_generation_request(
            article_id="a-1",
            title="Title",
            content="Content",
            feed_id="f-1",
            reply_to="reply-stream",
            correlation_id="corr-123",
        )

        mock_publisher.publish_reply.assert_called_once()
        call_args = mock_publisher.publish_reply.call_args
        assert call_args[0][0] == "reply-stream"
        payload = call_args[0][1]["payload"]
        assert payload["success"] is True
        assert payload["article_id"] == "a-1"

    async def test_no_reply_when_no_publisher(self):
        from tag_generator.usecase.handle_stream_event import HandleStreamEventUsecase

        mock_extract = MagicMock()
        mock_extract.execute.return_value = _make_result()

        usecase = HandleStreamEventUsecase(extract_usecase=mock_extract)
        result = await usecase.handle_tag_generation_request(
            article_id="a-1",
            title="Title",
            content="Content",
            feed_id="f-1",
            reply_to="reply-stream",
        )
        # Should not raise, just skip publishing
        assert not result.is_empty
