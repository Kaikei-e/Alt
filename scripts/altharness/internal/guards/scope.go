package guards

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	historicalK8s = "stopped-using-k8s"
)

var excludedGeneratedPrefixes = []string{"stopped-using-k8s/", "scripts/harness/corpus/"}

// RunScope is the EXEC-02 packet-scope and generated-file guard.
func RunScope(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = ctx
	_ = args
	_ = stdout
	if stderr == nil {
		stderr = io.Discard
	}
	payload, code := readEditPayload(stdin, stderr)
	if code >= 0 {
		return code
	}
	repo := ResolveRepoRoot("", payload.Cwd)
	packet, pktErr := LoadActivePacket(repo, "")
	if pktErr != "" {
		fmt.Fprintf(stderr, "BLOCKED [EXEC-02]: %s.\n", pktErr)
		return 2
	}
	var violations []string
	for _, path := range payload.Paths {
		if err := checkScopePath(path, repo, packet, payload.Cwd); err != "" {
			violations = append(violations, err)
		}
	}
	if len(violations) > 0 {
		fmt.Fprintln(stderr, strings.Join(violations, "\n"))
		return 2
	}
	return 0
}

func readEditPayload(stdin io.Reader, stderr io.Writer) (HookPayload, int) {
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "guard could not read the hook payload: %v\n", err)
		return HookPayload{}, 2
	}
	payload, ok, err := ParseHookStdin(string(raw))
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return HookPayload{}, 2
	}
	if !ok {
		return HookPayload{}, 0
	}
	return payload, -1
}

func checkScopePath(rawPath, repoRoot string, packet map[string]any, cwd string) string {
	rel, resolveErr := resolveRepoRel(rawPath, repoRoot, cwd)
	if resolveErr != "" {
		return resolveErr
	}
	if rel == "" {
		if packet != nil {
			id := packetID(packet)
			return fmt.Sprintf("BLOCKED [EXEC-02]: '%s' is outside repository root and outside allowed_paths in active packet '%s'.", rawPath, id)
		}
		return ""
	}
	return checkInsideRepo(rel, repoRoot, packet)
}

func packetID(packet map[string]any) string {
	id, _ := packet["packet_id"].(string)
	if id == "" {
		return "unknown"
	}
	return id
}

func resolveRepoRel(rawPath, repoRoot, cwd string) (string, string) {
	if strings.Contains(rawPath, "\x00") || strings.TrimSpace(rawPath) == "" {
		return "", fmt.Sprintf("BLOCKED [EXEC-02]: invalid target path: %#v", rawPath)
	}
	repoResolved := resolveExisting(repoRoot)
	if repoResolved == "" {
		return "", fmt.Sprintf("BLOCKED [EXEC-02]: could not resolve target path '%s' (empty repoRoot)", rawPath)
	}
	repoAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		repoAbs = filepath.Clean(repoRoot)
	}
	base := relativeBase(repoResolved, cwd)
	var candidate string
	if filepath.IsAbs(rawPath) {
		candidate = rawPath
	} else {
		candidate = filepath.Join(base, rawPath)
	}
	lexical := filepath.Clean(candidate)
	resolved, err := resolveNonStrict(candidate)
	if err != nil {
		return "", fmt.Sprintf("BLOCKED [EXEC-02]: could not resolve target path '%s' (%v)", rawPath, err)
	}
	lexicalInside := pathInside(lexical, repoResolved) || pathInside(lexical, repoAbs) || pathInside(lexical, filepath.Clean(repoRoot))
	resolvedInside := pathInside(resolved, repoResolved)
	if lexicalInside && !resolvedInside {
		return "", fmt.Sprintf("BLOCKED [EXEC-02]: symlink escapes repository root: '%s' -> '%s'", rawPath, resolved)
	}
	if !resolvedInside {
		return "", ""
	}
	rel, err := filepath.Rel(repoResolved, resolved)
	if err != nil {
		return "", fmt.Sprintf("BLOCKED [EXEC-02]: could not resolve target path '%s' (%v)", rawPath, err)
	}
	return filepath.ToSlash(rel), ""
}

func relativeBase(repoResolved, cwd string) string {
	if strings.TrimSpace(cwd) != "" {
		return cwd
	}
	if proc, err := os.Getwd(); err == nil {
		resolvedProc := resolveExisting(proc)
		if resolvedProc != "" && pathInside(resolvedProc, repoResolved) {
			return resolvedProc
		}
	}
	return repoResolved
}

func resolveNonStrict(path string) (string, error) {
	return resolveNonStrictDepth(path, 0)
}

