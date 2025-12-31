package backfill

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// Article represents an article to be indexed.
type Article struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Config holds the runner configuration.
type Config struct {
	DatabaseURL     string
	OrchestratorURL string
	CursorFile      string
	FromDate        time.Time
	ToDate          time.Time
	Concurrency     int
	BatchSize       int
	DryRun          bool
	RequestTimeout  time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		OrchestratorURL: "http://localhost:9010",
		CursorFile:      "cursor.json",
		Concurrency:     4,
		BatchSize:       40,
		RequestTimeout:  100 * time.Second,
	}
}

// Stats holds runtime statistics.
type Stats struct {
	Processed int64
	Failed    int64
	Skipped   int64
	StartTime time.Time
}

// Rate returns the processing rate per second.
func (s *Stats) Rate() float64 {
	elapsed := time.Since(s.StartTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&s.Processed)) / elapsed
}

// Runner executes the backfill process.
type Runner struct {
	cfg           Config
	db            *sql.DB
	client        *http.Client
	cursorManager *CursorManager
	logger        *slog.Logger
	stats         *Stats
	limiter       *rate.Limiter
}

// NewRunner creates a new backfill runner.
func NewRunner(cfg Config, logger *slog.Logger) (*Runner, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          cfg.Concurrency * 2,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: cfg.RequestTimeout,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout + 10*time.Second,
	}

	// Rate limit: concurrency requests per 100ms
	limiter := rate.NewLimiter(rate.Every(100*time.Millisecond), cfg.Concurrency)

	return &Runner{
		cfg:           cfg,
		db:            db,
		client:        client,
		cursorManager: NewCursorManager(cfg.CursorFile),
		logger:        logger,
		stats:         &Stats{StartTime: time.Now()},
		limiter:       limiter,
	}, nil
}

// Close releases resources.
func (r *Runner) Close() error {
	return r.db.Close()
}

// Run executes the backfill process.
func (r *Runner) Run(ctx context.Context) error {
	// Acquire lock
	if err := r.cursorManager.Lock(); err != nil {
		return fmt.Errorf("acquire cursor lock: %w", err)
	}
	defer r.cursorManager.Unlock()

	// Load cursor
	cursor, err := r.cursorManager.Load()
	if err != nil {
		return fmt.Errorf("load cursor: %w", err)
	}

	if !cursor.IsEmpty() {
		r.logger.Info("resuming from cursor",
			slog.Time("last_created_at", cursor.LastCreatedAt),
			slog.String("last_id", cursor.LastID),
			slog.Int("processed_count", cursor.ProcessedCount),
		)
	}

	// Start progress reporter
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()
	go r.reportProgress(progressCtx)

	// Process by date if date range specified
	if !r.cfg.FromDate.IsZero() {
		return r.runByDateRange(ctx, cursor)
	}

	// Otherwise, process all with cursor resume
	return r.runAll(ctx, cursor)
}

// runByDateRange processes articles within a date range, day by day.
func (r *Runner) runByDateRange(ctx context.Context, cursor Cursor) error {
	fromDate := r.cfg.FromDate
	toDate := r.cfg.ToDate
	if toDate.IsZero() {
		toDate = time.Now()
	}

	// Normalize to start of day
	fromDate = time.Date(fromDate.Year(), fromDate.Month(), fromDate.Day(), 0, 0, 0, 0, time.UTC)
	toDate = time.Date(toDate.Year(), toDate.Month(), toDate.Day(), 0, 0, 0, 0, time.UTC)

	// Process newest first (DESC order)
	for day := toDate; !day.Before(fromDate); day = day.AddDate(0, 0, -1) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		dayStr := day.Format("2006-01-02")

		// Skip if cursor indicates we've passed this date
		if cursor.CurrentDate != "" && dayStr > cursor.CurrentDate {
			r.logger.Info("skipping already processed date", slog.String("date", dayStr))
			continue
		}

		r.logger.Info("processing date", slog.String("date", dayStr))

		count, err := r.processDay(ctx, day, &cursor)
		if err != nil {
			return fmt.Errorf("process day %s: %w", dayStr, err)
		}

		r.logger.Info("date completed",
			slog.String("date", dayStr),
			slog.Int("articles", count),
		)

		// Update cursor to next day
		cursor.CurrentDate = day.AddDate(0, 0, -1).Format("2006-01-02")
		cursor.LastCreatedAt = time.Time{}
		cursor.LastID = ""
		if err := r.cursorManager.Save(cursor); err != nil {
			r.logger.Warn("failed to save cursor", slog.String("error", err.Error()))
		}
	}

	r.logger.Info("backfill completed",
		slog.Int64("total_processed", atomic.LoadInt64(&r.stats.Processed)),
		slog.Int64("total_failed", atomic.LoadInt64(&r.stats.Failed)),
	)

	return nil
}

// processDay processes all articles for a single day.
func (r *Runner) processDay(ctx context.Context, day time.Time, cursor *Cursor) (int, error) {
	dayStart := day
	dayEnd := day.AddDate(0, 0, 1)

	query := `
		SELECT id, title, content, url, user_id, created_at
		FROM articles
		WHERE content IS NOT NULL AND content != ''
		  AND deleted_at IS NULL
		  AND created_at >= $1
		  AND created_at < $2
	`
	args := []interface{}{dayStart, dayEnd}

	// Apply cursor within the day
	if !cursor.LastCreatedAt.IsZero() && cursor.CurrentDate == day.Format("2006-01-02") {
		query += ` AND (created_at, id) < ($3, $4)`
		args = append(args, cursor.LastCreatedAt, cursor.LastID)
	}

	query += ` ORDER BY created_at DESC, id DESC`

	return r.processBatches(ctx, query, args, cursor)
}

