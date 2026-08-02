package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/dataplane/usecase/outbox_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WithWave3Capabilities wires the capabilities ADR-000954 Wave 3 moved out of
// alt-backend and alt-harvester (catalog §2.A / §2.D / §2.E / §2.L / §2.O).
//
// Every argument is required and a nil one panics, which is a departure from
// the WithPhaseN options above. Those wire procedures whose consumers are
// separate services that may or may not be deployed; these wire the only route
// two binaries in this repository have to their own database. A data hub that
// started with a nil outbox port would answer ClaimOutboxBatch with
// Unimplemented — the same answer a genuinely retired procedure gives — and
// alt-harvester would tick every five seconds, log nothing unusual, and
// deliver no article to rag-orchestrator until someone noticed the search
// index had stopped moving (CLAUDE.md rule 8, ADR-000928).
func WithWave3Capabilities(
	outbox *outbox_usecase.OutboxUsecase,
	ogImage datahub_capability_port.OgImagePort,
	imageProxyCache datahub_capability_port.ImageProxyCachePort,
	scrapingPolicy datahub_capability_port.ScrapingPolicyPort,
	autoFulltext datahub_capability_port.AutoFulltextPort,
) HandlerOption {
	switch {
	case outbox == nil:
		panic("datahubapi: OutboxUsecase is required — alt-harvester's outbox worker has no other route to outbox_events")
	case ogImage == nil:
		panic("datahubapi: OgImagePort is required — the OG image pipeline has no other route to article_heads")
	case imageProxyCache == nil:
		panic("datahubapi: ImageProxyCachePort is required — the image proxy has no other route to image_proxy_cache")
	case scrapingPolicy == nil:
		panic("datahubapi: ScrapingPolicyPort is required — the article fetch path checks scraping_domains before every body fetch")
	case autoFulltext == nil:
		panic("datahubapi: AutoFulltextPort is required")
	}

	return func(h *Handler) {
		h.outboxUsecase = outbox
		h.ogImage = ogImage
		h.imageProxyCache = imageProxyCache
		h.scrapingPolicy = scrapingPolicy
		h.autoFulltext = autoFulltext
	}
}

// ---------------------------------------------------------------------------
// §2.A Outbox
// ---------------------------------------------------------------------------

// ClaimOutboxBatch takes ownership of pending events.
//
// The response reports the events as PROCESSING because that is what they are
// by the time the transaction commits. Reporting the status the rows had when
// the SELECT matched them would describe a state no other caller can observe.
func (h *Handler) ClaimOutboxBatch(ctx context.Context, req *connect.Request[datahubv1.ClaimOutboxBatchRequest]) (*connect.Response[datahubv1.ClaimOutboxBatchResponse], error) {
	events, err := h.outboxUsecase.ClaimBatch(ctx, int(req.Msg.GetLimit()))
	if err != nil {
		h.logger.ErrorContext(ctx, "ClaimOutboxBatch failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to claim outbox batch"))
	}

	out := make([]*datahubv1.OutboxEvent, 0, len(events))
	for _, e := range events {
		out = append(out, &datahubv1.OutboxEvent{
			Id:        e.ID,
			EventType: e.EventType,
			Payload:   e.Payload,
			Status:    outboxStatusToProto(e.Status),
			CreatedAt: timestampOrNil(e.CreatedAt),
		})
	}
	return connect.NewResponse(&datahubv1.ClaimOutboxBatchResponse{Events: out}), nil
}

// MarkOutboxProcessed records a terminal outcome.
//
// A non-terminal status is InvalidArgument, not a quiet write: the caller
// asked for a transition this procedure does not own, and answering OK would
// leave the row in whatever state it was already in while the caller believed
// it had moved.
func (h *Handler) MarkOutboxProcessed(ctx context.Context, req *connect.Request[datahubv1.MarkOutboxProcessedRequest]) (*connect.Response[datahubv1.MarkOutboxProcessedResponse], error) {
	status := outboxStatusFromProto(req.Msg.GetStatus())

	err := h.outboxUsecase.MarkProcessed(ctx, req.Msg.GetId(), status, req.Msg.GetErrorMessage())
	switch {
	case errors.Is(err, outbox_usecase.ErrNotTerminalStatus), errors.Is(err, outbox_usecase.ErrMissingID):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "MarkOutboxProcessed failed", "error", err, "event_id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to mark outbox event processed"))
	}

	return connect.NewResponse(&datahubv1.MarkOutboxProcessedResponse{}), nil
}

