package sovereign_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ProjectionCheckpoint is one projector's cursor read as an
// optimistic-concurrency token: the sequence it stands at, plus enough of the
// row's identity to tell whether anybody has written it since.
//
// UpdatedAt is that witness, and it is load-bearing rather than informational.
// last_event_seq on its own cannot distinguish "still the 0 I read" from "reset
// to 0 again by a second rebuild while I was folding" — and re-running a
// rebuild is exactly what the reproject runbooks tell an operator to do when
// they are unsure the first one landed. Exists is the second witness: a
// projector that has never run has no row at all, and a rebuild inserts the row
// it did not find, so "absent" and "present holding 0" are different states.
//
// Obtain one from ReadProjectionCheckpointForAdvance; a hand-built value
// describes a row nobody read.
type ProjectionCheckpoint struct {
	LastEventSeq int64
	UpdatedAt    time.Time
	Exists       bool
}

// GetProjectionCheckpoint returns the last processed event sequence for a
// projector, reporting one that has never run as 0. This is the WIRE-level
// form: the GetProjectionCheckpoint RPC forwards the value verbatim.
//
// In-process projectors MUST read with ReadProjectionCheckpointForAdvance
// instead. A bare int64 collapses "no row yet" into 0 and carries nothing that
// says when the row was last written, so it cannot be paired with a
// compare-and-set — see AdvanceProjectionCheckpointIfUnchanged for why a
// projector needs one.
func (r *Repository) GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error) {
	query := `SELECT last_event_seq FROM knowledge_projection_checkpoints WHERE projector_name = $1`
	var seq int64
	err := r.pool.QueryRow(ctx, query, projectorName).Scan(&seq)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("GetProjectionCheckpoint: %w", err)
	}
	return seq, nil
}

// UpdateProjectionCheckpoint upserts the projection checkpoint unconditionally.
// This is the WIRE-level form: the UpdateProjectionCheckpoint RPC forwards the
// sequence verbatim, and alt-backend's reproject swap calls it to *set* a
// checkpoint it chose, which is only expressible as an unconditional write.
//
// In-process projectors MUST use ReadProjectionCheckpointForAdvance +
// AdvanceProjectionCheckpointIfUnchanged instead. Unconditional here means a
// batch that read the checkpoint before a concurrent RebuildProjection will
// cheerfully restore the pre-rebuild sequence on top of the read models that
// rebuild just emptied, and the projector then only ever fetches events beyond
// it: PM-2026-010, ~326 articles left unprojected behind a checkpoint standing
// at a tip the projection had never reached. RebuildProjection's
// SELECT ... FOR UPDATE makes such a write wait. It does not make it re-read.
func (r *Repository) UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error {
	query := `INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (projector_name) DO UPDATE SET last_event_seq = $2, updated_at = now()`
	_, err := r.pool.Exec(ctx, query, projectorName, lastSeq)
	if err != nil {
		return fmt.Errorf("UpdateProjectionCheckpoint: %w", err)
	}
	return nil
}

// ReadProjectionCheckpointForAdvance reads a projector's checkpoint as the
// token AdvanceProjectionCheckpointIfUnchanged compares against. A projector
// that has never run yields the zero token (Exists false), which is a state in
// its own right and not a stored 0.
func (r *Repository) ReadProjectionCheckpointForAdvance(ctx context.Context, projectorName string) (ProjectionCheckpoint, error) {
	query := `SELECT last_event_seq, updated_at FROM knowledge_projection_checkpoints WHERE projector_name = $1`
	cp, err := scanProjectionCheckpoint(r.pool.QueryRow(ctx, query, projectorName))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProjectionCheckpoint{}, nil
		}
		return ProjectionCheckpoint{}, fmt.Errorf("ReadProjectionCheckpointForAdvance %q: %w", projectorName, err)
	}
	return cp, nil
}

// AdvanceProjectionCheckpointIfUnchanged moves a projector's checkpoint to
// toSeq only if the stored row is still exactly the state `from` was read from,
// and reports whether it applied. It is the in-process counterpart of the
// wire-facing UpdateProjectionCheckpoint, in the same relation
// AppendKnowledgeEventIfNew stands in to AppendKnowledgeEvent.
//
// A projector reads its checkpoint, spends a batch folding events, and only
// then writes the new sequence back. Anything that writes the row inside that
// window — an operator's RebuildProjection, another service's reproject swap —
// has decided where the projector should stand, and a batch working from the
// older view must not overwrite that decision. Guarding on the sequence *and*
// the updated_at witness makes both a reset to a different sequence and a reset
// to the same sequence visible; guarding on the row's absence makes the
// first-ever batch safe against a rebuild that created the row underneath it,
// which is the case RebuildProjection's row lock cannot cover because there is
// no row to lock.
//
// A rejected advance is a normal outcome, not a failure: it is reported as
// (false, nil) rather than an error, so callers must branch on applied rather
// than on err — the same shape, and the same reason, as
// AppendKnowledgeEventIfNew. Callers must not retry; the stored checkpoint is
// authoritative and the next batch re-reads it.
//
// Two writes would have to land in the same microsecond *and* choose the same
// sequence for the witness to mistake them for no write at all.
func (r *Repository) AdvanceProjectionCheckpointIfUnchanged(
	ctx context.Context,
	projectorName string,
	from ProjectionCheckpoint,
	toSeq int64,
) (bool, error) {
	if !from.Exists {
		// Insert-only. ON CONFLICT DO UPDATE here would overwrite precisely
		// the row a concurrent rebuild had just created.
		insertQuery := `INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
			VALUES ($1, $2, now())
			ON CONFLICT (projector_name) DO NOTHING`
		tag, err := r.pool.Exec(ctx, insertQuery, buildAdvanceCheckpointInsertArgs(projectorName, toSeq)...)
		if err != nil {
			return false, fmt.Errorf("AdvanceProjectionCheckpointIfUnchanged insert %q: %w", projectorName, err)
		}
		return tag.RowsAffected() == 1, nil
	}

	casQuery := `UPDATE knowledge_projection_checkpoints
		SET last_event_seq = $3, updated_at = now()
		WHERE projector_name = $1 AND last_event_seq = $2 AND updated_at = $4`
	tag, err := r.pool.Exec(ctx, casQuery, buildAdvanceCheckpointCASArgs(projectorName, from, toSeq)...)
	if err != nil {
		return false, fmt.Errorf("AdvanceProjectionCheckpointIfUnchanged %q: %w", projectorName, err)
	}
	return tag.RowsAffected() == 1, nil
}

