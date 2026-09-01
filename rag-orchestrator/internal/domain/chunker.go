package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ChunkerVersion defines the version of the chunking algorithm.
// It is recorded on every document version, so a corpus can be queried for
// mixed states and a rebuild can tell "already current" from "still stale".
type ChunkerVersion string

const (
	// ChunkerVersionV1 is the initial paragraph-based chunker.
	ChunkerVersionV1 ChunkerVersion = "v1"
	// ChunkerVersionV2 is the improved chunker with min/max length constraints.
	ChunkerVersionV2 ChunkerVersion = "v2"
	// ChunkerVersionV3 is v2 with trailing short chunk handling.
	ChunkerVersionV3 ChunkerVersion = "v3"
	// ChunkerVersionV4 fixes mid-stream short chunk handling.
	ChunkerVersionV4 ChunkerVersion = "v4"
	// ChunkerVersionV5 fixes leading short chunks by prepending to first long paragraph.
	ChunkerVersionV5 ChunkerVersion = "v5"
	// ChunkerVersionV6 improves consecutive short chunk merging and raises MinChunkLength to 80.
	ChunkerVersionV6 ChunkerVersion = "v6"
	ChunkerVersionV7 ChunkerVersion = "v7"
	ChunkerVersionV8 ChunkerVersion = "v8"
	// ChunkerVersionV9 adds HTML sanitization before chunking.
	ChunkerVersionV9 ChunkerVersion = "v9"
	// ChunkerVersionV10 adds sentence-boundary overlap between adjacent chunks,
	// merge-forward fragment handling, and CJK-aware sentence detection.
	ChunkerVersionV10 ChunkerVersion = "v10"
)

const (
	// MinChunkLength is the minimum chunk length in runes. Paragraphs shorter
	// than this accumulate into the following paragraph.
	MinChunkLength = 80
	// MaxChunkLength is the maximum length in runes of a chunk's own text.
	// Overlap carried from the preceding chunk is added on top of it.
	MaxChunkLength = 1000
	// DefaultOverlapRatio is the fraction of MaxChunkLength carried from a
	// chunk into its successor: ~150 runes, which is one to a few sentences.
	DefaultOverlapRatio = 0.15

	// overlapSeparator joins the carried-over text to the chunk's own text.
	overlapSeparator = " "
)

// Chunk represents a single piece of a document.
type Chunk struct {
	Ordinal int    // Sequence number (0-indexed)
	Content string // The actual text content, including any carried-over overlap
	Hash    string // Stable hash of Content (SHA-256)
	// OverlapRunes is how many leading runes of Content were carried over from
	// the preceding chunk. Zero for the first chunk and when overlap is off.
	OverlapRunes int
}

// Chunker defines the interface for splitting text into chunks.
type Chunker interface {
	Chunk(body string) ([]Chunk, error)
	Version() ChunkerVersion
}

// ChunkerConfig holds the tunable knobs of the chunker. Every knob
// participates in Version(), because a document chunked under different knobs
// is not interchangeable with one chunked under the defaults.
type ChunkerConfig struct {
	// OverlapRatio is the fraction of MaxChunkLength carried from a chunk into
	// its successor. Zero disables overlap.
	OverlapRatio float64
}

// DefaultChunkerConfig returns the configuration NewChunker uses.
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{OverlapRatio: DefaultOverlapRatio}
}

func (c ChunkerConfig) validate() error {
	if math.IsNaN(c.OverlapRatio) || c.OverlapRatio < 0 || c.OverlapRatio >= 1 {
		return fmt.Errorf("chunker: overlap ratio %v is out of range [0, 1)", c.OverlapRatio)
	}
	return nil
}

// version encodes non-default knobs into the recorded version, so a corpus
// chunked with a tuned overlap does not read back as up to date against a
// default-config rebuild.
func (c ChunkerConfig) version() ChunkerVersion {
	if c == DefaultChunkerConfig() {
		return ChunkerVersionV10
	}
	return ChunkerVersion(fmt.Sprintf("%s/ov%g", ChunkerVersionV10, c.OverlapRatio))
}

