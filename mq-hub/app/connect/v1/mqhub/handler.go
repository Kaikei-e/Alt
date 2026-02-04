// Package mqhub provides the Connect-RPC handler for mq-hub service.
package mqhub

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"mq-hub/domain"
	mqhubv1 "mq-hub/gen/proto/services/mqhub/v1"
	"mq-hub/usecase"
)

// Handler implements the MQHubService Connect-RPC interface.
type Handler struct {
	publishUsecase      *usecase.PublishUsecase
	generateTagsUsecase *usecase.GenerateTagsUsecase
}

// NewHandler creates a new Handler.
func NewHandler(publishUsecase *usecase.PublishUsecase) *Handler {
	return &Handler{publishUsecase: publishUsecase}
}

// NewHandlerWithGenerateTags creates a new Handler with tag generation support.
func NewHandlerWithGenerateTags(publishUsecase *usecase.PublishUsecase, generateTagsUsecase *usecase.GenerateTagsUsecase) *Handler {
	return &Handler{
		publishUsecase:      publishUsecase,
		generateTagsUsecase: generateTagsUsecase,
	}
}

// Publish sends a single event to a Redis Stream.
func (h *Handler) Publish(ctx context.Context, req *connect.Request[mqhubv1.PublishRequest]) (*connect.Response[mqhubv1.PublishResponse], error) {
	protoEvent := req.Msg.Event
	if protoEvent == nil {
		return connect.NewResponse(&mqhubv1.PublishResponse{
			Success: false,
		}), connect.NewError(connect.CodeInvalidArgument, nil)
	}

	event := protoEventToDomain(protoEvent)

	result, err := h.publishUsecase.Publish(ctx, domain.StreamKey(req.Msg.Stream), event)
	if err != nil {
		return connect.NewResponse(&mqhubv1.PublishResponse{
			Success: false,
		}), err
	}

	return connect.NewResponse(&mqhubv1.PublishResponse{
		MessageId: result.MessageID,
		Success:   result.Success,
	}), nil
}

// PublishBatch sends multiple events to a Redis Stream.
func (h *Handler) PublishBatch(ctx context.Context, req *connect.Request[mqhubv1.PublishBatchRequest]) (*connect.Response[mqhubv1.PublishBatchResponse], error) {
	events := make([]*domain.Event, len(req.Msg.Events))
	for i, protoEvent := range req.Msg.Events {
		events[i] = protoEventToDomain(protoEvent)
	}

	result, err := h.publishUsecase.PublishBatch(ctx, domain.StreamKey(req.Msg.Stream), events)
	if err != nil {
		return connect.NewResponse(&mqhubv1.PublishBatchResponse{
			SuccessCount: 0,
			FailureCount: int32(len(events)),
		}), err
	}

	protoErrors := make([]*mqhubv1.PublishError, len(result.Errors))
	for i, e := range result.Errors {
		protoErrors[i] = &mqhubv1.PublishError{
			Index:        int32(e.Index),
			ErrorMessage: e.ErrorMessage,
		}
	}

	return connect.NewResponse(&mqhubv1.PublishBatchResponse{
		MessageIds:   result.MessageIDs,
		SuccessCount: result.SuccessCount,
		FailureCount: result.FailureCount,
		Errors:       protoErrors,
	}), nil
}

// CreateConsumerGroup creates a consumer group for a stream.
func (h *Handler) CreateConsumerGroup(ctx context.Context, req *connect.Request[mqhubv1.CreateConsumerGroupRequest]) (*connect.Response[mqhubv1.CreateConsumerGroupResponse], error) {
	err := h.publishUsecase.CreateConsumerGroup(
		ctx,
		domain.StreamKey(req.Msg.Stream),
		domain.ConsumerGroup(req.Msg.Group),
		req.Msg.StartId,
	)
	if err != nil {
		return connect.NewResponse(&mqhubv1.CreateConsumerGroupResponse{
			Success: false,
			Message: err.Error(),
		}), err
	}

	return connect.NewResponse(&mqhubv1.CreateConsumerGroupResponse{
		Success: true,
		Message: "consumer group created",
	}), nil
}

