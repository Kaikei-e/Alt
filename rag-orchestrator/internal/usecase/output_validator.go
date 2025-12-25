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
	if err := json.Unmarshal([]byte(trimmed), &answer); err != nil {
		return nil, fmt.Errorf("failed to parse llm response: %w", err)
	}

	if len(answer.Quotes) == 0 && !answer.Fallback {
		return nil, errors.New("missing quotes in response")
	}
	if len(answer.Citations) == 0 && !answer.Fallback {
		return nil, errors.New("missing citations in response")
	}

	if len(contexts) > 0 {
		allowed := make(map[string]struct{}, len(contexts))
		for _, ctx := range contexts {
			allowed[ctx.ChunkID.String()] = struct{}{}
		}
		for _, cite := range answer.Citations {
			if cite.ChunkID == "" {
				return nil, errors.New("citation missing chunk_id")
			}
			if _, ok := allowed[cite.ChunkID]; !ok {
				return nil, fmt.Errorf("citation references unknown chunk %s", cite.ChunkID)
			}
		}
	}

	return &answer, nil
}

// LLMAnswer models the JSON output the prompt format section enforces.
type LLMAnswer struct {
	Quotes    []LLMQuote    `json:"quotes"`
	Answer    string        `json:"answer"`
	Citations []LLMCitation `json:"citations"`
	Fallback  bool          `json:"fallback"`
	Reason    string        `json:"reason"`
}

// LLMQuote describes a quoted chunk the LLM must produce before answering.
type LLMQuote struct {
	ChunkID string `json:"chunk_id"`
	Quote   string `json:"quote"`
}

// LLMCitation declares the chunks referenced in the final answer.
type LLMCitation struct {
	ChunkID string   `json:"chunk_id"`
	URL     string   `json:"url"`
	Title   string   `json:"title"`
	Score   *float32 `json:"score"`
}
