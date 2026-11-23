package suppress

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

func checkSQLSuppressionReason(rel string, line int, codes []string, reason string) []violation {
	var sqlCodes []string
	for _, c := range codes {
		up := strings.ToUpper(c)
		if _, ok := sqlSuppressionCodes[up]; ok {
			sqlCodes = append(sqlCodes, c)
		}
	}
	if len(sqlCodes) == 0 {
		return nil
	}
	label := strings.Join(sqlCodes, ",")
	var violations []violation
	if sqlValueRE.MatchString(reason) {
		violations = append(violations, violation{
			Path: rel, Line: line, Rule: "source.sql.value-interpolation-forbidden",
			Message: fmt.Sprintf("SQL suppression (%s) forbids value interpolation; use bind parameters", label),
		})
	} else if !sqlKindRE.MatchString(reason) {
		violations = append(violations, violation{
			Path: rel, Line: line, Rule: "source.sql.missing-kind",
			Message: fmt.Sprintf("SQL suppression (%s) reason must classify kind (identifier/識別子 or not-sql/非SQL)", label),
		})
	}
	if sqlUntrustedRE.MatchString(reason) {
		violations = append(violations, violation{
			Path: rel, Line: line, Rule: "source.sql.untrusted-boundary-forbidden",
			Message: fmt.Sprintf("SQL suppression (%s) forbids untrusted/external trust boundary", label),
		})
	} else if !sqlTrustRE.MatchString(reason) {
		violations = append(violations, violation{
			Path: rel, Line: line, Rule: "source.sql.missing-trust-boundary",
			Message: fmt.Sprintf("SQL suppression (%s) reason must state trust boundary (trusted/internal/constant/allowlist/信頼境界内)", label),
		})
	}
	return violations
}

func checkBroadSuppression(rel string, line int, desc, context string, repo string, today time.Time) []violation {
	ids := adrIDs(context)
	if len(ids) == 0 {
		return []violation{{
			Path: rel, Line: line, Rule: "source.broad.missing-adr",
			Message: fmt.Sprintf("%s requires an ADR-NNNNNN reference, reason comment, and 'expires: YYYY-MM-DD'", desc),
		}}
	}
	var violations []violation
	for _, num := range ids {
		adrPath := filepath.Join(repo, "docs", "ADR", num+".md")
		if !isRegularFile(adrPath) {
			violations = append(violations, violation{
				Path: rel, Line: line, Rule: "source.broad.nonexistent-adr",
				Message: fmt.Sprintf("%s cites ADR-%s but docs/ADR/%s.md does not exist", desc, num, num),
			})
		}
	}
	if reasonLen(context) < minReasonLength {
		violations = append(violations, violation{
			Path: rel, Line: line, Rule: "source.broad.missing-reason",
			Message: fmt.Sprintf("%s requires a reason comment (>= %d non-space chars alongside ADR-NNNNNN)", desc, minReasonLength),
		})
	}
	dates := extractExpiresLoose(context)
	if len(dates) == 0 {
		violations = append(violations, violation{
			Path: rel, Line: line, Rule: "source.broad.missing-expires",
			Message: fmt.Sprintf("%s requires 'expires: YYYY-MM-DD' in comment context", desc),
		})
		return violations
	}
	earliest, furthest := dates[0], dates[0]
	for _, d := range dates[1:] {
		if d.Before(earliest) {
			earliest = d
		}
		if d.After(furthest) {
			furthest = d
		}
	}
	if earliest.Before(today) {
		violations = append(violations, violation{
			Path: rel, Line: line, Rule: "source.broad.expired",
			Message: fmt.Sprintf("%s expired on %s (today is %s)", desc, earliest.Format("2006-01-02"), today.Format("2006-01-02")),
		})
	}
	maxExpires := today.AddDate(0, 0, maxExpiresHorizonDays)
	if furthest.After(maxExpires) {
		violations = append(violations, violation{
			Path: rel, Line: line, Rule: "source.broad.expires-too-far",
			Message: fmt.Sprintf("%s 'expires' (%s) exceeds maximum horizon of %d days from %s (max %s)",
				desc, furthest.Format("2006-01-02"), maxExpiresHorizonDays, today.Format("2006-01-02"), maxExpires.Format("2006-01-02")),
		})
	}
	return violations
}

func extractPythonNoqaReason(tail string) string {
	cleaned := strings.TrimSpace(pySecondaryRE.ReplaceAllString(tail, ""))
	cleaned = strings.TrimSpace(noqaLeadRE.ReplaceAllString(cleaned, ""))
	if strings.HasPrefix(cleaned, "(") && strings.HasSuffix(cleaned, ")") && len(cleaned) >= 2 {
		cleaned = strings.TrimSpace(cleaned[1 : len(cleaned)-1])
	}
	return cleaned
}

