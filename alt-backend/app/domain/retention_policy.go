package domain

// Retention tier constants.
const (
	RetentionTierHot     = "hot"
	RetentionTierWarm    = "warm"
	RetentionTierCold    = "cold"
	RetentionTierArchive = "archive"
)

// RetentionPolicy defines the retention tier and export priority for an entity type.
type RetentionPolicy struct {
	EntityType     string `json:"entity_type"`
	Tier           string `json:"tier"`
	ExportPriority string `json:"export_priority"`
}

// DefaultRetentionMatrix defines the default retention tier for each entity type.
// This is a policy matrix, NOT a database column (per ADR-000532 Section 6).
var DefaultRetentionMatrix = map[string]RetentionPolicy{
	"article_metadata":     {EntityType: "article_metadata", Tier: RetentionTierHot, ExportPriority: "high"},
	"article_raw_body":     {EntityType: "article_raw_body", Tier: RetentionTierWarm, ExportPriority: "medium"},
	"summary_latest":       {EntityType: "summary_latest", Tier: RetentionTierHot, ExportPriority: "high"},
	"summary_old_versions": {EntityType: "summary_old_versions", Tier: RetentionTierCold, ExportPriority: "low"},
	"tag_latest":           {EntityType: "tag_latest", Tier: RetentionTierHot, ExportPriority: "high"},
	"tag_old_versions":     {EntityType: "tag_old_versions", Tier: RetentionTierCold, ExportPriority: "low"},
	"recall_raw_signals":   {EntityType: "recall_raw_signals", Tier: RetentionTierWarm, ExportPriority: "low"},
	"recall_aggregates":    {EntityType: "recall_aggregates", Tier: RetentionTierHot, ExportPriority: "low"},
	"recap_result":         {EntityType: "recap_result", Tier: RetentionTierHot, ExportPriority: "medium"},
	"user_curation_state":  {EntityType: "user_curation_state", Tier: RetentionTierHot, ExportPriority: "high"},
	"export_manifest":      {EntityType: "export_manifest", Tier: RetentionTierHot, ExportPriority: "high"},
}
