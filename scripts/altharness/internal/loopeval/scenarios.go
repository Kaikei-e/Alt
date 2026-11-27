package loopeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"alt/scripts/altharness/internal/execx"
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/packet"
	"alt/scripts/altharness/internal/tomlx"
)

type corpusMeta struct {
	ID         string `toml:"id"`
	Title      string `toml:"title"`
	Language   string `toml:"language"`
	BeforeFile string `toml:"before_file"`
	AfterFile  string `toml:"after_file"`
}

func scenariosDir(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(scenariosRelDir))
}

func corpusDir(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(corpusRelDir))
}

func scenarioNumericID(id string) int {
	m := scenarioIDRE.FindStringSubmatch(id)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func discoverScenarioDirs(repoRoot string) ([]string, []Issue, error) {
	base := scenariosDir(repoRoot)
	st, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, []Issue{{
				Code:    "SCENARIOS_DIR_MISSING",
				Path:    scenariosRelDir,
				Message: fmt.Sprintf("scenarios directory not found: %s", base),
			}}, nil
		}
		return nil, nil, err
	}
	if !st.IsDir() {
		return nil, nil, fmt.Errorf("scenarios path is not a directory: %s", base)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, nil, err
	}
	var ids []string
	var issues []Issue
	for _, ent := range entries {
		name := ent.Name()
		if _, skip := ignoredScenarioRootFiles[name]; skip {
			continue
		}
		if ent.Type()&os.ModeSymlink != 0 {
			issues = append(issues, Issue{
				Code:    "UNSAFE_SCENARIO_ENTRY",
				Path:    filepath.ToSlash(filepath.Join(scenariosRelDir, name)),
				Message: fmt.Sprintf("scenario entry %q must not be a symlink", name),
			})
			continue
		}
		if !ent.IsDir() || !scenarioIDRE.MatchString(name) {
			issues = append(issues, Issue{
				Code:    "UNEXPECTED_SCENARIO_DIR",
				Path:    filepath.ToSlash(filepath.Join(scenariosRelDir, name)),
				Message: fmt.Sprintf("unexpected entry %q in %s (expected E-NN directory)", name, scenariosRelDir),
			})
			continue
		}
		ids = append(ids, name)
	}
	sort.Slice(ids, func(i, j int) bool {
		ni, nj := scenarioNumericID(ids[i]), scenarioNumericID(ids[j])
		if ni != nj {
			return ni < nj
		}
		return ids[i] < ids[j]
	})
	return ids, issues, nil
}

func loadScenario(repoRoot, id string) (Scenario, error) {
	if !scenarioIDRE.MatchString(id) {
		return Scenario{}, fmt.Errorf("invalid scenario id %q (expected E-NN)", id)
	}
	dir := filepath.Join(scenariosDir(repoRoot), id)
	if isSymlink(dir) {
		return Scenario{}, fmt.Errorf("scenario directory must not be a symlink: %s", dir)
	}
	tomlPath := filepath.Join(dir, "scenario.toml")
	if !isRegularNonSymlink(tomlPath) {
		return Scenario{}, fmt.Errorf("scenario.toml not found: %s", tomlPath)
	}
	raw, err := readFileContainment(dir, "scenario.toml")
	if err != nil {
		return Scenario{}, fmt.Errorf("read %s: %w", tomlPath, err)
	}
	var sc Scenario
	if err := tomlx.Unmarshal(raw, &sc); err != nil {
		return Scenario{}, fmt.Errorf("parse %s: %w", tomlPath, err)
	}
	sc.Dir = dir
	if sc.RequiredReads == nil {
		sc.RequiredReads = []string{}
	}
	return sc, nil
}

func loadCorpusCase(repoRoot, corpusID string) (corpusMeta, string, error) {
	if !corpusIDRE.MatchString(corpusID) {
		return corpusMeta{}, "", fmt.Errorf("invalid corpus_id %q", corpusID)
	}
	caseDir := filepath.Join(corpusDir(repoRoot), corpusID)
	metaPath := filepath.Join(caseDir, "meta.toml")
	if !isRegularNonSymlink(metaPath) {
		return corpusMeta{}, caseDir, fmt.Errorf("corpus meta.toml not found: %s", metaPath)
	}
	raw, err := readFileContainment(caseDir, "meta.toml")
	if err != nil {
		return corpusMeta{}, caseDir, err
	}
	var cm corpusMeta
	if err := tomlx.Unmarshal(raw, &cm); err != nil {
		return corpusMeta{}, caseDir, err
	}
	return cm, caseDir, nil
}

