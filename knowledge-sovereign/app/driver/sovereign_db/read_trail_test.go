package sovereign_db

import (
	"context"
	"testing"

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