// GetStreamInfo returns information about a stream.
func (h *Handler) GetStreamInfo(ctx context.Context, req *connect.Request[mqhubv1.StreamInfoRequest]) (*connect.Response[mqhubv1.StreamInfoResponse], error) {
	info, err := h.publishUsecase.GetStreamInfo(ctx, domain.StreamKey(req.Msg.Stream))
	if err != nil {
		return nil, err
	}

	groups := make([]*mqhubv1.ConsumerGroupInfo, len(info.Groups))
	for i, g := range info.Groups {
		groups[i] = &mqhubv1.ConsumerGroupInfo{
			Name:            g.Name,
			Consumers:       g.Consumers,
			Pending:         g.Pending,
			LastDeliveredId: g.LastDeliveredID,
		}
	}

	return connect.NewResponse(&mqhubv1.StreamInfoResponse{
		Length:         info.Length,
		RadixTreeKeys:  info.RadixTreeKeys,
		RadixTreeNodes: info.RadixTreeNodes,
		FirstEntryId:   info.FirstEntryID,
		LastEntryId:    info.LastEntryID,
		Groups:         groups,
	}), nil
}

// HealthCheck checks the health of the service.
func (h *Handler) HealthCheck(ctx context.Context, req *connect.Request[mqhubv1.HealthCheckRequest]) (*connect.Response[mqhubv1.HealthCheckResponse], error) {
	health := h.publishUsecase.HealthCheck(ctx)

	return connect.NewResponse(&mqhubv1.HealthCheckResponse{
		Healthy:       health.Healthy,
		RedisStatus:   health.RedisStatus,
		UptimeSeconds: health.UptimeSeconds,
	}), nil
}

// protoEventToDomain converts a proto Event to a domain Event.
func protoEventToDomain(protoEvent *mqhubv1.Event) *domain.Event {
	if protoEvent == nil {
		return nil
	}

	createdAt := time.Now()
	if protoEvent.CreatedAt != nil {
		createdAt = protoEvent.CreatedAt.AsTime()
	}

	return &domain.Event{
		EventID:   protoEvent.EventId,
		EventType: domain.EventType(protoEvent.EventType),
		Source:    protoEvent.Source,
		CreatedAt: createdAt,
		Payload:   protoEvent.Payload,
		Metadata:  protoEvent.Metadata,
	}
}

// GenerateTagsForArticle synchronously generates tags for an article.
func (h *Handler) GenerateTagsForArticle(ctx context.Context, req *connect.Request[mqhubv1.GenerateTagsRequest]) (*connect.Response[mqhubv1.GenerateTagsResponse], error) {
	if h.generateTagsUsecase == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, nil)
	}

	ucReq := &usecase.GenerateTagsRequest{
		ArticleID: req.Msg.ArticleId,
		Title:     req.Msg.Title,
		Content:   req.Msg.Content,
		FeedID:    req.Msg.FeedId,
		TimeoutMs: req.Msg.TimeoutMs,
	}

	result, err := h.generateTagsUsecase.GenerateTagsForArticle(ctx, ucReq)
	if err != nil {
		return connect.NewResponse(&mqhubv1.GenerateTagsResponse{
			Success:      false,
			ArticleId:    req.Msg.ArticleId,
			ErrorMessage: err.Error(),
		}), err
	}

	// Convert tags to proto format
	protoTags := make([]*mqhubv1.GeneratedTag, len(result.Tags))
	for i, t := range result.Tags {
		protoTags[i] = &mqhubv1.GeneratedTag{
			Id:         t.ID,
			Name:       t.Name,
			Confidence: t.Confidence,
		}
	}

	return connect.NewResponse(&mqhubv1.GenerateTagsResponse{
		Success:      result.Success,
		ArticleId:    result.ArticleID,
		Tags:         protoTags,
		InferenceMs:  result.InferenceMs,
		ErrorMessage: result.ErrorMessage,
	}), nil
}

// domainEventToProto converts a domain Event to a proto Event.
func domainEventToProto(event *domain.Event) *mqhubv1.Event {
	if event == nil {
		return nil
	}

	return &mqhubv1.Event{
		EventId:   event.EventID,
		EventType: string(event.EventType),
		Source:    event.Source,
		CreatedAt: timestamppb.New(event.CreatedAt),
		Payload:   event.Payload,
		Metadata:  event.Metadata,
	}
}
