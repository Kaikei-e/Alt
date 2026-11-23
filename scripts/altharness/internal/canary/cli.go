package canary

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2

	outputSnippetLimit     = 400
	entrypointSnippetLimit = 800
	minExemptReasonLen     = 10
)

type streams struct {
	stdout io.Writer
	stderr io.Writer
}

func newStreams(stdout, stderr io.Writer) streams {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return streams{stdout: stdout, stderr: stderr}
}

func (s streams) info(message string) {
	fmt.Fprintln(s.stdout, message)
}

func (s streams) error(message string) {
	fmt.Fprintln(s.stderr, message)
}

func snippet(text string) string {
	compact := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(compact) <= outputSnippetLimit {
		return compact
	}
	return trimRunes(compact, outputSnippetLimit) + "..."
}

func snippetTail(text string) string {
	clean := stripANSI(text)
	if utf8.RuneCountInString(clean) <= entrypointSnippetLimit {
		return strings.TrimSpace(clean)
	}
	runes := []rune(clean)
	tail := strings.TrimSpace(string(runes[len(runes)-entrypointSnippetLimit:]))
	return "..." + tail
}

func trimRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func stripANSI(text string) string {
	return ansiRE.ReplaceAllString(text, "")
}

func resolvePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("empty path")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", p, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs, nil
	}
	return resolved, nil
}

func pyQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\', '\'':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func pyList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = pyQuote(item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

type parsedFlags struct {
	vals  map[string]string
	bools map[string]bool
}

func parseFlags(args []string, required, optional, boolFlags, boolValFlags []string) (parsedFlags, string) {
	stringOK := map[string]bool{}
	boolOK := map[string]bool{}
	boolValOK := map[string]bool{}
	for _, name := range required {
		stringOK[name] = true
	}
	for _, name := range optional {
		stringOK[name] = true
	}
	for _, name := range boolFlags {
		boolOK[name] = true
	}
	for _, name := range boolValFlags {
		boolValOK[name] = true
	}
	out := parsedFlags{vals: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			if i+1 < len(args) {
				return parsedFlags{}, "error: unrecognized arguments: " + strings.Join(args[i+1:], " ")
			}
			break
		}
		if a == "-h" || a == "--help" {
			return parsedFlags{}, "help"
		}
		if !strings.HasPrefix(a, "-") {
			return parsedFlags{}, "error: unrecognized arguments: " + strings.Join(args[i:], " ")
		}
		name := strings.TrimPrefix(a, "--")
		if name == a {
			return parsedFlags{}, "error: unrecognized arguments: " + a
		}
		val := ""
		hasVal := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			val = name[eq+1:]
			name = name[:eq]
			hasVal = true
		}
		switch {
		case boolOK[name]:
			if hasVal {
				return parsedFlags{}, fmt.Sprintf("error: argument --%s: ignored explicit argument %s", name, pyQuote(val))
			}
			out.bools[name] = true
		case boolValOK[name]:
			if !hasVal {
				if i+1 >= len(args) {
					return parsedFlags{}, fmt.Sprintf("error: argument --%s: expected one argument", name)
				}
				i++
				val = args[i]
			}
			parsed, err := parseBoolFlag(val)
			if err != nil {
				return parsedFlags{}, fmt.Sprintf("error: argument --%s: %s", name, err.Error())
			}
			if parsed {
				out.vals[name] = "true"
			} else {
				out.vals[name] = "false"
			}
		case stringOK[name]:
			if !hasVal {
				if i+1 >= len(args) {
					return parsedFlags{}, fmt.Sprintf("error: argument --%s: expected one argument", name)
				}
				i++
				val = args[i]
			}
			out.vals[name] = val
		default:
			return parsedFlags{}, "error: unrecognized arguments: " + a
		}
	}
	var missing []string
	for _, name := range required {
		if _, ok := out.vals[name]; !ok {
			missing = append(missing, "--"+name)
		}
	}
	if len(missing) > 0 {
		return parsedFlags{}, "error: the following arguments are required: " + strings.Join(missing, ", ")
	}
	return out, ""
}

func parseBoolFlag(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", value)
	}
}

func (p parsedFlags) string(name string) string {
	return p.vals[name]
}

func (p parsedFlags) boolVal(name string) bool {
	return p.vals[name] == "true"
}

func (p parsedFlags) optPath(name string) (string, error) {
	raw := p.vals[name]
	if raw == "" {
		return "", nil
	}
	return resolvePath(raw)
}

func (p parsedFlags) reqPath(name string) (string, error) {
	return resolvePath(p.vals[name])
}

func notePath(p string) {
	if p == "" {
		return
	}
	_, _ = filepath.Abs(p)
}
