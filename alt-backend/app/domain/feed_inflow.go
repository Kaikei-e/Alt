package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FeedInflowWindowDays is the rolling window length used to compute per-feed
// inflow and read engagement statistics.
const FeedInflowWindowDays = 30

// ReadingSourceEntry is one article's first-read attribution from knowledge-sovereign.
// ReadRuleVersion == 0 represents legacy opens; ReadRuleVersion >= 1 represents counted reads.
type ReadingSourceEntry struct {
	ArticleID       uuid.UUID
	FirstReadAt     time.Time
	ReadRuleVersion int32
}

// FeedInflow is the raw item inflow count for one subscribed feed within a window.
type FeedInflow struct {
	FeedLinkID  uuid.UUID
	FeedURL     string
	DailyCap    *int
	InflowCount int64
}

// FeedInflowStat is the per-feed 30-day inflow and engagement summary returned to callers.
type FeedInflowStat struct {
	FeedLinkID        string
	FeedURL           string
	DailyCap          *int
	InflowCount       int64
	ReadCount         int64
	OpenedCountLegacy int64
	ReadRate          float64
}

// ExceedsDailyCap evaluates in the domain layer whether this feed item should
// be hidden by the calling user's per-feed daily cap.
// When DailyCap is set, DailyRank must also be present; a missing rank indicates
// an unwired query path and fails loudly rather than silently bypassing the cap.
func (f *FeedItem) ExceedsDailyCap() (bool, error) {
	if f == nil || f.DailyCap == nil {
		return false, nil
	}
	if *f.DailyCap <= 0 {
		return false, fmt.Errorf("invalid daily_cap %d: must be positive", *f.DailyCap)
	}
	if f.DailyRank == nil {
		return false, fmt.Errorf("missing daily_rank for capped feed item %s", f.FeedID)
	}
	return *f.DailyRank > *f.DailyCap, nil
}
