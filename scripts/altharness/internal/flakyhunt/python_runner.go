package flakyhunt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"alt/scripts/altharness/internal/pytestrand"
)

var (
	pySeedLineRE       = regexp.MustCompile(`^PYTEST_RANDOM_SEED=(\d+)\s*$`)
	pySummaryFailedRE  = regexp.MustCompile(`^FAILED\s+(\S+)`)
	pyProgressFailedRE = regexp.MustCompile(`^(\S+::\S+)\s+FAILED\b`)
	pyErrorLineRE      = regexp.MustCompile(`^ERROR\s+(?:collecting\s+)?(\S+)`)
)

func runPythonHunt(ctx context.Context, cfg config, relModule, modDir, evidenceDir string, baseEnv map[string]string, rep *Report) error {
	for round := 1; round <= cfg.rounds; round++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		seed, err := resolveRoundSeed(cfg.seed)
		if err != nil {
			return fmt.Errorf("round %d seed generation failed: %w", round, err)
		}
		seedStr := strconv.FormatInt(seed, 10)

		roundStart := time.Now()
		var combinedLog strings.Builder
		roundExitCode := 0
		var roundFailures []Failure

		for iter := 1; iter <= cfg.count; iter++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			args := []string{
				"--seed", seedStr,
				"--pytest-cmd", cfg.pytestCmd,
				modDir,
				"--",
				"-o", "addopts=",
				"-v",
				"--tb=short",
			}
			if cfg.pkgExplicit {
				args = append(args, cfg.pkgs...)
			}
			args = append(args, cfg.extraArgs...)

			var outBuf, errBuf bytes.Buffer
			rc := pytestrand.RunWithOptions(ctx, args, pytestrand.Options{
				Stdout: &outBuf,
				Stderr: &errBuf,
				Env:    baseEnv,
			})

			stdoutStr := outBuf.String()
			stderrStr := errBuf.String()
			if captured := extractPythonSeed(stdoutStr); captured != "" {
				seedStr = captured
			}

			if rc != 0 {
				if roundExitCode == 0 {
					roundExitCode = rc
				}
				combinedLog.WriteString(stdoutStr)
				if stderrStr != "" {
					combinedLog.WriteString("--- STDERR ---\n")
					combinedLog.WriteString(stderrStr)
				}
				repro := buildPythonReproCommand(relModule, seedStr, cfg)
				extracted := parsePytestFailures(stdoutStr, stderrStr, round, relModule, seedStr, repro)
				if len(extracted) == 0 {
					extracted = append(extracted, Failure{
						Round:        round,
						Seed:         seedStr,
						TestName:     "<pytest>",
						Package:      relModule,
						Kind:         "exit_error",
						ReproCommand: repro,
						Snippet:      trimSnippet(stdoutStr + "\n" + stderrStr),
					})
				}
				roundFailures = append(roundFailures, extracted...)
			}
		}

		summary := RoundSummary{
			Round:      round,
			ExitCode:   roundExitCode,
			Passed:     roundExitCode == 0 && len(roundFailures) == 0,
			DurationMS: time.Since(roundStart).Milliseconds(),
			Seed:       seedStr,
		}

		if !summary.Passed {
			rep.FailedRounds++
			logPath, lerr := writeFailureLog(evidenceDir, round, combinedLog.String())
			if lerr != nil {
				return fmt.Errorf("write failure log for round %d: %w", round, lerr)
			}
			summary.LogPath = logPath
			for i := range roundFailures {
				roundFailures[i].LogPath = logPath
				rep.Failures = append(rep.Failures, roundFailures[i])
			}
		} else {
			rep.PassedRounds++
		}
		rep.RoundResults = append(rep.RoundResults, summary)
	}
	return nil
}

func resolveRoundSeed(requested string) (int64, error) {
	if requested != "" && requested != "on" {
		return strconv.ParseInt(requested, 10, 64)
	}
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint32(buf[:])), nil
}

func extractPythonSeed(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if m := pySeedLineRE.FindStringSubmatch(strings.TrimSpace(line)); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

func parsePytestFailures(stdout, stderr string, round int, relModule, seed, repro string) []Failure {
	seen := make(map[string]bool)
	var out []Failure

	add := func(nodeID, kind, line string) {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" || seen[nodeID] {
			return
		}
		seen[nodeID] = true
		pkg := relModule
		if filePart, _, ok := strings.Cut(nodeID, "::"); ok && filePart != "" {
			pkg = filePart
		}
		out = append(out, Failure{
			Round:        round,
			Seed:         seed,
			TestName:     nodeID,
			Package:      pkg,
			Kind:         kind,
			ReproCommand: repro,
			Snippet:      trimSnippet(line),
		})
	}

	for _, text := range []string{stdout, stderr} {
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if m := pySummaryFailedRE.FindStringSubmatch(trimmed); len(m) == 2 {
				add(m[1], "test_fail", trimmed)
				continue
			}
			if m := pyProgressFailedRE.FindStringSubmatch(trimmed); len(m) == 2 {
				add(m[1], "test_fail", trimmed)
				continue
			}
			if m := pyErrorLineRE.FindStringSubmatch(trimmed); len(m) == 2 {
				add(m[1], "test_fail", trimmed)
			}
		}
	}
	return out
}

func buildPythonReproCommand(relModule, seed string, cfg config) string {
	parts := []string{"./scripts/altharness/alt-harness", "pytest-rand", "--seed", seed}
	if cfg.pytestCmd != "" && cfg.pytestCmd != defaultPytestCmd {
		parts = append(parts, "--pytest-cmd", strconv.Quote(cfg.pytestCmd))
	}
	parts = append(parts, relModule, "--", "-o", "addopts=", "-v", "--tb=short")
	if cfg.pkgExplicit {
		parts = append(parts, cfg.pkgs...)
	}
	parts = append(parts, cfg.extraArgs...)
	return strings.Join(parts, " ")
}
