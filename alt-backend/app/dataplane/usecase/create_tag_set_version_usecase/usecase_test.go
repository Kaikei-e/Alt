package create_tag_set_version_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTagSetPort struct {
	created []domain.TagSetVersion
	err     error
}

func (m *mockTagSetPort) CreateTagSetVersion(_ context.Context, tsv domain.TagSetVersion) error {
	if m.err != nil {
		return m.err
	}
	m.created = append(m.created, tsv)
	return nil
}

func (m *mockTagSetPort) GetLatestTagSetVersion(_ context.Context, articleID uuid.UUID) (*domain.TagSetVersion, error) {
	if m.err != nil {
		return nil, m.err
	}
	for i := len(m.created) - 1; i >= 0; i-- {
		if m.created[i].ArticleID == articleID && m.created[i].SupersededBy == nil {
			cp := m.created[i]
			return &cp, nil
		}
	}
	return nil, nil
}

type mockEventPort struct {
	appended []domain.KnowledgeEvent
	err      error
}

func (m *mockEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	m.appended = append(m.appended, event)
	return int64(len(m.appended)), nil
}

type mockMarkTagSupersededPort struct {
	prev *domain.TagSetVersion
	err  error
}

func (m *mockMarkTagSupersededPort) MarkTagSetVersionSuperseded(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*domain.TagSetVersion, error) {
	return m.prev, m.err
}

func TestCreateTagSetVersionUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	articleID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name       string
		tsv        domain.TagSetVersion
		tagSetPort *mockTagSetPort
		eventPort  *mockEventPort
		wantErr    bool
	}{
		{
			name: "success - creates version and emits event",
			tsv: domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  json.RawMessage(`["go","rust"]`),
			},
			tagSetPort: &mockTagSetPort{},
			eventPort:  &mockEventPort{},
		},
		{
			name: "error - missing article_id",
			tsv: domain.TagSetVersion{
				UserID:   userID,
				TagsJSON: json.RawMessage(`["go"]`),
			},
			tagSetPort: &mockTagSetPort{},
			eventPort:  &mockEventPort{},
			wantErr:    true,
		},
		{
			name: "error - missing user_id",
			tsv: domain.TagSetVersion{
				ArticleID: articleID,
				TagsJSON:  json.RawMessage(`["go"]`),
			},
			tagSetPort: &mockTagSetPort{},
			eventPort:  &mockEventPort{},
			wantErr:    true,
		},
		{
			name: "error - empty tags_json",
			tsv: domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
			},
			tagSetPort: &mockTagSetPort{},
			eventPort:  &mockEventPort{},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewCreateTagSetVersionUsecase(tt.tagSetPort, tt.eventPort, &mockMarkTagSupersededPort{})
			err := uc.Execute(context.Background(), tt.tsv)

			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, tt.tagSetPort.created)
				assert.Empty(t, tt.eventPort.appended)
				return
			}

			require.NoError(t, err)
			assert.Len(t, tt.tagSetPort.created, 1)
			assert.Len(t, tt.eventPort.appended, 1)
			assert.Equal(t, domain.EventTagSetVersionCreated, tt.eventPort.appended[0].EventType)
		})
	}
}

