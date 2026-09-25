package datahub_capability_port

import (
	"context"

	"alt/domain"

	"github.com/google/uuid"
)

// SummaryVersionPort is the append-only summary artifact store
// (catalog §2.K W3-K1 … K4).
//
// There is no Update and no Delete, here or on the wire, and that is the
// interface saying what the table is: a summary is never rewritten, it is
// superseded by a newer one and stays readable so a reprojection can still
// resolve the event that named it.
//
// The method names are the artifact's rather than the group's — Create would
// have read better here and would have cost more elsewhere. summary_version_port
// (the caller's port, and the one the chained SaveArticleSummary usecase still
// depends on inside this process) declares exactly these four names, so one
// gateway satisfies both and there is no adapter whose only job is to rename a
// call.
type SummaryVersionPort interface {
	// CreateSummaryVersion appends one version. The id is the caller's, so
	// there is nothing to return.
	CreateSummaryVersion(ctx context.Context, sv domain.SummaryVersion) error
	// MarkSummaryVersionSuperseded points every current version of the article
	// at newVersionID and returns the one that was current before, or nil when
	// this is the article's first.
	//
	// One method, not a Get followed by an Update, because the pair runs
	// under a per-article pg_advisory_xact_lock and an advisory *xact* lock is
	// released at commit. An interface that exposed the two halves would let a
	// caller take them in separate transactions, which is the interleaving the
	// lock exists to prevent: two concurrent supersedes for one article each
	// mark the other, and the article ends up with no current version at all.
	// Row-level locking does not help — each call's WHERE excludes only its
	// own new id, so the two UPDATEs never touch the same row.
	MarkSummaryVersionSuperseded(ctx context.Context, articleID, newVersionID uuid.UUID) (*domain.SummaryVersion, error)
	// GetSummaryVersionByID is the reproject-safe read: the version an old
	// event named, not whichever is current now.
	GetSummaryVersionByID(ctx context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error)
	// GetLatestSummaryVersion is the current version — the one nothing has
	// superseded.
	GetLatestSummaryVersion(ctx context.Context, articleID uuid.UUID) (domain.SummaryVersion, error)
}

//go:generate go run go.uber.org/mock/mockgen -destination=../../../mocks/mock_tag_set_version_port.go -package=mocks alt/dataplane/port/datahub_capability_port CreateTagSetVersionPort,GetTagSetVersionByIDPort,MarkTagSetVersionSupersededPort

// CreateTagSetVersionPort creates versioned tag set snapshots.
type CreateTagSetVersionPort interface {
	CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error
}

// GetTagSetVersionByIDPort reads a specific tag set version by its ID.
type GetTagSetVersionByIDPort interface {
	GetTagSetVersionByID(ctx context.Context, tagSetVersionID uuid.UUID) (domain.TagSetVersion, error)
}

// MarkTagSetVersionSupersededPort marks all non-superseded versions as superseded by the new version.
// Returns the previous latest version (before marking), or nil if none existed.
type MarkTagSetVersionSupersededPort interface {
	MarkTagSetVersionSuperseded(ctx context.Context, articleID, newVersionID uuid.UUID) (*domain.TagSetVersion, error)
}

// TagSetVersionPort is the same shape for tag sets (catalog §2.K W3-K5 … K7),
// including the advisory lock on MarkSuperseded and the reason for it.
type TagSetVersionPort interface {
	CreateTagSetVersionPort
	MarkTagSetVersionSupersededPort
	GetTagSetVersionByIDPort
}
