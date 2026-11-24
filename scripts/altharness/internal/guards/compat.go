package guards

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func pathInside(path, root string) bool {
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func pathWithin(path, root string) bool { return pathInside(path, root) }

func boolVal(v any, fallback bool) bool {
	b, ok := v.(bool)
	if !ok {
		return fallback
	}
	return b
}

// strictStringList parses a list of strings, rejecting non-list values and any
// non-string element (failing closed rather than silently dropping bad entries).
func strictStringList(v any, allowNil, requireNonEmpty, requireNonBlank bool) ([]string, bool) {
	if v == nil {
		if allowNil && !requireNonEmpty {
			return []string{}, true
		}
		return nil, false
	}
	var out []string
	switch items := v.(type) {
	case []any:
		out = make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			if requireNonBlank && strings.TrimSpace(s) == "" {
				return nil, false
			}
			out = append(out, s)
		}
	case []string:
		out = make([]string, 0, len(items))
		for _, s := range items {
			if requireNonBlank && strings.TrimSpace(s) == "" {
				return nil, false
			}
			out = append(out, s)
		}
	default:
		return nil, false
	}
	if requireNonEmpty && len(out) == 0 {
		return nil, false
	}
	return out, true
}

// RunPacketScope is the EXEC-02 guard entry point.
func RunPacketScope(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return RunScope(ctx, args, stdin, stdout, stderr)
}

// CheckTargetPath reports why an edit is outside packet scope.
func CheckTargetPath(ctx context.Context, rawPath, repoRoot string, packet map[string]any, cwd string) string {
	_ = ctx
	return checkScopePath(rawPath, repoRoot, packet, cwd)
}
