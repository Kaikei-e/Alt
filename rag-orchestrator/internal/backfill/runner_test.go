package backfill

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestRunner_FailedArticleIsRetriedOnNextRun(t *testing.T) {
	base := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	rows := []Article{
		testArticle("a3", base.Add(2*time.Hour)),
		testArticle("a2", base.Add(1*time.Hour)),
		testArticle("a1", base),
	}

	orch := newFakeOrchestrator("a2")
	srv := httptest.NewServer(orch)
	defer srv.Close()

	cursorPath := filepath.Join(t.TempDir(), "cursor.json")

	first := newTestRunner(t, cursorPath, srv.URL, rows)
	require.NoError(t, first.Run(context.Background()))
	require.Equal(t, int64(1), atomic.LoadInt64(&first.stats.Failed))
	require.ElementsMatch(t, []string{"a3", "a1"}, orch.indexedIDs())

	// The orchestrator recovers: the article that failed must still be
	// reachable, otherwise it drops out of the index for good.
	orch.healAll()
	orch.forgetIndexed()

	second := newTestRunner(t, cursorPath, srv.URL, rows)
	require.NoError(t, second.Run(context.Background()))

	assert.Contains(t, orch.indexedIDs(), "a2",
		"article that failed in the first run must be re-selected by the next run")
	assert.Equal(t, int64(0), atomic.LoadInt64(&second.stats.Failed))
}

func TestRunner_CursorStopsBeforeFailedArticle(t *testing.T) {
	base := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	rows := []Article{
		testArticle("a3", base.Add(2*time.Hour)),
		testArticle("a2", base.Add(1*time.Hour)),
		testArticle("a1", base),
	}

	tests := []struct {
		name string
		// failID is the article the orchestrator rejects.
		failID string
		// wantLastID is the article the cursor must stop at; empty means the
		// cursor must not move at all.
		wantLastID string
	}{
		{
			name:       "failure in the middle stops the cursor at the previous article",
			failID:     "a2",
			wantLastID: "a3",
		},
		{
			name:       "failure on the first article leaves the cursor untouched",
			failID:     "a3",
			wantLastID: "",
		},
		{
			name:       "failure on the last article stops the cursor at the previous article",
			failID:     "a1",
			wantLastID: "a2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch := newFakeOrchestrator(tt.failID)
			srv := httptest.NewServer(orch)
			defer srv.Close()

			cursorPath := filepath.Join(t.TempDir(), "cursor.json")
			runner := newTestRunner(t, cursorPath, srv.URL, rows)
			require.NoError(t, runner.Run(context.Background()))

			cursor, err := runner.GetCursor()
			require.NoError(t, err)

			if tt.wantLastID == "" {
				assert.True(t, cursor.IsEmpty(),
					"cursor must not advance past a failed article, got %+v", cursor)
				return
			}
			assert.Equal(t, tt.wantLastID, cursor.LastID)
		})
	}
}

func TestRunner_DateRange_FailedArticleIsRetriedOnNextRun(t *testing.T) {
	day1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	rows := []Article{
		testArticle("d2-b", day2.Add(11*time.Hour)),
		testArticle("d2-a", day2.Add(10*time.Hour)),
		testArticle("d1-a", day1.Add(10*time.Hour)),
	}

	orch := newFakeOrchestrator("d2-a")
	srv := httptest.NewServer(orch)
	defer srv.Close()

	cursorPath := filepath.Join(t.TempDir(), "cursor.json")

	first := newTestRunner(t, cursorPath, srv.URL, rows)
	first.cfg.FromDate = day1
	first.cfg.ToDate = day2
	require.NoError(t, first.Run(context.Background()))
	require.Equal(t, int64(1), atomic.LoadInt64(&first.stats.Failed))

	cursor, err := first.GetCursor()
	require.NoError(t, err)
	assert.Equal(t, "2026-03-02", cursor.CurrentDate,
		"the day holding a failed article must stay the resume point")

	orch.healAll()
	orch.forgetIndexed()

	second := newTestRunner(t, cursorPath, srv.URL, rows)
	second.cfg.FromDate = day1
	second.cfg.ToDate = day2
	require.NoError(t, second.Run(context.Background()))

	assert.Contains(t, orch.indexedIDs(), "d2-a",
		"article that failed in the first run must be re-selected by the next run")
}

func testArticle(id string, createdAt time.Time) Article {
	return Article{
		ID:        id,
		Title:     "title " + id,
		Body:      "body " + id,
		URL:       "https://example.test/" + id,
		UserID:    "user-1",
		CreatedAt: createdAt,
	}
}

// newTestRunner builds a Runner wired to an in-memory article table and the
// given orchestrator, bypassing NewRunner so no live Postgres is needed.
func newTestRunner(t *testing.T, cursorPath, orchestratorURL string, rows []Article) *Runner {
	t.Helper()

	db := sql.OpenDB(&fakeArticleConnector{rows: rows})
	t.Cleanup(func() { _ = db.Close() })

	cfg := DefaultConfig()
	cfg.OrchestratorURL = orchestratorURL
	cfg.CursorFile = cursorPath
	cfg.Concurrency = 4
	cfg.RequestTimeout = 5 * time.Second

	return &Runner{
		cfg:           cfg,
		db:            db,
		client:        &http.Client{Timeout: 10 * time.Second},
		cursorManager: NewCursorManager(cursorPath),
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		stats:         &Stats{StartTime: time.Now()},
		limiter:       rate.NewLimiter(rate.Inf, 1),
	}
}

// fakeOrchestrator stands in for the indexing endpoint and rejects a
// configurable set of article IDs, the way a flaky embedder does.
type fakeOrchestrator struct {
	mu      sync.Mutex
	failIDs map[string]struct{}
	indexed []string
}

