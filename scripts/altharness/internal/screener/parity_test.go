package screener

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPythonScreenerParity(t *testing.T) {
	root := repoRoot(t)
	py := filepath.Join(root, "scripts", "lint", "behavior_screener.py")
	cases := []struct {
		name    string
		before  string
		after   string
		logical string
	}{
		{"C-01", "C-01/before.go", "C-01/after.go", "corpus/C-01/after.go"},
		{"C-02", "C-02/before.go", "C-02/after.go", "corpus/C-02/after.go"},
		{"C-03", "C-03/before.go", "C-03/after.go", "corpus/C-03/after.go"},
		{"C-06", "C-06/before.py", "C-06/after.py", "corpus/C-06/after.py"},
		{"C-07", "C-07/before.py", "C-07/after.py", "corpus/C-07/after.py"},
		{"C-10", "C-10/before.ts", "C-10/after.ts", "corpus/C-10/after.ts"},
		{"C-11", "C-11/before.go", "C-11/after.go", "corpus/C-11/after.go"},
		{"C-12", "C-12/before.go", "C-12/after.go", "corpus/C-12/after.go"},
		{"C-13", "C-13/before.go", "C-13/after.go", "corpus/C-13/after.go"},
		{"C-14", "C-14/before.go", "C-14/after.go", "corpus/C-14/after.go"},
		{"C-15", "C-15/before.go", "C-15/after.go", "corpus/C-15/after.go"},
		{"C-17", "C-17/before.rs", "C-17/after.rs", "corpus/C-17/after.rs"},
	}
	corpus := filepath.Join(root, "scripts", "harness", "corpus")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := filepath.Join(corpus, tc.before)
			after := filepath.Join(corpus, tc.after)
			_, got, _, stderr := runCLI(t, "--before", before, "--after", after, "--logical-path", tc.logical)
			if stderr != "" {
				t.Fatalf("go stderr: %s", stderr)
			}
			cmd := exec.Command("python3", py, "--before", before, "--after", after, "--logical-path", tc.logical)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("python: %v", err)
			}
			var want []Candidate
			if err := json.Unmarshal(out, &want); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("count go=%d py=%d\ngo=%s\npy=%s", len(got), len(want), dumpC(got), dumpC(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("candidate %d\ngo=%+v\npy=%+v", i, got[i], want[i])
				}
			}
		})
	}
}

func dumpC(c []Candidate) string {
	b, _ := json.Marshal(c)
	return string(b)
}
