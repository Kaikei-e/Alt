package packet

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func safeEvidencePath(runsDir, branchDir, packetID string) (string, bool) {
	if !packetIDRe.MatchString(packetID) {
		return "", false
	}
	raw := filepath.Join(runsDir, branchDir, "verify", packetID+".json")
	cur := runsDir
	for _, part := range []string{branchDir, "verify", packetID + ".json"} {
		cur = filepath.Join(cur, part)
		if isSymlink(cur) {
			return "", false
		}
	}
	if !withinDir(raw, runsDir) {
		return "", false
	}
	return raw, true
}

func preValidateVerifyHandoff(handoffDoc, packetDoc *Document, schema map[string]any, repo, runsDir string, checkEvidencePath bool) ([]Issue, error) {
	var issues []Issue
	hoID, hoOK := asString(handoffDoc.Data["packet_id"])
	pktID, pktOK := asString(packetDoc.Data["packet_id"])
	if !hoOK || !packetIDRe.MatchString(hoID) || !pktOK || !packetIDRe.MatchString(pktID) {
		if err := validateSchemaNode(handoffDoc.Data, refSchema("TaskHandoff"), schema, "$", &issues); err != nil {
			return issues, err
		}
		var pktIssues []Issue
		if err := validateSchemaNode(packetDoc.Data, refSchema("TaskPacket"), schema, "$", &pktIssues); err != nil {
			return issues, err
		}
		for _, item := range pktIssues {
			issues = append(issues, Issue{
				Code:    "PACKET_" + item.Code,
				Path:    "packet:" + item.Path,
				Message: item.Message,
			})
		}
		if !hasCode(issues, "SCHEMA_PATTERN") && !hasCode(issues, "PACKET_SCHEMA_PATTERN") {
			issues = append(issues, Issue{
				Code:    "INVALID_PACKET_ID",
				Path:    "$.packet_id",
				Message: fmt.Sprintf("refusing verify-handoff for invalid packet_id (handoff=%v, packet=%v)", handoffDoc.Data["packet_id"], packetDoc.Data["packet_id"]),
			})
		}
		return issues, nil
	}
	if checkEvidencePath {
		branch := currentBranchSanitized(repo)
		if _, ok := safeEvidencePath(runsDir, branch, pktID); !ok {
			issues = append(issues, Issue{
				Code:    "UNSAFE_EVIDENCE_PATH",
				Path:    "$.packet_id",
				Message: fmt.Sprintf("evidence path for packet_id=%q escapes runs_dir or hits symlink", pktID),
			})
			return issues, nil
		}
	}
	return nil, nil
}

