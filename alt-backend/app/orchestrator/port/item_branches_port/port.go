// Package item_branches_port defines the patch-exit branch lookup (Wave 10,
// D26): the branches anchored on one item, shown at the article read-end.
package item_branches_port

import (
	"context"

	"alt/domain"

	"github.com/google/uuid"
)

// GetItemBranchesPort fetches the user's open branches anchored on one item
// from the knowledge authority.
type GetItemBranchesPort interface {
	GetTrailBranchesForAnchor(ctx context.Context, userID uuid.UUID, anchorItemKey string, limit int) ([]domain.TrailBranch, error)
}
