package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

// TestTrimStreamsUsecase_CoversDLQStreams reproduces the mq-hub half of the
// finding: the maintenance pass walked the four live streams only, so the DLQ
// the consumers XADD into had no ceiling from either side. XTRIM is the only
// control that still works once redis-streams is at maxmemory under
// noeviction (XADD is denyoom there), so a stream left out of this pass is a
// stream nothing can shrink back.
//
// pre-processor, search-indexer and tag-generator all consume
// alt:events:articles under their own group and all derive the same DLQ key,
// so the one entry below covers all three. It is spelled out as a literal on
// purpose: what has to match is the key those services actually write to, not
// whatever the constant happens to be named here.
func TestTrimStreamsUsecase_CoversDLQStreams(t *testing.T) {
	const articlesDLQ = domain.StreamKey("alt:events:articles:dlq")

	trimmer := &stubTrimmer{deleted: map[domain.StreamKey]int64{}}

	_, err := NewTrimStreamsUsecase(trimmer, 50000).Execute(context.Background())

	require.NoError(t, err)
	assert.Contains(t, trimmer.calls, articlesDLQ,
		"the DLQ is XADDed by three consumers and read by nobody; if this pass skips it, nothing bounds it")
}
