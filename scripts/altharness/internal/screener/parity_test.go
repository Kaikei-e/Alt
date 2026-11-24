package screener

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestCorpusGroundTruth(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	corpus := filepath.Join(root, "scripts", "harness", "corpus")
	cases := []struct {
		name      string
		before    string
		after     string
		logical   string
		wantKinds []string
	}{
		{"C-01", "C-01/before.go", "C-01/after.go", "corpus/C-01/after.go", []string{"go.condition_operator_change"}},
		{"C-02", "C-02/before.go", "C-02/after.go", "corpus/C-02/after.go", []string{"go.condition_operator_change"}},
		{"C-03", "C-03/before.go", "C-03/after.go", "corpus/C-03/after.go", []string{"go.test_logger_discard_change"}},
		{"C-06", "C-06/before.py", "C-06/after.py", "corpus/C-06/after.py", []string{"python.control_flow_return_change"}},
		{"C-07", "C-07/before.py", "C-07/after.py", "corpus/C-07/after.py", []string{"python.context_manager_change", "python.finally_body_change"}},
		{"C-10", "C-10/before.ts", "C-10/after.ts", "corpus/C-10/after.ts", []string{"ts.playwright_wait_change"}},
		{"C-11", "C-11/before.go", "C-11/after.go", "corpus/C-11/after.go", []string{"go.http_server_fields_change"}},
		{"C-12", "C-12/before.go", "C-12/after.go", "corpus/C-12/after.go", []string{"go.file_mode_change"}},
		{"C-13", "C-13/before.go", "C-13/after.go", "corpus/C-13/after.go", []string{"go.output_escaping_change"}},
		{"C-14", "C-14/before.go", "C-14/after.go", "corpus/C-14/after.go", []string{"go.reverse_proxy_rewrite_change"}},
		{"C-15", "C-15/before.go", "C-15/after.go", "corpus/C-15/after.go", []string{"go.body_limit_change"}},
		{"C-17", "C-17/before.rs", "C-17/after.rs", "corpus/C-17/after.rs", []string{"rust.fallible_error_swallow"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := filepath.Join(corpus, tc.before)
			after := filepath.Join(corpus, tc.after)

			code, got, _, stderr := runCLI(t, "--before", before, "--after", after, "--logical-path", tc.logical)
			if code != exitOK || stderr != "" {
				t.Fatalf("pair mode: code=%d stderr=%q", code, stderr)
			}
			var gotKinds []string
			for _, c := range got {
				if c.File != tc.logical {
					t.Fatalf("candidate file=%q want %q (%s)", c.File, tc.logical, dumpC(got))
				}
				if c.ID == "" || c.Function == "" || c.Before == "" || c.After == "" {
					t.Fatalf("incomplete candidate: %+v", c)
				}
				if c.Before == c.After {
					t.Fatalf("candidate before == after: %+v", c)
				}
				gotKinds = append(gotKinds, c.Kind)
			}
			sort.Strings(gotKinds)
			wantKinds := append([]string(nil), tc.wantKinds...)
			sort.Strings(wantKinds)
			if !reflect.DeepEqual(gotKinds, wantKinds) {
				t.Fatalf("kinds=%v want=%v\ncands=%s", gotKinds, wantKinds, dumpC(got))
			}

			failCode, failGot, _, _ := runCLI(t, "--before", before, "--after", after, "--logical-path", tc.logical, "--fail-on-candidates")
			if failCode != exitCandidates || len(failGot) != len(got) {
				t.Fatalf("--fail-on-candidates: code=%d len=%d want code=%d len=%d", failCode, len(failGot), exitCandidates, len(got))
			}
		})
	}
}

func dumpC(c []Candidate) string {
	b, _ := json.Marshal(c)
	return string(b)
}
