package canary

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type goCanaryConfig struct {
	projectDir string
	configFile string
	golangci   string
	gosecBin   string
	workDir    string
}

type pythonCanaryConfig struct {
	projectDir   string
	checkBandit  bool
	checkPyrefly bool
	workDir      string
}

type rustCanaryConfig struct {
	projectDir    string
	toolchainFile string
	cargoBin      string
	targetDir     string
	workDir       string
}

type frontendCanaryConfig struct {
	projectDir string
	biomeBin   string
	workDir    string
}

type denoCanaryConfig struct {
	projectDir string
	denoBin    string
	workDir    string
}

type protoCanaryConfig struct {
	bufBin  string
	workDir string
}

type actionlintCanaryConfig struct {
	actionlintBin string
	workDir       string
}

func runGoCanary(ctx context.Context, st streams, cfg goCanaryConfig) int {
	notePath(cfg.gosecBin)
	goModPath := filepath.Join(cfg.projectDir, "go.mod")
	if !isFile(goModPath) {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (go): go.mod not found at %s", goModPath))
		return exitFail
	}
	goVersion, toolchain, err := extractGoModDirectives(goModPath)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (go): %s", err.Error()))
		return exitFail
	}
	if goVersion == "" {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (go): missing 'go' directive in %s", goModPath))
		return exitFail
	}
	env := mergedEnv(map[string]string{"GOFLAGS": "-buildvcs=false"})
	code := withWorkDir(st, cfg.workDir, "alt-go-canary-", "go", func(canaryDir string) int {
		if err := prepareGoCanaryDir(canaryDir, goVersion, toolchain); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (go): %s", err.Error()))
			return exitFail
		}
		if !checkGolangciCanary(ctx, st, cfg, canaryDir, env) {
			return exitFail
		}
		return exitOK
	})
	if code != exitOK {
		return code
	}
	st.info(fmt.Sprintf(
		"canary audit passed (go): golangci-lint (exit %d, SARIF rules %s, %s)",
		golangciFindingsRC, goCanaryExpectedRule, goCanaryAltLintMarker,
	))
	return exitOK
}

func prepareGoCanaryDir(canaryDir, goVersion, toolchain string) error {
	lines := []string{"module canary", "", "go " + goVersion}
	if toolchain != "" {
		lines = append(lines, "toolchain "+toolchain)
	}
	if err := os.WriteFile(filepath.Join(canaryDir, "go.mod"), []byte(strings.Join(lines, "\n")+"\n"), fsperm.File); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}
	if err := os.WriteFile(filepath.Join(canaryDir, "canary.go"), []byte(goCanarySource), fsperm.File); err != nil {
		return fmt.Errorf("write canary.go: %w", err)
	}
	return nil
}

func checkGolangciCanary(ctx context.Context, st streams, cfg goCanaryConfig, canaryDir string, env []string) bool {
	sarifPath := filepath.Join(canaryDir, "canary.sarif")
	cmd := []string{
		"run",
		"--config", cfg.configFile,
		"--output.text.path", "stdout",
		"--output.sarif.path", sarifPath,
	}
	res := runCmd(ctx, 0, cfg.golangci, cmd, canaryDir, env)
	if res.err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (go/golangci-lint): execution error: %s", res.err.Error()))
		return false
	}
	combined := res.combined()
	if res.code != golangciFindingsRC {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (go/golangci-lint): expected exit %d with SARIF rules %s and %s, got exit %d (output: %s)",
			golangciFindingsRC, goCanaryExpectedRule, goCanaryAltLintMarker, res.code, snippet(combined),
		))
		return false
	}
	if !isFile(sarifPath) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (go/golangci-lint): missing canary SARIF output at %s (output: %s)",
			sarifPath, snippet(combined),
		))
		return false
	}
	sarifData, err := loadJSONFile(sarifPath)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (go/golangci-lint): invalid canary SARIF JSON: %s", err.Error()))
		return false
	}
	ruleIDs := extractSARIFRuleIDs(sarifData)
	var missing []string
	for _, rule := range []string{goCanaryExpectedRule, goCanaryAltLintMarker} {
		if _, ok := ruleIDs[rule]; !ok {
			missing = append(missing, rule)
		}
	}
	if len(missing) > 0 {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (go/golangci-lint): canary SARIF missing expected rule(s) %s, found rules=%s (output: %s)",
			pyList(missing), pyList(sortedSet(ruleIDs)), snippet(combined),
		))
		return false
	}
	return true
}

