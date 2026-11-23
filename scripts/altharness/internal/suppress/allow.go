package suppress

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	maxRuffGlobal = map[string]struct{}{"E501": {}, "RUF001": {}, "RUF002": {}, "RUF003": {}}
	maxRuffTest   = map[string]struct{}{"S101": {}}
	frozenBandit  = map[string][2]map[string]struct{}{
		"tag-generator/app/pyproject.toml": {
			setOf("B101", "B404", "B603", "B110", "B104", "B108", "B602"),
			setOf("tests", ".venv", "__pycache__", "*.egg-info", "build", "dist"),
		},
		"tts-speaker/pyproject.toml": {
			setOf("B101", "B104"),
			setOf("tests", ".venv", "__pycache__", "*.egg-info", "build", "dist"),
		},
	}
)

func setOf(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, s := range items {
		m[s] = struct{}{}
	}
	return m
}

func loadAllowlist(text, rel, repo string, today time.Time) (allowlist, []violation, error) {
	if !validUTF8([]byte(text)) {
		return allowlist{}, nil, fmt.Errorf("'utf-8' codec can't decode allowlist")
	}
	data, err := parseTOML(text)
	if err != nil {
		return allowlist{}, nil, err
	}
	var violations []violation
	ruffCfg, _ := asMap(data["ruff"])
	if _, ok := data["ruff"]; ok && ruffCfg == nil {
		return allowlist{}, nil, fmt.Errorf("ruff allowlist section has invalid type")
	}
	if ruffCfg == nil {
		ruffCfg = map[string]any{}
	}
	ruffGlobal, err := stringSetField(ruffCfg, "allowed_global_ignores")
	if err != nil {
		return allowlist{}, nil, err
	}
	ruffTest, err := stringSetField(ruffCfg, "allowed_test_per_file_ignores")
	if err != nil {
		return allowlist{}, nil, err
	}
	if !subset(ruffGlobal, maxRuffGlobal) || !subset(ruffTest, maxRuffTest) {
		if !pyTruthy(ruffCfg["adr"]) {
			violations = append(violations, violation{
				Path: rel, Rule: "config.allowlist.invalid",
				Message: fmt.Sprintf("[ruff] exceeds policy ceiling (%s / %s) without 'adr = \"ADR-NNNNNN\"'",
					pyList(sortedKeys(maxRuffGlobal)), pyList(sortedKeys(maxRuffTest))),
			})
		} else {
			violations = append(violations, validateLedgerMetadata(ruffCfg, "ruff", rel, repo, today)...)
		}
	}
	bandit, err := loadBandit(data["bandit"], rel, repo, &violations, today)
	if err != nil {
		return allowlist{}, nil, err
	}
	gol, err := loadGolangci(data["golangci_exclusions"], rel, repo, &violations, today)
	if err != nil {
		return allowlist{}, nil, err
	}
	cargo, err := loadCargoAllows(data["cargo_lint_allows"], rel, repo, &violations, today)
	if err != nil {
		return allowlist{}, nil, err
	}
	return allowlist{
		RuffGlobal: ruffGlobal, RuffTestPerFile: ruffTest, Bandit: bandit, Golangci: gol, Cargo: cargo,
	}, violations, nil
}

