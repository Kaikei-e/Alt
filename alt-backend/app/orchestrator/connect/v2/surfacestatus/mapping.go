package surfacestatus

import (
	"alt/domain"
	recapv2 "alt/gen/proto/alt/recap/v2"
)

// ToProto converts domain.RecapSurfaceStatus to the shared proto message.
func ToProto(s *domain.RecapSurfaceStatus) *recapv2.RecapSurfaceStatus {
	if s == nil {
		return nil
	}
	var latestRun *recapv2.RecapSurfaceRun
	if s.LatestRun != nil {
		latestRun = &recapv2.RecapSurfaceRun{
			JobId:     s.LatestRun.JobID,
			Status:    s.LatestRun.Status,
			KickedAt:  s.LatestRun.KickedAt,
			UpdatedAt: s.LatestRun.UpdatedAt,
		}
	}
	reasons := make([]*recapv2.RecapDegradationReason, len(s.DegradationReasons))
	for i, r := range s.DegradationReasons {
		reasons[i] = &recapv2.RecapDegradationReason{
			Code:   r.Code,
			Detail: r.Detail,
		}
	}
	return &recapv2.RecapSurfaceStatus{
		GeneratedAt:        s.GeneratedAt,
		WindowStart:        s.WindowStart,
		WindowEnd:          s.WindowEnd,
		IsStale:            s.IsStale,
		StaleAfter:         s.StaleAfter,
		LatestRun:          latestRun,
		DegradationReasons: reasons,
	}
}