// runAll processes all articles using cursor-based pagination.
func (r *Runner) runAll(ctx context.Context, cursor Cursor) error {
	query := `
		SELECT id, title, content, url, user_id, created_at
		FROM articles
		WHERE content IS NOT NULL AND content != ''
		  AND deleted_at IS NULL
	`
	args := []interface{}{}

	if !cursor.LastCreatedAt.IsZero() {
		query += ` AND (created_at, id) < ($1, $2)`
		args = append(args, cursor.LastCreatedAt, cursor.LastID)
	}

	query += ` ORDER BY created_at DESC, id DESC`

	_, err := r.processBatches(ctx, query, args, &cursor)
	if err != nil {
		return err
	}

	r.logger.Info("backfill completed",
		slog.Int64("total_processed", atomic.LoadInt64(&r.stats.Processed)),
		slog.Int64("total_failed", atomic.LoadInt64(&r.stats.Failed)),
	)

	return nil
}

// processBatches processes articles in batches from the given query.
func (r *Runner) processBatches(ctx context.Context, query string, args []interface{}, cursor *Cursor) (int, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("query articles: %w", err)
	}
	defer rows.Close()

	totalCount := 0
	sem := make(chan struct{}, r.cfg.Concurrency)

	for {
		batch := make([]Article, 0, r.cfg.BatchSize)

		// Fetch batch
		for i := 0; i < r.cfg.BatchSize && rows.Next(); i++ {
			var a Article
			if err := rows.Scan(&a.ID, &a.Title, &a.Body, &a.URL, &a.UserID, &a.CreatedAt); err != nil {
				r.logger.Warn("failed to scan article", slog.String("error", err.Error()))
				continue
			}
			batch = append(batch, a)
		}

		if len(batch) == 0 {
			break
		}

		// Process batch concurrently
		var wg sync.WaitGroup
		for _, a := range batch {
			select {
			case <-ctx.Done():
				return totalCount, ctx.Err()
			default:
			}

			// Rate limit
			if err := r.limiter.Wait(ctx); err != nil {
				return totalCount, err
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(article Article) {
				defer wg.Done()
				defer func() { <-sem }()

				if r.cfg.DryRun {
					r.logger.Info("dry-run: would process",
						slog.String("id", article.ID),
						slog.String("title", truncate(article.Title, 50)),
					)
					atomic.AddInt64(&r.stats.Skipped, 1)
					return
				}

				if err := r.sendArticle(ctx, article); err != nil {
					r.logger.Warn("failed to send article",
						slog.String("id", article.ID),
						slog.String("error", err.Error()),
					)
					atomic.AddInt64(&r.stats.Failed, 1)
				} else {
					atomic.AddInt64(&r.stats.Processed, 1)
				}
			}(a)
		}
		wg.Wait()

		// Update cursor
		lastArticle := batch[len(batch)-1]
		cursor.LastCreatedAt = lastArticle.CreatedAt
		cursor.LastID = lastArticle.ID
		cursor.ProcessedCount += len(batch)

		if !r.cfg.DryRun {
			if err := r.cursorManager.Save(*cursor); err != nil {
				r.logger.Warn("failed to save cursor", slog.String("error", err.Error()))
			}
		}

		totalCount += len(batch)
	}

	if err := rows.Err(); err != nil {
		return totalCount, fmt.Errorf("iterate rows: %w", err)
	}

	return totalCount, nil
}

// sendArticle sends an article to the orchestrator for indexing.
func (r *Runner) sendArticle(ctx context.Context, a Article) error {
	payload := map[string]interface{}{
		"article_id":   a.ID,
		"user_id":      a.UserID,
		"title":        a.Title,
		"body":         a.Body,
		"url":          a.URL,
		"published_at": a.CreatedAt.Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(ctx, r.cfg.RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST",
		r.cfg.OrchestratorURL+"/internal/rag/index/upsert",
		bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		if os.IsTimeout(err) || err == context.DeadlineExceeded {
			return fmt.Errorf("timeout: %w", err)
		}
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)

	// Handle duplicate key (race condition) as success
	if resp.StatusCode == http.StatusInternalServerError &&
		(bytes.Contains(bodyBytes, []byte("duplicate key")) ||
			bytes.Contains(bodyBytes, []byte("Unique constraint"))) {
		return nil
	}

	return fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
}

// reportProgress logs progress periodically.
func (r *Runner) reportProgress(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.logger.Info("progress",
				slog.Int64("processed", atomic.LoadInt64(&r.stats.Processed)),
				slog.Int64("failed", atomic.LoadInt64(&r.stats.Failed)),
				slog.Int64("skipped", atomic.LoadInt64(&r.stats.Skipped)),
				slog.Float64("rate_per_sec", r.stats.Rate()),
			)
		}
	}
}

// ResetCursor clears the cursor file.
func (r *Runner) ResetCursor() error {
	return r.cursorManager.Reset()
}

// GetCursor returns the current cursor state.
func (r *Runner) GetCursor() (Cursor, error) {
	return r.cursorManager.Load()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