func newFakeOrchestrator(failIDs ...string) *fakeOrchestrator {
	fail := make(map[string]struct{}, len(failIDs))
	for _, id := range failIDs {
		fail[id] = struct{}{}
	}
	return &fakeOrchestrator{failIDs: fail}
}

func (o *fakeOrchestrator) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		ArticleID string `json:"article_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if _, fail := o.failIDs[payload.ArticleID]; fail {
		http.Error(w, "embedder unavailable", http.StatusInternalServerError)
		return
	}

	o.indexed = append(o.indexed, payload.ArticleID)
	w.WriteHeader(http.StatusOK)
}

func (o *fakeOrchestrator) healAll() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.failIDs = map[string]struct{}{}
}

func (o *fakeOrchestrator) forgetIndexed() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.indexed = nil
}

func (o *fakeOrchestrator) indexedIDs() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.indexed...)
}

// fakeArticleConnector serves the backfill queries from an in-memory table.
// rows must already be ordered the way the query asks for them
// (created_at DESC, id DESC).
type fakeArticleConnector struct {
	rows []Article
}

func (c *fakeArticleConnector) Connect(context.Context) (driver.Conn, error) {
	return &fakeArticleConn{rows: c.rows}, nil
}

func (c *fakeArticleConnector) Driver() driver.Driver { return fakeArticleDriver{} }

type fakeArticleDriver struct{}

func (fakeArticleDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("fake db: Open is not supported, use sql.OpenDB")
}

type fakeArticleConn struct {
	rows []Article
}

func (c *fakeArticleConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("fake db: Prepare is not supported")
}

func (c *fakeArticleConn) Close() error { return nil }

func (c *fakeArticleConn) Begin() (driver.Tx, error) {
	return nil, errors.New("fake db: Begin is not supported")
}

// QueryContext applies the day window and the keyset predicate
// `(created_at, id) < ($n, $n+1)` exactly as Postgres would, so a cursor that
// jumped over a row really does lose that row.
func (c *fakeArticleConn) QueryContext(_ context.Context, _ string, args []driver.NamedValue) (driver.Rows, error) {
	filter, err := parseArticleQueryArgs(args)
	if err != nil {
		return nil, err
	}

	selected := make([]Article, 0, len(c.rows))
	for _, a := range c.rows {
		if filter.hasDayWindow && (a.CreatedAt.Before(filter.dayStart) || !a.CreatedAt.Before(filter.dayEnd)) {
			continue
		}
		if filter.hasKeyset && !beforeKey(a, filter.lastCreatedAt, filter.lastID) {
			continue
		}
		selected = append(selected, a)
	}

	return &fakeArticleRows{rows: selected}, nil
}

type articleQueryFilter struct {
	dayStart      time.Time
	dayEnd        time.Time
	hasDayWindow  bool
	lastCreatedAt time.Time
	lastID        string
	hasKeyset     bool
}

func parseArticleQueryArgs(args []driver.NamedValue) (articleQueryFilter, error) {
	var f articleQueryFilter

	values := make([]any, len(args))
	for i, a := range args {
		values[i] = a.Value
	}

	switch len(values) {
	case 0:
	case 2:
		// Either the day window (time, time) or the keyset (time, string).
		if _, isKeyset := values[1].(string); isKeyset {
			return withKeyset(f, values[0], values[1])
		}
		return withDayWindow(f, values[0], values[1])
	case 4:
		f, err := withDayWindow(f, values[0], values[1])
		if err != nil {
			return f, err
		}
		return withKeyset(f, values[2], values[3])
	default:
		return f, fmt.Errorf("fake db: unexpected arg count %d", len(values))
	}

	return f, nil
}

func withDayWindow(f articleQueryFilter, start, end any) (articleQueryFilter, error) {
	dayStart, ok := start.(time.Time)
	if !ok {
		return f, fmt.Errorf("fake db: day start: want time.Time, got %T", start)
	}
	dayEnd, ok := end.(time.Time)
	if !ok {
		return f, fmt.Errorf("fake db: day end: want time.Time, got %T", end)
	}
	f.dayStart, f.dayEnd, f.hasDayWindow = dayStart, dayEnd, true
	return f, nil
}

func withKeyset(f articleQueryFilter, createdAt, id any) (articleQueryFilter, error) {
	lastCreatedAt, ok := createdAt.(time.Time)
	if !ok {
		return f, fmt.Errorf("fake db: cursor created_at: want time.Time, got %T", createdAt)
	}
	lastID, ok := id.(string)
	if !ok {
		return f, fmt.Errorf("fake db: cursor id: want string, got %T", id)
	}
	f.lastCreatedAt, f.lastID, f.hasKeyset = lastCreatedAt, lastID, true
	return f, nil
}

// beforeKey reports whether (a.created_at, a.id) < (createdAt, id).
func beforeKey(a Article, createdAt time.Time, id string) bool {
	if a.CreatedAt.Before(createdAt) {
		return true
	}
	return a.CreatedAt.Equal(createdAt) && a.ID < id
}

type fakeArticleRows struct {
	rows []Article
	next int
}

func (r *fakeArticleRows) Columns() []string {
	return []string{"id", "title", "content", "url", "user_id", "created_at"}
}

func (r *fakeArticleRows) Close() error { return nil }

func (r *fakeArticleRows) Next(dest []driver.Value) error {
	if r.next >= len(r.rows) {
		return io.EOF
	}
	a := r.rows[r.next]
	r.next++

	dest[0] = a.ID
	dest[1] = a.Title
	dest[2] = a.Body
	dest[3] = a.URL
	dest[4] = a.UserID
	dest[5] = a.CreatedAt
	return nil
}
