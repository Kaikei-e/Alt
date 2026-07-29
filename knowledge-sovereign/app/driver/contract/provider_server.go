//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1connect "knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
	"knowledge-sovereign/handler"
)

// The provider under verification is the real thing: the generated
// Connect-RPC handler wrapping handler.SovereignHandler, plus the real admin
// REST handlers. What is faked is the database, not the wire.
//
// This distinction is the whole point. A hand-written HTTP stub encodes what
// its author believed the contract to be, so a consumer can pin a field the
// proto has never had and the gate still reports green — which is how six
// interactions came to require a `success` field that
// AppendKnowledgeEventResponse does not define. Responses built by the
// generated protojson marshaler cannot carry a field that is not in the
// proto, so a proto change now invalidates the pact instead of passing
// silently past it.

// fakeRepo serves the fixtures the consumer pacts describe. It embeds
// handler.ReadDB so the compiler accepts it without ~50 stub methods; an RPC
// that reaches an un-overridden method nil-panics, which is the loud failure
// we want — it means a pact exercises a path this fixture never modelled.
type fakeRepo struct {
	handler.ReadDB

	// rejectMutation is flipped by the "mutation is rejected" provider state.
	rejectMutation bool
}

const (
	fixtureSnapshotID   = "11111111-2222-3333-4444-555555555555"
	fixtureItemsSum     = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixtureDigestSum    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	fixtureRecallSum    = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	fixtureBuildRef     = "staging"
	fixtureSchemaVer    = "00009"
	fixtureEventSeq     = int64(1)
	fixtureRetentionTbl = "knowledge_events"
	fixtureRetentionPar = "knowledge_events_y2025m01"
)

func fixtureTime() time.Time {
	return time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)
}

// --- MutationRepository: every mutation RPC routes through one of these ---

// errMutationRejected is what the "mutation is rejected" provider state
// produces. The handler turns a repository error into success=false plus an
// error_message, both of which are real fields of the response proto.
var errMutationRejected = errors.New("projection version mismatch")

func (f *fakeRepo) mutationResult() error {
	if f.rejectMutation {
		return errMutationRejected
	}
	return nil
}

func (f *fakeRepo) UpsertKnowledgeHomeItem(context.Context, json.RawMessage) error {
	return f.mutationResult()
}
func (f *fakeRepo) DismissKnowledgeHomeItem(context.Context, json.RawMessage) error {
	return f.mutationResult()
}
func (f *fakeRepo) ClearSupersedeState(context.Context, json.RawMessage) error {
	return f.mutationResult()
}
func (f *fakeRepo) UpsertTodayDigest(context.Context, json.RawMessage) error {
	return f.mutationResult()
}
func (f *fakeRepo) UpsertRecallCandidate(context.Context, json.RawMessage) error {
	return f.mutationResult()
}
func (f *fakeRepo) SnoozeRecallCandidate(context.Context, json.RawMessage) error {
	return f.mutationResult()
}
func (f *fakeRepo) DismissRecallCandidate(context.Context, json.RawMessage) error {
	return f.mutationResult()
}
func (f *fakeRepo) PatchKnowledgeHomeItemURL(context.Context, json.RawMessage) error {
	return f.mutationResult()
}

// --- Events ---

// AppendKnowledgeEvent returns a non-zero sequence: alt-backend's URL-backfill
// SkippedDuplicate counter (ADR-000869) reads seq > 0 as "genuinely appended"
// and seq == 0 as a dedupe-registry hit, so the pact pins the field.
func (f *fakeRepo) AppendKnowledgeEvent(context.Context, sovereign_db.KnowledgeEvent) (int64, error) {
	return 123, nil
}

// --- Trail read path ---

func (f *fakeRepo) GetTrailFootprints(_ context.Context, userID uuid.UUID, _ string, _ int, filterTags []string) ([]sovereign_db.TrailFootprint, string, bool, error) {
	return []sovereign_db.TrailFootprint{{
		UserID:          userID,
		FootprintKey:    "open:article:1",
		Verb:            "read",
		ItemKey:         "article:1",
		OccurredAt:      time.Date(2026, 6, 10, 9, 12, 0, 0, time.UTC),
		Wear:            "worn",
		ContactCount:    2,
		FirstOccurredAt: time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
	}}, "", false, nil
}

func (f *fakeRepo) GetOpenTrailBranches(context.Context, uuid.UUID) ([]sovereign_db.TrailBranch, error) {
	return []sovereign_db.TrailBranch{fixtureBranch()}, nil
}

func (f *fakeRepo) GetOpenTrailBranchesForAnchor(context.Context, uuid.UUID, string, int) ([]sovereign_db.TrailBranch, error) {
	b := fixtureBranch()
	b.Why = `Because you read "US military courts in the UK" — joins rust`
	b.TargetTitle = "Async Rust"
	return []sovereign_db.TrailBranch{b}, nil
}

