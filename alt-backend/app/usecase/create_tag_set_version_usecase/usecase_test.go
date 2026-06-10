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
				TagsJSON: json.RawMessage(`["go"]`),
			},
			tagSetPort: &mockTagSetPort{},
			eventPort:  &mockEventPort{},
			wantErr:    true,
		},
		{
			name: "error - empty tags_json",
			tsv: domain.TagSetVersion{
				ArticleID: articleID,
			},
			tagSetPort: &mockTagSetPort{},
			eventPort:  &mockEventPort{},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewCreateTagSetVersionUsecase(tt.tagSetPort, tt.eventPort, nil)
			err := uc.Execute(context.Background(), tt.tsv)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, tt.tagSetPort.created, 1)
			assert.Len(t, tt.eventPort.appended, 1)
			assert.Equal(t, domain.EventTagSetVersionCreated, tt.eventPort.appended[0].EventType)
		})
	}
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

	uc := NewCreateTagSetVersionUsecase(tagSetPort, eventPort, nil)
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
