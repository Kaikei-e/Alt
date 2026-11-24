package packet

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	rustCharLitRe  = regexp.MustCompile(`^'(?:\\(?:x[0-9A-Fa-f]{2}|u\{[0-9A-Fa-f_]+\}|.)|[^'\\])'`)
	rustFnDeclRe   = regexp.MustCompile(`\bfn\s+([A-Za-z_][A-Za-z0-9_]*)`)
	goFuncDeclRe   = regexp.MustCompile(`\bfunc\s+(?:\(([^)]*)\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*(?:\[[^\]]*\])?\s*\(`)
	tsFuncDeclRe   = regexp.MustCompile(`\b(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*\(`)
	tsArrowDeclRe  = regexp.MustCompile(`\b(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=\n]+)?=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=\n]+)?=>`)
	tsClassDeclRe  = regexp.MustCompile(`\b(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)[^{]*\{`)
	tsMethodDeclRe = regexp.MustCompile(`(?:^|[{;,\n])\s*(?:(?:public|private|protected|static|readonly|override|async|get|set)\s+)*([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>\n]*>)?\s*\([^)]*\)\s*(?::[^{;=\n]+)?\{`)
	rustImplDeclRe = regexp.MustCompile(`\b(?:unsafe\s+)?impl(?:\s*<[^>]*>)?\s+([^{;]+?)\s*\{`)
	rustPreFnRe    = regexp.MustCompile(`(?:(?:pub(?:\s*\([^)]*\))?|default|const|async|unsafe|extern(?:\s+"[^"]*")?)\s+)+$`)
	svelteScriptRe = regexp.MustCompile(`(?is)<script\b[^>]*>(.*?)</script>`)
)

var tsControlKeywords = map[string]struct{}{
	"if": {}, "for": {}, "while": {}, "switch": {}, "catch": {}, "with": {},
	"return": {}, "function": {}, "class": {}, "else": {}, "try": {},
}

var extToLang = map[string]string{
	".go": "go", ".py": "python", ".rs": "rust",
	".ts": "typescript", ".tsx": "typescript", ".mts": "typescript", ".cts": "typescript",
	".js": "typescript", ".jsx": "typescript", ".mjs": "typescript", ".cjs": "typescript",
	".svelte": "typescript",
}

func inferLanguage(path string) string {
	ext := strings.ToLower(filepathExt(path))
	return extToLang[ext]
}

func filepathExt(path string) string {
	i := strings.LastIndex(path, "/")
	base := path
	if i >= 0 {
		base = path[i+1:]
	}
	j := strings.LastIndex(base, ".")
	if j <= 0 {
		return ""
	}
	return base[j:]
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b >= 0x80
}

func scanRustRawStringEnd(src string, start int) (int, bool) {
	if start > 0 && isIdentByte(src[start-1]) {
		return 0, false
	}
	if start >= len(src) {
		return 0, false
	}
	ch := src[start]
	rIdx := start
	switch ch {
	case 'r':
		rIdx = start
	case 'b', 'c':
		if start+1 < len(src) && src[start+1] == 'r' {
			rIdx = start + 1
		} else {
			return 0, false
		}
	default:
		return 0, false
	}
	j := rIdx + 1
	for j < len(src) && src[j] == '#' {
		j++
	}
	if j >= len(src) || src[j] != '"' {
		return 0, false
	}
	hashes := j - (rIdx + 1)
	endDelim := "\"" + strings.Repeat("#", hashes)
	rest := src[j+1:]
	pos := strings.Index(rest, endDelim)
	if pos < 0 {
		return len(src), true
	}
	return j + 1 + pos + len(endDelim), true
}

func scanEscapedQuoteEnd(src string, start int, quote byte) int {
	idx := start + 1
	for idx < len(src) {
		c := src[idx]
		if c == '\\' {
			idx += 2
			continue
		}
		if c == quote {
			return idx + 1
		}
		idx++
	}
	return len(src)
}

func scanStringLiteralEnd(src string, start int, language string) (int, bool) {
	if start < 0 || start >= len(src) {
		return 0, false
	}
	ch := src[start]
	if language == "rust" {
		if ch == 'r' || ((ch == 'b' || ch == 'c') && start+1 < len(src) && src[start+1] == 'r') {
			return scanRustRawStringEnd(src, start)
		}
		if ch == 'b' && start+1 < len(src) && src[start+1] == '\'' {
			if start > 0 && isIdentByte(src[start-1]) {
				return 0, false
			}
			sliceEnd := start + 1 + 16
			if sliceEnd > len(src) {
				sliceEnd = len(src)
			}
			m := rustCharLitRe.FindStringIndex(src[start+1 : sliceEnd])
			if m == nil {
				return 0, false
			}
			return start + 1 + m[1], true
		}
		if ch == '\'' {
			sliceEnd := start + 16
			if sliceEnd > len(src) {
				sliceEnd = len(src)
			}
			m := rustCharLitRe.FindStringIndex(src[start:sliceEnd])
			if m == nil {
				return 0, false
			}
			return start + m[1], true
		}
		if ch == '"' {
			return scanEscapedQuoteEnd(src, start, ch), true
		}
		return 0, false
	}
	if language == "go" && ch == '`' {
		end := strings.IndexByte(src[start+1:], '`')
		if end < 0 {
			return len(src), true
		}
		return start + 1 + end + 1, true
	}
	if language == "go" {
		if ch == '"' || ch == '\'' {
			return scanEscapedQuoteEnd(src, start, ch), true
		}
		return 0, false
	}
	if ch == '"' || ch == '\'' || ch == '`' {
		return scanEscapedQuoteEnd(src, start, ch), true
	}
	return 0, false
}

