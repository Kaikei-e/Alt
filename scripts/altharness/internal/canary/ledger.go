package canary

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var (
	validLedgerKinds = map[string]struct{}{
		"go": {}, "python": {}, "rust": {}, "frontend": {}, "deno": {}, "proto": {}, "named": {},
	}
	validScannerStatuses = map[string]struct{}{
		"canary": {}, "exempt": {}, "not-applicable": {},
	}
	validCanaryCommands = map[string]struct{}{
		"go-canary": {}, "go-verify-govulncheck": {}, "python-canary": {}, "rust-canary": {},
		"frontend-canary": {}, "deno-canary": {}, "proto-canary": {}, "actionlint-canary": {},
	}
	requiredCanaryScannersByKind = map[string][]string{
		"go":       {"golangci-lint", "gosec", "govulncheck"},
		"python":   {"ruff"},
		"rust":     {"clippy"},
		"frontend": {"biome"},
		"deno":     {"deno-lint"},
		"proto":    {"buf-lint"},
		"named":    {},
	}
	sortedLedgerKinds = []string{"deno", "frontend", "go", "named", "proto", "python", "rust"}
	sortedStatuses    = []string{"canary", "exempt", "not-applicable"}

	gitSectionRE = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*(?:[#;].*)?$`)
	gitKVRE      = regexp.MustCompile(`^\s*([A-Za-z0-9-]+)(?:\s*=\s*(.*?))?\s*(?:[#;].*)?$`)
)

type ledgerTarget struct {
	Kind     string                  `toml:"kind"`
	Scanners map[string]scannerEntry `toml:"scanners"`
}

type scannerEntry struct {
	Status string `toml:"status"`
	Canary string `toml:"canary"`
	Reason string `toml:"reason"`
}

type ledgerDoc struct {
	Targets map[string]ledgerTarget `toml:"targets"`
}

func readPythonDevDeps(pyprojectPath string) (map[string]struct{}, error) {
	data, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", pyprojectPath, err)
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid TOML in %s: %w", pyprojectPath, err)
	}
	names := map[string]struct{}{}
	groups, _ := root["dependency-groups"].(map[string]any)
	dev, _ := groups["dev"].([]any)
	for _, entry := range dev {
		text, ok := entry.(string)
		if !ok {
			continue
		}
		parts := depNameSplitRE.Split(strings.TrimSpace(text), 2)
		rawName := ""
		if len(parts) > 0 {
			rawName = parts[0]
		}
		if rawName == "" {
			continue
		}
		names[strings.ToLower(depNormRE.ReplaceAllString(rawName, "-"))] = struct{}{}
	}
	return names, nil
}

func validateSingleScannerEntry(target, scannerName string, cfg scannerEntry, present bool) string {
	if !present {
		return fmt.Sprintf("target %s scanner %s: entry must be a table", pyQuote(target), pyQuote(scannerName))
	}
	if _, ok := validScannerStatuses[cfg.Status]; !ok {
		return fmt.Sprintf(
			"target %s scanner %s: invalid status %s (expected one of %s)",
			pyQuote(target), pyQuote(scannerName), pyQuote(cfg.Status), pyList(sortedStatuses),
		)
	}
	if cfg.Status == "canary" {
		if _, ok := validCanaryCommands[cfg.Canary]; !ok {
			return fmt.Sprintf(
				"target %s scanner %s: status='canary' requires valid canary command (got %s)",
				pyQuote(target), pyQuote(scannerName), pyQuote(cfg.Canary),
			)
		}
		return ""
	}
	if len(strings.TrimSpace(cfg.Reason)) < minExemptReasonLen {
		return fmt.Sprintf(
			"target %s scanner %s: status=%s requires non-empty reason (>=%d chars)",
			pyQuote(target), pyQuote(scannerName), pyQuote(cfg.Status), minExemptReasonLen,
		)
	}
	return ""
}

func gitConfigHasSparseCheckout(cfgPath string) bool {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}
	inCore := false
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		raw = strings.TrimRight(raw, "\r")
		stripped := strings.TrimSpace(raw)
		if stripped == "" || strings.HasPrefix(stripped, "#") || strings.HasPrefix(stripped, ";") {
			continue
		}
		if sec := gitSectionRE.FindStringSubmatch(raw); sec != nil {
			inCore = strings.EqualFold(strings.TrimSpace(sec[1]), "core")
			continue
		}
		if !inCore {
			continue
		}
		kv := gitKVRE.FindStringSubmatch(raw)
		if kv == nil || !strings.EqualFold(kv[1], "sparsecheckout") {
			continue
		}
		if !strings.Contains(raw, "=") {
			return true
		}
		val := ""
		if len(kv) > 2 {
			val = kv[2]
		}
		switch strings.ToLower(strings.Trim(strings.TrimSpace(val), `"`)) {
		case "true", "1", "yes", "on":
			return true
		}
	}
	return false
}

