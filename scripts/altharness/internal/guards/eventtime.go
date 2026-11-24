package guards

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

// GuardedGlobs are the projector trees that must stay reproject-safe.
var GuardedGlobs = []string{
	"knowledge-sovereign/app/usecase/knowledge_home_projector/*.go",
	"knowledge-sovereign/app/usecase/knowledge_trail_projector/*.go",
}

var (
	wallClockRE = regexp.MustCompile(`\btime\.(?:Now|Since|Until)\s*\(`)
	getLatestRE = regexp.MustCompile(`\bGetLatest[A-Z][A-Za-z0-9_]*`)
)

// RunEventTime blocks wall-clock reads and GetLatest queries in projector edits.
func RunEventTime(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
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
	var violations []string
	for _, path := range payload.Paths {
		if !IsGuardedPathInCwd(path, payload.Cwd) {
			continue
		}
		if call := FindEventTimeViolation(payload.AddedText[path]); call != "" {
			violations = append(violations, fmt.Sprintf("Blocked edit to %s: found '%s' in projector code. Projectors must be reproject-safe: derive time from the event's OccurredAt and read the version id carried in the event payload (see ADR-000468 and ADR-000447).", path, call))
		}
	}
	if len(violations) > 0 {
		fmt.Fprintln(stderr, strings.Join(violations, "\n"))
		return 2
	}
	return 0
}

// IsGuardedPathInCwd checks rawPath and its cwd/symlink-resolved form.
func IsGuardedPathInCwd(rawPath, cwd string) bool {
	if IsGuardedPath(rawPath) {
		return true
	}
	base := strings.TrimSpace(cwd)
	var candidate string
	if filepath.IsAbs(rawPath) {
		candidate = rawPath
	} else if base != "" {
		candidate = filepath.Join(base, rawPath)
	}
	if candidate != "" {
		if IsGuardedPath(candidate) {
			return true
		}
		if resolved, err := resolveNonStrict(candidate); err == nil && resolved != "" && IsGuardedPath(resolved) {
			return true
		}
	}
	return false
}

// IsGuardedPath reports whether rawPath is a non-test projector source file.
func IsGuardedPath(rawPath string) bool {
	norm := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(strings.ReplaceAll(rawPath, `\`, "/"))), "./")
	if norm == "" || strings.HasSuffix(norm, "_test.go") {
		return false
	}
	parts := strings.Split(norm, "/")
	var cleaned []string
	for _, part := range parts {
		if part != "" && part != "/" {
			cleaned = append(cleaned, part)
		}
	}
	regexes := make([]*regexp.Regexp, len(GuardedGlobs))
	for i, pat := range GuardedGlobs {
		regexes[i] = globRegex(pat)
	}
	for idx := range cleaned {
		suffix := strings.Join(cleaned[idx:], "/")
		for _, rx := range regexes {
			if rx.MatchString(suffix) {
				return true
			}
		}
	}
	return false
}

// StripGoLineComments removes // comments that are outside double-quoted strings.
func StripGoLineComments(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		inDouble := false
		escaped := false
		cut := -1
		for i := 0; i < len(line); i++ {
			ch := line[i]
			if inDouble {
				if escaped {
					escaped = false
				} else if ch == '\\' {
					escaped = true
				} else if ch == '"' {
					inDouble = false
				}
				continue
			}
			if ch == '"' {
				inDouble = true
				continue
			}
			if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
				cut = i
				break
			}
		}
		if cut >= 0 {
			line = line[:cut]
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// FindEventTimeViolation returns the offending call token, or "".
func FindEventTimeViolation(added string) string {
	code := StripGoLineComments(added)
	if m := wallClockRE.FindString(code); m != "" {
		return strings.TrimRight(m, " \t")
	}
	if m := getLatestRE.FindString(code); m != "" {
		return m
	}
	return ""
}