func resolveNonStrictDepth(path string, depth int) (string, error) {
	if depth > 40 {
		return "", fmt.Errorf("too many symlink hops resolving %s", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	vol := filepath.VolumeName(abs)
	rest := strings.TrimPrefix(abs, vol)
	parts := strings.Split(rest, string(filepath.Separator))
	cur := vol + string(filepath.Separator)
	var pending []string
	for _, part := range parts {
		if part == "" {
			continue
		}
		if len(pending) == 0 {
			next := filepath.Join(cur, part)
			if st, err := os.Lstat(next); err == nil {
				if st.Mode()&os.ModeSymlink != 0 {
					target, err := filepath.EvalSymlinks(next)
					if err == nil {
						cur = target
						continue
					}
					if os.IsNotExist(err) {
						linkTarget, lerr := os.Readlink(next)
						if lerr != nil {
							return "", lerr
						}
						if !filepath.IsAbs(linkTarget) {
							linkTarget = filepath.Join(cur, linkTarget)
						}
						resolvedLink, rerr := resolveNonStrictDepth(linkTarget, depth+1)
						if rerr != nil {
							return "", rerr
						}
						cur = resolvedLink
						continue
					}
					return "", err
				}
				cur = next
				continue
			}
		}
		pending = append(pending, part)
	}
	if len(pending) > 0 {
		cur = filepath.Join(append([]string{cur}, pending...)...)
	}
	return filepath.Clean(cur), nil
}

func checkInsideRepo(rel, repoRoot string, packet map[string]any) string {
	allowGen := packet != nil && boolVal(packet["allow_generated_code_edits"], false)
	excluded := false
	for _, prefix := range excludedGeneratedPrefixes {
		if strings.HasPrefix(rel, prefix) {
			excluded = true
			break
		}
	}
	if !allowGen && !excluded && repoRelIsGenerated(repoRoot, rel) {
		return fmt.Sprintf("BLOCKED [EXEC-02]: generated DO NOT EDIT file '%s' cannot be edited directly. Regenerate it from its source schema/spec instead (or opt in via allow_generated_code_edits: true in an active packet).", rel)
	}
	if k8s := checkHistoricalK8s(rel, packet); k8s != "" {
		return k8s
	}
	if packet != nil {
		return checkPacketScope(rel, packet)
	}
	return ""
}

func checkHistoricalK8s(rel string, packet map[string]any) string {
	if rel != historicalK8s && !strings.HasPrefix(rel, historicalK8s+"/") {
		return ""
	}
	if packet != nil {
		if allowed, ok := packet["allowed_paths"].([]any); ok {
			for _, pat := range allowed {
				s, ok := pat.(string)
				if ok && strings.Contains(s, historicalK8s) && matchesGlob(s, rel) {
					return ""
				}
			}
		}
	}
	return fmt.Sprintf("BLOCKED [EXEC-02]: historical asset '%s' under stopped-using-k8s/ is read-only unless explicitly authorized in active packet allowed_paths.", rel)
}

func checkPacketScope(rel string, packet map[string]any) string {
	id := packetID(packet)
	if forbidden, ok := packet["forbidden_paths"].([]any); ok {
		for _, pat := range forbidden {
			if s, ok := pat.(string); ok && matchesGlob(s, rel) {
				return fmt.Sprintf("BLOCKED [EXEC-02]: '%s' matches forbidden_paths in active packet '%s'.", rel, id)
			}
		}
	}
	allowed, _ := packet["allowed_paths"].([]any)
	matched := false
	for _, pat := range allowed {
		if s, ok := pat.(string); ok && matchesGlob(s, rel) {
			matched = true
			break
		}
	}
	if len(allowed) == 0 || !matched {
		return fmt.Sprintf("BLOCKED [EXEC-02]: '%s' is outside allowed_paths in active packet '%s'.", rel, id)
	}
	return ""
}

func matchesGlob(pattern, rel string) bool {
	rel = strings.TrimPrefix(strings.TrimSpace(rel), "./")
	for _, alt := range expandBraces(pattern) {
		if globRegex(alt).MatchString(rel) {
			return true
		}
	}
	return false
}

func expandBraces(pattern string) []string {
	start := -1
	depth := 0
	for idx, ch := range pattern {
		if ch == '{' {
			if depth == 0 {
				start = idx
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start != -1 {
				prefix := pattern[:start]
				inner := pattern[start+1 : idx]
				suffix := pattern[idx+1:]
				var options []string
				var buf strings.Builder
				innerDepth := 0
				for _, c := range inner {
					switch c {
					case '{':
						innerDepth++
						buf.WriteRune(c)
					case '}':
						innerDepth--
						buf.WriteRune(c)
					case ',':
						if innerDepth == 0 {
							options = append(options, buf.String())
							buf.Reset()
						} else {
							buf.WriteRune(c)
						}
					default:
						buf.WriteRune(c)
					}
				}
				options = append(options, buf.String())
				var expanded []string
				for _, opt := range options {
					expanded = append(expanded, expandBraces(prefix+opt+suffix)...)
				}
				return expanded
			}
		}
	}
	return []string{pattern}
}

func globRegex(pattern string) *regexp.Regexp {
	pat := strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pat); {
		if strings.HasPrefix(pat[i:], "**/") {
			b.WriteString("(?:.+/)?")
			i += 3
			continue
		}
		if strings.HasPrefix(pat[i:], "/**") && i+3 == len(pat) {
			b.WriteString("/.+")
			i += 3
			continue
		}
		if strings.HasPrefix(pat[i:], "**") {
			b.WriteString(".*")
			i += 2
			continue
		}
		if pat[i] == '*' {
			b.WriteString("[^/]*")
			i++
			continue
		}
		if pat[i] == '?' {
			b.WriteString("[^/]")
			i++
			continue
		}
		b.WriteString(regexp.QuoteMeta(pat[i : i+1]))
		i++
	}
	b.WriteByte('$')
	return regexp.MustCompile(b.String())
}
