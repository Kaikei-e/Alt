package bootstrap

import (
	"context"

	"alt/internal/pki"
)

// EnrollmentErrorType is a non-secret identifier for process-level PKI logs.
func EnrollmentErrorType(err error) string {
	return pki.LogSafeError(err)
}

// StartEnrollment is the composition-root hook for in-process cert lifecycle.
// Missing required config when enabled is a non-zero startup (caller exits).
func StartEnrollment(ctx context.Context, rt *Runtime, serviceName string) error {
	h, err := pki.Start(ctx, rt.Log, serviceName)
	if err != nil {
		return err
	}
	if h != nil {
		rt.AddShutdownHook(func(context.Context) error {
			h.Stop()
			return nil
		})
	}
	return nil
}
