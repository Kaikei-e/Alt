package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

type surfacePlanRecomputedPayload struct {
	LensModeID     string
	PlannerVersion sovereignv1.SurfacePlannerVersion
	EntryInputs    []surfacePlanEntryInput
}

type surfacePlanEntryInput struct {
	EntryKey string
	Inputs   SurfaceScoreInputs
}

func (p *Projector) projectSurfacePlanRecomputed(
	ctx context.Context,
	ev *sovereign_db.KnowledgeEvent,
	fallbackLensModeID string,
) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	if ev.UserID == nil {
		return nil, nil
	}
	payload := parseSurfacePlanRecomputedPayload(ev.Payload, ev.OccurredAt)
	lensModeID := payload.LensModeID
	if lensModeID == "" {
		lensModeID = fallbackLensModeID
	}
	if lensModeID == "" {
		lensModeID = defaultLensModeID
	}

	var last *sovereign_db.KnowledgeLoopUpsertResult
	var appliedResult *sovereign_db.KnowledgeLoopUpsertResult
	applied := false
	for _, item := range payload.EntryInputs {
		bucket := decideBucketV2(item.Inputs)
		scoreInputs := marshalSurfaceScoreInputs(item.Inputs)
		res, err := p.repo.PatchKnowledgeLoopEntrySurfacePlan(
			ctx,
			ev.UserID.String(),
			ev.TenantID.String(),
			lensModeID,
			item.EntryKey,
			ev.EventSeq,
			bucket,
			pickRenderDepth(bucket),
			pickLoopPriority(bucket),
			payload.PlannerVersion,
			scoreInputs,
		)
		if err != nil {
			return last, fmt.Errorf("patch surface plan for %s: %w", item.EntryKey, err)
		}
		if res != nil {
			last = res
			if res.Applied {
				applied = true
				appliedResult = res
			}
		}
		observeSurfaceBucketAssigned(
			plannerVersionMetricLabel(payload.PlannerVersion),
			bucketMetricLabel(bucket),
		)
	}
	if applied {
		last = appliedResult
		return last, p.recomputeSurfaces(ctx, ev, lensModeID)
	}
	return last, nil
}

func parseSurfacePlanRecomputedPayload(raw json.RawMessage, occurredAt time.Time) surfacePlanRecomputedPayload {
	out := surfacePlanRecomputedPayload{
		PlannerVersion: sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2,
	}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	out.LensModeID = pickAnyStringField(m, "lens_mode_id", "lensModeId")
	if v, ok := pickAnyField(m, "planner_version", "plannerVersion"); ok {
		out.PlannerVersion = surfacePlannerVersionFromAny(v)
	}

	inputs, ok := pickAnyField(m, "entry_inputs", "entryInputs", "entry_score_inputs", "entryScoreInputs", "entries")
	if !ok {
		return out
	}
	for _, item := range anySlice(inputs) {
		entryMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entryKey := pickAnyStringField(entryMap, "entry_key", "entryKey")
		if !keyFormat.MatchString(entryKey) {
			continue
		}
		out.EntryInputs = append(out.EntryInputs, surfacePlanEntryInput{
			EntryKey: entryKey,
			Inputs:   parseSurfaceScoreInputs(entryMap, occurredAt),
		})
	}
	return out
}

