package sovereign_db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ProjectionRebuildTarget names one rebuildable read-model set: the tables to
// empty and the projector checkpoint to reset so the always-running in-process
// projector re-folds the event log from the beginning.
//
// Every field is unexported and the package exposes no constructor, so a caller
// cannot name an arbitrary table as a rebuild target. That is deliberate:
// knowledge_events and knowledge_event_dedupes are the source of truth, they are
// not disposable, and "the API cannot express it" is a stronger guarantee than a
// comment saying not to. Targets are obtained by name from LookupRebuildTarget
// or listed with RebuildTargets.
type ProjectionRebuildTarget struct {
	name          string
	tables        []string
	projectorName string
}

// Name is the operator-facing target name.
func (t ProjectionRebuildTarget) Name() string { return t.name }

// Tables returns a copy of the read-model tables the rebuild empties. A copy,
// not the backing array: handing out the allowlist itself would let a caller
// rewrite an entry in place, which is the hole the unexported fields close.
func (t ProjectionRebuildTarget) Tables() []string { return slices.Clone(t.tables) }

// ProjectorName is the knowledge_projection_checkpoints key reset to 0.
func (t ProjectionRebuildTarget) ProjectorName() string { return t.projectorName }

// IsZero reports whether this is the unusable zero value.
func (t ProjectionRebuildTarget) IsZero() bool {
	return t.name == "" || len(t.tables) == 0 || t.projectorName == ""
}

// The allowlist. Table sets mirror each projector's write surface and the
// reproject runbooks (docs/runbooks/knowledge-trail-reproject.md,
// docs/runbooks/knowledge-home-reproject-operations.md); the projector names
// are the checkpoint keys the in-process projectors read on every tick.
var (
	// RebuildTargetKnowledgeHome rebuilds the Knowledge Home read models
	// folded by knowledge_home_projector.
	RebuildTargetKnowledgeHome = ProjectionRebuildTarget{
		name: "knowledge-home",
		tables: []string{
			"knowledge_home_items",
			"today_digest_view",
			"recall_candidate_view",
		},
		projectorName: "knowledge-home-projector",
	}

	// RebuildTargetKnowledgeTrail rebuilds the Knowledge Trail read models
	// folded by knowledge_trail_projector.
	RebuildTargetKnowledgeTrail = ProjectionRebuildTarget{
		name: "knowledge-trail",
		tables: []string{
			"knowledge_trail_footprints",
			"knowledge_trail_branches",
			"knowledge_trail_act_outcomes",
		},
		projectorName: "knowledge-trail-projector",
	}
)

var rebuildTargets = []ProjectionRebuildTarget{
	RebuildTargetKnowledgeHome,
	RebuildTargetKnowledgeTrail,
}

// protectedTables can never be emptied by a rebuild. The append-only event log
// and the ingest dedupe barrier are the source of truth every projection is
// derived from; the user event log is the same class of data. This is defence
// in depth behind the unexported target fields, checked before a transaction is
// ever opened.
var protectedTables = []string{
	"knowledge_events",
	"knowledge_event_dedupes",
	"knowledge_user_events",
}

// RebuildTargets lists every rebuildable projection.
func RebuildTargets() []ProjectionRebuildTarget { return slices.Clone(rebuildTargets) }

// RebuildTargetNames lists the allowlisted target names, for error messages and
// the admin surface.
func RebuildTargetNames() []string {
	names := make([]string, 0, len(rebuildTargets))
	for _, t := range rebuildTargets {
		names = append(names, t.name)
	}
	return names
}

// LookupRebuildTarget resolves an operator-supplied name against the allowlist.
// Anything else — including a bare table name — is rejected.
func LookupRebuildTarget(name string) (ProjectionRebuildTarget, error) {
	for _, t := range rebuildTargets {
		if t.name == name {
			return t, nil
		}
	}
	return ProjectionRebuildTarget{}, fmt.Errorf("unknown rebuild target %q: valid targets are %s",
		name, strings.Join(RebuildTargetNames(), ", "))
}

// ProjectionRebuildResult describes what a rebuild did, for the admin response
// and the operator's log trail.
type ProjectionRebuildResult struct {
	Target           string   `json:"target"`
	Tables           []string `json:"tables"`
	TablesTruncated  int      `json:"tables_truncated"`
	ProjectorName    string   `json:"projector_name"`
	CheckpointBefore int64    `json:"checkpoint_before"`
}

