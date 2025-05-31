package alt_db

import (
	"alt/driver/models"
)

func (r *AltDBRepository) GetSingleFeed() (*models.Feed, error) {
	query := `
		SELECT * FROM feeds ORDER BY created_at DESC LIMIT 1
	`

	var feed models.Feed
	err := r.db.QueryRow(query).Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate)
	if err != nil {
		return nil, err
	}

	return &feed, nil
}
