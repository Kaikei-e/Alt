package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"knowledge-sovereign/driver/sovereign_db"
)

// These tests pin the invariants that Phase 1 of the Knowledge Loop Completion
// plan demands. They don't drive new behaviour — the resolver and its SQL are
// already shaped correctly. They exist so that a future refactor cannot
// silently break:
//
//   - replay determinism (same input → byte-identical SurfaceScoreInputs)
//   - F-001 counter increments when a cross-user lookup leaks
//   - the driver SQL still binds user_id physically as $1
//
// See docs/plan/knowledge-loop-completion-01-v2-planner.md §3.

// TestInvariant_ReplayDeterminism asserts that resolving twice from the same
// event log produces a byte-level identical SurfaceScoreInputs JSON. Reproject
// safety hinges on this: a non-deterministic resolver makes every reproject
// produce a different read model from the same event stream.
func TestInvariant_ReplayDeterminism(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	fixture := []sovereign_db.KnowledgeEvent{
		ev(EventSummaryVersionCreated, now.Add(-3*time.Hour), uid, map[string]any{
			"article_id": "art-1",
			"tags":       []any{"energy", "earnings"},
		}),
		ev(EventSummarySuperseded, now.Add(-1*time.Hour), uid, map[string]any{
			"article_id": "art-1",
		}),
		ev(EventAugurConversationLinked, now.Add(-90*time.Minute), uid, map[string]any{
			"entry_key":       "entry:art-1",
			"conversation_id": "conv-1",
		}),
		ev(EventRecapTopicSnapshotted, now.Add(-2*time.Hour), uid, map[string]any{
			"recap_topic_snapshot_id": "22222222-2222-4222-8222-222222222222",
			"top_terms":               []any{"energy", "markets"},
		}),
		ev(EventHomeItemOpened, now.Add(-30*time.Minute), uid, map[string]any{
			"entry_key": "entry:art-1",
		}),
	}
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id":   "art-1",
		"entry_key":    "entry:art-1",
		"tags":         []any{"energy", "earnings"},
		"published_at": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339),
	})

	r := NewEventLogSurfaceScoreResolver(&fakeEventLogLookup{events: fixture})

	first := r.Resolve(context.Background(), &target)
	second := r.Resolve(context.Background(), &target)

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}

	if string(firstJSON) != string(secondJSON) {
		t.Errorf("replay determinism violated:\n  first  = %s\n  second = %s", firstJSON, secondJSON)
	}
}

// TestInvariant_ReplayDeterminism_SourceURLPin pins the v14 reproject-safety
// contract: when the resolver pins SourceURL from a prior ArticleCreated /
// ArticleUpdated / ArticleUrlBackfilled, replaying the same event log must
// produce a byte-identical SurfaceScoreInputs (including SourceURL). A
// non-deterministic SourceURL pin would make every full reproject of an
// article's chain drift, exactly the failure mode v14 is supposed to retire.
func TestInvariant_ReplayDeterminism_SourceURLPin(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	articleID := "art-replay"
	entryKey := "entry:" + articleID
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)

	fixture := []sovereign_db.KnowledgeEvent{
		ev(EventArticleCreated, now.Add(-3*time.Hour), uid, map[string]any{
			"article_id": articleID,
			"url":        "https://example.com/p",
		}),
		ev(EventSummaryVersionCreated, now.Add(-2*time.Hour), uid, map[string]any{
			"article_id": articleID,
			"entry_key":  entryKey,
		}),
		ev(EventArticleUrlBackfilled, now.Add(-90*time.Minute), uid, map[string]any{
			"article_id":  articleID,
			"url":         "https://example.com/p-canonical",
			"reason_code": "missing_at_emit",
		}),
	}
	target := ev(EventAugurConversationLinked, now, uid, map[string]any{
		"entry_key":       entryKey,
		"conversation_id": "conv-1",
	})

	r := NewEventLogSurfaceScoreResolver(&fakeEventLogLookup{events: fixture})

	first := r.Resolve(context.Background(), &target)
	second := r.Resolve(context.Background(), &target)

	if first.SourceURL == "" {
		t.Fatalf("SourceURL not pinned on first resolve; want %q", "https://example.com/p-canonical")
	}
	if first.SourceURL != "https://example.com/p-canonical" {
		t.Errorf("SourceURL: want latest-by-event-seq %q, got %q",
			"https://example.com/p-canonical", first.SourceURL)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Errorf("SourceURL pin replay drift:\n  first  = %s\n  second = %s", firstJSON, secondJSON)
	}
}

