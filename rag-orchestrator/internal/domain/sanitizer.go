package domain

import (
	htmlstd "html"
	"strings"

	htmlpkg "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// boilerplateTriggers are heading/section text patterns that mark the start of
// non-article boilerplate (navigation, sidebar, footer). When any of these
// strings appear as the full text content of an h2/h3 element, all subsequent
// sibling content is discarded.
var boilerplateTriggers = []string{
	"注目ワード",
	"あわせて読みたい",
	"深掘りコンテンツ",
	"最新・注目の動画",
	"天気予報・防災情報",
	"新着ニュース",
	"各地のニュース",
}

// boilerplateSubstrings are substrings that, if found anywhere in a text node,
// indicate boilerplate content to be dropped.
var boilerplateSubstrings = []string{
	"受信契約",
}

// blockElements are HTML elements that should produce a line break when rendered.
var blockElements = map[atom.Atom]bool{
	atom.P:          true,
	atom.Div:        true,
	atom.H1:         true,
	atom.H2:         true,
	atom.H3:         true,
	atom.H4:         true,
	atom.H5:         true,
	atom.H6:         true,
	atom.Br:         true,
	atom.Li:         true,
	atom.Ul:         true,
	atom.Ol:         true,
	atom.Blockquote: true,
	atom.Section:    true,
	atom.Article:    true,
	atom.Header:     true,
	atom.Footer:     true,
	atom.Nav:        true,
	atom.Table:      true,
	atom.Tr:         true,
	atom.Td:         true,
	atom.Th:         true,
	atom.Pre:        true,
	atom.Hr:         true,
}

// droppedElements are elements whose entire subtree should be discarded.
var droppedElements = map[atom.Atom]bool{
	atom.Script:   true,
	atom.Style:    true,
	atom.Noscript: true,
	atom.Svg:      true,
	atom.Iframe:   true,
}

// SanitizeHTML extracts article body text from HTML.
// It uses golang.org/x/net/html to parse the DOM tree and:
//   - Strips all HTML tags, preserving inner text
//   - Keeps <a> link display text, drops href URLs
//   - Drops <script>, <style>, <noscript>, <svg>, <iframe> subtrees entirely
//   - Inserts line breaks at block element boundaries (p, div, h1-h6, li, etc.)
//   - Removes known boilerplate sections (navigation, sidebar, footer)
//   - Decodes HTML entities (&amp; → &)
//   - Normalizes consecutive blank lines
//
// If the input contains no HTML tags, it is returned unchanged.
func SanitizeHTML(body string) string {
	if body == "" {
		return ""
	}

	// Fast path: if no HTML tags detected, return as-is.
	if !strings.Contains(body, "<") {
		return body
	}

	nodes, err := htmlpkg.ParseFragment(strings.NewReader(body), &htmlpkg.Node{
		Type:     htmlpkg.ElementNode,
		DataAtom: atom.Body,
		Data:     "body",
	})
	if err != nil || len(nodes) == 0 {
		// Parse failure: strip entities and return.
		return htmlstd.UnescapeString(body)
	}

	var sb strings.Builder
	for _, n := range nodes {
		extractText(&sb, n, false)
	}

	result := sb.String()

	// Normalize whitespace: collapse 3+ consecutive newlines into 2.
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(result)
}

// extractText walks the DOM tree depth-first and appends visible text to sb.
// skipRemaining is set to true when a boilerplate trigger heading is encountered,
// causing all subsequent sibling content to be skipped.
func extractText(sb *strings.Builder, n *htmlpkg.Node, skipRemaining bool) bool {
	if n == nil {
		return skipRemaining
	}

	switch n.Type {
	case htmlpkg.TextNode:
		if skipRemaining {
			return true
		}
		text := n.Data
		// Check boilerplate substrings
		for _, bp := range boilerplateSubstrings {
			if strings.Contains(text, bp) {
				return skipRemaining
			}
		}
		sb.WriteString(text)
		return false

	case htmlpkg.ElementNode:
		if skipRemaining {
			return true
		}

		a := n.DataAtom

		// Drop entire subtree for script/style/etc.
		if droppedElements[a] {
			return false
		}

		// Drop <figure> subtrees (typically images with empty alt text)
		if a == atom.Figure {
			return false
		}

		// Check if this is a boilerplate trigger heading (h2/h3).
		if a == atom.H2 || a == atom.H3 {
			headingText := collectTextContent(n)
			trimmed := strings.TrimSpace(headingText)
			for _, trigger := range boilerplateTriggers {
				if trimmed == trigger {
					// Everything from here on is boilerplate.
					return true
				}
			}
		}

		// Block elements: insert newline before content.
		isBlock := blockElements[a]
		if isBlock {
			sb.WriteString("\n")
		}

		// Recurse into children.
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			skipRemaining = extractText(sb, c, skipRemaining)
			if skipRemaining {
				return true
			}
		}

		// Block elements: insert newline after content.
		if isBlock {
			sb.WriteString("\n")
		}

		return false

	case htmlpkg.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			skipRemaining = extractText(sb, c, skipRemaining)
			if skipRemaining {
				return true
			}
		}
		return skipRemaining
	}

	return skipRemaining
}

// collectTextContent returns all text content within a node, concatenated.
func collectTextContent(n *htmlpkg.Node) string {
	if n == nil {
		return ""
	}
	if n.Type == htmlpkg.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(collectTextContent(c))
	}
	return sb.String()
}