func validateAllScenarios(ctx context.Context, repoRoot string, minScenarios int) (ValidationReport, error) {
	rep := ValidationReport{
		ScenariosDir: scenariosDir(repoRoot),
		MinScenarios: minScenarios,
		Errors:       []Issue{},
	}
	ids, dirIssues, err := discoverScenarioDirs(repoRoot)
	if err != nil {
		return rep, err
	}
	rep.Errors = append(rep.Errors, dirIssues...)
	rep.ScenarioCount = len(ids)
	if len(ids) < minScenarios {
		rep.Errors = append(rep.Errors, Issue{
			Code:    "INSUFFICIENT_SCENARIOS",
			Path:    scenariosRelDir,
			Message: fmt.Sprintf("found %d scenario(s), minimum required is %d", len(ids), minScenarios),
		})
	}

	seenNeutral := map[string]string{}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		scPath := filepath.ToSlash(filepath.Join(scenariosRelDir, id, "scenario.toml"))
		sc, err := loadScenario(repoRoot, id)
		if err != nil {
			rep.Errors = append(rep.Errors, Issue{
				Code:    "SCENARIO_TOML_INVALID",
				Path:    scPath,
				Message: err.Error(),
			})
			continue
		}
		validateSingleScenario(ctx, repoRoot, id, sc, seenNeutral, &rep.Errors)
	}
	rep.Valid = len(rep.Errors) == 0
	return rep, nil
}

