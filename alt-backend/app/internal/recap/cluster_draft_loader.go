package recap

import (
	"encoding/json"
	"fmt"
	"os"

	"alt/domain"
)

// ClusterDraftLoader loads genre reorganization drafts from a JSON snapshot.
type ClusterDraftLoader struct {
	path string
}

// NewClusterDraftLoader returns a loader for the given file path.
func NewClusterDraftLoader(path string) *ClusterDraftLoader {
	return &ClusterDraftLoader{path: path}
}

// LoadDraft fetches a draft by ID. If the file does not exist or the draft is missing, nil is returned.
func (l *ClusterDraftLoader) LoadDraft(draftID string) (*domain.ClusterDraft, error) {
	if draftID == "" {
		return nil, nil
	}

	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cluster draft file: %w", err)
	}

	var payload clusterDraftFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to decode cluster draft file: %w", err)
	}

	for i := range payload.Drafts {
		if payload.Drafts[i].ID == draftID {
			draftCopy := payload.Drafts[i]
			return &draftCopy, nil
		}
	}

	return nil, nil
}

type clusterDraftFile struct {
	Drafts []domain.ClusterDraft `json:"drafts"`
}
