package canary

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	goCanaryExpectedRule   = "G402"
	goCanaryAltLintMarker  = "ineffassign"
	golangciFindingsRC     = 1
	ruffCanaryExpectedRule = "F401"
	ruffFindingsRC         = 1
	denoCanaryExpectedRule = "no-debugger"
	actionlintGoModule     = "github.com/rhysd/actionlint/cmd/actionlint@v1.7.12"
)

var (
	goDirectiveRE        = regexp.MustCompile(`^go\s+(\S+)`)
	toolchainDirectiveRE = regexp.MustCompile(`^toolchain\s+(\S+)`)
	gosecFilesRE         = regexp.MustCompile(`(?m)^\s*Files\s*:\s*(\d+)\s*$`)
	gosecRulePrefixRE    = regexp.MustCompile(`^(G\d{3}):`)
	banditLocRE          = regexp.MustCompile(`Total lines of code:\s*(\d+)`)
	rustChannelRE        = regexp.MustCompile(`^\s*channel\s*=\s*"([^"]+)"`)
	depNameSplitRE       = regexp.MustCompile(`[\s\[<>=!~;@]`)
	depNormRE            = regexp.MustCompile(`[-_.]+`)

	banditCanaryRules       = []string{"B602", "B307"}
	pyreflyCanaryMarkers    = []string{"bad-assignment", "not assignable", "error"}
	rustCanaryMarkers       = []string{"eq_op", "equal expressions"}
	frontendCanaryMarkers   = []string{"nodebugger", "debugger"}
	protoCanaryMarkers      = []string{"bad_enum", "badpkg", "enum_", "package_"}
	actionlintCanaryMarkers = []string{"invalid_context", "expression"}

	goExcludedDirs = map[string]struct{}{
		".git": {}, "vendor": {}, "gen": {}, "_reports": {}, "node_modules": {},
	}
	goGeneratedSuffixes = []string{".pb.go", ".connect.go"}
	goGeneratedExact    = map[string]struct{}{"server.gen.go": {}}

	pyExcludedDirs = map[string]struct{}{
		".git": {}, ".venv": {}, "venv": {}, "build": {}, "dist": {},
		"__pycache__": {}, ".mypy_cache": {}, ".pytest_cache": {}, ".ruff_cache": {},
		"node_modules": {}, "gen": {}, "generated": {},
	}
	pyGeneratedSuffixes = []string{"_pb2.py", "_pb2_grpc.py", "_connect.py", "_gen.py", ".gen.py"}
)

const goCanarySource = "package canary\n\nimport \"crypto/tls\"\n\n" +
	"// CanaryCheck intentionally contains known lint and security violations\n" +
	"// to verify static analyzers do not silently pass with zero findings.\n" +
	"func CanaryCheck() *tls.Config {\n" +
	"\tx := 1\n" +
	"\tx = 2\n" +
	"\t_ = x\n" +
	"\treturn &tls.Config{InsecureSkipVerify: true}\n" +
	"}\n"

const pyCanarySource = "import os\nimport subprocess\n\n\ndef canary_check(cmd: str) -> None:\n" +
	"    subprocess.Popen(cmd, shell=True)\n" +
	"    eval(cmd)\n"

const pyPyreflyCanarySource = "\"\"\"Isolated pyrefly type-error canary.\"\"\"\n\n\n" +
	"def canary_type_error() -> int:\n" +
	"    value: int = \"not_an_int\"\n" +
	"    return value\n"

const rustCanaryCargoTOML = "[package]\nname = \"alt-rust-canary\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"

const rustCanaryLibRS = "pub fn canary_check(x: i32) -> bool {\n    x == x\n}\n"

const frontendCanaryBiomeJSON = "{\n  \"linter\": {\n    \"enabled\": true,\n    \"rules\": {\n      \"recommended\": true\n    }\n  }\n}\n"

const tsDebuggerCanarySource = "export function canaryCheck(value: number): number {\n  debugger;\n  return value;\n}\n"

const protoCanaryBufYAML = "version: v2\nlint:\n  use:\n    - STANDARD\n"

const protoCanarySource = "syntax = \"proto3\";\n\npackage badpkg;\n\nenum bad_enum {\n  bad_value = 0;\n}\n"

