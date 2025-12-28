package usecase

import (
	"fmt"
	"rag-orchestrator/internal/domain"
	"strings"
)

// PromptContext transports the metadata needed when composing the generation prompt.
type PromptContext struct {
	ChunkID         string
	Title           string
	URL             string
	PublishedAt     string
	Score           float32
	DocumentVersion int
	ChunkText       string
}

// PromptInput contains the pieces that feed into the prompt builder.
type PromptInput struct {
	Query         string
	Locale        string
	PromptVersion string
	Contexts      []PromptContext
	Stage         string   // "citations" or "answer" (empty = combined)
	Citations     []string // For "answer" stage, pass previously extracted citations
}

// PromptBuilder builds the chat messages sent to the LLM.
type PromptBuilder interface {
	Build(input PromptInput) ([]domain.Message, error)
}

// XMLPromptBuilder creates structured prompts that separate context, instructions, query, and format.
type XMLPromptBuilder struct {
	additionalInstructions []string
}

// NewXMLPromptBuilder creates a prompt builder with optional extra instructions appended.
func NewXMLPromptBuilder(additionalInstructions ...string) PromptBuilder {
	return &XMLPromptBuilder{
		additionalInstructions: additionalInstructions,
	}
}

// Build renders the Messages for Chat API.
func (b *XMLPromptBuilder) Build(input PromptInput) ([]domain.Message, error) {
	if input.PromptVersion == "" {
		return nil, fmt.Errorf("prompt version is required")
	}

	// 1. Build System Message (Instructions + Format)
	var sysSb strings.Builder
	sysSb.WriteString("You are a helpful assistant.\n")
	sysSb.WriteString("Reasoning: medium\n") // Implicitly setting reasoning level
	sysSb.WriteString("Answer the User Query based ONLY on the Request Context.\n\n")

	sysSb.WriteString("### Instructions\n")
	sysSb.WriteString("1. Analyze the context documents provided below (identified by [index]).\n")
	sysSb.WriteString("2. Answer the <query> strictly using facts from the <context>.\n")
	sysSb.WriteString("3. Answer is the length around 300 ~ 800 words.\n")
	sysSb.WriteString("4. If the query is in Japanese, answer in natural Japanese.\n")
	sysSb.WriteString("5. Value the information in the documents regardless of their language. Translate facts if necessary.\n")
	sysSb.WriteString("6. You MUST cite the source even if it is in a different language.\n")
	sysSb.WriteString("7. Output MUST be valid JSON with keys: \"answer\", \"citations\", \"fallback\", \"reason\".\n")
	sysSb.WriteString("8. \"answer\": A Markdown string. Use [index] at the end of sentences to cite sources.\n")
	sysSb.WriteString("9. \"citations\": A list of { \"chunk_id\": \"index\", \"reason\": \"...\" } for every chunk used.\n")
	sysSb.WriteString("10. \"fallback\": Set to true ONLY if the context contains NO relevant information.\n")

	if len(b.additionalInstructions) > 0 {
		sysSb.WriteString("\n### Additional Rules\n")
		for _, inst := range b.additionalInstructions {
			sysSb.WriteString(fmt.Sprintf("- %s\n", inst))
		}
	}

	sysSb.WriteString("\n### Response Format\n")
	sysSb.WriteString("```json\n")
	sysSb.WriteString("{\n")
	sysSb.WriteString("  \"answer\": \"Markdown answer... [1] [2]\",\n")
	sysSb.WriteString("  \"citations\": [{\"chunk_id\": \"1\", \"reason\": \"Support point X\"}],\n")
	sysSb.WriteString("  \"fallback\": false,\n")
	sysSb.WriteString("  \"reason\": \"Explain why fallback is true or citation selection\"\n")
	sysSb.WriteString("}\n")
	sysSb.WriteString("```\n")

	// 2. Build User Message (Context + Query)
	var userSb strings.Builder
	userSb.WriteString("### Context\n")
	for i, ctx := range input.Contexts {
		// Dense format: [Index] Title (Date): Content
		// Using 1-based index
		index := i + 1
		userSb.WriteString(fmt.Sprintf("[%d] %s", index, ctx.Title))
		if ctx.PublishedAt != "" {
			userSb.WriteString(fmt.Sprintf(" (%s)", ctx.PublishedAt))
		}
		userSb.WriteString("\n")
		userSb.WriteString(ctx.ChunkText)
		userSb.WriteString("\n\n")
	}

	userSb.WriteString("### Query\n")
	userSb.WriteString(input.Query)
	// Add Locale hint if present
	if input.Locale != "" {
		userSb.WriteString(fmt.Sprintf("\n(Language: %s)", input.Locale))
	}

	return []domain.Message{
		{Role: "system", Content: sysSb.String()},
		{Role: "user", Content: userSb.String()},
	}, nil
}

func escape(value string) string {
	s := strings.TrimSpace(value)
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}
