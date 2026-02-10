package usecase_test

import (
	"fmt"
	"strings"
	"testing"

	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
)

func BenchmarkOutputValidator_ShortAnswer(b *testing.B) {
	validator := usecase.NewOutputValidator()
	input := `{"answer": "Short answer about AI.", "citations": [], "fallback": false, "reason": ""}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = validator.Validate(input, nil)
	}
}

func BenchmarkOutputValidator_LongAnswer(b *testing.B) {
	validator := usecase.NewOutputValidator()
	longAnswer := strings.Repeat("This is a detailed answer about artificial intelligence. ", 100)
	input := fmt.Sprintf(`{"answer": "%s", "citations": [], "fallback": false, "reason": ""}`, longAnswer)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = validator.Validate(input, nil)
	}
}

func BenchmarkOutputValidator_WithCitations(b *testing.B) {
	validator := usecase.NewOutputValidator()
	chunkIDs := make([]uuid.UUID, 10)
	contexts := make([]usecase.ContextItem, 10)
	var citationsJSON strings.Builder
	citationsJSON.WriteString("[")
	for i := 0; i < 10; i++ {
		chunkIDs[i] = uuid.New()
		contexts[i] = usecase.ContextItem{
			ChunkID:   chunkIDs[i],
			ChunkText: "chunk text",
		}
		if i > 0 {
			citationsJSON.WriteString(",")
		}
		citationsJSON.WriteString(fmt.Sprintf(`{"chunk_id":"%s","reason":"relevant"}`, chunkIDs[i].String()))
	}
	citationsJSON.WriteString("]")

	input := fmt.Sprintf(`{"answer": "Answer with citations [1][2][3].", "citations": %s, "fallback": false, "reason": ""}`, citationsJSON.String())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = validator.Validate(input, contexts)
	}
}

func BenchmarkOutputValidator_RepairJSON(b *testing.B) {
	validator := usecase.NewOutputValidator()
	// Truncated JSON that needs repair
	input := `{"answer": "This is a truncated answer about technology`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = validator.Validate(input, nil)
	}
}
