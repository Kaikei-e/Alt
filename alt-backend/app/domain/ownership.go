package domain

// Ownership domain constants for Knowledge Sovereign.
const (
	OwnershipKnowledgeEvents = "knowledge_events"
	OwnershipHomeProjection  = "home_projection"
	OwnershipRecallCandidate = "recall_candidate"
	OwnershipCurationState   = "curation_state"
	OwnershipRetention       = "retention"
	OwnershipExportPolicy    = "export_policy"
)

// OwnershipEntry describes the current and future owner of a domain area.
type OwnershipEntry struct {
	Domain       string `json:"domain"`
	CurrentOwner string `json:"current_owner"`
	FutureOwner  string `json:"future_owner"`
	Note         string `json:"note"`
}
