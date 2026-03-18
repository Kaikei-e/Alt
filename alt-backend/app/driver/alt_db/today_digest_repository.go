package alt_db

import (
	"alt/domain"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// GetTodayDigest returns the today digest for a user and date.
func (r *AltDBRepository) GetTodayDigest(ctx context.Context, userID uuid.UUID, date time.Time) (domain.TodayDigest, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetTodayDigest")
	defer span.End()

	query := `SELECT user_id, digest_date, new_articles, summarized_articles,
		unsummarized_articles, top_tags_json, pulse_refs_json, updated_at
		FROM today_digest_view
		WHERE user_id = $1 AND digest_date = $2`

	var digest domain.TodayDigest
	var topTagsJSON, pulseRefsJSON []byte

	err := r.pool.QueryRow(ctx, query, userID, date.Format("2006-01-02")).Scan(
		&digest.UserID, &digest.DigestDate, &digest.NewArticles,
		&digest.SummarizedArticles, &digest.UnsummarizedArticles,
		&topTagsJSON, &pulseRefsJSON, &digest.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Return empty digest for the date
			return domain.TodayDigest{
				UserID:     userID,
				DigestDate: date,
				UpdatedAt:  time.Now(),
			}, nil
		}
		return domain.TodayDigest{}, fmt.Errorf("GetTodayDigest: %w", err)
	}

	_ = json.Unmarshal(topTagsJSON, &digest.TopTags)
	return digest, nil
}

// UpsertTodayDigest inserts or updates a today digest.
func (r *AltDBRepository) UpsertTodayDigest(ctx context.Context, digest domain.TodayDigest) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.UpsertTodayDigest")
	defer span.End()

	topTags := digest.TopTags
	if topTags == nil {
		topTags = []string{}
	}
	topTagsJSON, _ := json.Marshal(topTags)
	pulseRefsJSON := []byte("[]")

	query := `INSERT INTO today_digest_view
		(user_id, digest_date, new_articles, summarized_articles,
		 unsummarized_articles, top_tags_json, pulse_refs_json, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, digest_date) DO UPDATE SET
		 new_articles = today_digest_view.new_articles + EXCLUDED.new_articles,
		 summarized_articles = today_digest_view.summarized_articles + EXCLUDED.summarized_articles,
		 unsummarized_articles = GREATEST(0, today_digest_view.unsummarized_articles + EXCLUDED.unsummarized_articles),
		 top_tags_json = CASE WHEN EXCLUDED.top_tags_json != '[]'::jsonb THEN EXCLUDED.top_tags_json ELSE today_digest_view.top_tags_json END,
		 pulse_refs_json = CASE WHEN EXCLUDED.pulse_refs_json != '[]'::jsonb THEN EXCLUDED.pulse_refs_json ELSE today_digest_view.pulse_refs_json END,
		 updated_at = EXCLUDED.updated_at`

	_, err := r.pool.Exec(ctx, query,
		digest.UserID, digest.DigestDate.Format("2006-01-02"),
		digest.NewArticles, digest.SummarizedArticles,
		digest.UnsummarizedArticles, string(topTagsJSON), string(pulseRefsJSON),
		digest.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("UpsertTodayDigest: %w", err)
	}

	return nil
}
