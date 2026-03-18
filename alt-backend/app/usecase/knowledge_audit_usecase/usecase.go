package knowledge_audit_usecase

import (
	"alt/domain"
	"alt/port/knowledge_reproject_port"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Usecase orchestrates projection audit operations.
type Usecase struct {
	createAuditPort knowledge_reproject_port.CreateProjectionAuditPort
	listAuditsPort  knowledge_reproject_port.ListProjectionAuditsPort
}

// NewUsecase creates a new audit usecase.
func NewUsecase(
	createAuditPort knowledge_reproject_port.CreateProjectionAuditPort,
	listAuditsPort knowledge_reproject_port.ListProjectionAuditsPort,
) *Usecase {
	return &Usecase{
		createAuditPort: createAuditPort,
		listAuditsPort:  listAuditsPort,
	}
}

// RunProjectionAudit validates inputs, creates an audit record, and returns it.
func (u *Usecase) RunProjectionAudit(ctx context.Context, projectionName, projectionVersion string, sampleSize int) (*domain.ProjectionAudit, error) {
	if projectionName == "" {
		return nil, fmt.Errorf("projection_name is required")
	}
	if sampleSize <= 0 {
		return nil, fmt.Errorf("sample_size must be greater than 0")
	}

	audit := &domain.ProjectionAudit{
		AuditID:           uuid.New(),
		ProjectionName:    projectionName,
		ProjectionVersion: projectionVersion,
		CheckedAt:         time.Now(),
		SampleSize:        sampleSize,
		MismatchCount:     0,
	}

	if err := u.createAuditPort.CreateProjectionAudit(ctx, audit); err != nil {
		return nil, fmt.Errorf("create projection audit: %w", err)
	}

	return audit, nil
}

// ListProjectionAudits returns audit results for a projection.
func (u *Usecase) ListProjectionAudits(ctx context.Context, projectionName string, limit int) ([]domain.ProjectionAudit, error) {
	audits, err := u.listAuditsPort.ListProjectionAudits(ctx, projectionName, limit)
	if err != nil {
		return nil, fmt.Errorf("list projection audits: %w", err)
	}
	return audits, nil
}
