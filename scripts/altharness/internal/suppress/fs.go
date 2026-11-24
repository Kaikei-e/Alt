package suppress

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func resolvePath(p string) string {
	if p == "" {
		p = "."
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	abs = filepath.Clean(abs)
	cur := abs
	var rest []string
	for {
		if _, err := os.Lstat(cur); err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				resolved = cur
			}
			for i := len(rest) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, rest[i])
			}
			return resolved
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		rest = append(rest, filepath.Base(cur))
		cur = parent
	}
}

func relTo(path, root string) (string, bool) {
	rp := resolvePath(path)
	rr := resolvePath(root)
	if rp == rr {
		return ".", true
	}
	rel, err := filepath.Rel(rr, rp)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func toSlash(p string) string { return filepath.ToSlash(p) }

func joinPath(elem ...string) string { return filepath.Join(elem...) }

func isRegularFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func isSymlink(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func readFileStrict(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", &utf8Error{path: path}
	}
	return string(b), nil
}

type utf8Error struct{ path string }

func (e *utf8Error) Error() string {
	return "utf-8 decode error reading " + e.path
}

func readFileReplace(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes.ToValidUTF8(b, []byte("\uFFFD"))), nil
}