func TestCreateTagSetVersionUsecase_RejectsNilUserID(t *testing.T) {
	logger.InitLogger()

	tagSetPort := &mockTagSetPort{}
	eventPort := &mockEventPort{}
	uc := NewCreateTagSetVersionUsecase(tagSetPort, eventPort, &mockMarkTagSupersededPort{})

	err := uc.Execute(context.Background(), domain.TagSetVersion{
		ArticleID: uuid.New(),
		UserID:    uuid.Nil,
		Generator: "tag-generator",
		TagsJSON:  json.RawMessage(`[{"name":"go","confidence":0.9}]`),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
	assert.Empty(t, tagSetPort.created, "no tag_set_versions row may be written when user_id is uuid.Nil")
	assert.Empty(t, eventPort.appended, "no knowledge event may be appended when user_id is uuid.Nil")
}

// TestCreateTagSetVersionUsecase_EmitsTagsInPayload pins the producer payload
// shape for TagSetVersionCreated. The knowledge-sovereign evidence accumulator
// reads the article's tag names from this event via
// readPayloadStringSlice(raw, "tags", "article_tags") to derive Cluster
// relations, so the `tags` key must carry the version's tag names as a JSON
// array of strings. Reproject-safe: the names come from the version being
// created, not a latest-state lookup.
func TestCreateTagSetVersionUsecase_EmitsTagsInPayload(t *testing.T) {
	logger.InitLogger()

	tagSetPort := &mockTagSetPort{}
	eventPort := &mockEventPort{}

	uc := NewCreateTagSetVersionUsecase(tagSetPort, eventPort, &mockMarkTagSupersededPort{})
	err := uc.Execute(context.Background(), domain.TagSetVersion{
		ArticleID: uuid.New(),
		UserID:    uuid.New(),
		Generator: "tag-generator",
		TagsJSON:  json.RawMessage(`[{"name":"go","confidence":0.9},{"name":"rust","confidence":0.8}]`),
	})

	require.NoError(t, err)
	require.Len(t, eventPort.appended, 1)
	require.Equal(t, domain.EventTagSetVersionCreated, eventPort.appended[0].EventType)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(eventPort.appended[0].Payload, &payload))

	rawTags, ok := payload["tags"]
	require.True(t, ok, "TagSetVersionCreated payload must carry a `tags` key")

	tagsAny, ok := rawTags.([]any)
	require.True(t, ok, "`tags` must be a JSON array")

	var tags []string
	for _, v := range tagsAny {
		s, ok := v.(string)
		require.True(t, ok, "`tags` entries must be strings")
		tags = append(tags, s)
	}
	assert.Equal(t, []string{"go", "rust"}, tags)
}

func TestCreateTagSetVersionUsecase_FirstVersion_NoSupersedeEvent(t *testing.T) {
	logger.InitLogger()

	tagSetPort := &mockTagSetPort{}
	eventPort := &mockEventPort{}
	markPort := &mockMarkTagSupersededPort{prev: nil}

	uc := NewCreateTagSetVersionUsecase(tagSetPort, eventPort, markPort)
	err := uc.Execute(context.Background(), domain.TagSetVersion{
		ArticleID: uuid.New(),
		UserID:    uuid.New(),
		TagsJSON:  json.RawMessage(`[{"name":"go","confidence":0.9}]`),
	})

	require.NoError(t, err)
	assert.Len(t, eventPort.appended, 1, "first version: only TagSetVersionCreated")
	assert.Equal(t, domain.EventTagSetVersionCreated, eventPort.appended[0].EventType)
}

func TestCreateTagSetVersionUsecase_SecondVersion_EmitsSupersedeEvent(t *testing.T) {
	logger.InitLogger()

	articleID := uuid.New()
	userID := uuid.New()
	oldVersionID := uuid.New()

	tagSetPort := &mockTagSetPort{}
	eventPort := &mockEventPort{}
	markPort := &mockMarkTagSupersededPort{
		prev: &domain.TagSetVersion{
			TagSetVersionID: oldVersionID,
			ArticleID:       articleID,
			UserID:          userID,
			TagsJSON:        json.RawMessage(`[{"name":"old-tag","confidence":0.8}]`),
		},
	}

	uc := NewCreateTagSetVersionUsecase(tagSetPort, eventPort, markPort)
	err := uc.Execute(context.Background(), domain.TagSetVersion{
		ArticleID: articleID,
		UserID:    userID,
		TagsJSON:  json.RawMessage(`[{"name":"new-tag","confidence":0.9}]`),
	})

	require.NoError(t, err)
	require.Len(t, eventPort.appended, 2, "second version: TagSetVersionCreated + TagSetSuperseded")
	assert.Equal(t, domain.EventTagSetVersionCreated, eventPort.appended[0].EventType)
	assert.Equal(t, domain.EventTagSetSuperseded, eventPort.appended[1].EventType)
	assert.Contains(t, string(eventPort.appended[1].Payload), "previous_tags")
}

type failingOnEventTypePort struct {
	failType string
	appended []domain.KnowledgeEvent
}

func (f *failingOnEventTypePort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	if event.EventType == f.failType {
		return 0, assert.AnError
	}
	f.appended = append(f.appended, event)
	return int64(len(f.appended)), nil
}

func TestCreateTagSetVersionUsecase_SupersedeEventAppendFailure_IsFatal(t *testing.T) {
	logger.InitLogger()

	articleID := uuid.New()
	userID := uuid.New()
	eventPort := &failingOnEventTypePort{failType: domain.EventTagSetSuperseded}
	markPort := &mockMarkTagSupersededPort{
		prev: &domain.TagSetVersion{
			TagSetVersionID: uuid.New(),
			ArticleID:       articleID,
			UserID:          userID,
			TagsJSON:        json.RawMessage(`[{"name":"old-tag","confidence":0.8}]`),
		},
	}

	uc := NewCreateTagSetVersionUsecase(&mockTagSetPort{}, eventPort, markPort)
	err := uc.Execute(context.Background(), domain.TagSetVersion{
		ArticleID: articleID,
		UserID:    userID,
		TagsJSON:  json.RawMessage(`[{"name":"new-tag","confidence":0.9}]`),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "append tag set superseded event")
}

func TestNewCreateTagSetVersionUsecase_PanicsOnNilPort(t *testing.T) {
	assert.Panics(t, func() {
		NewCreateTagSetVersionUsecase(nil, &mockEventPort{}, &mockMarkTagSupersededPort{})
	})
	assert.Panics(t, func() {
		NewCreateTagSetVersionUsecase(&mockTagSetPort{}, nil, &mockMarkTagSupersededPort{})
	})
	assert.Panics(t, func() {
		NewCreateTagSetVersionUsecase(&mockTagSetPort{}, &mockEventPort{}, nil)
	})
}

// statefulTagSetStore stands in for alt-db's tag_set_versions table across
// process restarts (fresh CreateTagSetVersionUsecase instances):
//   - CreateTagSetVersion implements ON CONFLICT (tag_set_version_id) DO NOTHING
//   - GetLatestTagSetVersion returns the newest row with superseded_by IS NULL
//     ordered by (generated_at DESC, tag_set_version_id DESC)
//   - MarkTagSetVersionSuperseded mirrors the SQL transaction in
//     TagRepository.MarkTagSetVersionSuperseded
type statefulTagSetStore struct {
	rows                   []domain.TagSetVersion
	failCreateOnce         bool
	failMarkSupersededOnce bool
}

func newerTagSetRow(a, b domain.TagSetVersion) bool {
	if !a.GeneratedAt.Equal(b.GeneratedAt) {
		return a.GeneratedAt.After(b.GeneratedAt)
	}
	return bytes.Compare(a.TagSetVersionID[:], b.TagSetVersionID[:]) > 0
}

func cloneTagSetVersion(tsv domain.TagSetVersion) domain.TagSetVersion {
	cp := tsv
	if tsv.TagsJSON != nil {
		cp.TagsJSON = append(json.RawMessage(nil), tsv.TagsJSON...)
	}
	if tsv.SupersededBy != nil {
		id := *tsv.SupersededBy
		cp.SupersededBy = &id
	}
	return cp
}

func (s *statefulTagSetStore) GetLatestTagSetVersion(_ context.Context, articleID uuid.UUID) (*domain.TagSetVersion, error) {
	var best *domain.TagSetVersion
	for i := range s.rows {
		r := s.rows[i]
		if r.ArticleID != articleID || r.SupersededBy != nil {
			continue
		}
		if best == nil || newerTagSetRow(r, *best) {
			cp := cloneTagSetVersion(r)
			best = &cp
		}
	}
	return best, nil
}

func (s *statefulTagSetStore) CreateTagSetVersion(_ context.Context, tsv domain.TagSetVersion) error {
	if s.failCreateOnce {
		s.failCreateOnce = false
		return assert.AnError
	}
	for _, r := range s.rows {
		if r.TagSetVersionID == tsv.TagSetVersionID {
			return nil // ON CONFLICT (tag_set_version_id) DO NOTHING
		}
	}
	s.rows = append(s.rows, cloneTagSetVersion(tsv))
	return nil
}

func (s *statefulTagSetStore) MarkTagSetVersionSuperseded(_ context.Context, articleID, newVersionID uuid.UUID) (*domain.TagSetVersion, error) {
	if s.failMarkSupersededOnce {
		s.failMarkSupersededOnce = false
		return nil, assert.AnError
	}

	newIdx := -1
	for i := range s.rows {
		if s.rows[i].TagSetVersionID == newVersionID {
			newIdx = i
			break
		}
	}
	if newIdx < 0 {
		return nil, fmt.Errorf("tag set version %s does not exist", newVersionID)
	}
	newRow := s.rows[newIdx]

	var winner *domain.TagSetVersion
	for i := range s.rows {
		r := s.rows[i]
		if r.ArticleID == articleID && r.SupersededBy == nil && newerTagSetRow(r, newRow) {
			if winner == nil || newerTagSetRow(r, *winner) {
				cp := cloneTagSetVersion(r)
				winner = &cp
			}
		}
	}
	if winner != nil {
		if s.rows[newIdx].SupersededBy == nil {
			wID := winner.TagSetVersionID
			s.rows[newIdx].SupersededBy = &wID
		}
		return nil, nil
	}

	var prev *domain.TagSetVersion
	for i := range s.rows {
		r := s.rows[i]
		if r.ArticleID != articleID {
			continue
		}
		if r.SupersededBy != nil && *r.SupersededBy != newVersionID {
			continue
		}
		if !newerTagSetRow(newRow, r) {
			continue
		}
		if prev == nil || newerTagSetRow(r, *prev) {
			cp := cloneTagSetVersion(r)
			prev = &cp
		}
	}
	if prev == nil {
		return nil, nil
	}

	for i := range s.rows {
		r := s.rows[i]
		if r.ArticleID != articleID {
			continue
		}
		if r.SupersededBy != nil && *r.SupersededBy != newVersionID {
			continue
		}
		if newerTagSetRow(newRow, r) {
			id := newVersionID
			s.rows[i].SupersededBy = &id
		}
	}
	return prev, nil
}

func (s *statefulTagSetStore) hasVersion(versionID uuid.UUID) bool {
	for _, r := range s.rows {
		if r.TagSetVersionID == versionID {
			return true
		}
	}
	return false
}

func (s *statefulTagSetStore) currentForArticle(articleID uuid.UUID) []domain.TagSetVersion {
	var out []domain.TagSetVersion
	for _, r := range s.rows {
		if r.ArticleID == articleID && r.SupersededBy == nil {
			out = append(out, cloneTagSetVersion(r))
		}
	}
	return out
}

type dedupEventLog struct {
	failEventTypeOnce string
	appended          []domain.KnowledgeEvent
	byDedupeKey       map[string]int64
}

func (l *dedupEventLog) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	if l.failEventTypeOnce != "" && event.EventType == l.failEventTypeOnce {
		l.failEventTypeOnce = ""
		return 0, assert.AnError
	}
	if l.byDedupeKey == nil {
		l.byDedupeKey = make(map[string]int64)
	}
	if event.DedupeKey != "" {
		if seq, exists := l.byDedupeKey[event.DedupeKey]; exists {
			return seq, nil
		}
	}
	l.appended = append(l.appended, event)
	seq := int64(len(l.appended))
	if event.DedupeKey != "" {
		l.byDedupeKey[event.DedupeKey] = seq
	}
	return seq, nil
}

func assertNoOrphanEvents(t *testing.T, store *statefulTagSetStore, events []domain.KnowledgeEvent) {
	t.Helper()
	for _, ev := range events {
		var payload map[string]any
		require.NoError(t, json.Unmarshal(ev.Payload, &payload))
		switch ev.EventType {
		case domain.EventTagSetVersionCreated:
			rawID, ok := payload["tag_set_version_id"].(string)
			require.True(t, ok, "TagSetVersionCreated must have tag_set_version_id")
			verID, err := uuid.Parse(rawID)
			require.NoError(t, err)
			assert.True(t, store.hasVersion(verID), "orphan TagSetVersionCreated: version %s missing from store", verID)
		case domain.EventTagSetSuperseded:
			newIDStr, ok := payload["new_tag_set_version_id"].(string)
			require.True(t, ok, "TagSetSuperseded must have new_tag_set_version_id")
			newID, err := uuid.Parse(newIDStr)
			require.NoError(t, err)
			assert.True(t, store.hasVersion(newID), "orphan TagSetSuperseded: new version %s missing from store", newID)

			oldIDStr, ok := payload["old_tag_set_version_id"].(string)
			require.True(t, ok, "TagSetSuperseded must have old_tag_set_version_id")
			oldID, err := uuid.Parse(oldIDStr)
			require.NoError(t, err)
			assert.True(t, store.hasVersion(oldID), "orphan TagSetSuperseded: old version %s missing from store", oldID)
		}
	}
}

func TestCreateTagSetVersionUsecase_RetryAndReplayAcrossRestarts(t *testing.T) {
	logger.InitLogger()

	newUC := func(store *statefulTagSetStore, eventLog *dedupEventLog) *CreateTagSetVersionUsecase {
		return NewCreateTagSetVersionUsecase(store, eventLog, store)
	}

	tagsA := json.RawMessage(`[{"name":"go","confidence":0.9}]`)
	tagsB := json.RawMessage(`[{"name":"rust","confidence":0.8}]`)
	tagsC := json.RawMessage(`[{"name":"python","confidence":0.7}]`)

	t.Run("retry after restart for each failure point converges to one new row, one created event, and one superseded event with the right predecessor", func(t *testing.T) {
		failurePoints := []struct {
			name   string
			inject func(store *statefulTagSetStore, eventLog *dedupEventLog)
		}{
			{
				name: "failure at row insert (CreateTagSetVersion)",
				inject: func(store *statefulTagSetStore, _ *dedupEventLog) {
					store.failCreateOnce = true
				},
			},
			{
				name: "failure after row insert at TagSetVersionCreated append",
				inject: func(_ *statefulTagSetStore, eventLog *dedupEventLog) {
					eventLog.failEventTypeOnce = domain.EventTagSetVersionCreated
				},
			},
			{
				name: "failure after TagSetVersionCreated append at MarkTagSetVersionSuperseded",
				inject: func(store *statefulTagSetStore, _ *dedupEventLog) {
					store.failMarkSupersededOnce = true
				},
			},
			{
				name: "failure after MarkTagSetVersionSuperseded at TagSetSuperseded append",
				inject: func(_ *statefulTagSetStore, eventLog *dedupEventLog) {
					eventLog.failEventTypeOnce = domain.EventTagSetSuperseded
				},
			},
		}

		for _, fp := range failurePoints {
			t.Run(fp.name, func(t *testing.T) {
				articleID := uuid.New()
				userID := uuid.New()
				store := &statefulTagSetStore{}
				eventLog := &dedupEventLog{}

				// Predecessor version A committed on fresh usecase instance 1.
				require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
					ArticleID: articleID,
					UserID:    userID,
					Generator: "tag-generator",
					TagsJSON:  tagsA,
				}))
				require.Len(t, store.rows, 1)
				vAID := store.rows[0].TagSetVersionID
				require.Len(t, eventLog.appended, 1)

				inputB := domain.TagSetVersion{
					ArticleID: articleID,
					UserID:    userID,
					Generator: "tag-generator",
					TagsJSON:  tagsB,
				}

				// Attempt 1 of version B on fresh usecase instance 2 fails at injected point.
				fp.inject(store, eventLog)
				err := newUC(store, eventLog).Execute(context.Background(), inputB)
				require.Error(t, err, "attempt 1 must fail at %s", fp.name)
				assertNoOrphanEvents(t, store, eventLog.appended)

				// Retry of version B after process restart (fresh usecase instance 3) must converge.
				err = newUC(store, eventLog).Execute(context.Background(), inputB)
				require.NoError(t, err, "retry on fresh usecase instance must succeed")

				// Replay of version B after another restart (fresh usecase instance 4) must remain converged.
				err = newUC(store, eventLog).Execute(context.Background(), inputB)
				require.NoError(t, err, "replay on fresh usecase instance must succeed")

				require.Len(t, store.rows, 2, "must have exactly 1 predecessor row A and 1 new row B")
				vA := store.rows[0]
				vB := store.rows[1]
				assert.Equal(t, vAID, vA.TagSetVersionID)
				assert.Equal(t, deriveTagSetVersionID(articleID, vA.TagSetVersionID, canonicalTagsHash(tagsB)), vB.TagSetVersionID,
					"version B ID must be derived from predecessor A read from store, not uuid.Nil")
				require.NotNil(t, vA.SupersededBy, "predecessor A must be marked superseded")
				assert.Equal(t, vB.TagSetVersionID, *vA.SupersededBy)
				assert.Nil(t, vB.SupersededBy, "new version B must be the current non-superseded version")

				current := store.currentForArticle(articleID)
				require.Len(t, current, 1)
				assert.Equal(t, vB.TagSetVersionID, current[0].TagSetVersionID)

				// Events: Created(A), Created(B), Superseded(A->B)
				require.Len(t, eventLog.appended, 3, "must have 1 Created for A, 1 Created for B, and 1 Superseded for B")
				assert.Equal(t, domain.EventTagSetVersionCreated, eventLog.appended[0].EventType)
				assert.Equal(t, domain.EventTagSetVersionCreated, eventLog.appended[1].EventType)
				assert.Equal(t, fmt.Sprintf("TagSetVersionCreated:%s", vB.TagSetVersionID), eventLog.appended[1].DedupeKey)
				assert.Equal(t, domain.EventTagSetSuperseded, eventLog.appended[2].EventType)
				assert.Equal(t, fmt.Sprintf("TagSetSuperseded:%s", vB.TagSetVersionID), eventLog.appended[2].DedupeKey)

				var supPayload map[string]any
				require.NoError(t, json.Unmarshal(eventLog.appended[2].Payload, &supPayload))
				assert.Equal(t, vB.TagSetVersionID.String(), supPayload["new_tag_set_version_id"])
				assert.Equal(t, vA.TagSetVersionID.String(), supPayload["old_tag_set_version_id"])
				assert.Equal(t, []any{"go"}, supPayload["previous_tags"])

				assertNoOrphanEvents(t, store, eventLog.appended)
			})
		}
	})

	t.Run("A->B then restart then A yields three distinct versions and current version is A", func(t *testing.T) {
		articleID := uuid.New()
		userID := uuid.New()
		store := &statefulTagSetStore{}
		eventLog := &dedupEventLog{}

		// Step 1: Write A on usecase instance 1.
		require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  tagsA,
		}))

		// Step 2: Write B on fresh usecase instance 2.
		require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  tagsB,
		}))

		// Step 3: Process restarts (fresh usecase instance 3); write A again.
		require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  tagsA,
		}))

		// Step 4: Replay A after another restart (fresh usecase instance 4).
		require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  tagsA,
		}))

		require.Len(t, store.rows, 3, "A->B->A across restarts must create three version rows")
		vA1 := store.rows[0]
		vB := store.rows[1]
		vA2 := store.rows[2]

		assert.NotEqual(t, vA1.TagSetVersionID, vB.TagSetVersionID)
		assert.NotEqual(t, vB.TagSetVersionID, vA2.TagSetVersionID)
		assert.NotEqual(t, vA1.TagSetVersionID, vA2.TagSetVersionID, "returning to A after B across restart must mint a new version ID")

		require.NotNil(t, vA1.SupersededBy)
		assert.Equal(t, vB.TagSetVersionID, *vA1.SupersededBy)
		require.NotNil(t, vB.SupersededBy, "B must be superseded by the second A")
		assert.Equal(t, vA2.TagSetVersionID, *vB.SupersededBy)
		assert.Nil(t, vA2.SupersededBy, "second A must be the current version")

		current := store.currentForArticle(articleID)
		require.Len(t, current, 1)
		assert.Equal(t, vA2.TagSetVersionID, current[0].TagSetVersionID)
		assert.JSONEq(t, string(tagsA), string(current[0].TagsJSON))

		require.Len(t, eventLog.appended, 5, "3 Created + 2 Superseded events")
		assertNoOrphanEvents(t, store, eventLog.appended)
	})

	t.Run("C arriving between a failed B attempt and B retry converges without orphan events or duplicate rows", func(t *testing.T) {
		t.Run("when B fails at row insert before C arrives", func(t *testing.T) {
			articleID := uuid.New()
			userID := uuid.New()
			store := &statefulTagSetStore{}
			eventLog := &dedupEventLog{}

			// Initial version A on fresh usecase.
			require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  tagsA,
			}))

			// B attempt 1 on fresh usecase fails at CreateTagSetVersion (row not written).
			store.failCreateOnce = true
			require.Error(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  tagsB,
			}))
			assertNoOrphanEvents(t, store, eventLog.appended)

			// Interleaved C arrives on a fresh usecase and succeeds.
			require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  tagsC,
			}))

			// B retries on a fresh usecase and succeeds.
			require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  tagsB,
			}))

			// Expected end state: 3 rows (vA -> vC -> vB current), 5 events (Created A, Created C, Superseded A->C, Created B, Superseded C->B), zero orphans.
			require.Len(t, store.rows, 3)
			vA := store.rows[0]
			vC := store.rows[1]
			vB := store.rows[2]
			assert.Equal(t, deriveTagSetVersionID(articleID, vA.TagSetVersionID, canonicalTagsHash(tagsC)), vC.TagSetVersionID)
			assert.Equal(t, deriveTagSetVersionID(articleID, vC.TagSetVersionID, canonicalTagsHash(tagsB)), vB.TagSetVersionID)
			require.NotNil(t, vA.SupersededBy)
			assert.Equal(t, vC.TagSetVersionID, *vA.SupersededBy)
			require.NotNil(t, vC.SupersededBy)
			assert.Equal(t, vB.TagSetVersionID, *vC.SupersededBy)
			assert.Nil(t, vB.SupersededBy)
			assert.JSONEq(t, string(tagsB), string(vB.TagsJSON))

			require.Len(t, eventLog.appended, 5)
			assertNoOrphanEvents(t, store, eventLog.appended)
		})

		t.Run("when B fails after row insert at created event append before C arrives", func(t *testing.T) {
			articleID := uuid.New()
			userID := uuid.New()
			store := &statefulTagSetStore{}
			eventLog := &dedupEventLog{}

			// Initial version A.
			require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  tagsA,
			}))

			// B attempt 1 inserts vB1 row then fails at TagSetVersionCreated append.
			eventLog.failEventTypeOnce = domain.EventTagSetVersionCreated
			require.Error(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  tagsB,
			}))

			// Interleaved C arrives and supersedes vB1 (and vA).
			require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  tagsC,
			}))

			// B retries on a fresh usecase, sees vC as current, and creates vB2 superseding vC.
			require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
				ArticleID: articleID,
				UserID:    userID,
				Generator: "tag-generator",
				TagsJSON:  tagsB,
			}))

			// Expected end state: 4 distinct rows (vA, vB1, vC all superseded; vB2 current with tags B), zero orphan events.
			require.Len(t, store.rows, 4)
			vA := store.rows[0]
			vB1 := store.rows[1]
			vC := store.rows[2]
			vB2 := store.rows[3]
			assert.NotEqual(t, vB1.TagSetVersionID, vB2.TagSetVersionID)
			require.NotNil(t, vA.SupersededBy)
			assert.Equal(t, vC.TagSetVersionID, *vA.SupersededBy)
			require.NotNil(t, vB1.SupersededBy)
			assert.Equal(t, vC.TagSetVersionID, *vB1.SupersededBy)
			require.NotNil(t, vC.SupersededBy)
			assert.Equal(t, vB2.TagSetVersionID, *vC.SupersededBy)
			assert.Nil(t, vB2.SupersededBy)

			current := store.currentForArticle(articleID)
			require.Len(t, current, 1)
			assert.Equal(t, vB2.TagSetVersionID, current[0].TagSetVersionID)
			assert.JSONEq(t, string(tagsB), string(current[0].TagsJSON))

			assertNoOrphanEvents(t, store, eventLog.appended)
		})
	})

	t.Run("canonical tags hash normalizes JSON object key order and whitespace across restarts", func(t *testing.T) {
		articleID := uuid.New()
		userID := uuid.New()
		store := &statefulTagSetStore{}
		eventLog := &dedupEventLog{}

		require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  json.RawMessage(`[{"name":"go","confidence":0.9}]`),
		}))

		// Replay after restart with reversed object keys and whitespace must be recognized as identical tags.
		require.NoError(t, newUC(store, eventLog).Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  json.RawMessage(`[ { "confidence" : 0.9 , "name" : "go" } ]`),
		}))

		require.Len(t, store.rows, 1, "canonical JSON replay with reordered object keys must not create a second version")
		require.Len(t, eventLog.appended, 1)
	})
}
