package canary

import (
	"context"
	"fmt"
	"io"
	"strings"
)

const auditHelp = `Isolated canary self-audit, scan coverage guard, and ledger verifier.

subcommands:
  go-canary --project-dir DIR --config FILE --golangci-lint-bin BIN [--gosec-bin BIN] [--work-dir DIR]
  go-verify-scan --project-dir DIR --gosec-sarif FILE [--gosec-out FILE]
  go-verify-govulncheck --project-dir DIR --govulncheck-sarif FILE [--govulncheck-json FILE]
  python-canary --project-dir DIR [--check-bandit BOOL] [--check-pyrefly BOOL] [--work-dir DIR]
  python-verify-files --project-dir DIR
  python-verify-bandit-scan --project-dir DIR --bandit-out FILE
  rust-canary --project-dir DIR [--toolchain-file FILE] [--cargo-bin BIN] [--target-dir DIR] [--work-dir DIR]
  frontend-canary --project-dir DIR [--biome-bin BIN] [--work-dir DIR]
  deno-canary --project-dir DIR [--deno-bin BIN] [--work-dir DIR]
  proto-canary [--buf-bin BIN] [--work-dir DIR]
  actionlint-canary [--actionlint-bin BIN] [--work-dir DIR]
  tauto-canary [--tauto-bin BIN] [--work-dir DIR]
  verify-ledger --repo-root DIR [--ledger FILE] [--allow-sparse]
`

var auditCommandNames = []string{
	"go-canary",
	"go-verify-scan",
	"go-verify-govulncheck",
	"python-canary",
	"python-verify-files",
	"python-verify-bandit-scan",
	"rust-canary",
	"frontend-canary",
	"deno-canary",
	"proto-canary",
	"actionlint-canary",
	"tauto-canary",
	"verify-ledger",
}

// RunAudit executes the canary_audit.py CLI. args excludes argv[0].
// A scan that covers zero files exits non-zero.
func RunAudit(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	st := newStreams(stdout, stderr)
	if ctx == nil {
		st.error("error: nil context")
		return exitUsage
	}
	if len(args) == 0 {
		st.error("error: the following arguments are required: command")
		return exitUsage
	}
	if args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(st.stdout, auditHelp)
		return exitOK
	}
	if strings.HasPrefix(args[0], "-") {
		st.error("error: the following arguments are required: command")
		return exitUsage
	}
	command := args[0]
	rest := args[1:]
	if !knownAuditCommand(command) {
		st.error(fmt.Sprintf(
			"error: argument command: invalid choice: %s (choose from %s)",
			pyQuote(command), strings.Join(auditCommandNames, ", "),
		))
		return exitUsage
	}
	if hasHelp(rest) {
		fmt.Fprint(st.stdout, auditHelp)
		return exitOK
	}
	switch command {
	case "go-canary":
		return cmdGoCanary(ctx, st, rest)
	case "go-verify-scan":
		return cmdGoVerifyScan(ctx, st, rest)
	case "go-verify-govulncheck":
		return cmdGoVerifyGovulncheck(ctx, st, rest)
	case "python-canary":
		return cmdPythonCanary(ctx, st, rest)
	case "python-verify-files":
		return cmdPythonVerifyFiles(st, rest)
	case "python-verify-bandit-scan":
		return cmdPythonVerifyBandit(st, rest)
	case "rust-canary":
		return cmdRustCanary(ctx, st, rest)
	case "frontend-canary":
		return cmdFrontendCanary(ctx, st, rest)
	case "deno-canary":
		return cmdDenoCanary(ctx, st, rest)
	case "proto-canary":
		return cmdProtoCanary(ctx, st, rest)
	case "actionlint-canary":
		return cmdActionlintCanary(ctx, st, rest)
	case "tauto-canary":
		return cmdTautoCanary(ctx, st, rest)
	case "verify-ledger":
		return cmdVerifyLedger(ctx, st, rest)
	default:
		st.error("ERROR: unknown command: " + command)
		return exitUsage
	}
}

func knownAuditCommand(name string) bool {
	for _, command := range auditCommandNames {
		if command == name {
			return true
		}
	}
	return false
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func usageResult(st streams, msg string) int {
	if msg == "help" {
		fmt.Fprint(st.stdout, auditHelp)
		return exitOK
	}
	st.error(msg)
	return exitUsage
}

func cmdGoCanary(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args,
		[]string{"project-dir", "config", "golangci-lint-bin"},
		[]string{"gosec-bin", "work-dir"},
		nil, nil,
	)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, err := flags.reqPath("project-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	configFile, err := flags.reqPath("config")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	golangci, err := flags.reqPath("golangci-lint-bin")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	gosecBin, err := flags.optPath("gosec-bin")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	workDir, err := flags.optPath("work-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return runGoCanary(ctx, st, goCanaryConfig{
		projectDir: projectDir,
		configFile: configFile,
		golangci:   golangci,
		gosecBin:   gosecBin,
		workDir:    workDir,
	})
}

func cmdGoVerifyScan(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args,
		[]string{"project-dir", "gosec-sarif"},
		[]string{"gosec-out"},
		nil, nil,
	)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, sarifPath, err := twoPaths(flags, "project-dir", "gosec-sarif")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	gosecOut, err := flags.optPath("gosec-out")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return verifyGoScan(ctx, st, projectDir, gosecOut, sarifPath)
}

