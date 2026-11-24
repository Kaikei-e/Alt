// Package pytestrand implements `alt-harness pytest-rand`, a pure-Go port of
// `scripts/verify/run_pytest_randomized.py`.
package pytestrand

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const pluginModuleName = "_alt_pytest_random_order_plugin"

const pluginSource = `"""Temporary pytest plugin injected by alt-harness pytest-rand."""

from __future__ import annotations

import os
import random


def pytest_collection_modifyitems(items: list[object]) -> None:
    raw_seed = os.environ.get("ALT_PYTEST_RANDOM_SEED")
    if raw_seed is None:
        return
    rng = random.Random(int(raw_seed))
    rng.shuffle(items)
`

// Options configures a single pytest-rand invocation without mutating process globals.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
	Env    map[string]string
}

type parsedArgs struct {
	targetDir string
	hasSeed   bool
	seed      int64
	seedLog   string
	pytestCmd string
	extraArgs []string
	showHelp  bool
}

type seedLogRecord struct {
	ExitCode  int    `json:"exit_code"`
	Seed      int64  `json:"seed"`
	Target    string `json:"target"`
	Timestamp string `json:"timestamp"`
}

// Run executes `alt-harness pytest-rand` using the current process environment.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return RunWithOptions(ctx, args, Options{
		Stdout: stdout,
		Stderr: stderr,
	})
}

// RunWithOptions executes `alt-harness pytest-rand` with explicit streams and environment.
func RunWithOptions(ctx context.Context, args []string, opts Options) int {
	if ctx == nil {
		ctx = context.Background()
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	parsed, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\nusage: alt-harness pytest-rand [--seed <int>] [--seed-log <path>] [--pytest-cmd <cmd>] <target_dir> [-- <pytest_args>...]\n", err)
		return 2
	}
	if parsed.showHelp {
		fmt.Fprintln(stdout, "usage: alt-harness pytest-rand [--seed <int>] [--seed-log <path>] [--pytest-cmd <cmd>] <target_dir> [-- <pytest_args>...]")
		return 0
	}

	st, statErr := os.Stat(parsed.targetDir)
	if statErr != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "ERROR: target directory does not exist: %s\n", parsed.targetDir)
		return 2
	}

	seed := parsed.seed
	if !parsed.hasSeed {
		seed, err = cryptoRandSeed32()
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to generate random seed: %v\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "PYTEST_RANDOM_SEED=%d\n", seed)

	baseCmd, err := splitShellWords(parsed.pytestCmd)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: invalid --pytest-cmd: %v\n", err)
		return 2
	}
	if len(baseCmd) == 0 {
		fmt.Fprintln(stderr, "ERROR: --pytest-cmd must not be empty")
		return 2
	}

	pluginDir, err := os.MkdirTemp("", "alt-pytest-rand-*")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to create temp plugin dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(pluginDir) }()

	pluginPath := filepath.Join(pluginDir, pluginModuleName+".py")
	if err := os.WriteFile(pluginPath, []byte(pluginSource), 0o600); err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to write pytest plugin: %v\n", err)
		return 1
	}

	env := make(map[string]string)
	if opts.Env != nil {
		for k, v := range opts.Env {
			env[k] = v
		}
	} else {
		for _, kv := range os.Environ() {
			if k, v, ok := strings.Cut(kv, "="); ok {
				env[k] = v
			}
		}
	}
	env["ALT_PYTEST_RANDOM_SEED"] = strconv.FormatInt(seed, 10)
	if existing := env["PYTHONPATH"]; existing != "" {
		env["PYTHONPATH"] = pluginDir + string(os.PathListSeparator) + existing
	} else {
		env["PYTHONPATH"] = pluginDir
	}

	bin, err := lookPathInEnv(baseCmd[0], env)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: command not found: %s\n", baseCmd[0])
		return 127
	}

	cmdArgs := make([]string, 0, len(baseCmd)-1+2+len(parsed.extraArgs))
	cmdArgs = append(cmdArgs, baseCmd[1:]...)
	cmdArgs = append(cmdArgs, "-p", pluginModuleName)
	cmdArgs = append(cmdArgs, parsed.extraArgs...)

	cmd := exec.CommandContext(ctx, bin, cmdArgs...)
	cmd.Dir = parsed.targetDir
	cmd.Env = mapToEnvSlice(env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitCode := 0
	if runErr := cmd.Run(); runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			fmt.Fprintf(stderr, "ERROR: pytest invocation failed: %v\n", runErr)
			exitCode = 1
		}
	}

	if parsed.seedLog != "" {
		if err := appendSeedLog(parsed.seedLog, parsed.targetDir, seed, exitCode); err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to write seed log: %v\n", err)
			return 1
		}
	}

	return exitCode
}

