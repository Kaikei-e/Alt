package create_tag_set_version_usecase

import (
	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	"alt/shared/port/knowledge_event_port"
	"alt/utils/logger"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type completedVersion struct {
	versionID uuid.UUID
	tagsHash  string
}

type inflightVersion struct {
	versionID      uuid.UUID
	tagsHash       string
	generatedAt    time.Time
	rowCreated     bool
	supersededPrev *domain.TagSetVersion
}

// CreateTagSetVersionUsecase creates a new tag set version and emits an event.
type CreateTagSetVersionUsecase struct {
	tagSetPort         datahub_capability_port.CreateTagSetVersionPort
	eventPort          knowledge_event_port.AppendKnowledgeEventPort
	markSupersededPort datahub_capability_port.MarkTagSetVersionSupersededPort

	mu                sync.Mutex
	latestByArticle   map[uuid.UUID]completedVersion
	inflightByArticle map[uuid.UUID]*inflightVersion
}

// NewCreateTagSetVersionUsecase creates a new CreateTagSetVersionUsecase.
func NewCreateTagSetVersionUsecase(
	tagSetPort datahub_capability_port.CreateTagSetVersionPort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
	markSupersededPort datahub_capability_port.MarkTagSetVersionSupersededPort,
) *CreateTagSetVersionUsecase {
	// All three ports are required and wired unconditionally at composition root
	// (di/datahub/container.go, di/knowledge_module.go). A nil is a DI wiring bug,
	// not a disabled feature — panic instead of a defensive nil guard that would
	// silently skip the supersede fact (CLAUDE.md rule 8 / .claude/rules/di-wiring.md).
	if tagSetPort == nil {
		panic("create_tag_set_version_usecase: CreateTagSetVersionPort is nil — wire it at composition root")
	}
	if eventPort == nil {
		panic("create_tag_set_version_usecase: AppendKnowledgeEventPort is nil — wire it at composition root")
	}
	if markSupersededPort == nil {
		panic("create_tag_set_version_usecase: MarkTagSetVersionSupersededPort is nil — wire it at composition root")
	}
	return &CreateTagSetVersionUsecase{
		tagSetPort:         tagSetPort,
		eventPort:          eventPort,
		markSupersededPort: markSupersededPort,
		latestByArticle:    make(map[uuid.UUID]completedVersion),
		inflightByArticle:  make(map[uuid.UUID]*inflightVersion),
	}
}

var (
	// ErrMissingArticleID is returned when a tag set version has a nil article ID.
	ErrMissingArticleID = errors.New("article_id is required")
	// ErrMissingUserID is returned when a tag set version has a nil tenant owner.
	ErrMissingUserID = errors.New("user_id is required")
	// ErrEmptyTagsJSON is returned when a tag set version has empty tags JSON.
	ErrEmptyTagsJSON = errors.New("tags_json is required")
)

// Execute creates a tag set version and appends a TagSetVersionCreated event.
// If a previous version exists, it also marks the old versions as superseded
// and emits a TagSetSuperseded event.
func (u *CreateTagSetVersionUsecase) Execute(ctx context.Context, tsv domain.TagSetVersion) error {
	if tsv.ArticleID == uuid.Nil {
		return ErrMissingArticleID
	}
	if tsv.UserID == uuid.Nil {
		return ErrMissingUserID
	}
	if len(tsv.TagsJSON) == 0 {
		return ErrEmptyTagsJSON
	}

	tagsHash := canonicalTagsHash(tsv.TagsJSON)

	u.mu.Lock()
	if tsv.TagSetVersionID == uuid.Nil {
		if latest, ok := u.latestByArticle[tsv.ArticleID]; ok && latest.tagsHash == tagsHash {
			u.mu.Unlock()
			return nil
		}
	}

	inflight := u.inflightByArticle[tsv.ArticleID]
	if inflight != nil && inflight.tagsHash == tagsHash &&
		(tsv.TagSetVersionID == uuid.Nil || tsv.TagSetVersionID == inflight.versionID) {
		tsv.TagSetVersionID = inflight.versionID
		if tsv.GeneratedAt.IsZero() {
			tsv.GeneratedAt = inflight.generatedAt
		}
	} else {
		if tsv.TagSetVersionID == uuid.Nil {
			var prevID uuid.UUID
			if latest, ok := u.latestByArticle[tsv.ArticleID]; ok {
				prevID = latest.versionID
			}
			tsv.TagSetVersionID = deriveTagSetVersionID(tsv.ArticleID, prevID, tagsHash)
		}
		if tsv.GeneratedAt.IsZero() {
			tsv.GeneratedAt = time.Now()
		}
		inflight = &inflightVersion{
			versionID:   tsv.TagSetVersionID,
			tagsHash:    tagsHash,
			generatedAt: tsv.GeneratedAt,
		}
		u.inflightByArticle[tsv.ArticleID] = inflight
	}
	rowCreated := inflight.rowCreated
	cachedPrev := inflight.supersededPrev
	u.mu.Unlock()

	// Extract canonical tag names from the version being created so the
	// sovereign evidence accumulator can derive Cluster relations
	// (readPayloadStringSlice "tags"). Reproject-safe: names come from this
	// version's payload, never a latest-state lookup.
	tagNames := tagNamesFromJSON(tsv.TagsJSON)

	// Append TagSetVersionCreated before inserting the version row so a retry
	// deduplicates the event via the deterministic version ID.
	payload, _ := json.Marshal(map[string]any{
		"tag_set_version_id": tsv.TagSetVersionID.String(),
		"article_id":         tsv.ArticleID.String(),
		"generator":          tsv.Generator,
		"tags":               tagNames,
	})

	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tsv.UserID,
		UserID:        &tsv.UserID,
		ActorType:     domain.ActorService,
		ActorID:       "tag-generator",
		EventType:     domain.EventTagSetVersionCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   tsv.ArticleID.String(),
		DedupeKey:     fmt.Sprintf("TagSetVersionCreated:%s", tsv.TagSetVersionID),
		Payload:       payload,
	}

	if _, err := u.eventPort.AppendKnowledgeEvent(ctx, event); err != nil {
		return fmt.Errorf("append tag set version event: %w", err)
	}

	if !rowCreated {
		if err := u.tagSetPort.CreateTagSetVersion(ctx, tsv); err != nil {
			return fmt.Errorf("create tag set version: %w", err)
		}
		u.mu.Lock()
		if u.inflightByArticle[tsv.ArticleID] == inflight {
			inflight.rowCreated = true
		}
		u.mu.Unlock()
	}

	// Mark previous versions as superseded and emit TagSetSuperseded event.
	prev, err := u.markSupersededPort.MarkTagSetVersionSuperseded(ctx, tsv.ArticleID, tsv.TagSetVersionID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to mark tag set version superseded",
			"error", err, "article_id", tsv.ArticleID)
		return fmt.Errorf("mark tag set version superseded: %w", err)
	}
	if prev == nil {
		prev = cachedPrev
	} else {
		u.mu.Lock()
		if u.inflightByArticle[tsv.ArticleID] == inflight {
			inflight.supersededPrev = prev
		}
		u.mu.Unlock()
	}

	if prev != nil {
		prevTagNames := tagNamesFromJSON(prev.TagsJSON)

		supersedePayload, _ := json.Marshal(map[string]interface{}{
			"article_id":             tsv.ArticleID.String(),
			"new_tag_set_version_id": tsv.TagSetVersionID.String(),
			"old_tag_set_version_id": prev.TagSetVersionID.String(),
			"previous_tags":          prevTagNames,
		})

		supersedeEvent := domain.KnowledgeEvent{
			EventID:       uuid.New(),
			OccurredAt:    time.Now(),
			TenantID:      tsv.UserID,
			UserID:        &tsv.UserID,
			ActorType:     domain.ActorService,
			ActorID:       "tag-generator",
			EventType:     domain.EventTagSetSuperseded,
			AggregateType: domain.AggregateArticle,
			AggregateID:   tsv.ArticleID.String(),
			DedupeKey:     fmt.Sprintf("TagSetSuperseded:%s", tsv.TagSetVersionID),
			Payload:       supersedePayload,
		}

		// Append-first durable write: a failure to land the supersede event must
		// fail the command (like recall_snooze_usecase), not be swallowed. The
		// dedupe_key makes the caller's retry idempotent.
		if _, err := u.eventPort.AppendKnowledgeEvent(ctx, supersedeEvent); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to append TagSetSuperseded event",
				"error", err, "article_id", tsv.ArticleID)
			return fmt.Errorf("append tag set superseded event: %w", err)
		}
	}

	u.mu.Lock()
	if u.inflightByArticle[tsv.ArticleID] == inflight {
		delete(u.inflightByArticle, tsv.ArticleID)
	}
	u.latestByArticle[tsv.ArticleID] = completedVersion{
		versionID: tsv.TagSetVersionID,
		tagsHash:  tagsHash,
	}
	u.mu.Unlock()

	return nil
}

func deriveTagSetVersionID(articleID, prevVersionID uuid.UUID, tagsHash string) uuid.UUID {
	data := fmt.Sprintf("tag_set_version:%s:%s:%s", articleID, prevVersionID, tagsHash)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(data))
}

func canonicalTagsHash(tagsJSON json.RawMessage) string {
	var compact bytes.Buffer
	raw := []byte(tagsJSON)
	if err := json.Compact(&compact, raw); err == nil {
		raw = compact.Bytes()
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// tagNamesFromJSON extracts the canonical tag-name list from a TagSetVersion's
// TagsJSON ([{"name":...,"confidence":...}, ...]). Empty names are skipped.
func tagNamesFromJSON(tagsJSON json.RawMessage) []string {
	var items []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(tagsJSON, &items); err != nil {
		return nil
	}
	var names []string
	for _, t := range items {
		if t.Name != "" {
			names = append(names, t.Name)
		}
	}
	return names
}