func runPythonCanary(ctx context.Context, st streams, cfg pythonCanaryConfig) int {
	pyproject := filepath.Join(cfg.projectDir, "pyproject.toml")
	if !isFile(pyproject) {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (python): pyproject.toml not found at %s", pyproject))
		return exitFail
	}
	uvBin := look("uv")
	if uvBin == "" {
		st.error("ERROR: CANARY AUDIT FAILED (python): uv executable not found in PATH")
		return exitFail
	}
	code := withWorkDir(st, cfg.workDir, "alt-py-canary-", "python", func(canaryDir string) int {
		canaryPy := filepath.Join(canaryDir, "canary.py")
		if err := os.WriteFile(canaryPy, []byte(pyCanarySource), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (python/ruff): %s", err.Error()))
			return exitFail
		}
		if !checkRuffCanary(ctx, st, uvBin, cfg.projectDir, canaryPy) {
			return exitFail
		}
		if cfg.checkBandit && !checkBanditCanary(ctx, st, uvBin, cfg.projectDir, canaryDir) {
			return exitFail
		}
		if cfg.checkPyrefly && !checkPyreflyCanary(ctx, st, uvBin, cfg.projectDir, canaryDir) {
			return exitFail
		}
		return exitOK
	})
	if code != exitOK {
		return code
	}
	var extras []string
	if cfg.checkBandit {
		extras = append(extras, "bandit (B602/B307)")
	}
	if cfg.checkPyrefly {
		extras = append(extras, "pyrefly (bad-assignment)")
	}
	extraNote := ""
	if len(extras) > 0 {
		extraNote = ", " + strings.Join(extras, ", ")
	}
	st.info(fmt.Sprintf("canary audit passed (python): ruff (%s)%s", ruffCanaryExpectedRule, extraNote))
	return exitOK
}

func checkRuffCanary(ctx context.Context, st streams, uvBin, projectDir, canaryPy string) bool {
	cmd := []string{
		"run", "--frozen", "--only-group", "dev", "ruff", "check", "--no-cache",
		"--config", filepath.Join(projectDir, "pyproject.toml"), canaryPy,
	}
	res := runCmd(ctx, 0, uvBin, cmd, projectDir, nil)
	if res.err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (python/ruff): execution error: %s", res.err.Error()))
		return false
	}
	combined := res.combined()
	if res.code != ruffFindingsRC || !strings.Contains(combined, ruffCanaryExpectedRule) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (python/ruff): expected exit %d with rule %s, got exit %d (output: %s)",
			ruffFindingsRC, ruffCanaryExpectedRule, res.code, snippet(combined),
		))
		return false
	}
	return true
}

func checkBanditCanary(ctx context.Context, st streams, uvBin, projectDir, canaryDir string) bool {
	cmd := []string{"run", "--frozen", "--only-group", "dev", "bandit", "-r", canaryDir, "-ll", "-ii"}
	res := runCmd(ctx, 0, uvBin, cmd, projectDir, nil)
	if res.err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (python/bandit): execution error: %s", res.err.Error()))
		return false
	}
	combined := res.combined()
	if res.code == 0 || !anyMarker(combined, banditCanaryRules) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (python/bandit): expected non-zero exit and one of %s, got exit %d (output: %s)",
			pyTuple(banditCanaryRules), res.code, snippet(combined),
		))
		return false
	}
	return true
}

