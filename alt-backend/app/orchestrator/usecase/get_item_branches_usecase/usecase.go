// Package get_item_branches_usecase reads the branches anchored on one item —
// the Wave 10 (D26) patch-exit surface: max 1-2 branches shown at the article
// read-end, anchored on the article the user just finished.
package get_item_branches_usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"alt/domain"
	"alt/orchestrator/port/item_branches_port"

	"github.com/google/uuid"
)

// defaultLimit and maxLimit enforce the D26 patch-exit discipline: a handful
// of branches, never an inbox. defaultLimit is the "少数精鋭" (1-2) default;
// maxLimit is a hard ceiling even a client explicitly asking for more cannot
// cross.
const (
	defaultLimit = 2
	maxLimit     = 5
)

// ErrInvalidRequest wraps client-side validation failures so the handler can
// map them to InvalidArgument (vs a port failure, which is Internal).
var ErrInvalidRequest = errors.New("invalid get-item-branches request")

// GetItemBranchesUsecase reads the branches anchored on one item.
type GetItemBranchesUsecase struct {
	port item_branches_port.GetItemBranchesPort
}

func NewGetItemBranchesUsecase(port item_branches_port.GetItemBranchesPort) *GetItemBranchesUsecase {
	return &GetItemBranchesUsecase{port: port}
}

// Execute returns the open branches anchored on itemKey, clamped to the D26
// patch-exit range (default 2, max 5).
func (u *GetItemBranchesUsecase) Execute(ctx context.Context, userID uuid.UUID, itemKey string, limit int) ([]domain.TrailBranch, error) {
	itemKey = strings.TrimSpace(itemKey)
	if itemKey == "" {
		return nil, fmt.Errorf("%w: item_key required", ErrInvalidRequest)
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	branches, err := u.port.GetTrailBranchesForAnchor(ctx, userID, itemKey, limit)
	if err != nil {
		return nil, fmt.Errorf("get item branches: %w", err)
	}
	return branches, nil
}
