package create_tag_set_version_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
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

type failOnceDedupEventPort struct {
	failEventType string
	failedOnce    bool
	calls         int
	appended      []domain.KnowledgeEvent
	byDedupeKey   map[string]int64
}

func (f *failOnceDedupEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	f.calls++
	if f.failEventType != "" && event.EventType == f.failEventType && !f.failedOnce {
		f.failedOnce = true
		return 0, assert.AnError
	}
	if f.byDedupeKey == nil {
		f.byDedupeKey = make(map[string]int64)
	}
	if event.DedupeKey != "" {
		if _, exists := f.byDedupeKey[event.DedupeKey]; exists {
			return 0, nil
		}
	}
	f.appended = append(f.appended, event)
	seq := int64(len(f.appended))
	if event.DedupeKey != "" {
		f.byDedupeKey[event.DedupeKey] = seq
	}
	return seq, nil
}

func TestCreateTagSetVersionUsecase_RetryAndReplayIdempotency(t *testing.T) {
	logger.InitLogger()

	t.Run("first version: event append fails once then succeeds produces one row and one event, and replay is a no-op", func(t *testing.T) {
		articleID := uuid.New()
		userID := uuid.New()
		tagSetPort := &mockTagSetPort{}
		eventPort := &failOnceDedupEventPort{failEventType: domain.EventTagSetVersionCreated}
		markPort := &mockMarkTagSupersededPort{}

		uc := NewCreateTagSetVersionUsecase(tagSetPort, eventPort, markPort)
		input := domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  json.RawMessage(`[{"name":"go","confidence":0.9}]`),
		}

		err := uc.Execute(context.Background(), input)
		require.Error(t, err, "first call must fail when event append fails")

		err = uc.Execute(context.Background(), input)
		require.NoError(t, err, "retry with identical input must succeed")
		require.Len(t, tagSetPort.created, 1, "two Execute calls after partial failure must produce exactly one version row")
		require.Len(t, eventPort.appended, 1, "two Execute calls after partial failure must produce exactly one TagSetVersionCreated")
		assert.Equal(t, domain.EventTagSetVersionCreated, eventPort.appended[0].EventType)

		callsBeforeReplay := eventPort.calls
		err = uc.Execute(context.Background(), input)
		require.NoError(t, err, "identical-tags replay after full success must succeed")
		assert.Len(t, tagSetPort.created, 1, "identical-tags replay must not insert another version row")
		assert.Len(t, eventPort.appended, 1, "identical-tags replay must not emit another event")
		assert.Equal(t, callsBeforeReplay, eventPort.calls, "identical-tags replay must be a no-op")
	})

	t.Run("second version: supersede event append fails once then succeeds produces one version row, one created event, and one superseded event", func(t *testing.T) {
		articleID := uuid.New()
		userID := uuid.New()
		oldVersionID := uuid.New()

		tagSetPort := &mockTagSetPort{}
		eventPort := &failOnceDedupEventPort{failEventType: domain.EventTagSetSuperseded}
		markPort := &mockMarkTagSupersededPort{
			prev: &domain.TagSetVersion{
				TagSetVersionID: oldVersionID,
				ArticleID:       articleID,
				UserID:          userID,
				TagsJSON:        json.RawMessage(`[{"name":"old-tag","confidence":0.8}]`),
			},
		}

		uc := NewCreateTagSetVersionUsecase(tagSetPort, eventPort, markPort)
		input := domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  json.RawMessage(`[{"name":"new-tag","confidence":0.9}]`),
		}

		err := uc.Execute(context.Background(), input)
		require.Error(t, err, "first call must fail when TagSetSuperseded append fails")

		err = uc.Execute(context.Background(), input)
		require.NoError(t, err, "retry must succeed and complete the supersede event")
		require.Len(t, tagSetPort.created, 1, "retry must produce exactly one new version row")
		require.Len(t, eventPort.appended, 2, "retry must produce exactly one TagSetVersionCreated and one TagSetSuperseded")
		assert.Equal(t, domain.EventTagSetVersionCreated, eventPort.appended[0].EventType)
		assert.Equal(t, domain.EventTagSetSuperseded, eventPort.appended[1].EventType)

		callsBeforeReplay := eventPort.calls
		err = uc.Execute(context.Background(), input)
		require.NoError(t, err, "identical-tags replay after full success must be a no-op")
		assert.Len(t, tagSetPort.created, 1)
		assert.Len(t, eventPort.appended, 2)
		assert.Equal(t, callsBeforeReplay, eventPort.calls)
	})

	t.Run("A->B->A transitions get distinct version IDs while immediate replay is a no-op", func(t *testing.T) {
		articleID := uuid.New()
		userID := uuid.New()
		tagSetPort := &mockTagSetPort{}
		eventPort := &failOnceDedupEventPort{}
		markPort := &mockMarkTagSupersededPort{}

		uc := NewCreateTagSetVersionUsecase(tagSetPort, eventPort, markPort)

		tagsA := json.RawMessage(`[{"name":"go","confidence":0.9}]`)
		tagsB := json.RawMessage(`[{"name":"rust","confidence":0.8}]`)

		require.NoError(t, uc.Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  tagsA,
		}))
		require.NoError(t, uc.Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  tagsB,
		}))
		require.NoError(t, uc.Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  tagsA,
		}))
		// Immediate replay of tagsA must be a no-op.
		require.NoError(t, uc.Execute(context.Background(), domain.TagSetVersion{
			ArticleID: articleID,
			UserID:    userID,
			Generator: "tag-generator",
			TagsJSON:  tagsA,
		}))

		require.Len(t, tagSetPort.created, 3, "A->B->A followed by replay of A must create exactly three versions")
		v1 := tagSetPort.created[0].TagSetVersionID
		v2 := tagSetPort.created[1].TagSetVersionID
		v3 := tagSetPort.created[2].TagSetVersionID
		assert.NotEqual(t, v1, v2)
		assert.NotEqual(t, v2, v3)
		assert.NotEqual(t, v1, v3, "returning to tags A after B must mint a distinct version ID")
	})
}