func checkPyreflyCanary(ctx context.Context, st streams, uvBin, projectDir, canaryDir string) bool {
	pyreflyDir := filepath.Join(canaryDir, "pyrefly")
	if err := os.MkdirAll(pyreflyDir, fsperm.Dir); err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (python/pyrefly): %s", err.Error()))
		return false
	}
	canaryType := filepath.Join(pyreflyDir, "canary_type.py")
	if err := os.WriteFile(canaryType, []byte(pyPyreflyCanarySource), fsperm.File); err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (python/pyrefly): %s", err.Error()))
		return false
	}
	pyproject := filepath.Join(projectDir, "pyproject.toml")
	configPath := filepath.Join(pyreflyDir, "pyrefly.toml")
	if pyprojectHasPyrefly(pyproject) {
		configPath = pyproject
	} else if err := os.WriteFile(configPath, []byte("project-includes = [\"canary_type.py\"]\n"), fsperm.File); err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (python/pyrefly): %s", err.Error()))
		return false
	}
	cmd := []string{
		"run", "--frozen", "--only-group", "dev", "pyrefly", "check",
		"--config", configPath, canaryType,
	}
	res := runCmd(ctx, 0, uvBin, cmd, projectDir, nil)
	if res.err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (python/pyrefly): execution error: %s", res.err.Error()))
		return false
	}
	combined := res.combined()
	if res.code == 0 || !anyMarker(strings.ToLower(combined), pyreflyCanaryMarkers) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (python/pyrefly): expected non-zero exit with type-error diagnostic, got exit %d (output: %s)",
			res.code, snippet(combined),
		))
		return false
	}
	return true
}

func pyprojectHasPyrefly(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return false
	}
	tool, ok := root["tool"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = tool["pyrefly"].(map[string]any)
	return ok
}

func findRustToolchainFile(projectDir string) string {
	current, err := filepath.Abs(projectDir)
	if err != nil {
		current = projectDir
	}
	for {
		candidate := filepath.Join(current, "rust-toolchain.toml")
		if isFile(candidate) {
			return candidate
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func extractRustChannel(toolchainFile string) (string, error) {
	data, err := os.ReadFile(toolchainFile)
	if err != nil {
		return "", err
	}
	for _, raw := range strings.Split(string(data), "\n") {
		if m := rustChannelRE.FindStringSubmatch(raw); m != nil {
			return m[1], nil
		}
	}
	return "", nil
}

func runRustCanary(ctx context.Context, st streams, cfg rustCanaryConfig) int {
	toolchain := cfg.toolchainFile
	if toolchain == "" {
		toolchain = findRustToolchainFile(cfg.projectDir)
	}
	if toolchain == "" || !isFile(toolchain) {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (rust/clippy): rust-toolchain.toml not found for %s", cfg.projectDir))
		return exitFail
	}
	channel, err := extractRustChannel(toolchain)
	if err != nil || channel == "" {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (rust/clippy): missing channel in %s", toolchain))
		return exitFail
	}
	cargoBin := cfg.cargoBin
	if cargoBin == "" {
		cargoBin = look("cargo")
	}
	if cargoBin == "" {
		st.error("ERROR: CANARY AUDIT FAILED (rust/clippy): cargo executable not found in PATH")
		return exitFail
	}
	targetDir := cfg.targetDir
	if targetDir == "" {
		targetDir = filepath.Join(cacheRoot(), "alt-lint", "rust-canary-target")
	}
	if err := os.MkdirAll(targetDir, fsperm.Dir); err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (rust/clippy): %s", err.Error()))
		return exitFail
	}
	env := mergedEnv(map[string]string{"CARGO_TARGET_DIR": targetDir})
	var res cmdResult
	code := withWorkDir(st, cfg.workDir, "alt-rust-canary-", "rust/clippy", func(canaryDir string) int {
		if err := os.MkdirAll(filepath.Join(canaryDir, "src"), fsperm.Dir); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (rust/clippy): %s", err.Error()))
			return exitFail
		}
		toolchainBody := fmt.Sprintf("[toolchain]\nchannel = \"%s\"\ncomponents = [\"clippy\"]\n", channel)
		if err := os.WriteFile(filepath.Join(canaryDir, "rust-toolchain.toml"), []byte(toolchainBody), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (rust/clippy): %s", err.Error()))
			return exitFail
		}
		if err := os.WriteFile(filepath.Join(canaryDir, "Cargo.toml"), []byte(rustCanaryCargoTOML), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (rust/clippy): %s", err.Error()))
			return exitFail
		}
		if err := os.WriteFile(filepath.Join(canaryDir, "src", "lib.rs"), []byte(rustCanaryLibRS), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (rust/clippy): %s", err.Error()))
			return exitFail
		}
		res = runCmd(ctx, 0, cargoBin, []string{"clippy", "--offline", "--", "-D", "warnings"}, canaryDir, env)
		if res.err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (rust/clippy): execution error: %s", res.err.Error()))
			return exitFail
		}
		return exitOK
	})
	if code != exitOK {
		return code
	}
	combined := res.combined()
	if res.code == 0 || !anyMarker(strings.ToLower(combined), rustCanaryMarkers) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (rust/clippy): expected non-zero exit with clippy::eq_op diagnostic, got exit %d (output: %s)",
			res.code, snippet(combined),
		))
		return exitFail
	}
	st.info(fmt.Sprintf("canary audit passed (rust): cargo clippy (%s, clippy::eq_op)", channel))
	return exitOK
}

