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
	sysSb.WriteString("<instructions>\n")
	sysSb.WriteString("  <locale>")
	sysSb.WriteString(escape(input.Locale))
	sysSb.WriteString("</locale>\n")

	// Single Phase Instructions
	selectedInstructions := []string{
		"You are an AI assistant that answers questions based ONLY on the provided <context>.",
		"1. Analyze the <context> documents carefully.",
		"2. Answer the <query> using strictly the facts from the <context>.",
		"3. IMPORTANT: Only set \"fallback\": true if there is absolutely NO relevant information in the context. If there is ANY relevant information, you MUST provide an answer, even if partial.",
		"4. Your \"answer\" field MUST be a Markdown string with the following structure:",
		"   ## Overview",
		"   [Brief introduction to the topic]",
		"",
		"   ## Key Points",
		"   - **Point 1**: [Description with citation] [chunk_id]",
		"   - **Point 2**: [Description with citation] [chunk_id]",
		"",
		"   ## Summary",
		"   [Conclusion with key takeaways]",
		"",
		"5. Target length: 200-500 words depending on available context.",
		"6. You MUST include citations for your statements using the metadata from the context.",
		"   - The \"citations\" array in your JSON output must list every chunk_id used in your answer.",
		"   - In the text of your answer, refer to the source by appending [chunk_id] at the end of sentences.",
		"7. Do not include external knowledge or hallucinate facts.",
		"8. If the query is in Japanese, translate English facts from the context into natural Japanese.",
		"9. Follow the JSON format specified below EXACTLY.",
	}

	for _, inst := range append(selectedInstructions, b.additionalInstructions...) {
		sysSb.WriteString("  <line>")
		sysSb.WriteString(escape(inst))
		sysSb.WriteString("</line>\n")
	}
	sysSb.WriteString("</instructions>\n\n")

	sysSb.WriteString("<format>\n")
	sysSb.WriteString("JSON: {\n")
	sysSb.WriteString("  \"answer\": \"Markdown text value... [chunk_id]\",\n")
	sysSb.WriteString("  \"citations\": [{\"chunk_id\":\"...\", \"reason\":\"optional reason\"}],\n")
	sysSb.WriteString("  \"fallback\": false,  // Set true ONLY if no relevant context exists\n")
	sysSb.WriteString("  \"reason\": \"\"  // Explain why fallback is true, if applicable\n")
	sysSb.WriteString("}\n")
	sysSb.WriteString("</format>\n")

	// 2. Build User Message (Context + Query)
	var userSb strings.Builder
	userSb.WriteString(fmt.Sprintf("<context version=\"%s\">\n", escape(input.PromptVersion)))
	for _, ctx := range input.Contexts {
		userSb.WriteString("  <document>\n")
		userSb.WriteString("    <chunk_id>")
		userSb.WriteString(escape(ctx.ChunkID))
		userSb.WriteString("</chunk_id>\n")
		userSb.WriteString("    <title>")
		userSb.WriteString(escape(ctx.Title))
		userSb.WriteString("</title>\n")
		userSb.WriteString("    <url>")
		userSb.WriteString(escape(ctx.URL))
		userSb.WriteString("</url>\n")
		userSb.WriteString("    <published_at>")
		userSb.WriteString(escape(ctx.PublishedAt))
		userSb.WriteString("</published_at>\n")
		userSb.WriteString("    <score>")
		userSb.WriteString(fmt.Sprintf("%.6f", ctx.Score))
		userSb.WriteString("</score>\n")
		userSb.WriteString("    <document_version>")
		userSb.WriteString(fmt.Sprintf("%d", ctx.DocumentVersion))
		userSb.WriteString("</document_version>\n")
		userSb.WriteString("    <chunk_text>")
		userSb.WriteString(escape(ctx.ChunkText))
		userSb.WriteString("</chunk_text>\n")
		userSb.WriteString("  </document>\n")
	}
	userSb.WriteString("</context>\n\n")

	userSb.WriteString("<query>\n")
	userSb.WriteString(escape(input.Query))
	userSb.WriteString("\n</query>\n")

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
