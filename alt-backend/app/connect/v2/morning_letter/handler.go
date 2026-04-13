package morning_letter

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/connect/errorhandler"
	"alt/domain"
	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
	"alt/port/morning_letter_port"
)

// Handler implements both MorningLetterServiceHandler (StreamChat) and
// MorningLetterReadServiceHandler (GetLatestLetter, GetLetterByDate, GetLetterSources).
type Handler struct {
	streamChat      morning_letter_port.StreamChatPort
	morningLetterUC morning_letter_port.MorningLetterUsecase
	logger          *slog.Logger
}

// Ensure Handler implements both interfaces
var _ morningletterv2connect.MorningLetterServiceHandler = (*Handler)(nil)
var _ morningletterv2connect.MorningLetterReadServiceHandler = (*Handler)(nil)

// NewHandler creates a new MorningLetterService + MorningLetterReadService handler
func NewHandler(
	streamChat morning_letter_port.StreamChatPort,
	morningLetterUC morning_letter_port.MorningLetterUsecase,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		streamChat:      streamChat,
		morningLetterUC: morningLetterUC,
		logger:          logger,
	}
}

// StreamChat proxies streaming chat requests to rag-orchestrator (unchanged)
func (h *Handler) StreamChat(
	ctx context.Context,
	req *connect.Request[morningletterv2.StreamChatRequest],
	stream *connect.ServerStream[morningletterv2.StreamChatResponse],
) error {
	_, err := domain.GetUserFromContext(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "authentication failed", slog.String("error", err.Error()))
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if len(req.Msg.Messages) == 0 {
		h.logger.WarnContext(ctx, "no messages in request")
		return connect.NewError(connect.CodeInvalidArgument, nil)
	}

	withinHours := req.Msg.WithinHours
	if withinHours <= 0 {
		withinHours = 24
	}
	if withinHours > 168 {
		withinHours = 168
	}

	h.logger.InfoContext(ctx, "proxying MorningLetter.StreamChat to rag-orchestrator",
		slog.Int("message_count", len(req.Msg.Messages)),
		slog.Int("within_hours", int(withinHours)))

	upstreamStream, err := h.streamChat.StreamChat(ctx, req.Msg.Messages, withinHours)
	if err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.ConnectUpstream")
	}
	defer func() {
		if closeErr := upstreamStream.Close(); closeErr != nil {
			h.logger.DebugContext(ctx, "failed to close upstream stream", slog.String("error", closeErr.Error()))
		}
	}()

	eventCount := 0
	for upstreamStream.Receive() {
		event := upstreamStream.Msg()
		if err := stream.Send(event); err != nil {
			return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.SendEvent")
		}
		eventCount++
	}

	if err := upstreamStream.Err(); err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamChat.UpstreamError")
	}

	h.logger.InfoContext(ctx, "MorningLetter.StreamChat completed",
		slog.Int("events_sent", eventCount))

	return nil
}

// =============================================================================
// MorningLetterReadService methods
// =============================================================================

func (h *Handler) GetLatestLetter(
	ctx context.Context,
	req *connect.Request[morningletterv2.GetLatestLetterRequest],
) (*connect.Response[morningletterv2.GetLatestLetterResponse], error) {
	if _, err := domain.GetUserFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	doc, err := h.morningLetterUC.GetLatestLetter(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetLatestLetter")
	}
	if doc == nil {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}

	return connect.NewResponse(&morningletterv2.GetLatestLetterResponse{
		Letter: domainToProto(doc),
	}), nil
}

func (h *Handler) GetLetterByDate(
	ctx context.Context,
	req *connect.Request[morningletterv2.GetLetterByDateRequest],
) (*connect.Response[morningletterv2.GetLetterByDateResponse], error) {
	if _, err := domain.GetUserFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.TargetDate == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	doc, err := h.morningLetterUC.GetLetterByDate(ctx, req.Msg.TargetDate)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetLetterByDate")
	}
	if doc == nil {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}

	return connect.NewResponse(&morningletterv2.GetLetterByDateResponse{
		Letter: domainToProto(doc),
	}), nil
}

func (h *Handler) GetLetterEnrichment(
	ctx context.Context,
	req *connect.Request[morningletterv2.GetLetterEnrichmentRequest],
) (*connect.Response[morningletterv2.GetLetterEnrichmentResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.LetterId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	enrichments, err := h.morningLetterUC.GetLetterEnrichment(
		ctx, req.Msg.LetterId, user.UserID.String(),
	)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetLetterEnrichment")
	}

	out := make([]*morningletterv2.MorningLetterBulletEnrichment, len(enrichments))
	for i, e := range enrichments {
		related := make([]*morningletterv2.RelatedArticleTeaser, len(e.RelatedArticles))
		for j, r := range e.RelatedArticles {
			related[j] = &morningletterv2.RelatedArticleTeaser{
				ArticleId:      r.ArticleID,
				Title:          r.Title,
				ArticleAltHref: r.ArticleAltHref,
				FeedTitle:      r.FeedTitle,
			}
		}
		out[i] = &morningletterv2.MorningLetterBulletEnrichment{
			SectionKey:      e.SectionKey,
			ArticleId:       e.ArticleID,
			ArticleTitle:    e.ArticleTitle,
			ArticleUrl:      e.ArticleURL,
			ArticleAltHref:  e.ArticleAltHref,
			FeedTitle:       e.FeedTitle,
			Tags:            e.Tags,
			RelatedArticles: related,
			SummaryExcerpt:  e.SummaryExcerpt,
			AcolyteHref:     e.AcolyteHref,
		}
	}
	return connect.NewResponse(&morningletterv2.GetLetterEnrichmentResponse{
		Enrichments: out,
	}), nil
}