func parseSurfaceScoreInputs(m map[string]any, occurredAt time.Time) SurfaceScoreInputs {
	in := SurfaceScoreInputs{
		FreshnessAt: occurredAt,
		EventType:   EventKnowledgeLoopSurfacePlanRecomputed,
	}
	if v, ok := pickUint32Field(m, "topic_overlap_count", "topicOverlapCount"); ok {
		in.TopicOverlapCount = v
	}
	if v, ok := pickUint32Field(m, "tag_overlap_count", "tagOverlapCount"); ok {
		in.TagOverlapCount = v
	}
	if v, ok := pickAnyField(m, "augur_link_id", "augurLinkId"); ok && stringFromAny(v) != "" {
		in.HasAugurLink = true
	}
	if v, ok := pickBoolField(m, "has_augur_link", "hasAugurLink"); ok {
		in.HasAugurLink = in.HasAugurLink || v
	}
	if v, ok := pickUint32Field(m, "version_drift_count", "versionDriftCount"); ok {
		in.VersionDriftCount = v
	}
	if v, ok := pickBoolField(m, "has_open_interaction", "hasOpenInteraction"); ok {
		in.HasOpenInteraction = v
	}
	if v, ok := pickTimeField(m, "freshness_at", "freshnessAt"); ok {
		in.FreshnessAt = v
	}
	if v := pickAnyStringField(m, "event_type", "eventType"); v != "" {
		in.EventType = v
	}
	if v := pickAnyStringField(m, "recap_topic_snapshot_id", "recapTopicSnapshotId"); v != "" {
		in.RecapTopicSnapshotID = v
	}
	if v, ok := pickUint32Field(m, "evidence_density", "evidenceDensity"); ok {
		in.EvidenceDensity = v
	}
	if v, ok := pickUint32Field(m, "recap_cluster_momentum", "recapClusterMomentum"); ok {
		in.RecapClusterMomentum = v
	}
	if v, ok := pickUint32Field(m, "question_continuation_score", "questionContinuationScore"); ok {
		in.QuestionContinuationScore = v
	}
	if v, ok := pickUint32Field(m, "report_worthiness_score", "reportWorthinessScore"); ok {
		in.ReportWorthinessScore = v
	}
	if v, ok := pickUint32Field(m, "staleness_score", "stalenessScore"); ok {
		in.StalenessScore = v
	}
	if v, ok := pickUint32Field(m, "contradiction_count", "contradictionCount"); ok {
		in.ContradictionCount = v
	}
	return in
}

func pickAnyField(m map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func pickAnyStringField(m map[string]any, keys ...string) string {
	if v, ok := pickAnyField(m, keys...); ok {
		return stringFromAny(v)
	}
	return ""
}

func pickUint32Field(m map[string]any, keys ...string) (uint32, bool) {
	if v, ok := pickAnyField(m, keys...); ok {
		return uint32FromAny(v)
	}
	return 0, false
}

func pickBoolField(m map[string]any, keys ...string) (bool, bool) {
	if v, ok := pickAnyField(m, keys...); ok {
		return boolFromAny(v)
	}
	return false, false
}

func pickTimeField(m map[string]any, keys ...string) (time.Time, bool) {
	if v, ok := pickAnyField(m, keys...); ok {
		return timeFromAny(v)
	}
	return time.Time{}, false
}

func anySlice(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	default:
		return nil
	}
}

func stringFromAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func uint32FromAny(v any) (uint32, bool) {
	switch t := v.(type) {
	case float64:
		if t < 0 || t > float64(^uint32(0)) {
			return 0, false
		}
		return uint32(t), true
	case int:
		if t < 0 {
			return 0, false
		}
		return uint32(t), true
	case int64:
		if t < 0 || t > int64(^uint32(0)) {
			return 0, false
		}
		return uint32(t), true
	case uint32:
		return t, true
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(t), 10, 32)
		if err != nil {
			return 0, false
		}
		return uint32(n), true
	default:
		return 0, false
	}
}

func boolFromAny(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(t))
		if err != nil {
			return false, false
		}
		return b, true
	default:
		return false, false
	}
}

func timeFromAny(v any) (time.Time, bool) {
	switch t := v.(type) {
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(t))
		if err != nil {
			return time.Time{}, false
		}
		return parsed.UTC(), true
	case map[string]any:
		seconds, ok := int64FromAny(t["seconds"])
		if !ok {
			return time.Time{}, false
		}
		nanos, _ := int64FromAny(t["nanos"])
		return time.Unix(seconds, nanos).UTC(), true
	default:
		return time.Time{}, false
	}
}

func int64FromAny(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int:
		return int64(t), true
	case int64:
		return t, true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func surfacePlannerVersionFromAny(v any) sovereignv1.SurfacePlannerVersion {
	switch t := v.(type) {
	case string:
		switch strings.ToUpper(strings.TrimSpace(t)) {
		case "SURFACE_PLANNER_VERSION_V1", "V1", "1":
			return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V1
		case "SURFACE_PLANNER_VERSION_V2", "V2", "2":
			return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2
		}
	case float64:
		if int64(t) == 1 {
			return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V1
		}
		if int64(t) == 2 {
			return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2
		}
	case int:
		if t == 1 {
			return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V1
		}
		if t == 2 {
			return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2
		}
	}
	return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2
}