func validateSingleScenario(ctx context.Context, repoRoot, dirID string, sc Scenario, seenNeutral map[string]string, issues *[]Issue) {
	scPath := filepath.ToSlash(filepath.Join(scenariosRelDir, dirID, "scenario.toml"))
	if sc.ID != dirID {
		*issues = append(*issues, Issue{
			Code:    "SCENARIO_ID_MISMATCH",
			Path:    scPath,
			Message: fmt.Sprintf("scenario.toml id %q does not match directory %q", sc.ID, dirID),
		})
	}
	if strings.TrimSpace(sc.Title) == "" {
		*issues = append(*issues, Issue{
			Code:    "EMPTY_TITLE",
			Path:    scPath,
			Message: "scenario title must be non-empty",
		})
	}
	if _, ok := validLanguages[sc.Language]; !ok {
		*issues = append(*issues, Issue{
			Code:    "INVALID_LANGUAGE",
			Path:    scPath,
			Message: fmt.Sprintf("unsupported language %q", sc.Language),
		})
	}
	if !commitSHARE.MatchString(sc.BaseCommit) || allZeroSHARE.MatchString(sc.BaseCommit) {
		*issues = append(*issues, Issue{
			Code:    "INVALID_BASE_COMMIT",
			Path:    scPath,
			Message: fmt.Sprintf("base_commit %q must be a non-zero 7..40 hex git SHA", sc.BaseCommit),
		})
	}

	if msg, ok := checkNeutralName(sc.NeutralWorkspaceName); !ok {
		*issues = append(*issues, Issue{
			Code:    "NEUTRALITY_VIOLATION",
			Path:    scPath,
			Message: msg,
		})
	} else if prev, dup := seenNeutral[sc.NeutralWorkspaceName]; dup {
		*issues = append(*issues, Issue{
			Code:    "DUPLICATE_NEUTRAL_WORKSPACE_NAME",
			Path:    scPath,
			Message: fmt.Sprintf("neutral_workspace_name %q duplicates scenario %s", sc.NeutralWorkspaceName, prev),
		})
	} else {
		seenNeutral[sc.NeutralWorkspaceName] = dirID
	}

	if unsafeRelPath(sc.TargetFile) {
		*issues = append(*issues, Issue{
			Code:    "UNSAFE_TARGET_FILE",
			Path:    scPath,
			Message: fmt.Sprintf("target_file %q must be a safe relative path", sc.TargetFile),
		})
	} else {
		lowerTarget := strings.ToLower(sc.TargetFile)
		for _, tok := range forbiddenNeutralTokens {
			if strings.Contains(lowerTarget, tok) {
				*issues = append(*issues, Issue{
					Code:    "NEUTRALITY_VIOLATION",
					Path:    scPath,
					Message: fmt.Sprintf("target_file %q contains forbidden observer-effect substring %q", sc.TargetFile, tok),
				})
				break
			}
		}
	}

	if len(sc.RequiredReads) == 0 {
		*issues = append(*issues, Issue{
			Code:    "EMPTY_REQUIRED_READS",
			Path:    scPath,
			Message: "required_reads must contain at least one relative path",
		})
	} else {
		seenReads := map[string]struct{}{}
		for _, r := range sc.RequiredReads {
			if unsafeRelPath(r) {
				*issues = append(*issues, Issue{
					Code:    "UNSAFE_REQUIRED_READ",
					Path:    scPath,
					Message: fmt.Sprintf("required_reads entry %q must be a safe relative path", r),
				})
				continue
			}
			if _, dup := seenReads[r]; dup {
				*issues = append(*issues, Issue{
					Code:    "DUPLICATE_REQUIRED_READ",
					Path:    scPath,
					Message: fmt.Sprintf("duplicate required_reads entry %q", r),
				})
			}
			seenReads[r] = struct{}{}
		}
	}

	// Corpus reference check.
	cm, caseDir, err := loadCorpusCase(repoRoot, sc.CorpusID)
	if err != nil {
		*issues = append(*issues, Issue{
			Code:    "MISSING_CORPUS_CASE",
			Path:    scPath,
			Message: fmt.Sprintf("corpus_id %q check failed: %v", sc.CorpusID, err),
		})
	} else {
		if cm.ID != sc.CorpusID {
			*issues = append(*issues, Issue{
				Code:    "CORPUS_ID_MISMATCH",
				Path:    scPath,
				Message: fmt.Sprintf("corpus meta.toml id %q does not match %q", cm.ID, sc.CorpusID),
			})
		}
		if cm.Language != "" && sc.Language != "" && cm.Language != sc.Language {
			*issues = append(*issues, Issue{
				Code:    "CORPUS_LANGUAGE_MISMATCH",
				Path:    scPath,
				Message: fmt.Sprintf("scenario language %q differs from corpus %s language %q", sc.Language, sc.CorpusID, cm.Language),
			})
		}
		for _, pair := range [][2]string{{"before_file", cm.BeforeFile}, {"after_file", cm.AfterFile}} {
			if unsafeRelPath(pair[1]) {
				*issues = append(*issues, Issue{
					Code:    "MISSING_CORPUS_FILE",
					Path:    scPath,
					Message: fmt.Sprintf("corpus %s has invalid %s %q", sc.CorpusID, pair[0], pair[1]),
				})
				continue
			}
			full := filepath.Join(caseDir, filepath.FromSlash(pair[1]))
			if !isRegularNonSymlink(full) || !withinDir(full, caseDir) {
				*issues = append(*issues, Issue{
					Code:    "MISSING_CORPUS_FILE",
					Path:    scPath,
					Message: fmt.Sprintf("corpus %s %s not found: %s", sc.CorpusID, pair[0], pair[1]),
				})
			}
		}
	}

	// Characterization test checks.
	if sc.HasCharTest {
		if strings.TrimSpace(sc.CharTestCommand) == "" {
			*issues = append(*issues, Issue{
				Code:    "EMPTY_CHAR_TEST_COMMAND",
				Path:    scPath,
				Message: "has_char_test=true requires non-empty char_test_command",
			})
		} else {
			if bad := matchForbiddenAutofix(sc.CharTestCommand); bad != "" {
				*issues = append(*issues, Issue{
					Code:    "FORBIDDEN_AUTOFIX_COMMAND",
					Path:    scPath,
					Message: fmt.Sprintf("char_test_command contains forbidden autofix (%s)", bad),
				})
			}
			if hasBackgroundExecution(sc.CharTestCommand) {
				*issues = append(*issues, Issue{
					Code:    "FORBIDDEN_BACKGROUND_EXECUTION",
					Path:    scPath,
					Message: "char_test_command must run synchronously in foreground",
				})
			}
			if strings.Contains(sc.CharTestCommand, "test_characterization.sh") {
				if _, ok := findCharTestScript(sc, caseDir); !ok {
					*issues = append(*issues, Issue{
						Code:    "MISSING_CHAR_TEST_SCRIPT",
						Path:    scPath,
						Message: "char_test_command references test_characterization.sh but script not found in scenario or corpus dir",
					})
				}
			}
		}
	}

	// Packet validation and observer-effect neutrality check.
	pktPath := filepath.Join(sc.Dir, "packet.md")
	pktRel := filepath.ToSlash(filepath.Join(scenariosRelDir, dirID, "packet.md"))
	if !isRegularNonSymlink(pktPath) {
		*issues = append(*issues, Issue{
			Code:    "MISSING_PACKET",
			Path:    pktRel,
			Message: "scenario packet.md not found",
		})
		return
	}
	rawPkt, err := readFileContainment(sc.Dir, "packet.md")
	if err != nil {
		*issues = append(*issues, Issue{
			Code:    "PACKET_READ_ERROR",
			Path:    pktRel,
			Message: err.Error(),
		})
		return
	}
	pktText := string(rawPkt)
	for _, phrase := range checkObserverEffectLeaks(pktText) {
		*issues = append(*issues, Issue{
			Code:    "OBSERVER_EFFECT_LEAK",
			Path:    pktRel,
			Message: fmt.Sprintf("packet.md contains observer-effect leak phrase %q", phrase),
		})
	}
	if sc.CorpusID != "" && strings.Contains(pktText, sc.CorpusID) {
		*issues = append(*issues, Issue{
			Code:    "OBSERVER_EFFECT_LEAK",
			Path:    pktRel,
			Message: fmt.Sprintf("packet.md leaks corpus_id %q", sc.CorpusID),
		})
	}
	if sc.ID != "" && strings.Contains(pktText, sc.ID) {
		*issues = append(*issues, Issue{
			Code:    "OBSERVER_EFFECT_LEAK",
			Path:    pktRel,
			Message: fmt.Sprintf("packet.md leaks scenario_id %q", sc.ID),
		})
	}

	var pOut, pErr bytes.Buffer
	rc := packet.Run(ctx, []string{"validate-packet", pktPath, "--format", "json"}, &pOut, &pErr)
	if rc != 0 {
		var pktRes struct {
			Errors []Issue `json:"errors"`
		}
		if json.Unmarshal(pOut.Bytes(), &pktRes) == nil && len(pktRes.Errors) > 0 {
			for _, item := range pktRes.Errors {
				*issues = append(*issues, Issue{
					Code:    "PACKET_" + item.Code,
					Path:    pktRel + ":" + item.Path,
					Message: item.Message,
				})
			}
		} else {
			*issues = append(*issues, Issue{
				Code:    "PACKET_INVALID",
				Path:    pktRel,
				Message: fmt.Sprintf("packet validate-packet failed (rc=%d): %s", rc, strings.TrimSpace(pErr.String())),
			})
		}
	}
}

