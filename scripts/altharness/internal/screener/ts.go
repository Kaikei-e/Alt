package screener

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	tsWaitTimeoutRE = regexp.MustCompile(`\.\s*waitForTimeout\s*\(([^)]*)\)`)
	tsWaitLoadRE    = regexp.MustCompile(`\.\s*waitForLoadState\s*\(([^)]*)\)`)
	svelteScriptRE  = regexp.MustCompile(`(?i)<script\b`)
)

func screenTypeScript(before, after, logical string) []Candidate {
	bClean := stripCStyle(prepareSvelte(logical, before), false)
	aClean := stripCStyle(prepareSvelte(logical, after), false)
	bTimeouts, bLoads := extractTSWaits(bClean)
	aTimeouts, aLoads := extractTSWaits(aClean)
	dropped := len(aTimeouts) < len(bTimeouts) || (!sameStringSlice(bTimeouts, aTimeouts) && len(bTimeouts) > 0)
	replaced := len(aTimeouts) < len(bTimeouts) && len(aLoads) > len(bLoads)
	if !dropped && !replaced {
		return nil
	}
	beforeText := fmt.Sprintf("waitForTimeout=%d (%s), waitForLoadState=%d", len(bTimeouts), pyList(bTimeouts), len(bLoads))
	afterText := fmt.Sprintf("waitForTimeout=%d (%s), waitForLoadState=%d", len(aTimeouts), pyList(aTimeouts), len(aLoads))
	return []Candidate{makeCandidate(logical, "<file>", "ts.playwright_wait_change", beforeText, afterText)}
}

func extractTSWaits(cleaned string) (timeouts, loads []string) {
	for _, m := range tsWaitTimeoutRE.FindAllStringSubmatch(cleaned, -1) {
		timeouts = append(timeouts, strings.Join(strings.Fields(m[1]), " "))
	}
	for _, m := range tsWaitLoadRE.FindAllStringSubmatch(cleaned, -1) {
		loads = append(loads, strings.Join(strings.Fields(m[1]), " "))
	}
	return timeouts, loads
}

func pyList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = "'" + strings.ReplaceAll(item, "'", `\'`) + "'"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func prepareSvelte(path, src string) string {
	if !strings.EqualFold(filepath.Ext(path), ".svelte") {
		return src
	}
	extracted, ok := extractSvelteScripts(src)
	if !ok {
		return src
	}
	return extracted
}

func extractSvelteScripts(src string) (string, bool) {
	if !svelteScriptRE.MatchString(src) {
		return "", false
	}
	var b strings.Builder
	found := false
	i := 0
	for i < len(src) {
		loc := svelteScriptRE.FindStringIndex(src[i:])
		if loc == nil {
			break
		}
		j := i + loc[1]
		quote := byte(0)
		selfClose := false
		for j < len(src) {
			c := src[j]
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				j++
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				j++
				continue
			}
			if c == '>' {
				if j > 0 && src[j-1] == '/' {
					selfClose = true
				}
				j++
				break
			}
			j++
		}
		if selfClose {
			i = j
			continue
		}
		end := findScriptClose(src, j)
		if end < 0 {
			if !found {
				return "", false
			}
			break
		}
		if found {
			b.WriteByte('\n')
		}
		b.WriteString(src[j:end])
		found = true
		i = end + len("</script>")
	}
	if !found {
		return "", false
	}
	return b.String(), true
}

func findScriptClose(src string, i int) int {
	const closing = "</script>"
	for i < len(src) {
		if end, ok := scanStringLiteralEnd(src, i, "typescript"); ok {
			i = end
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			if i+1 < len(src) {
				i += 2
			}
			continue
		}
		if i+len(closing) <= len(src) && strings.EqualFold(src[i:i+len(closing)], closing) {
			return i
		}
		i++
	}
	return -1
}
