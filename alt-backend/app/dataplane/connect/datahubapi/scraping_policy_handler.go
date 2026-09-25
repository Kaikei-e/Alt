package datahubapi

import (
	"context"
	"errors"
	"fmt"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

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
		Domain:              msg.GetDomain(),
		Scheme:              msg.GetScheme(),
		AllowFetchBody:      msg.GetAllowFetchBody(),
		AllowMLTraining:     msg.GetAllowMlTraining(),
		AllowCacheDays:      int(msg.GetAllowCacheDays()),
		ForceRespectRobots:  msg.GetForceRespectRobots(),
		RobotsDisallowPaths: msg.GetRobotsDisallowPaths(),
		CreatedAt:           timeOrZero(msg.GetCreatedAt()),
		UpdatedAt:           timeOrZero(msg.GetUpdatedAt()),
	}
	if sd.RobotsDisallowPaths == nil {
		// nil would be marshalled as SQL NULL by the driver, which reads back
		// as "unknown" rather than "no disallow rules".
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
