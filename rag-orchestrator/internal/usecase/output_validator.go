package usecase

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// OutputValidator ensures the LLM output follows expected structure and references retrieved chunks.
type OutputValidator struct {
	minAnswerLength int
}

// NewOutputValidator creates a validator instance.
// minAnswerLength sets the minimum rune count for answers; 0 disables the check.
func NewOutputValidator(minAnswerLength int) OutputValidator {
	return OutputValidator{minAnswerLength: minAnswerLength}
}

// Validate parses and checks the JSON output emitted by the LLM.
func (v OutputValidator) Validate(raw string, contexts []ContextItem) (*LLMAnswer, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("llm response is empty")
	}

	var answer LLMAnswer
	// 1. Try standard unmarshal
	if err := json.Unmarshal([]byte(trimmed), &answer); err != nil {
		// 2. Try repairing JSON (often missing closing brace/quote)
		repaired := repairJSON(trimmed)
		if err2 := json.Unmarshal([]byte(repaired), &answer); err2 != nil {
			// 3. Fallback: Regex extraction for Answer only
			// This is useful if the stream cut off mid-JSON but we have some answer text
			extractedAnswer := extractAnswerOnly(trimmed)
			if extractedAnswer != "" {
				// Apply the same post-processing as the normal path
				extractedAnswer = strings.TrimSpace(extractedAnswer)
				extractedAnswer = convertLiteralEscapes(extractedAnswer)
				return &LLMAnswer{
					Answer:   extractedAnswer,
					Fallback: false, // It's technically a fallback, but we retrieved content
					Reason:   "recovered_from_truncated_json",
				}, nil
			}
			return nil, fmt.Errorf("failed to parse llm response (raw: %s): %w", trimmed, err)
		}
	}

	// Validate Citations if present
	if len(contexts) > 0 {
		allowed := make(map[string]struct{}, len(contexts)*2)
		for i, ctx := range contexts {
			allowed[ctx.ChunkID.String()] = struct{}{}
			// Also allow 1-based index citations (e.g., "1", "2", "3")
			// which the prompt instructs the LLM to use
			allowed[fmt.Sprintf("%d", i+1)] = struct{}{}
		}
		// Validate citations - filter out invalid ones
		validCitations := make([]LLMCitation, 0, len(answer.Citations))
		for _, cite := range answer.Citations {
			if cite.ChunkID == "" {
				continue // Skip empty citations
			}
			if _, exists := allowed[cite.ChunkID]; exists {
				validCitations = append(validCitations, cite)
			}
			// Invalid citations (not in allowed set) are silently dropped
		}
		answer.Citations = validCitations
	}

	// Sanitize output
	answer.Answer = strings.TrimSpace(answer.Answer)
	// Convert any remaining literal escape sequences (for GPT-OSS model quirks)
	// The model sometimes outputs literal \n instead of proper JSON escapes
	answer.Answer = convertLiteralEscapes(answer.Answer)

	// Reject empty answer when model didn't flag as fallback.
	// This catches the 8B model "headers-only" failure where the model outputs
	// section headings but runs out of tokens before filling content.
	if answer.Answer == "" && !answer.Fallback {
		return nil, errors.New("llm returned empty answer without fallback flag")
	}

	// Soft validation: flag short answers (rune count) but don't reject.
	// 8B models may produce shorter answers for narrow queries.
	if v.minAnswerLength > 0 && utf8.RuneCountInString(answer.Answer) < v.minAnswerLength {
		answer.ShortAnswer = true
	}

	return &answer, nil
}

func repairJSON(raw string) string {
	// Simple heuristic: if it doesn't end with "}", try appending it.
	// If it ends with string content, close quote then brace.
	// This is very naive but covers common "truncated at end" cases.
	trimmed := strings.TrimSpace(raw)
	if strings.HasSuffix(trimmed, "}") {
		return trimmed
	}
	// Try appending "}"
	try1 := trimmed + "}"
	if json.Valid([]byte(try1)) {
		return try1
	}
	// Try appending "]}" (for array truncation?)
	try2 := trimmed + "]}"
	if json.Valid([]byte(try2)) {
		return try2
	}
	// Try appending "\"}" (for string truncation)
	try3 := trimmed + "\"}"
	if json.Valid([]byte(try3)) {
		return try3
	}
	// Try appending "]\"}" (array + string)
	try4 := trimmed + "\"]}"
	if json.Valid([]byte(try4)) {
		return try4
	}
	return raw
}

func extractAnswerOnly(raw string) string {
	// Look for "answer": "..." pattern
	// Dealing with escaped quotes is tricky with regex, simpler manual loop or smart index finding

	key := "\"answer\":"
	idx := strings.Index(raw, key)
	if idx == -1 {
		return ""
	}

	// Start of value
	start := idx + len(key)
	// Skip whitespace
	for start < len(raw) && (raw[start] == ' ' || raw[start] == '\n' || raw[start] == '\t' || raw[start] == '\r') {
		start++
	}

	if start >= len(raw) || raw[start] != '"' {
		return "" // unexpected format (maybe null?)
	}
	start++ // skip opening quote

	// Find end quote
	// Use a simple scan handling escapings
	var sb strings.Builder
	escaped := false
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if escaped {
			// Properly unescape JSON escape sequences
			switch c {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			default:
				sb.WriteByte(c)
			}
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			// End of string found
			return sb.String()
		}
		sb.WriteByte(c)
	}

	// If we reached here, the string is truncated (no closing quote).
	// Return what we have!
	return sb.String()
}

