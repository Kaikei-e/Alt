package usecase

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// OutputValidator ensures the LLM output follows expected structure and references retrieved chunks.
type OutputValidator struct{}

// NewOutputValidator creates a validator instance (currently stateless).
func NewOutputValidator() OutputValidator {
	return OutputValidator{}
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
		allowed := make(map[string]struct{}, len(contexts))
		for _, ctx := range contexts {
			allowed[ctx.ChunkID.String()] = struct{}{}
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
	Answer    string        `json:"answer"`
	Citations []LLMCitation `json:"citations"`
	Fallback  bool          `json:"fallback"`
	Reason    string        `json:"reason"`
}

// LLMCitation declares the chunks referenced in the final answer.
type LLMCitation struct {
	ChunkID string `json:"chunk_id"`
	Reason  string `json:"reason,omitempty"`
}

// convertLiteralEscapes converts literal backslash-n to actual newline characters.
// This handles cases where the LLM outputs literal \n instead of proper JSON escapes
// (a known issue with GPT-OSS models).
// Note: We intentionally only convert \n, not \t or \r, to avoid false positives
// with paths like C:\temp or C:\readme.txt
func convertLiteralEscapes(s string) string {
	return strings.ReplaceAll(s, "\\n", "\n")
}
