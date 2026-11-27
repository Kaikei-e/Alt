package loopeval

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/guards"
)

type compareOptions struct {
	RepoRoot        string
	BeforePath      string
	AfterPath       string
	MinTrials       int
	TemplateVersion string
	Branch          string
	DecisionsFile   string
	RunsDir         string
	Record          bool
}

const compareEpsilon = 1e-9

func compareGradeSets(opt compareOptions) (CompareReport, int, error) {
	beforeRes, err := loadGradeResults(opt.BeforePath)
	if err != nil {
		return CompareReport{}, exitUsage, fmt.Errorf("load --before %s: %w", opt.BeforePath, err)
	}
	afterRes, err := loadGradeResults(opt.AfterPath)
	if err != nil {
		return CompareReport{}, exitUsage, fmt.Errorf("load --after %s: %w", opt.AfterPath, err)
	}

	rep := CompareReport{
		TemplateVersion: opt.TemplateVersion,
		MinTrials:       opt.MinTrials,
		BeforeSource:    opt.BeforePath,
		AfterSource:     opt.AfterPath,
		Scenarios:       []ScenarioComparison{},
		Improvements:    []string{},
		Regressions:     []string{},
		Errors:          []Issue{},
	}

	beforeByID := map[string][]GradeResult{}
	afterByID := map[string][]GradeResult{}
	scenarioSet := map[string]struct{}{}

	for idx, r := range beforeRes {
		if strings.TrimSpace(r.ScenarioID) == "" || r.MaxScore <= 0 || r.Score < 0 || r.Score > r.MaxScore {
			rep.Errors = append(rep.Errors, Issue{
				Code:    "INVALID_GRADE_RECORD",
				Path:    fmt.Sprintf("before[%d]", idx),
				Message: fmt.Sprintf("invalid grade record (scenario_id=%q score=%d max_score=%d)", r.ScenarioID, r.Score, r.MaxScore),
			})
			continue
		}
		beforeByID[r.ScenarioID] = append(beforeByID[r.ScenarioID], r)
		scenarioSet[r.ScenarioID] = struct{}{}
	}
	for idx, r := range afterRes {
		if strings.TrimSpace(r.ScenarioID) == "" || r.MaxScore <= 0 || r.Score < 0 || r.Score > r.MaxScore {
			rep.Errors = append(rep.Errors, Issue{
				Code:    "INVALID_GRADE_RECORD",
				Path:    fmt.Sprintf("after[%d]", idx),
				Message: fmt.Sprintf("invalid grade record (scenario_id=%q score=%d max_score=%d)", r.ScenarioID, r.Score, r.MaxScore),
			})
			continue
		}
		afterByID[r.ScenarioID] = append(afterByID[r.ScenarioID], r)
		scenarioSet[r.ScenarioID] = struct{}{}
	}

	if len(scenarioSet) == 0 {
		rep.Errors = append(rep.Errors, Issue{
			Code:    "EMPTY_EVAL_SET",
			Path:    "$",
			Message: "no grade results found in --before or --after",
		})
		return rep, exitFail, nil
	}

	ids := make([]string, 0, len(scenarioSet))
	for id := range scenarioSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		ni, nj := scenarioNumericID(ids[i]), scenarioNumericID(ids[j])
		if ni != nj {
			return ni < nj
		}
		return ids[i] < ids[j]
	})

	for _, id := range ids {
		bCount := len(beforeByID[id])
		aCount := len(afterByID[id])
		if bCount < opt.MinTrials || aCount < opt.MinTrials {
			rep.Errors = append(rep.Errors, Issue{
				Code:    "INSUFFICIENT_TRIALS",
				Path:    id,
				Message: fmt.Sprintf("scenario %s requires at least %d trials in both before (got %d) and after (got %d)", id, opt.MinTrials, bCount, aCount),
			})
		}
	}
	if len(rep.Errors) > 0 {
		rep.Valid = false
		return rep, exitFail, nil
	}

	var sumBeforePass, sumAfterPass float64
	var sumBeforeScore, sumAfterScore float64
	var sumBeforeFid, sumAfterFid float64
	var cntBeforeFid, cntAfterFid int

	for _, id := range ids {
		bs := beforeByID[id]
		as := afterByID[id]
		bPass, bScore, bFid := summarizeTrials(bs)
		aPass, aScore, aFid := summarizeTrials(as)

		scCmp := ScenarioComparison{
			ScenarioID:              id,
			BeforeTrials:            len(bs),
			AfterTrials:             len(as),
			BeforePassRate:          round4(bPass),
			AfterPassRate:           round4(aPass),
			DeltaPassRate:           round4(aPass - bPass),
			BeforeMeanScore:         round4(bScore),
			AfterMeanScore:          round4(aScore),
			DeltaMeanScore:          round4(aScore - bScore),
			BeforeReadAuditFidelity: roundPtr4(bFid),
			AfterReadAuditFidelity:  roundPtr4(aFid),
		}
		if bFid != nil && aFid != nil {
			d := round4(*aFid - *bFid)
			scCmp.DeltaReadAuditFidelity = &d
		}

		regressed := (aPass < bPass-compareEpsilon) ||
			(aScore < bScore-compareEpsilon) ||
			(bFid != nil && aFid != nil && *aFid < *bFid-compareEpsilon)
		improved := !regressed && ((aPass > bPass+compareEpsilon) ||
			(aScore > bScore+compareEpsilon) ||
			(bFid != nil && aFid != nil && *aFid > *bFid+compareEpsilon))

		switch {
		case regressed:
			scCmp.Status = "regressed"
			rep.Regressions = append(rep.Regressions, id)
		case improved:
			scCmp.Status = "improved"
			rep.Improvements = append(rep.Improvements, id)
		default:
			scCmp.Status = "no_change"
		}
		rep.Scenarios = append(rep.Scenarios, scCmp)

		sumBeforePass += bPass
		sumAfterPass += aPass
		sumBeforeScore += bScore
		sumAfterScore += aScore
		if bFid != nil {
			sumBeforeFid += *bFid
			cntBeforeFid++
		}
		if aFid != nil {
			sumAfterFid += *aFid
			cntAfterFid++
		}
	}

	n := float64(len(ids))
	rep.BeforePassRate = round4(sumBeforePass / n)
	rep.AfterPassRate = round4(sumAfterPass / n)
	rep.DeltaPassRate = round4(rep.AfterPassRate - rep.BeforePassRate)
	rep.BeforeMeanScore = round4(sumBeforeScore / n)
	rep.AfterMeanScore = round4(sumAfterScore / n)
	rep.DeltaMeanScore = round4(rep.AfterMeanScore - rep.BeforeMeanScore)
	if cntBeforeFid > 0 {
		v := round4(sumBeforeFid / float64(cntBeforeFid))
		rep.BeforeReadAuditFidelity = &v
	}
	if cntAfterFid > 0 {
		v := round4(sumAfterFid / float64(cntAfterFid))
		rep.AfterReadAuditFidelity = &v
	}
	if rep.BeforeReadAuditFidelity != nil && rep.AfterReadAuditFidelity != nil {
		d := round4(*rep.AfterReadAuditFidelity - *rep.BeforeReadAuditFidelity)
		rep.DeltaReadAuditFidelity = &d
	}

	switch {
	case len(rep.Regressions) > 0:
		rep.Verdict = "regressed"
	case len(rep.Improvements) > 0:
		rep.Verdict = "improved"
	default:
		rep.Verdict = "no_change"
	}
	rep.Valid = true

	if opt.Record {
		artifactPath, decPath, recErr := recordComparison(opt, &rep)
		if recErr != nil {
			return rep, exitUsage, fmt.Errorf("record comparison: %w", recErr)
		}
		rep.RecordedArtifact = artifactPath
		rep.DecisionsFile = decPath
	}

	if rep.Verdict == "regressed" {
		return rep, exitFail, nil
	}
	return rep, exitOK, nil
}