// ReleaseOutboxEvent returns a claimed-but-unattempted event to PENDING.
func (h *Handler) ReleaseOutboxEvent(ctx context.Context, req *connect.Request[datahubv1.ReleaseOutboxEventRequest]) (*connect.Response[datahubv1.ReleaseOutboxEventResponse], error) {
	err := h.outboxUsecase.Release(ctx, req.Msg.GetId())
	switch {
	case errors.Is(err, outbox_usecase.ErrMissingID):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "ReleaseOutboxEvent failed", "error", err, "event_id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to release outbox event"))
	}

	return connect.NewResponse(&datahubv1.ReleaseOutboxEventResponse{}), nil
}

// PruneOutboxEvents deletes PROCESSED rows past the caller's retention window.
func (h *Handler) PruneOutboxEvents(ctx context.Context, req *connect.Request[datahubv1.PruneOutboxEventsRequest]) (*connect.Response[datahubv1.PruneOutboxEventsResponse], error) {
	retention := time.Duration(req.Msg.GetOlderThanSeconds()) * time.Second

	pruned, err := h.outboxUsecase.Prune(ctx, retention)
	switch {
	case errors.Is(err, outbox_usecase.ErrInvalidRetention):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "PruneOutboxEvents failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to prune outbox events"))
	}

	return connect.NewResponse(&datahubv1.PruneOutboxEventsResponse{PrunedCount: pruned}), nil
}

func outboxStatusToProto(s domain.OutboxEventStatus) datahubv1.OutboxEventStatus {
	switch s {
	case domain.OutboxPending:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PENDING
	case domain.OutboxProcessing:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSING
	case domain.OutboxProcessed:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSED
	case domain.OutboxFailed:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_FAILED
	default:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_UNSPECIFIED
	}
}

// outboxStatusFromProto maps an unrecognised or unspecified enum to the empty
// status, which the usecase rejects as non-terminal. Defaulting to PROCESSED
// here would turn "the caller sent a field this build does not know" into "the
// event was delivered".
func outboxStatusFromProto(s datahubv1.OutboxEventStatus) domain.OutboxEventStatus {
	switch s {
	case datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PENDING:
		return domain.OutboxPending
	case datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSING:
		return domain.OutboxProcessing
	case datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSED:
		return domain.OutboxProcessed
	case datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_FAILED:
		return domain.OutboxFailed
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// §2.D OG image / article_heads
// ---------------------------------------------------------------------------

// GetArticleHead returns the scraped head, leaving the field unset when the
// article has never been scraped. The caller re-scrapes on the absence, so an
// empty ArticleHead here would stop it.
func (h *Handler) GetArticleHead(ctx context.Context, req *connect.Request[datahubv1.GetArticleHeadRequest]) (*connect.Response[datahubv1.GetArticleHeadResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	head, err := h.ogImage.GetArticleHead(ctx, req.Msg.GetArticleId())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleHead failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article head"))
	}

	resp := &datahubv1.GetArticleHeadResponse{}
	if head != nil {
		resp.Head = &datahubv1.ArticleHead{
			Id:         head.ID,
			ArticleId:  head.ArticleID,
			HeadHtml:   head.HeadHTML,
			OgImageUrl: head.OgImageURL,
		}
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) BatchGetOgImageURLs(ctx context.Context, req *connect.Request[datahubv1.BatchGetOgImageURLsRequest]) (*connect.Response[datahubv1.BatchGetOgImageURLsResponse], error) {
	ids := req.Msg.GetArticleIds()
	if len(ids) == 0 {
		return connect.NewResponse(&datahubv1.BatchGetOgImageURLsResponse{OgImageUrls: map[string]string{}}), nil
	}
	if len(ids) > maxLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_ids exceeds the %d id limit", maxLimit))
	}

	urls, err := h.ogImage.BatchGetOgImageURLs(ctx, ids)
	if err != nil {
		h.logger.ErrorContext(ctx, "BatchGetOgImageURLs failed", "error", err, "count", len(ids))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get og image urls"))
	}
	if urls == nil {
		urls = map[string]string{}
	}
	return connect.NewResponse(&datahubv1.BatchGetOgImageURLsResponse{OgImageUrls: urls}), nil
}

func (h *Handler) ListFeedsMissingOgImage(ctx context.Context, req *connect.Request[datahubv1.ListFeedsMissingOgImageRequest]) (*connect.Response[datahubv1.ListFeedsMissingOgImageResponse], error) {
	candidates, err := h.ogImage.ListFeedsMissingOgImage(ctx, clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedsMissingOgImage failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list og image candidates"))
	}

	out := make([]*datahubv1.OgImageBackfillCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, &datahubv1.OgImageBackfillCandidate{ArticleId: c.ArticleID, Url: c.URL})
	}
	return connect.NewResponse(&datahubv1.ListFeedsMissingOgImageResponse{Candidates: out}), nil
}

func (h *Handler) ListUnwarmedOgImageURLs(ctx context.Context, req *connect.Request[datahubv1.ListUnwarmedOgImageURLsRequest]) (*connect.Response[datahubv1.ListUnwarmedOgImageURLsResponse], error) {
	urls, err := h.ogImage.ListUnwarmedOgImageURLs(ctx, clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListUnwarmedOgImageURLs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list unwarmed og image urls"))
	}
	return connect.NewResponse(&datahubv1.ListUnwarmedOgImageURLsResponse{Urls: urls}), nil
}

func (h *Handler) PurgeExpiredArticleHeads(ctx context.Context, req *connect.Request[datahubv1.PurgeExpiredArticleHeadsRequest]) (*connect.Response[datahubv1.PurgeExpiredArticleHeadsResponse], error) {
	ttl, err := retentionFromSeconds(req.Msg.GetTtlSeconds())
	if err != nil {
		return nil, err
	}

	purged, err := h.ogImage.PurgeExpiredArticleHeads(ctx, ttl)
	if err != nil {
		h.logger.ErrorContext(ctx, "PurgeExpiredArticleHeads failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to purge article heads"))
	}
	return connect.NewResponse(&datahubv1.PurgeExpiredArticleHeadsResponse{PurgedCount: purged}), nil
}

