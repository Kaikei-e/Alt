package fetch_feed_usecase

import "errors"

// validateCursorLimit validates that pagination limit is within (0, 100].
func validateCursorLimit(limit int) error {
	if limit <= 0 {
		return errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		return errors.New("limit cannot exceed 100")
	}
	return nil
}
