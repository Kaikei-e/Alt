package loopreview

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"alt/scripts/altharness/internal/guards"
)

var loop04FocusDecisionRE = regexp.MustCompile(`^(?:focus\s+)?(F-0[1-7])\s*\((safe|fixed|deferred)\)(?:\s|:|$)`)

// CheckCoverage reads decisionsPath and verifies all 7 canonical LOOP-04 focus
// areas (F-01..F-07) have a valid phase="loop-04" info verdict row recorded
// (safe, fixed, or deferred).
func CheckCoverage(decisionsPath string) CoverageResult {
	return CheckCoverageWithOptions(decisionsPath, CoverageOptions{})
}

// CheckCoverageWithOptions validates decisionsPath fail-closed via
// guards.ValidateDecisionsFile and checks LOOP-04 coverage across F-01..F-07.
// Only phase="loop-04" rows with result="info" are counted. By default
// safe|fixed|deferred count as covered; when opts.StrictResolved is true only
// safe|fixed count as covered.
func CheckCoverageWithOptions(decisionsPath string, opts CoverageOptions) CoverageResult {
	canonical := CanonicalFocusAreas()
	res := CoverageResult{
		StrictResolved: opts.StrictResolved,
		DecisionsFile:  decisionsPath,
		TotalRequired:  len(canonical),
		CoveredIDs:     []string{},
		MissingIDs:     []string{},
		MissingItems:   []FocusItem{},
	}

	if err := ensureNoSymlinkPath(decisionsPath); err != nil {
		res.Error = err.Error()
		fillMissing(&res, canonical, map[string]struct{}{})
		return res
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

	_, findings, verr := guards.ValidateDecisionsFile(decisionsPath)
	if verr != nil {
		res.Error = fmt.Sprintf("validate decisions file %s: %v", decisionsPath, verr)
		fillMissing(&res, canonical, map[string]struct{}{})
		return res
	}
	if len(findings) > 0 {
		first := findings[0]
		res.Error = fmt.Sprintf("invalid decisions.tsv (%d issue(s), first at line %d [%s]): %s",
			len(findings), first.Line, first.Code, first.Message)
		fillMissing(&res, canonical, map[string]struct{}{})
		return res
	}

	raw, err := readRegularFileSafe(decisionsPath)
	if err != nil {
		res.Error = fmt.Sprintf("read decisions file %s: %v", decisionsPath, err)
		fillMissing(&res, canonical, map[string]struct{}{})
		return res
	}

	latestStatus := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for idx, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if idx == 0 && strings.TrimSpace(line) == "ts\tphase\tdecision\twhy\tevidence\tresult" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 6 {
			continue
		}
		if strings.TrimSpace(parts[1]) != "loop-04" {
			continue
		}
		if strings.TrimSpace(parts[5]) != "info" {
			continue
		}
		decision := strings.TrimSpace(parts[2])
		if m := loop04FocusDecisionRE.FindStringSubmatch(decision); m != nil {
			latestStatus[m[1]] = m[2]
		}
	}

	coveredSet := map[string]struct{}{}
	for fid, status := range latestStatus {
		if status == "safe" || status == "fixed" || (!opts.StrictResolved && status == "deferred") {
			coveredSet[fid] = struct{}{}
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
