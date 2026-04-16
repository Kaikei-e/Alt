package handler

import (
	"context"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
)

// ReadOperations defines all read/write methods beyond generic mutations.
type ReadOperations interface {
	// Projections
	GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *sovereign_db.LensFilter) ([]sovereign_db.KnowledgeHomeItem, string, bool, error)
	GetTodayDigest(ctx context.Context, userID uuid.UUID, date time.Time) (*sovereign_db.TodayDigest, error)
	GetRecallCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]sovereign_db.RecallCandidate, error)
	ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error)
	CountNeedToKnowItems(ctx context.Context, userID uuid.UUID, date time.Time) (int, error)
	GetProjectionFreshness(ctx context.Context, projectorName string) (*time.Time, error)

	// Events
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	ListKnowledgeEventsSinceForUser(ctx context.Context, tenantID, userID uuid.UUID, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	GetLatestKnowledgeEventSeqForUser(ctx context.Context, tenantID, userID uuid.UUID) (int64, error)
	AppendKnowledgeEvent(ctx context.Context, event sovereign_db.KnowledgeEvent) (int64, error)
	AreArticlesVisibleInLens(ctx context.Context, tenantID, userID uuid.UUID, articleIDs []uuid.UUID, filter *sovereign_db.LensFilter) (map[uuid.UUID]bool, error)

	// Projection infra
	GetActiveProjectionVersion(ctx context.Context) (*sovereign_db.ProjectionVersion, error)
	ListProjectionVersions(ctx context.Context) ([]sovereign_db.ProjectionVersion, error)
	CreateProjectionVersion(ctx context.Context, v sovereign_db.ProjectionVersion) error
	ActivateProjectionVersion(ctx context.Context, version int) error
	GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error)
	UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error
	GetProjectionLag(ctx context.Context) (float64, error)
	GetProjectionAge(ctx context.Context) (float64, error)

	// Reproject
	GetReprojectRun(ctx context.Context, runID uuid.UUID) (*sovereign_db.ReprojectRun, error)
	ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]sovereign_db.ReprojectRun, error)
	CreateReprojectRun(ctx context.Context, run sovereign_db.ReprojectRun) error
	UpdateReprojectRun(ctx context.Context, run sovereign_db.ReprojectRun) error
	CompareProjections(ctx context.Context, fromVersion, toVersion string) (*sovereign_db.ReprojectDiffSummary, error)
	ListProjectionAudits(ctx context.Context, projectionName string, limit int) ([]sovereign_db.ProjectionAudit, error)
	CreateProjectionAudit(ctx context.Context, audit sovereign_db.ProjectionAudit) error

	// Backfill
	GetBackfillJob(ctx context.Context, jobID uuid.UUID) (*sovereign_db.BackfillJob, error)
	ListBackfillJobs(ctx context.Context) ([]sovereign_db.BackfillJob, error)
	CreateBackfillJob(ctx context.Context, j sovereign_db.BackfillJob) error
	UpdateBackfillJob(ctx context.Context, j sovereign_db.BackfillJob) error

	// Lens
	ListLenses(ctx context.Context, userID uuid.UUID) ([]sovereign_db.KnowledgeLens, error)
	GetLens(ctx context.Context, lensID uuid.UUID) (*sovereign_db.KnowledgeLens, error)
	GetCurrentLensVersion(ctx context.Context, lensID uuid.UUID) (*sovereign_db.KnowledgeLensVersion, error)
	GetCurrentLensSelection(ctx context.Context, userID uuid.UUID) (*sovereign_db.KnowledgeCurrentLens, error)
	ResolveLensFilter(ctx context.Context, userID uuid.UUID, lensID *uuid.UUID) (*sovereign_db.LensFilter, error)
	CreateLens(ctx context.Context, l sovereign_db.KnowledgeLens) error
	CreateLensVersion(ctx context.Context, v sovereign_db.KnowledgeLensVersion) error
	SelectCurrentLens(ctx context.Context, c sovereign_db.KnowledgeCurrentLens) error
	ClearCurrentLens(ctx context.Context, userID uuid.UUID) error
	ArchiveLens(ctx context.Context, lensID uuid.UUID) error

	// Recall signals
	ListRecallSignalsByUser(ctx context.Context, userID uuid.UUID, sinceDays int) ([]sovereign_db.RecallSignal, error)
	AppendRecallSignal(ctx context.Context, s sovereign_db.RecallSignal) error

	// User events
	AppendKnowledgeUserEvent(ctx context.Context, event sovereign_db.KnowledgeUserEvent) error
}

// Compile-time check.
var _ ReadOperations = (*sovereign_db.Repository)(nil)
var _ ReadDB = (*sovereign_db.Repository)(nil)

// --- helper: parse UUID from string, return nil UUID on empty ---
func parseUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

func parseUUIDPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

