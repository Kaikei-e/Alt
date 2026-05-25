package domain

// Feature flag name constants for Knowledge Home.
//
// FlagRecallRail was retired by ADR-000913 §D-9 PR 13 once the recall
// rail merged into the canonical GetKnowledgeHome payload. The flag is
// intentionally absent rather than redefined so a future feature does
// not silently inherit the stale name.
const (
	FlagKnowledgeHomePage         = "enable_knowledge_home_page"
	FlagKnowledgeHomeTracking     = "enable_knowledge_home_tracking"
	FlagKnowledgeHomeProjectionV2 = "enable_knowledge_home_projection_v2"
	FlagLensV0                    = "enable_lens"
	FlagStreamUpdates             = "enable_stream_updates"
	FlagSupersedeUX               = "enable_supersede_ux"
)
