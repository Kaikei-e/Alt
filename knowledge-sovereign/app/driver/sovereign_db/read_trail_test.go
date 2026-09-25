package sovereign_db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetTrailFootprints_CollapsesRepeatedContacts pins the D24 read shape:
// the spine query groups raw footprints by (item_key, verb) so repeated
// contacts with one article collapse into a single row carrying the contact
// count and the first/latest contact times. The wear CTE keeps counting raw
// rows (a revisit still deepens the path).
func TestGetTrailFootprints_CollapsesRepeatedContacts(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, _, _, err := repo.GetTrailFootprints(context.Background(), uuid.New(), "", 20, nil)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1, "expected one spine query")

	sql := mock.queryCalls[0].SQL
	assert.Contains(t, sql, "count(*) AS contact_count",
		"repeated contacts must be counted, not repeated as rows")
	assert.Contains(t, sql, "min(occurred_at) AS first_occurred_at",
		"the earliest contact must survive the collapse")
	assert.Contains(t, sql, "max(occurred_at) AS occurred_at",
		"the collapsed row must sort by its latest contact")
	assert.Contains(t, sql, "GROUP BY tenant_id, item_key, verb",
		"the collapse key is (item_key, verb) within the user's spine")
	assert.Contains(t, sql, "GROUP BY item_key",
		"path wear must still fold over raw footprint rows")
}

func TestBuildTrailFootprintsFilter(t *testing.T) {
	fixedTime := time.Date(2026, time.September, 25, 7, 0, 0, 123456000, time.UTC)
	validCursor := encodeTrailCursor(fixedTime, "fp-test-key")

	tests := []struct {
		name           string
		cursor         string
		filterTags     []string
		baseArgPos     int
		wantWhere      string
		wantArgs       []any
		wantNextArgPos int
		wantErrSubstr  string
	}{
		{
			name:           "empty cursor and empty tags at pos 3",
			cursor:         "",
			filterTags:     nil,
			baseArgPos:     3,
			wantWhere:      "WHERE TRUE",
			wantArgs:       nil,
			wantNextArgPos: 3,
		},
		{
			name:           "with cursor and empty tags",
			cursor:         validCursor,
			filterTags:     nil,
			baseArgPos:     3,
			wantWhere:      "WHERE TRUE AND (f.occurred_at, f.footprint_key) < ($3, $4)",
			wantArgs:       []any{fixedTime, "fp-test-key"},
			wantNextArgPos: 5,
		},
		{
			name:       "empty cursor with tags",
			cursor:     "",
			filterTags: []string{"tag-a", "tag-b"},
			baseArgPos: 3,
			wantWhere: "WHERE TRUE AND EXISTS (\n" +
				"\t\t\tSELECT 1 FROM jsonb_array_elements_text(COALESCE(khi.tags_json, '[]')) AS tag_name\n" +
				"\t\t\tWHERE tag_name = ANY($3)\n" +
				"\t\t)",
			wantArgs:       []any{[]string{"tag-a", "tag-b"}},
			wantNextArgPos: 4,
		},
		{
			name:       "with cursor and tags at pos 1",
			cursor:     validCursor,
			filterTags: []string{"tag-1"},
			baseArgPos: 1,
			wantWhere: "WHERE TRUE AND (f.occurred_at, f.footprint_key) < ($1, $2) AND EXISTS (\n" +
				"\t\t\tSELECT 1 FROM jsonb_array_elements_text(COALESCE(khi.tags_json, '[]')) AS tag_name\n" +
				"\t\t\tWHERE tag_name = ANY($3)\n" +
				"\t\t)",
			wantArgs:       []any{fixedTime, "fp-test-key", []string{"tag-1"}},
			wantNextArgPos: 4,
		},
		{
			name:          "invalid cursor not base64",
			cursor:        "not-base64???",
			filterTags:    nil,
			baseArgPos:    3,
			wantErrSubstr: "invalid cursor",
		},
		{
			name:          "invalid cursor malformed format",
			cursor:        "bm9fcGlwZQ==",
			filterTags:    nil,
			baseArgPos:    3,
			wantErrSubstr: "malformed cursor",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotWhere, gotArgs, gotNextPos, err := buildTrailFootprintsFilter(tc.cursor, tc.filterTags, tc.baseArgPos)
			if tc.wantErrSubstr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrSubstr)
				assert.Empty(t, gotWhere)
				assert.Nil(t, gotArgs)
				assert.Equal(t, tc.baseArgPos, gotNextPos)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantWhere, gotWhere)
			assert.Equal(t, tc.wantArgs, gotArgs)
			assert.Equal(t, tc.wantNextArgPos, gotNextPos)
		})
	}
}

