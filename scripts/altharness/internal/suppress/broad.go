package suppress

import (
	"strings"
	"time"
)

type broadRecord struct {
	line int
	desc string
	adrs []string
	max  *time.Time
}

func makeBroadRecord(line int, desc, context string) broadRecord {
	dates := extractExpiresLoose(context)
	rec := broadRecord{line: line, desc: desc, adrs: adrIDs(context)}
	if len(dates) > 0 {
		furthest := dates[0]
		for _, d := range dates[1:] {
			if d.After(furthest) {
				furthest = d
			}
		}
		rec.max = &furthest
	}
	return rec
}

func collectBroad(rel, text string) []broadRecord {
	if isExcludedSourcePath(rel) {
		return nil
	}
	switch {
	case strings.HasSuffix(rel, ".py"):
		return collectBroadPy(text)
	case strings.HasSuffix(rel, ".go"):
		return collectBroadGo(text)
	case strings.HasSuffix(rel, ".rs"):
		return collectBroadRust(text)
	case hasSuffixAny(rel, tsExts):
		return collectBroadTS(rel, text)
	default:
		return nil
	}
}

func collectBroadPy(text string) []broadRecord {
	comments := scanPyComments(text)
	byLine := map[int]string{}
	for _, c := range comments {
		byLine[c.line] = stripCommentDelimiters(c.raw)
	}
	var records []broadRecord
	for _, c := range comments {
		m := pyFileNoqaRE.FindStringSubmatch(c.raw)
		if m == nil {
			continue
		}
		trailing := pythonFileNoqaTrailingReason(m[1])
		preceding := precedingComments(byLine, c.line)
		records = append(records, makeBroadRecord(c.line, "file-level Python noqa", strings.TrimSpace(preceding+"\n"+trailing)))
	}
	return records
}

func collectBroadGo(text string) []broadRecord {
	comments := scanGoComments(text)
	byLine, onLine := goCommentMaps(comments)
	var records []broadRecord
	for _, c := range comments {
		lintersRaw, rest, ok := matchGoNolint(c.raw)
		if !ok {
			continue
		}
		var names []string
		if strings.TrimSpace(lintersRaw) != "" {
			for _, name := range strings.Split(lintersRaw, ",") {
				name = strings.ToLower(strings.TrimSpace(name))
				if name != "" {
					names = append(names, name)
				}
			}
		}
		if containsStr(names, "gosec") {
			continue
		}
		if c.before || len(names) == 0 || containsStr(names, "all") {
			trailing := trailingSameLine(onLine, c.line, c.raw)
			reason := extractGoNolintReason(rest, trailing)
			preceding := precedingComments(byLine, c.line)
			desc := "blanket Go nolint"
			if c.before {
				desc = "file-level Go nolint"
			}
			records = append(records, makeBroadRecord(c.line, desc, strings.TrimSpace(preceding+"\n"+reason)))
		}
	}
	return records
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func collectBroadRust(text string) []broadRecord {
	tokens := scanRustTokens(text)
	var records []broadRecord
	for idx, tok := range tokens {
		if tok.kind == "ATTR_INNER" && rustInnerAllowRE.MatchString(tok.text) {
			records = append(records, makeBroadRecord(tok.startLine, "Rust crate/module inner #![allow(...)]", rustContextForAttr(tokens, idx)))
		} else if tok.kind == "ATTR_OUTER" && rustOuterAllowRE.MatchString(tok.text) && rustAttrAppliesToMod(tokens, idx) {
			records = append(records, makeBroadRecord(tok.startLine, "Rust module-level #[allow(...)] on mod", rustContextForAttr(tokens, idx)))
		}
	}
	return records
}

func collectBroadTS(rel, text string) []broadRecord {
	comments := scanTSComments(text, strings.HasSuffix(rel, ".svelte"))
	byLine := map[int]string{}
	for _, c := range comments {
		if c.line == c.endLine {
			byLine[c.line] = stripCommentDelimiters(c.raw)
		}
	}
	var records []broadRecord
	for _, c := range comments {
		body := stripCommentDelimiters(c.raw)
		preceding := precedingComments(byLine, c.line)
		if rest, ok := directiveRest(body, "biome-ignore-all"); ok {
			trailing := rest
			if i := strings.Index(trailing, ":"); i >= 0 {
				trailing = trailing[i+1:]
			}
			records = append(records, makeBroadRecord(c.line, "file-level biome-ignore-all", strings.TrimSpace(preceding+"\n"+trailing)))
			continue
		}
		if rest, ok := directiveRest(body, "deno-lint-ignore-file"); ok {
			records = append(records, makeBroadRecord(c.line, "file-level deno-lint-ignore-file", strings.TrimSpace(preceding+"\n"+rest)))
			continue
		}
		if rest, ok := directiveRest(body, "eslint-disable", "-next-line", "-line"); ok {
			reason := ""
			if i := strings.Index(rest, "--"); i >= 0 {
				reason = rest[i+2:]
			}
			records = append(records, makeBroadRecord(c.line, "file/block-level eslint-disable", strings.TrimSpace(preceding+"\n"+reason)))
		}
	}
	return records
}

func compareBroadGroup(rel string, baseGroup, curGroup []broadRecord, changed map[string]struct{}) []violation {
	if len(baseGroup) == len(curGroup) {
		var violations []violation
		for i := range curGroup {
			prev, cur := baseGroup[i], curGroup[i]
			if prev.max != nil && cur.max != nil && cur.max.After(*prev.max) && !anyADRChanged(cur.adrs, changed) {
				violations = append(violations, violation{
					Path: rel, Line: cur.line, Rule: "source.broad.expires-extended-without-adr-update",
					Message: "extended",
				})
				violations[len(violations)-1].Message = broadExtendMessage(cur.desc, *prev.max, *cur.max)
			}
		}
		return violations
	}
	var baseDates []time.Time
	for _, rec := range baseGroup {
		if rec.max != nil {
			baseDates = append(baseDates, *rec.max)
		}
	}
	if len(baseDates) == 0 {
		return nil
	}
	baseMax := baseDates[0]
	for _, d := range baseDates[1:] {
		if d.After(baseMax) {
			baseMax = d
		}
	}
	var violations []violation
	for _, cur := range curGroup {
		if cur.max != nil && cur.max.After(baseMax) && !anyADRChanged(cur.adrs, changed) {
			violations = append(violations, violation{
				Path: rel, Line: cur.line, Rule: "source.broad.expires-extended-without-adr-update",
				Message: broadExtendMessage(cur.desc, baseMax, *cur.max),
			})
		}
	}
	return violations
}

func broadExtendMessage(desc string, prev, cur time.Time) string {
	return desc + " extended 'expires' from " + prev.Format("2006-01-02") + " to " + cur.Format("2006-01-02") + " without modifying or adding a cited docs/ADR/NNNNNN.md in the same diff"
}

func anyADRChanged(ids []string, changed map[string]struct{}) bool {
	for _, id := range ids {
		if _, ok := changed[id]; ok {
			return true
		}
	}
	return false
}
