package suppress

import (
	"fmt"
	"sort"
	"strings"
)

func checkPyproject(relPath, text string, al allowlist) ([]violation, error) {
	data, err := parseTOML(text)
	if err != nil {
		return nil, err
	}
	tool, ok := asMap(data["tool"])
	if !ok {
		return nil, nil
	}
	var violations []violation
	if ruff, ok := asMap(tool["ruff"]); ok {
		for _, key := range legacyRuffKeys {
			if _, present := ruff[key]; present {
				violations = append(violations, violation{
					Path: relPath, Rule: "config.pyproject.legacy-ruff-key",
					Message: fmt.Sprintf("Legacy [tool.ruff].%s is forbidden; use [tool.ruff.lint]", key),
				})
			}
		}
		if lint, ok := asMap(ruff["lint"]); ok {
			if _, present := lint["extend-ignore"]; present {
				violations = append(violations, violation{
					Path: relPath, Rule: "config.pyproject.ruff-extend-ignore",
					Message: "[tool.ruff.lint].extend-ignore is forbidden",
				})
			}
			if raw, present := lint["ignore"]; present {
				if ignores, ok := stringList(raw); ok {
					var bad []string
					for _, code := range ignores {
						if _, allowed := al.RuffGlobal[code]; !allowed || strings.EqualFold(code, "ALL") || strings.Contains(code, "*") {
							bad = append(bad, code)
						}
					}
					if len(bad) > 0 {
						violations = append(violations, violation{
							Path: relPath, Rule: "config.pyproject.ruff-ignore-disallowed",
							Message: fmt.Sprintf("[tool.ruff.lint].ignore contains disallowed codes %s (allowed: %s)",
								pyList(bad), pyList(sortedKeys(al.RuffGlobal))),
						})
					}
				}
			}
			for _, tableName := range []string{"per-file-ignores", "extend-per-file-ignores"} {
				if raw, present := lint[tableName]; present {
					violations = append(violations, checkRuffPerFileIgnores(relPath, tableName, raw, al)...)
				}
			}
		}
	}
	if _, present := tool["bandit"]; present {
		violations = append(violations, checkBanditSection(relPath, tool["bandit"], al)...)
	}
	return violations, nil
}

func checkRuffPerFileIgnores(relPath, tableName string, mapping any, al allowlist) []violation {
	m, ok := asMap(mapping)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var violations []violation
	for _, pattern := range keys {
		rules := m[pattern]
		if !isTestPathPattern(pattern) {
			violations = append(violations, violation{
				Path: relPath, Rule: "config.pyproject.ruff-per-file-ignores-path",
				Message: fmt.Sprintf("[tool.ruff.lint.%s] pattern %s is not a test-only path", tableName, pyQuote(pattern)),
			})
		}
		var ruleList []string
		if list, ok := stringList(rules); ok {
			ruleList = list
		} else if s, ok := rules.(string); ok {
			ruleList = []string{s}
		} else if rules != nil {
			ruleList = []string{fmt.Sprint(rules)}
		}
		var disallowed []string
		for _, r := range ruleList {
			_, allowed := al.RuffTestPerFile[r]
			if !allowed || strings.EqualFold(r, "ALL") || strings.Contains(r, "*") {
				disallowed = append(disallowed, r)
			}
		}
		if len(disallowed) > 0 {
			violations = append(violations, violation{
				Path: relPath, Rule: "config.pyproject.ruff-per-file-ignores-rule",
				Message: fmt.Sprintf("[tool.ruff.lint.%s] pattern %s has disallowed rules %s (allowed: %s)",
					tableName, pyQuote(pattern), pyList(disallowed), pyList(sortedKeys(al.RuffTestPerFile))),
			})
		}
	}
	return violations
}

func checkBanditSection(relPath string, banditCfg any, al allowlist) []violation {
	cfg, ok := asMap(banditCfg)
	if !ok {
		return nil
	}
	allowed, known := al.Bandit[relPath]
	if !known {
		return []violation{{
			Path: relPath, Rule: "config.pyproject.bandit-unallowlisted",
			Message: "[tool.bandit] is not allowlisted for this project in suppression_allowlist.toml",
		}}
	}
	var violations []violation
	skips := map[string]struct{}{}
	if raw, ok := cfg["skips"]; ok {
		for _, s := range stringListLoose(raw) {
			skips[s] = struct{}{}
		}
	}
	if extra := diffSorted(skips, allowed.Skips); len(extra) > 0 {
		violations = append(violations, violation{
			Path: relPath, Rule: "config.pyproject.bandit-skip-disallowed",
			Message: fmt.Sprintf("[tool.bandit].skips adds unallowlisted skips %s", pyList(extra)),
		})
	}
	dirs := map[string]struct{}{}
	if raw, ok := cfg["exclude_dirs"]; ok {
		for _, s := range stringListLoose(raw) {
			dirs[s] = struct{}{}
		}
	}
	if extra := diffSorted(dirs, allowed.ExcludeDirs); len(extra) > 0 {
		violations = append(violations, violation{
			Path: relPath, Rule: "config.pyproject.bandit-exclude-dir-disallowed",
			Message: fmt.Sprintf("[tool.bandit].exclude_dirs adds unallowlisted dirs %s", pyList(extra)),
		})
	}
	return violations
}

