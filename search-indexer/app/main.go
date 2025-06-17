package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	"search-indexer/db"
	"search-indexer/handler"
	"search-indexer/logger"
	"search-indexer/models"
	"search-indexer/search_engine"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meilisearch/meilisearch-go"
)

const (
	INDEX_INTERVAL       = 1 * time.Minute
	INDEX_BATCH_SIZE     = 200
	INDEX_RETRY_INTERVAL = 1 * time.Minute
	HTTP_ADDR            = ":9300"
	DB_TIMEOUT           = 10 * time.Second
	MEILI_TIMEOUT        = 15 * time.Second
)

func main() {
	// ──────────── init ────────────
	logger.Init()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dbPool := db.Init(ctx)
	defer dbPool.Close()

	msClient := search_engine.NewMeilisearchClient(
		os.Getenv("MEILISEARCH_HOST"),
		os.Getenv("MEILISEARCH_API_KEY"),
	)
	idx := msClient.Index("articles")

	// フィルタ属性は起動時に一度だけ設定
	task, err := ensureFilterable(ctx, idx)
	if err != nil {
		logger.Logger.Error("filterable attributes", "err", err)
		return
	}
	if task != nil {
		logger.Logger.Info("filterable attributes", "task", task)
	}

	// ──────────── batch indexer ────────────
	go indexLoop(ctx, dbPool, idx)

	// ──────────── HTTP server ────────────
	http.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
		handler.SearchArticles(w, r, idx)
	})

	srv := &http.Server{
		Addr:              HTTP_ADDR,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Logger.Info("http listen", "addr", HTTP_ADDR)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Error("http", "err", err)
		}
	}()

	<-ctx.Done() // Ctrl-C
	_ = srv.Shutdown(context.Background())
}

func ensureFilterable(ctx context.Context, idx meilisearch.IndexManager) (*meilisearch.Task, error) {
	settings, err := idx.GetSettings()
	if err != nil {
		return nil, err
	}
	for _, f := range settings.FilterableAttributes {
		if f == "tags" {
			logger.Logger.Info("tags already registered")
			return nil, nil
		}
	}
	task, err := idx.UpdateFilterableAttributes(&[]string{"tags"})
	if err != nil {
		return nil, err
	}

	duration := 1 * time.Minute
	return idx.WaitForTask(task.TaskUID, duration)
}

// indexLoop は記事 + タグを取得して meilisearch に Upsert する。
func indexLoop(ctx context.Context, dbPool *pgxpool.Pool, idx meilisearch.IndexManager) {
	defer func() {
		if r := recover(); r != nil {
			logger.Logger.Error("index loop panic", "err", r)
		}
	}()

	var lastCreatedAt *time.Time
	var lastID string

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// ── fetch articles
		dbCtx, cancel := context.WithTimeout(ctx, DB_TIMEOUT)
		articles, newLastTS, newLastID, err := db.GetArticlesWithTags(
			dbCtx, dbPool, lastCreatedAt, lastID, INDEX_BATCH_SIZE,
		)
		cancel()
		if err != nil {
			logger.Logger.Error("db fetch", "err", err)
			time.Sleep(INDEX_RETRY_INTERVAL)
			continue
		}
		if len(articles) == 0 {
			logger.Logger.Info("no new articles")
			time.Sleep(INDEX_INTERVAL)
			continue
		}

		// ── convert
		docs := make([]models.Doc, 0, len(articles))
		for _, art := range articles {
			tags := make([]string, len(art.Tags))
			for i, t := range art.Tags {
				tags[i] = t.Name
			}
			docs = append(docs, models.Doc{
				ID:      art.ID,
				Title:   art.Title,
				Content: art.Content,
				Tags:    tags,
			})
		}

		// ── send to Meilisearch
		task, err := idx.AddDocuments(docs)
		if err != nil {
			logger.Logger.Error("meili add", "err", err)
			time.Sleep(INDEX_RETRY_INTERVAL)
			continue
		}
		// タスク完了を同期的に確認
		if _, err := idx.WaitForTask(task.TaskUID, MEILI_TIMEOUT); err != nil {
			logger.Logger.Error("meili wait task", "err", err)
			time.Sleep(INDEX_RETRY_INTERVAL)
			continue
		}
		logger.Logger.Info("indexed", "count", len(docs))

		// ── advance cursor
		lastCreatedAt, lastID = newLastTS, newLastID
	}
}