// TestInvariant_F001MetricIncrementsOnCrossUserLeak pins the F-001 telemetry
// contract: when the lookup returns an event for a different user, the
// resolver bumps cross_user_isolation_violation_total. The behavioural side
// (returning empty inputs) is already covered by
// TestEventLogResolver_F001CrossUserMismatchReturnsEmpty; here we pin the
// alerting signal that paging depends on.
func TestInvariant_F001MetricIncrementsOnCrossUserLeak(t *testing.T) {
	// Not parallel: reads a process-wide counter. Other tests in the package
	// don't touch this counter under a normal lookup path, but mixing
	// parallel reads with this assertion would still be racy.
	uid := uuid.New()
	otherUID := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	other := otherUID
	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{{
		EventType:  EventAugurConversationLinked,
		OccurredAt: now.Add(-1 * time.Hour),
		UserID:     &other,
		Payload:    json.RawMessage(`{"entry_key":"entry:art-1"}`),
	}}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"entry_key": "entry:art-1",
	})

	before := testutil.ToFloat64(crossUserIsolationViolationTotal)
	_ = r.Resolve(context.Background(), &target)
	after := testutil.ToFloat64(crossUserIsolationViolationTotal)

	if after-before != 1 {
		t.Errorf("cross_user_isolation_violation_total delta = %v; want exactly 1", after-before)
	}
}

// TestInvariant_DriverSQLBindsUserIDPhysically is a regression test against
// the SQL string in driver/sovereign_db/read_events.go. F-001 mitigation
// depends on `WHERE user_id = $1` being present and bound as the *first*
// parameter (matching the resolver's call convention). A future refactor that
// loosens this predicate (e.g. adding `OR user_id IS NULL` for system events)
// would silently re-open the cross-user evidence leak vector.
//
// We grep the source rather than execute the query because:
//   - the package under test is purely behavioural and has no DB harness
//   - a behavioural test against a fake repo would not catch a regression
//     that only changes the SQL text
func TestInvariant_DriverSQLBindsUserIDPhysically(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// usecase/knowledge_loop_projector → up to app, then driver/sovereign_db.
	appDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	target := filepath.Join(appDir, "driver", "sovereign_db", "read_events.go")

	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	src := string(body)

	const fnMarker = "func (r *Repository) ListKnowledgeEventsForUserInWindow"
	fnStart := strings.Index(src, fnMarker)
	if fnStart < 0 {
		t.Fatalf("ListKnowledgeEventsForUserInWindow not found in %s — has the function moved? Update this regression test.", target)
	}
	// Scan only the body of the function we care about. The end of the
	// function is the next `\nfunc ` after fnStart, or EOF.
	rest := src[fnStart:]
	end := strings.Index(rest[1:], "\nfunc ")
	if end > 0 {
		rest = rest[:end+1]
	}

	if !strings.Contains(rest, "WHERE user_id = $1") {
		t.Errorf("ListKnowledgeEventsForUserInWindow no longer binds user_id as $1 — F-001 regression. Source:\n%s", rest)
	}
	if strings.Contains(rest, "OR user_id IS NULL") {
		t.Errorf("ListKnowledgeEventsForUserInWindow admits user_id IS NULL rows — F-001 regression. Source:\n%s", rest)
	}
}