func loadGradeResults(path string) ([]GradeResult, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(strings.ToLower(ent.Name()), ".json") {
				continue
			}
			names = append(names, ent.Name())
		}
		sort.Strings(names)
		var out []GradeResult
		for _, name := range names {
			full := filepath.Join(path, name)
			if !isRegularNonSymlink(full) {
				return nil, fmt.Errorf("refusing non-regular or symlink file: %s", full)
			}
			items, err := parseGradeFile(full)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			out = append(out, items...)
		}
		return out, nil
	}
	if !isRegularNonSymlink(path) {
		return nil, fmt.Errorf("refusing symlink or non-regular file: %s", path)
	}
	return parseGradeFile(path)
}

func parseGradeFile(path string) ([]GradeResult, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := bytesTrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty JSON file")
	}
	if trimmed[0] == '[' {
		var list []GradeResult
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	if trimmed[0] == '{' {
		var wrapper struct {
			Results []GradeResult `json:"results"`
		}
		if err := json.Unmarshal(trimmed, &wrapper); err == nil && wrapper.Results != nil {
			return wrapper.Results, nil
		}
		var single GradeResult
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return nil, err
		}
		return []GradeResult{single}, nil
	}
	return nil, fmt.Errorf("expected JSON array or object")
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func summarizeTrials(trials []GradeResult) (passRate float64, meanScore float64, readFidelity *float64) {
	if len(trials) == 0 {
		return 0, 0, nil
	}
	passes := 0
	totalScore := 0.0
	fidSum := 0.0
	fidCount := 0
	for _, t := range trials {
		if t.Passed {
			passes++
		}
		totalScore += float64(t.Score)
		if t.Signals.TranscriptReadAudit.Evaluated {
			fidSum += t.Signals.TranscriptReadAudit.Fidelity
			fidCount++
		}
	}
	n := float64(len(trials))
	passRate = float64(passes) / n
	meanScore = totalScore / n
	if fidCount > 0 {
		v := fidSum / float64(fidCount)
		readFidelity = &v
	}
	return passRate, meanScore, readFidelity
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func roundPtr4(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := round4(*p)
	return &v
}

func recordComparison(opt compareOptions, rep *CompareReport) (string, string, error) {
	var runDir string
	if strings.TrimSpace(opt.RunsDir) != "" {
		safeBranch := guards.SanitizeBranchName(guards.ResolveGitBranch(opt.RepoRoot, opt.Branch))
		runDir = filepath.Join(opt.RunsDir, safeBranch)
		if !withinDir(runDir, opt.RunsDir) {
			return "", "", fmt.Errorf("run directory escapes --runs-dir: %s", runDir)
		}
	} else {
		var err error
		runDir, err = guards.RunDirForBranch(opt.RepoRoot, opt.Branch)
		if err != nil {
			return "", "", err
		}
	}

	evalDir := filepath.Join(runDir, "eval")
	if isSymlink(runDir) || isSymlink(evalDir) {
		return "", "", fmt.Errorf("refusing symlink in eval artifact directory: %s", evalDir)
	}
	if err := os.MkdirAll(evalDir, fsperm.Dir); err != nil {
		return "", "", err
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	artifactPath := filepath.Join(evalDir, "compare-"+ts+".json")
	for suffix := 1; ; suffix++ {
		if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
			break
		}
		artifactPath = filepath.Join(evalDir, fmt.Sprintf("compare-%s-%d.json", ts, suffix))
	}

	decisionsPath := strings.TrimSpace(opt.DecisionsFile)
	if decisionsPath == "" {
		decisionsPath = filepath.Join(runDir, "decisions.tsv")
	}

	rep.RecordedArtifact = artifactPath
	rep.DecisionsFile = decisionsPath
	encoded, err := marshalJSON(rep)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(artifactPath, encoded, fsperm.File); err != nil {
		return "", "", err
	}

	evidence := artifactPath
	if resolvedRepo, err := resolvePath(opt.RepoRoot); err == nil {
		if resolvedArt, err := resolvePath(artifactPath); err == nil && withinDir(resolvedArt, resolvedRepo) {
			if rel, err := filepath.Rel(resolvedRepo, resolvedArt); err == nil {
				evidence = filepath.ToSlash(rel)
			}
		}
	}

	tplLabel := strings.TrimSpace(opt.TemplateVersion)
	if tplLabel == "" {
		tplLabel = "unspecified"
	}
	decisionText := fmt.Sprintf(
		"template=%s; verdict=%s; scenarios=%d; min_trials=%d; delta_pass_rate=%+.4f; delta_mean_score=%+.4f",
		sanitizeSingleLine(tplLabel),
		rep.Verdict,
		len(rep.Scenarios),
		rep.MinTrials,
		rep.DeltaPassRate,
		rep.DeltaMeanScore,
	)
	whyText := fmt.Sprintf(
		"LOOP-05 before/after eval comparison (improved=%d, regressed=%d)",
		len(rep.Improvements),
		len(rep.Regressions),
	)
	if err := guards.AppendDecisionToFile(decisionsPath, "loop-eval", decisionText, whyText, evidence, "info"); err != nil {
		return artifactPath, decisionsPath, err
	}
	return artifactPath, decisionsPath, nil
}

func sanitizeSingleLine(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
