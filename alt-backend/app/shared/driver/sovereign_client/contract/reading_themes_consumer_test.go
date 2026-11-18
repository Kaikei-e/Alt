//go:build contract

package contract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
)

func TestListReadingThemesContract(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const userID = "22222222-2222-2222-2222-222222222222"

	err := mockProvider.
		AddInteraction().
		Given("reading themes exist for the user").
		UponReceiving("a ListReadingThemes request for the user").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.sovereign.v1.KnowledgeSovereignService/ListReadingThemes"),
			Headers: sovereignHeaders(),
			Body: matchers.MapMatcher{
				"userId": matchers.Like(userID),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"themes": matchers.EachLike(matchers.MapMatcher{
					"themeId":           matchers.Like("44444444-4444-4444-4444-444444444444"),
					"title":             matchers.Like("Systems Architecture"),
					"latestVersion":     matchers.Integer(2),
					"articleCount":      matchers.Integer(5),
					"createdAt":         matchers.Like("2026-11-17T08:00:00Z"),
					"updatedAt":         matchers.Like("2026-11-18T08:00:00Z"),
					"lastOpenedAt":      matchers.Like("2026-11-18T09:00:00Z"),
					"lastOpenedVersion": matchers.Integer(1),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			resp, err := client.ListReadingThemes(context.Background(), connect.NewRequest(&sovereignv1.ListReadingThemesRequest{
				UserId: userID,
				Limit:  0,
			}))
			if err != nil {
				return fmt.Errorf("ListReadingThemes failed: %w", err)
			}
			require.NotEmpty(t, resp.Msg.Themes)
			theme := resp.Msg.Themes[0]
			assert.NotEmpty(t, theme.ThemeId)
			assert.NotEmpty(t, theme.Title)
			assert.Equal(t, int32(2), theme.LatestVersion)
			assert.Equal(t, int32(5), theme.ArticleCount)
			assert.NotNil(t, theme.CreatedAt)
			assert.NotNil(t, theme.UpdatedAt)
			assert.NotNil(t, theme.LastOpenedAt)
			assert.Equal(t, int32(1), theme.LastOpenedVersion)
			return nil
		})
	require.NoError(t, err)
}

func TestAppendKnowledgeEvent_ReadingThemeOpened(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const (
		tenantID      = "11111111-1111-1111-1111-111111111111"
		userID        = "22222222-2222-2222-2222-222222222222"
		themeID       = "44444444-4444-4444-4444-444444444444"
		eventID       = "019a0000-0007-7000-8000-000000000007"
		clientEventID = "019a0000-0007-7000-8000-000000000107"
	)
	occurredAt := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	dedupeKey := "reading.theme_opened:" + clientEventID

	payload, err := json.Marshal(map[string]any{
		"theme_id":  themeID,
		"version":   2,
		"opened_at": "2026-11-18T09:00:00Z",
	})
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(payload)

	err = mockProvider.
		AddInteraction().
		Given("alt-backend may append theme opened events").
		UponReceiving("an AppendKnowledgeEvent request for reading.theme_opened.v1").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: sovereignHeaders(),
			Body: matchers.MapMatcher{
				"event": matchers.StructMatcher{
					"eventId":       matchers.Like(eventID),
					"occurredAt":    matchers.Like("2026-11-18T09:00:00Z"),
					"tenantId":      matchers.Like(tenantID),
					"userId":        matchers.Like(userID),
					"actorType":     matchers.String("user"),
					"actorId":       matchers.Like(userID),
					"eventType":     matchers.String("reading.theme_opened.v1"),
					"aggregateType": matchers.String("reading_theme"),
					"aggregateId":   matchers.Like(themeID),
					"dedupeKey":     matchers.Like(dedupeKey),
					"payload":       matchers.Like(encoded),
				},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"eventSeq": matchers.Like("123"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			_, err := client.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
				Event: &sovereignv1.KnowledgeEvent{
					EventId:       eventID,
					OccurredAt:    timestamppb.New(occurredAt),
					TenantId:      tenantID,
					UserId:        userID,
					ActorType:     "user",
					ActorId:       userID,
					EventType:     "reading.theme_opened.v1",
					AggregateType: "reading_theme",
					AggregateId:   themeID,
					DedupeKey:     dedupeKey,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}