func TestEncodeDecodeTrailCursor(t *testing.T) {
	ts := time.Date(2026, time.September, 25, 14, 30, 0, 500000000, time.UTC)
	tests := []struct {
		name         string
		occurredAt   time.Time
		footprintKey string
	}{
		{
			name:         "standard utc",
			occurredAt:   ts,
			footprintKey: "k-1",
		},
		{
			name:         "empty footprint key",
			occurredAt:   ts,
			footprintKey: "",
		},
		{
			name:         "special characters in key",
			occurredAt:   ts,
			footprintKey: "key:with|pipe&and=symbols",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc := encodeTrailCursor(tc.occurredAt, tc.footprintKey)
			require.NotEmpty(t, enc)

			gotTime, gotKey, err := decodeTrailCursor(enc)
			require.NoError(t, err)
			assert.True(t, tc.occurredAt.Equal(gotTime))
			assert.Equal(t, tc.footprintKey, gotKey)
		})
	}
}

func TestDecodeTrailCursor_Errors(t *testing.T) {
	tests := []struct {
		name       string
		cursor     string
		errMessage string
	}{
		{
			name:       "invalid base64",
			cursor:     "!!!not-base64",
			errMessage: "decode cursor:",
		},
		{
			name:       "missing pipe separator",
			cursor:     "YWJj",
			errMessage: "malformed cursor",
		},
		{
			name:       "invalid time format",
			cursor:     "bm90LWEtdGltZXxrZXkx",
			errMessage: "parse cursor time:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := decodeTrailCursor(tc.cursor)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMessage)
		})
	}
}

func TestScanTrailFootprintRow(t *testing.T) {
	t.Run("successful scan", func(t *testing.T) {
		userID := uuid.New()
		tenantID := uuid.New()
		ts := time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC)
		mock := &mockRow{
			scanFunc: func(dest ...interface{}) error {
				*dest[0].(*uuid.UUID) = tenantID
				*dest[1].(*string) = "fp-1"
				*dest[2].(*string) = "read"
				*dest[3].(*string) = "art-1"
				*dest[4].(*string) = "test note"
				*dest[5].(*string) = "test.event.v1"
				*dest[6].(*time.Time) = ts
				*dest[7].(*time.Time) = ts.Add(-time.Hour)
				*dest[8].(*int) = 3
				*dest[9].(*string) = "Article Title"
				*dest[10].(*string) = "Excerpt"
				*dest[11].(*[]byte) = []byte(`["tag1","tag2"]`)
				*dest[12].(*string) = "worn"
				return nil
			},
		}

		fp, err := scanTrailFootprintRow(mock, userID)
		require.NoError(t, err)
		assert.Equal(t, userID, fp.UserID)
		assert.Equal(t, tenantID, fp.TenantID)
		assert.Equal(t, "fp-1", fp.FootprintKey)
		assert.Equal(t, "read", fp.Verb)
		assert.Equal(t, "art-1", fp.ItemKey)
		assert.Equal(t, "test note", fp.Note)
		assert.Equal(t, "test.event.v1", fp.SourceEventType)
		assert.Equal(t, ts, fp.OccurredAt)
		assert.Equal(t, ts.Add(-time.Hour), fp.FirstOccurredAt)
		assert.Equal(t, 3, fp.ContactCount)
		assert.Equal(t, "Article Title", fp.Title)
		assert.Equal(t, "Excerpt", fp.Excerpt)
		assert.Equal(t, []string{"tag1", "tag2"}, fp.Tags)
		assert.Equal(t, "worn", fp.Wear)
	})

	t.Run("scan error propagation", func(t *testing.T) {
		expectedErr := errors.New("scan failure")
		mock := &mockRow{
			scanFunc: func(dest ...interface{}) error {
				return expectedErr
			},
		}

		_, err := scanTrailFootprintRow(mock, uuid.New())
		require.ErrorIs(t, err, expectedErr)
	})
}
