package datahub_capability_gateway

import (
	"context"
	"fmt"

	"alt/domain"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// §2.K Versioned artifacts
// ---------------------------------------------------------------------------

// summaryVersionDriver is the slice of alt_db the summary_versions capabilities
// use.
//
// MarkSummaryVersionSuperseded is one method here for the same reason it is one
// RPC: the driver holds a transaction open across `SELECT
// pg_advisory_xact_lock(...)`, the read of the previous version and the UPDATE
// that supersedes it. Nothing in this package may sit between those, because
// the lock is released at commit and there is no way to hold it across two
// calls.
type summaryVersionDriver interface {
	CreateSummaryVersion(ctx context.Context, sv domain.SummaryVersion) error
	MarkSummaryVersionSuperseded(ctx context.Context, articleID, newVersionID uuid.UUID) (*domain.SummaryVersion, error)
	GetSummaryVersionByID(ctx context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error)
	GetLatestSummaryVersion(ctx context.Context, articleID uuid.UUID) (domain.SummaryVersion, error)
}

// SummaryVersionGateway implements datahub_capability_port.SummaryVersionPort.
type SummaryVersionGateway struct {
	db summaryVersionDriver
}

func NewSummaryVersionGateway(db *alt_db.AltDBRepository) *SummaryVersionGateway {
	return &SummaryVersionGateway{db: db}
}

func (g *SummaryVersionGateway) CreateSummaryVersion(ctx context.Context, sv domain.SummaryVersion) error {
	if err := g.db.CreateSummaryVersion(ctx, sv); err != nil {
		return fmt.Errorf("create summary version %s: %w", sv.SummaryVersionID, err)
	}
	return nil
}

// MarkSummaryVersionSuperseded forwards the driver's nil-without-error for "this is the
// article's first version".
//
// That nil is the answer the caller acts on — it emits SummarySuperseded only
// when a previous version came back — so it must not become a zero-valued
// struct on the way through.
func (g *SummaryVersionGateway) MarkSummaryVersionSuperseded(ctx context.Context, articleID, newVersionID uuid.UUID) (*domain.SummaryVersion, error) {
	prev, err := g.db.MarkSummaryVersionSuperseded(ctx, articleID, newVersionID)
	if err != nil {
		return nil, fmt.Errorf("mark summary versions superseded for article %s: %w", articleID, err)
	}
	return prev, nil
}

func (g *SummaryVersionGateway) GetSummaryVersionByID(ctx context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error) {
	sv, err := g.db.GetSummaryVersionByID(ctx, summaryVersionID)
	if err != nil {
		return domain.SummaryVersion{}, fmt.Errorf("get summary version %s: %w", summaryVersionID, err)
	}
	return sv, nil
}

func (g *SummaryVersionGateway) GetLatestSummaryVersion(ctx context.Context, articleID uuid.UUID) (domain.SummaryVersion, error) {
	sv, err := g.db.GetLatestSummaryVersion(ctx, articleID)
	if err != nil {
		return domain.SummaryVersion{}, fmt.Errorf("get latest summary version for article %s: %w", articleID, err)
	}
	return sv, nil
}

// tagSetVersionDriver is the tag-set half, with the same advisory-lock note as
// summaryVersionDriver.
type tagSetVersionDriver interface {
	CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error
	MarkTagSetVersionSuperseded(ctx context.Context, articleID, newVersionID uuid.UUID) (*domain.TagSetVersion, error)
	GetTagSetVersionByID(ctx context.Context, tagSetVersionID uuid.UUID) (domain.TagSetVersion, error)
}

// TagSetVersionGateway implements datahub_capability_port.TagSetVersionPort.
type TagSetVersionGateway struct {
	db tagSetVersionDriver
}

func NewTagSetVersionGateway(db *alt_db.AltDBRepository) *TagSetVersionGateway {
	return &TagSetVersionGateway{db: db}
}

func (g *TagSetVersionGateway) CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error {
	if err := g.db.CreateTagSetVersion(ctx, tsv); err != nil {
		return fmt.Errorf("create tag set version %s: %w", tsv.TagSetVersionID, err)
	}
	return nil
}

func (g *TagSetVersionGateway) MarkTagSetVersionSuperseded(ctx context.Context, articleID, newVersionID uuid.UUID) (*domain.TagSetVersion, error) {
	prev, err := g.db.MarkTagSetVersionSuperseded(ctx, articleID, newVersionID)
	if err != nil {
		return nil, fmt.Errorf("mark tag set versions superseded for article %s: %w", articleID, err)
	}
	return prev, nil
}

func (g *TagSetVersionGateway) GetTagSetVersionByID(ctx context.Context, tagSetVersionID uuid.UUID) (domain.TagSetVersion, error) {
	tsv, err := g.db.GetTagSetVersionByID(ctx, tagSetVersionID)
	if err != nil {
		return domain.TagSetVersion{}, fmt.Errorf("get tag set version %s: %w", tagSetVersionID, err)
	}
	return tsv, nil
}