// ---------------------------------------------------------------------------
// §2.E Image proxy cache
// ---------------------------------------------------------------------------

func (h *Handler) GetImageProxyCache(ctx context.Context, req *connect.Request[datahubv1.GetImageProxyCacheRequest]) (*connect.Response[datahubv1.GetImageProxyCacheResponse], error) {
	if req.Msg.GetUrlHash() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url_hash is required"))
	}

	entry, err := h.imageProxyCache.Get(ctx, req.Msg.GetUrlHash())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetImageProxyCache failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get image proxy cache"))
	}

	resp := &datahubv1.GetImageProxyCacheResponse{}
	if entry != nil {
		resp.Entry = imageProxyCacheEntryToProto(entry)
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) PutImageProxyCache(ctx context.Context, req *connect.Request[datahubv1.PutImageProxyCacheRequest]) (*connect.Response[datahubv1.PutImageProxyCacheResponse], error) {
	msg := req.Msg.GetEntry()
	if msg == nil || msg.GetUrlHash() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("entry.url_hash is required"))
	}

	entry := &domain.ImageProxyCacheEntry{
		URLHash:     msg.GetUrlHash(),
		OriginalURL: msg.GetOriginalUrl(),
		Data:        msg.GetData(),
		ContentType: msg.GetContentType(),
		Width:       int(msg.GetWidth()),
		Height:      int(msg.GetHeight()),
		SizeBytes:   int(msg.GetSizeBytes()),
		ETag:        msg.GetEtag(),
		CreatedAt:   timeOrZero(msg.GetCreatedAt()),
		ExpiresAt:   timeOrZero(msg.GetExpiresAt()),
	}
	if err := h.imageProxyCache.Put(ctx, entry); err != nil {
		h.logger.ErrorContext(ctx, "PutImageProxyCache failed", "error", err, "url_hash", entry.URLHash)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save image proxy cache"))
	}
	return connect.NewResponse(&datahubv1.PutImageProxyCacheResponse{}), nil
}

