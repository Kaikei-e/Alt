package datahubapi

import (
	"context"
	"errors"
	"fmt"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
)

// ---------------------------------------------------------------------------
// §2.F Feed links
// ---------------------------------------------------------------------------

// maxBulkFeedLinks caps one OPML import. The bound is here rather than in the
// gateway because only the delivery layer can refuse the request outright; a
// gateway that truncated would import a prefix and report success for the
// whole file.
const maxBulkFeedLinks = 5000

func (h *Handler) RegisterFeedLink(ctx context.Context, req *connect.Request[datahubv1.RegisterFeedLinkRequest]) (*connect.Response[datahubv1.RegisterFeedLinkResponse], error) {
	if req.Msg.GetUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}

	alreadyExisted, err := h.feedLink.Register(ctx, req.Msg.GetUrl())
	if err != nil {
		h.logger.ErrorContext(ctx, "RegisterFeedLink failed", "error", err, "url", req.Msg.GetUrl())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to register feed link"))
	}

	return connect.NewResponse(&datahubv1.RegisterFeedLinkResponse{
		AlreadyExisted: alreadyExisted,
	}), nil
}

func (h *Handler) BulkRegisterFeedLinks(ctx context.Context, req *connect.Request[datahubv1.BulkRegisterFeedLinksRequest]) (*connect.Response[datahubv1.BulkRegisterFeedLinksResponse], error) {
	urls := req.Msg.GetUrls()
	if len(urls) == 0 {
		return connect.NewResponse(&datahubv1.BulkRegisterFeedLinksResponse{FailedUrls: []string{}}), nil
	}
	if len(urls) > maxBulkFeedLinks {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("urls exceeds the %d entry limit", maxBulkFeedLinks))
	}

	registered, skipped, failed, err := h.feedLink.BulkRegister(ctx, urls)
	if err != nil {
		h.logger.ErrorContext(ctx, "BulkRegisterFeedLinks failed", "error", err, "url_count", len(urls))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to bulk register feed links"))
	}

	return connect.NewResponse(&datahubv1.BulkRegisterFeedLinksResponse{
		Registered: safeconv.Int32(registered),
		Skipped:    safeconv.Int32(skipped),
		FailedUrls: failed,
	}), nil
}

func (h *Handler) ListFeedLinks(ctx context.Context, _ *connect.Request[datahubv1.ListFeedLinksRequest]) (*connect.Response[datahubv1.ListFeedLinksResponse], error) {
	links, err := h.feedLink.List(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedLinks failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed links"))
	}

	out := make([]*datahubv1.FeedLink, 0, len(links))
	for _, l := range links {
		if l == nil {
			continue
		}
		out = append(out, &datahubv1.FeedLink{Id: l.ID.String(), Url: l.URL})
	}
	return connect.NewResponse(&datahubv1.ListFeedLinksResponse{FeedLinks: out}), nil
}

func (h *Handler) ListFeedLinksWithHealth(ctx context.Context, _ *connect.Request[datahubv1.ListFeedLinksWithHealthRequest]) (*connect.Response[datahubv1.ListFeedLinksWithHealthResponse], error) {
	links, err := h.feedLink.ListWithHealth(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedLinksWithHealth failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed links with health"))
	}

	out := make([]*datahubv1.FeedLinkWithHealth, 0, len(links))
	for _, l := range links {
		if l == nil {
			continue
		}
		out = append(out, &datahubv1.FeedLinkWithHealth{
			FeedLink: &datahubv1.FeedLink{Id: l.ID.String(), Url: l.URL},
			// Left unset for a link that has never been polled. The admin
			// screen renders "unknown" for the absence and "healthy" for a
			// zero-failure row, and those are different facts.
			Availability: feedLinkAvailabilityToProto(l.Availability),
		})
	}
	return connect.NewResponse(&datahubv1.ListFeedLinksWithHealthResponse{FeedLinks: out}), nil
}

func (h *Handler) DeleteFeedLink(ctx context.Context, req *connect.Request[datahubv1.DeleteFeedLinkRequest]) (*connect.Response[datahubv1.DeleteFeedLinkResponse], error) {
	id, err := requiredUUID(req.Msg.GetId(), "id")
	if err != nil {
		return nil, err
	}

	if err := h.feedLink.Delete(ctx, id); err != nil {
		h.logger.ErrorContext(ctx, "DeleteFeedLink failed", "error", err, "id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete feed link"))
	}
	return connect.NewResponse(&datahubv1.DeleteFeedLinkResponse{}), nil
}