func parseArgs(args []string) (parsedArgs, error) {
	out := parsedArgs{pytestCmd: "uv run pytest"}
	runnerArgs := args
	for idx, a := range args {
		if a == "--" {
			runnerArgs = args[:idx]
			out.extraArgs = args[idx+1:]
			break
		}
	}

	var positionals []string
	for i := 0; i < len(runnerArgs); i++ {
		arg := runnerArgs[i]
		switch {
		case arg == "-h" || arg == "--help":
			out.showHelp = true
			return out, nil
		case arg == "--seed":
			if i+1 >= len(runnerArgs) {
				return out, errors.New("--seed requires an integer value")
			}
			i++
			val, err := strconv.ParseInt(runnerArgs[i], 10, 64)
			if err != nil {
				return out, fmt.Errorf("invalid --seed %q: %w", runnerArgs[i], err)
			}
			out.hasSeed = true
			out.seed = val
		case strings.HasPrefix(arg, "--seed="):
			raw := strings.TrimPrefix(arg, "--seed=")
			val, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return out, fmt.Errorf("invalid --seed %q: %w", raw, err)
			}
			out.hasSeed = true
			out.seed = val
		case arg == "--seed-log":
			if i+1 >= len(runnerArgs) {
				return out, errors.New("--seed-log requires a path value")
			}
			i++
			out.seedLog = runnerArgs[i]
		case strings.HasPrefix(arg, "--seed-log="):
			out.seedLog = strings.TrimPrefix(arg, "--seed-log=")
		case arg == "--pytest-cmd":
			if i+1 >= len(runnerArgs) {
				return out, errors.New("--pytest-cmd requires a command value")
			}
			i++
			out.pytestCmd = runnerArgs[i]
		case strings.HasPrefix(arg, "--pytest-cmd="):
			out.pytestCmd = strings.TrimPrefix(arg, "--pytest-cmd=")
		case strings.HasPrefix(arg, "-"):
			return out, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}

	if len(positionals) != 1 {
		return out, fmt.Errorf("expected exactly 1 target_dir, got %d", len(positionals))
	}
	out.targetDir = positionals[0]
	return out, nil
}

func cryptoRandSeed32() (int64, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint32(buf[:])), nil
}

func appendSeedLog(seedLog, targetDir string, seed int64, exitCode int) error {
	if err := os.MkdirAll(filepath.Dir(seedLog), 0o755); err != nil {
		return err
	}
	rec := seedLogRecord{
		ExitCode:  exitCode,
		Seed:      seed,
		Target:    targetDir,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	fh, err := os.OpenFile(seedLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	_, err = fh.Write(append(raw, '\n'))
	return err
}

func splitShellWords(input string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inToken := false
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if escaped {
			cur.WriteByte(ch)
			inToken = true
			escaped = false
			continue
		}
		if inSingle {
			if ch == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(ch)
			}
			continue
		}
		if inDouble {
			switch ch {
			case '"':
				inDouble = false
			case '\\':
				escaped = true
			default:
				cur.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case ' ', '\t', '\n', '\r':
			if inToken {
				words = append(words, cur.String())
				cur.Reset()
				inToken = false
			}
		case '\'':
			inSingle = true
			inToken = true
		case '"':
			inDouble = true
			inToken = true
		case '\\':
			escaped = true
			inToken = true
		default:
			cur.WriteByte(ch)
			inToken = true
		}
	}
	if escaped || inSingle || inDouble {
		return nil, errors.New("unclosed quote or trailing escape")
	}
	if inToken {
		words = append(words, cur.String())
	}
	return words, nil
}

func lookPathInEnv(name string, env map[string]string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		st, err := os.Stat(name)
		if err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			return "", exec.ErrNotFound
		}
		return name, nil
	}
	for _, dir := range filepath.SplitList(env["PATH"]) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		st, err := os.Stat(candidate)
		if err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func mapToEnvSlice(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}