// RebuildProjection empties a projection's read models and resets its projector
// checkpoint to 0 in a single transaction, so the in-process projector re-folds
// the whole event log on its next tick.
//
// The single transaction is the point. PM-2026-010 was caused by running the
// TRUNCATE and the checkpoint reset as two statements: the always-running
// projector advanced the checkpoint in the gap, and every event in that gap was
// never folded into the rebuilt model (~326 articles).
//
// The checkpoint row is locked first, but that lock is narrower than it looks
// and is not what makes the reset survive a running projector:
//
//   - FOR UPDATE makes a concurrent writer wait. It does not make it re-read. A
//     projector batch that read the checkpoint before this transaction began
//     and writes it after this one commits lands on top of the reset rather
//     than behind it, restoring the pre-rebuild sequence over the tables just
//     emptied — the same end state PM-2026-010 had.
//   - When the projector has never run there is no row at all, so the
//     FOR UPDATE below locks nothing and the window is wide open.
//
// What closes both is on the projector's side: it advances its checkpoint with
// AdvanceProjectionCheckpointIfUnchanged, a compare-and-set against the
// (last_event_seq, updated_at, row-exists) state it read at the start of its
// batch. That makes the updated_at written by the reset below load-bearing
// rather than decorative — it is the witness that invalidates an in-flight
// batch even when the sequence is 0 on both sides of the rebuild, which is what
// a second, idempotent rebuild looks like.
//
// TRUNCATE is issued without CASCADE on purpose: should a foreign key ever be
// added from a table that is not disposable, the rebuild must fail loudly
// instead of cascading into it.
func (r *Repository) RebuildProjection(ctx context.Context, target ProjectionRebuildTarget) (ProjectionRebuildResult, error) {
	if target.IsZero() {
		return ProjectionRebuildResult{}, fmt.Errorf("RebuildProjection: no rebuild target given; valid targets are %s",
			strings.Join(RebuildTargetNames(), ", "))
	}
	for _, table := range target.tables {
		if slices.Contains(protectedTables, table) {
			return ProjectionRebuildResult{}, fmt.Errorf(
				"RebuildProjection: refusing to rebuild %q: %s is the source of truth, not a projection",
				target.name, table)
		}
	}

	slog.InfoContext(ctx, "projection.rebuild.start",
		"target", target.name,
		"tables", len(target.tables),
		"table_names", target.tables,
		"projector_name", target.projectorName)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("RebuildProjection begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit has succeeded

	const lockQuery = `SELECT last_event_seq FROM knowledge_projection_checkpoints
		WHERE projector_name = $1 FOR UPDATE`
	var checkpointBefore int64
	if err := tx.QueryRow(ctx, lockQuery, target.projectorName).Scan(&checkpointBefore); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return ProjectionRebuildResult{}, fmt.Errorf("RebuildProjection lock checkpoint %q: %w", target.projectorName, err)
		}
		// No checkpoint row yet: the projector has never run. Nothing to lock —
		// see the note above on why the lock is not what protects the reset —
		// and the reset below inserts the row.
		checkpointBefore = 0
	}

	truncateQuery := "TRUNCATE TABLE " + strings.Join(target.tables, ", ")
	if _, err := tx.Exec(ctx, truncateQuery); err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("RebuildProjection truncate %q: %w", target.name, err)
	}

	const resetQuery = `INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
		VALUES ($1, 0, now())
		ON CONFLICT (projector_name) DO UPDATE SET last_event_seq = 0, updated_at = now()`
	if _, err := tx.Exec(ctx, resetQuery, target.projectorName); err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("RebuildProjection reset checkpoint %q: %w", target.projectorName, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("RebuildProjection commit: %w", err)
	}

	result := ProjectionRebuildResult{
		Target:           target.name,
		Tables:           target.Tables(),
		TablesTruncated:  len(target.tables),
		ProjectorName:    target.projectorName,
		CheckpointBefore: checkpointBefore,
	}

	slog.InfoContext(ctx, "projection.rebuild.done",
		"target", result.Target,
		"tables", result.TablesTruncated,
		"table_names", result.Tables,
		"projector_name", result.ProjectorName,
		"checkpoint_reset_from", result.CheckpointBefore)

	return result, nil
}
