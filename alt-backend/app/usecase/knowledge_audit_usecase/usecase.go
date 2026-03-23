package knowledge_audit_usecase

import (
	"alt/domain"
	"alt/port/knowledge_reproject_port"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

// Usecase orchestrates projection audit operations.
type Usecase struct {
	createAuditPort knowledge_reproject_port.CreateProjectionAuditPort
	listAuditsPort  knowledge_reproject_port.ListProjectionAuditsPort
	comparePort     knowledge_reproject_port.CompareProjectionsPort
}

// NewUsecase creates a new audit usecase (backward compatible).
func NewUsecase(
	createAuditPort knowledge_reproject_port.CreateProjectionAuditPort,
	listAuditsPort knowledge_reproject_port.ListProjectionAuditsPort,
) *Usecase {
	return &Usecase{
		createAuditPort: createAuditPort,
		listAuditsPort:  listAuditsPort,
	}
}

// NewUsecaseWithVerification creates an audit usecase with projection verification capability.
func NewUsecaseWithVerification(
	createAuditPort knowledge_reproject_port.CreateProjectionAuditPort,
	listAuditsPort knowledge_reproject_port.ListProjectionAuditsPort,
	comparePort knowledge_reproject_port.CompareProjectionsPort,
) *Usecase {
	return &Usecase{
		createAuditPort: createAuditPort,
		listAuditsPort:  listAuditsPort,
		comparePort:     comparePort,
	}
}

// RunProjectionAudit validates inputs, runs verification if available, and creates an audit record.
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

	// Run verification if compare port is available
	if u.comparePort != nil {
		mismatchCount, detailsJSON := u.verifyProjection(ctx, projectionVersion)
		audit.MismatchCount = mismatchCount
		audit.DetailsJSON = detailsJSON
	}

	if err := u.createAuditPort.CreateProjectionAudit(ctx, audit); err != nil {
		return nil, fmt.Errorf("create projection audit: %w", err)
	}

	return audit, nil
}

// verifyProjection compares the given version with the previous version and returns mismatch info.
func (u *Usecase) verifyProjection(ctx context.Context, projectionVersion string) (int, json.RawMessage) {
	// Compare current version with "v1" as baseline
	diff, err := u.comparePort.CompareProjections(ctx, "v1", projectionVersion)
	if err != nil {
		details, _ := json.Marshal(map[string]string{"error": err.Error()})
		return 0, details
	}

	mismatchCount := 0
	var mismatches []map[string]any

	// Check item count drift
	if diff.FromItemCount > 0 {
		drift := math.Abs(float64(diff.ToItemCount-diff.FromItemCount)) / float64(diff.FromItemCount)
		if drift > 0.05 {
			mismatchCount++
			mismatches = append(mismatches, map[string]any{
				"type":           "item_count_drift",
				"from_count":     diff.FromItemCount,
				"to_count":       diff.ToItemCount,
				"drift_pct":      drift,
			})
		}
	}

	// Check score drift
	if diff.FromAvgScore > 0 {
		scoreDrift := math.Abs(diff.ToAvgScore-diff.FromAvgScore) / diff.FromAvgScore
		if scoreDrift > 0.1 {
			mismatchCount++
			mismatches = append(mismatches, map[string]any{
				"type":           "score_drift",
				"from_avg_score": diff.FromAvgScore,
				"to_avg_score":   diff.ToAvgScore,
				"drift_pct":      scoreDrift,
			})
		}
	}

	// Check empty summary rate drift
	if diff.FromItemCount > 0 && diff.ToItemCount > 0 {
		fromEmptyRate := float64(diff.FromEmptyCount) / float64(diff.FromItemCount)
		toEmptyRate := float64(diff.ToEmptyCount) / float64(diff.ToItemCount)
		if math.Abs(toEmptyRate-fromEmptyRate) > 0.05 {
			mismatchCount++
			mismatches = append(mismatches, map[string]any{
				"type":            "empty_rate_drift",
				"from_empty_rate": fromEmptyRate,
				"to_empty_rate":   toEmptyRate,
			})
		}
	}

	detailsJSON, _ := json.Marshal(map[string]any{
		"mismatches": mismatches,
		"diff":       diff,
	})
	return mismatchCount, detailsJSON
}

// ListProjectionAudits returns audit results for a projection.
func (u *Usecase) ListProjectionAudits(ctx context.Context, projectionName string, limit int) ([]domain.ProjectionAudit, error) {
	audits, err := u.listAuditsPort.ListProjectionAudits(ctx, projectionName, limit)
	if err != nil {
		return nil, fmt.Errorf("list projection audits: %w", err)
	}
	return audits, nil
}