type overlapChunker struct {
	cfg     ChunkerConfig
	version ChunkerVersion
}

// NewChunker creates the default Chunker (ChunkerVersionV10).
func NewChunker() Chunker {
	cfg := DefaultChunkerConfig()
	return &overlapChunker{cfg: cfg, version: cfg.version()}
}

// NewChunkerWithConfig creates a Chunker with tuned knobs. An out-of-range
// knob is an error, not a silently clamped value: the recorded chunker version
// would otherwise claim a chunking the corpus does not have.
func NewChunkerWithConfig(cfg ChunkerConfig) (Chunker, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &overlapChunker{cfg: cfg, version: cfg.version()}, nil
}

func (c *overlapChunker) Version() ChunkerVersion {
	return c.version
}

// Chunk sanitizes, splits and overlaps a document body.
//
// The pipeline is: HTML sanitization (tags, script/style subtrees and known
// navigation boilerplate never reach an embedding) -> paragraph split ->
// merge-forward so no fragment is dropped -> sentence-aligned split at
// MaxChunkLength -> sentence-boundary overlap between adjacent chunks.
func (c *overlapChunker) Chunk(body string) ([]Chunk, error) {
	sanitized := SanitizeHTML(body)

	normalized := strings.ReplaceAll(sanitized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	var paragraphs []string
	for _, part := range strings.Split(normalized, "\n\n") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
	}

	cores := splitToMax(mergeForward(paragraphs))

	chunks := make([]Chunk, 0, len(cores))
	for i, core := range cores {
		content := core
		overlapRunes := 0

		if i > 0 {
			tail := overlapTail(cores[i-1], overlapBudget(cores[i-1], c.cfg.OverlapRatio))
			if tail != "" {
				content = tail + overlapSeparator + core
				overlapRunes = utf8.RuneCountInString(tail)
			}
		}

		sum := sha256.Sum256([]byte(content))
		chunks = append(chunks, Chunk{
			Ordinal:      i,
			Content:      content,
			Hash:         hex.EncodeToString(sum[:]),
			OverlapRunes: overlapRunes,
		})
	}

	return chunks, nil
}

// mergeForward groups paragraphs so that every group reaches MinChunkLength.
//
// A fragment shorter than the minimum accumulates into the FOLLOWING
// paragraph. Merging forward — rather than folding a fragment backwards into
// whatever section happened to precede it — keeps a heading or lead-in
// attached to the text it introduces, and means no paragraph can be dropped
// for being short. Only a trailing remainder, which has no successor left to
// merge into, is appended to the previous group.
func mergeForward(paragraphs []string) []string {
	var groups []string
	pending := ""

	for _, p := range paragraphs {
		if pending == "" {
			pending = p
		} else {
			pending = pending + "\n\n" + p
		}
		if utf8.RuneCountInString(pending) >= MinChunkLength {
			groups = append(groups, pending)
			pending = ""
		}
	}

	if pending != "" {
		if len(groups) > 0 {
			groups[len(groups)-1] = groups[len(groups)-1] + "\n\n" + pending
		} else {
			groups = append(groups, pending)
		}
	}

	return groups
}

// splitToMax breaks groups longer than MaxChunkLength into sentence-aligned
// cores. A single sentence longer than the maximum is cut at rune boundaries:
// there is no smaller unit left to align to.
func splitToMax(groups []string) []string {
	var out []string

	for _, group := range groups {
		if utf8.RuneCountInString(group) <= MaxChunkLength {
			out = appendTrimmed(out, group)
			continue
		}

		current := ""
		for _, sentence := range splitSentences(group) {
			sentenceLen := utf8.RuneCountInString(sentence)

			if sentenceLen > MaxChunkLength {
				out = appendTrimmed(out, current)
				current = ""
				for _, piece := range sliceRunes(sentence, MaxChunkLength) {
					out = appendTrimmed(out, piece)
				}
				continue
			}

			if utf8.RuneCountInString(current)+sentenceLen > MaxChunkLength {
				out = appendTrimmed(out, current)
				current = ""
			}
			current += sentence
		}
		out = appendTrimmed(out, current)
	}

	return out
}

