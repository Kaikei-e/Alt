package domain

// Export tier constants.
const (
	ExportTierA = "A" // Non-regenerable, must not lose.
	ExportTierB = "B" // Regenerable but high cost.
	ExportTierC = "C" // Easily regenerable.
)

// ExportClassification defines the export tier and reason for an entity type.
type ExportClassification struct {
	EntityType string `json:"entity_type"`
	Tier       string `json:"tier"`
	Reason     string `json:"reason"`
}

// DefaultExportClassification defines the export classification for each entity type.
var DefaultExportClassification = map[string]ExportClassification{
	"feed_subscriptions":     {EntityType: "feed_subscriptions", Tier: ExportTierA, Reason: "User settings, non-regenerable"},
	"user_curation_state":    {EntityType: "user_curation_state", Tier: ExportTierA, Reason: "User-specific judgments"},
	"summary_latest":         {EntityType: "summary_latest", Tier: ExportTierB, Reason: "Regenerable but high cost"},
	"tag_latest":             {EntityType: "tag_latest", Tier: ExportTierB, Reason: "Regenerable but high cost"},
	"recall_candidates":      {EntityType: "recall_candidates", Tier: ExportTierC, Reason: "Recomputable"},
	"projection_checkpoints": {EntityType: "projection_checkpoints", Tier: ExportTierC, Reason: "Rebuildable"},
}
