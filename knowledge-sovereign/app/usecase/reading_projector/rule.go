// Package reading_projector folds reading and queue events from the
// append-only event log into reading_sources and reading_queue_items.
package reading_projector

const (
	// LegacyReadRuleVersion marks reading_sources rows projected from historical
	// HomeItemOpened events rather than reading.session_ended.v1.
	LegacyReadRuleVersion = 0

	// ReadRuleVersionV1 identifies the first explicit session-based read rule.
	ReadRuleVersionV1 = 1

	// ReadRuleV1MinReachedEndDwellMs is the minimum foreground dwell required
	// when the reader scrolled to the end of the article body.
	ReadRuleV1MinReachedEndDwellMs = int64(5000)

	// ReadRuleV1MinDwellMs is the floor dwell duration when body-length scaling
	// determines whether an article counted as read without reaching the end.
	ReadRuleV1MinDwellMs = int64(30000)

	// ReadRuleV1MsPerChar scales required foreground dwell with article body
	// length when body_length_chars > 0.
	ReadRuleV1MsPerChar = int64(30)
)

// EvaluateReadRuleV1 determines whether a reading session counts as read under
// read_rule_version = 1 using event payload fields only.
func EvaluateReadRuleV1(reachedEnd bool, activeDwellMs int64, bodyLengthChars int32) bool {
	if reachedEnd && activeDwellMs >= ReadRuleV1MinReachedEndDwellMs {
		return true
	}
	if bodyLengthChars > 0 {
		threshold := int64(bodyLengthChars) * ReadRuleV1MsPerChar
		if threshold < ReadRuleV1MinDwellMs {
			threshold = ReadRuleV1MinDwellMs
		}
		if activeDwellMs >= threshold {
			return true
		}
	}
	return false
}