func extractPythonNosecReason(tail string) string {
	cleaned := strings.TrimSpace(pyNosecSecondary.ReplaceAllString(tail, ""))
	cleaned = strings.TrimSpace(noqaLeadRE.ReplaceAllString(cleaned, ""))
	if strings.HasPrefix(cleaned, "(") && strings.HasSuffix(cleaned, ")") && len(cleaned) >= 2 {
		cleaned = strings.TrimSpace(cleaned[1 : len(cleaned)-1])
	}
	return cleaned
}

func pythonFileNoqaTrailingReason(rest string) string {
	stripped := strings.TrimSpace(rest)
	if strings.HasPrefix(stripped, ":") {
		after := strings.TrimSpace(stripped[1:])
		for _, sep := range []string{"#", "--", " - ", "—"} {
			if i := strings.Index(after, sep); i >= 0 {
				return strings.TrimSpace(after[i+len(sep):])
			}
		}
		return ""
	}
	return stripped
}

func splitUpperCodes(s string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, part := range strings.Split(s, ",") {
		c := strings.ToUpper(strings.TrimSpace(part))
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	sortStrings(out)
	return out
}

func codesKey(codes []string) string {
	return strings.Join(codes, "\x00")
}

func checkPythonInlineNoqa(rel string, line int, rest, srcLine string, baseline map[noqaKey]int, seen map[noqaKey]int) ([]violation, string) {
	idx := pyNoqaCodesRE.FindStringSubmatchIndex(rest)
	if idx == nil || pyTrailingBadRE.MatchString(rest) {
		return []violation{{
			Path: rel, Line: line, Rule: "source.python.noqa-missing-code",
			Message: "inline '# noqa' must specify concrete rule code(s) (e.g. '# noqa: S603')",
		}}, ""
	}
	codes := splitUpperCodes(rest[idx[2]:idx[3]])
	tail := rest[idx[3]:]
	reason := extractPythonNoqaReason(tail)
	var violations []violation
	if !hasSufficientReason(reason) {
		key := noqaKey{rel, codesKey(codes), lineFingerprintSHA256(srcLine)}
		seen[key]++
		if seen[key] > baseline[key] {
			violations = append(violations, violation{
				Path: rel, Line: line, Rule: "source.python.noqa-missing-reason",
				Message: fmt.Sprintf("inline '# noqa: %s' requires a reason comment (e.g. '-- <reason>' or '# <reason>', >= %d non-space chars)",
					strings.Join(codes, ", "), minReasonLength),
			})
		}
	}
	violations = append(violations, checkSQLSuppressionReason(rel, line, codes, reason)...)
	return violations, reason
}

func checkPythonNosec(rel string, line int, srcLine, rest, shared string, baseline map[noqaKey]int, seen map[noqaKey]int) []violation {
	idx := pyNosecCodesRE.FindStringSubmatchIndex(rest)
	if idx == nil {
		return []violation{{
			Path: rel, Line: line, Rule: "source.python.nosec-missing-code",
			Message: "inline '# nosec' must specify concrete Bandit rule code(s) (e.g. '# nosec B310 -- <reason>')",
		}}
	}
	found := bCodeRE.FindAllString(rest[idx[2]:idx[3]], -1)
	var codes []string
	seenCode := map[string]struct{}{}
	for _, c := range found {
		up := strings.ToUpper(c)
		if _, ok := seenCode[up]; ok {
			continue
		}
		seenCode[up] = struct{}{}
		codes = append(codes, up)
	}
	sortStrings(codes)
	own := extractPythonNosecReason(rest[idx[3]:])
	effective := shared
	if hasSufficientReason(own) {
		effective = own
	}
	var violations []violation
	if !hasSufficientReason(effective) {
		key := noqaKey{rel, codesKey(codes), lineFingerprintSHA256(srcLine)}
		seen[key]++
		if seen[key] > baseline[key] {
			violations = append(violations, violation{
				Path: rel, Line: line, Rule: "source.python.nosec-missing-reason",
				Message: fmt.Sprintf("inline '# nosec %s' requires a reason comment (e.g. '-- <reason>' or '# <reason>', >= %d non-space chars)",
					strings.Join(codes, " "), minReasonLength),
			})
		}
	}
	violations = append(violations, checkSQLSuppressionReason(rel, line, codes, effective)...)
	return violations
}

func checkPythonSource(rel, text, repo string, today time.Time, baseline map[noqaKey]int) []violation {
	if baseline == nil {
		baseline = map[noqaKey]int{}
	}
	comments := scanPyComments(text)
	lines := splitLines(text)
	byLine := map[int]string{}
	for _, c := range comments {
		byLine[c.line] = stripCommentDelimiters(c.raw)
	}
	seen := map[noqaKey]int{}
	var violations []violation
	for _, c := range comments {
		if m := pyFileNoqaRE.FindStringSubmatch(c.raw); m != nil {
			trailing := pythonFileNoqaTrailingReason(m[1])
			preceding := precedingComments(byLine, c.line)
			context := strings.TrimSpace(preceding + "\n" + trailing)
			violations = append(violations, checkBroadSuppression(rel, c.line, "file-level Python noqa", context, repo, today)...)
			continue
		}
		src := c.raw
		if c.line >= 1 && c.line <= len(lines) {
			src = lines[c.line-1]
		}
		shared := ""
		if m := pyInlineNoqaRE.FindStringSubmatch(c.raw); m != nil {
			extra, reason := checkPythonInlineNoqa(rel, c.line, m[1], src, baseline, seen)
			violations = append(violations, extra...)
			shared = reason
		}
		if m := pyNosecRE.FindStringSubmatch(c.raw); m != nil {
			violations = append(violations, checkPythonNosec(rel, c.line, src, m[1], shared, baseline, seen)...)
		}
	}
	return violations
}

func checkGoNolint(rel string, line int, before bool, preceding, trailing, lintersRaw, rest, repo string, today time.Time) []violation {
	var names []string
	if strings.TrimSpace(lintersRaw) != "" {
		for _, name := range strings.Split(lintersRaw, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name != "" {
				names = append(names, name)
			}
		}
	}
	for _, name := range names {
		if name == "gosec" {
			return []violation{{
				Path: rel, Line: line, Rule: "source.go.nolint-gosec-forbidden",
				Message: "use '// #nosec G<NNN> -- <reason>' instead of '//nolint:gosec' (ARCH-01 single gosec suppression format)",
			}}
		}
	}
	hasAll := false
	for _, name := range names {
		if name == "all" {
			hasAll = true
		}
	}
	if before || len(names) == 0 || hasAll {
		reasonPart := extractGoNolintReason(rest, trailing)
		combined := strings.TrimSpace(preceding + "\n" + reasonPart)
		desc := "blanket Go nolint"
		if before {
			desc = "file-level Go nolint"
		}
		return checkBroadSuppression(rel, line, desc, combined, repo, today)
	}
	reason := extractGoNolintReason(rest, trailing)
	if !hasSufficientReason(reason) {
		return []violation{{
			Path: rel, Line: line, Rule: "source.go.nolint-missing-reason",
			Message: fmt.Sprintf("//nolint:%s requires same-line '// <reason>' (>= %d non-space chars)", lintersRaw, minReasonLength),
		}}
	}
	return nil
}

func checkGoNosec(rel string, line int, rest string) []violation {
	cleaned := strings.TrimSpace(rest)
	if strings.HasSuffix(strings.TrimRightFunc(rest, unicode.IsSpace), "*/") && len(rest) >= 2 {
		cleaned = strings.TrimSpace(rest[:len(rest)-2])
	}
	if m := goNosecRulesRE.FindStringSubmatch(cleaned); m != nil {
		reason := strings.TrimSpace(m[2])
		if !hasSufficientReason(reason) {
			return []violation{{
				Path: rel, Line: line, Rule: "source.go.nosec-missing-reason",
				Message: fmt.Sprintf("#nosec requires '-- <reason>' or '// <reason>' (>= %d non-space chars)", minReasonLength),
			}}
		}
		codes := gCodeRE.FindAllString(m[1], -1)
		return checkSQLSuppressionReason(rel, line, codes, reason)
	}
	if goNosecRuleOnlyRE.MatchString(cleaned) {
		return []violation{{
			Path: rel, Line: line, Rule: "source.go.nosec-missing-reason",
			Message: fmt.Sprintf("#nosec requires '-- <reason>' or '// <reason>' (>= %d non-space chars)", minReasonLength),
		}}
	}
	return []violation{{
		Path: rel, Line: line, Rule: "source.go.nosec-missing-rule",
		Message: "#nosec must specify concrete G<NNN> rule(s) and '-- <reason>' or '// <reason>'",
	}}
}

func checkGoSource(rel, text, repo string, today time.Time) []violation {
	comments := scanGoComments(text)
	byLine, onLine := goCommentMaps(comments)
	var violations []violation
	for _, c := range comments {
		trailing := trailingSameLine(onLine, c.line, c.raw)
		if linters, rest, ok := matchGoNolint(c.raw); ok {
			violations = append(violations, checkGoNolint(rel, c.line, c.before, precedingComments(byLine, c.line), trailing, linters, rest, repo, today)...)
			continue
		}
		if m := goNosecRE.FindStringSubmatch(c.raw); m != nil {
			violations = append(violations, checkGoNosec(rel, c.line, m[1])...)
		}
	}
	return violations
}

func checkRustSource(rel, text, repo string, today time.Time) []violation {
	tokens := scanRustTokens(text)
	var violations []violation
	for idx, tok := range tokens {
		if tok.kind == "ATTR_INNER" && rustInnerAllowRE.MatchString(tok.text) {
			ctx := rustContextForAttr(tokens, idx)
			violations = append(violations, checkBroadSuppression(rel, tok.startLine, "Rust crate/module inner #![allow(...)]", ctx, repo, today)...)
		} else if tok.kind == "ATTR_OUTER" && rustOuterAllowRE.MatchString(tok.text) && rustAttrAppliesToMod(tokens, idx) {
			ctx := rustContextForAttr(tokens, idx)
			violations = append(violations, checkBroadSuppression(rel, tok.startLine, "Rust module-level #[allow(...)] on mod", ctx, repo, today)...)
		}
	}
	return violations
}

func checkTSBiomeLine(rel string, line int, rest string) []violation {
	stripped := strings.TrimSpace(rest)
	m := tsBiomeSpecRE.FindStringSubmatch(stripped)
	if m == nil {
		return []violation{{
			Path: rel, Line: line, Rule: "source.biome.ignore-missing-rule",
			Message: "biome-ignore must specify a lint rule/category and ': <reason>'",
		}}
	}
	reason := ""
	if len(m) > 2 {
		reason = strings.TrimSpace(m[2])
	}
	if !hasSufficientReason(reason) {
		return []violation{{
			Path: rel, Line: line, Rule: "source.biome.ignore-missing-reason",
			Message: fmt.Sprintf("biome-ignore requires ': <reason>' (>= %d non-space chars)", minReasonLength),
		}}
	}
	return nil
}

func checkTSBroad(rel string, line int, body, preceding, repo string, today time.Time) ([]violation, bool) {
	if rest, ok := directiveRest(body, "biome-ignore-all"); ok {
		trailing := rest
		if i := strings.Index(trailing, ":"); i >= 0 {
			trailing = trailing[i+1:]
		}
		ctx := strings.TrimSpace(preceding + "\n" + trailing)
		return checkBroadSuppression(rel, line, "file-level biome-ignore-all", ctx, repo, today), true
	}
	if rest, ok := directiveRest(body, "deno-lint-ignore-file"); ok {
		ctx := strings.TrimSpace(preceding + "\n" + rest)
		return checkBroadSuppression(rel, line, "file-level deno-lint-ignore-file", ctx, repo, today), true
	}
	if rest, ok := directiveRest(body, "eslint-disable", "-next-line", "-line"); ok {
		reasonPart := ""
		if i := strings.Index(rest, "--"); i >= 0 {
			reasonPart = rest[i+2:]
		}
		ctx := strings.TrimSpace(preceding + "\n" + reasonPart)
		return checkBroadSuppression(rel, line, "file/block-level eslint-disable", ctx, repo, today), true
	}
	return nil, false
}

func checkTSSource(rel, text, repo string, today time.Time) []violation {
	comments := scanTSComments(text, strings.HasSuffix(rel, ".svelte"))
	byLine := map[int]string{}
	for _, c := range comments {
		if c.line == c.endLine {
			byLine[c.line] = stripCommentDelimiters(c.raw)
		}
	}
	var violations []violation
	for _, c := range comments {
		body := stripCommentDelimiters(c.raw)
		preceding := precedingComments(byLine, c.line)
		if extra, ok := checkTSBroad(rel, c.line, body, preceding, repo, today); ok {
			violations = append(violations, extra...)
			continue
		}
		if rest, ok := directiveRest(body, "biome-ignore", "-all"); ok {
			violations = append(violations, checkTSBiomeLine(rel, c.line, rest)...)
			continue
		}
		if rest, ok := directiveRest(body, "deno-lint-ignore", "-file"); ok {
			if !tsDenoRuleRE.MatchString(strings.TrimSpace(rest)) {
				violations = append(violations, violation{
					Path: rel, Line: c.line, Rule: "source.deno.ignore-missing-rule",
					Message: "deno-lint-ignore must specify at least one concrete rule name",
				})
			}
		}
	}
	return violations
}

func checkSourceFile(rel, text, repo string, today time.Time, baseline map[noqaKey]int) []violation {
	if isExcludedSourcePath(rel) {
		return nil
	}
	switch {
	case strings.HasSuffix(rel, ".py"):
		return checkPythonSource(rel, text, repo, today, baseline)
	case strings.HasSuffix(rel, ".go"):
		return checkGoSource(rel, text, repo, today)
	case strings.HasSuffix(rel, ".rs"):
		return checkRustSource(rel, text, repo, today)
	case hasSuffixAny(rel, tsExts):
		return checkTSSource(rel, text, repo, today)
	default:
		return nil
	}
}
