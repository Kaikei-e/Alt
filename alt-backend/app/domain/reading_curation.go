package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxCurationTextRunes bounds plain-text excerpts and 1-line user notes to
	// keep micro-externalizations concise and safe against oversized payloads.
	MaxCurationTextRunes = 500

	// Aggregate types for reading micro-externalizations and theme curations.
	AggregateReadingAnnotation    = "reading_annotation"
	AggregateReadingThemeCuration = "reading_theme_curation"
)

var (
	ErrInvalidAnnotationInput = errors.New("invalid annotation input")
	ErrInvalidCurationInput   = errors.New("invalid theme curation input")
)

// AnnotationStance classifies the user's epistemic reaction to an article takeaway.
type AnnotationStance string

const (
	StanceSurprise      AnnotationStance = "surprise"
	StanceQuestion      AnnotationStance = "question"
	StanceContradiction AnnotationStance = "contradiction"
	StanceInsight       AnnotationStance = "insight"
)

// IsValidAnnotationStance reports whether s is an allow-listed stance value.
func IsValidAnnotationStance(s AnnotationStance) bool {
	switch s {
	case StanceSurprise, StanceQuestion, StanceContradiction, StanceInsight:
		return true
	default:
		return false
	}
}

// ThemeCurationKind classifies a user's co-creation action on a theme page.
type ThemeCurationKind string

const (
	CurationPinKeyPoint         ThemeCurationKind = "pin_key_point"
	CurationDismissKeyPoint     ThemeCurationKind = "dismiss_key_point"
	CurationAddQuestion         ThemeCurationKind = "add_question"
	CurationResolveQuestion     ThemeCurationKind = "resolve_question"
	CurationAddSeedNote         ThemeCurationKind = "add_seed_note"
	CurationPromoteAugurInsight ThemeCurationKind = "promote_augur_insight"
)

// IsValidThemeCurationKind reports whether k is an allow-listed curation kind.
func IsValidThemeCurationKind(k ThemeCurationKind) bool {
	switch k {
	case CurationPinKeyPoint,
		CurationDismissKeyPoint,
		CurationAddQuestion,
		CurationResolveQuestion,
		CurationAddSeedNote,
		CurationPromoteAugurInsight:
		return true
	default:
		return false
	}
}

// ArticleAnnotation is a pure domain representation of one user takeaway.
type ArticleAnnotation struct {
	AnnotationID uuid.UUID
	ArticleID    uuid.UUID
	Stance       AnnotationStance
	Excerpt      string
	Note         string
	CreatedAt    time.Time
}

// ThemeCuration is a pure domain representation of one user theme co-creation action.
type ThemeCuration struct {
	CurationID          uuid.UUID
	ThemeID             uuid.UUID
	Kind                ThemeCurationKind
	TargetHash          string
	TargetText          string
	UserNote            string
	AugurConversationID string
	CreatedAt           time.Time
}

// SanitizeMicroText strips control characters (except space/newline/tab),
// trims surrounding whitespace, and enforces MaxCurationTextRunes.
func SanitizeMicroText(raw string) (string, bool) {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		b.WriteRune(r)
	}
	cleaned := strings.TrimSpace(b.String())
	if utf8.RuneCountInString(cleaned) > MaxCurationTextRunes {
		return "", false
	}
	return cleaned, true
}

// HashTargetSentence returns a stable 16-hex-char SHA-256 prefix for matching
// curated sentences across renders and versions.
func HashTargetSentence(text string) string {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:8])
}
