package datahub_gateway

import (
	"context"
	"fmt"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// VersionGateway is the versioned-artifact store: summary_versions and
// tag_set_versions (capability catalog §2.K).
//
// One gateway for both tables because they are the same shape asked twice — an
// append, a supersede, a read-by-id — and every caller that holds one holds the
// other. Splitting them would mean two Connect clients over one connection and
// two places to keep the append-first rules straight.
//
// It satisfies four summary ports and three tag-set ports:
// summary_version_port.{Create,GetLatest,GetByID,MarkSuperseded} and
// tag_set_version_port.{Create,GetByID,MarkSuperseded}.
//
// What does not move here is the knowledge event. create_summary_version_usecase
// writes the version, appends SummaryVersionCreated to knowledge-sovereign, then
// supersedes and appends SummarySuperseded; the two appends are calls to another
// service and stay with the caller (ADR-000954 D4). The order between them is
// the caller's too, which is why this gateway offers no combined procedure that
// would make that order invisible.
type VersionGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewVersionGateway(client datahubv1connect.DataHubServiceClient) *VersionGateway {
	if client == nil {
		panic("datahub_gateway: VersionGateway requires a DataHubService client — " +
			"a nil client would make every summary and tag-set version fail to " +
			"persist while the knowledge events describing them were still " +
			"appended, leaving sovereign pointing at versions that do not exist " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &VersionGateway{client: client}
}

// ---------------------------------------------------------------------------
// summary_versions (catalog §2.K W3-K1 … K4)
// ---------------------------------------------------------------------------

// CreateSummaryVersion appends one version. Append-only: there is no update
// beside it, here or on the wire.
//
// sv.ArticleTitle is deliberately not sent. It is transport for the
// SummaryVersionCreated event payload rather than a column, and the caller that
// set it is the same one that builds the event.
func (g *VersionGateway) CreateSummaryVersion(ctx context.Context, sv domain.SummaryVersion) error {
	_, err := g.client.CreateSummaryVersion(ctx, connect.NewRequest(&datahubv1.CreateSummaryVersionRequest{
		Version: summaryVersionToProto(sv),
	}))
	if err != nil {
		return fmt.Errorf("create summary version %s for article %s: %w",
			sv.SummaryVersionID, sv.ArticleID, err)
	}
	return nil
}

// MarkSummaryVersionSuperseded returns the version that was current before the
// call, or nil when the new one is the article's first.
//
// The advisory lock that makes two concurrent calls for one article safe is
// held entirely inside the procedure — see the RPC comment. Nothing on this
// side may be added between "read the previous version" and "point it at the
// new one", because there is no longer a place on this side where those two
// are separate.
func (g *VersionGateway) MarkSummaryVersionSuperseded(ctx context.Context, articleID, newVersionID uuid.UUID) (*domain.SummaryVersion, error) {
	resp, err := g.client.MarkSummaryVersionSuperseded(ctx, connect.NewRequest(&datahubv1.MarkSummaryVersionSupersededRequest{
		ArticleId:    articleID.String(),
		NewVersionId: newVersionID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("mark summary versions superseded for article %s: %w", articleID, err)
	}

	// Absent, not zero. The caller emits SummarySuperseded only when a previous
	// version existed, so an empty message here would announce that a summary
	// nobody wrote had been replaced.
	if resp.Msg.PreviousVersion == nil {
		return nil, nil
	}
	prev, convErr := summaryVersionFromProto(resp.Msg.GetPreviousVersion())
	if convErr != nil {
		return nil, convErr
	}
	return &prev, nil
}

// GetSummaryVersionByID resolves one version by its own id — the reproject-safe
// read. A version that does not exist is an error rather than a zero value,
// matching the driver: a projector that silently got an empty summary would
// render an empty card instead of failing the replay.
func (g *VersionGateway) GetSummaryVersionByID(ctx context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error) {
	resp, err := g.client.GetSummaryVersionByID(ctx, connect.NewRequest(&datahubv1.GetSummaryVersionByIDRequest{
		SummaryVersionId: summaryVersionID.String(),
	}))
	if err != nil {
		return domain.SummaryVersion{}, fmt.Errorf("get summary version %s: %w", summaryVersionID, err)
	}
	return summaryVersionFromProto(resp.Msg.GetVersion())
}

// GetLatestSummaryVersion resolves the article's current version.
func (g *VersionGateway) GetLatestSummaryVersion(ctx context.Context, articleID uuid.UUID) (domain.SummaryVersion, error) {
	resp, err := g.client.GetLatestSummaryVersion(ctx, connect.NewRequest(&datahubv1.GetLatestSummaryVersionRequest{
		ArticleId: articleID.String(),
	}))
	if err != nil {
		return domain.SummaryVersion{}, fmt.Errorf("get latest summary version for article %s: %w", articleID, err)
	}
	return summaryVersionFromProto(resp.Msg.GetVersion())
}

// ---------------------------------------------------------------------------
// tag_set_versions (catalog §2.K W3-K5 … K7)
// ---------------------------------------------------------------------------

func (g *VersionGateway) CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error {
	_, err := g.client.CreateTagSetVersion(ctx, connect.NewRequest(&datahubv1.CreateTagSetVersionRequest{
		Version: tagSetVersionToProto(tsv),
	}))
	if err != nil {
		return fmt.Errorf("create tag set version %s for article %s: %w",
			tsv.TagSetVersionID, tsv.ArticleID, err)
	}
	return nil
}

// MarkTagSetVersionSuperseded is the tag-set twin, with the same advisory-lock
// reasoning as its summary counterpart.
func (g *VersionGateway) MarkTagSetVersionSuperseded(ctx context.Context, articleID, newVersionID uuid.UUID) (*domain.TagSetVersion, error) {
	resp, err := g.client.MarkTagSetVersionSuperseded(ctx, connect.NewRequest(&datahubv1.MarkTagSetVersionSupersededRequest{
		ArticleId:    articleID.String(),
		NewVersionId: newVersionID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("mark tag set versions superseded for article %s: %w", articleID, err)
	}

	if resp.Msg.PreviousVersion == nil {
		return nil, nil
	}
	prev, convErr := tagSetVersionFromProto(resp.Msg.GetPreviousVersion())
	if convErr != nil {
		return nil, convErr
	}
	return &prev, nil
}

func (g *VersionGateway) GetTagSetVersionByID(ctx context.Context, tagSetVersionID uuid.UUID) (domain.TagSetVersion, error) {
	resp, err := g.client.GetTagSetVersionByID(ctx, connect.NewRequest(&datahubv1.GetTagSetVersionByIDRequest{
		TagSetVersionId: tagSetVersionID.String(),
	}))
	if err != nil {
		return domain.TagSetVersion{}, fmt.Errorf("get tag set version %s: %w", tagSetVersionID, err)
	}
	return tagSetVersionFromProto(resp.Msg.GetVersion())
}