func loadBandit(raw any, rel, repo string, violations *[]violation, today time.Time) (map[string]banditAllow, error) {
	out := map[string]banditAllow{}
	if raw == nil {
		return out, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("bandit allowlist section has invalid type")
	}
	for idx, item := range arr {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("bandit[%d] is not a table", idx)
		}
		label := fmt.Sprintf("bandit[%d]", idx)
		*violations = append(*violations, validateLedgerMetadata(entry, label, rel, repo, today)...)
		pathVal, pathOK := entry["path"].(string)
		skips, skipsOK := asStringSet(entry["skips"])
		if _, has := entry["skips"]; !has {
			skips, skipsOK = map[string]struct{}{}, true
		}
		excl, exclOK := asStringSet(entry["exclude_dirs"])
		if _, has := entry["exclude_dirs"]; !has {
			excl, exclOK = map[string]struct{}{}, true
		}
		if !pathOK || !skipsOK || !exclOK {
			*violations = append(*violations, violation{Path: rel, Rule: "config.allowlist.invalid", Message: label + ": invalid path/skips/exclude_dirs types"})
			continue
		}
		itemAllow := banditAllow{Path: pathVal, Skips: skips, ExcludeDirs: excl}
		if entry["status"] == "frozen-baseline" && !pyTruthy(entry["adr"]) {
			base, known := frozenBandit[pathVal]
			if !known || !subset(itemAllow.Skips, base[0]) || !subset(itemAllow.ExcludeDirs, base[1]) {
				*violations = append(*violations, violation{
					Path: rel, Rule: "config.allowlist.invalid",
					Message: fmt.Sprintf("%s: %s exceeds frozen-baseline ceiling and requires 'adr = \"ADR-NNNNNN\"'", label, pyQuote(pathVal)),
				})
			}
		}
		out[pathVal] = itemAllow
	}
	return out, nil
}

func loadGolangci(raw any, rel, repo string, violations *[]violation, today time.Time) (map[string][]golangciEntry, error) {
	grouped := map[string][]golangciEntry{}
	if raw == nil {
		return grouped, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("golangci_exclusions allowlist section has invalid type")
	}
	for idx, item := range arr {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("golangci_exclusions[%d] is not a table", idx)
		}
		label := fmt.Sprintf("golangci_exclusions[%d]", idx)
		*violations = append(*violations, validateLedgerMetadata(entry, label, rel, repo, today)...)
		config := ".golangci.yml"
		if v, ok := entry["config"]; ok {
			s, ok := v.(string)
			if !ok {
				*violations = append(*violations, violation{Path: rel, Rule: "config.allowlist.invalid", Message: label + ": invalid config/linters types"})
				continue
			}
			config = s
		}
		linters, lOK := asStringSet(entry["linters"])
		if _, has := entry["linters"]; !has {
			linters, lOK = map[string]struct{}{}, true
		}
		if !lOK {
			*violations = append(*violations, violation{Path: rel, Rule: "config.allowlist.invalid", Message: label + ": invalid config/linters types"})
			continue
		}
		g := golangciEntry{Config: config, Linters: linters}
		var err error
		if g.Path, err = optString(entry, "path"); err != nil {
			continue
		}
		if g.PathExcept, err = optString(entry, "path_except"); err != nil {
			continue
		}
		if g.Text, err = optString(entry, "text"); err != nil {
			continue
		}
		if g.Source, err = optString(entry, "source"); err != nil {
			continue
		}
		if entry["status"] == "frozen-baseline" && !pyTruthy(entry["adr"]) && !frozenGolangciMatch(g) {
			*violations = append(*violations, violation{
				Path: rel, Rule: "config.allowlist.invalid",
				Message: label + ": exclusion is not part of frozen-baseline and requires 'adr = \"ADR-NNNNNN\"'",
			})
		}
		grouped[config] = append(grouped[config], g)
	}
	return grouped, nil
}

func frozenGolangciMatch(g golangciEntry) bool {
	want := []golangciEntry{
		{Config: ".golangci.yml", Path: sp(`\.pb\.go$`), Linters: setOf("all")},
		{Config: ".golangci.yml", Path: sp(`\.connect\.go$`), Linters: setOf("all")},
		{Config: ".golangci.yml", Path: sp(`server\.gen\.go$`), Linters: setOf("all")},
		{Config: ".golangci.yml", Path: sp(`internal/pki/.*_test\.go$`), Linters: setOf("gosec")},
		{Config: ".golangci.yml", Path: sp(`_test\.go$`), Text: sp("G101"), Linters: setOf("gosec")},
		{Config: ".golangci.yml", PathExcept: sp(`^knowledge-sovereign/app/usecase/(knowledge_home_projector|knowledge_trail_projector)/[^/]+\.go$`), Linters: setOf("forbidigo")},
		{Config: ".golangci.yml", Path: sp(`_test\.go$`), Linters: setOf("forbidigo")},
	}
	for _, e := range want {
		if e.Config == g.Config && ptrEq(e.Path, g.Path) && ptrEq(e.PathExcept, g.PathExcept) && ptrEq(e.Text, g.Text) && ptrEq(e.Source, g.Source) && sameSet(e.Linters, g.Linters) {
			return true
		}
	}
	return false
}

func ptrEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameSet(a, b map[string]struct{}) bool { return subset(a, b) && subset(b, a) }

func sp(s string) *string { return &s }

func loadCargoAllows(raw any, rel, repo string, violations *[]violation, today time.Time) (map[cargoKey]map[string]struct{}, error) {
	out := map[cargoKey]map[string]struct{}{}
	if raw == nil {
		return out, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("cargo_lint_allows allowlist section has invalid type")
	}
	frozen := map[cargoKey]map[string]struct{}{
		{path: "recap-worker/recap-worker/Cargo.toml", section: "lints.clippy"}: setOf(
			"assert_is_empty", "cast_possible_truncation", "cast_possible_wrap", "cast_precision_loss",
			"collapsible_if", "doc_markdown", "double_must_use", "explicit_iter_loop", "items_after_statements",
			"missing_errors_doc", "missing_panics_doc", "module_name_repetitions", "must_use_candidate",
			"needless_pass_by_value", "option_if_let_else", "or_fun_call", "redundant_closure", "uninlined_format_args",
		),
	}
	for idx, item := range arr {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cargo_lint_allows[%d] is not a table", idx)
		}
		label := fmt.Sprintf("cargo_lint_allows[%d]", idx)
		*violations = append(*violations, validateLedgerMetadata(entry, label, rel, repo, today)...)
		pathVal, pathOK := entry["path"].(string)
		section, secOK := entry["section"].(string)
		allows, allOK := asStringSet(entry["allows"])
		if _, has := entry["allows"]; !has {
			allows, allOK = map[string]struct{}{}, true
		}
		if !pathOK || !secOK || !allOK {
			*violations = append(*violations, violation{Path: rel, Rule: "config.allowlist.invalid", Message: label + ": invalid path/section/allows types"})
			continue
		}
		key := cargoKey{path: pathVal, section: section}
		if entry["status"] == "frozen-baseline" && !pyTruthy(entry["adr"]) {
			base, known := frozen[key]
			if !known || !subset(allows, base) {
				*violations = append(*violations, violation{
					Path: rel, Rule: "config.allowlist.invalid",
					Message: fmt.Sprintf("%s: %s exceeds frozen-baseline ceiling and requires 'adr = \"ADR-NNNNNN\"'", label, pyTuple(pathVal, section)),
				})
			}
		}
		out[key] = allows
	}
	return out, nil
}

