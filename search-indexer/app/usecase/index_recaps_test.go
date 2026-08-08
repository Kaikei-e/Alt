package usecase

import (
	"context"
	"search-indexer/domain"
	"sort"
	"testing"
	"time"
)

// mockInclusiveBoundaryRecapRepo mirrors recap-worker's actual SQL contract
// for /v1/recaps/genres/indexable: "WHERE rj.kicked_at >= $1" (see
// recap-worker/recap-worker/src/store/dao/output.rs, fetch_indexable_genres).
// The boundary is inclusive, so a naive cursor that echoes back the max
// ExecutedAt of the returned batch will match the same rows again on the
// next call. This mock reproduces that inclusive comparison so the usecase
// test exercises the real server contract instead of a lenient stand-in.
type mockInclusiveBoundaryRecapRepo struct {
	docs []domain.RecapDocument
}

func (m *mockInclusiveBoundaryRecapRepo) GetIndexableGenres(ctx context.Context, since string, limit int) ([]domain.RecapDocument, bool, error) {
	// recap-worker parses "since" into a real chrono::DateTime and compares
	// it against a Postgres timestamptz column, i.e. chronological ordering,
	// not lexicographic string ordering. A fractional-seconds cursor (e.g.
	// "...:05.000001Z") sorts AFTER a whole-second timestamp ("...:05Z") as
	// a real time value even though 'Z' > '.' would say the opposite as
	// plain strings, so this mock must parse both sides to stay faithful to
	// the real server contract.
	var sinceTS time.Time
	if since != "" {
		var err error
		sinceTS, err = time.Parse(time.RFC3339Nano, since)
		if err != nil {
			panic("mockInclusiveBoundaryRecapRepo: invalid since: " + err.Error())
		}
		// Postgres timestamptz carries microsecond resolution, so any
		// finer-grained bound the client sends is collapsed to the µs grid
		// before the comparison happens server-side. Reproducing that here
		// matters: a cursor nudged by less than one microsecond (e.g. 1ns)
		// collapses back onto the boundary value and the inclusive bound
		// re-matches the whole boundary batch — the exact reindex loop the
		// fix exists to stop.
		sinceTS = sinceTS.Truncate(time.Microsecond)
	}

	var out []domain.RecapDocument
	for _, d := range m.docs {
		if since == "" {
			out = append(out, d)
			continue
		}
		docTS, err := time.Parse(time.RFC3339Nano, d.ExecutedAt)
		if err != nil {
			panic("mockInclusiveBoundaryRecapRepo: invalid ExecutedAt: " + err.Error())
		}
		if !docTS.Before(sinceTS) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExecutedAt < out[j].ExecutedAt })

	// recap-worker's handler sets has_more = hits.len() == limit, i.e. "this
	// page came back full" -- not "there were more rows than we asked for"
	// (see recap-worker/recap-worker/src/api/fetch.rs, get_indexable_genres).
	// Those two conditions coincide once you truncate to limit either way,
	// but the truncation must happen BEFORE computing has_more or a page
	// that is exactly `limit` rows long is wrongly reported as complete.
	hasMore := len(out) >= limit
	if len(out) > limit {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// TestMockInclusiveBoundaryRecapRepo_HasMoreMatchesServerOffByOne pins the
// mock's has_more semantics to recap-worker's own: has_more = (returned
// count == limit), true even when a page that came back exactly limit-sized
// happens to exhaust every currently-qualifying row. A mock using
// "count > limit" instead would report has_more = false for such a page,
// letting nextRecapCursor nudge past a boundary it cannot actually know is
// complete -- masking exactly the defect this suite exists to catch.
func TestMockInclusiveBoundaryRecapRepo_HasMoreMatchesServerOffByOne(t *testing.T) {
	docs := []domain.RecapDocument{
		{ID: "job-a__tech", JobID: "job-a", ExecutedAt: "2026-08-07T17:00:00Z", Genre: "tech"},
		{ID: "job-b__tech", JobID: "job-b", ExecutedAt: "2026-08-07T17:00:01Z", Genre: "tech"},
		{ID: "job-c__tech", JobID: "job-c", ExecutedAt: "2026-08-07T17:00:02Z", Genre: "tech"},
	}
	repo := &mockInclusiveBoundaryRecapRepo{docs: docs}

	out, hasMore, err := repo.GetIndexableGenres(context.Background(), "", 3)
	if err != nil {
		t.Fatalf("GetIndexableGenres: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}
	if !hasMore {
		t.Fatal("hasMore = false, want true: a page that came back exactly `limit` rows long must report has_more=true to match recap-worker's hits.len() == limit contract")
	}
}

// TestIndexRecapsUsecase_ExecuteIncremental_DoesNotReindexBoundaryBatchForever
// reproduces the HIGH finding: 17 recap documents sharing the newest
// executed_at bucket were re-pushed to Meilisearch every 5m15s, 69 times in
// 6h (93% of the task queue), because the cursor advance loop in
// ExecuteIncremental only updates lastSince when a document's ExecutedAt is
// STRICTLY GREATER than the current cursor. When every document in the
// batch sits exactly on the boundary (ExecutedAt == since), lastSince never
// moves, so the next poll re-sends the identical "since" value against
// recap-worker's inclusive "kicked_at >= $1" query and receives the exact
// same batch back -- forever. This test drives the usecase the way
// bootstrap/app.go's persistent Phase 2 loop does: repeatedly call
// ExecuteIncremental with the previous call's LastSince as the next since.
func TestIndexRecapsUsecase_ExecuteIncremental_DoesNotReindexBoundaryBatchForever(t *testing.T) {
	// One recap job producing three genres, all sharing the same kicked_at --
	// exactly the shape described in the evidence (17 docs, one bucket).
	docs := []domain.RecapDocument{
		{ID: "job-1__tech", JobID: "job-1", ExecutedAt: "2026-08-07T17:00:05Z", Genre: "tech"},
		{ID: "job-1__sports", JobID: "job-1", ExecutedAt: "2026-08-07T17:00:05Z", Genre: "sports"},
		{ID: "job-1__world", JobID: "job-1", ExecutedAt: "2026-08-07T17:00:05Z", Genre: "world"},
	}
	repo := &mockInclusiveBoundaryRecapRepo{docs: docs}
	engine := &mockRecapSearchEngine{}
	u := NewIndexRecapsUsecase(repo, engine)

	since := ""
	result, err := u.ExecuteIncremental(context.Background(), since, 200)
	if err != nil {
		t.Fatalf("ExecuteIncremental (first call): %v", err)
	}
	if result.IndexedCount != 3 {
		t.Fatalf("first call IndexedCount = %d, want 3", result.IndexedCount)
	}
	since = result.LastSince

	// Simulate the persistent polling loop: a correctly advanced cursor must
	// stop matching the boundary batch. If the cursor is stuck, every one of
	// these iterations re-indexes the same 3 documents (IndexedCount == 3
	// forever) instead of settling on IndexedCount == 0.
	for i := range 5 {
		result, err = u.ExecuteIncremental(context.Background(), since, 200)
		if err != nil {
			t.Fatalf("ExecuteIncremental (iteration %d): %v", i, err)
		}
		if result.IndexedCount != 0 {
			t.Fatalf("iteration %d: re-indexed the boundary batch again (count=%d) using since=%q -- infinite reindex loop reproduced (search-indexer-boundary-reindex-loop)", i, result.IndexedCount, since)
		}
		since = result.LastSince
	}
}

// TestIndexRecapsUsecase_ExecuteIncremental_StillReturnsGenuinelyNewDocs
// guards against overcorrection: a cursor fix that advances too far could
// skip legitimate new recap jobs. The new job here sits at EXACTLY
// boundary + 1 microsecond -- the tightest margin the fix's own nudge
// produces -- computed independently of the cursor the usecase returns, not
// relative to it. This makes any overshoot bigger than one microsecond
// (e.g. a mutant that nudges by a full second instead) detectable: the
// fixed doc would then fall before the mutated cursor and be skipped.
func TestIndexRecapsUsecase_ExecuteIncremental_StillReturnsGenuinelyNewDocs(t *testing.T) {
	boundaryDocs := []domain.RecapDocument{
		{ID: "job-1__tech", JobID: "job-1", ExecutedAt: "2026-08-07T17:00:05Z", Genre: "tech"},
	}
	repo := &mockInclusiveBoundaryRecapRepo{docs: boundaryDocs}
	engine := &mockRecapSearchEngine{}
	u := NewIndexRecapsUsecase(repo, engine)

	result, err := u.ExecuteIncremental(context.Background(), "", 200)
	if err != nil {
		t.Fatalf("ExecuteIncremental (first call): %v", err)
	}
	since := result.LastSince

	boundaryTS, err := time.Parse(time.RFC3339Nano, boundaryDocs[0].ExecutedAt)
	if err != nil {
		t.Fatalf("parse boundary fixture timestamp: %v", err)
	}
	job2At := boundaryTS.Add(time.Microsecond).Format(time.RFC3339Nano)
	repo.docs = append(repo.docs, domain.RecapDocument{
		ID: "job-2__tech", JobID: "job-2", ExecutedAt: job2At, Genre: "tech",
	})

	result, err = u.ExecuteIncremental(context.Background(), since, 200)
	if err != nil {
		t.Fatalf("ExecuteIncremental (second call): %v", err)
	}
	if result.IndexedCount != 1 {
		t.Fatalf("IndexedCount = %d, want 1 (the new job-2 document at %q, since=%q); cursor overshot and skipped real data", result.IndexedCount, job2At, since)
	}
}

// TestIndexRecapsUsecase_ExecuteBackfill_CursorSurvivesPhase1ToPhase2Handoff
// pins the same fix for ExecuteBackfill, whose cursor hand-off feeds Phase 2
// Incremental's initial "since" (see bootstrap/app.go). If backfill's final
// LastSince is left sitting exactly on the boundary, Phase 2's very first
// poll re-indexes the last backfilled batch before the steady-state loop
// even starts.
func TestIndexRecapsUsecase_ExecuteBackfill_CursorSurvivesPhase1ToPhase2Handoff(t *testing.T) {
	docs := []domain.RecapDocument{
		{ID: "job-1__tech", JobID: "job-1", ExecutedAt: "2026-08-07T17:00:05Z", Genre: "tech"},
		{ID: "job-1__sports", JobID: "job-1", ExecutedAt: "2026-08-07T17:00:05Z", Genre: "sports"},
	}
	repo := &mockInclusiveBoundaryRecapRepo{docs: docs}
	engine := &mockRecapSearchEngine{}
	u := NewIndexRecapsUsecase(repo, engine)

	backfillResult, err := u.ExecuteBackfill(context.Background(), "", 200)
	if err != nil {
		t.Fatalf("ExecuteBackfill: %v", err)
	}
	if backfillResult.HasMore {
		t.Fatalf("HasMore = true, want false (batch smaller than limit)")
	}

	incResult, err := u.ExecuteIncremental(context.Background(), backfillResult.LastSince, 200)
	if err != nil {
		t.Fatalf("ExecuteIncremental after handoff: %v", err)
	}
	if incResult.IndexedCount != 0 {
		t.Fatalf("Phase 2's first poll re-indexed %d already-backfilled docs using since=%q -- boundary handoff bug", incResult.IndexedCount, backfillResult.LastSince)
	}
}

// TestIndexRecapsUsecase_ExecuteIncremental_SplitTieBoundaryDoesNotLoseDocs
// reproduces the untied ORDER BY / LIMIT pagination hazard directly: a tie
// group (two genres sharing one job's kicked_at) that straddles the page
// boundary, so only one of the two lands on the first page. A cursor that
// nudges past the tied timestamp regardless of truncation would permanently
// drop whichever tied doc missed the first page. This test polls the
// usecase the way bootstrap/app.go's persistent loop does and asserts every
// doc in the corpus is eventually indexed at least once, and that polling
// converges (does not loop forever).
func TestIndexRecapsUsecase_ExecuteIncremental_SplitTieBoundaryDoesNotLoseDocs(t *testing.T) {
	const tie = "2026-08-07T17:00:02Z"
	docs := []domain.RecapDocument{
		{ID: "job-a1__tech", JobID: "job-a1", ExecutedAt: "2026-08-07T17:00:00Z", Genre: "tech"},
		{ID: "job-a2__tech", JobID: "job-a2", ExecutedAt: "2026-08-07T17:00:01Z", Genre: "tech"},
		// job-b produces two genres at the exact same kicked_at -- the tie
		// group that a limit of 3 will split across the first two pages.
		{ID: "job-b__g1", JobID: "job-b", ExecutedAt: tie, Genre: "g1"},
		{ID: "job-b__g2", JobID: "job-b", ExecutedAt: tie, Genre: "g2"},
		{ID: "job-c__tech", JobID: "job-c", ExecutedAt: "2026-08-07T17:00:03Z", Genre: "tech"},
	}
	repo := &mockInclusiveBoundaryRecapRepo{docs: docs}
	engine := &mockRecapSearchEngine{}
	u := NewIndexRecapsUsecase(repo, engine)

	wantIDs := make(map[string]bool, len(docs))
	for _, d := range docs {
		wantIDs[d.ID] = true
	}

	since := ""
	converged := false
	const maxPolls = 20
	for i := 0; i < maxPolls; i++ {
		result, err := u.ExecuteIncremental(context.Background(), since, 3)
		if err != nil {
			t.Fatalf("poll %d (since=%q): %v", i, since, err)
		}
		if result.IndexedCount == 0 && !result.HasMore {
			converged = true
			break
		}
		since = result.LastSince
	}
	if !converged {
		t.Fatalf("polling did not converge within %d iterations (stuck at since=%q) -- possible cursor livelock", maxPolls, since)
	}

	seenIDs := make(map[string]bool, len(engine.indexedDocs))
	for _, d := range engine.indexedDocs {
		seenIDs[d.ID] = true
	}
	for id := range wantIDs {
		if !seenIDs[id] {
			t.Fatalf("doc %q was never indexed -- lost at a tie-group page boundary (search-indexer-boundary-reindex-loop)", id)
		}
	}
}

// staticRecapRepo returns a fixed page every call, ignoring since/limit --
// enough to test what the usecase does with one already-fetched batch.
type staticRecapRepo struct {
	docs    []domain.RecapDocument
	hasMore bool
}

func (m *staticRecapRepo) GetIndexableGenres(ctx context.Context, since string, limit int) ([]domain.RecapDocument, bool, error) {
	return m.docs, m.hasMore, nil
}

// TestIndexRecapsUsecase_MalformedTimestampFailsBeforeIndexing guards the
// write ordering: a batch containing a doc whose ExecutedAt cannot be
// parsed must fail BEFORE anything is sent to Meilisearch. Retrying a
// write-then-validate ordering after a parse failure would re-push the same
// batch to the search engine on every backoff attempt for no benefit, since
// the cursor never advances past a batch that keeps failing to parse.
func TestIndexRecapsUsecase_MalformedTimestampFailsBeforeIndexing(t *testing.T) {
	badDocs := []domain.RecapDocument{
		{ID: "job-x__tech", JobID: "job-x", ExecutedAt: "not-a-timestamp", Genre: "tech"},
	}

	t.Run("ExecuteBackfill", func(t *testing.T) {
		repo := &staticRecapRepo{docs: badDocs}
		engine := &mockRecapSearchEngine{}
		u := NewIndexRecapsUsecase(repo, engine)

		if _, err := u.ExecuteBackfill(context.Background(), "", 200); err == nil {
			t.Fatal("expected error for malformed ExecutedAt, got nil")
		}
		if engine.indexCalls != 0 {
			t.Fatalf("IndexRecapDocuments was called %d time(s) before the timestamp parse failure was returned", engine.indexCalls)
		}
	})

	t.Run("ExecuteIncremental", func(t *testing.T) {
		repo := &staticRecapRepo{docs: badDocs}
		engine := &mockRecapSearchEngine{}
		u := NewIndexRecapsUsecase(repo, engine)

		if _, err := u.ExecuteIncremental(context.Background(), "", 200); err == nil {
			t.Fatal("expected error for malformed ExecutedAt, got nil")
		}
		if engine.indexCalls != 0 {
			t.Fatalf("IndexRecapDocuments was called %d time(s) before the timestamp parse failure was returned", engine.indexCalls)
		}
	})

	// The first doc's timestamp is parsed on a separate code path from the
	// rest of the batch, so a guard that fails on docs[0] but skips a
	// malformed docs[1:] entry (advancing the cursor past data it never
	// validated) would pass the single-doc cases above.
	t.Run("malformed doc not first in batch", func(t *testing.T) {
		repo := &staticRecapRepo{docs: []domain.RecapDocument{
			{ID: "job-ok__tech", JobID: "job-ok", ExecutedAt: "2026-08-07T17:00:05Z", Genre: "tech"},
			{ID: "job-x__tech", JobID: "job-x", ExecutedAt: "not-a-timestamp", Genre: "tech"},
		}}
		engine := &mockRecapSearchEngine{}
		u := NewIndexRecapsUsecase(repo, engine)

		if _, err := u.ExecuteIncremental(context.Background(), "", 200); err == nil {
			t.Fatal("expected error for malformed ExecutedAt in docs[1:], got nil")
		}
		if engine.indexCalls != 0 {
			t.Fatalf("IndexRecapDocuments was called %d time(s) despite a malformed timestamp in the batch", engine.indexCalls)
		}
	})
}

// TestIndexRecapsUsecase_ExecuteIncremental_MixedPrecisionTimestampsConverge
// pins the cursor's max selection to chronological order. RFC3339Nano allows
// the same instant family to serialize with different fractional-digit
// counts, and lexicographic order disagrees with chronological order across
// them: "...:05Z" > "...:05.000100Z" as strings ('Z' > '.') but is 100µs
// EARLIER as a time. A cursor that picks the string-max would nudge from the
// older timestamp and leave the true newest doc inside the inclusive bound,
// re-indexing it on every poll forever.
func TestIndexRecapsUsecase_ExecuteIncremental_MixedPrecisionTimestampsConverge(t *testing.T) {
	docs := []domain.RecapDocument{
		{ID: "job-frac__tech", JobID: "job-frac", ExecutedAt: "2026-08-07T17:00:05.000100Z", Genre: "tech"},
		{ID: "job-whole__tech", JobID: "job-whole", ExecutedAt: "2026-08-07T17:00:05Z", Genre: "tech"},
	}
	repo := &mockInclusiveBoundaryRecapRepo{docs: docs}
	engine := &mockRecapSearchEngine{}
	u := NewIndexRecapsUsecase(repo, engine)

	result, err := u.ExecuteIncremental(context.Background(), "", 200)
	if err != nil {
		t.Fatalf("ExecuteIncremental (first call): %v", err)
	}
	if result.IndexedCount != 2 {
		t.Fatalf("first call IndexedCount = %d, want 2", result.IndexedCount)
	}

	result, err = u.ExecuteIncremental(context.Background(), result.LastSince, 200)
	if err != nil {
		t.Fatalf("ExecuteIncremental (second call): %v", err)
	}
	if result.IndexedCount != 0 {
		t.Fatalf("second call re-indexed %d doc(s): cursor picked the lexicographic max instead of the chronological max and left the newest doc inside the inclusive bound", result.IndexedCount)
	}
}

// TestIndexRecapsUsecase_ExecuteBackfill_PaginatesUntilComplete drives the
// backfill loop the way bootstrap does: keep calling with the returned
// cursor while HasMore is true. HasMore is load-bearing output — reporting
// false for a truncated page would end Phase 1 with part of the corpus
// unindexed until incremental happens to walk past it.
func TestIndexRecapsUsecase_ExecuteBackfill_PaginatesUntilComplete(t *testing.T) {
	docs := []domain.RecapDocument{
		{ID: "job-1__tech", JobID: "job-1", ExecutedAt: "2026-08-07T17:00:00Z", Genre: "tech"},
		{ID: "job-2__tech", JobID: "job-2", ExecutedAt: "2026-08-07T17:00:01Z", Genre: "tech"},
		{ID: "job-3__tech", JobID: "job-3", ExecutedAt: "2026-08-07T17:00:02Z", Genre: "tech"},
		{ID: "job-4__tech", JobID: "job-4", ExecutedAt: "2026-08-07T17:00:03Z", Genre: "tech"},
	}
	repo := &mockInclusiveBoundaryRecapRepo{docs: docs}
	engine := &mockRecapSearchEngine{}
	u := NewIndexRecapsUsecase(repo, engine)

	first, err := u.ExecuteBackfill(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("ExecuteBackfill (first page): %v", err)
	}
	if first.IndexedCount != 2 {
		t.Fatalf("first page IndexedCount = %d, want 2", first.IndexedCount)
	}
	if !first.HasMore {
		t.Fatal("first page HasMore = false, want true: the page was truncated at the limit, so backfill must continue")
	}

	since := first.LastSince
	hasMore := true
	for polls := 0; hasMore; polls++ {
		if polls > 10 {
			t.Fatalf("backfill did not complete within 10 pages (stuck at since=%q)", since)
		}
		result, err := u.ExecuteBackfill(context.Background(), since, 2)
		if err != nil {
			t.Fatalf("ExecuteBackfill (since=%q): %v", since, err)
		}
		hasMore = result.HasMore
		if result.LastSince != "" {
			since = result.LastSince
		}
	}

	seenIDs := make(map[string]bool, len(engine.indexedDocs))
	for _, d := range engine.indexedDocs {
		seenIDs[d.ID] = true
	}
	for _, d := range docs {
		if !seenIDs[d.ID] {
			t.Fatalf("doc %q was never indexed by the backfill loop", d.ID)
		}
	}
}
