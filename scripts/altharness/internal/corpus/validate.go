package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func nonEmptyStr(table map[string]any, key, label string, errors *[]string) string {
	val, ok := table[key]
	s, isStr := val.(string)
	if !ok || !isStr || strings.TrimSpace(s) == "" {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' must be a non-empty string", label, key))
		return ""
	}
	return s
}

func strList(table map[string]any, key, label string, errors *[]string, allowEmpty bool) []string {
	raw, ok := table[key]
	items, isList := raw.([]any)
	if !ok || !isList {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' must be a list of strings", label, key))
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, isStr := item.(string)
		if !isStr {
			*errors = append(*errors, fmt.Sprintf("%s: '%s' must be a list of strings", label, key))
			return nil
		}
		out = append(out, s)
	}
	if !allowEmpty && len(out) == 0 {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' must not be empty", label, key))
	}
	return out
}

func validatePlainFilename(caseDir, filename, fieldName, label string, errors *[]string) (string, bool) {
	if filename == "" || filename == "." || filename == ".." || strings.Contains(filename, "/") || strings.Contains(filename, `\`) || strings.Contains(filename, "..") {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' must be a plain relative filename inside case dir, got %s", label, fieldName, pyRepr(filename)))
		return "", false
	}
	candidate := filepath.Join(caseDir, filename)
	info, err := os.Lstat(candidate)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' file not found: %s", label, fieldName, pyRepr(filename)))
		return "", false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' must not be a symlink: %s", label, fieldName, pyRepr(filename)))
		return "", false
	}
	if !info.Mode().IsRegular() {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' file not found: %s", label, fieldName, pyRepr(filename)))
		return "", false
	}
	caseRoot, err := filepath.EvalSymlinks(caseDir)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' escapes case directory: %s", label, fieldName, pyRepr(filename)))
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' escapes case directory: %s", label, fieldName, pyRepr(filename)))
		return "", false
	}
	if !isRelTo(resolved, caseRoot) || filepath.Dir(resolved) != caseRoot {
		*errors = append(*errors, fmt.Sprintf("%s: '%s' escapes case directory: %s", label, fieldName, pyRepr(filename)))
		return "", false
	}
	return candidate, true
}

func isNormalizedRepoRelPath(rel string) bool { return isNormalizedRepoRel(rel) }

func pathWithin(path, root string) bool { return isRelTo(path, root) }

func isRelTo(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func validateOrigin(data map[string]any, label string, errors *[]string) (Origin, bool) {
	originRaw, ok := asMap(data["origin"])
	if !ok {
		*errors = append(*errors, fmt.Sprintf("%s: missing [origin] table", label))
		return Origin{}, false
	}
	olabel := label + ".origin"
	branch := nonEmptyStr(originRaw, "branch", olabel, errors)
	baseCommit := nonEmptyStr(originRaw, "base_commit", olabel, errors)
	if baseCommit != "" && !commitSHARe.MatchString(baseCommit) {
		*errors = append(*errors, fmt.Sprintf("%s: 'base_commit' must be 7..40 lowercase hex chars, got %s", olabel, pyRepr(baseCommit)))
	}
	files := strList(originRaw, "files", olabel, errors, false)
	for _, rel := range files {
		if !isNormalizedRepoRel(rel) {
			*errors = append(*errors, fmt.Sprintf("%s: invalid repo-relative path in 'files': %s", olabel, pyRepr(rel)))
		}
	}
	commits := strList(originRaw, "commits", olabel, errors, false)
	for _, commit := range commits {
		if commit != WorktreeUncommitted && !commitSHARe.MatchString(commit) {
			*errors = append(*errors, fmt.Sprintf("%s: invalid commit %s (expected 7..40 hex SHA or %s)", olabel, pyRepr(commit), pyRepr(WorktreeUncommitted)))
		}
	}
	sourceRef := nonEmptyStr(originRaw, "source_ref", olabel, errors)
	notes := nonEmptyStr(originRaw, "notes", olabel, errors)
	if notes != "" && nonSpaceLen(notes) < MinNotesNonSpaceLen {
		*errors = append(*errors, fmt.Sprintf("%s: 'notes' must contain at least %d non-space chars", olabel, MinNotesNonSpaceLen))
	}
	return Origin{
		Branch:     branch,
		BaseCommit: baseCommit,
		Files:      files,
		Commits:    commits,
		SourceRef:  sourceRef,
		Notes:      notes,
	}, true
}

func validateCheckLists(planned, active []string, detLabel string, errors *[]string) {
	if len(unique(planned)) != len(planned) {
		*errors = append(*errors, fmt.Sprintf("%s: duplicate entries in 'planned_checks'", detLabel))
	}
	if len(unique(active)) != len(active) {
		*errors = append(*errors, fmt.Sprintf("%s: duplicate entries in 'active_checks'", detLabel))
	}
	var unknown []string
	for _, c := range planned {
		if !knownCheck(c) {
			unknown = append(unknown, c)
		}
	}
	if len(unknown) > 0 {
		*errors = append(*errors, fmt.Sprintf("%s: unknown 'planned_checks': %s", detLabel, pyList(unknown)))
	}
	var unimplemented []string
	for _, c := range active {
		if !has(ImplementedChecks, c) {
			unimplemented = append(unimplemented, c)
		}
	}
	if len(unimplemented) > 0 {
		*errors = append(*errors, fmt.Sprintf("%s: 'active_checks' contains non-implemented check(s): %s", detLabel, pyList(unimplemented)))
	}
	plannedSet := toSet(planned)
	var extra []string
	for _, c := range active {
		if !has(plannedSet, c) {
			extra = append(extra, c)
		}
	}
	if len(extra) > 0 {
		*errors = append(*errors, fmt.Sprintf("%s: 'active_checks' must be a subset of 'planned_checks', extra: %s", detLabel, pyList(extra)))
	}
	activeSet := toSet(active)
	var unactivated []string
	for _, c := range planned {
		if has(ImplementedChecks, c) && !has(ProcessChecks, c) && !has(activeSet, c) {
			unactivated = append(unactivated, c)
		}
	}
	if len(unactivated) > 0 {
		*errors = append(*errors, fmt.Sprintf("%s: implemented check(s) in 'planned_checks' must be listed in 'active_checks': %s", detLabel, pyList(unactivated)))
	}
}

func parseExpectedSignalsTable(raw any, active []string, detLabel string, errors *[]string) map[string]string {
	table, ok := asMap(raw)
	if !ok {
		*errors = append(*errors, fmt.Sprintf("%s: 'expected_signals' must be a table of string -> string", detLabel))
		return map[string]string{}
	}
	parsed := map[string]string{}
	keys := make([]string, 0, len(table))
	for k, v := range table {
		s, isStr := v.(string)
		if !isStr {
			*errors = append(*errors, fmt.Sprintf("%s: 'expected_signals' must be a table of string -> string", detLabel))
			return map[string]string{}
		}
		parsed[k] = s
		keys = append(keys, k)
	}
	if !sameSet(keys, active) {
		*errors = append(*errors, fmt.Sprintf("%s: 'expected_signals' keys %s must match 'active_checks' %s", detLabel, pyList(sortedStrings(keys)), pyList(sortedStrings(active))))
	}
	var empty []string
	for k, v := range parsed {
		if strings.TrimSpace(v) == "" {
			empty = append(empty, k)
		}
	}
	if len(empty) > 0 {
		*errors = append(*errors, fmt.Sprintf("%s: 'expected_signals' has empty signal(s) for check(s): %s", detLabel, pyList(sortedStrings(empty))))
	}
	return parsed
}

func validateExpectedSignals(det map[string]any, active []string, caught bool, detLabel string, errors *[]string) (string, map[string]string) {
	expected := ""
	if raw, ok := det["expected_signal"]; ok {
		s, isStr := raw.(string)
		if !isStr {
			*errors = append(*errors, fmt.Sprintf("%s: 'expected_signal' must be a string", detLabel))
		} else {
			expected = s
		}
	}
	_, hasTable := det["expected_signals"]
	if !caught {
		if expected != "" {
			*errors = append(*errors, fmt.Sprintf("%s: 'expected_signal' must be '' when currently_caught=false", detLabel))
		}
		if hasTable {
			if table, ok := asMap(det["expected_signals"]); !ok || len(table) != 0 {
				*errors = append(*errors, fmt.Sprintf("%s: 'expected_signals' must be empty when currently_caught=false", detLabel))
			}
		}
		return expected, map[string]string{}
	}
	if hasTable {
		if expected != "" {
			*errors = append(*errors, fmt.Sprintf("%s: do not set both non-empty 'expected_signal' and 'expected_signals'", detLabel))
		}
		parsed := parseExpectedSignalsTable(det["expected_signals"], active, detLabel, errors)
		return expected, parsed
	}
	if len(active) > 1 {
		*errors = append(*errors, fmt.Sprintf("%s: multiple active_checks require [detection.expected_signals] mapping each active check to its expected signal", detLabel))
		return expected, map[string]string{}
	}
	if strings.TrimSpace(expected) == "" {
		*errors = append(*errors, fmt.Sprintf("%s: 'expected_signal' must be non-empty when currently_caught=true", detLabel))
		return expected, map[string]string{}
	}
	out := map[string]string{}
	if len(active) == 1 {
		out[active[0]] = expected
	}
	return expected, out
}

func validateDetection(data map[string]any, label string, errors *[]string) (Detection, bool) {
	detRaw, ok := asMap(data["detection"])
	if !ok {
		*errors = append(*errors, fmt.Sprintf("%s: missing [detection] table", label))
		return Detection{}, false
	}
	detLabel := label + ".detection"
	planned := strList(detRaw, "planned_checks", detLabel, errors, false)
	active := strList(detRaw, "active_checks", detLabel, errors, true)
	validateCheckLists(planned, active, detLabel, errors)
	caughtVal, hasCaught := detRaw["currently_caught"]
	caught, isBool := caughtVal.(bool)
	if !hasCaught || !isBool {
		*errors = append(*errors, fmt.Sprintf("%s: 'currently_caught' must be a boolean", detLabel))
		caught = false
	} else if caught != (len(active) > 0) {
		*errors = append(*errors, fmt.Sprintf("%s: 'currently_caught' (%s) must equal bool(active_checks) (%s)", detLabel, pyBool(caught), pyBool(len(active) > 0)))
	}
	method := nonEmptyStr(detRaw, "current_method", detLabel, errors)
	signal, signals := validateExpectedSignals(detRaw, active, caught, detLabel, errors)
	return Detection{
		PlannedChecks:   planned,
		ActiveChecks:    active,
		CurrentlyCaught: caught,
		CurrentMethod:   method,
		ExpectedSignal:  signal,
		ExpectedSignals: signals,
	}, true
}

func validateSnippetBytes(beforePath, afterPath, language, label string, errors *[]string) {
	beforeBytes, err := os.ReadFile(beforePath)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s: before file is not readable: %v", label, err))
		return
	}
	afterBytes, err := os.ReadFile(afterPath)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s: after file is not readable: %v", label, err))
		return
	}
	if string(beforeBytes) == string(afterBytes) {
		*errors = append(*errors, fmt.Sprintf("%s: before and after files are byte-identical", label))
	}
	if len(beforeBytes) > MaxSnippetBytes {
		*errors = append(*errors, fmt.Sprintf("%s: before file exceeds %d bytes (%d bytes)", label, MaxSnippetBytes, len(beforeBytes)))
	}
	if len(afterBytes) > MaxSnippetBytes {
		*errors = append(*errors, fmt.Sprintf("%s: after file exceeds %d bytes (%d bytes)", label, MaxSnippetBytes, len(afterBytes)))
	}
	if language == "binary" {
		return
	}
	for _, role := range []struct {
		name string
		raw  []byte
	}{{"before", beforeBytes}, {"after", afterBytes}} {
		if !utf8.Valid(role.raw) {
			*errors = append(*errors, fmt.Sprintf("%s: %s file is not valid UTF-8 for language=%s: invalid UTF-8", label, role.name, pyRepr(language)))
			continue
		}
		lineCount := len(pySplitLines(string(role.raw)))
		if lineCount > MaxSnippetLines {
			*errors = append(*errors, fmt.Sprintf("%s: %s file exceeds %d lines (%d lines)", label, role.name, MaxSnippetLines, lineCount))
		}
	}
}

func validateCaseFiles(caseDir, beforeFile, afterFile, language string, support []string, label string, errors *[]string) {
	if beforeFile != "" && afterFile != "" && beforeFile == afterFile {
		*errors = append(*errors, fmt.Sprintf("%s: 'before_file' and 'after_file' must be distinct", label))
	}
	var beforePath, afterPath string
	if beforeFile != "" {
		if p, ok := validatePlainFilename(caseDir, beforeFile, "before_file", label, errors); ok {
			beforePath = p
		}
	}
	if afterFile != "" {
		if p, ok := validatePlainFilename(caseDir, afterFile, "after_file", label, errors); ok {
			afterPath = p
		}
	}
	if len(unique(support)) != len(support) {
		*errors = append(*errors, fmt.Sprintf("%s: duplicate entries in 'support_files'", label))
	}
	reserved := map[string]struct{}{beforeFile: {}, afterFile: {}, "meta.toml": {}}
	for _, sf := range support {
		if _, dup := reserved[sf]; dup {
			*errors = append(*errors, fmt.Sprintf("%s: 'support_files' must not duplicate %s", label, pyRepr(sf)))
			continue
		}
		validatePlainFilename(caseDir, sf, "support_files", label, errors)
	}
	if beforePath != "" && afterPath != "" {
		validateSnippetBytes(beforePath, afterPath, language, label, errors)
	}
}

// LoadCase parses and strictly validates one corpus case directory.
func LoadCase(caseDir string) (*Case, []string) {
	expectedID := filepath.Base(caseDir)
	label := "case " + expectedID
	metaPath := filepath.Join(caseDir, "meta.toml")
	info, err := os.Lstat(metaPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, []string{fmt.Sprintf("%s: missing regular meta.toml", label)}
	}
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: failed to parse meta.toml: %v", label, err)}
	}
	if !utf8.Valid(raw) {
		return nil, []string{fmt.Sprintf("%s: failed to parse meta.toml: invalid UTF-8", label)}
	}
	data, err := ParseTOML(string(raw))
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: failed to parse meta.toml: %v", label, err)}
	}
	var errors []string
	caseID := nonEmptyStr(data, "id", label, &errors)
	if caseID != "" && caseID != expectedID {
		errors = append(errors, fmt.Sprintf("%s: meta.toml id %s does not match directory %s", label, pyRepr(caseID), pyRepr(expectedID)))
	}
	title := nonEmptyStr(data, "title", label, &errors)
	category := nonEmptyStr(data, "category", label, &errors)
	if category != "" && !has(ValidCategories, category) {
		errors = append(errors, fmt.Sprintf("%s: invalid category %s", label, pyRepr(category)))
	}
	language := nonEmptyStr(data, "language", label, &errors)
	if language != "" && !has(ValidLanguages, language) {
		errors = append(errors, fmt.Sprintf("%s: invalid language %s", label, pyRepr(language)))
	}
	beforeFile := nonEmptyStr(data, "before_file", label, &errors)
	afterFile := nonEmptyStr(data, "after_file", label, &errors)
	var support []string
	if _, ok := data["support_files"]; ok {
		support = strList(data, "support_files", label, &errors, true)
	}
	mode := nonEmptyStr(data, "evaluation_mode", label, &errors)
	if mode != "" && !has(ValidEvaluationModes, mode) {
		errors = append(errors, fmt.Sprintf("%s: invalid evaluation_mode %s", label, pyRepr(mode)))
	}
	origin, originOK := validateOrigin(data, label, &errors)
	detection, detOK := validateDetection(data, label, &errors)
	validateCaseFiles(caseDir, beforeFile, afterFile, language, support, label, &errors)
	if len(errors) > 0 || !originOK || !detOK {
		return nil, errors
	}
	return &Case{
		CaseID:         caseID,
		CaseDir:        caseDir,
		Title:          title,
		Category:       category,
		Language:       language,
		BeforeFile:     beforeFile,
		AfterFile:      afterFile,
		SupportFiles:   support,
		SupportSet:     toSet(support),
		EvaluationMode: mode,
		Origin:         origin,
		Detection:      detection,
	}, nil
}

func unique(items []string) map[string]struct{} {
	return toSet(items)
}

func toSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		out[item] = struct{}{}
	}
	return out
}

func sameSet(a, b []string) bool {
	sa := toSet(a)
	sb := toSet(b)
	if len(sa) != len(sb) {
		return false
	}
	return containsAll(sa, b)
}

func pyBool(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

func containsAll(set map[string]struct{}, items []string) bool {
	for _, item := range items {
		if !has(set, item) {
			return false
		}
	}
	return true
}