// projectionLagQuery measures the lag as a duration, because that is what the
// field it fills means: GetProjectionLagResponse.lag_seconds, which alt-backend
// multiplies by time.Second and publishes as alt_home_projector_lag_seconds,
// alerted on at > 600 (ticket) and > 1800 (page).
//
// The duration is the age of the oldest event no live projector has folded yet
// — the frontier is the minimum checkpoint, and the first event past it is the
// one that has been waiting longest. A caught-up projection has no such event
// and so has no lag; the COALESCE turns that into 0 rather than a NULL the
// float64 scan destination could not take. GREATEST clamps a future-dated event
// to 0, since alt-backend reads a negative lag as its "unavailable" sentinel.
//
// The frontier is bound to the roster rather than taken over the whole
// checkpoint table. Retiring a projection drops its read models but leaves its
// checkpoint row behind (migration 00028 for the Knowledge Loop, whose
// `knowledge-loop-projector` and `surface_planner_v2` rows the loop runbook
// still documents), and a frozen row is the permanent minimum: the reported lag
// would climb forever while every running projector stayed current. LEFT JOIN,
// not inner: a projector that has never run has no row at all, and that is the
// worst lag there is, not an absence to skip.
//
// The frontier probe is an ordered LIMIT 1 over idx_knowledge_events_seq rather
// than an aggregate, for the reason spelled out on
// knowledgeEventLastOccurrenceAgesQuery — a MergeAppend across the partitions
// stopped after the first row, instead of a scan that evicts the read path's
// shared_buffers pages on every sample.
const projectionLagQuery = `
WITH frontier AS (
	SELECT COALESCE(MIN(COALESCE(c.last_event_seq, 0)), 0) AS seq
	FROM unnest($1::text[]) AS p(projector_name)
	LEFT JOIN knowledge_projection_checkpoints c USING (projector_name)
)
SELECT COALESCE((
	SELECT GREATEST(EXTRACT(EPOCH FROM (now() - ke.occurred_at)), 0)
	FROM knowledge_events ke
	WHERE ke.event_seq > (SELECT seq FROM frontier)
	ORDER BY ke.event_seq
	LIMIT 1
), 0)::float8`

// liveProjectorNames is the checkpoint roster the lag gauge measures: the
// projectors this process actually runs. It is derived from the rebuild
// allowlist because those targets already carry the checkpoint key of each
// in-process projector, so a projector can never be added to one list and
// forgotten in the other.
func liveProjectorNames() []string {
	targets := RebuildTargets()
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.ProjectorName())
	}
	return names
}

// GetProjectionLag returns how many SECONDS the farthest-behind live projector
// is behind the event log — the age of the oldest event it has not folded yet,
// 0 when it is caught up. Seconds, not events: the value is forwarded verbatim
// as lag_seconds and alerted on in seconds, and the two units do not correlate
// (a healthy projector 700 events behind would page, an hour-dead one 100
// events behind would stay silent).
func (r *Repository) GetProjectionLag(ctx context.Context) (float64, error) {
	var lag float64
	if err := r.pool.QueryRow(ctx, projectionLagQuery, liveProjectorNames()).Scan(&lag); err != nil {
		return 0, fmt.Errorf("GetProjectionLag: %w", err)
	}
	return lag, nil
}

// GetProjectionAge returns the age in seconds since the last checkpoint update.
func (r *Repository) GetProjectionAge(ctx context.Context) (float64, error) {
	query := `SELECT EXTRACT(EPOCH FROM (now() - COALESCE(MAX(updated_at), now()))) FROM knowledge_projection_checkpoints`
	var age float64
	if err := r.pool.QueryRow(ctx, query).Scan(&age); err != nil {
		return 0, fmt.Errorf("GetProjectionAge: %w", err)
	}
	return age, nil
}

func scanProjectionCheckpoint(row rowScanner) (ProjectionCheckpoint, error) {
	var cp ProjectionCheckpoint
	err := row.Scan(&cp.LastEventSeq, &cp.UpdatedAt)
	if err != nil {
		return ProjectionCheckpoint{}, err
	}
	cp.Exists = true
	return cp, nil
}

func buildAdvanceCheckpointInsertArgs(projectorName string, toSeq int64) []any {
	return []any{projectorName, toSeq}
}

func buildAdvanceCheckpointCASArgs(projectorName string, from ProjectionCheckpoint, toSeq int64) []any {
	return []any{projectorName, from.LastEventSeq, toSeq, from.UpdatedAt}
}
