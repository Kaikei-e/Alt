package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func collectCaseDirs(corpusDir string, errors *[]string) []string {
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("corpus directory not readable: %v", err))
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var caseDirs []string
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(corpusDir, name)
		info, lerr := os.Lstat(path)
		if lerr != nil {
			*errors = append(*errors, fmt.Sprintf("unexpected file in corpus root: %s", pyRepr(name)))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			*errors = append(*errors, fmt.Sprintf("symlink entry not allowed in corpus: %s", pyRepr(name)))
			continue
		}
		if info.IsDir() {
			if !caseDirRE.MatchString(name) {
				*errors = append(*errors, fmt.Sprintf("unexpected non-case directory in corpus: %s", pyRepr(name)))
				continue
			}
			caseDirs = append(caseDirs, path)
			continue
		}
		if !has(ignoredCorpusRootFiles, name) {
			*errors = append(*errors, fmt.Sprintf("unexpected file in corpus root: %s", pyRepr(name)))
		}
	}
	return caseDirs
}

// DiscoverCases validates a contiguous C-01..C-NN layout and loads every case.
func DiscoverCases(corpusDir string, minCases int, allowEmpty bool) ([]Case, []string) {
	info, err := os.Lstat(corpusDir)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, []string{fmt.Sprintf("corpus directory must not be a symlink: %s", corpusDir)}
	}
	if err != nil || !info.IsDir() {
		if allowEmpty {
			return nil, nil
		}
		return nil, []string{fmt.Sprintf("corpus directory not found: %s", corpusDir)}
	}
	var errors []string
	caseDirs := collectCaseDirs(corpusDir, &errors)
	if len(caseDirs) == 0 {
		if allowEmpty && len(errors) == 0 {
			return nil, nil
		}
		errors = append(errors, fmt.Sprintf("corpus directory has 0 cases (expected >= %d): %s", minCases, corpusDir))
		return nil, errors
	}
	if len(caseDirs) < minCases {
		errors = append(errors, fmt.Sprintf("corpus has %d case(s), expected at least %d", len(caseDirs), minCases))
	}
	expected := make([]string, len(caseDirs))
	actual := make([]string, len(caseDirs))
	for i := range caseDirs {
		expected[i] = fmt.Sprintf("C-%02d", i+1)
		actual[i] = filepath.Base(caseDirs[i])
	}
	if !stringSliceEqual(actual, expected) {
		errors = append(errors, fmt.Sprintf("case directories must be contiguous C-01..C-%02d: expected %s, got %s", len(caseDirs), pyList(expected), pyList(actual)))
	}
	var cases []Case
	for _, dir := range caseDirs {
		loaded, caseErrors := LoadCase(dir)
		errors = append(errors, caseErrors...)
		if loaded != nil {
			cases = append(cases, *loaded)
		}
	}
	return cases, errors
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
