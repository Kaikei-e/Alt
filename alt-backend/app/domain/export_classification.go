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
