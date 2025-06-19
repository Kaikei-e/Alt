package indexer

import (
	"context"
	"search-indexer/config"
	"search-indexer/driver"
	"search-indexer/logger"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

type Indexer struct {
	config   *config.Config
	dbDriver *driver.DatabaseDriver
	index    meilisearch.IndexManager

	lastCreatedAt *time.Time
	lastID        string
}

func New(cfg *config.Config, dbDriver *driver.DatabaseDriver, idx meilisearch.IndexManager) *Indexer {
	return &Indexer{
		config:   cfg,
		dbDriver: dbDriver,
		index:    idx,
	}
}

func (i *Indexer) Start(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Logger.Error("indexer panic recovered", "err", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Logger.Info("indexer stopping")
			return
		default:
			if err := i.indexBatch(ctx); err != nil {
				logger.Logger.Error("indexing batch failed", "err", err)
				time.Sleep(i.config.Indexer.RetryDelay)
				continue
			}
			time.Sleep(i.config.Indexer.Interval)
		}
	}
}

func (i *Indexer) indexBatch(ctx context.Context) error {
	dbCtx, cancel := context.WithTimeout(ctx, i.config.Database.Timeout)
	defer cancel()

	articles, newLastTS, newLastID, err := i.dbDriver.GetArticlesWithTags(
		dbCtx, i.lastCreatedAt, i.lastID, i.config.Indexer.BatchSize,
	)
	if err != nil {
		return err
	}

	if len(articles) == 0 {
		logger.Logger.Info("no new articles to index")
		return nil
	}

	docs := i.convertToDocuments(articles)

	if err := i.indexDocuments(docs); err != nil {
		return err
	}

	logger.Logger.Info("indexed articles", "count", len(docs))
	i.lastCreatedAt, i.lastID = newLastTS, newLastID
	return nil
}

func (i *Indexer) convertToDocuments(articles []*driver.ArticleWithTags) []driver.SearchDocumentDriver {
	docs := make([]driver.SearchDocumentDriver, 0, len(articles))

	for _, art := range articles {
		tags := make([]string, len(art.Tags))
		for j, t := range art.Tags {
			tags[j] = t.Name
		}

		docs = append(docs, driver.SearchDocumentDriver{
			ID:      art.ID,
			Title:   art.Title,
			Content: art.Content,
			Tags:    tags,
		})
	}

	return docs
}

func (i *Indexer) indexDocuments(docs []driver.SearchDocumentDriver) error {
	task, err := i.index.AddDocuments(docs)
	if err != nil {
		return err
	}

	_, err = i.index.WaitForTask(task.TaskUID, i.config.Meilisearch.Timeout)
	return err
}
