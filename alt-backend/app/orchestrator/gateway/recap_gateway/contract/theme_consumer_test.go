//go:build contract

package contract

import (
	"context"
	"fmt"
	"testing"

	"alt/domain"
	"alt/orchestrator/gateway/recap_gateway"

	"github.com/google/uuid"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetThemeContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	themeID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	userID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	err := mockProvider.
		AddInteraction().
		Given("a theme exists for the user").
		UponReceiving("a request for a theme page").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/themes/" + themeID.String()),
			Query: matchers.MapMatcher{
				"user_id": matchers.Like(userID.String()),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: map[string]interface{}{
				"theme_id":       matchers.Regex("44444444-4444-4444-4444-444444444444", uuidLikePattern),
				"title":          matchers.Like("Systems Architecture"),
				"version":        matchers.Like(2),
				"latest_version": matchers.Like(2),
				"created_at":     matchers.Like("2026-11-18T08:00:00Z"),
				"key_points": matchers.EachLike(map[string]interface{}{
					"text":               matchers.Like("Event logs keep projections rebuildable."),
					"source_article_ids": matchers.EachLike(matchers.Regex("33333333-3333-3333-3333-333333333333", uuidLikePattern), 1),
				}, 1),
				"timeline": matchers.EachLike(map[string]interface{}{
					"date":               matchers.Like("2026-11-15"),
					"text":               matchers.Like("Initial event schema adopted."),
					"source_article_ids": matchers.EachLike(matchers.Regex("33333333-3333-3333-3333-333333333333", uuidLikePattern), 1),
				}, 1),
				"what_changed": matchers.EachLike(map[string]interface{}{
					"text":               matchers.Like("Added versioned projection notes."),
					"source_article_ids": matchers.EachLike(matchers.Regex("33333333-3333-3333-3333-333333333333", uuidLikePattern), 1),
				}, 1),
				"open_questions": matchers.EachLike(map[string]interface{}{
					"text":               matchers.Like("How should backfill windows be bounded?"),
					"source_article_ids": matchers.EachLike(matchers.Regex("33333333-3333-3333-3333-333333333333", uuidLikePattern), 1),
				}, 1),
				"sources": matchers.EachLike(map[string]interface{}{
					"article_id": matchers.Regex("33333333-3333-3333-3333-333333333333", uuidLikePattern),
					"title":      matchers.Like("Event Sourcing Notes"),
					"url":        matchers.Like("https://example.com/event-sourcing"),
				}, 1),
				"versions": matchers.EachLike(map[string]interface{}{
					"version":       matchers.Like(2),
					"created_at":    matchers.Like("2026-11-18T08:00:00Z"),
					"article_count": matchers.Like(5),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetTheme(context.Background(), userID, themeID, nil)
			if err != nil {
				return fmt.Errorf("GetTheme failed: %w", err)
			}
			require.NotNil(t, result)
			assert.Equal(t, themeID.String(), result.ThemeID)
			assert.Equal(t, "Systems Architecture", result.Title)
			assert.Equal(t, int32(2), result.Version)
			assert.Equal(t, int32(2), result.LatestVersion)
			assert.False(t, result.CreatedAt.IsZero())
			require.Len(t, result.KeyPoints, 1)
			require.Len(t, result.Timeline, 1)
			require.Len(t, result.WhatChanged, 1)
			require.Len(t, result.OpenQuestions, 1)
			require.Len(t, result.Sources, 1)
			require.Len(t, result.Versions, 1)
			return nil
		})
	require.NoError(t, err)
}

func TestGetThemeNotFoundContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	missingThemeID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	userID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	err := mockProvider.
		AddInteraction().
		Given("no theme exists for the user").
		UponReceiving("a request for a missing theme page").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/themes/" + missingThemeID.String()),
			Query: matchers.MapMatcher{
				"user_id": matchers.Like(userID.String()),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  404,
			Headers: jsonResponseHeaders(),
			Body: map[string]interface{}{
				"error": matchers.Like("theme not found"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			_, err := gw.GetTheme(context.Background(), userID, missingThemeID, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrThemeNotFound)
			return nil
		})
	require.NoError(t, err)
}