func (h *Handler) EvictExpiredImageProxyCache(ctx context.Context, _ *connect.Request[datahubv1.EvictExpiredImageProxyCacheRequest]) (*connect.Response[datahubv1.EvictExpiredImageProxyCacheResponse], error) {
	evicted, err := h.imageProxyCache.EvictExpired(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "EvictExpiredImageProxyCache failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to evict expired image cache"))
	}
	return connect.NewResponse(&datahubv1.EvictExpiredImageProxyCacheResponse{EvictedCount: evicted}), nil
}

func (h *Handler) PurgeImageProxyCacheOlderThan(ctx context.Context, req *connect.Request[datahubv1.PurgeImageProxyCacheOlderThanRequest]) (*connect.Response[datahubv1.PurgeImageProxyCacheOlderThanResponse], error) {
	ttl, err := retentionFromSeconds(req.Msg.GetTtlSeconds())
	if err != nil {
		return nil, err
	}

	purged, err := h.imageProxyCache.PurgeOlderThan(ctx, ttl)
	if err != nil {
		h.logger.ErrorContext(ctx, "PurgeImageProxyCacheOlderThan failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to purge image cache"))
	}
	return connect.NewResponse(&datahubv1.PurgeImageProxyCacheOlderThanResponse{PurgedCount: purged}), nil
}

func imageProxyCacheEntryToProto(e *domain.ImageProxyCacheEntry) *datahubv1.ImageProxyCacheEntry {
	return &datahubv1.ImageProxyCacheEntry{
		UrlHash:     e.URLHash,
		OriginalUrl: e.OriginalURL,
		Data:        e.Data,
		ContentType: e.ContentType,
		Width:       safeconv.Int32(e.Width),
		Height:      safeconv.Int32(e.Height),
		SizeBytes:   int64(e.SizeBytes),
		Etag:        e.ETag,
		CreatedAt:   timestampOrNil(e.CreatedAt),
		ExpiresAt:   timestampOrNil(e.ExpiresAt),
	}
}

// ---------------------------------------------------------------------------
// §2.L Scraping policy
// ---------------------------------------------------------------------------

func (h *Handler) GetScrapingDomainByDomain(ctx context.Context, req *connect.Request[datahubv1.GetScrapingDomainByDomainRequest]) (*connect.Response[datahubv1.GetScrapingDomainByDomainResponse], error) {
	if req.Msg.GetDomain() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("domain is required"))
	}

	sd, err := h.scrapingPolicy.GetByDomain(ctx, req.Msg.GetDomain())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetScrapingDomainByDomain failed", "error", err, "domain", req.Msg.GetDomain())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get scraping domain"))
	}

	resp := &datahubv1.GetScrapingDomainByDomainResponse{}
	if sd != nil {
		resp.ScrapingDomain = scrapingDomainToProto(sd)
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) GetScrapingDomainByID(ctx context.Context, req *connect.Request[datahubv1.GetScrapingDomainByIDRequest]) (*connect.Response[datahubv1.GetScrapingDomainByIDResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id must be a uuid: %w", err))
	}

	sd, err := h.scrapingPolicy.GetByID(ctx, id)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetScrapingDomainByID failed", "error", err, "id", id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get scraping domain"))
	}

	resp := &datahubv1.GetScrapingDomainByIDResponse{}
	if sd != nil {
		resp.ScrapingDomain = scrapingDomainToProto(sd)
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) SaveScrapingDomain(ctx context.Context, req *connect.Request[datahubv1.SaveScrapingDomainRequest]) (*connect.Response[datahubv1.SaveScrapingDomainResponse], error) {
	msg := req.Msg.GetScrapingDomain()
	if msg == nil || msg.GetDomain() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("scraping_domain.domain is required"))
	}

	sd, err := scrapingDomainFromProto(msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	saved, err := h.scrapingPolicy.Save(ctx, sd)
	if err != nil {
		h.logger.ErrorContext(ctx, "SaveScrapingDomain failed", "error", err, "domain", msg.GetDomain())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save scraping domain"))
	}

	return connect.NewResponse(&datahubv1.SaveScrapingDomainResponse{
		ScrapingDomain: scrapingDomainToProto(saved),
	}), nil
}

