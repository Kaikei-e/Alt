package datahubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"alt/dataplane/port/internal_article_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/shared/port/event_publisher_port"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// ── Article write operations (pre-processor) ──

func (h *Handler) CheckArticleExists(ctx context.Context, req *connect.Request[datahubv1.CheckArticleExistsRequest]) (*connect.Response[datahubv1.CheckArticleExistsResponse], error) {
	if h.checkArticleExists == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.Url == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}
	if req.Msg.FeedId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_id is required"))
	}

	exists, articleID, err := h.checkArticleExists.CheckArticleExists(ctx, req.Msg.Url, req.Msg.FeedId)
	if err != nil {
		h.logger.Error("CheckArticleExists failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check article existence"))
	}

	return connect.NewResponse(&datahubv1.CheckArticleExistsResponse{
		Exists:    exists,
		ArticleId: articleID,
	}), nil
}

func (h *Handler) CreateArticle(ctx context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
	if h.createArticle == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.Url == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}
	if req.Msg.FeedId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_id is required"))
	}

	// The owner is checked here rather than on the event path below, where an
	// unparseable one used to mean the ArticleCreated event was quietly not
	// built. It is not an event-only concern: articles.user_id is UUID NOT
	// NULL, so a request without a usable one has no row to write either.
	userID, err := uuid.Parse(req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("user_id is required and must be a UUID"))
	}

	// Not every feed item has a pubDate, and published_at is a message field:
	// an omitted one arrives as a nil Timestamp. The absence travels as an
	// absence to the ArticleCreated payload, which is written once and never
	// again — see appendArticleCreated.
	//
	// The upsert and the mq-hub notification below still type it as a
	// non-nullable time.Time, so they get the zero. For the upsert that is the
	// 0001-01-01 that lands in articles.published_at, where NULL is the
	// column's own "unknown" and what the knowledge backfill's
	// COALESCE(published_at, created_at) reads. Carrying it through as a NULL
	// needs CreateArticleParams to hold a *time.Time and the ON CONFLICT to
	// COALESCE it — the port, the gateway and the driver rather than this
	// handler.
	publishedAtOrZero := timeOrZero(req.Msg.PublishedAt)
	var publishedAt *time.Time
	if !publishedAtOrZero.IsZero() {
		publishedAt = &publishedAtOrZero
	}

	articleID, created, err := h.createArticle.CreateArticle(ctx, internal_article_port.CreateArticleParams{
		Title:       req.Msg.Title,
		URL:         req.Msg.Url,
		Content:     req.Msg.Content,
		FeedID:      req.Msg.FeedId,
		UserID:      req.Msg.UserId,
		Language:    req.Msg.Language,
		PublishedAt: publishedAtOrZero,
	})
	if err != nil {
		h.logger.Error("CreateArticle failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create article"))
	}

	if err := h.appendArticleCreated(ctx, articleID, userID, req.Msg, publishedAt); err != nil {
		return nil, err
	}

	// A publisher that reports itself off is a deployment without
	// notifications; a nil one is a composition root that dropped the option,
	// and skipping it would write the article, answer 200, and leave
	// summarisation and indexing with nothing to consume — the two states must
	// not look alike from in here (CLAUDE.md rule 8 / ADR-000928).
	if h.eventPublisher == nil {
		panic("datahubapi: event_publisher_port.EventPublisherPort is nil — " +
			"DataHubService.CreateArticle is where mq-hub learns about an ingested article; " +
			"wire it at the composition root, and wire a disabled publisher to turn it off " +
			"(see .claude/rules/di-wiring.md)")
	}

	if h.eventPublisher.IsEnabled() {
		if created {
			if pubErr := h.eventPublisher.PublishArticleCreated(ctx, event_publisher_port.ArticleCreatedEvent{
				ArticleID:   articleID,
				UserID:      req.Msg.UserId,
				FeedID:      req.Msg.FeedId,
				Title:       req.Msg.Title,
				URL:         req.Msg.Url,
				Content:     req.Msg.Content,
				PublishedAt: publishedAtOrZero,
			}); pubErr != nil {
				h.logger.Warn("failed to publish ArticleCreated event (non-fatal)",
					"article_id", articleID, "error", pubErr)
				recordArticlePublishFailure(ctx, "article_created")
			}
		} else if pubErr := h.eventPublisher.PublishArticleUpdated(ctx, event_publisher_port.ArticleUpdatedEvent{
			ArticleID:   articleID,
			UserID:      req.Msg.UserId,
			FeedID:      req.Msg.FeedId,
			Title:       req.Msg.Title,
			URL:         req.Msg.Url,
			Content:     req.Msg.Content,
			PublishedAt: publishedAtOrZero,
		}); pubErr != nil {
			h.logger.Warn("failed to publish ArticleUpdated event (non-fatal)",
				"article_id", articleID, "error", pubErr)
			recordArticlePublishFailure(ctx, "article_updated")
		}
	}

	return connect.NewResponse(&datahubv1.CreateArticleResponse{
		ArticleId: articleID,
	}), nil
}