func isGitSparseCheckout(repoRoot string) bool {
	dotGit := filepath.Join(repoRoot, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil {
		return false
	}
	var gitDir, commonDir string
	if info.IsDir() {
		gitDir = dotGit
		commonDir = dotGit
	} else if info.Mode().IsRegular() {
		data, err := os.ReadFile(dotGit)
		if err != nil {
			return false
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) == 0 {
			return false
		}
		first := strings.TrimSpace(lines[0])
		if !strings.HasPrefix(first, "gitdir:") {
			return false
		}
		gitDir = first[len("gitdir:"):]
		gitDir = strings.TrimSpace(gitDir)
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(repoRoot, gitDir)
		}
		gitDir = resolveExisting(gitDir)
		commonDir = gitDir
		commonFile := filepath.Join(gitDir, "commondir")
		if isFile(commonFile) {
			relRaw, err := os.ReadFile(commonFile)
			if err == nil {
				rel := strings.TrimSpace(string(relRaw))
				if rel != "" {
					if filepath.IsAbs(rel) {
						commonDir = resolveExisting(rel)
					} else {
						commonDir = resolveExisting(filepath.Join(gitDir, rel))
					}
				}
			}
		}
	} else {
		return false
	}
	candidates := []string{
		filepath.Join(gitDir, "config.worktree"),
		filepath.Join(gitDir, "config"),
		filepath.Join(commonDir, "config"),
	}
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if isFile(candidate) && gitConfigHasSparseCheckout(candidate) {
			return true
		}
	}
	return false
}

func resolveExisting(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func validatePythonLedgerScanners(target string, scanners map[string]scannerEntry, repoRoot string, requirePyproject bool) []string {
	var errors []string
	for _, optionalTool := range []string{"bandit", "pyrefly"} {
		if _, ok := scanners[optionalTool]; !ok {
			errors = append(errors, fmt.Sprintf(
				"target %s (python): missing explicit scanner entry for %s",
				pyQuote(target), pyQuote(optionalTool),
			))
		}
	}
	if len(errors) > 0 {
		return errors
	}
	pyprojectPath := filepath.Join(repoRoot, target, "pyproject.toml")
	if !isFile(pyprojectPath) {
		if requirePyproject {
			return []string{fmt.Sprintf("target %s: missing pyproject.toml at %s", pyQuote(target), pyprojectPath)}
		}
		return nil
	}
	devDeps, err := readPythonDevDeps(pyprojectPath)
	if err != nil {
		return []string{fmt.Sprintf("target %s: %s", pyQuote(target), err.Error())}
	}
	for _, optionalTool := range []string{"bandit", "pyrefly"} {
		entry := scanners[optionalTool]
		_, declared := devDeps[optionalTool]
		expected := "not-applicable"
		if declared {
			expected = "canary"
		}
		if entry.Status != expected {
			verb := "does not declare"
			if declared {
				verb = "declares"
			}
			errors = append(errors, fmt.Sprintf(
				"target %s (python): pyproject.toml %s %s, so ledger status must be %s (got %s)",
				pyQuote(target), verb, pyQuote(optionalTool), pyQuote(expected), pyQuote(entry.Status),
			))
		}
	}
	return errors
}

func validateLedgerScanners(target, kind string, scanners map[string]scannerEntry, repoRoot string, requirePyproject bool) []string {
	if len(scanners) == 0 {
		return []string{fmt.Sprintf("target %s: missing or empty 'scanners' table", pyQuote(target))}
	}
	names := make([]string, 0, len(scanners))
	for name := range scanners {
		names = append(names, name)
	}
	sort.Strings(names)
	var errors []string
	for _, name := range names {
		if errText := validateSingleScannerEntry(target, name, scanners[name], true); errText != "" {
			errors = append(errors, errText)
		}
	}
	for _, required := range requiredCanaryScannersByKind[kind] {
		entry, ok := scanners[required]
		if !ok || entry.Status != "canary" {
			errors = append(errors, fmt.Sprintf(
				"target %s (%s): required scanner %s must have status = 'canary'",
				pyQuote(target), kind, pyQuote(required),
			))
		}
	}
	if kind == "python" {
		errors = append(errors, validatePythonLedgerScanners(target, scanners, repoRoot, requirePyproject)...)
	}
	return errors
}

func listRepoLintTargets(ctx context.Context, repoRoot string) ([]string, string) {
	runSh := filepath.Join(repoRoot, "scripts", "lint", "run.sh")
	if !isFile(runSh) {
		return nil, fmt.Sprintf("missing %s", runSh)
	}
	bashBin := look("bash")
	if bashBin == "" {
		return nil, "bash executable not found in PATH"
	}
	res := runCmd(ctx, 0, bashBin, []string{runSh, "--list"}, repoRoot, nil)
	if res.err != nil {
		return nil, fmt.Sprintf("run.sh --list failed: %s", res.err.Error())
	}
	if res.code != 0 {
		return nil, fmt.Sprintf("run.sh --list failed (exit %d): %s", res.code, snippet(res.stderr))
	}
	seen := map[string]struct{}{}
	var targets []string
	for _, line := range strings.Split(res.stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		targets = append(targets, line)
	}
	sort.Strings(targets)
	if targets == nil {
		targets = []string{}
	}
	return targets, ""
}

func loadLedgerTargets(ledgerPath string) (map[string]ledgerTarget, string) {
	if !isFile(ledgerPath) {
		return nil, fmt.Sprintf("missing ledger file %s", ledgerPath)
	}
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		return nil, fmt.Sprintf("invalid TOML in %s: %s", ledgerPath, err.Error())
	}
	var doc ledgerDoc
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Sprintf("invalid TOML in %s: %s", ledgerPath, err.Error())
	}
	if doc.Targets == nil {
		return nil, fmt.Sprintf("missing [targets] table in %s", ledgerPath)
	}
	return doc.Targets, ""
}

