package usecase

import (
	"context"
	"fmt"
	"search-indexer/domain"
	"search-indexer/port"
	"time"
)

// IndexRecapsUsecase handles indexing recap genres into Meilisearch.
type IndexRecapsUsecase struct {
	recapRepo         port.RecapRepository
	recapSearchEngine port.RecapSearchEngine
}

// RecapIndexResult contains the result of a recap indexing operation.
type RecapIndexResult struct {
	IndexedCount int
	HasMore      bool
	LastSince    string // RFC3339 timestamp for next incremental fetch
}

// NewIndexRecapsUsecase creates a new index recaps usecase.
func NewIndexRecapsUsecase(recapRepo port.RecapRepository, recapSearchEngine port.RecapSearchEngine) *IndexRecapsUsecase {
	return &IndexRecapsUsecase{
		recapRepo:         recapRepo,
		recapSearchEngine: recapSearchEngine,
	}
}

// ExecuteBackfill fetches recap genres and indexes them, paginating via since cursor.
func (u *IndexRecapsUsecase) ExecuteBackfill(ctx context.Context, since string, batchSize int) (*RecapIndexResult, error) {
	docs, hasMore, err := u.recapRepo.GetIndexableGenres(ctx, since, batchSize)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return &RecapIndexResult{IndexedCount: 0, HasMore: false}, nil
	}

	// Compute the cursor (which validates every doc's ExecutedAt) before
	// writing anything, so a malformed timestamp fails closed with no side
	// effect instead of writing the batch and then erroring -- a retry after
	// a write-then-fail would keep re-pushing the same batch to Meilisearch
	// on every backoff attempt for no benefit, since `since` never advances.
	lastSince, err := nextRecapCursor(docs, hasMore)
	if err != nil {
		return nil, err
	}

	if err := u.recapSearchEngine.IndexRecapDocuments(ctx, docs); err != nil {
		return nil, err
	}

	return &RecapIndexResult{
		IndexedCount: len(docs),
		HasMore:      hasMore,
		LastSince:    lastSince,
	}, nil
}

// ExecuteIncremental fetches new recap genres since the given timestamp.
func (u *IndexRecapsUsecase) ExecuteIncremental(ctx context.Context, since string, batchSize int) (*RecapIndexResult, error) {
	docs, hasMore, err := u.recapRepo.GetIndexableGenres(ctx, since, batchSize)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return &RecapIndexResult{IndexedCount: 0, HasMore: false, LastSince: since}, nil
	}

	lastSince, err := nextRecapCursor(docs, hasMore)
	if err != nil {
		return nil, err
	}

	if err := u.recapSearchEngine.IndexRecapDocuments(ctx, docs); err != nil {
		return nil, err
	}

	return &RecapIndexResult{
		IndexedCount: len(docs),
		HasMore:      hasMore,
		LastSince:    lastSince,
	}, nil
}

// nextRecapCursor computes the next fetch boundary for a batch.
// recap-worker's /v1/recaps/genres/indexable endpoint treats "since" as an
// inclusive lower bound (SQL: "kicked_at >= $1") and paginates with
// "ORDER BY kicked_at ASC LIMIT n" -- no secondary tiebreaker on job_id or
// genre. Two consequences follow from that:
//
//  1. Echoing a batch's own max ExecutedAt back as the next cursor makes
//     the server return the exact same batch again forever, since an
//     inclusive bound never ages out against itself. A batch the server
//     did NOT truncate (hasMore == false) is known-complete, so it is safe
//     to nudge the cursor one microsecond past its max -- the finest
//     precision a Postgres timestamptz preserves -- turning the bound
//     effectively exclusive.
//  2. A batch the server DID truncate at the page limit (hasMore == true)
//     may have split a tie group arbitrarily: rows with the exact same
//     kicked_at commonly occur because every genre a single recap job
//     produces shares that job's kicked_at, and an untied ORDER BY gives no
//     guarantee about which subset of a tied group lands on which page.
//     Nudging past the boundary in that case would permanently drop
//     whichever fraction of the tie group missed this page. So when
//     hasMore is true, the cursor stays AT the max value (still inclusive):
//     the next poll re-fetches that boundary group in full (idempotent
//     re-indexing of anything already written) instead of silently losing
//     part of it. Every doc strictly before the max is unaffected by this,
//     since ORDER BY ASC LIMIT n guarantees nothing smaller was skipped.
//
// Comparison is done on parsed times, not raw strings: RFC3339Nano strings
// with different fractional-digit counts (e.g. a whole-second "...:05Z" vs
// "...:05.000001Z") do not sort the same lexicographically as they do
// chronologically.
//
// docs must be non-empty; callers already early-return before reaching
// this point when the batch is empty.
func nextRecapCursor(docs []domain.RecapDocument, hasMore bool) (string, error) {
	maxExecutedAt := docs[0].ExecutedAt
	maxTS, err := time.Parse(time.RFC3339Nano, maxExecutedAt)
	if err != nil {
		return "", fmt.Errorf("recap cursor: parse executed_at %q: %w", maxExecutedAt, err)
	}
	for _, d := range docs[1:] {
		ts, err := time.Parse(time.RFC3339Nano, d.ExecutedAt)
		if err != nil {
			return "", fmt.Errorf("recap cursor: parse executed_at %q: %w", d.ExecutedAt, err)
		}
		if ts.After(maxTS) {
			maxTS = ts
			maxExecutedAt = d.ExecutedAt
		}
	}

	if hasMore {
		return maxExecutedAt, nil
	}
	return maxTS.Add(time.Microsecond).Format(time.RFC3339Nano), nil
}