func (h *Handler) ResolveFeedLinkIDByURL(ctx context.Context, req *connect.Request[datahubv1.ResolveFeedLinkIDByURLRequest]) (*connect.Response[datahubv1.ResolveFeedLinkIDByURLResponse], error) {
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	id, err := h.feedLink.ResolveIDByURL(ctx, req.Msg.GetFeedUrl())
	if err != nil {
		h.logger.ErrorContext(ctx, "ResolveFeedLinkIDByURL failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve feed link id"))
	}

	return connect.NewResponse(&datahubv1.ResolveFeedLinkIDByURLResponse{FeedLinkId: id}), nil
}

func (h *Handler) ListFeedLinkDomains(ctx context.Context, _ *connect.Request[datahubv1.ListFeedLinkDomainsRequest]) (*connect.Response[datahubv1.ListFeedLinkDomainsResponse], error) {
	domains, err := h.feedLink.ListDomains(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedLinkDomains failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed link domains"))
	}

	out := make([]*datahubv1.FeedLinkDomain, 0, len(domains))
	for _, d := range domains {
		out = append(out, &datahubv1.FeedLinkDomain{Domain: d.Domain, Scheme: d.Scheme})
	}
	return connect.NewResponse(&datahubv1.ListFeedLinkDomainsResponse{Domains: out}), nil
}

func (h *Handler) ListRSSFeedURLs(ctx context.Context, _ *connect.Request[datahubv1.ListRSSFeedURLsRequest]) (*connect.Response[datahubv1.ListRSSFeedURLsResponse], error) {
	links, err := h.feedLink.ListPollable(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListRSSFeedURLs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list rss feed urls"))
	}

	out := make([]*datahubv1.FeedLink, 0, len(links))
	for _, l := range links {
		out = append(out, &datahubv1.FeedLink{Id: l.ID.String(), Url: l.URL})
	}
	return connect.NewResponse(&datahubv1.ListRSSFeedURLsResponse{FeedLinks: out}), nil
}

func (h *Handler) ListFeedLinksForExport(ctx context.Context, _ *connect.Request[datahubv1.ListFeedLinksForExportRequest]) (*connect.Response[datahubv1.ListFeedLinksForExportResponse], error) {
	entries, err := h.feedLink.ListForExport(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedLinksForExport failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed links for export"))
	}

	out := make([]*datahubv1.FeedLinkExportEntry, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		out = append(out, &datahubv1.FeedLinkExportEntry{Url: e.URL, Title: e.Title})
	}
	return connect.NewResponse(&datahubv1.ListFeedLinksForExportResponse{Entries: out}), nil
}

// ---------------------------------------------------------------------------
// §2.G Feed link availability
// ---------------------------------------------------------------------------

// RecordFeedLinkFailure increments the failure run and disables the link in the
// same transaction once it crosses the caller's threshold (catalog §4-4).
func (h *Handler) RecordFeedLinkFailure(ctx context.Context, req *connect.Request[datahubv1.RecordFeedLinkFailureRequest]) (*connect.Response[datahubv1.RecordFeedLinkFailureResponse], error) {
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	availability, disabledNow, err := h.feedLinkAvailability.RecordFailure(ctx,
		req.Msg.GetFeedUrl(), req.Msg.GetReason(), int(req.Msg.GetDisableAfterFailures()))
	if err != nil {
		h.logger.ErrorContext(ctx, "RecordFeedLinkFailure failed", "error", err, "feed_url", req.Msg.GetFeedUrl())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to record feed link failure"))
	}

	return connect.NewResponse(&datahubv1.RecordFeedLinkFailureResponse{
		Availability: feedLinkAvailabilityToProto(availability),
		DisabledNow:  disabledNow,
	}), nil
}

func (h *Handler) ResetFeedLinkFailures(ctx context.Context, req *connect.Request[datahubv1.ResetFeedLinkFailuresRequest]) (*connect.Response[datahubv1.ResetFeedLinkFailuresResponse], error) {
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	if err := h.feedLinkAvailability.ResetFailures(ctx, req.Msg.GetFeedUrl()); err != nil {
		h.logger.ErrorContext(ctx, "ResetFeedLinkFailures failed", "error", err, "feed_url", req.Msg.GetFeedUrl())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to reset feed link failures"))
	}
	return connect.NewResponse(&datahubv1.ResetFeedLinkFailuresResponse{}), nil
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
