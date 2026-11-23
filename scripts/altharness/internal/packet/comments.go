package packet

import (
	"strings"
)

var cStyleCommentExts = map[string]struct{}{
	".go": {}, ".rs": {}, ".ts": {}, ".tsx": {}, ".mts": {}, ".cts": {},
	".js": {}, ".jsx": {}, ".mjs": {}, ".cjs": {}, ".svelte": {},
}

var shellCommentExts = map[string]struct{}{
	".sh": {}, ".bash": {}, ".zsh": {},
}

func addCount(c map[string]int, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c[text]++
}

func extractPythonComments(source string) map[string]int {
	counts := map[string]int{}
	if strings.TrimSpace(source) == "" {
		return counts
	}
	var quote byte
	triple := false
	escaped := false
	lineStart := 0
	for i := 0; i < len(source); i++ {
		ch := source[i]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if ch == '\\' && !triple {
				escaped = true
				continue
			}
			if triple && i+2 < len(source) && source[i] == quote && source[i+1] == quote && source[i+2] == quote {
				quote = 0
				triple = false
				i += 2
				continue
			}
			if !triple && ch == quote {
				quote = 0
			}
			if ch == '\n' {
				lineStart = i + 1
			}
			continue
		}
		if ch == '#' {
			nl := strings.IndexByte(source[i:], '\n')
			end := len(source)
			if nl >= 0 {
				end = i + nl
			}
			addCount(counts, source[i:end])
			i = end - 1
			continue
		}
		if (ch == '"' || ch == '\'') && i+2 < len(source) && source[i+1] == ch && source[i+2] == ch {
			quote = ch
			triple = true
			i += 2
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			triple = false
			continue
		}
		if ch == '\n' {
			lineStart = i + 1
		}
		_ = lineStart
	}
	return counts
}

func extractCStyleComments(source, language string) map[string]int {
	counts := map[string]int{}
	if strings.TrimSpace(source) == "" {
		return counts
	}
	nested := language == "rust"
	i := 0
	n := len(source)
	for i < n {
		if end, ok := scanStringLiteralEnd(source, i, language); ok {
			i = end
			continue
		}
		ch := source[i]
		var nxt byte
		if i+1 < n {
			nxt = source[i+1]
		}
		if ch == '/' && nxt == '/' {
			nl := strings.IndexByte(source[i+2:], '\n')
			end := n
			if nl >= 0 {
				end = i + 2 + nl
			}
			addCount(counts, source[i:end])
			i = end
			continue
		}
		if ch == '/' && nxt == '*' {
			end := scanBlockCommentEnd(source, i, nested)
			addCount(counts, source[i:end])
			i = end
			continue
		}
		i++
	}
	return counts
}

func extractShellComments(source string) map[string]int {
	counts := map[string]int{}
	for _, line := range splitLines(source) {
		var quote byte
		escaped := false
		for i := 0; i < len(line); i++ {
			ch := line[i]
			if escaped {
				escaped = false
				continue
			}
			if quote == '"' && ch == '\\' {
				escaped = true
				continue
			}
			if quote != 0 {
				if ch == quote {
					quote = 0
				}
				continue
			}
			if ch == '"' || ch == '\'' {
				quote = ch
				continue
			}
			if ch == '#' && (i == 0 || isASCIISpace(line[i-1])) {
				addCount(counts, line[i:])
				break
			}
		}
	}
	return counts
}

func extractCodeComments(rel, source string) map[string]int {
	suffix := strings.ToLower(filepathExt(rel))
	switch {
	case suffix == ".py":
		return extractPythonComments(source)
	case suffix == ".svelte":
		return extractCStyleComments(extractSvelteScripts(source), "typescript")
	default:
		if _, ok := cStyleCommentExts[suffix]; ok {
			lang := "typescript"
			if suffix == ".go" {
				lang = "go"
			} else if suffix == ".rs" {
				lang = "rust"
			}
			return extractCStyleComments(source, lang)
		}
		if _, ok := shellCommentExts[suffix]; ok {
			return extractShellComments(source)
		}
	}
	return map[string]int{}
}

func subtractCounts(after, before map[string]int) []string {
	var added []string
	for text, n := range after {
		if n > before[text] {
			added = append(added, text)
		}
	}
	return added
}