func pyTuple(items ...string) string {
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = pyQuote(s)
	}
	if len(parts) == 1 {
		return "(" + parts[0] + ",)"
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func validateLedgerMetadata(entry map[string]any, label, rel, repo string, today time.Time) []violation {
	var violations []violation
	reason, isStr := entry["reason"].(string)
	if !isStr || nonSpaceLen(reason) < minReasonLength {
		violations = append(violations, violation{
			Path: rel, Rule: "config.allowlist.invalid",
			Message: fmt.Sprintf("%s: missing or too-short 'reason' (>= %d non-space chars)", label, minReasonLength),
		})
	}
	violations = append(violations, validateLedgerExpires(entry, label, rel, today)...)
	status, _ := entry["status"].(string)
	adr := entry["adr"]
	adrValid := false
	if status != "frozen-baseline" && !pyTruthy(adr) {
		violations = append(violations, violation{
			Path: rel, Rule: "config.allowlist.invalid",
			Message: fmt.Sprintf("%s: non-baseline entry must specify 'adr = \"ADR-NNNNNN\"'", label),
		})
	}
	if adr != nil {
		s, ok := adr.(string)
		if !ok {
			violations = append(violations, violation{Path: rel, Rule: "config.allowlist.invalid", Message: fmt.Sprintf("%s: 'adr' must be a string", label)})
		} else if m := adrExactRE.FindStringSubmatch(strings.TrimSpace(s)); m == nil {
			violations = append(violations, violation{
				Path: rel, Rule: "config.allowlist.invalid",
				Message: fmt.Sprintf("%s: invalid ADR id %s (expected ADR-NNNNNN)", label, pyQuote(s)),
			})
		} else if !fileExists(joinPath(repo, "docs", "ADR", m[1]+".md")) {
			violations = append(violations, violation{
				Path: rel, Rule: "config.allowlist.invalid",
				Message: fmt.Sprintf("%s: %s referenced but docs/ADR/%s.md missing", label, strings.TrimSpace(s), m[1]),
			})
		} else {
			adrValid = true
		}
	}
	if status == "frozen-baseline" && !adrValid {
		if raw, ok := entry["expires"].(string); ok {
			if exp, ok := parseISODate(raw); ok && exp.After(frozenBaselineMaxExpires) {
				violations = append(violations, violation{
					Path: rel, Rule: "config.allowlist.expires-extended-without-adr",
					Message: fmt.Sprintf("%s: frozen-baseline 'expires' (%s) exceeds ceiling %s and requires a valid existing 'adr = \"ADR-NNNNNN\"'", label, exp.Format("2006-01-02"), frozenBaselineMaxExpires.Format("2006-01-02")),
				})
			}
		}
	}
	return violations
}

func validateLedgerExpires(entry map[string]any, label, rel string, today time.Time) []violation {
	expires, has := entry["expires"]
	if !has || expires == nil || (isString(expires) && strings.TrimSpace(asStr(expires)) == "") {
		return []violation{{Path: rel, Rule: "config.allowlist.missing-expires", Message: fmt.Sprintf("%s: missing required 'expires = \"YYYY-MM-DD\"'", label)}}
	}
	s, ok := expires.(string)
	if !ok {
		return []violation{{Path: rel, Rule: "config.allowlist.invalid", Message: fmt.Sprintf("%s: 'expires' must be a YYYY-MM-DD string", label)}}
	}
	exp, ok := parseISODate(s)
	if !ok {
		return []violation{{Path: rel, Rule: "config.allowlist.invalid", Message: fmt.Sprintf("%s: invalid 'expires' date %s (expected YYYY-MM-DD)", label, pyQuote(s))}}
	}
	if exp.Before(today) {
		return []violation{{Path: rel, Rule: "config.allowlist.expired", Message: fmt.Sprintf("%s: suppression expired on %s (today is %s)", label, exp.Format("2006-01-02"), today.Format("2006-01-02"))}}
	}
	maxExp := today.AddDate(0, 0, maxExpiresHorizonDays)
	if exp.After(maxExp) {
		return []violation{{
			Path: rel, Rule: "config.allowlist.expires-too-far",
			Message: fmt.Sprintf("%s: 'expires' (%s) exceeds maximum horizon of %d days from %s (max %s)", label, exp.Format("2006-01-02"), maxExpiresHorizonDays, today.Format("2006-01-02"), maxExp.Format("2006-01-02")),
		}}
	}
	return nil
}

func stringSetField(m map[string]any, key string) (map[string]struct{}, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return map[string]struct{}{}, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s has invalid type", key)
	}
	return stringSetSlice(arr), nil
}

func asStringSet(v any) (map[string]struct{}, bool) {
	if v == nil {
		return nil, false
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	return stringSetSlice(arr), true
}

func stringSetSlice(arr []any) map[string]struct{} {
	m := make(map[string]struct{}, len(arr))
	for _, el := range arr {
		m[fmt.Sprint(el)] = struct{}{}
	}
	return m
}

func optString(entry map[string]any, key string) (*string, error) {
	v, ok := entry[key]
	if !ok || v == nil {
		return nil, nil
	}
	s := fmt.Sprint(v)
	if str, ok := v.(string); ok {
		s = str
	}
	return &s, nil
}

func isString(v any) bool { _, ok := v.(string); return ok }
func asStr(v any) string  { s, _ := v.(string); return s }

func readAllowFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
