package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

func TestGenerateTagsUsecase_GenerateTagsForArticle(t *testing.T) {
	t.Run("generates tags successfully with reply", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewGenerateTagsUsecase(mockPort)

		ctx := context.Background()
		req := &GenerateTagsRequest{
			ArticleID: "article-123",
			Title:     "Test Article",
			Content:   "This is the content of the test article.",
			FeedID:    "feed-456",
			TimeoutMs: 5000,
		}

		// Create expected reply event
		replyPayload, _ := json.Marshal(map[string]interface{}{
			"success":      true,
			"article_id":   "article-123",
			"inference_ms": 150.5,
			"tags": []map[string]interface{}{
				{"id": "tag-1", "name": "technology", "confidence": 0.95},
				{"id": "tag-2", "name": "testing", "confidence": 0.85},
			},
		})
		replyEvent := &domain.Event{
			EventID:   "reply-event-1",
			EventType: domain.EventTypeTagGenerationCompleted,
			Source:    "tag-generator",
			CreatedAt: time.Now(),
			Payload:   replyPayload,
		}

		// Expect Publish to be called with TagGenerationRequested event
		mockPort.On("Publish", ctx, domain.StreamKeyTags, mock.MatchedBy(func(e *domain.Event) bool {
			return e.EventType == domain.EventTypeTagGenerationRequested &&
				e.Metadata["reply_to"] != "" &&
				e.Metadata["correlation_id"] != ""
		})).Return("1234567890123-0", nil)

		// Expect SubscribeWithTimeout to be called with the reply stream
		mockPort.On("SubscribeWithTimeout", ctx, mock.MatchedBy(func(s domain.StreamKey) bool {
			return s.String() != "" // reply stream should be non-empty
		}), 5*time.Second).Return(replyEvent, nil)

		// Expect cleanup (Expire safety net + DeleteStream) using a context
		// detached from the request ctx.
		mockPort.On("Expire", mock.Anything, mock.MatchedBy(func(s domain.StreamKey) bool {
			return s.String() != ""
		}), replyStreamTTL).Return(nil)
		mockPort.On("DeleteStream", mock.Anything, mock.MatchedBy(func(s domain.StreamKey) bool {
			return s.String() != ""
		})).Return(nil)

		resp, err := uc.GenerateTagsForArticle(ctx, req)

		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "article-123", resp.ArticleID)
		assert.Len(t, resp.Tags, 2)
		assert.Equal(t, "technology", resp.Tags[0].Name)
		assert.InDelta(t, 0.95, resp.Tags[0].Confidence, 0.01)
		assert.InDelta(t, 150.5, resp.InferenceMs, 0.1)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns error when publish fails", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewGenerateTagsUsecase(mockPort)

		ctx := context.Background()
		req := &GenerateTagsRequest{
			ArticleID: "article-123",
			Title:     "Test Article",
			Content:   "Content",
			FeedID:    "feed-456",
			TimeoutMs: 5000,
		}

		mockPort.On("Publish", ctx, domain.StreamKeyTags, mock.AnythingOfType("*domain.Event")).
			Return("", errors.New("redis error"))

		// Cleanup should still be attempted
		mockPort.On("Expire", mock.Anything, mock.AnythingOfType("domain.StreamKey"), replyStreamTTL).Return(nil).Maybe()
		mockPort.On("DeleteStream", mock.Anything, mock.AnythingOfType("domain.StreamKey")).Return(nil).Maybe()

		resp, err := uc.GenerateTagsForArticle(ctx, req)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "publish request")
		assert.False(t, resp.Success)
		assert.Equal(t, "article-123", resp.ArticleID)
	})

	t.Run("returns error when timeout expires", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewGenerateTagsUsecase(mockPort)

		ctx := context.Background()
		req := &GenerateTagsRequest{
			ArticleID: "article-123",
			Title:     "Test Article",
			Content:   "Content",
			FeedID:    "feed-456",
			TimeoutMs: 1000,
		}

		mockPort.On("Publish", ctx, domain.StreamKeyTags, mock.AnythingOfType("*domain.Event")).
			Return("1234567890123-0", nil)

		mockPort.On("SubscribeWithTimeout", ctx, mock.AnythingOfType("domain.StreamKey"), 1*time.Second).
			Return(nil, errors.New("timeout waiting for reply"))

		mockPort.On("Expire", mock.Anything, mock.AnythingOfType("domain.StreamKey"), replyStreamTTL).Return(nil)
		mockPort.On("DeleteStream", mock.Anything, mock.AnythingOfType("domain.StreamKey")).Return(nil)

		resp, err := uc.GenerateTagsForArticle(ctx, req)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
		assert.False(t, resp.Success)
		mockPort.AssertExpectations(t)
	})

	t.Run("uses default timeout when not specified", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewGenerateTagsUsecase(mockPort)

		ctx := context.Background()
		req := &GenerateTagsRequest{
			ArticleID: "article-123",
			Title:     "Test Article",
			Content:   "Content",
			FeedID:    "feed-456",
			TimeoutMs: 0, // No timeout specified, should use default
		}

		replyPayload, _ := json.Marshal(map[string]interface{}{
			"success":    true,
			"article_id": "article-123",
			"tags":       []map[string]interface{}{},
		})
		replyEvent := &domain.Event{
			EventID:   "reply-1",
			EventType: domain.EventTypeTagGenerationCompleted,
			Payload:   replyPayload,
		}

		mockPort.On("Publish", ctx, domain.StreamKeyTags, mock.AnythingOfType("*domain.Event")).
			Return("123-0", nil)

		// Default timeout should be 60 seconds
		mockPort.On("SubscribeWithTimeout", ctx, mock.AnythingOfType("domain.StreamKey"), 60*time.Second).
			Return(replyEvent, nil)

		mockPort.On("Expire", mock.Anything, mock.AnythingOfType("domain.StreamKey"), replyStreamTTL).Return(nil)
		mockPort.On("DeleteStream", mock.Anything, mock.AnythingOfType("domain.StreamKey")).Return(nil)

		resp, err := uc.GenerateTagsForArticle(ctx, req)

		require.NoError(t, err)
		assert.True(t, resp.Success)
		mockPort.AssertExpectations(t)
	})

	t.Run("handles error response from tag-generator", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewGenerateTagsUsecase(mockPort)

		ctx := context.Background()
		req := &GenerateTagsRequest{
			ArticleID: "article-123",
			Title:     "Test Article",
			Content:   "Content",
			FeedID:    "feed-456",
			TimeoutMs: 5000,
		}

		replyPayload, _ := json.Marshal(map[string]interface{}{
			"success":       false,
			"article_id":    "article-123",
			"error_message": "model inference failed",
		})
		replyEvent := &domain.Event{
			EventID:   "reply-1",
			EventType: domain.EventTypeTagGenerationCompleted,
			Payload:   replyPayload,
		}

		mockPort.On("Publish", ctx, domain.StreamKeyTags, mock.AnythingOfType("*domain.Event")).
			Return("123-0", nil)

		mockPort.On("SubscribeWithTimeout", ctx, mock.AnythingOfType("domain.StreamKey"), 5*time.Second).
			Return(replyEvent, nil)

		mockPort.On("Expire", mock.Anything, mock.AnythingOfType("domain.StreamKey"), replyStreamTTL).Return(nil)
		mockPort.On("DeleteStream", mock.Anything, mock.AnythingOfType("domain.StreamKey")).Return(nil)

		resp, err := uc.GenerateTagsForArticle(ctx, req)

		require.NoError(t, err) // No error at RPC level, but success=false in response
		assert.False(t, resp.Success)
		assert.Equal(t, "model inference failed", resp.ErrorMessage)
		mockPort.AssertExpectations(t)
	})

	t.Run("cleanup runs even when subscribe fails", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewGenerateTagsUsecase(mockPort)

		ctx := context.Background()
		req := &GenerateTagsRequest{
			ArticleID: "article-123",
			Title:     "Test",
			Content:   "Content",
			FeedID:    "feed-456",
			TimeoutMs: 1000,
		}

		mockPort.On("Publish", ctx, domain.StreamKeyTags, mock.AnythingOfType("*domain.Event")).
			Return("123-0", nil)

		mockPort.On("SubscribeWithTimeout", ctx, mock.AnythingOfType("domain.StreamKey"), 1*time.Second).
			Return(nil, errors.New("connection lost"))

		// Cleanup (Expire + DeleteStream) should still be called
		mockPort.On("Expire", mock.Anything, mock.AnythingOfType("domain.StreamKey"), replyStreamTTL).Return(nil)
		mockPort.On("DeleteStream", mock.Anything, mock.AnythingOfType("domain.StreamKey")).Return(nil)

		_, err := uc.GenerateTagsForArticle(ctx, req)

		require.Error(t, err)
		mockPort.AssertExpectations(t)
	})

	t.Run("cleanup uses a context detached from a canceled request context", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewGenerateTagsUsecase(mockPort)

		reqCtx, cancel := context.WithCancel(context.Background())

		req := &GenerateTagsRequest{
			ArticleID: "article-123",
			Title:     "Test",
			Content:   "Content",
			FeedID:    "feed-456",
			TimeoutMs: 1000,
		}

		mockPort.On("Publish", reqCtx, domain.StreamKeyTags, mock.AnythingOfType("*domain.Event")).
			Return("123-0", nil)

		// Simulate the request being canceled while waiting for a reply.
		mockPort.On("SubscribeWithTimeout", reqCtx, mock.AnythingOfType("domain.StreamKey"), 1*time.Second).
			Run(func(args mock.Arguments) {
				cancel()
			}).
			Return(nil, context.Canceled)

		// Checks must happen inside the Run callback: the cleanup context's own
		// bounded timeout is canceled via defer as soon as cleanup finishes,
		// so asserting on it after GenerateTagsForArticle returns would always
		// observe context.Canceled regardless of whether the request ctx leaked in.
		mockPort.On("Expire", mock.Anything, mock.AnythingOfType("domain.StreamKey"), replyStreamTTL).
			Run(func(args mock.Arguments) {
				if err := args.Get(0).(context.Context).Err(); err != nil {
					t.Fatalf("Expire cleanup ctx must not be the canceled request ctx, got Err()=%v", err)
				}
			}).
			Return(nil)
		mockPort.On("DeleteStream", mock.Anything, mock.AnythingOfType("domain.StreamKey")).
			Run(func(args mock.Arguments) {
				if err := args.Get(0).(context.Context).Err(); err != nil {
					t.Fatalf("DeleteStream cleanup ctx must not be the canceled request ctx, got Err()=%v", err)
				}
			}).
			Return(nil)

		_, err := uc.GenerateTagsForArticle(reqCtx, req)

		require.Error(t, err)
		mockPort.AssertExpectations(t)
	})
}
