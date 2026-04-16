package sovereign_db

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// AreArticlesVisibleInLens checks, for each input article_id, whether the
// article would appear in the user's lens-filtered Knowledge Home view.
//
// The query reuses the same predicates as GetKnowledgeHomeItems so stream
// delivery and unary fetch agree on visibility. It does not mutate any
// projection — it is a pure read against knowledge_home_items.
//
// Articles missing from the result map should be treated as not visible by
// the caller (fail-closed).
func (r *Repository) AreArticlesVisibleInLens(ctx context.Context, tenantID, userID uuid.UUID, articleIDs []uuid.UUID, filter *LensFilter) (map[uuid.UUID]bool, error) {
	if len(articleIDs) == 0 {
		return map[uuid.UUID]bool{}, nil
	}

	var query strings.Builder
	args := []interface{}{tenantID, userID, articleIDs}
	argPos := 4

	query.WriteString(`SELECT khi.primary_ref_id
		FROM knowledge_home_items khi
		WHERE khi.tenant_id = $1
		  AND khi.user_id = $2
		  AND khi.item_type = 'article'
		  AND khi.primary_ref_id = ANY($3)
		  AND khi.projection_version = COALESCE((
		  	SELECT version FROM knowledge_projection_versions
		  	WHERE status = 'active'
		  	ORDER BY version DESC LIMIT 1
		  ), 1)
		  AND khi.dismissed_at IS NULL`)

	if filter != nil {
		if filter.QueryText != "" {
			query.WriteString(fmt.Sprintf(` AND (
				khi.title ILIKE $%d
				OR COALESCE(khi.summary_excerpt, '') ILIKE $%d
				OR EXISTS (
					SELECT 1 FROM jsonb_array_elements_text(khi.tags_json) AS tag_name
					WHERE tag_name ILIKE $%d
				)
			)`, argPos, argPos, argPos))
			args = append(args, "%"+filter.QueryText+"%")
			argPos++
		}
		if len(filter.TagNames) > 0 {
			query.WriteString(fmt.Sprintf(` AND EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(khi.tags_json) AS tag_name
				WHERE tag_name = ANY($%d)
			)`, argPos))
			args = append(args, filter.TagNames)
			argPos++
		}
		if filter.TimeWindow != "" {
			cutoff, err := cutoffFromTimeWindow(filter.TimeWindow)
			if err != nil {
				return nil, fmt.Errorf("AreArticlesVisibleInLens: %w", err)
			}
			query.WriteString(fmt.Sprintf(` AND khi.published_at >= $%d`, argPos))
			args = append(args, cutoff)
			argPos++
		}
	}

	rows, err := r.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("AreArticlesVisibleInLens: %w", err)
	}
	defer rows.Close()

	visible := make(map[uuid.UUID]bool, len(articleIDs))
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("AreArticlesVisibleInLens scan: %w", err)
		}
		visible[id] = true
	}
	return visible, nil
}
