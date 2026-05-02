package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDedupeKeyArticleUrlBackfill_IsV2Namespace(t *testing.T) {
	require.Equal(t,
		"article-url-backfill-v2:%s",
		DedupeKeyArticleUrlBackfill,
		"DedupeKeyArticleUrlBackfill must be at v2 namespace; "+
			"v1 corrective events are already in the dedupe registry "+
			"so v1 emit re-fires are silent no-ops")
}