// appendArticleCreated appends the Knowledge Home ArticleCreated event for the
// article CreateArticle just wrote. It is the second of the two producers of
// that event; the first is alt-harvester's outbox worker.
//
// It runs on both branches of the upsert. `created == false` says alt-db
// already had the (url, user_id) row, not that sovereign already has the
// event: it is the state every retry of a failed CreateArticle arrives in, and
// the state an unchanged article is re-sent in on every crawl. Skipping it
// there made the first miss permanent, and a Home row whose ArticleCreated
// never landed gets created later by SummaryVersionCreated with a blank title
// and no url. AppendKnowledgeEvent dedupes on DedupeKeyArticleCreated, so the
// repeat is a lookup when sovereign already has the event and a repair when it
// does not — event_seq 0 is a dedupe hit, and a success.
//
// A failed append fails the RPC. Unlike the outbox worker, which after
// 5d553fff withholds the outbox ACK and lets the next tick retry both side
// effects, this path writes no outbox row — the RPC's own result is the only
// acknowledgement there is, so it is what gets withheld. Unavailable rather
// than Internal because the caller's correct response is to send the article
// again, which the upsert and the dedupe key both make safe.
func (h *Handler) appendArticleCreated(
	ctx context.Context,
	articleID string,
	userID uuid.UUID,
	msg *datahubv1.CreateArticleRequest,
	publishedAt *time.Time,
) error {
	if h.knowledgeEventPort == nil {
		panic("datahubapi: knowledge_event_port.AppendKnowledgeEventPort is nil — " +
			"DataHubService.CreateArticle is a Knowledge Home ArticleCreated producer and " +
			"must be wired at the composition root (see .claude/rules/di-wiring.md)")
	}

	// An article whose feed gave no pubDate travels with an empty
	// published_at, not with the formatted zero time. knowledge_events is
	// INSERT-only, so whatever goes in here is what Knowledge Home reads
	// forever, and the read model ranks recency over
	// COALESCE(published_at, generated_at): a 0001-01-01 item scores as two
	// millennia stale and never re-enters a recent or today window. Empty is
	// the payload's "unknown" — the projector folds it to a NULL published_at
	// and the ranking falls back to this event's own generated_at.
	publishedAtWire := ""
	if publishedAt != nil {
		publishedAtWire = publishedAt.Format(time.RFC3339)
	}

	// Canonical wire schema — see domain.ArticleCreatedPayload comment and
	// docs/glossary/ubiquitous-language.md. URL goes under the "url" key; raw
	// map literals here historically wrote "link" (PM-2026-041) and silently
	// broke the projector.
	payload, err := json.Marshal(domain.ArticleCreatedPayload{
		ArticleID:   articleID,
		Title:       msg.Title,
		PublishedAt: publishedAtWire,
		TenantID:    userID.String(),
		URL:         msg.Url,
	})
	if err != nil {
		h.logger.Error("failed to marshal knowledge ArticleCreated payload",
			"article_id", articleID, "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to build ArticleCreated event"))
	}

	kevent := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      userID,
		UserID:        &userID,
		ActorType:     domain.ActorService,
		ActorID:       "pre-processor",
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   articleID,
		DedupeKey:     fmt.Sprintf(domain.DedupeKeyArticleCreated, articleID),
		Payload:       payload,
	}
	if _, err := h.knowledgeEventPort.AppendKnowledgeEvent(ctx, kevent); err != nil {
		h.logger.Error("failed to append knowledge ArticleCreated event; refusing to acknowledge the write",
			"article_id", articleID, "error", err)
		return connect.NewError(connect.CodeUnavailable,
			errors.New("article written but ArticleCreated could not be appended; retry"))
	}
	return nil
}

// ---------------------------------------------------------------------------
// §2.O Automatic full-text fetch groundwork
// ---------------------------------------------------------------------------

func (h *Handler) CheckArticleExistsByURLForUser(ctx context.Context, req *connect.Request[datahubv1.CheckArticleExistsByURLForUserRequest]) (*connect.Response[datahubv1.CheckArticleExistsByURLForUserResponse], error) {
	if req.Msg.GetUrl() == "" || req.Msg.GetUserId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url and user_id are required"))
	}

	exists, articleID, err := h.autoFulltext.CheckArticleExistsByURLForUser(ctx, req.Msg.GetUrl(), req.Msg.GetUserId())
	if err != nil {
		h.logger.ErrorContext(ctx, "CheckArticleExistsByURLForUser failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check article existence"))
	}
	return connect.NewResponse(&datahubv1.CheckArticleExistsByURLForUserResponse{
		Exists:    exists,
		ArticleId: articleID,
	}), nil
}