func cmdGoVerifyGovulncheck(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args,
		[]string{"project-dir", "govulncheck-sarif"},
		[]string{"govulncheck-json"},
		nil, nil,
	)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, sarifPath, err := twoPaths(flags, "project-dir", "govulncheck-sarif")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	jsonPath, err := flags.optPath("govulncheck-json")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return verifyGoGovulncheck(ctx, st, projectDir, sarifPath, jsonPath)
}

func cmdPythonCanary(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args,
		[]string{"project-dir"},
		[]string{"work-dir"},
		nil,
		[]string{"check-bandit", "check-pyrefly"},
	)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, err := flags.reqPath("project-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	workDir, err := flags.optPath("work-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return runPythonCanary(ctx, st, pythonCanaryConfig{
		projectDir:   projectDir,
		checkBandit:  flags.boolVal("check-bandit"),
		checkPyrefly: flags.boolVal("check-pyrefly"),
		workDir:      workDir,
	})
}

func cmdPythonVerifyFiles(st streams, args []string) int {
	flags, msg := parseFlags(args, []string{"project-dir"}, nil, nil, nil)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, err := flags.reqPath("project-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return verifyPythonFiles(st, projectDir)
}

func cmdPythonVerifyBandit(st streams, args []string) int {
	flags, msg := parseFlags(args, []string{"project-dir", "bandit-out"}, nil, nil, nil)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, banditOut, err := twoPaths(flags, "project-dir", "bandit-out")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return verifyPythonBanditScan(st, projectDir, banditOut)
}

func cmdRustCanary(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args,
		[]string{"project-dir"},
		[]string{"toolchain-file", "cargo-bin", "target-dir", "work-dir"},
		nil, nil,
	)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, err := flags.reqPath("project-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	toolchain, err := flags.optPath("toolchain-file")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	cargoBin, err := flags.optPath("cargo-bin")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	targetDir, err := flags.optPath("target-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	workDir, err := flags.optPath("work-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return runRustCanary(ctx, st, rustCanaryConfig{
		projectDir:    projectDir,
		toolchainFile: toolchain,
		cargoBin:      cargoBin,
		targetDir:     targetDir,
		workDir:       workDir,
	})
}

func cmdFrontendCanary(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args,
		[]string{"project-dir"},
		[]string{"biome-bin", "work-dir"},
		nil, nil,
	)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, err := flags.reqPath("project-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	biomeBin, err := flags.optPath("biome-bin")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	workDir, err := flags.optPath("work-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return runFrontendCanary(ctx, st, frontendCanaryConfig{projectDir: projectDir, biomeBin: biomeBin, workDir: workDir})
}

func cmdDenoCanary(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args,
		[]string{"project-dir"},
		[]string{"deno-bin", "work-dir"},
		nil, nil,
	)
	if msg != "" {
		return usageResult(st, msg)
	}
	projectDir, err := flags.reqPath("project-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	denoBin, err := flags.optPath("deno-bin")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	workDir, err := flags.optPath("work-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return runDenoCanary(ctx, st, denoCanaryConfig{projectDir: projectDir, denoBin: denoBin, workDir: workDir})
}

func cmdProtoCanary(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args, nil, []string{"buf-bin", "work-dir"}, nil, nil)
	if msg != "" {
		return usageResult(st, msg)
	}
	bufBin, err := flags.optPath("buf-bin")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	workDir, err := flags.optPath("work-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return runProtoCanary(ctx, st, protoCanaryConfig{bufBin: bufBin, workDir: workDir})
}

func cmdActionlintCanary(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args, nil, []string{"actionlint-bin", "work-dir"}, nil, nil)
	if msg != "" {
		return usageResult(st, msg)
	}
	bin, err := flags.optPath("actionlint-bin")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	workDir, err := flags.optPath("work-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return runActionlintCanary(ctx, st, actionlintCanaryConfig{actionlintBin: bin, workDir: workDir})
}

func cmdTautoCanary(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args, nil, []string{"tauto-bin", "work-dir"}, nil, nil)
	if msg != "" {
		return usageResult(st, msg)
	}
	bin, err := flags.optPath("tauto-bin")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	workDir, err := flags.optPath("work-dir")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return runTautoCanary(ctx, st, tautoCanaryConfig{tautoBin: bin, workDir: workDir})
}

func cmdVerifyLedger(ctx context.Context, st streams, args []string) int {
	flags, msg := parseFlags(args,
		[]string{"repo-root"},
		[]string{"ledger"},
		[]string{"allow-sparse"},
		nil,
	)
	if msg != "" {
		return usageResult(st, msg)
	}
	repoRoot, err := flags.reqPath("repo-root")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	ledger, err := flags.optPath("ledger")
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	return verifyCanaryLedger(ctx, st, repoRoot, ledger, flags.bools["allow-sparse"])
}

func twoPaths(flags parsedFlags, a, b string) (string, string, error) {
	left, err := flags.reqPath(a)
	if err != nil {
		return "", "", err
	}
	right, err := flags.reqPath(b)
	if err != nil {
		return "", "", err
	}
	return left, right, nil
}