func tallyScannerStatuses(scanners map[string]scannerEntry) (int, int, int) {
	canary, exempt, na := 0, 0, 0
	for _, cfg := range scanners {
		switch cfg.Status {
		case "canary":
			canary++
		case "exempt":
			exempt++
		case "not-applicable":
			na++
		}
	}
	return canary, exempt, na
}

func validateLedgerEntries(targetsToCheck []string, listed map[string]struct{}, targets map[string]ledgerTarget, repoRoot string, sparseActive bool) ([]string, int, int, int) {
	var errors []string
	canaryCount, exemptCount, naCount := 0, 0, 0
	for _, target := range targetsToCheck {
		entry, ok := targets[target]
		if !ok {
			continue
		}
		if _, valid := validLedgerKinds[entry.Kind]; !valid {
			errors = append(errors, fmt.Sprintf(
				"target %s: invalid kind %s (expected one of %s)",
				pyQuote(target), pyQuote(entry.Kind), pyList(sortedLedgerKinds),
			))
			continue
		}
		_, inListed := listed[target]
		requirePyproject := inListed || !sparseActive
		errors = append(errors, validateLedgerScanners(target, entry.Kind, entry.Scanners, repoRoot, requirePyproject)...)
		cCount, eCount, nCount := tallyScannerStatuses(entry.Scanners)
		canaryCount += cCount
		exemptCount += eCount
		naCount += nCount
	}
	return errors, canaryCount, exemptCount, naCount
}

func verifyCanaryLedger(ctx context.Context, st streams, repoRoot, ledgerPath string, allowSparse bool) int {
	resolved := ledgerPath
	if resolved == "" {
		resolved = filepath.Join(repoRoot, "scripts", "lint", "canary_ledger.toml")
	}
	listedTargets, listErr := listRepoLintTargets(ctx, repoRoot)
	if listErr != "" || listedTargets == nil {
		st.error("ERROR: LEDGER CHECK FAILED: " + listErr)
		return exitFail
	}
	targetsTable, loadErr := loadLedgerTargets(resolved)
	if loadErr != "" || targetsTable == nil {
		st.error("ERROR: LEDGER CHECK FAILED: " + loadErr)
		return exitFail
	}
	isSparse := isGitSparseCheckout(repoRoot)
	sparseActive := allowSparse && isSparse
	listedSet := map[string]struct{}{}
	for _, target := range listedTargets {
		listedSet[target] = struct{}{}
	}
	ledgerSet := map[string]struct{}{}
	for target := range targetsTable {
		ledgerSet[target] = struct{}{}
	}
	var errors []string
	missing := sortedDifference(listedSet, ledgerSet)
	extra := sortedDifference(ledgerSet, listedSet)
	if len(missing) > 0 {
		errors = append(errors, "targets in run.sh --list missing from ledger: "+pyList(missing))
	}
	if len(extra) > 0 && !sparseActive {
		hint := ""
		if isSparse {
			hint = " (git core.sparseCheckout=true; run verify-ledger on a full checkout or pass --allow-sparse for checked-out subset validation)"
		}
		errors = append(errors, "stale targets in ledger absent from run.sh --list: "+pyList(extra)+hint)
	}
	targetsToCheck := listedTargets
	if sparseActive {
		targetsToCheck = sortedSet(ledgerSet)
	}
	entryErrors, canaryCount, exemptCount, naCount := validateLedgerEntries(targetsToCheck, listedSet, targetsTable, repoRoot, sparseActive)
	errors = append(errors, entryErrors...)
	if len(errors) > 0 {
		for _, errText := range errors {
			st.error("ERROR: LEDGER CHECK FAILED: " + errText)
		}
		return exitFail
	}
	sparseNote := ""
	if sparseActive {
		sparseNote = fmt.Sprintf("sparse materialized=%d, ", len(listedTargets))
	}
	st.info(fmt.Sprintf(
		"canary ledger verified: %d targets accounted for (%scanary=%d, exempt=%d, not-applicable=%d)",
		len(targetsToCheck), sparseNote, canaryCount, exemptCount, naCount,
	))
	return exitOK
}

func sortedDifference(left, right map[string]struct{}) []string {
	var out []string
	for item := range left {
		if _, ok := right[item]; !ok {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}