func verifyHandoffActualFiles(ctx context.Context, handoff, packet map[string]any, repo, resolvedBase string, issues *[]Issue) (map[string]struct{}, error) {
	actual, err := gitActualModifiedFiles(ctx, repo, resolvedBase)
	if err != nil {
		return nil, err
	}
	declared := map[string]struct{}{}
	if raw, ok := asSlice(handoff["modified_files"]); ok {
		for _, entry := range raw {
			item, ok := asMap(entry)
			if !ok {
				continue
			}
			if rel, ok := asString(item["path"]); ok {
				declared[normalizeRepoRel(rel)] = struct{}{}
			}
		}
	}
	var undeclared, phantom []string
	for rel := range actual {
		if _, ok := declared[rel]; !ok {
			undeclared = append(undeclared, rel)
		}
	}
	for rel := range declared {
		if _, ok := actual[rel]; !ok {
			phantom = append(phantom, rel)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(phantom)
	for _, rel := range undeclared {
		*issues = append(*issues, Issue{
			Code:    "UNDECLARED_MODIFIED_FILE",
			Path:    "$.modified_files",
			Message: fmt.Sprintf("git worktree modified/added '%s' since %s but it is missing from handoff.modified_files", rel, shortSHA(resolvedBase)),
		})
	}
	for _, rel := range phantom {
		*issues = append(*issues, Issue{
			Code:    "PHANTOM_MODIFIED_FILE",
			Path:    "$.modified_files",
			Message: fmt.Sprintf("handoff declares modified file '%s' with no diff against %s", rel, shortSHA(resolvedBase)),
		})
	}
	scope, ok := asMap(packet["scope"])
	if !ok {
		return actual, nil
	}
	if maxFiles, ok := asInt(scope["max_files"]); ok && int64(len(actual)) > maxFiles && !hasCode(*issues, "MAX_FILES_EXCEEDED") {
		*issues = append(*issues, Issue{
			Code:    "MAX_FILES_EXCEEDED",
			Path:    "$.modified_files",
			Message: fmt.Sprintf("actual git modified files count %d exceeds packet max_files=%d", len(actual), maxFiles),
		})
	}
	allowed := stringList(scope["allowed_paths"])
	forbidden := stringList(scope["forbidden_paths"])
	allowGenerated := scope["allow_generated_code_edits"] == true
	for _, rel := range undeclared {
		if len(allowed) > 0 && !matchesAnyGlob(rel, allowed) {
			*issues = append(*issues, Issue{
				Code:    "PATH_OUTSIDE_ALLOWED_PATHS",
				Path:    "git:" + rel,
				Message: fmt.Sprintf("actual modified file '%s' is outside packet scope.allowed_paths %v", rel, allowed),
			})
		}
		if len(forbidden) > 0 && matchesAnyGlob(rel, forbidden) {
			*issues = append(*issues, Issue{
				Code:    "PATH_IN_FORBIDDEN_PATHS",
				Path:    "git:" + rel,
				Message: fmt.Sprintf("actual modified file '%s' matches packet scope.forbidden_paths %v", rel, forbidden),
			})
		}
		if !allowGenerated && repoRelIsGenerated(ctx, repo, rel, resolvedBase) {
			*issues = append(*issues, Issue{
				Code:    "GENERATED_CODE_MODIFIED",
				Path:    "git:" + rel,
				Message: fmt.Sprintf("actual modified file '%s' carries a generated-code DO NOT EDIT header", rel),
			})
		}
	}
	return actual, nil
}

func hasCode(issues []Issue, code string) bool {
	for _, item := range issues {
		if item.Code == code {
			return true
		}
	}
	return false
}

func hasPrefixCode(issues []Issue, prefix string) bool {
	for _, item := range issues {
		if strings.HasPrefix(item.Code, prefix) {
			return true
		}
	}
	return false
}

func verifyHandoffAST(ctx context.Context, handoff, packet map[string]any, repo, resolvedBase string, actual map[string]struct{}, issues *[]Issue) error {
	var files []string
	for rel := range actual {
		files = append(files, rel)
	}
	sort.Strings(files)
	count, err := countActualModifiedFunctions(func(rel string) (string, string, error) {
		before, err := gitFileTextAtRev(ctx, repo, resolvedBase, rel)
		if err != nil {
			return "", "", err
		}
		return before, readWorktreeText(repo, rel), nil
	}, files)
	if err != nil {
		return err
	}
	if declared, ok := asInt(handoff["modified_functions_count"]); ok && declared != int64(count) {
		*issues = append(*issues, Issue{
			Code:    "MODIFIED_FUNCTIONS_MISMATCH",
			Path:    "$.modified_functions_count",
			Message: fmt.Sprintf("handoff declares modified_functions_count=%d, but AST diff found %d modified function(s)", declared, count),
		})
	}
	scope, _ := asMap(packet["scope"])
	if maxFn, ok := asInt(scope["max_functions"]); ok && int64(count) > maxFn && !hasCode(*issues, "MAX_FUNCTIONS_EXCEEDED") {
		*issues = append(*issues, Issue{
			Code:    "MAX_FUNCTIONS_EXCEEDED",
			Path:    "$.modified_functions_count",
			Message: fmt.Sprintf("actual AST modified functions count %d exceeds packet max_functions=%d", count, maxFn),
		})
	}
	return nil
}

func verifyHandoffHostRerun(ctx context.Context, handoff, packet map[string]any, repo, runsDir, resolvedBase string, issues *[]Issue) error {
	if hasPrefixCode(*issues, "PACKET_") {
		return nil
	}
	policy, ok := asMap(packet["execution_policy"])
	if !ok {
		return nil
	}
	verifyCmd, ok := asString(policy["verification_command"])
	if !ok || strings.TrimSpace(verifyCmd) == "" {
		return nil
	}
	if matchForbiddenAutofix(verifyCmd) != "" || hasBackgroundAmpersand(verifyCmd) {
		return nil
	}
	rawID := packet["packet_id"]
	if rawID == nil {
		rawID = handoff["packet_id"]
	}
	packetID, ok := rawID.(string)
	if !ok || !packetIDRe.MatchString(packetID) {
		*issues = append(*issues, Issue{
			Code:    "INVALID_PACKET_ID",
			Path:    "$.packet_id",
			Message: fmt.Sprintf("refusing to write rerun evidence for invalid packet_id %v", rawID),
		})
		return nil
	}
	branch := currentBranchSanitized(repo)
	evidence, ok := safeEvidencePath(runsDir, branch, packetID)
	if !ok {
		*issues = append(*issues, Issue{
			Code:    "UNSAFE_EVIDENCE_PATH",
			Path:    "$.packet_id",
			Message: fmt.Sprintf("evidence path for packet_id=%q escapes runs_dir or hits symlink", packetID),
		})
		return nil
	}
	budget := resolveCommandBudget(policy)
	hostRC, duration, outB, errB, err := runBash(ctx, verifyCmd, repo, timeDur(budget+measureTimeoutSlackS))
	if err != nil {
		return err
	}
	outputSHA := sha256Hex(append(append([]byte{}, outB...), append([]byte("\n"), errB...)...))
	if err := os.MkdirAll(filepath.Dir(evidence), fsperm.Dir); err != nil {
		return err
	}
	if isSymlink(evidence) || !withinDir(evidence, runsDir) {
		return pErr("evidence path escapes runs_dir: %s", evidence)
	}
	head, err := currentHeadSHA(ctx, repo)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"packet_id":     packetID,
		"command":       verifyCmd,
		"exit_code":     hostRC,
		"duration_s":    duration,
		"output_sha256": outputSHA,
		"base_commit":   resolvedBase,
		"head_commit":   head,
	}
	encoded, err := writeJSON(payload)
	if err != nil {
		return err
	}
	if err := os.WriteFile(evidence, encoded, fsperm.File); err != nil {
		return err
	}
	claimed := any(nil)
	if verification, ok := asMap(handoff["verification"]); ok {
		claimed = verification["exit_code"]
	}
	if code, ok := asInt(claimed); ok && int64(hostRC) != code {
		*issues = append(*issues, Issue{
			Code:    "VERIFICATION_EXIT_CODE_MISMATCH",
			Path:    "$.verification.exit_code",
			Message: fmt.Sprintf("host re-execution exited with %d, but handoff claimed exit_code=%d", hostRC, code),
		})
	}
	if handoff["status"] == "completed" && hostRC != 0 {
		*issues = append(*issues, Issue{
			Code:    "VERIFICATION_RERUN_FAILED",
			Path:    "$.verification.exit_code",
			Message: fmt.Sprintf("host re-execution of verification_command failed with exit_code=%d while handoff status='completed'", hostRC),
		})
	}
	return nil
}

func verifyHandoffLintFix(ctx context.Context, packet map[string]any, repo, resolvedBase string, actual map[string]struct{}, screener []map[string]any, issues *[]Issue) error {
	taskKind := "lint-fix"
	if k, ok := asString(packet["task_kind"]); ok {
		taskKind = k
	}
	if taskKind != "lint-fix" {
		return nil
	}
	var files []string
	for rel := range actual {
		files = append(files, rel)
	}
	sort.Strings(files)
	for _, rel := range files {
		if sequenceTestFileRe.MatchString(rel) {
			continue
		}
		before, err := gitFileTextAtRev(ctx, repo, resolvedBase, rel)
		if err != nil {
			return err
		}
		after := readWorktreeText(repo, rel)
		added := subtractCounts(extractCodeComments(rel, after), extractCodeComments(rel, before))
		sort.Strings(added)
		for _, comment := range added {
			if allowedLintCommentRe.MatchString(comment) {
				continue
			}
			shown := comment
			if len([]rune(shown)) > 80 {
				shown = string([]rune(shown)[:80])
			}
			*issues = append(*issues, Issue{
				Code:    "UNREQUESTED_COMMENT_ADDED",
				Path:    "diff:" + rel,
				Message: fmt.Sprintf("lint-fix task added unrequested comment: %q", shown),
			})
			break
		}
	}
	scope, _ := asMap(packet["scope"])
	allowTests := scope["allow_test_expectation_changes"] == true
	if allowTests {
		return nil
	}
	for _, cand := range screener {
		kind, _ := asString(cand["kind"])
		if kind == "test.expectation_change" || kind == "test.file_deleted" {
			*issues = append(*issues, Issue{
				Code:    "FORBIDDEN_TEST_EXPECTATION_CHANGE",
				Path:    fmt.Sprintf("diff:%v", cand["file"]),
				Message: fmt.Sprintf("lint-fix task forbids %s in '%v' unless scope.allow_test_expectation_changes=true", kind, cand["file"]),
			})
		}
	}
	return nil
}

func appendGenerated(ctx context.Context, handoff map[string]any, repo string, allow bool, issues *[]Issue, base string) error {
	var extra []Issue
	if err := validateGeneratedCodeEdits(ctx, handoff, repo, allow, &extra, base); err != nil {
		return err
	}
	existing := map[string]struct{}{}
	for _, item := range *issues {
		if item.Code == "GENERATED_CODE_MODIFIED" {
			existing[item.Path] = struct{}{}
		}
	}
	for _, item := range extra {
		if _, ok := existing[item.Path]; ok {
			continue
		}
		*issues = append(*issues, item)
	}
	return nil
}

func checkC1Integrity(ctx context.Context, repo, baseSHA, c1 string, issues *[]Issue) ([]string, error) {
	entries, err := gitCommitFilesWithStatus(ctx, repo, c1)
	if err != nil {
		return nil, err
	}
	var c1Files []string
	for _, e := range entries {
		c1Files = append(c1Files, e[1])
	}
	var nonTest []string
	for _, rel := range c1Files {
		if !sequenceTestFileRe.MatchString(rel) {
			nonTest = append(nonTest, rel)
		}
	}
	if len(c1Files) == 0 || len(nonTest) > 0 {
		*issues = append(*issues, Issue{
			Code:    "SEQUENCE_FIRST_COMMIT_NOT_TEST_ONLY",
			Path:    "commit:" + shortSHA(c1),
			Message: fmt.Sprintf("first commit %s must modify only test files (files=%s, non_test=%s)", shortSHA(c1), pyQuoteList(c1Files), pyQuoteList(nonTest)),
		})
		return c1Files, nil
	}
	var deleted []string
	for _, e := range entries {
		if e[0] == "D" {
			deleted = append(deleted, e[1])
		}
	}
	if len(deleted) > 0 {
		*issues = append(*issues, Issue{
			Code:    "SEQUENCE_FIRST_COMMIT_DELETED_TEST",
			Path:    "commit:" + shortSHA(c1),
			Message: fmt.Sprintf("first commit %s deleted test file(s): %s", shortSHA(c1), pyQuoteList(deleted)),
		})
	}
	var links []string
	for _, rel := range c1Files {
		mode, err := gitBlobMode(ctx, repo, c1, rel)
		if err != nil {
			return nil, err
		}
		if mode == gitSymlinkMode {
			links = append(links, rel)
		}
	}
	if len(links) > 0 {
		*issues = append(*issues, Issue{
			Code:    "SEQUENCE_FIRST_COMMIT_SYMLINK",
			Path:    "commit:" + shortSHA(c1),
			Message: fmt.Sprintf("first commit %s introduced symlink(s): %s", shortSHA(c1), pyQuoteList(links)),
		})
	}
	for _, e := range entries {
		status, rel := e[0], e[1]
		if status != "M" {
			continue
		}
		baseMode, err := gitBlobMode(ctx, repo, baseSHA, rel)
		if err != nil {
			return nil, err
		}
		if baseMode == gitSymlinkMode {
			continue
		}
		lang := inferLanguage(rel)
		if lang == "" {
			continue
		}
		before, err := gitFileTextAtRev(ctx, repo, baseSHA, rel)
		if err != nil {
			return nil, err
		}
		after, err := gitFileTextAtRev(ctx, repo, c1, rel)
		if err != nil {
			return nil, err
		}
		for _, cand := range screenTestExpectations(before, after, rel, lang) {
			*issues = append(*issues, Issue{
				Code:    "SEQUENCE_FIRST_COMMIT_WEAKENED_TEST",
				Path:    "commit:" + shortSHA(c1),
				Message: fmt.Sprintf("first commit %s weakened existing test (%s) in '%s'", shortSHA(c1), cand.kind, cand.file),
			})
		}
	}
	return c1Files, nil
}

func checkSequenceCommits(ctx context.Context, repo, baseSHA string) ([]string, []string, []Issue, error) {
	var issues []Issue
	dirty, err := gitDirtyFiles(ctx, repo)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(dirty) > 0 {
		var names []string
		for rel := range dirty {
			names = append(names, rel)
		}
		sort.Strings(names)
		issues = append(issues, Issue{
			Code:    "SEQUENCE_DIRTY_WORKTREE",
			Path:    "worktree",
			Message: fmt.Sprintf("repo_root has uncommitted or untracked changes: %s", pyQuoteList(names)),
		})
		return nil, nil, issues, nil
	}
	stdout, stderr, code, err := runGit(ctx, repo, "rev-list", "--reverse", "--no-merges", baseSHA+"..HEAD")
	if err != nil {
		return nil, nil, nil, err
	}
	if code != 0 {
		return nil, nil, nil, pErr("git rev-list failed: %s", strings.TrimSpace(string(stderr)))
	}
	var commits []string
	for _, line := range splitLines(string(stdout)) {
		line = strings.TrimSpace(line)
		if line != "" {
			commits = append(commits, line)
		}
	}
	if len(commits) < sequenceMinCommits {
		issues = append(issues, Issue{
			Code:    "SEQUENCE_TOO_FEW_COMMITS",
			Path:    "$.commits",
			Message: fmt.Sprintf("verify-sequence requires at least %d non-merge commits in %s..HEAD (found %d)", sequenceMinCommits, shortSHA(baseSHA), len(commits)),
		})
		return commits, nil, issues, nil
	}
	c1Files, err := checkC1Integrity(ctx, repo, baseSHA, commits[0], &issues)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(issues) > 0 {
		return commits, c1Files, issues, nil
	}
	c1Set := map[string]struct{}{}
	for _, rel := range c1Files {
		c1Set[rel] = struct{}{}
	}
	for _, commit := range commits[1:] {
		files, err := gitCommitFiles(ctx, repo, commit)
		if err != nil {
			return nil, nil, nil, err
		}
		var mutated []string
		for _, rel := range files {
			if _, ok := c1Set[rel]; ok {
				mutated = append(mutated, rel)
			}
		}
		sort.Strings(mutated)
		if len(mutated) > 0 {
			issues = append(issues, Issue{
				Code:    "SEQUENCE_TEST_MUTATED_IN_IMPL",
				Path:    "commit:" + shortSHA(commit),
				Message: fmt.Sprintf("implementation commit %s mutated commit 1 test file(s): %s", shortSHA(commit), pyQuoteList(mutated)),
			})
		}
	}
	return commits, c1Files, issues, nil
}

func overlayCommitFiles(ctx context.Context, repo, rev string, files []string, wt string) error {
	wtResolved, err := resolvePath(wt)
	if err != nil {
		return err
	}
	for _, rel := range files {
		if unsafeRel(rel) {
			return pErr("refusing to overlay unsafe path: %q", rel)
		}
		cur := wt
		parts := splitRelParts(rel)
		for _, part := range parts[:len(parts)-1] {
			cur = filepath.Join(cur, part)
			if isSymlink(cur) {
				return pErr("refusing to overlay through symlink parent directory: %q", rel)
			}
		}
		rawDest := filepath.Join(wt, filepath.FromSlash(rel))
		if isSymlink(rawDest) {
			if err := os.Remove(rawDest); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(rawDest), fsperm.Dir); err != nil {
			return err
		}
		parentResolved, err := resolvePath(filepath.Dir(rawDest))
		if err != nil {
			return err
		}
		if !pathWithin(parentResolved, wtResolved) {
			return pErr("refusing to overlay path outside worktree: %s", rel)
		}
		mode, err := gitBlobMode(ctx, repo, rev, rel)
		if err != nil {
			return err
		}
		if mode == gitSymlinkMode {
			return pErr("refusing to overlay symlink from commit %s: %s", shortSHA(rev), rel)
		}
		stdout, _, code, err := runGit(ctx, repo, "show", "--end-of-options", rev+":"+rel)
		if err != nil {
			return err
		}
		if code != 0 {
			if _, statErr := os.Lstat(rawDest); statErr == nil {
				if err := os.Remove(rawDest); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			continue
		}
		if err := os.WriteFile(rawDest, stdout, fsperm.File); err != nil {
			return err
		}
		if mode == gitExecMode {
			if err := os.Chmod(rawDest, fsperm.Dir); err != nil {
				return err
			}
		}
	}
	return nil
}

func unsafeRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, `\`) {
		return true
	}
	for _, part := range splitRelParts(rel) {
		if part == ".." {
			return true
		}
	}
	return false
}

func splitRelParts(rel string) []string {
	var parts []string
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}

func pathWithin(path, dir string) bool {
	path = filepath.Clean(path)
	dir = filepath.Clean(dir)
	if path == dir {
		return true
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func runTestInDetachedWorktree(ctx context.Context, repo, checkout, testCommand string, overlayRev string, overlayFiles []string) (int, string, error) {
	parent, err := os.MkdirTemp("", "alt-verify-seq-")
	if err != nil {
		return 0, "", err
	}
	wt := filepath.Join(parent, "wt")
	added := false
	defer func() {
		if added {
			_, _, _, _ = runGit(context.Background(), repo, "worktree", "remove", "--force", wt)
			_, _, _, _ = runGit(context.Background(), repo, "worktree", "prune")
		}
		_ = os.RemoveAll(parent)
	}()
	_, stderr, code, err := runGit(ctx, repo, "worktree", "add", "--detach", wt, checkout)
	if err != nil {
		return 0, "", err
	}
	if code != 0 {
		return 0, "", pErr("git worktree add failed for %s: %s", checkout, strings.TrimSpace(string(stderr)))
	}
	added = true
	if overlayRev != "" && len(overlayFiles) > 0 {
		if err := overlayCommitFiles(ctx, repo, overlayRev, overlayFiles, wt); err != nil {
			return 0, "", err
		}
	}
	rc, _, outB, errB, err := runBash(ctx, testCommand, wt, timeDur(defaultCommandBudgetS+measureTimeoutSlackS))
	if err != nil {
		return 0, "", err
	}
	snippetSrc := errB
	if len(snippetSrc) == 0 {
		snippetSrc = outB
	}
	snippet := runeTail(strings.TrimSpace(validUTF8(snippetSrc)), 240)
	return rc, snippet, nil
}

type sequenceConfig struct {
	preserving bool
	packetID   any
	baseCommit string
}

// resolveSequenceConfig loads packet_id, base_commit, and behavior_preserving
// from packetPath when provided, while letting explicit CLI flags override
// base_commit or mode without discarding packet metadata.
func resolveSequenceConfig(flagBase string, hasFlagBase, bugfix, preserving bool, packetPath string) (sequenceConfig, error) {
	cfg := sequenceConfig{
		preserving: true,
		baseCommit: flagBase,
	}
	if packetPath != "" {
		doc, err := loadDocument(packetPath)
		if err != nil {
			return sequenceConfig{}, err
		}
		if v, ok := doc.Data["packet_id"]; ok {
			cfg.packetID = v
		}
		if !hasFlagBase {
			if bc, ok := asString(doc.Data["base_commit"]); ok {
				cfg.baseCommit = bc
			}
		}
		if bp, ok := doc.Data["behavior_preserving"].(bool); ok {
			cfg.preserving = bp
		} else if doc.Data["task_kind"] == "bugfix" {
			cfg.preserving = false
		}
	}
	if bugfix {
		cfg.preserving = false
	} else if preserving {
		cfg.preserving = true
	}
	return cfg, nil
}