func findCharTestScript(sc Scenario, corpusCaseDir string) (string, bool) {
	cand := filepath.Join(sc.Dir, "test_characterization.sh")
	if isRegularNonSymlink(cand) {
		return cand, true
	}
	if corpusCaseDir != "" {
		fallback := filepath.Join(corpusCaseDir, "test_characterization.sh")
		if isRegularNonSymlink(fallback) {
			return fallback, true
		}
	}
	return "", false
}

func stageScenario(ctx context.Context, repoRoot, scenarioID, outDir string) (StageResult, error) {
	sc, err := loadScenario(repoRoot, scenarioID)
	if err != nil {
		return StageResult{}, err
	}
	if msg, ok := checkNeutralName(sc.NeutralWorkspaceName); !ok {
		return StageResult{}, errors.New(msg)
	}
	if unsafeRelPath(sc.TargetFile) {
		return StageResult{}, fmt.Errorf("unsafe target_file %q", sc.TargetFile)
	}
	cm, caseDir, err := loadCorpusCase(repoRoot, sc.CorpusID)
	if err != nil {
		return StageResult{}, fmt.Errorf("load corpus %s: %w", sc.CorpusID, err)
	}
	if unsafeRelPath(cm.BeforeFile) {
		return StageResult{}, fmt.Errorf("corpus %s has unsafe before_file %q", sc.CorpusID, cm.BeforeFile)
	}
	beforePath := filepath.Join(caseDir, filepath.FromSlash(cm.BeforeFile))
	if !isRegularNonSymlink(beforePath) || !withinDir(beforePath, caseDir) {
		return StageResult{}, fmt.Errorf("corpus before_file missing: %s", beforePath)
	}

	if strings.TrimSpace(outDir) == "" {
		return StageResult{}, errors.New("--out-dir must be non-empty")
	}
	if err := checkOutDirNeutrality(outDir); err != nil {
		return StageResult{}, err
	}
	if st, err := os.Lstat(outDir); err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return StageResult{}, fmt.Errorf("refusing symlink --out-dir: %s", outDir)
		}
		if !st.IsDir() {
			return StageResult{}, fmt.Errorf("--out-dir is not a directory: %s", outDir)
		}
	} else if os.IsNotExist(err) {
		if mkErr := os.MkdirAll(outDir, fsperm.Dir); mkErr != nil {
			return StageResult{}, fmt.Errorf("create --out-dir: %w", mkErr)
		}
	} else {
		return StageResult{}, fmt.Errorf("stat --out-dir: %w", err)
	}

	resolvedOut, err := resolvePath(outDir)
	if err != nil {
		return StageResult{}, err
	}
	wsDir := filepath.Join(resolvedOut, sc.NeutralWorkspaceName)
	if !withinDir(wsDir, resolvedOut) || wsDir == resolvedOut {
		return StageResult{}, fmt.Errorf("staged workspace path escapes --out-dir: %s", wsDir)
	}
	if _, err := os.Lstat(wsDir); err == nil {
		return StageResult{}, fmt.Errorf("staged workspace already exists (refusing overwrite): %s", wsDir)
	} else if !os.IsNotExist(err) {
		return StageResult{}, err
	}

	if err := os.MkdirAll(wsDir, fsperm.Dir); err != nil {
		return StageResult{}, fmt.Errorf("create staged workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(wsDir)
		}
	}()

	// Copy corpus before_file into neutral target_file path, stripping internal
	// harness label comments (e.g. ARCH-02) so zero internal labels leak.
	targetDest := filepath.Join(wsDir, filepath.FromSlash(sc.TargetFile))
	if !withinDir(targetDest, wsDir) {
		return StageResult{}, fmt.Errorf("target_file escapes staged workspace: %s", sc.TargetFile)
	}
	if err := os.MkdirAll(filepath.Dir(targetDest), fsperm.Dir); err != nil {
		return StageResult{}, fmt.Errorf("create target parent: %w", err)
	}
	beforeBytes, err := readFileContainment(caseDir, cm.BeforeFile)
	if err != nil {
		return StageResult{}, fmt.Errorf("read corpus before_file: %w", err)
	}
	beforeBytes = stripHarnessLabelComments(beforeBytes)
	if strings.HasSuffix(sc.TargetFile, ".sh") {
		if err := fsperm.WriteExecutable(targetDest, beforeBytes); err != nil {
			return StageResult{}, fmt.Errorf("write target script: %w", err)
		}
	} else if err := writeFileContainment(wsDir, sc.TargetFile, beforeBytes, fsperm.File); err != nil {
		return StageResult{}, fmt.Errorf("write target file: %w", err)
	}

	// NOTE: test_characterization.sh is intentionally NEVER copied into wsDir;
	// grading executes the trusted oracle directly from sc.Dir against wsDir.

	// Materialize neutral ./scripts/lint/run.sh entrypoint supporting --policy
	// and deterministic target-rule verification.
	runShDir := filepath.Join(wsDir, "scripts", "lint")
	if err := os.MkdirAll(runShDir, fsperm.Dir); err != nil {
		return StageResult{}, fmt.Errorf("create scripts/lint dir: %w", err)
	}
	runShBody := renderNeutralLintShim(sc)
	if err := fsperm.WriteExecutable(filepath.Join(runShDir, "run.sh"), []byte(runShBody)); err != nil {
		return StageResult{}, fmt.Errorf("write scripts/lint/run.sh: %w", err)
	}

	// Write a minimal neutral .gitignore so runtime bytecode/caches never dirty the git diff.
	gitignoreBody := "__pycache__/\n*.py[cod]\n.pytest_cache/\n"
	if err := os.WriteFile(filepath.Join(wsDir, ".gitignore"), []byte(gitignoreBody), fsperm.File); err != nil {
		return StageResult{}, fmt.Errorf("write .gitignore: %w", err)
	}

	// Initialize neutral git repository and create baseline commit.
	gitSteps := [][]string{
		{"init", "-q"},
		{"config", "user.name", "Workspace Init"},
		{"config", "user.email", "dev@example.com"},
		{"config", "commit.gpgsign", "false"},
		{"add", "-A"},
		{"commit", "-q", "-m", "initial baseline"},
	}
	for _, step := range gitSteps {
		if _, stderr, code, runErr := runGit(ctx, wsDir, step...); runErr != nil {
			return StageResult{}, fmt.Errorf("git %s: %w", step[0], runErr)
		} else if code != 0 {
			return StageResult{}, fmt.Errorf("git %s failed (rc=%d): %s", step[0], code, strings.TrimSpace(stderr))
		}
	}
	shaOut, stderr, code, err := runGit(ctx, wsDir, "rev-parse", "HEAD")
	if err != nil {
		return StageResult{}, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	if code != 0 {
		return StageResult{}, fmt.Errorf("git rev-parse HEAD failed: %s", strings.TrimSpace(stderr))
	}
	baseSHA := strings.TrimSpace(shaOut)
	cleanup = false

	return StageResult{
		ScenarioID:           sc.ID,
		WorkspaceDir:         wsDir,
		NeutralWorkspaceName: sc.NeutralWorkspaceName,
		BaseCommit:           baseSHA,
		TargetFile:           sc.TargetFile,
	}, nil
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func renderNeutralLintShim(sc Scenario) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("set -euo pipefail\n")
	b.WriteString("policy=\"${LINT_POLICY:-fix-existing}\"\n")
	b.WriteString("target=\"\"\n")
	b.WriteString("while [[ $# -gt 0 ]]; do\n")
	b.WriteString("  case \"$1\" in\n")
	b.WriteString("    --policy)\n")
	b.WriteString("      if [[ $# -lt 2 ]]; then\n")
	b.WriteString("        echo \"error: --policy requires an argument\" >&2\n")
	b.WriteString("        exit 2\n")
	b.WriteString("      fi\n")
	b.WriteString("      policy=\"$2\"\n")
	b.WriteString("      shift 2\n")
	b.WriteString("      ;;\n")
	b.WriteString("    --policy=*)\n")
	b.WriteString("      policy=\"${1#--policy=}\"\n")
	b.WriteString("      shift\n")
	b.WriteString("      ;;\n")
	b.WriteString("    -*)\n")
	b.WriteString("      echo \"error: unknown flag: $1\" >&2\n")
	b.WriteString("      exit 2\n")
	b.WriteString("      ;;\n")
	b.WriteString("    *)\n")
	b.WriteString("      if [[ -n \"$target\" ]]; then\n")
	b.WriteString("        echo \"error: unexpected extra argument: $1\" >&2\n")
	b.WriteString("        exit 2\n")
	b.WriteString("      fi\n")
	b.WriteString("      target=\"$1\"\n")
	b.WriteString("      shift\n")
	b.WriteString("      ;;\n")
	b.WriteString("  esac\n")
	b.WriteString("done\n")
	b.WriteString("if [[ \"$policy\" != \"fix-existing\" && \"$policy\" != \"block-new\" ]]; then\n")
	b.WriteString("  echo \"error: invalid --policy: $policy\" >&2\n")
	b.WriteString("  exit 2\n")
	b.WriteString("fi\n")
	b.WriteString("if [[ \"$target\" != \"" + sc.NeutralWorkspaceName + "\" ]]; then\n")
	b.WriteString("  echo \"error: unknown target: $target\" >&2\n")
	b.WriteString("  exit 2\n")
	b.WriteString("fi\n")
	b.WriteString("ROOT_DIR=\"$(cd \"$(dirname \"$0\")/../..\" && pwd)\"\n")
	b.WriteString("cd \"$ROOT_DIR\"\n")
	b.WriteString("TARGET_REL=" + shellSingleQuote(sc.TargetFile) + "\n")
	b.WriteString("TARGET=\"$ROOT_DIR/$TARGET_REL\"\n")
	b.WriteString("if [[ ! -f \"$TARGET\" ]]; then\n")
	b.WriteString("  echo \"error: missing file $TARGET_REL\" >&2\n")
	b.WriteString("  exit 1\n")
	b.WriteString("fi\n")

	switch sc.Language {
	case "go":
		b.WriteString("gofmt -e \"$TARGET\" >/dev/null\n")
	case "python":
		b.WriteString("python3 -B -c 'import ast, pathlib, sys; ast.parse(pathlib.Path(sys.argv[1]).read_text(encoding=\"utf-8\"))' \"$TARGET\"\n")
	case "shell":
		b.WriteString("bash -n \"$TARGET\"\n")
	}

	switch sc.LintRule {
	case "errorlint":
		b.WriteString("if [[ \"$policy\" == \"fix-existing\" ]]; then\n")
		b.WriteString("  if grep -Eq 'err\\.\\(\\*' \"$TARGET\"; then\n")
		b.WriteString("    echo \"$TARGET_REL: errorlint: type assertion on error fails on wrapped errors; use errors.As\" >&2\n")
		b.WriteString("    exit 1\n")
		b.WriteString("  fi\n")
		b.WriteString("else\n")
		b.WriteString("  if git diff HEAD -- \"$TARGET_REL\" | grep -Eq '^\\+[^+].*err\\.\\(\\*'; then\n")
		b.WriteString("    echo \"$TARGET_REL: errorlint: new type assertion on error introduced\" >&2\n")
		b.WriteString("    exit 1\n")
		b.WriteString("  fi\n")
		b.WriteString("fi\n")
	case "python-complexity":
		b.WriteString("if [[ \"$policy\" == \"fix-existing\" ]]; then\n")
		b.WriteString("  python3 -B - \"$TARGET\" <<'PY'\n")
		b.WriteString("import ast, pathlib, sys\n")
		b.WriteString("tree = ast.parse(pathlib.Path(sys.argv[1]).read_text(encoding=\"utf-8\"))\n")
		b.WriteString("defs = [n for n in ast.walk(tree) if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef))]\n")
		b.WriteString("if len(defs) < 3:\n")
		b.WriteString("    sys.stderr.write(\"C901: acquire requires helper decomposition\\n\")\n")
		b.WriteString("    sys.exit(1)\n")
		b.WriteString("PY\n")
		b.WriteString("fi\n")
	case "python-aclosing":
		b.WriteString("python3 -B - \"$TARGET\" <<'PY'\n")
		b.WriteString("import ast, pathlib, sys\n")
		b.WriteString("tree = ast.parse(pathlib.Path(sys.argv[1]).read_text(encoding=\"utf-8\"))\n")
		b.WriteString("for node in ast.walk(tree):\n")
		b.WriteString("    if isinstance(node, ast.Try):\n")
		b.WriteString("        for stmt in node.finalbody:\n")
		b.WriteString("            for sub in ast.walk(stmt):\n")
		b.WriteString("                if isinstance(sub, ast.Attribute) and sub.attr == \"aclose\":\n")
		b.WriteString("                    sys.stderr.write(\"lint: avoid manual aclose() in finally; use contextlib.aclosing\\n\")\n")
		b.WriteString("                    sys.exit(1)\n")
		b.WriteString("PY\n")
	case "go-httpserver-g112":
		b.WriteString("if grep -Eq '&http\\.Server\\{' \"$TARGET\"; then\n")
		b.WriteString("  echo \"$TARGET_REL: G112: construct servers via httpserver.New rather than raw &http.Server{}\" >&2\n")
		b.WriteString("  exit 1\n")
		b.WriteString("fi\n")
	case "go-body-limit":
		b.WriteString("if grep -Eq 'BodyLimit\\(1\\s*<<\\s*20\\)' \"$TARGET\"; then\n")
		b.WriteString("  echo \"$TARGET_REL: lint: decimal 1M body limit drifted to 1 << 20\" >&2\n")
		b.WriteString("  exit 1\n")
		b.WriteString("fi\n")
	case "generated-code-guard":
		b.WriteString("if grep -q 'DO NOT EDIT' \"$TARGET\" && grep -q 'v2\\.5\\.1' \"$TARGET\" && grep -q 'github\\.com/labstack/echo/v5' \"$TARGET\"; then\n")
		b.WriteString("  echo \"$TARGET_REL: lint: generated file must not be hand-edited\" >&2\n")
		b.WriteString("  exit 1\n")
		b.WriteString("fi\n")
	case "compose-var-check":
		b.WriteString("if grep -q -- '--no-env-resolution' \"$TARGET\"; then\n")
		b.WriteString("  echo \"$TARGET_REL: lint: --no-env-resolution disables env_file checks\" >&2\n")
		b.WriteString("  exit 1\n")
		b.WriteString("fi\n")
	}
	b.WriteString("exit 0\n")
	return b.String()
}

func runGit(ctx context.Context, repo string, args ...string) (string, string, int, error) {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(defaultCharTimeoutS)*time.Second)
	defer cancel()
	argv := make([]string, 0, len(args)+3)
	argv = append(argv, "git", "-C", repo)
	argv = append(argv, args...)
	return execx.Combined(runCtx, execx.Cmd{Args: argv})
}