func appendTrimmed(out []string, s string) []string {
	if trimmed := strings.TrimSpace(s); trimmed != "" {
		return append(out, trimmed)
	}
	return out
}

func sliceRunes(s string, size int) []string {
	runes := []rune(s)
	var out []string
	for len(runes) > 0 {
		take := size
		if len(runes) < take {
			take = len(runes)
		}
		out = append(out, string(runes[:take]))
		runes = runes[take:]
	}
	return out
}

// overlapBudget is how many runes may be carried from prev into its successor:
// a fraction of a full chunk, but never more than half of prev, so a short
// chunk is not duplicated wholesale into the next one.
func overlapBudget(prev string, ratio float64) int {
	if ratio <= 0 {
		return 0
	}
	budget := int(math.Ceil(ratio * float64(MaxChunkLength)))
	if half := utf8.RuneCountInString(prev) / 2; budget > half {
		budget = half
	}
	return budget
}

// overlapTail returns the text carried from prev into the next chunk: the
// longest run of whole trailing sentences that fits the budget. When not even
// the last sentence fits, the tail is cut to the budget and advanced to the
// next word boundary — partial context beats none, and CJK text has no word
// boundary to find anyway.
func overlapTail(prev string, budget int) string {
	if budget <= 0 {
		return ""
	}

	sentences := splitSentences(prev)
	taken, start := 0, len(sentences)
	for i := len(sentences) - 1; i >= 0; i-- {
		n := utf8.RuneCountInString(sentences[i])
		if taken+n > budget {
			break
		}
		taken += n
		start = i
	}
	if start < len(sentences) {
		return strings.TrimSpace(strings.Join(sentences[start:], ""))
	}

	runes := []rune(prev)
	tail := runes[len(runes)-budget:]
	for i, r := range tail {
		if unicode.IsSpace(r) {
			if advanced := strings.TrimSpace(string(tail[i+1:])); advanced != "" {
				return advanced
			}
			break
		}
	}
	return strings.TrimSpace(string(tail))
}

// splitSentences splits text at sentence terminators, preserving every rune:
// concatenating the result reproduces the input exactly.
//
// A CJK terminator (。！？) ends a sentence on its own. Japanese prose puts no
// space after it, so a splitter that requires trailing whitespace finds no
// boundary at all in a Japanese article and degrades to cutting at a fixed
// rune count, mid-sentence. An ASCII terminator (.!?) ends a sentence only
// when whitespace or the end of the text follows, so decimals and
// abbreviations stay intact.
func splitSentences(text string) []string {
	runes := []rune(text)
	var out []string
	start := 0

	for i := 0; i < len(runes); i++ {
		if !isSentenceTerminator(runes[i]) {
			continue
		}
		if isASCIITerminator(runes[i]) && i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
			continue
		}

		end := i + 1
		for end < len(runes) && (isSentenceTerminator(runes[end]) || isSentenceCloser(runes[end])) {
			end++
		}
		out = append(out, string(runes[start:end]))
		start = end
		i = end - 1
	}

	if start < len(runes) {
		out = append(out, string(runes[start:]))
	}
	return out
}

func isSentenceTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', '。', '！', '？':
		return true
	}
	return false
}

func isASCIITerminator(r rune) bool {
	return r == '.' || r == '!' || r == '?'
}

// isSentenceCloser reports whether r is punctuation that belongs to the
// sentence that just ended rather than to the next one.
func isSentenceCloser(r rune) bool {
	switch r {
	case '」', '』', '）', '"', '\'', '”', '’', ')', '…':
		return true
	}
	return false
}