func fixtureBranch() sovereign_db.TrailBranch {
	return sovereign_db.TrailBranch{
		BranchKey:     "cluster:u:article:z",
		AnchorItemKey: "article:1",
		RelationKind:  "cluster",
		Why:           "Joins a topic you follow.",
		Confidence:    "plausible",
		TargetItemKey: "article:z",
		EvidenceRefs: []sovereign_db.TrailEvidenceRef{
			{RefID: "rust", Label: "rust", Kind: "tag"},
		},
	}
}

// --- Admin REST repositories ---
//
// These are declared explicitly rather than embedded: SnapshotRepository,
// RetentionRepository and StorageRepository overlap on several method names,
// and a promoted method that is ambiguous does not satisfy an interface.

func (f *fakeRepo) InsertSnapshot(context.Context, *sovereign_db.SnapshotMetadata) error { return nil }
func (f *fakeRepo) UpdateSnapshotStatus(context.Context, uuid.UUID, string) error        { return nil }
func (f *fakeRepo) GetTableRowCount(context.Context, string) (int, error)                { return 0, nil }
func (f *fakeRepo) GetMaxEventSeq(context.Context) (int64, error)                        { return fixtureEventSeq, nil }
func (f *fakeRepo) ExportTableToWriter(context.Context, string, io.Writer) (int64, error) {
	return 0, nil
}

func (f *fakeRepo) GetActiveProjectionVersion(context.Context) (*sovereign_db.ProjectionVersion, error) {
	return &sovereign_db.ProjectionVersion{Version: 1, Status: "active"}, nil
}

func (f *fakeRepo) GetLatestValidSnapshot(context.Context) (*sovereign_db.SnapshotMetadata, error) {
	s := fixtureSnapshot()
	return &s, nil
}

func (f *fakeRepo) ListSnapshots(context.Context, int) ([]sovereign_db.SnapshotMetadata, error) {
	return []sovereign_db.SnapshotMetadata{fixtureSnapshot()}, nil
}

func fixtureSnapshot() sovereign_db.SnapshotMetadata {
	return sovereign_db.SnapshotMetadata{
		SnapshotID:        uuid.MustParse(fixtureSnapshotID),
		SnapshotType:      "full",
		ProjectionVersion: 1,
		ProjectorBuildRef: fixtureBuildRef,
		SchemaVersion:     fixtureSchemaVer,
		SnapshotAt:        fixtureTime(),
		EventSeqBoundary:  fixtureEventSeq,
		ItemsChecksum:     fixtureItemsSum,
		DigestChecksum:    fixtureDigestSum,
		RecallChecksum:    fixtureRecallSum,
		Status:            "valid",
	}
}

func (f *fakeRepo) ListPartitions(context.Context, string) ([]sovereign_db.PartitionInfo, error) {
	return []sovereign_db.PartitionInfo{{
		Name:       fixtureRetentionPar,
		RangeStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		RangeEnd:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		RowCount:   1,
		SizeBytes:  1,
	}}, nil
}

func (f *fakeRepo) InsertRetentionLog(context.Context, sovereign_db.RetentionLogEntry) error {
	return nil
}

func (f *fakeRepo) ListRetentionLogs(context.Context, int) ([]sovereign_db.RetentionLogEntry, error) {
	return []sovereign_db.RetentionLogEntry{{
		LogID:           uuid.MustParse(fixtureSnapshotID),
		RunAt:           fixtureTime(),
		Action:          "export",
		TargetTable:     fixtureRetentionTbl,
		TargetPartition: fixtureRetentionPar,
		RowsAffected:    1,
		DryRun:          false,
		Status:          "success",
	}}, nil
}

func (f *fakeRepo) GetStorageStats(context.Context) ([]sovereign_db.TableStorageInfo, error) {
	return []sovereign_db.TableStorageInfo{{
		TableName: fixtureRetentionTbl,
		TotalSize: "128 kB",
		TableSize: "96 kB",
		IndexSize: "32 kB",
		RowCount:  0,
	}}, nil
}

// startProviderServer mounts the production Connect-RPC and admin REST
// handlers on one listener. pact-go routes every interaction through a single
// ProviderBaseURL, so both surfaces share the port.
func startProviderServer(t *testing.T, repo *fakeRepo) int {
	t.Helper()

	mux := http.NewServeMux()

	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(handler.NewSovereignHandler(repo))
	mux.Handle(path, rpcHandler)

	handler.NewSnapshotHandler(repo, t.TempDir(), fixtureBuildRef, fixtureSchemaVer).RegisterRoutes(mux)
	handler.NewRetentionHandler(repo, t.TempDir()).RegisterRoutes(mux)
	handler.NewStorageHandler(repo).RegisterRoutes(mux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().(*net.TCPAddr).Port
}
