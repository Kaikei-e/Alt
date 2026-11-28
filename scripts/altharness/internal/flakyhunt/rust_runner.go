package flakyhunt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	rustTestFailedRE = regexp.MustCompile(`^(?:test\s+(\S+)\s+\.\.\.\s+FAILED|(\S+)\s+---\s+FAILED)\s*$`)
	rustPanicRE      = regexp.MustCompile(`^thread\s+'([^']+)'(?:\s+\(\d+\))?\s+panicked at\s+(.+)$`)
	rustResultLineRE = regexp.MustCompile(`^test result:\s+(?:ok|FAILED)\.\s+(\d+)\s+passed;\s+(\d+)\s+failed;`)
)

func runRustHunt(ctx context.Context, cfg config, relModule, modDir, evidenceDir string, baseEnv map[string]string, rep *Report) error {
	cargoBin, err := lookPathInEnv(baseEnv, "cargo")
	if err != nil {
		return fmt.Errorf("cargo executable not found on PATH: %w", err)
	}

	envSlice := envMapToSlice(baseEnv)
	cargoArgs := buildRustCargoArgs(cfg)
	reproCmd := buildRustReproCommand(relModule, cargoArgs)

	for round := 1; round <= cfg.rounds; round++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		roundStart := time.Now()
		var combinedLog strings.Builder
		roundExitCode := 0
		var roundFailures []Failure

		for iter := 1; iter <= cfg.count; iter++ {
			if err := ctx.Err(); err != nil {
				return err
			}

			var stdoutBuf, stderrBuf bytes.Buffer
			cmd := exec.CommandContext(ctx, cargoBin, cargoArgs...)
			cmd.Dir = modDir
			cmd.Env = envSlice
			cmd.Stdout = &stdoutBuf
			cmd.Stderr = &stderrBuf
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Cancel = func() error {
				if cmd.Process != nil {
					return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				return nil
			}
			cmd.WaitDelay = 2 * time.Second

			exitCode := 0
			runErr := cmd.Run()
			if runErr != nil {
				var exitErr *exec.ExitError
				if errors.As(runErr, &exitErr) {
					exitCode = exitErr.ExitCode()
				} else {
					return fmt.Errorf("round %d iter %d cargo test invocation failed: %w", round, iter, runErr)
				}
			}

			stdoutStr := stdoutBuf.String()
			stderrStr := stderrBuf.String()
			parsed := parseRustTestOutput(stdoutStr, stderrStr, exitCode, round, relModule, reproCmd)
			if exitCode == 0 && len(parsed.failures) == 0 && parsed.testsRun == 0 {
				parsed.failures = append(parsed.failures, Failure{
					Round:        round,
					TestName:     "<zero_tests>",
					Package:      relModule,
					Kind:         "zero_tests",
					ReproCommand: reproCmd,
					Snippet:      "silent 0-test pass blocked (anti-FP-01): cargo test executed 0 tests",
				})
			}

			if exitCode != 0 || len(parsed.failures) > 0 {
				if roundExitCode == 0 && exitCode != 0 {
					roundExitCode = exitCode
				}
				if stdoutStr != "" {
					combinedLog.WriteString(stdoutStr)
					if !strings.HasSuffix(stdoutStr, "\n") {
						combinedLog.WriteByte('\n')
					}
				}
				if strings.TrimSpace(stderrStr) != "" {
					combinedLog.WriteString("--- STDERR ---\n")
					combinedLog.WriteString(stderrStr)
					if !strings.HasSuffix(stderrStr, "\n") {
						combinedLog.WriteByte('\n')
					}
				}
				if len(parsed.failures) == 0 {
					kind := "exit_error"
					if strings.Contains(stderrStr, "could not compile") || strings.Contains(stderrStr, "error[E") {
						kind = "build_error"
					}
					parsed.failures = append(parsed.failures, Failure{
						Round:        round,
						TestName:     "<cargo>",
						Package:      relModule,
						Kind:         kind,
						ReproCommand: reproCmd,
						Snippet:      trimSnippet(stdoutStr + "\n" + stderrStr),
					})
				}
				roundFailures = append(roundFailures, parsed.failures...)
			}
		}

		summary := RoundSummary{
			Round:      round,
			ExitCode:   roundExitCode,
			Passed:     roundExitCode == 0 && len(roundFailures) == 0,
			DurationMS: time.Since(roundStart).Milliseconds(),
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

type rustRoundParse struct {
	failures []Failure
	testsRun int
}

func parseRustTestOutput(stdoutText, stderrText string, exitCode, round int, relModule, repro string) rustRoundParse {
	testsRun := 0
	panicDetail := make(map[string]string)
	var failedOrder []string
	failedSet := make(map[string]bool)

	scanLines := func(text string) {
		lines := strings.Split(text, "\n")
		for idx, raw := range lines {
			trimmed := strings.TrimSpace(raw)
			if m := rustResultLineRE.FindStringSubmatch(trimmed); len(m) == 3 {
				p, _ := strconv.Atoi(m[1])
				f, _ := strconv.Atoi(m[2])
				testsRun += p + f
				continue
			}
			if m := rustTestFailedRE.FindStringSubmatch(trimmed); len(m) == 3 {
				name := m[1]
				if name == "" {
					name = m[2]
				}
				if name != "" && !failedSet[name] {
					failedSet[name] = true
					failedOrder = append(failedOrder, name)
				}
				continue
			}
			if m := rustPanicRE.FindStringSubmatch(trimmed); len(m) == 3 {
				snippet := trimmed
				if idx+1 < len(lines) {
					if next := strings.TrimSpace(lines[idx+1]); next != "" && !strings.HasPrefix(next, "stack backtrace:") && !strings.HasPrefix(next, "note:") {
						snippet = snippet + " | " + next
					}
				}
				panicDetail[m[1]] = snippet
			}
		}
	}

	scanLines(stdoutText)
	scanLines(stderrText)

	var failures []Failure
	for _, name := range failedOrder {
		kind := "test_fail"
		snippet := "test " + name + " ... FAILED"
		if detail, ok := panicDetail[name]; ok {
			kind = "panic"
			snippet = detail
		}
		failures = append(failures, Failure{
			Round:        round,
			TestName:     name,
			Package:      relModule,
			Kind:         kind,
			ReproCommand: repro,
			Snippet:      trimSnippet(snippet),
		})
	}

	// If cargo test failed without emitting `test <name> ... FAILED` (e.g. fatal
	// abort or panic outside libtest summary), surface recorded panic details.
	if exitCode != 0 && len(failures) == 0 && len(panicDetail) > 0 {
		for name, detail := range panicDetail {
			failures = append(failures, Failure{
				Round:        round,
				TestName:     name,
				Package:      relModule,
				Kind:         "panic",
				ReproCommand: repro,
				Snippet:      trimSnippet(detail),
			})
		}
	}

	return rustRoundParse{
		failures: failures,
		testsRun: testsRun,
	}
}

func buildRustCargoArgs(cfg config) []string {
	args := []string{"test"}
	if len(cfg.extraArgs) > 0 {
		args = append(args, cfg.extraArgs...)
		if cfg.pkgExplicit {
			args = append(args, cfg.pkgs...)
		}
		return args
	}
	if cfg.pkgExplicit {
		args = append(args, cfg.pkgs...)
		return args
	}
	return append(args, "--all")
}

func buildRustReproCommand(relModule string, cargoArgs []string) string {
	manifest := filepath.ToSlash(filepath.Join(relModule, "Cargo.toml"))
	parts := []string{"cargo", "test", "--manifest-path", manifest}
	if len(cargoArgs) > 1 {
		parts = append(parts, cargoArgs[1:]...)
	}
	return strings.Join(parts, " ")
}

func lookPathInEnv(env map[string]string, bin string) (string, error) {
	if pathEnv, ok := env["PATH"]; ok && pathEnv != "" {
		for _, dir := range filepath.SplitList(pathEnv) {
			if dir == "" {
				continue
			}
			cand := filepath.Join(dir, bin)
			if st, err := os.Stat(cand); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
				return cand, nil
			}
		}
	}
	return exec.LookPath(bin)
}
