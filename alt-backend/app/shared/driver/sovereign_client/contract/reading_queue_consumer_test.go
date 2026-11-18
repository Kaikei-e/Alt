//go:build contract

package contract

import (
	"context"
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

func TestGetReadingQueueContract(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const userID = "22222222-2222-2222-2222-222222222222"
	asOf := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	err := mockProvider.
		AddInteraction().
		Given("a user has active reading queue items").
		UponReceiving("a GetReadingQueue request for the user as of a timestamp").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.sovereign.v1.KnowledgeSovereignService/GetReadingQueue"),
			Headers: sovereignHeaders(),
			Body: matchers.MapMatcher{
				"userId": matchers.Like(userID),
				"asOf":   matchers.Like("2026-11-18T09:00:00Z"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"items": matchers.EachLike(matchers.MapMatcher{
					"articleId":          matchers.Like("33333333-3333-3333-3333-333333333333"),
					"queuedAt":           matchers.Like("2026-11-18T08:00:00Z"),
					"expiresAt":          matchers.Like("2026-11-20T08:00:00Z"),
					"queuePolicyVersion": matchers.Integer(1),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			resp, err := client.GetReadingQueue(context.Background(), connect.NewRequest(&sovereignv1.GetReadingQueueRequest{
				UserId: userID,
				AsOf:   timestamppb.New(asOf),
			}))
			if err != nil {
				return fmt.Errorf("GetReadingQueue failed: %w", err)
			}
			require.NotEmpty(t, resp.Msg.Items)
			assert.NotEmpty(t, resp.Msg.Items[0].ArticleId)
			assert.NotNil(t, resp.Msg.Items[0].QueuedAt)
			assert.NotNil(t, resp.Msg.Items[0].ExpiresAt)
			assert.Equal(t, int32(1), resp.Msg.Items[0].QueuePolicyVersion)
			return nil
		})
	require.NoError(t, err)
}

func TestCountReadingSourcesSinceContract(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const userID = "22222222-2222-2222-2222-222222222222"
	since := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

	err := mockProvider.
		AddInteraction().
		Given("a user has reading sources recorded since midnight").
		UponReceiving("a CountReadingSourcesSince request for the user").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.sovereign.v1.KnowledgeSovereignService/CountReadingSourcesSince"),
			Headers: sovereignHeaders(),
			Body: matchers.MapMatcher{
				"userId": matchers.Like(userID),
				"since":  matchers.Like("2026-11-18T00:00:00Z"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"count": matchers.Integer(3),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			resp, err := client.CountReadingSourcesSince(context.Background(), connect.NewRequest(&sovereignv1.CountReadingSourcesSinceRequest{
				UserId: userID,
				Since:  timestamppb.New(since),
			}))
			if err != nil {
				return fmt.Errorf("CountReadingSourcesSince failed: %w", err)
			}
			assert.Equal(t, int32(3), resp.Msg.Count)
			return nil
		})
	require.NoError(t, err)
}
