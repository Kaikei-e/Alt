package lang

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// repoRoot is the repository that contains scripts/lint, independent of the
// process working directory when the test binary or a caller lives elsewhere.
func repoRoot() (string, error) {
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		dir := start
		for {
			marker := filepath.Join(dir, "scripts", "lint", "go.sh")
			if st, err := os.Stat(marker); err == nil && !st.IsDir() {
				if resolved, err := filepath.EvalSymlinks(dir); err == nil {
					return resolved, nil
				}
				abs, err := filepath.Abs(dir)
				if err != nil {
					return dir, nil
				}
				return abs, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", os.ErrNotExist
}

func mustRepo(stderr io.Writer) (string, int) {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(stderr, "error: repository root not found")
		return "", 1
	}
	return root, 0
}
