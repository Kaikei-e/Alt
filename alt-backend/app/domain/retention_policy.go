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
