package corpus

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func verifyPair(checkID string, baseline, regression CheckOutcome, signal string) string {
	if baseline.RunnerError != "" {
		return fmt.Sprintf("%s:runner-error(%s)", checkID, baseline.RunnerError)
	}
	if baseline.ExitCode != 0 {
		return fmt.Sprintf("%s:false-positive(before exit=%d, out=%s)", checkID, baseline.ExitCode, snippet(baseline.Output))
	}
	if regression.RunnerError != "" {
		return fmt.Sprintf("%s:runner-error(%s)", checkID, regression.RunnerError)
	}
	if regression.ExitCode == 0 {
		return fmt.Sprintf("%s:unexpected-miss(after exit=0, out=%s)", checkID, snippet(regression.Output))
	}
	if !strings.Contains(regression.Output, signal) {
		return fmt.Sprintf("%s:wrong-signal(expected %s in output, exit=%d, out=%s)", checkID, pyRepr(signal), regression.ExitCode, snippet(regression.Output))
	}
	return ""
}

func runRegistered(ctx context.Context, checkID string, spec runner, c Case, repoRoot string, timeout time.Duration) (CheckOutcome, CheckOutcome) {
	secs := int(timeout / time.Second)
	if secs <= 0 {
		secs = SubprocessTimeoutSecond
	}
	if spec.mode == "diff" || has(DiffChecks, checkID) {
		if spec.runDiff == nil {
			err := CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("diff runner %s missing run_diff callable", pyRepr(checkID))}
			return err, err
		}
		beforePath := filepath.Join(c.CaseDir, c.BeforeFile)
		afterPath := filepath.Join(c.CaseDir, c.AfterFile)
		baseline := spec.runDiff(ctx, c, beforePath, beforePath, repoRoot)
		if baseline.RunnerError != "" || baseline.ExitCode != 0 {
			return baseline, CheckOutcome{ExitCode: 0, Output: ""}
		}
		return baseline, spec.runDiff(ctx, c, beforePath, afterPath, repoRoot)
	}
	if spec.runSingle == nil {
		err := CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("after runner %s missing run_single callable", pyRepr(checkID))}
		return err, err
	}
	baseline := spec.runSingle(ctx, c, "before", repoRoot, secs)
	if baseline.RunnerError != "" || baseline.ExitCode != 0 {
		return baseline, CheckOutcome{ExitCode: 0, Output: ""}
	}
	return baseline, spec.runSingle(ctx, c, "after", repoRoot, secs)
}

func runnerFromExtra(spec RegisteredRunner) runner {
	return runner{
		mode: spec.Mode,
		runSingle: func(ctx context.Context, c Case, phase, repoRoot string, timeoutSecs int) Outcome {
			if spec.RunSingle == nil {
				return Outcome{}
			}
			return spec.RunSingle(ctx, c, phase, repoRoot, time.Duration(timeoutSecs)*time.Second)
		},
		runDiff: spec.RunDiff,
	}
}

// EvaluateCase runs every active check. A missing runner fails closed.
func EvaluateCase(ctx context.Context, c Case, repoRoot string, extra map[string]RegisteredRunner, timeout time.Duration) CaseEvaluation {
	if ctx == nil {
		return CaseEvaluation{CaseID: c.CaseID, Title: c.Title, Status: "fail", Detail: "runner-error(nil context)"}
	}
	active := c.Detection.ActiveChecks
	if len(active) == 0 {
		return CaseEvaluation{
			CaseID: c.CaseID,
			Title:  c.Title,
			Status: "expected-miss",
			Detail: "planned=" + strings.Join(c.Detection.PlannedChecks, ","),
		}
	}
	var failures []string
	for _, checkID := range active {
		var spec runner
		ok := false
		if extra != nil {
			if er, exists := extra[checkID]; exists {
				spec = runnerFromExtra(er)
				ok = true
			}
		}
		if !ok {
			spec, ok = builtinRunners[checkID]
		}
		if !ok {
			failures = append(failures, fmt.Sprintf("%s:runner-error(no runner registered)", checkID))
			continue
		}
		signal := c.Detection.ExpectedSignal
		if v, exists := c.Detection.ExpectedSignals[checkID]; exists {
			signal = v
		}
		baseline, regression := runRegistered(ctx, checkID, spec, c, repoRoot, timeout)
		if failure := verifyPair(checkID, baseline, regression, signal); failure != "" {
			failures = append(failures, failure)
		}
	}
	if len(failures) > 0 {
		return CaseEvaluation{CaseID: c.CaseID, Title: c.Title, Status: "fail", Detail: strings.Join(failures, "; ")}
	}
	return CaseEvaluation{CaseID: c.CaseID, Title: c.Title, Status: "caught", Detail: strings.Join(active, ",")}
}

// FormatCaptureRateLine matches the Python capture-rate summary exactly.
func FormatCaptureRateLine(evaluations []CaseEvaluation, validationErrorCount int) string {
	var caught []string
	miss, evalFail := 0, 0
	for _, ev := range evaluations {
		switch ev.Status {
		case "caught":
			caught = append(caught, ev.CaseID)
		case "expected-miss":
			miss++
		case "fail":
			evalFail++
		}
	}
	totalFailures := evalFail + validationErrorCount
	total := len(evaluations)
	pct := 0.0
	if total > 0 {
		pct = 100.0 * float64(len(caught)) / float64(total)
	}
	caughtList := "none"
	if len(caught) > 0 {
		caughtList = strings.Join(caught, ",")
	}
	return fmt.Sprintf(
		"CORPUS CAPTURE RATE: %d/%d (%.1f%%) [caught=%d: %s | expected-miss=%d | failures=%d]",
		len(caught), total, pct, len(caught), caughtList, miss, totalFailures,
	)
}
