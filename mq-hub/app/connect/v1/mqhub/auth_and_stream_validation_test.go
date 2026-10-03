package mqhub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"mq-hub/domain"
	mqhubv1 "mq-hub/gen/proto/services/mqhub/v1"
	"mq-hub/gen/proto/services/mqhub/v1/mqhubv1connect"
	"mq-hub/usecase"
)

func TestC03_MQHub_AuthInterceptor_EveryBusinessRPC(t *testing.T) {
	validToken := "mqhub-secret-test-token-safe-ascii"
	mockPort := new(MockStreamPort)
	uc := usecase.NewPublishUsecase(mockPort)
	genUc := usecase.NewGenerateTagsUsecase(mockPort)
	handler := NewHandlerWithGenerateTags(uc, genUc)

	mux := http.NewServeMux()
	path, h := mqhubv1connect.NewMQHubServiceHandler(handler, connect.WithInterceptors(NewAuthInterceptor(validToken)))
	mux.Handle(path, h)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx := context.Background()

	businessRPCs := []struct {
		name       string
		methodName string
		invoke     func(client mqhubv1connect.MQHubServiceClient, authHeader string) error
	}{
		{
			name:       "Publish",
			methodName: "Publish",
			invoke: func(client mqhubv1connect.MQHubServiceClient, authHeader string) error {
				req := connect.NewRequest(&mqhubv1.PublishRequest{
					Stream: string(domain.StreamKeyArticles),
					Event: &mqhubv1.Event{
						EventId:   "evt-1",
						EventType: "ArticleCreated",
						Source:    "alt-backend",
					},
				})
				if authHeader != "" {
					req.Header().Set("Authorization", authHeader)
				}
				_, err := client.Publish(ctx, req)
				return err
			},
		},
		{
			name:       "PublishBatch",
			methodName: "PublishBatch",
			invoke: func(client mqhubv1connect.MQHubServiceClient, authHeader string) error {
				req := connect.NewRequest(&mqhubv1.PublishBatchRequest{
					Stream: string(domain.StreamKeyArticles),
					Events: []*mqhubv1.Event{
						{
							EventId:   "evt-1",
							EventType: "ArticleCreated",
							Source:    "alt-backend",
						},
					},
				})
				if authHeader != "" {
					req.Header().Set("Authorization", authHeader)
				}
				_, err := client.PublishBatch(ctx, req)
				return err
			},
		},
		{
			name:       "CreateConsumerGroup",
			methodName: "CreateConsumerGroup",
			invoke: func(client mqhubv1connect.MQHubServiceClient, authHeader string) error {
				req := connect.NewRequest(&mqhubv1.CreateConsumerGroupRequest{
					Stream:  string(domain.StreamKeyArticles),
					Group:   string(domain.ConsumerGroupPreProcessor),
					StartId: "0",
				})
				if authHeader != "" {
					req.Header().Set("Authorization", authHeader)
				}
				_, err := client.CreateConsumerGroup(ctx, req)
				return err
			},
		},
		{
			name:       "GetStreamInfo",
			methodName: "GetStreamInfo",
			invoke: func(client mqhubv1connect.MQHubServiceClient, authHeader string) error {
				req := connect.NewRequest(&mqhubv1.GetStreamInfoRequest{
					Stream: string(domain.StreamKeyArticles),
				})
				if authHeader != "" {
					req.Header().Set("Authorization", authHeader)
				}
				_, err := client.GetStreamInfo(ctx, req)
				return err
			},
		},
		{
			name:       "GenerateTagsForArticle",
			methodName: "GenerateTagsForArticle",
			invoke: func(client mqhubv1connect.MQHubServiceClient, authHeader string) error {
				req := connect.NewRequest(&mqhubv1.GenerateTagsForArticleRequest{
					ArticleId: "art-1",
					Title:     "Title",
					Content:   "Content",
				})
				if authHeader != "" {
					req.Header().Set("Authorization", authHeader)
				}
				_, err := client.GenerateTagsForArticle(ctx, req)
				return err
			},
		},
	}

	for _, tc := range businessRPCs {
		t.Run(tc.name+" unauthenticated request is denied before driver", func(t *testing.T) {
			client := mqhubv1connect.NewMQHubServiceClient(ts.Client(), ts.URL)
			err := tc.invoke(client, "")
			require.Error(t, err)
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
			mockPort.AssertNotCalled(t, tc.methodName)
		})

		t.Run(tc.name+" wrong token is denied before driver", func(t *testing.T) {
			client := mqhubv1connect.NewMQHubServiceClient(ts.Client(), ts.URL)
			err := tc.invoke(client, "Bearer wrong-token")
			require.Error(t, err)
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
			mockPort.AssertNotCalled(t, tc.methodName)
		})
	}

	t.Run("health check RPC is exempt from auth and succeeds without credentials", func(t *testing.T) {
		mockPort.On("Ping", mock.Anything).Return(nil).Once()
		client := mqhubv1connect.NewMQHubServiceClient(ts.Client(), ts.URL)
		req := connect.NewRequest(&mqhubv1.HealthCheckRequest{})
		resp, err := client.HealthCheck(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Msg.Healthy)
		mockPort.AssertExpectations(t)
	})

	t.Run("valid bearer token allows business RPC execution", func(t *testing.T) {
		mockPort.On("Publish", mock.Anything, domain.StreamKeyArticles, mock.AnythingOfType("*domain.Event")).
			Return("1000-0", nil).Once()

		client := mqhubv1connect.NewMQHubServiceClient(ts.Client(), ts.URL)
		req := connect.NewRequest(&mqhubv1.PublishRequest{
			Stream: string(domain.StreamKeyArticles),
			Event: &mqhubv1.Event{
				EventId:   "evt-1",
				EventType: "ArticleCreated",
				Source:    "alt-backend",
				CreatedAt: timestamppb.New(time.Now()),
				Payload:   []byte(`{"article_id":"123"}`),
			},
		})
		req.Header().Set("Authorization", "Bearer "+validToken)
		resp, err := client.Publish(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Msg.Success)
		assert.Equal(t, "1000-0", resp.Msg.MessageId)
		mockPort.AssertExpectations(t)
	})
}