const actionlintCanaryWorkflow = "name: Canary Broken Workflow\n" +
	"on: push\n" +
	"jobs:\n" +
	"  broken:\n" +
	"    runs-on: ubuntu-latest\n" +
	"    steps:\n" +
	"      - run: echo ${{ invalid_context.nonexistent }}\n"

func extractGoModDirectives(goModPath string) (string, string, error) {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", "", fmt.Errorf("read go.mod: %w", err)
	}
	var goVersion, toolchain string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if goVersion == "" {
			if m := goDirectiveRE.FindStringSubmatch(line); m != nil {
				goVersion = m[1]
			}
		}
		if toolchain == "" {
			if m := toolchainDirectiveRE.FindStringSubmatch(line); m != nil {
				toolchain = m[1]
			}
		}
	}
	return goVersion, toolchain, nil
}

func extractSARIFRuleIDs(sarifData any) map[string]struct{} {
	ruleIDs := map[string]struct{}{}
	root, ok := asDict(sarifData)
	if !ok {
		return ruleIDs
	}
	runs, ok := asList(root["runs"])
	if !ok {
		return ruleIDs
	}
	for _, runVal := range runs {
		run, ok := asDict(runVal)
		if !ok {
			continue
		}
		results, ok := asList(run["results"])
		if !ok {
			continue
		}
		for _, itemVal := range results {
			item, ok := asDict(itemVal)
			if !ok {
				continue
			}
			ruleID, ok := item["ruleId"].(string)
			if !ok || ruleID == "" {
				continue
			}
			ruleIDs[ruleID] = struct{}{}
			if ruleID != "gosec" {
				continue
			}
			message, _ := asDict(item["message"])
			msgText, _ := message["text"].(string)
			if m := gosecRulePrefixRE.FindStringSubmatch(msgText); m != nil {
				ruleIDs[m[1]] = struct{}{}
			}
		}
	}
	return ruleIDs
}

