package packet

import (
	"alt/scripts/altharness/internal/fsperm"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func formatMeasuredYAML(indent string, measured map[string]any) []string {
	child := indent + "  "
	lines := []string{
		indent + "measured:",
		fmt.Sprintf("%sduration_s: %v", child, yamlNumber(measured["duration_s"])),
		fmt.Sprintf("%sexit_code: %v", child, yamlNumber(measured["exit_code"])),
		fmt.Sprintf("%sat_commit: %s", child, yamlQuote(pyStr(measured["at_commit"]))),
	}
	if _, ok := measured["command_sha256"]; ok {
		lines = append(lines, fmt.Sprintf("%scommand_sha256: %s", child, yamlQuote(pyStr(measured["command_sha256"]))))
	}
	return lines
}

func yamlNumber(v any) string {
	switch t := v.(type) {
	case float64:
		return strconvFormat(t)
	case int64:
		return fmt.Sprintf("%d", t)
	case int:
		return fmt.Sprintf("%d", t)
	default:
		return pyStr(v)
	}
}

func strconvFormat(f float64) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", f), "0"), ".")
	if !strings.ContainsAny(s, ".eE") {
		// Keep a fractional form when the rounded value is integral so YAML
		// re-parses duration_s as a number. Integers are valid numbers too.
		return s
	}
	return s
}

func yamlQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return strconvQuote(s)
	}
	return string(b)
}

func strconvQuote(s string) string { return fmt.Sprintf("%q", s) }

func yamlScalarString(v any) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return strconvFormat(t)
	case string:
		return yamlQuote(t)
	default:
		return yamlQuote(pyStr(v))
	}
}

func formatExecutionPolicyYAML(epIndent int, policy, measured map[string]any) []string {
	base := strings.Repeat(" ", epIndent)
	child := strings.Repeat(" ", epIndent+2)
	lines := []string{base + "execution_policy:"}
	// Stable key order for the keys we know, then any extras.
	order := []string{"policy", "foreground_only", "forbid_repo_wide_test_build", "forbid_bulk_autofix", "verification_command", "max_command_budget_s"}
	seen := map[string]struct{}{}
	for _, key := range order {
		if key == "measured" {
			continue
		}
		val, ok := policy[key]
		if !ok {
			continue
		}
		seen[key] = struct{}{}
		lines = append(lines, fmt.Sprintf("%s%s: %s", child, key, yamlScalarString(val)))
	}
	var extra []string
	for key := range policy {
		if key == "measured" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		extra = append(extra, key)
	}
	sortStrings(extra)
	for _, key := range extra {
		lines = append(lines, fmt.Sprintf("%s%s: %s", child, key, yamlScalarString(policy[key])))
	}
	lines = append(lines, formatMeasuredYAML(child, measured)...)
	return lines
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j] < s[j-1] {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
}

func stripMeasuredFromFrontmatter(fm []string) []string {
	epIdx := -1
	for i, line := range fm {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "execution_policy:") {
			epIdx = i
			break
		}
	}
	if epIdx < 0 {
		return fm
	}
	epIndent := len(fm[epIdx]) - len(strings.TrimLeft(fm[epIdx], " "))
	blockEnd := epIdx + 1
	for blockEnd < len(fm) {
		raw := fm[blockEnd]
		if strings.TrimSpace(raw) != "" && len(raw)-len(strings.TrimLeft(raw, " ")) <= epIndent {
			break
		}
		blockEnd++
	}
	var cleaned []string
	idx := epIdx + 1
	for idx < blockEnd {
		raw := fm[idx]
		stripped := strings.TrimLeft(raw, " ")
		cur := len(raw) - len(stripped)
		if strings.HasPrefix(stripped, "measured:") && cur > epIndent {
			idx++
			for idx < blockEnd {
				nxt := fm[idx]
				if strings.TrimSpace(nxt) != "" && len(nxt)-len(strings.TrimLeft(nxt, " ")) <= cur {
					break
				}
				idx++
			}
			continue
		}
		cleaned = append(cleaned, raw)
		idx++
	}
	out := append([]string{}, fm[:epIdx+1]...)
	out = append(out, cleaned...)
	out = append(out, fm[blockEnd:]...)
	return out
}