func TestC03_StreamValidation_ZeroRedisWrites(t *testing.T) {
	mockPort := new(MockStreamPort)
	uc := usecase.NewPublishUsecase(mockPort)
	handler := NewHandler(uc)
	ctx := context.Background()

	t.Run("Publish with invalid stream returns InvalidArgument with zero writes", func(t *testing.T) {
		req := connect.NewRequest(&mqhubv1.PublishRequest{
			Stream: "malicious:stream:bypass",
			Event: &mqhubv1.Event{
				EventId:   "evt-1",
				EventType: "ArticleCreated",
				Source:    "alt-backend",
			},
		})
		_, err := handler.Publish(ctx, req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "invalid stream key")
		mockPort.AssertNotCalled(t, "Publish")
	})

	t.Run("PublishBatch with invalid stream returns InvalidArgument with zero writes", func(t *testing.T) {
		req := connect.NewRequest(&mqhubv1.PublishBatchRequest{
			Stream: "invalid:stream",
			Events: []*mqhubv1.Event{
				{
					EventId:   "evt-1",
					EventType: "ArticleCreated",
					Source:    "alt-backend",
				},
			},
		})
		_, err := handler.PublishBatch(ctx, req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "invalid stream key")
		mockPort.AssertNotCalled(t, "PublishBatch")
	})

	t.Run("CreateConsumerGroup with invalid stream returns InvalidArgument and prevents MKSTREAM bypass", func(t *testing.T) {
		req := connect.NewRequest(&mqhubv1.CreateConsumerGroupRequest{
			Stream:  "unauthorized:stream:attempt",
			Group:   string(domain.ConsumerGroupPreProcessor),
			StartId: "0",
		})
		_, err := handler.CreateConsumerGroup(ctx, req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "invalid stream key")
		mockPort.AssertNotCalled(t, "CreateConsumerGroup")
	})

	t.Run("CreateConsumerGroup with invalid consumer group returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&mqhubv1.CreateConsumerGroupRequest{
			Stream:  string(domain.StreamKeyArticles),
			Group:   "attacker-group",
			StartId: "0",
		})
		_, err := handler.CreateConsumerGroup(ctx, req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "invalid consumer group")
		mockPort.AssertNotCalled(t, "CreateConsumerGroup")
	})

	t.Run("GetStreamInfo with invalid stream returns InvalidArgument with zero writes", func(t *testing.T) {
		req := connect.NewRequest(&mqhubv1.GetStreamInfoRequest{
			Stream: "malicious:stream:bypass",
		})
		_, err := handler.GetStreamInfo(ctx, req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "invalid stream key")
		mockPort.AssertNotCalled(t, "GetStreamInfo")
	})

	t.Run("All 5 canonical public streams including DLQ are accepted", func(t *testing.T) {
		canonicalStreams := []domain.StreamKey{
			domain.StreamKeyArticles,
			domain.StreamKeySummaries,
			domain.StreamKeyTags,
			domain.StreamKeyIndex,
			domain.StreamKeyArticlesDLQ,
		}

		for _, s := range canonicalStreams {
			mockPort.On("Publish", ctx, s, mock.AnythingOfType("*domain.Event")).
				Return("123-0", nil).Once()

			req := connect.NewRequest(&mqhubv1.PublishRequest{
				Stream: string(s),
				Event: &mqhubv1.Event{
					EventId:   "test-canonical",
					EventType: "ArticleCreated",
					Source:    "alt-backend",
				},
			})

			resp, err := handler.Publish(ctx, req)
			require.NoError(t, err, "Stream %s should be valid", s)
			assert.True(t, resp.Msg.Success)
		}
		mockPort.AssertExpectations(t)
	})
}
