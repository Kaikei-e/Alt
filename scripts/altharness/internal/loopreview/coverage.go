package loopreview

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var loop04FocusDecisionRE = regexp.MustCompile(`^(?:focus\s+)?(F-0[1-7])(?:\s|\(|:|/|$)`)

// CheckCoverage reads decisionsPath and verifies all 7 canonical LOOP-04 focus
// areas (F-01..F-07) have at least one phase="loop-04" verdict row recorded.
func CheckCoverage(decisionsPath string) CoverageResult {
	canonical := CanonicalFocusAreas()
	res := CoverageResult{
		DecisionsFile: decisionsPath,
		TotalRequired: len(canonical),
		CoveredIDs:    []string{},
		MissingIDs:    []string{},
		MissingItems:  []FocusItem{},
	}

	info, err := os.Lstat(decisionsPath)
	if err != nil {
		res.Error = fmt.Sprintf("decisions file not found: %s", decisionsPath)
		fillMissing(&res, canonical, map[string]struct{}{})
		return res
	}
	if info.Mode()&os.ModeSymlink != 0 {
		res.Error = fmt.Sprintf("decisions file must not be a symlink: %s", decisionsPath)
		fillMissing(&res, canonical, map[string]struct{}{})
		return res
	}
	if !info.Mode().IsRegular() {
		res.Error = fmt.Sprintf("decisions path is not a regular file: %s", decisionsPath)
		fillMissing(&res, canonical, map[string]struct{}{})
		return res
	}

	raw, err := readRegularFileSafe(decisionsPath)
	if err != nil {
		res.Error = fmt.Sprintf("read decisions file %s: %v", decisionsPath, err)
		fillMissing(&res, canonical, map[string]struct{}{})
		return res
	}

	coveredSet := map[string]struct{}{}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for idx, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if idx == 0 && strings.TrimSpace(line) == "ts\tphase\tdecision\twhy\tevidence\tresult" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		if strings.TrimSpace(parts[1]) != "loop-04" {
			continue
		}
		decision := strings.TrimSpace(parts[2])
		if m := loop04FocusDecisionRE.FindStringSubmatch(decision); m != nil {
			coveredSet[m[1]] = struct{}{}
		}
	}

	fillMissing(&res, canonical, coveredSet)
	return res
}

func fillMissing(res *CoverageResult, canonical []FocusItem, coveredSet map[string]struct{}) {
	for _, item := range canonical {
		if _, ok := coveredSet[item.ID]; ok {
			res.CoveredIDs = append(res.CoveredIDs, item.ID)
		} else {
			res.MissingIDs = append(res.MissingIDs, item.ID)
			res.MissingItems = append(res.MissingItems, item)
		}
	}
	res.CoveredCount = len(res.CoveredIDs)
	res.Covered = len(res.MissingIDs) == 0 && res.Error == ""
}