func runFrontendCanary(ctx context.Context, st streams, cfg frontendCanaryConfig) int {
	biomeBin := cfg.biomeBin
	if biomeBin == "" {
		biomeBin = filepath.Join(cfg.projectDir, "node_modules", ".bin", "biome")
	}
	if !isFile(biomeBin) {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (frontend/biome): biome binary not found at %s", biomeBin))
		return exitFail
	}
	var res cmdResult
	code := withWorkDir(st, cfg.workDir, "alt-frontend-canary-", "frontend/biome", func(canaryDir string) int {
		if err := os.WriteFile(filepath.Join(canaryDir, "biome.json"), []byte(frontendCanaryBiomeJSON), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (frontend/biome): %s", err.Error()))
			return exitFail
		}
		canaryTS := filepath.Join(canaryDir, "canary.ts")
		if err := os.WriteFile(canaryTS, []byte(tsDebuggerCanarySource), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (frontend/biome): %s", err.Error()))
			return exitFail
		}
		res = runCmd(ctx, 0, biomeBin, []string{"lint", "--config-path", canaryDir, canaryTS}, canaryDir, nil)
		if res.err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (frontend/biome): execution error: %s", res.err.Error()))
			return exitFail
		}
		return exitOK
	})
	if code != exitOK {
		return code
	}
	combined := res.combined()
	if res.code == 0 || !anyMarker(strings.ToLower(combined), frontendCanaryMarkers) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (frontend/biome): expected non-zero exit with noDebugger diagnostic, got exit %d (output: %s)",
			res.code, snippet(combined),
		))
		return exitFail
	}
	st.info("canary audit passed (frontend): biome lint (noDebugger)")
	return exitOK
}

func runDenoCanary(ctx context.Context, st streams, cfg denoCanaryConfig) int {
	notePath(cfg.projectDir)
	denoBin := cfg.denoBin
	if denoBin == "" {
		denoBin = look("deno")
	}
	if denoBin == "" {
		st.error("ERROR: CANARY AUDIT FAILED (deno/lint): deno executable not found in PATH")
		return exitFail
	}
	var res cmdResult
	code := withWorkDir(st, cfg.workDir, "alt-deno-canary-", "deno/lint", func(canaryDir string) int {
		canaryTS := filepath.Join(canaryDir, "canary.ts")
		if err := os.WriteFile(canaryTS, []byte(tsDebuggerCanarySource), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (deno/lint): %s", err.Error()))
			return exitFail
		}
		res = runCmd(ctx, 0, denoBin, []string{"lint", canaryTS}, canaryDir, nil)
		if res.err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (deno/lint): execution error: %s", res.err.Error()))
			return exitFail
		}
		return exitOK
	})
	if code != exitOK {
		return code
	}
	combined := res.combined()
	if res.code == 0 || !strings.Contains(combined, denoCanaryExpectedRule) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (deno/lint): expected non-zero exit with %s diagnostic, got exit %d (output: %s)",
			denoCanaryExpectedRule, res.code, snippet(combined),
		))
		return exitFail
	}
	st.info(fmt.Sprintf("canary audit passed (deno): deno lint (%s)", denoCanaryExpectedRule))
	return exitOK
}