func countNonGeneratedGoFiles(projectDir string) (int, error) {
	count := 0
	err := walkProject(projectDir, func(parent, name string) bool {
		_, skip := goExcludedDirs[name]
		return skip
	}, func(dir, name string) {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return
		}
		if _, exact := goGeneratedExact[name]; exact || hasAnySuffix(name, goGeneratedSuffixes) {
			return
		}
		count++
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func listNonGeneratedPyFiles(projectDir string) ([]string, error) {
	var files []string
	err := walkProject(projectDir, func(parent, name string) bool {
		if _, skip := pyExcludedDirs[name]; skip || strings.HasSuffix(name, ".egg-info") {
			return true
		}
		return parent == "harness" && name == "corpus"
	}, func(dir, name string) {
		if !strings.HasSuffix(name, ".py") || hasAnySuffix(name, pyGeneratedSuffixes) {
			return
		}
		files = append(files, filepath.Join(dir, name))
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func walkProject(root string, skipDir func(parent, name string) bool, fileFn func(dir, name string)) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			if skipDir(filepath.Base(filepath.Dir(path)), d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		fileFn(filepath.Dir(path), d.Name())
		return nil
	})
}

func hasAnySuffix(name string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func fileHasCodeLines(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		stripped := strings.TrimSpace(sc.Text())
		if stripped != "" && !strings.HasPrefix(stripped, "#") {
			return true
		}
	}
	return false
}

func sarifContainsCanaryURI(sarifData any) bool {
	root, ok := asDict(sarifData)
	if !ok {
		return false
	}
	runs, ok := asList(root["runs"])
	if !ok {
		return false
	}
	for _, runVal := range runs {
		run, ok := asDict(runVal)
		if !ok {
			continue
		}
		results, ok := resultList(run)
		if !ok {
			continue
		}
		for _, itemVal := range results {
			item, ok := asDict(itemVal)
			if !ok {
				continue
			}
			locs, ok := asList(item["locations"])
			if !ok {
				continue
			}
			for _, locVal := range locs {
				loc, ok := asDict(locVal)
				if !ok {
					continue
				}
				phys, _ := asDict(loc["physicalLocation"])
				art, _ := asDict(phys["artifactLocation"])
				uri, _ := art["uri"].(string)
				if strings.Contains(uri, "alt-go-canary-") {
					return true
				}
			}
		}
	}
	return false
}

func resultList(run map[string]any) ([]any, bool) {
	raw, exists := run["results"]
	if !exists {
		return []any{}, true
	}
	return asList(raw)
}

func validateSARIFRun(st streams, run any, gosecSARIFPath string) bool {
	runMap, ok := asDict(run)
	if !ok {
		st.error(fmt.Sprintf("ERROR: invalid gosec SARIF at %s: run entry must be an object", gosecSARIFPath))
		return false
	}
	tool, _ := asDict(runMap["tool"])
	driver, _ := asDict(tool["driver"])
	rules, ok := asList(driver["rules"])
	if !ok {
		st.error(fmt.Sprintf("ERROR: invalid gosec SARIF at %s: missing tool.driver.rules list (SARIF 2.1.0 compliance)", gosecSARIFPath))
		return false
	}
	declared := map[string]struct{}{}
	for _, ruleVal := range rules {
		rule, ok := asDict(ruleVal)
		if !ok {
			continue
		}
		id, ok := rule["id"].(string)
		if ok && id != "" {
			declared[id] = struct{}{}
		}
	}
	artifacts, exists := runMap["artifacts"]
	hasArtifacts := exists && artifacts != nil
	results, ok := resultList(runMap)
	if !ok {
		st.error(fmt.Sprintf("ERROR: invalid gosec SARIF at %s: run.results must be a list", gosecSARIFPath))
		return false
	}
	for _, itemVal := range results {
		item, ok := asDict(itemVal)
		if !ok {
			continue
		}
		ruleID, isStr := item["ruleId"].(string)
		if isStr && ruleID != "" {
			if _, known := declared[ruleID]; !known {
				st.error(fmt.Sprintf("ERROR: invalid gosec SARIF at %s: result ruleId %s missing from tool.driver.rules", gosecSARIFPath, pyQuote(ruleID)))
				return false
			}
		}
		if hasArtifacts {
			continue
		}
		locs, ok := asList(item["locations"])
		if !ok {
			continue
		}
		for _, locVal := range locs {
			loc, ok := asDict(locVal)
			if !ok {
				continue
			}
			phys, _ := asDict(loc["physicalLocation"])
			art, _ := asDict(phys["artifactLocation"])
			if art == nil {
				continue
			}
			if _, hasIndex := art["index"]; hasIndex {
				st.error(fmt.Sprintf("ERROR: invalid gosec SARIF at %s: artifactLocation.index present without run.artifacts", gosecSARIFPath))
				return false
			}
		}
	}
	return true
}

func loadJSONFile(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func validateGosecSARIF(st streams, gosecSARIFPath string) bool {
	info, err := os.Stat(gosecSARIFPath)
	if err != nil || info.IsDir() {
		st.error(fmt.Sprintf("ERROR: missing gosec SARIF at %s", gosecSARIFPath))
		return false
	}
	sarifData, err := loadJSONFile(gosecSARIFPath)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: invalid gosec SARIF at %s: %s", gosecSARIFPath, err.Error()))
		return false
	}
	root, ok := asDict(sarifData)
	runs, runsOK := asList(root["runs"])
	if !ok || !runsOK {
		st.error(fmt.Sprintf("ERROR: invalid gosec SARIF at %s: missing top-level 'runs' list", gosecSARIFPath))
		return false
	}
	if sarifContainsCanaryURI(sarifData) {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (go): production SARIF %s contaminated by canary artifact", gosecSARIFPath))
		return false
	}
	for _, run := range runs {
		if !validateSARIFRun(st, run, gosecSARIFPath) {
			return false
		}
	}
	return true
}

func parseGosecFilesScanned(st streams, gosecOutPath, projectDir string, nonGenCount int) (int, bool) {
	info, err := os.Stat(gosecOutPath)
	if err != nil || info.IsDir() {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go): missing gosec output file %s despite %d non-generated .go files",
			gosecOutPath, nonGenCount,
		))
		return 0, false
	}
	data, err := os.ReadFile(gosecOutPath)
	if err != nil {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go): missing gosec output file %s despite %d non-generated .go files",
			gosecOutPath, nonGenCount,
		))
		return 0, false
	}
	outText := string(data)
	match := gosecFilesRE.FindStringSubmatch(outText)
	if match == nil {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go): gosec output missing 'Files:' summary despite %d non-generated .go files",
			nonGenCount,
		))
		return 0, false
	}
	filesScanned := atoi(match[1])
	if filesScanned == 0 {
		loadNote := ""
		if strings.Contains(outText, "Golang errors in file:") {
			loadNote = " (with Golang package load errors)"
		}
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go): gosec scanned 0 files%s despite %d non-generated .go files in %s",
			loadNote, nonGenCount, projectDir,
		))
		return 0, false
	}
	return filesScanned, true
}

