package recap

import (
	"time"

	recapv2 "alt/gen/proto/alt/recap/v2"

	"alt/domain"
	"alt/utils/safeconv"
)

// eveningPulseDomainToProto converts domain.EveningPulse to proto response.
func eveningPulseDomainToProto(pulse *domain.EveningPulse) *recapv2.GetEveningPulseResponse {
	topics := make([]*recapv2.PulseTopic, len(pulse.Topics))
	for i, t := range pulse.Topics {
		topics[i] = &recapv2.PulseTopic{
			ClusterId:              t.ClusterID,
			Role:                   topicRoleToProto(t.Role),
			Title:                  t.Title,
			Rationale:              rationaleToProto(t.Rationale),
			ArticleCount:           safeconv.Int32(t.ArticleCount),
			SourceCount:            safeconv.Int32(t.SourceCount),
			TimeAgo:                t.TimeAgo,
			ArticleIds:             t.ArticleIDs,
			RepresentativeArticles: representativeArticlesToProto(t.RepresentativeArticles),
			TopEntities:            t.TopEntities,
			SourceNames:            t.SourceNames,
		}
		if t.Tier1Count != nil {
			tier1 := safeconv.Int32(*t.Tier1Count)
			topics[i].Tier1Count = &tier1
		}
		if t.TrendMultiplier != nil {
			topics[i].TrendMultiplier = t.TrendMultiplier
		}
		if t.Genre != nil {
			topics[i].Genre = t.Genre
		}
	}

	resp := &recapv2.GetEveningPulseResponse{
		JobId:       pulse.JobID,
		Date:        pulse.Date,
		GeneratedAt: pulse.GeneratedAt.Format(time.RFC3339),
		Status:      pulseStatusToProto(pulse.Status),
		Topics:      topics,
	}

	if pulse.QuietDay != nil {
		resp.QuietDay = quietDayToProto(pulse.QuietDay)
	}

	return resp
}

// topicRoleToProto converts domain.TopicRole to proto enum.
func topicRoleToProto(role domain.TopicRole) recapv2.TopicRole {
	switch role {
	case domain.TopicRoleNeedToKnow:
		return recapv2.TopicRole_TOPIC_ROLE_NEED_TO_KNOW
	case domain.TopicRoleTrend:
		return recapv2.TopicRole_TOPIC_ROLE_TREND
	case domain.TopicRoleSerendipity:
		return recapv2.TopicRole_TOPIC_ROLE_SERENDIPITY
	default:
		return recapv2.TopicRole_TOPIC_ROLE_UNSPECIFIED
	}
}

// pulseStatusToProto converts domain.PulseStatus to proto enum.
func pulseStatusToProto(status domain.PulseStatus) recapv2.PulseStatus {
	switch status {
	case domain.PulseStatusNormal:
		return recapv2.PulseStatus_PULSE_STATUS_NORMAL
	case domain.PulseStatusPartial:
		return recapv2.PulseStatus_PULSE_STATUS_PARTIAL
	case domain.PulseStatusQuietDay:
		return recapv2.PulseStatus_PULSE_STATUS_QUIET_DAY
	case domain.PulseStatusError:
		return recapv2.PulseStatus_PULSE_STATUS_ERROR
	default:
		return recapv2.PulseStatus_PULSE_STATUS_UNSPECIFIED
	}
}

// confidenceToProto converts domain.Confidence to proto enum.
func confidenceToProto(conf domain.Confidence) recapv2.Confidence {
	switch conf {
	case domain.ConfidenceHigh:
		return recapv2.Confidence_CONFIDENCE_HIGH
	case domain.ConfidenceMedium:
		return recapv2.Confidence_CONFIDENCE_MEDIUM
	case domain.ConfidenceLow:
		return recapv2.Confidence_CONFIDENCE_LOW
	default:
		return recapv2.Confidence_CONFIDENCE_UNSPECIFIED
	}
}

// rationaleToProto converts domain.PulseRationale to proto.
func rationaleToProto(r domain.PulseRationale) *recapv2.PulseRationale {
	return &recapv2.PulseRationale{
		Text:       r.Text,
		Confidence: confidenceToProto(r.Confidence),
	}
}

// representativeArticlesToProto converts domain representative articles to proto.
func representativeArticlesToProto(articles []domain.RepresentativeArticle) []*recapv2.RepresentativeArticle {
	if articles == nil {
		return nil
	}
	result := make([]*recapv2.RepresentativeArticle, len(articles))
	for i, a := range articles {
		result[i] = &recapv2.RepresentativeArticle{
			ArticleId:   a.ArticleID,
			Title:       a.Title,
			SourceUrl:   a.SourceURL,
			SourceName:  a.SourceName,
			PublishedAt: a.PublishedAt,
		}
	}
	return result
}

// quietDayToProto converts domain.QuietDayInfo to proto.
func quietDayToProto(qd *domain.QuietDayInfo) *recapv2.QuietDayInfo {
	highlights := make([]*recapv2.WeeklyHighlight, len(qd.WeeklyHighlights))
	for i, h := range qd.WeeklyHighlights {
		highlights[i] = &recapv2.WeeklyHighlight{
			Id:    h.ID,
			Title: h.Title,
			Date:  h.Date,
			Role:  h.Role,
		}
	}
	return &recapv2.QuietDayInfo{
		Message:          qd.Message,
		WeeklyHighlights: highlights,
	}
}
