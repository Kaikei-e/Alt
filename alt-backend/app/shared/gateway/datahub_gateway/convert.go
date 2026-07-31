// Package datahub_gateway is the anti-corruption layer between the domain
// types alt-backend and alt-harvester work in and the alt.datahub.v1 wire
// messages alt-data-hub serves (ADR-000954 D3).
//
// One package, several narrow gateways: each satisfies a port a usecase or a
// scheduled job already declared, so migrating a capability off the direct
// alt_db driver changes a DI line and nothing else. The mapping lives here and
// only here — no proto type escapes into a usecase, and no domain type is
// passed to the Connect client without going through this file.
package datahub_gateway

import (
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// timeFromProto converts a possibly-nil protobuf timestamp to a Go time.
// A nil or unset timestamp becomes the zero time, which is what every caller
// of these fields already checks for.
func timeFromProto(ts *timestamppb.Timestamp) time.Time {
	if ts == nil || !ts.IsValid() {
		return time.Time{}
	}
	return ts.AsTime()
}

// timeToProto is the inverse. The zero time becomes nil rather than the Unix
// epoch: "not set" and "1970-01-01" mean different things to every column
// these values land in.
func timeToProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// timePtrFromProto maps an absent optional timestamp to a nil *time.Time, so
// the domain structs keep expressing "no value" the way they always have.
func timePtrFromProto(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil || !ts.IsValid() {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func timePtrToProto(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
}

// ---------------------------------------------------------------------------
// Outbox
// ---------------------------------------------------------------------------

// outboxStatusFromProto maps the wire enum to the domain status. An
// unspecified value maps to the empty status rather than guessing PENDING:
// a claimed row that reports no status is a provider bug, and silently
// relabelling it as pending would hand it back to the claim loop forever.
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

func outboxEventFromProto(e *datahubv1.OutboxEvent) domain.OutboxEvent {
	if e == nil {
		return domain.OutboxEvent{}
	}
	return domain.OutboxEvent{
		ID:        e.GetId(),
		EventType: e.GetEventType(),
		Payload:   e.GetPayload(),
		Status:    outboxStatusFromProto(e.GetStatus()),
		CreatedAt: timeFromProto(e.GetCreatedAt()),
	}
}

// ---------------------------------------------------------------------------
// Article heads / OG images
// ---------------------------------------------------------------------------

func articleHeadFromProto(h *datahubv1.ArticleHead) *domain.ArticleHead {
	if h == nil {
		return nil
	}
	return &domain.ArticleHead{
		ID:         h.GetId(),
		ArticleID:  h.GetArticleId(),
		HeadHTML:   h.GetHeadHtml(),
		OgImageURL: h.GetOgImageUrl(),
	}
}

// ---------------------------------------------------------------------------
// Image proxy cache
// ---------------------------------------------------------------------------

func imageProxyCacheEntryFromProto(e *datahubv1.ImageProxyCacheEntry) *domain.ImageProxyCacheEntry {
	if e == nil {
		return nil
	}
	return &domain.ImageProxyCacheEntry{
		URLHash:     e.GetUrlHash(),
		OriginalURL: e.GetOriginalUrl(),
		Data:        e.GetData(),
		ContentType: e.GetContentType(),
		Width:       int(e.GetWidth()),
		Height:      int(e.GetHeight()),
		SizeBytes:   int(e.GetSizeBytes()),
		ETag:        e.GetEtag(),
		CreatedAt:   timeFromProto(e.GetCreatedAt()),
		ExpiresAt:   timeFromProto(e.GetExpiresAt()),
	}
}

func imageProxyCacheEntryToProto(e *domain.ImageProxyCacheEntry) *datahubv1.ImageProxyCacheEntry {
	if e == nil {
		return nil
	}
	return &datahubv1.ImageProxyCacheEntry{
		UrlHash:     e.URLHash,
		OriginalUrl: e.OriginalURL,
		Data:        e.Data,
		ContentType: e.ContentType,
		Width:       int32(e.Width),  //nolint:gosec // pixel dimensions, bounded by the resizer
		Height:      int32(e.Height), //nolint:gosec // pixel dimensions, bounded by the resizer
		SizeBytes:   int64(e.SizeBytes),
		Etag:        e.ETag,
		CreatedAt:   timeToProto(e.CreatedAt),
		ExpiresAt:   timeToProto(e.ExpiresAt),
	}
}

// ---------------------------------------------------------------------------
// Scraping policy
// ---------------------------------------------------------------------------

func scrapingDomainFromProto(sd *datahubv1.ScrapingDomain) (*domain.ScrapingDomain, error) {
	if sd == nil {
		return nil, nil
	}
	id, err := parseUUID(sd.GetId())
	if err != nil {
		return nil, err
	}

	out := &domain.ScrapingDomain{
		ID:                 id,
		Domain:             sd.GetDomain(),
		Scheme:             sd.GetScheme(),
		AllowFetchBody:     sd.GetAllowFetchBody(),
		AllowMLTraining:    sd.GetAllowMlTraining(),
		AllowCacheDays:     int(sd.GetAllowCacheDays()),
		ForceRespectRobots: sd.GetForceRespectRobots(),
		// The driver normalises a NULL text[] to an empty slice, and the
		// robots matcher treats nil and empty identically. Keeping it
		// non-nil here means a round trip through the wire cannot turn
		// "no disallow rules recorded" into a nil that reads as unknown.
		RobotsDisallowPaths: sd.GetRobotsDisallowPaths(),
		CreatedAt:           timeFromProto(sd.GetCreatedAt()),
		UpdatedAt:           timeFromProto(sd.GetUpdatedAt()),
	}
	if out.RobotsDisallowPaths == nil {
		out.RobotsDisallowPaths = []string{}
	}
	if sd.RobotsTxtUrl != nil {
		v := sd.GetRobotsTxtUrl()
		out.RobotsTxtURL = &v
	}
	if sd.RobotsTxtContent != nil {
		v := sd.GetRobotsTxtContent()
		out.RobotsTxtContent = &v
	}
	out.RobotsTxtFetchedAt = timePtrFromProto(sd.GetRobotsTxtFetchedAt())
	if sd.RobotsTxtLastStatus != nil {
		v := int(sd.GetRobotsTxtLastStatus())
		out.RobotsTxtLastStatus = &v
	}
	if sd.RobotsCrawlDelaySec != nil {
		v := int(sd.GetRobotsCrawlDelaySec())
		out.RobotsCrawlDelaySec = &v
	}
	return out, nil
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
		AllowCacheDays:      int32(sd.AllowCacheDays), //nolint:gosec // days, operator-supplied and small
		ForceRespectRobots:  sd.ForceRespectRobots,
		RobotsDisallowPaths: sd.RobotsDisallowPaths,
		RobotsTxtFetchedAt:  timePtrToProto(sd.RobotsTxtFetchedAt),
		CreatedAt:           timeToProto(sd.CreatedAt),
		UpdatedAt:           timeToProto(sd.UpdatedAt),
	}
	out.RobotsTxtUrl = sd.RobotsTxtURL
	out.RobotsTxtContent = sd.RobotsTxtContent
	if sd.RobotsTxtLastStatus != nil {
		v := int32(*sd.RobotsTxtLastStatus) //nolint:gosec // HTTP status code
		out.RobotsTxtLastStatus = &v
	}
	if sd.RobotsCrawlDelaySec != nil {
		v := int32(*sd.RobotsCrawlDelaySec) //nolint:gosec // seconds, from robots.txt
		out.RobotsCrawlDelaySec = &v
	}
	return out
}

func scrapingPolicyUpdateToProto(u *domain.ScrapingPolicyUpdate) *datahubv1.ScrapingPolicyUpdate {
	if u == nil {
		return &datahubv1.ScrapingPolicyUpdate{}
	}
	out := &datahubv1.ScrapingPolicyUpdate{
		AllowFetchBody:     u.AllowFetchBody,
		AllowMlTraining:    u.AllowMLTraining,
		ForceRespectRobots: u.ForceRespectRobots,
	}
	if u.AllowCacheDays != nil {
		v := int32(*u.AllowCacheDays) //nolint:gosec // days, operator-supplied and small
		out.AllowCacheDays = &v
	}
	return out
}