func scanBlockCommentEnd(src string, start int, nested bool) int {
	depth := 1
	idx := start + 2
	for idx < len(src) && depth > 0 {
		if nested && idx+1 < len(src) && src[idx] == '/' && src[idx+1] == '*' {
			depth++
			idx += 2
			continue
		}
		if idx+1 < len(src) && src[idx] == '*' && src[idx+1] == '/' {
			depth--
			idx += 2
			continue
		}
		idx++
	}
	return idx
}

func maskSpanPreserveNewlines(seg string) string {
	out := []byte(seg)
	for i, c := range out {
		if c != '\n' {
			out[i] = ' '
		}
	}
	return string(out)
}

func maskCommentsAndStrings(source, language string) (string, string) {
	var masked, comments strings.Builder
	n := len(source)
	i := 0
	nested := language == "rust"
	for i < n {
		if end, ok := scanStringLiteralEnd(source, i, language); ok {
			seg := source[i:end]
			masked.WriteString(maskSpanPreserveNewlines(seg))
			comments.WriteString(seg)
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
			blank := strings.Repeat(" ", end-i)
			masked.WriteString(blank)
			comments.WriteString(blank)
			i = end
			continue
		}
		if ch == '/' && nxt == '*' {
			end := scanBlockCommentEnd(source, i, nested)
			blank := maskSpanPreserveNewlines(source[i:end])
			masked.WriteString(blank)
			comments.WriteString(blank)
			i = end
			continue
		}
		masked.WriteByte(ch)
		comments.WriteByte(ch)
		i++
	}
	return masked.String(), comments.String()
}

func findMaskedBraceClose(masked string, openIdx int) int {
	depth := 1
	idx := openIdx + 1
	for idx < len(masked) && depth > 0 {
		switch masked[idx] {
		case '{':
			depth++
		case '}':
			depth--
		}
		idx++
	}
	return idx
}

func findMaskedParenClose(masked string, openIdx int) int {
	depth := 1
	idx := openIdx + 1
	for idx < len(masked) && depth > 0 {
		switch masked[idx] {
		case '(':
			depth++
		case ')':
			depth--
		}
		idx++
	}
	return idx
}

func findBodyBraceAfterParams(masked string, openParen int, language string) (int, bool) {
	pos := findMaskedParenClose(masked, openParen)
	parenDepth := 0
	for pos < len(masked) {
		ch := masked[pos]
		switch ch {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		default:
			if parenDepth == 0 {
				if ch == '{' {
					return pos, true
				}
				if ch == ';' && language != "go" {
					return 0, false
				}
				if ch == '\n' && language == "go" && pos+1 < len(masked) {
					nxt := strings.TrimLeft(masked[pos+1:], " \t")
					for _, prefix := range []string{"func ", "type ", "var ", "const ", "package ", "import "} {
						if strings.HasPrefix(nxt, prefix) {
							return 0, false
						}
					}
				}
			}
		}
		pos++
	}
	return 0, false
}

func findRustFnBodyOpen(src string, start int) (int, bool) {
	parenDepth := 0
	bracketDepth := 0
	for idx := start; idx < len(src); idx++ {
		switch src[idx] {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			if parenDepth == 0 && bracketDepth == 0 {
				return idx, true
			}
		case ';':
			if parenDepth == 0 && bracketDepth == 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

// hashNormalizedLang collapses whitespace outside string literals while
// preserving significant whitespace and newlines inside string literals.
func hashNormalizedLang(text, language string) string {
	var b strings.Builder
	b.Grow(len(text))
	n := len(text)
	i := 0
	needSpace := false
	for i < n {
		if end, ok := scanStringLiteralEnd(text, i, language); ok {
			if needSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(text[i:end])
			needSpace = false
			i = end
			continue
		}
		ch := text[i]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\v' {
			needSpace = true
			i++
			continue
		}
		if needSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte(ch)
		needSpace = false
		i++
	}
	return commandSHA256(b.String())
}

func isUnicodeAlnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