func (h *Handler) ListScrapingDomains(ctx context.Context, req *connect.Request[datahubv1.ListScrapingDomainsRequest]) (*connect.Response[datahubv1.ListScrapingDomainsResponse], error) {
	offset := int(req.Msg.GetOffset())
	if offset < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("offset must not be negative"))
	}

	domains, err := h.scrapingPolicy.List(ctx, offset, clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListScrapingDomains failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list scraping domains"))
	}

	out := make([]*datahubv1.ScrapingDomain, 0, len(domains))
	for _, sd := range domains {
		out = append(out, scrapingDomainToProto(sd))
	}
	return connect.NewResponse(&datahubv1.ListScrapingDomainsResponse{ScrapingDomains: out}), nil
}

func (h *Handler) UpdateScrapingDomainPolicy(ctx context.Context, req *connect.Request[datahubv1.UpdateScrapingDomainPolicyRequest]) (*connect.Response[datahubv1.UpdateScrapingDomainPolicyResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id must be a uuid: %w", err))
	}

	update := &domain.ScrapingPolicyUpdate{}
	if u := req.Msg.GetUpdate(); u != nil {
		update.AllowFetchBody = u.AllowFetchBody
		update.AllowMLTraining = u.AllowMlTraining
		update.ForceRespectRobots = u.ForceRespectRobots
		if u.AllowCacheDays != nil {
			v := int(u.GetAllowCacheDays())
			update.AllowCacheDays = &v
		}
	}

	if err := h.scrapingPolicy.UpdatePolicy(ctx, id, update); err != nil {
		h.logger.ErrorContext(ctx, "UpdateScrapingDomainPolicy failed", "error", err, "id", id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update scraping domain policy"))
	}
	return connect.NewResponse(&datahubv1.UpdateScrapingDomainPolicyResponse{}), nil
}

func (h *Handler) SaveDeclinedDomain(ctx context.Context, req *connect.Request[datahubv1.SaveDeclinedDomainRequest]) (*connect.Response[datahubv1.SaveDeclinedDomainResponse], error) {
	if req.Msg.GetUserId() == "" || req.Msg.GetDomain() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id and domain are required"))
	}

	if err := h.scrapingPolicy.SaveDeclinedDomain(ctx, req.Msg.GetUserId(), req.Msg.GetDomain()); err != nil {
		h.logger.ErrorContext(ctx, "SaveDeclinedDomain failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save declined domain"))
	}
	return connect.NewResponse(&datahubv1.SaveDeclinedDomainResponse{}), nil
}

func (h *Handler) IsDomainDeclined(ctx context.Context, req *connect.Request[datahubv1.IsDomainDeclinedRequest]) (*connect.Response[datahubv1.IsDomainDeclinedResponse], error) {
	if req.Msg.GetUserId() == "" || req.Msg.GetDomain() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id and domain are required"))
	}

	declined, err := h.scrapingPolicy.IsDomainDeclined(ctx, req.Msg.GetUserId(), req.Msg.GetDomain())
	if err != nil {
		h.logger.ErrorContext(ctx, "IsDomainDeclined failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check declined domain"))
	}
	return connect.NewResponse(&datahubv1.IsDomainDeclinedResponse{Declined: declined}), nil
}

func scrapingDomainToProto(sd *domain.ScrapingDomain) *datahubv1.ScrapingDomain {
	if sd == nil {
		return nil
	}
	out := &datahubv1.ScrapingDomain{
		Id:                  sd.ID.String(),
		Domain:              sd.Domain,
		Scheme:              sd.Scheme,
		AllowFetchBody:      sd.AllowFetchBody,
		AllowMlTraining:     sd.AllowMLTraining,
		AllowCacheDays:      safeconv.Int32(sd.AllowCacheDays),
		ForceRespectRobots:  sd.ForceRespectRobots,
		RobotsDisallowPaths: sd.RobotsDisallowPaths,
		RobotsTxtUrl:        sd.RobotsTxtURL,
		RobotsTxtContent:    sd.RobotsTxtContent,
		CreatedAt:           timestampOrNil(sd.CreatedAt),
		UpdatedAt:           timestampOrNil(sd.UpdatedAt),
	}
	if sd.RobotsTxtFetchedAt != nil {
		out.RobotsTxtFetchedAt = timestampOrNil(*sd.RobotsTxtFetchedAt)
	}
	if sd.RobotsTxtLastStatus != nil {
		v := safeconv.Int32(*sd.RobotsTxtLastStatus)
		out.RobotsTxtLastStatus = &v
	}
	if sd.RobotsCrawlDelaySec != nil {
		v := safeconv.Int32(*sd.RobotsCrawlDelaySec)
		out.RobotsCrawlDelaySec = &v
	}
	return out
}