func runProtoCanary(ctx context.Context, st streams, cfg protoCanaryConfig) int {
	bufBin := cfg.bufBin
	if bufBin == "" {
		bufBin = look("buf")
	}
	if bufBin == "" {
		st.error("ERROR: CANARY AUDIT FAILED (proto/buf): buf executable not found in PATH")
		return exitFail
	}
	var res cmdResult
	code := withWorkDir(st, cfg.workDir, "alt-proto-canary-", "proto/buf", func(canaryDir string) int {
		if err := os.WriteFile(filepath.Join(canaryDir, "buf.yaml"), []byte(protoCanaryBufYAML), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (proto/buf): %s", err.Error()))
			return exitFail
		}
		if err := os.WriteFile(filepath.Join(canaryDir, "canary.proto"), []byte(protoCanarySource), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (proto/buf): %s", err.Error()))
			return exitFail
		}
		res = runCmd(ctx, 0, bufBin, []string{"lint"}, canaryDir, nil)
		if res.err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (proto/buf): execution error: %s", res.err.Error()))
			return exitFail
		}
		return exitOK
	})
	if code != exitOK {
		return code
	}
	combined := res.combined()
	if res.code == 0 || !anyMarker(strings.ToLower(combined), protoCanaryMarkers) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (proto/buf): expected non-zero exit with buf STANDARD lint diagnostic, got exit %d (output: %s)",
			res.code, snippet(combined),
		))
		return exitFail
	}
	st.info("canary audit passed (proto): buf lint (STANDARD)")
	return exitOK
}

func runActionlintCanary(ctx context.Context, st streams, cfg actionlintCanaryConfig) int {
	var cmdPrefix []string
	if cfg.actionlintBin != "" {
		cmdPrefix = []string{cfg.actionlintBin}
	} else if found := look("actionlint"); found != "" {
		cmdPrefix = []string{found}
	} else if goBin := look("go"); goBin != "" {
		cmdPrefix = []string{goBin, "run", actionlintGoModule}
	} else {
		st.error("ERROR: CANARY AUDIT FAILED (repo/actionlint): neither actionlint nor go found in PATH")
		return exitFail
	}
	var res cmdResult
	code := withWorkDir(st, cfg.workDir, "alt-actionlint-canary-", "repo/actionlint", func(canaryDir string) int {
		canaryYML := filepath.Join(canaryDir, "canary.yml")
		if err := os.WriteFile(canaryYML, []byte(actionlintCanaryWorkflow), fsperm.File); err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (repo/actionlint): %s", err.Error()))
			return exitFail
		}
		args := append(append([]string{}, cmdPrefix[1:]...), "-shellcheck=", canaryYML)
		res = runCmd(ctx, 0, cmdPrefix[0], args, "", nil)
		if res.err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (repo/actionlint): execution error: %s", res.err.Error()))
			return exitFail
		}
		return exitOK
	})
	if code != exitOK {
		return code
	}
	combined := res.combined()
	if res.code == 0 || !anyMarker(strings.ToLower(combined), actionlintCanaryMarkers) {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (repo/actionlint): expected non-zero exit with expression diagnostic, got exit %d (output: %s)",
			res.code, snippet(combined),
		))
		return exitFail
	}
	st.info("canary audit passed (repo): actionlint (expression)")
	return exitOK
}

func pyTuple(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = pyQuote(item)
	}
	if len(parts) == 1 {
		return "(" + parts[0] + ",)"
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func cacheRoot() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), ".cache")
	}
	return filepath.Join(home, ".cache")
}
