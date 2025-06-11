package repository

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"pre-processor/logger"
	"pre-processor/models"

	"github.com/jackc/pgx/v5"
)

func Init(ctx context.Context) (*pgx.Conn, error) {
	db, err := pgx.Connect(ctx, fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("PRE_PROCESSOR_DB_USER"), os.Getenv("PRE_PROCESSOR_DB_PASSWORD"), os.Getenv("DB_NAME")))
	if err != nil {
		logger.Logger.Error("Failed to connect to database", "error", err)
		return nil, err
	}

	err = db.Ping(ctx)
	if err != nil {
		logger.Logger.Error("Failed to ping database", "error", err)
		panic(err)
	}

	logger.Logger.Info("Connected to database")

	return db, nil
}

func GetSourceURLs(offset int, ctx context.Context, db *pgx.Conn) ([]url.URL, error) {
	query := `
		SELECT link FROM feeds ORDER BY created_at DESC LIMIT 20 OFFSET $1
	`
	rows, err := db.Query(ctx, query, offset)
	if err != nil {
		logger.Logger.Error("Failed to get source URLs", "error", err)
		return nil, err
	}
	defer rows.Close()

	logger.Logger.Info("Getting source URLs", "offset", offset)

	urls := []url.URL{}
	for rows.Next() {
		var u string
		err = rows.Scan(&u)
		if err != nil {
			logger.Logger.Error("Failed to scan source URL", "error", err)
			return nil, err
		}

		ul, err := convertToURL(u)
		if err != nil {
			logger.Logger.Error("Failed to convert URL", "error", err)
			return nil, err
		}

		urls = append(urls, ul)
	}

	logger.Logger.Info("Got source URLs", "length", len(urls), "offset", offset)

	return urls, nil
}

func CreateArticle(ctx context.Context, db *pgx.Conn, article *models.Article) error {
	query := `
		INSERT INTO articles (title, content, url)
		VALUES ($1, $2, $3)
		ON CONFLICT (url) DO NOTHING
	`

	logger.Logger.Info("Creating article", "article link", article.URL)
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.Logger.Error("Failed to begin transaction", "error", err)
		return err
	}

	_, err = tx.Exec(ctx, query, article.Title, article.Content, article.URL)
	if err != nil {
		tx.Rollback(ctx)
		logger.Logger.Error("Failed to create article", "error", err)
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Failed to commit transaction", "error", err)
		return err
	}

	logger.Logger.Info("Article created", "article", article.Title)

	return nil
}

func convertToURL(u string) (url.URL, error) {
	ul, err := url.Parse(u)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to parse URL: %w", err)
	}

	return *ul, nil
}