func scrapingDomainFromProto(msg *datahubv1.ScrapingDomain) (*domain.ScrapingDomain, error) {
	sd := &domain.ScrapingDomain{
		Domain:             msg.GetDomain(),
		Scheme:             msg.GetScheme(),
		AllowFetchBody:     msg.GetAllowFetchBody(),
		AllowMLTraining:    msg.GetAllowMlTraining(),
		AllowCacheDays:     int(msg.GetAllowCacheDays()),
		ForceRespectRobots: msg.GetForceRespectRobots(),
		// nil would be marshalled as SQL NULL by the driver, which reads back
		// as "unknown" rather than "no disallow rules".
		RobotsDisallowPaths: msg.GetRobotsDisallowPaths(),
		CreatedAt:           timeOrZero(msg.GetCreatedAt()),
		UpdatedAt:           timeOrZero(msg.GetUpdatedAt()),
	}
	if sd.RobotsDisallowPaths == nil {
		sd.RobotsDisallowPaths = []string{}
	}

	// An empty id means "new row"; the driver assigns one. Anything else must
	// parse, or the upsert would silently target the zero UUID.
	if raw := msg.GetId(); raw != "" && raw != uuid.Nil.String() {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("scraping_domain.id must be a uuid: %w", err)
		}
		sd.ID = id
	}

	if msg.RobotsTxtUrl != nil {
		v := msg.GetRobotsTxtUrl()
		sd.RobotsTxtURL = &v
	}
	if msg.RobotsTxtContent != nil {
		v := msg.GetRobotsTxtContent()
		sd.RobotsTxtContent = &v
	}
	if ts := msg.GetRobotsTxtFetchedAt(); ts != nil && ts.IsValid() {
		t := ts.AsTime()
		sd.RobotsTxtFetchedAt = &t
	}
	if msg.RobotsTxtLastStatus != nil {
		v := int(msg.GetRobotsTxtLastStatus())
		sd.RobotsTxtLastStatus = &v
	}
	if msg.RobotsCrawlDelaySec != nil {
		v := int(msg.GetRobotsCrawlDelaySec())
		sd.RobotsCrawlDelaySec = &v
	}
	return sd, nil
}

// ---------------------------------------------------------------------------
// §2.O Automatic full-text fetch groundwork
// ---------------------------------------------------------------------------

func (h *Handler) ListSubscribedUserIDsByFeedLinkID(ctx context.Context, req *connect.Request[datahubv1.ListSubscribedUserIDsByFeedLinkIDRequest]) (*connect.Response[datahubv1.ListSubscribedUserIDsByFeedLinkIDResponse], error) {
	if req.Msg.GetFeedLinkId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_link_id is required"))
	}

	ids, err := h.autoFulltext.ListSubscribedUserIDsByFeedLinkID(ctx, req.Msg.GetFeedLinkId())
	if err != nil {
		h.logger.ErrorContext(ctx, "ListSubscribedUserIDsByFeedLinkID failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list subscribed user ids"))
	}
	return connect.NewResponse(&datahubv1.ListSubscribedUserIDsByFeedLinkIDResponse{UserIds: ids}), nil
}

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

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// retentionFromSeconds converts a retention window and rejects a non-positive
// one at the delivery layer.
//
// An omitted protobuf field and an explicit zero are the same bytes, so a
// provider that accepted zero would let a caller that forgot the field delete
// every row the retention query matches — for PurgeExpiredArticleHeads, the
// entire table.
func retentionFromSeconds(seconds int64) (time.Duration, error) {
	if seconds <= 0 {
		return 0, connect.NewError(connect.CodeInvalidArgument,
			errors.New("ttl_seconds must be positive: an omitted field and an explicit zero are indistinguishable on the wire, "+
				"and zero would purge every row"))
	}
	return time.Duration(seconds) * time.Second, nil
}

func timestampOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func timeOrZero(ts *timestamppb.Timestamp) time.Time {
	if ts == nil || !ts.IsValid() {
		return time.Time{}
	}
	return ts.AsTime()
}
