package alt_db

import (
	"alt/domain"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CountBackfillSummaryTitles returns the number of (summary_version, article)
// pairs the SummaryNarrativeBackfillJob will emit a discovered event for.
// Articles with deleted_at IS NOT NULL are excluded so the job does not
// resurrect narrative for deleted articles.
func (r *KnowledgeRepository) CountBackfillSummaryTitles(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM summary_versions sv
		JOIN articles a ON a.id = sv.article_id
		WHERE a.deleted_at IS NULL
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountBackfillSummaryTitles: %w", err)
	}
	return count, nil
}

// ListBackfillSummaryTitles returns a batch of (summary_version, article)
// pairs ordered by (generated_at ASC, summary_version_id ASC) for the
// SummaryNarrativeBackfillJob to walk via cursor pagination.
//
// Title is sourced from the current articles row at backfill time. The
// articles table is mutable (no article_versions snapshot table exists);
// the job captures whatever the title is at this moment and freezes it
// into the synthetic event payload (immutable thereafter). See ADR-000846
// for the trade-off rationale.
func (r *KnowledgeRepository) ListBackfillSummaryTitles(
	ctx context.Context,
	lastGeneratedAt *time.Time,
	lastSummaryVersionID *uuid.UUID,
	limit int,
) ([]domain.KnowledgeBackfillSummaryTitle, error) {
	var (
		rows pgx.Rows
		err  error
	)
	const selectClause = `
		SELECT sv.summary_version_id,
		       sv.article_id,
		       sv.user_id,
		       sv.user_id AS tenant_id,
		       a.title,
		       sv.generated_at
		FROM summary_versions sv
		JOIN articles a ON a.id = sv.article_id
		WHERE a.deleted_at IS NULL
	`
	if lastGeneratedAt == nil || lastSummaryVersionID == nil {
		rows, err = r.pool.Query(ctx, selectClause+`
			ORDER BY sv.generated_at ASC, sv.summary_version_id ASC
			LIMIT $1
		`, limit)
	} else {
		rows, err = r.pool.Query(ctx, selectClause+`
			  AND (sv.generated_at, sv.summary_version_id) > ($1, $2)
			ORDER BY sv.generated_at ASC, sv.summary_version_id ASC
			LIMIT $3
		`, *lastGeneratedAt, *lastSummaryVersionID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("ListBackfillSummaryTitles: %w", err)
	}
	defer rows.Close()

	out := make([]domain.KnowledgeBackfillSummaryTitle, 0, limit)
	for rows.Next() {
		var row domain.KnowledgeBackfillSummaryTitle
		if err := rows.Scan(
			&row.SummaryVersionID,
			&row.ArticleID,
			&row.UserID,
			&row.TenantID,
			&row.Title,
			&row.GeneratedAt,
		); err != nil {
			return nil, fmt.Errorf("ListBackfillSummaryTitles scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListBackfillSummaryTitles rows: %w", err)
	}
	return out, nil
}
