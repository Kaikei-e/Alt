package pki

import (
	"context"
	"errors"
)

// LogSafeError returns a stable, non-secret identifier for ops logs.
// It never echoes PasswordFile paths or password bytes.
func LogSafeError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrSharedRootSecret):
		return "shared_root_secret"
	case errors.Is(err, ErrSharedProvisioner):
		return "shared_provisioner"
	case errors.Is(err, ErrPasswordTooLarge):
		return "password_too_large"
	case errors.Is(err, ErrPasswordEmpty):
		return "password_empty"
	case errors.Is(err, ErrPasswordUnreadable):
		return "password_unreadable"
	case errors.Is(err, ErrPasswordFileName):
		return "password_file_name"
	case errors.Is(err, ErrNotHTTPS):
		return "insecure_ca_url"
	case errors.Is(err, ErrRedirect):
		return "redirect"
	case errors.Is(err, ErrResponseTooLarge):
		return "response_too_large"
	case errors.Is(err, ErrProvisionerPageLimit):
		return "provisioner_page_limit"
	case errors.Is(err, ErrCARejected):
		return "ca_rejected"
	case errors.Is(err, ErrCAUnavailable):
		return "ca_unavailable"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	default:
		return "pki_error"
	}
}
