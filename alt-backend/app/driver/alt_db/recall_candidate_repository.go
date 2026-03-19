package alt_db

import (
	"alt/domain"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (r *AltDBRepository) GetRecallCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]domain.RecallCandidate, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetRecallCandidates")
	defer span.End()

	query := `SELECT rc.user_id, rc.item_key, rc.recall_score, rc.reason_json, rc.next_suggest_at,
		rc.first_eligible_at, rc.snoozed_until, rc.updated_at, rc.projection_version,
		khi.item_key, khi.tenant_id, khi.item_type, khi.primary_ref_id, khi.title,
		khi.summary_excerpt, khi.tags_json, khi.why_json, khi.score, khi.published_at,
		khi.summary_state, COALESCE(art.url, '') AS link,
		art_fallback.title AS fb_title, art_fallback.url AS fb_url,
		art_fallback.published_at AS fb_published_at
		FROM recall_candidate_view rc
		LEFT JOIN knowledge_home_items khi
		  ON khi.user_id = rc.user_id
		  AND khi.item_key = rc.item_key
		  AND khi.projection_version = COALESCE((
		  	SELECT version FROM knowledge_projection_versions
		  	WHERE status = 'active'
		  	ORDER BY version DESC
		  	LIMIT 1
		  ), 1)
		  AND khi.dismissed_at IS NULL
		LEFT JOIN articles art
		  ON khi.primary_ref_id = art.id AND art.deleted_at IS NULL
		LEFT JOIN articles art_fallback
		  ON art_fallback.id = CASE
		  	WHEN rc.item_key ~ '^article:[0-9a-fA-F-]{36}$'
		  	THEN split_part(rc.item_key, ':', 2)::uuid
		  	ELSE NULL
		  END
		  AND art_fallback.deleted_at IS NULL
		WHERE rc.user_id = $1
		  AND (rc.snoozed_until IS NULL OR rc.snoozed_until < now())
		  AND rc.next_suggest_at IS NOT NULL
		  AND rc.next_suggest_at <= now()
		ORDER BY rc.recall_score DESC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetRecallCandidates: %w", err)
	}
	defer rows.Close()

	var candidates []domain.RecallCandidate
	for rows.Next() {
		var c domain.RecallCandidate
		var reasonJSON []byte
		var homeItemKey sql.NullString
		var tenantID sql.NullString
		var itemType sql.NullString
		var primaryRefID sql.NullString
		var title sql.NullString
		var summaryExcerpt sql.NullString
		var tagsJSON []byte
		var whyJSON []byte
		var itemScore sql.NullFloat64
		var publishedAt sql.NullTime
		var summaryState sql.NullString
		var link sql.NullString
		var fbTitle sql.NullString
		var fbURL sql.NullString
		var fbPublishedAt sql.NullTime
		if err := rows.Scan(&c.UserID, &c.ItemKey, &c.RecallScore, &reasonJSON,
			&c.NextSuggestAt, &c.FirstEligibleAt, &c.SnoozedUntil, &c.UpdatedAt, &c.ProjectionVersion,
			&homeItemKey, &tenantID, &itemType, &primaryRefID, &title,
			&summaryExcerpt, &tagsJSON, &whyJSON, &itemScore, &publishedAt,
			&summaryState, &link, &fbTitle, &fbURL, &fbPublishedAt); err != nil {
			return nil, fmt.Errorf("GetRecallCandidates scan: %w", err)
		}
		_ = json.Unmarshal(reasonJSON, &c.Reasons)
		if homeItemKey.Valid {
			item := domain.KnowledgeHomeItem{
				UserID:       c.UserID,
				ItemKey:      homeItemKey.String,
			}
			if tenantID.Valid {
				if parsedTenantID, err := uuid.Parse(tenantID.String); err == nil {
					item.TenantID = parsedTenantID
				}
			}
			if itemType.Valid {
				item.ItemType = itemType.String
			}
			if primaryRefID.Valid {
				if parsedPrimaryRefID, err := uuid.Parse(primaryRefID.String); err == nil {
					item.PrimaryRefID = &parsedPrimaryRefID
				}
			}
			if title.Valid {
				item.Title = title.String
			}
			if summaryExcerpt.Valid {
				item.SummaryExcerpt = summaryExcerpt.String
			}
			if len(tagsJSON) > 0 {
				_ = json.Unmarshal(tagsJSON, &item.Tags)
			}
			if len(whyJSON) > 0 {
				_ = json.Unmarshal(whyJSON, &item.WhyReasons)
			}
			if itemScore.Valid {
				item.Score = itemScore.Float64
			}
			if publishedAt.Valid {
				item.PublishedAt = &publishedAt.Time
			}
			if summaryState.Valid {
				item.SummaryState = summaryState.String
			} else {
				item.SummaryState = domain.SummaryStateMissing
			}
			if link.Valid {
				item.Link = link.String
			}
			c.Item = &item
		} else if fbTitle.Valid {
			item := domain.KnowledgeHomeItem{
				UserID:       c.UserID,
				ItemKey:      c.ItemKey,
				ItemType:     domain.ItemArticle,
				Title:        fbTitle.String,
				SummaryState: domain.SummaryStateMissing,
			}
			if fbURL.Valid {
				item.Link = fbURL.String
			}
			if fbPublishedAt.Valid {
				item.PublishedAt = &fbPublishedAt.Time
			}
			if articleID, err := uuid.Parse(splitArticleID(c.ItemKey)); err == nil {
				item.PrimaryRefID = &articleID
			}
			c.Item = &item
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetRecallCandidates rows: %w", err)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(candidates)))
	return candidates, nil
}

func splitArticleID(itemKey string) string {
	const prefix = "article:"
	if len(itemKey) <= len(prefix) || itemKey[:len(prefix)] != prefix {
		return ""
	}
	return itemKey[len(prefix):]
}

func (r *AltDBRepository) UpsertRecallCandidate(ctx context.Context, candidate domain.RecallCandidate) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.UpsertRecallCandidate")
	defer span.End()

	reasonJSON, _ := json.Marshal(candidate.Reasons)

	query := `INSERT INTO recall_candidate_view
		(user_id, item_key, recall_score, reason_json, next_suggest_at, first_eligible_at, updated_at, projection_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		  recall_score = EXCLUDED.recall_score,
		  reason_json = EXCLUDED.reason_json,
		  next_suggest_at = EXCLUDED.next_suggest_at,
		  updated_at = EXCLUDED.updated_at,
		  projection_version = EXCLUDED.projection_version`

	_, err := r.pool.Exec(ctx, query,
		candidate.UserID, candidate.ItemKey, candidate.RecallScore, string(reasonJSON),
		candidate.NextSuggestAt, candidate.FirstEligibleAt, candidate.UpdatedAt, candidate.ProjectionVersion,
	)
	if err != nil {
		return fmt.Errorf("UpsertRecallCandidate: %w", err)
	}
	return nil
}

func (r *AltDBRepository) SnoozeRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string, until time.Time) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.SnoozeRecallCandidate")
	defer span.End()

	query := `UPDATE recall_candidate_view SET snoozed_until = $1, updated_at = now()
		WHERE user_id = $2 AND item_key = $3`
	_, err := r.pool.Exec(ctx, query, until, userID, itemKey)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: %w", err)
	}
	return nil
}

func (r *AltDBRepository) DismissRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.DismissRecallCandidate")
	defer span.End()

	query := `DELETE FROM recall_candidate_view WHERE user_id = $1 AND item_key = $2`
	_, err := r.pool.Exec(ctx, query, userID, itemKey)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: %w", err)
	}
	return nil
}