func verifyGoPackagesLoaded(ctx context.Context, st streams, projectDir string, nonGenCount int) (int, bool) {
	goBin := look("go")
	if goBin == "" {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go): go executable not found in PATH despite %d non-generated .go files",
			nonGenCount,
		))
		return 0, false
	}
	res := runCmd(ctx, 0, goBin, []string{"list", "./..."}, projectDir, mergedEnv(map[string]string{"GOFLAGS": "-buildvcs=false"}))
	if res.err != nil {
		st.error(fmt.Sprintf("ERROR: ZERO-FILE SCAN DETECTED (go): failed to run 'go list ./...': %s", res.err.Error()))
		return 0, false
	}
	combined := res.combined()
	if res.code != 0 {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go): 'go list ./...' failed (exit %d) despite %d non-generated .go files in %s (output: %s)",
			res.code, nonGenCount, projectDir, snippet(combined),
		))
		return 0, false
	}
	var packages []string
	for _, line := range strings.Split(res.stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			packages = append(packages, line)
		}
	}
	if len(packages) == 0 {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go): 'go list ./...' resolved 0 Go packages despite %d non-generated .go files in %s (output: %s)",
			nonGenCount, projectDir, snippet(combined),
		))
		return 0, false
	}
	return len(packages), true
}

func verifyGoScan(ctx context.Context, st streams, projectDir, gosecOutPath, gosecSARIFPath string) int {
	nonGenCount, err := countNonGeneratedGoFiles(projectDir)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: ZERO-FILE SCAN DETECTED (go): %s", err.Error()))
		return exitFail
	}
	if gosecOutPath != "" {
		return verifyGoScanLegacy(st, projectDir, gosecOutPath, gosecSARIFPath, nonGenCount)
	}
	if !validateGosecSARIF(st, gosecSARIFPath) {
		return exitFail
	}
	if nonGenCount == 0 {
		st.info(fmt.Sprintf("go scan check skipped: 0 non-generated .go files in %s", projectDir))
		return exitOK
	}
	packageCount, ok := verifyGoPackagesLoaded(ctx, st, projectDir, nonGenCount)
	if !ok {
		return exitFail
	}
	st.info(fmt.Sprintf(
		"go scan check passed: resolved %d Go package(s) (%d non-generated .go files)",
		packageCount, nonGenCount,
	))
	return exitOK
}

func verifyGoScanLegacy(st streams, projectDir, gosecOutPath, gosecSARIFPath string, nonGenCount int) int {
	if nonGenCount == 0 {
		st.info(fmt.Sprintf("go scan check skipped: 0 non-generated .go files in %s", projectDir))
		return exitOK
	}
	filesScanned, ok := parseGosecFilesScanned(st, gosecOutPath, projectDir, nonGenCount)
	if !ok || !validateGosecSARIF(st, gosecSARIFPath) {
		return exitFail
	}
	st.info(fmt.Sprintf(
		"go scan check passed: gosec scanned %d files (%d non-generated .go files)",
		filesScanned, nonGenCount,
	))
	return exitOK
}

func verifyPythonFiles(st streams, projectDir string) int {
	pyFiles, err := listNonGeneratedPyFiles(projectDir)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: ZERO-FILE SCAN DETECTED (python): %s", err.Error()))
		return exitFail
	}
	if len(pyFiles) == 0 {
		st.error(fmt.Sprintf("ERROR: ZERO-FILE SCAN DETECTED (python): no non-generated .py files found in %s", projectDir))
		return exitFail
	}
	st.info(fmt.Sprintf("python source check passed: %d non-generated .py files found", len(pyFiles)))
	return exitOK
}

