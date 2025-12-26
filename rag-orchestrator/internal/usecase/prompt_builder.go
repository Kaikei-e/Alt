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

	var selectedInstructions []string
	if input.Stage == "citations" {
		selectedInstructions = []string{
			"Analyze the <context> and identify key facts relevant to the user query.",
			"Extract exact quotes and build citations.",
			"Return ONLY the 'quotes' and 'citations' fields in JSON.",
			"Leave 'answer' empty or null.",
			"Ensure citations point to specific <chunk_id>.",
		}
	} else if input.Stage == "answer" {
		selectedInstructions = []string{
			"Answer the query using the facts in <context>.",
			"You may also refer to the provided <citations> from previous step (if any) but prioritize <context>.",
			"Your 'answer' field MUST be a Markdown string.",
			"Target length: 300-500 words.",
			"Cite each sentence with [chunk_id].",
			"Return 'answer', 'fallback', and 'reason'.",
			"Do not return 'quotes' or 'citations' arrays again (keep them empty) as they are already known.",
		}
	} else {
		// Combined / Default
		selectedInstructions = []string{
			"Answer using the facts in <context> provided in the user message.",
			"Your \"answer\" field MUST be a Markdown string following the strict template below.",
			"Template:",
			"  ## Introduction",
			"  [Brief overview of the topic]",
			"",
			"  ## Details",
			"  - **Point 1**: [Description with citations]",
			"  - **Point 2**: [Description with citations]",
			"",
			"  ## Conclusion",
			"  [Summary of key findings]",
			"",
			"Include background context and future outlook/implications if available.",
			"Target a length of at least 300-500 words relative to the language.",
			"Cite each sentence with [chunk_id] referenced in the context.",
			"Translate English context facts into natural Japanese if the query is in Japanese.",
			"If you cannot answer with the available evidence, return {\"answer\":null,\"fallback\":true,\"reason\":\"insufficient_evidence\"}.",
			"Do not invent facts or assume information that is not in the context.",
		}
	}

	for _, inst := range append(selectedInstructions, b.additionalInstructions...) {
		sysSb.WriteString("  <line>")
		sysSb.WriteString(escape(inst))
		sysSb.WriteString("</line>\n")
	}
	sysSb.WriteString("</instructions>\n\n")

	sysSb.WriteString("<format>\n")
	sysSb.WriteString("JSON: {\n")
	sysSb.WriteString("  \"quotes\": [{\"chunk_id\":\"...\",\"quote\":\"...\"}],\n")
	sysSb.WriteString("  \"answer\":\"...\",\n")
	sysSb.WriteString("  \"citations\":[{\"chunk_id\":\"...\",\"url\":\"...\",\"title\":\"...\",\"score\":...}],\n")
	sysSb.WriteString("  \"fallback\":false,\n")
	sysSb.WriteString("  \"reason\":\"\"\n")
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

	if len(input.Citations) > 0 {
		userSb.WriteString("<citations>\n")
		for _, c := range input.Citations {
			userSb.WriteString("  <item>")
			userSb.WriteString(escape(c))
			userSb.WriteString("</item>\n")
		}
		userSb.WriteString("</citations>\n\n")
	}

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
