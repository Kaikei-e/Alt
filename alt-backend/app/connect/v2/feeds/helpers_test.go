package feeds

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeDescription_HTMLEntityDecoding(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"decodes &#39;", "Here&#39;s the news", "Here's the news"},
		{"decodes &amp;", "A &amp; B", "A & B"},
		{"decodes &quot;", "&quot;Hello&quot;", `"Hello"`},
		{"decodes &#x27;", "It&#x27;s fine", "It's fine"},
		{"strips tags and decodes", "<b>It&#39;s</b> &amp; more", "It's & more"},
		{"decodes multiple entities in title", "Here&#39;s what we&#39;re reading", "Here's what we're reading"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeDescription(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