func verifyPythonBanditScan(st streams, projectDir, banditOutPath string) int {
	pyFiles, err := listNonGeneratedPyFiles(projectDir)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: ZERO-FILE SCAN DETECTED (python/bandit): %s", err.Error()))
		return exitFail
	}
	codeFiles := 0
	for _, path := range pyFiles {
		if fileHasCodeLines(path) {
			codeFiles++
		}
	}
	if codeFiles == 0 {
		return exitOK
	}
	info, err := os.Stat(banditOutPath)
	if err != nil || info.IsDir() {
		st.error(fmt.Sprintf("ERROR: ZERO-FILE SCAN DETECTED (python/bandit): missing bandit output file %s", banditOutPath))
		return exitFail
	}
	data, err := os.ReadFile(banditOutPath)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: ZERO-FILE SCAN DETECTED (python/bandit): missing bandit output file %s", banditOutPath))
		return exitFail
	}
	text := string(data)
	match := banditLocRE.FindStringSubmatch(text)
	if match == nil {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (python/bandit): bandit output missing 'Total lines of code:' summary despite %d non-empty .py files",
			codeFiles,
		))
		return exitFail
	}
	linesScanned := atoi(match[1])
	if linesScanned == 0 {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (python/bandit): bandit scanned 0 lines of code despite %d non-empty .py files in %s",
			codeFiles, projectDir,
		))
		return exitFail
	}
	st.info(fmt.Sprintf("bandit scan check passed: %d lines of code scanned", linesScanned))
	return exitOK
}

func parseJSONStream(raw string) ([]map[string]any, bool) {
	dec := json.NewDecoder(strings.NewReader(raw))
	var objs []map[string]any
	for {
		var value any
		err := dec.Decode(&value)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false
		}
		obj, ok := asDict(value)
		if ok {
			objs = append(objs, obj)
		}
	}
	if len(objs) == 0 {
		return nil, false
	}
	return objs, true
}

func govulncheckSARIFError(sarifPath string, requireProperties bool) string {
	info, err := os.Stat(sarifPath)
	if err != nil || info.IsDir() {
		return fmt.Sprintf("missing SARIF at %s", sarifPath)
	}
	sarifData, err := loadJSONFile(sarifPath)
	if err != nil {
		return fmt.Sprintf("invalid SARIF JSON: %s", err.Error())
	}
	root, _ := asDict(sarifData)
	runs, ok := asList(root["runs"])
	if !ok || len(runs) == 0 {
		return fmt.Sprintf("SARIF %s has missing or empty runs", sarifPath)
	}
	first, ok := asDict(runs[0])
	if !ok {
		return fmt.Sprintf("SARIF %s missing tool.driver.name == 'govulncheck'", sarifPath)
	}
	tool, _ := asDict(first["tool"])
	driver, _ := asDict(tool["driver"])
	name, _ := driver["name"].(string)
	if name != "govulncheck" {
		return fmt.Sprintf("SARIF %s missing tool.driver.name == 'govulncheck'", sarifPath)
	}
	if _, ok := asList(driver["rules"]); !ok {
		return fmt.Sprintf("SARIF %s missing tool.driver.rules list", sarifPath)
	}
	return govulncheckPropsError(driver["properties"], sarifPath, requireProperties)
}

func govulncheckPropsError(props any, sarifPath string, requireProperties bool) string {
	propMap, ok := asDict(props)
	if !ok {
		if requireProperties {
			return fmt.Sprintf("SARIF %s missing tool.driver.properties metadata", sarifPath)
		}
		return ""
	}
	for _, key := range []string{"db", "go_version"} {
		value, exists := propMap[key]
		if exists && strings.TrimSpace(scalarString(value)) == "" {
			return fmt.Sprintf("SARIF %s has empty tool.driver.properties.%s", sarifPath, key)
		}
	}
	if requireProperties && (strings.TrimSpace(scalarString(propMap["db"])) == "" || strings.TrimSpace(scalarString(propMap["go_version"])) == "") {
		return fmt.Sprintf("SARIF %s missing db or go_version in tool.driver.properties", sarifPath)
	}
	return ""
}

func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "True"
		}
		return "False"
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprint(t)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func validateGovulncheckSARIF(st streams, sarifPath string, requireProperties bool) bool {
	if errText := govulncheckSARIFError(sarifPath, requireProperties); errText != "" {
		st.error("ERROR: CANARY AUDIT FAILED (go/govulncheck): " + errText)
		return false
	}
	return true
}

