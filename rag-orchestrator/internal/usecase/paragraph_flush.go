package usecase

import (
	"strings"
	"time"
	"unicode/utf8"
)

// ParagraphFlusher accumulates streamed answer text and flushes at
// paragraph or sentence boundaries for provisional preview display.
//
// Flush conditions (from eval.md spec):
//  1. Paragraph boundary (\n\n) — always flush, ignoring minRunes
//  2. Sentence end (。！？.!?) — flush only when pending >= minRunes
//  3. Time-based — flush when timeout elapsed AND pending >= timeoutMinRunes
type ParagraphFlusher struct {
	pending         []rune
	lastFlushAt     time.Time
	minRunes        int
	timeoutDur      time.Duration
	timeoutMinRunes int
}

// NewParagraphFlusher creates a flusher with the given thresholds.
func NewParagraphFlusher(minRunes int, timeoutDur time.Duration, timeoutMinRunes int) *ParagraphFlusher {
	return &ParagraphFlusher{
		minRunes:        minRunes,
		timeoutDur:      timeoutDur,
		timeoutMinRunes: timeoutMinRunes,
		lastFlushAt:     time.Now(),
	}
}

// Feed adds text to the pending buffer and returns a flush if a boundary is hit.
func (f *ParagraphFlusher) Feed(text string) (flush string, ok bool) {
	f.pending = append(f.pending, []rune(text)...)

	// Check for paragraph boundary (\n\n) — flush regardless of minRunes
	s := string(f.pending)
	if idx := strings.Index(s, "\n\n"); idx != -1 {
		// Include the \n\n in the flushed text
		cutByte := idx + 2
		flush = s[:cutByte]
		remaining := s[cutByte:]
		f.pending = []rune(remaining)
		f.lastFlushAt = time.Now()
		return flush, true
	}

	// Check for sentence end with minRunes threshold
	if len(f.pending) >= f.minRunes {
		if flushIdx := f.lastSentenceEnd(); flushIdx >= 0 {
			flush = string(f.pending[:flushIdx+1])
			f.pending = f.pending[flushIdx+1:]
			f.lastFlushAt = time.Now()
			return flush, true
		}
	}

	return "", false
}

// TimeFlush checks if timeout has elapsed and enough runes are pending.
func (f *ParagraphFlusher) TimeFlush() (flush string, ok bool) {
	if len(f.pending) < f.timeoutMinRunes {
		return "", false
	}
	if time.Since(f.lastFlushAt) < f.timeoutDur {
		return "", false
	}
	flush = string(f.pending)
	f.pending = f.pending[:0]
	f.lastFlushAt = time.Now()
	return flush, true
}

// Drain returns all remaining pending text.
func (f *ParagraphFlusher) Drain() string {
	if len(f.pending) == 0 {
		return ""
	}
	s := string(f.pending)
	f.pending = f.pending[:0]
	return s
}

// PendingRunes returns the number of pending runes.
func (f *ParagraphFlusher) PendingRunes() int {
	return len(f.pending)
}

// lastSentenceEnd finds the last sentence-ending rune index in pending.
// Sentence ends: 。！？. ! ?
func (f *ParagraphFlusher) lastSentenceEnd() int {
	last := -1
	for i, r := range f.pending {
		if isSentenceEnd(r) {
			last = i
		}
	}
	return last
}

func isSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '.', '!', '?':
		return true
	}
	return false
}

// ensure utf8 is used (prevents unused import)
var _ = utf8.RuneCountInString