func updateYAMLFrontmatterMeasured(fm []string, measured, policy map[string]any) ([]string, error) {
	epIdx := -1
	for i, line := range fm {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "execution_policy:") {
			epIdx = i
			break
		}
	}
	if epIdx < 0 {
		return nil, pErr("execution_policy block not found in YAML frontmatter")
	}
	epLine := fm[epIdx]
	epIndent := len(epLine) - len(strings.TrimLeft(epLine, " "))
	epStripped := stripYAMLComment(strings.TrimLeft(epLine, " "))
	colon := findUnquotedColon(epStripped)
	inline := ""
	if colon != -1 {
		inline = strings.TrimSpace(epStripped[colon+1:])
	}
	if inline != "" {
		expanded := formatExecutionPolicyYAML(epIndent, policy, measured)
		out := append([]string{}, fm[:epIdx]...)
		out = append(out, expanded...)
		out = append(out, fm[epIdx+1:]...)
		return out, nil
	}
	blockEnd := epIdx + 1
	for blockEnd < len(fm) {
		raw := fm[blockEnd]
		if strings.TrimSpace(raw) != "" && len(raw)-len(strings.TrimLeft(raw, " ")) <= epIndent {
			break
		}
		blockEnd++
	}
	childIndent := strings.Repeat(" ", epIndent+2)
	var cleaned []string
	idx := epIdx + 1
	for idx < blockEnd {
		raw := fm[idx]
		stripped := strings.TrimLeft(raw, " ")
		cur := len(raw) - len(stripped)
		if strings.HasPrefix(stripped, "measured:") && cur > epIndent {
			childIndent = strings.Repeat(" ", cur)
			idx++
			for idx < blockEnd {
				nxt := fm[idx]
				if strings.TrimSpace(nxt) != "" && len(nxt)-len(strings.TrimLeft(nxt, " ")) <= cur {
					break
				}
				idx++
			}
			continue
		}
		if strings.TrimSpace(raw) != "" && cur > epIndent && len(cleaned) == 0 {
			childIndent = strings.Repeat(" ", cur)
		}
		cleaned = append(cleaned, raw)
		idx++
	}
	measuredLines := formatMeasuredYAML(childIndent, measured)
	out := append([]string{}, fm[:epIdx+1]...)
	out = append(out, cleaned...)
	out = append(out, measuredLines...)
	out = append(out, fm[blockEnd:]...)
	return out, nil
}

var fencedJSONRe = regexp.MustCompile("(?s)```json\\s*\\n.*?\\n```")

func writeMeasuredToPacket(path string, doc *Document, measured map[string]any) error {
	policy, _ := asMap(doc.Data["execution_policy"])
	if policy == nil {
		return pErr("execution_policy block not found")
	}
	if !doc.IsMarkdown {
		policy["measured"] = measured
		doc.Data["execution_policy"] = policy
		encoded, err := writeJSON(doc.Data)
		if err != nil {
			return err
		}
		return os.WriteFile(path, encoded, fsperm.File)
	}
	rawBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	raw := string(rawBytes)
	lines := splitLines(raw)
	fmLines, body, err := extractFrontmatter(lines)
	if err != nil {
		return err
	}
	if fmLines != nil {
		parsed, err := parseYAMLSubset(fmLines)
		if err != nil {
			return err
		}
		if intersectsStructured(parsed) {
			updated, err := updateYAMLFrontmatterMeasured(fmLines, measured, policy)
			if err != nil {
				return err
			}
			rebuilt := append([]string{"---"}, updated...)
			rebuilt = append(rebuilt, "---")
			rebuilt = append(rebuilt, body...)
			return os.WriteFile(path, []byte(strings.Join(rebuilt, "\n")+"\n"), fsperm.File)
		}
	}
	policy["measured"] = measured
	doc.Data["execution_policy"] = policy
	encoded, err := writeJSON(doc.Data)
	if err != nil {
		return err
	}
	replacement := "```json\n" + strings.TrimRight(string(encoded), "\n") + "\n```"
	updated := fencedJSONRe.ReplaceAllString(raw, replacement)
	return os.WriteFile(path, []byte(updated), fsperm.File)
}