func extractGovulncheckJSONSections(path string) (map[string]any, map[string]any, string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, nil, fmt.Sprintf("missing JSON stream output at %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Sprintf("missing JSON stream output at %s", path)
	}
	objs, ok := parseJSONStream(string(data))
	if !ok {
		return nil, nil, fmt.Sprintf("invalid or empty JSON stream at %s", path)
	}
	var configObj, sbomObj map[string]any
	for _, obj := range objs {
		if cfg, ok := asDict(obj["config"]); ok {
			configObj = cfg
		}
		if sbom, ok := asDict(obj["SBOM"]); ok {
			sbomObj = sbom
		}
	}
	return configObj, sbomObj, ""
}

func verifyGovulncheckSBOM(st streams, projectDir string, sbomObj map[string]any, nonGenCount int) (int, int, bool) {
	if sbomObj == nil {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go/govulncheck): missing SBOM object despite %d non-generated .go files in %s",
			nonGenCount, projectDir,
		))
		return 0, 0, false
	}
	modules, _ := asList(sbomObj["modules"])
	roots, rootsOK := asList(sbomObj["roots"])
	packages, _ := asList(sbomObj["packages"])
	modCount := len(modules)
	pkgCount := 0
	if rootsOK && len(roots) > 0 {
		pkgCount = len(roots)
	} else {
		pkgCount = len(packages)
	}
	if modCount == 0 || pkgCount == 0 {
		st.error(fmt.Sprintf(
			"ERROR: ZERO-FILE SCAN DETECTED (go/govulncheck): govulncheck SBOM scanned %d module(s) and %d package(s) despite %d non-generated .go files in %s",
			modCount, pkgCount, nonGenCount, projectDir,
		))
		return 0, 0, false
	}
	return modCount, pkgCount, true
}

func verifyGovulncheckSARIFOnly(ctx context.Context, st streams, projectDir string, nonGenCount int) int {
	if nonGenCount == 0 {
		st.info(fmt.Sprintf("govulncheck scan check passed: valid SARIF metadata (0 non-generated .go files in %s)", projectDir))
		return exitOK
	}
	pkgCount, ok := verifyGoPackagesLoaded(ctx, st, projectDir, nonGenCount)
	if !ok {
		return exitFail
	}
	st.info(fmt.Sprintf("govulncheck scan check passed: valid SARIF DB metadata, %d package(s) resolved", pkgCount))
	return exitOK
}

func verifyGovulncheckJSONStream(st streams, projectDir, jsonPath string, nonGenCount int) int {
	configObj, sbomObj, streamErr := extractGovulncheckJSONSections(jsonPath)
	if streamErr != "" || configObj == nil {
		detail := streamErr
		if detail == "" {
			detail = "JSON output missing 'config' vulnerability DB metadata"
		}
		st.error("ERROR: CANARY AUDIT FAILED (go/govulncheck): " + detail)
		return exitFail
	}
	dbVal := strings.TrimSpace(scalarString(configObj["db"]))
	goVer := strings.TrimSpace(scalarString(configObj["go_version"]))
	if dbVal == "" || goVer == "" {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (go/govulncheck): config missing required db/go_version metadata (db=%s, go_version=%s)",
			pyQuote(dbVal), pyQuote(goVer),
		))
		return exitFail
	}
	if nonGenCount == 0 {
		st.info(fmt.Sprintf("govulncheck scan check passed: db=%s (%s), 0 non-generated .go files in %s", dbVal, goVer, projectDir))
		return exitOK
	}
	modCount, pkgCount, ok := verifyGovulncheckSBOM(st, projectDir, sbomObj, nonGenCount)
	if !ok {
		return exitFail
	}
	st.info(fmt.Sprintf(
		"govulncheck scan check passed: db=%s (%s), %d module(s), %d root package(s)",
		dbVal, goVer, modCount, pkgCount,
	))
	return exitOK
}

func verifyGoGovulncheck(ctx context.Context, st streams, projectDir, sarifPath, jsonPath string) int {
	if !validateGovulncheckSARIF(st, sarifPath, jsonPath == "") {
		return exitFail
	}
	nonGenCount, err := countNonGeneratedGoFiles(projectDir)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: ZERO-FILE SCAN DETECTED (go/govulncheck): %s", err.Error()))
		return exitFail
	}
	if jsonPath == "" {
		return verifyGovulncheckSARIFOnly(ctx, st, projectDir, nonGenCount)
	}
	return verifyGovulncheckJSONStream(st, projectDir, jsonPath, nonGenCount)
}

func asDict(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asList(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case nil:
		return nil, false
	default:
		return nil, false
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func sortedSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func anyMarker(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