func (h *Handler) RegenerateLatest(
	ctx context.Context,
	req *connect.Request[morningletterv2.RegenerateLatestRequest],
) (*connect.Response[morningletterv2.RegenerateLatestResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	tz := ""
	if req.Msg.EditionTimezone != nil {
		tz = *req.Msg.EditionTimezone
	}

	doc, regenerated, retryAfter, err := h.morningLetterUC.RegenerateLatest(ctx, user.UserID.String(), tz)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "RegenerateLatest")
	}
	if doc == nil {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}

	return connect.NewResponse(&morningletterv2.RegenerateLatestResponse{
		Letter:            domainToProto(doc),
		Regenerated:       regenerated,
		RetryAfterSeconds: int32(retryAfter.Seconds()),
	}), nil
}

func (h *Handler) GetLetterSources(
	ctx context.Context,
	req *connect.Request[morningletterv2.GetLetterSourcesRequest],
) (*connect.Response[morningletterv2.GetLetterSourcesResponse], error) {
	if _, err := domain.GetUserFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.LetterId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	sources, err := h.morningLetterUC.GetLetterSources(ctx, req.Msg.LetterId)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetLetterSources")
	}

	protoSources := make([]*morningletterv2.MorningLetterSourceProto, len(sources))
	for i, s := range sources {
		protoSources[i] = &morningletterv2.MorningLetterSourceProto{
			LetterId:   s.LetterID,
			SectionKey: s.SectionKey,
			ArticleId:  s.ArticleID.String(),
			SourceType: mapSourceType(s.SourceType),
			Position:   int32(s.Position),
		}
	}

	return connect.NewResponse(&morningletterv2.GetLetterSourcesResponse{
		Sources: protoSources,
	}), nil
}

// =============================================================================
// Mapping helpers
// =============================================================================

func domainToProto(doc *domain.MorningLetterDocument) *morningletterv2.MorningLetterDocument {
	sections := make([]*morningletterv2.MorningLetterSection, len(doc.Body.Sections))
	for i, s := range doc.Body.Sections {
		var genre *string
		if s.Genre != "" {
			g := s.Genre
			genre = &g
		}
		var narrative *string
		if s.Narrative != "" {
			n := s.Narrative
			narrative = &n
		}
		whys := make([]*morningletterv2.WhyReason, len(s.WhyReasons))
		for j, w := range s.WhyReasons {
			var refID, tag *string
			if w.RefID != "" {
				r := w.RefID
				refID = &r
			}
			if w.Tag != "" {
				t := w.Tag
				tag = &t
			}
			whys[j] = &morningletterv2.WhyReason{
				Code:  w.Code,
				RefId: refID,
				Tag:   tag,
			}
		}
		sections[i] = &morningletterv2.MorningLetterSection{
			Key:        s.Key,
			Title:      s.Title,
			Bullets:    s.Bullets,
			Genre:      genre,
			Narrative:  narrative,
			WhyReasons: whys,
		}
	}

	var windowDays *int32
	if doc.Body.SourceRecapWindowDays != nil {
		d := int32(*doc.Body.SourceRecapWindowDays)
		windowDays = &d
	}

	var throughLine *string
	if doc.Body.ThroughLine != "" {
		tl := doc.Body.ThroughLine
		throughLine = &tl
	}

	var prev *morningletterv2.PreviousLetterRef
	if p := doc.Body.PreviousLetterRef; p != nil {
		prev = &morningletterv2.PreviousLetterRef{
			Id:          p.ID,
			TargetDate:  p.TargetDate,
			ThroughLine: p.ThroughLine,
		}
	}

	return &morningletterv2.MorningLetterDocument{
		Id:                 doc.ID,
		TargetDate:         doc.TargetDate,
		EditionTimezone:    doc.EditionTimezone,
		IsDegraded:         doc.IsDegraded,
		SchemaVersion:      int32(doc.SchemaVersion),
		GenerationRevision: int32(doc.GenerationRevision),
		Model:              doc.Model,
		CreatedAt:          timestamppb.New(doc.CreatedAt),
		Etag:               doc.Etag,
		Body: &morningletterv2.MorningLetterBody{
			Lead:                  doc.Body.Lead,
			Sections:              sections,
			GeneratedAt:           timestamppb.New(doc.Body.GeneratedAt),
			SourceRecapWindowDays: windowDays,
			ThroughLine:           throughLine,
			PreviousLetterRef:     prev,
		},
	}
}

func mapSourceType(st string) morningletterv2.MorningLetterSourceType {
	switch st {
	case "recap":
		return morningletterv2.MorningLetterSourceType_MORNING_LETTER_SOURCE_TYPE_RECAP
	case "overnight":
		return morningletterv2.MorningLetterSourceType_MORNING_LETTER_SOURCE_TYPE_OVERNIGHT
	default:
		return morningletterv2.MorningLetterSourceType_MORNING_LETTER_SOURCE_TYPE_UNSPECIFIED
	}
}
