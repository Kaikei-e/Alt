"""Unit tests for stream event handler."""

from datetime import datetime
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from tag_generator.stream_consumer import Event
from tag_generator.stream_event_handler import TagGeneratorEventHandler


@pytest.fixture
def mock_service():
    """Create a mock TagGeneratorService."""
    service = MagicMock()
    # Mock tag_extractor
    service.tag_extractor = MagicMock()
    outcome = MagicMock()
    outcome.tags = ["technology", "testing"]
    outcome.tag_confidences = {"technology": 0.95, "testing": 0.85}
    service.tag_extractor.extract_tags_with_metrics.return_value = outcome
    return service


@pytest.fixture
def mock_stream_consumer():
    """Create a mock StreamConsumer."""
    consumer = MagicMock()
    consumer.publish_reply = AsyncMock(return_value="msg-123")
    return consumer


@pytest.fixture
def handler(mock_service, mock_stream_consumer):
    """Create handler with mocks."""
    return TagGeneratorEventHandler(mock_service, mock_stream_consumer)


@pytest.fixture
def tag_generation_request_event():
    """Create a TagGenerationRequested event."""
    return Event(
        message_id="msg-1",
        event_id="evt-1",
        event_type="TagGenerationRequested",
        source="mq-hub",
        created_at=datetime.now(),
        payload={
            "article_id": "article-123",
            "title": "Test Article Title",
            "content": "This is the content of the test article.",
            "feed_id": "feed-456",
        },
        metadata={
            "correlation_id": "corr-123",
            "reply_to": "alt:replies:tags:corr-123",
        },
    )


class TestTagGeneratorEventHandler:
    """Tests for TagGeneratorEventHandler."""

    @pytest.mark.anyio
    async def test_handle_event_routes_tag_generation_requested(self, handler, tag_generation_request_event):
        """Test that TagGenerationRequested events are routed correctly."""
        with patch.object(handler, "_handle_tag_generation_requested", new_callable=AsyncMock) as mock_handle:
            await handler.handle_event(tag_generation_request_event)
            mock_handle.assert_called_once_with(tag_generation_request_event)

    @pytest.mark.anyio
    async def test_handle_tag_generation_requested_success(
        self, handler, mock_service, mock_stream_consumer, tag_generation_request_event
    ):
        """Test successful tag generation with reply."""
        await handler._handle_tag_generation_requested(tag_generation_request_event)

        # Verify tag extraction was called
        mock_service.tag_extractor.extract_tags_with_metrics.assert_called_once_with(
            "Test Article Title",
            "This is the content of the test article.",
        )

        # Verify reply was published
        mock_stream_consumer.publish_reply.assert_called_once()
        call_args = mock_stream_consumer.publish_reply.call_args

        assert call_args[0][0] == "alt:replies:tags:corr-123"  # reply_to stream

        event_data = call_args[0][1]
        assert event_data["event_type"] == "TagGenerationCompleted"
        assert event_data["payload"]["success"] is True
        assert event_data["payload"]["article_id"] == "article-123"
        assert len(event_data["payload"]["tags"]) == 2
        assert event_data["payload"]["tags"][0]["name"] == "technology"

    @pytest.mark.anyio
    async def test_handle_tag_generation_requested_missing_reply_to(self, handler, mock_stream_consumer):
        """Test that missing reply_to is handled gracefully."""
        event = Event(
            message_id="msg-1",
            event_id="evt-1",
            event_type="TagGenerationRequested",
            source="mq-hub",
            created_at=datetime.now(),
            payload={"article_id": "article-123"},
            metadata={},  # No reply_to
        )

        await handler._handle_tag_generation_requested(event)

        # Should not publish reply
        mock_stream_consumer.publish_reply.assert_not_called()

    @pytest.mark.anyio
    async def test_handle_tag_generation_requested_no_consumer(self, mock_service, tag_generation_request_event):
        """Test that missing stream consumer is handled gracefully."""
        handler = TagGeneratorEventHandler(mock_service, stream_consumer=None)

        await handler._handle_tag_generation_requested(tag_generation_request_event)

        # Should not raise, but also not process

    @pytest.mark.anyio
    async def test_handle_tag_generation_requested_extraction_error(
        self, handler, mock_service, mock_stream_consumer, tag_generation_request_event
    ):
        """Test error handling during tag extraction."""
        mock_service.tag_extractor.extract_tags_with_metrics.side_effect = Exception("Model inference failed")

        await handler._handle_tag_generation_requested(tag_generation_request_event)

        # Verify error reply was published
        mock_stream_consumer.publish_reply.assert_called_once()
        call_args = mock_stream_consumer.publish_reply.call_args

        event_data = call_args[0][1]
        assert event_data["payload"]["success"] is False
        assert "Model inference failed" in event_data["payload"]["error_message"]

    @pytest.mark.anyio
    async def test_handle_tag_generation_requested_invalid_payload(self, handler, mock_service, mock_stream_consumer):
        """Test that oversized or invalid payloads are rejected with error reply."""
        event = Event(
            message_id="msg-1",
            event_id="evt-1",
            event_type="TagGenerationRequested",
            source="mq-hub",
            created_at=datetime.now(),
            payload={
                "article_id": "",  # Empty article_id should fail validation
                "title": "Title",
                "content": "Content",
            },
            metadata={
                "correlation_id": "corr-123",
                "reply_to": "alt:replies:tags:corr-123",
            },
        )

        await handler._handle_tag_generation_requested(event)

        # Should publish error reply, not call tag extraction
        mock_service.tag_extractor.extract_tags_with_metrics.assert_not_called()
        mock_stream_consumer.publish_reply.assert_called_once()
        call_args = mock_stream_consumer.publish_reply.call_args
        event_data = call_args[0][1]
        assert event_data["payload"]["success"] is False
        assert "Invalid payload" in event_data["payload"]["error_message"]

    @pytest.mark.anyio
    async def test_handle_event_ignores_unknown_events(self, handler, mock_stream_consumer):
        """Test that unknown events are ignored."""
        event = Event(
            message_id="msg-1",
            event_id="evt-1",
            event_type="SomeUnknownEvent",
            source="test",
            created_at=datetime.now(),
            payload={},
            metadata={},
        )

        await handler.handle_event(event)

        # Should not publish anything
        mock_stream_consumer.publish_reply.assert_not_called()