func checkGolangci(relPath, text string, al allowlist) ([]violation, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	doc, err := parseYAML(text)
	if err != nil {
		return nil, err
	}
	if doc.v == nil {
		return nil, nil
	}
	data, ok := asMap(doc.v)
	if !ok {
		return nil, nil
	}
	var violations []violation
	if linters, ok := asMap(data["linters"]); ok {
		if pyTruthy(linters["disable"]) {
			violations = append(violations, violation{
				Path: relPath, Rule: "config.golangci.disable-forbidden",
				Message: "linters.disable is forbidden in .golangci.yml",
			})
		}
		if pyTruthy(linters["disable-all"]) {
			violations = append(violations, violation{
				Path: relPath, Rule: "config.golangci.disable-forbidden",
				Message: "linters.disable-all is forbidden in .golangci.yml",
			})
		}
		if exclusions, ok := asMap(linters["exclusions"]); ok {
			for _, extraKey := range []string{"paths", "presets"} {
				if pyTruthy(exclusions[extraKey]) {
					violations = append(violations, violation{
						Path: relPath, Rule: "config.golangci.exclusion-unallowlisted",
						Message: fmt.Sprintf("linters.exclusions.%s is not allowlisted", extraKey),
					})
				}
			}
			allowed := al.Golangci[relPath]
			rules, _ := asSlice(exclusions["rules"])
			for idx, raw := range rules {
				rule, isMap := asMap(raw)
				if !isMap || !golangciRuleAllowed(rule, allowed) {
					violations = append(violations, violation{
						Path: relPath, Rule: "config.golangci.exclusion-unallowlisted",
						Message: fmt.Sprintf("linters.exclusions.rules[%d] is not in suppression_allowlist.toml: %s", idx, pyRepr(raw)),
					})
				}
			}
		}
	}
	if issues, ok := asMap(data["issues"]); ok && pyTruthy(issues["exclude-rules"]) {
		violations = append(violations, violation{
			Path: relPath, Rule: "config.golangci.exclusion-unallowlisted",
			Message: "legacy issues.exclude-rules is forbidden; use frozen linters.exclusions.rules",
		})
	}
	return violations, nil
}

func golangciRuleAllowed(rule map[string]any, allowed []golangciEntry) bool {
	known := map[string]struct{}{"path": {}, "path-except": {}, "text": {}, "source": {}, "linters": {}}
	for k := range rule {
		if _, ok := known[k]; !ok {
			return false
		}
	}
	rawLinters, ok := asSlice(rule["linters"])
	if !ok || len(rawLinters) == 0 {
		return false
	}
	ruleSet := map[string]struct{}{}
	for _, item := range rawLinters {
		ruleSet[fmt.Sprint(item)] = struct{}{}
	}
	for _, entry := range allowed {
		if yamlOpt(rule, "path") == ptrString(entry.Path) &&
			yamlOpt(rule, "path-except") == ptrString(entry.PathExcept) &&
			yamlOpt(rule, "text") == ptrString(entry.Text) &&
			yamlOpt(rule, "source") == ptrString(entry.Source) &&
			(subsetSet(ruleSet, entry.Linters) || hasKey(entry.Linters, "all")) {
			return true
		}
	}
	return false
}

func hasKey(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
}

func ptrString(p *string) string {
	if p == nil {
		return "\x00"
	}
	return *p
}

func yamlOpt(rule map[string]any, key string) string {
	v, ok := rule[key]
	if !ok || v == nil {
		return "\x00"
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprint(v)
	}
	return s
}

func checkCargoToml(relPath, text string, al allowlist) ([]violation, error) {
	data, err := parseTOML(text)
	if err != nil {
		return nil, err
	}
	var violations []violation
	for _, table := range collectCargoLintTables(data) {
		allowed := al.Cargo[cargoKey{path: relPath, section: table.section}]
		names := make([]string, 0, len(table.fields))
		for name := range table.fields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, lintName := range names {
			if isCargoAllow(table.fields[lintName]) && !hasKey(allowed, lintName) {
				violations = append(violations, violation{
					Path: relPath, Rule: "config.cargo.lint-allow-unallowlisted",
					Message: fmt.Sprintf("[%s].%s = allow is not allowlisted in suppression_allowlist.toml", table.section, lintName),
				})
			}
		}
	}
	return violations, nil
}

type cargoTable struct {
	section string
	fields  map[string]any
}

func collectCargoLintTables(data map[string]any) []cargoTable {
	var tables []cargoTable
	add := func(prefix string, root any) {
		obj, ok := asMap(root)
		if !ok {
			return
		}
		names := make([]string, 0, len(obj))
		for name := range obj {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, toolName := range names {
			if tool, ok := asMap(obj[toolName]); ok {
				tables = append(tables, cargoTable{prefix + "." + toolName, tool})
			}
		}
	}
	add("lints", data["lints"])
	if ws, ok := asMap(data["workspace"]); ok {
		add("workspace.lints", ws["lints"])
	}
	return tables
}

func isCargoAllow(value any) bool {
	switch t := value.(type) {
	case string:
		return strings.TrimSpace(t) == "allow"
	case map[string]any:
		return strings.TrimSpace(fmt.Sprint(t["level"])) == "allow"
	default:
		return false
	}
}
