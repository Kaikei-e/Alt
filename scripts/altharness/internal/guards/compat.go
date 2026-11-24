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

func stringList(v any) []string {
	switch items := v.(type) {
	case []any:
		var out []string
		for _, item := range items {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), items...)
	default:
		return nil
	}
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
