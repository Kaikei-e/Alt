package usecase

import (
	"fmt"
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
}

// PromptBuilder builds the textual prompt sent to the LLM.
type PromptBuilder interface {
	Build(input PromptInput) (string, error)
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

// Build renders the XML/JSON prompt.
func (b *XMLPromptBuilder) Build(input PromptInput) (string, error) {
	if input.PromptVersion == "" {
		return "", fmt.Errorf("prompt version is required")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<context version=\"%s\">\n", escape(input.PromptVersion)))
	for _, ctx := range input.Contexts {
		sb.WriteString("  <document>\n")
		sb.WriteString("    <chunk_id>")
		sb.WriteString(escape(ctx.ChunkID))
		sb.WriteString("</chunk_id>\n")
		sb.WriteString("    <title>")
		sb.WriteString(escape(ctx.Title))
		sb.WriteString("</title>\n")
		sb.WriteString("    <url>")
		sb.WriteString(escape(ctx.URL))
		sb.WriteString("</url>\n")
		sb.WriteString("    <published_at>")
		sb.WriteString(escape(ctx.PublishedAt))
		sb.WriteString("</published_at>\n")
		sb.WriteString("    <score>")
		sb.WriteString(fmt.Sprintf("%.6f", ctx.Score))
		sb.WriteString("</score>\n")
		sb.WriteString("    <document_version>")
		sb.WriteString(fmt.Sprintf("%d", ctx.DocumentVersion))
		sb.WriteString("</document_version>\n")
		sb.WriteString("    <chunk_text>")
		sb.WriteString(escape(ctx.ChunkText))
		sb.WriteString("</chunk_text>\n")
		sb.WriteString("  </document>\n")
	}
	sb.WriteString("</context>\n\n")

	sb.WriteString("<instructions>\n")
	sb.WriteString("  <locale>")
	sb.WriteString(escape(input.Locale))
	sb.WriteString("</locale>\n")

	defaultInstructions := []string{
		"Answer using the facts in <context> above.",
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
	for _, inst := range append(defaultInstructions, b.additionalInstructions...) {
		sb.WriteString("  <line>")
		sb.WriteString(escape(inst))
		sb.WriteString("</line>\n")
	}
	sb.WriteString("</instructions>\n\n")

	sb.WriteString("<query>\n")
	sb.WriteString(escape(input.Query))
	sb.WriteString("\n</query>\n\n")

	sb.WriteString("<format>\n")
	sb.WriteString("JSON: {\n")
	sb.WriteString("  \"quotes\": [{\"chunk_id\":\"...\",\"quote\":\"...\"}],\n")
	sb.WriteString("  \"answer\":\"...\",\n")
	sb.WriteString("  \"citations\":[{\"chunk_id\":\"...\",\"url\":\"...\",\"title\":\"...\",\"score\":...}],\n")
	sb.WriteString("  \"fallback\":false,\n")
	sb.WriteString("  \"reason\":\"\"\n")
	sb.WriteString("}\n")
	sb.WriteString("</format>\n")

	return sb.String(), nil
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