// LLMAnswer models the JSON output the prompt format section enforces.
type LLMAnswer struct {
	Answer      string        `json:"answer"`
	Citations   []LLMCitation `json:"citations"`
	Fallback    bool          `json:"fallback"`
	Reason      string        `json:"reason"`
	ShortAnswer bool          // Internal flag: true when answer is below minAnswerLength (rune count)
}

// LLMCitation declares the chunks referenced in the final answer.
type LLMCitation struct {
	ChunkID string `json:"chunk_id"`
	Reason  string `json:"reason,omitempty"`
}

// AssessAnswerQuality performs post-generation quality checks on the answer.
// All checks are string-based (no LLM calls).
// Returns a list of quality flag names for any failing checks.
func AssessAnswerQuality(answer, query string, citations []LLMCitation, intentType IntentType) []string {
	var flags []string

	// 1. Coverage check: do query keywords appear in the answer?
	if !checkKeywordCoverage(answer, query) {
		flags = append(flags, "low_keyword_coverage")
	}

	// 2. Citation density: long answers should have citations
	if !checkCitationDensity(answer, citations) {
		flags = append(flags, "low_citation_density")
	}

	// 3. Coherence: does the answer end with proper punctuation?
	if !checkCoherentEnding(answer) {
		flags = append(flags, "incoherent_ending")
	}

	// 4. Fact-check intent: answer should contain evidence keywords
	if intentType == IntentFactCheck && !checkFactCheckEvidence(answer) {
		flags = append(flags, "fact_check_missing_evidence")
	}

	return flags
}

func checkKeywordCoverage(answer, query string) bool {
	// For article-scoped queries, extract just the user question to avoid
	// matching English article title keywords against Japanese answer text.
	// The full query format is: "Regarding the article: TITLE [articleId: UUID]\n\nQuestion:\nUSER_QUESTION"
	effectiveQuery := extractUserQuestion(query)

	// Japanese/CJK text doesn't use spaces between words, so strings.Fields
	// treats the entire sentence as one "word". Word-based coverage is meaningless.
	// Skip for CJK-dominant queries.
	if isCJKDominant(effectiveQuery) {
		return true
	}

	lowerAnswer := strings.ToLower(answer)
	words := strings.Fields(strings.ToLower(effectiveQuery))

	significant := 0
	covered := 0
	for _, w := range words {
		if utf8.RuneCountInString(w) < 3 {
			continue
		}
		significant++
		if strings.Contains(lowerAnswer, w) {
			covered++
		}
	}

	if significant == 0 {
		return true
	}
	return float64(covered)/float64(significant) >= 0.5
}

// isCJKDominant returns true when more than 30% of runes are CJK characters.
// This matches the locale detection heuristic used elsewhere in rag-orchestrator.
func isCJKDominant(s string) bool {
	total := 0
	cjk := 0
	for _, r := range s {
		total++
		if (r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
			(r >= 0xFF00 && r <= 0xFFEF) { // Fullwidth Forms
			cjk++
		}
	}
	if total == 0 {
		return false
	}
	return float64(cjk)/float64(total) > 0.3
}

// extractUserQuestion strips article metadata from article-scoped queries,
// returning only the user's actual question for keyword analysis.
// For non-article-scoped queries, returns the original query unchanged.
func extractUserQuestion(query string) string {
	const sep = "\n\nQuestion:\n"
	idx := strings.LastIndex(query, sep)
	if idx >= 0 {
		return strings.TrimSpace(query[idx+len(sep):])
	}
	return query
}

func checkCitationDensity(answer string, citations []LLMCitation) bool {
	answerLen := utf8.RuneCountInString(answer)
	if answerLen < 200 {
		return true // Short answers don't need citations
	}
	// At least 1 citation per 500 characters
	minCitations := answerLen / 500
	if minCitations < 1 {
		minCitations = 1
	}
	return len(citations) >= minCitations
}

func checkCoherentEnding(answer string) bool {
	trimmed := strings.TrimSpace(answer)
	if len(trimmed) == 0 {
		return true
	}
	// Check if answer ends with sentence-ending punctuation
	endings := []string{"。", ".", "！", "!", "？", "?", "）", ")", "」", "\"", "\n"}
	for _, e := range endings {
		if strings.HasSuffix(trimmed, e) {
			return true
		}
	}
	return false
}

func checkFactCheckEvidence(answer string) bool {
	evidenceKeywords := []string{"根拠", "出典", "研究", "evidence", "source", "according", "study", "report", "データ", "調査"}
	lower := strings.ToLower(answer)
	for _, kw := range evidenceKeywords {
		if strings.Contains(lower, kw) || strings.Contains(answer, kw) {
			return true
		}
	}
	return false
}

// convertLiteralEscapes converts literal backslash-n to actual newline characters.
// This handles cases where the LLM outputs literal \n instead of proper JSON escapes
// (a known issue with GPT-OSS models).
// Note: We intentionally only convert \n, not \t or \r, to avoid false positives
// with paths like C:\temp or C:\readme.txt
func convertLiteralEscapes(s string) string {
	return strings.ReplaceAll(s, "\\n", "\n")
}
