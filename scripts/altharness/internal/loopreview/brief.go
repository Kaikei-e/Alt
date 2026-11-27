package loopreview

import (
	"bytes"
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
	"time"

	"alt/scripts/altharness/internal/corpus"
	"alt/scripts/altharness/internal/execx"
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/screener"
)

var corpusCaseDirRE = regexp.MustCompile(`^C-\d{2,}$`)

// BriefOptions configures `loop-review brief`.
type BriefOptions struct {
	RepoRoot  string
	BaseRev   string
	FocusFile string
}

// LoadFocusFile loads custom focus items from a JSON array or {"focus_items": [...]}.
func LoadFocusFile(path string) ([]FocusItem, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("focus file not found: %s", path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("focus file must not be a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("focus path is not a regular file: %s", path)
	}
	raw, err := readRegularFileSafe(path)
	if err != nil {
		return nil, fmt.Errorf("read focus file %s: %w", path, err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("focus file %s is empty", path)
	}
	var items []FocusItem
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, fmt.Errorf("parse focus file %s: %w", path, err)
		}
	} else {
		var wrapper struct {
			FocusItems []FocusItem `json:"focus_items"`
			FocusAreas []FocusItem `json:"focus_areas"`
		}
		if err := json.Unmarshal(trimmed, &wrapper); err != nil {
			return nil, fmt.Errorf("parse focus file %s: %w", path, err)
		}
		if len(wrapper.FocusItems) > 0 {
			items = wrapper.FocusItems
		} else {
			items = wrapper.FocusAreas
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("focus file %s contains 0 focus items", path)
	}
	seen := map[string]struct{}{}
	for i, item := range items {
		id := strings.TrimSpace(item.ID)
		title := strings.TrimSpace(item.Title)
		if isPlaceholder(id) || isPlaceholder(title) {
			return nil, fmt.Errorf("focus file %s item[%d] requires non-empty id and title", path, i)
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("focus file %s contains duplicate focus id %q", path, id)
		}
		seen[id] = struct{}{}
	}
	return items, nil
}

// LoadCorpusSummary reads id, title, category, and language from scripts/harness/corpus/C-*/meta.toml.
func LoadCorpusSummary(repoRoot string) ([]CorpusEntry, error) {
	corpusDir := filepath.Join(repoRoot, "scripts", "harness", "corpus")
	info, err := os.Lstat(corpusDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []CorpusEntry{}, nil
		}
		return nil, fmt.Errorf("stat corpus dir %s: %w", corpusDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("corpus directory must not be a symlink: %s", corpusDir)
	}
	if !info.IsDir() {
		return []CorpusEntry{}, nil
	}
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		return nil, fmt.Errorf("read corpus dir %s: %w", corpusDir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := make([]CorpusEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !corpusCaseDirRE.MatchString(name) {
			continue
		}
		caseDir := filepath.Join(corpusDir, name)
		dInfo, derr := os.Lstat(caseDir)
		if derr != nil || !dInfo.IsDir() || dInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		metaPath := filepath.Join(caseDir, "meta.toml")
		mInfo, merr := os.Lstat(metaPath)
		if merr != nil {
			continue
		}
		if mInfo.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("corpus meta.toml must not be a symlink: %s", metaPath)
		}
		if !mInfo.Mode().IsRegular() {
			continue
		}
		raw, rerr := readRegularFileSafe(metaPath)
		if rerr != nil {
			return nil, fmt.Errorf("read %s: %w", metaPath, rerr)
		}
		parsed, perr := corpus.ParseTOML(string(raw))
		if perr != nil {
			return nil, fmt.Errorf("parse %s: %w", metaPath, perr)
		}
		id, _ := parsed["id"].(string)
		if strings.TrimSpace(id) == "" {
			id = name
		}
		title, _ := parsed["title"].(string)
		category, _ := parsed["category"].(string)
		language, _ := parsed["language"].(string)
		out = append(out, CorpusEntry{
			ID:       strings.TrimSpace(id),
			Title:    strings.TrimSpace(title),
			Category: strings.TrimSpace(category),
			Language: strings.TrimSpace(language),
		})
	}
	return out, nil
}

// BuildBrief constructs the focused LOOP-04 adversarial review packet.
func BuildBrief(ctx context.Context, opts BriefOptions) (Brief, error) {
	if err := ctx.Err(); err != nil {
		return Brief{}, err
	}
	var focusAreas []FocusItem
	if strings.TrimSpace(opts.FocusFile) != "" {
		loaded, err := LoadFocusFile(opts.FocusFile)
		if err != nil {
			return Brief{}, err
		}
		focusAreas = loaded
	} else {
		focusAreas = CanonicalFocusAreas()
	}

	corpusEntries, err := LoadCorpusSummary(opts.RepoRoot)
	if err != nil {
		return Brief{}, err
	}

	brief := Brief{
		SchemaVersion: briefSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		RepoRoot:      opts.RepoRoot,
		BaseRev:       strings.TrimSpace(opts.BaseRev),
		Rules: []string{
			"Focused adversarial review only: inspect listed focus areas, CORPUS-01 precedents, and CHECK-01 candidates rather than dumping an undirected whole diff.",
			"Distinct model family required: reviewer model family MUST differ from author_model.",
			"auto_applied MUST remain false: never auto-apply fixes during /interrogate.",
			"Triage every finding into exactly one bucket: fix, investigate, note, or dismiss.",
			"Every 'fix' finding MUST include reproducible failing command/test proof (exit_code != 0).",
			"Every 'safe' focus verdict MUST include runnable blast_radius_proof (exit_code == 0, never prose-only).",
		},
		FocusAreas:         focusAreas,
		CorpusSummary:      corpusEntries,
		ScreenerCandidates: []screener.Candidate{},
	}

	if headSHA, ok := resolveGitCommitQuiet(ctx, opts.RepoRoot, "HEAD"); ok {
		brief.HeadCommit = headSHA
	}

	if brief.BaseRev != "" {
		if strings.HasPrefix(brief.BaseRev, "-") {
			return Brief{}, fmt.Errorf("invalid --base revision %q", brief.BaseRev)
		}
		resolvedBase, err := resolveGitCommit(ctx, opts.RepoRoot, brief.BaseRev)
		if err != nil {
			return Brief{}, err
		}
		brief.BaseCommit = resolvedBase

		cands, err := runScreenerInProcess(ctx, opts.RepoRoot, resolvedBase)
		if err != nil {
			return Brief{}, err
		}
		brief.ScreenerCandidates = cands

		diffSummary, err := collectDiffSummary(ctx, opts.RepoRoot, resolvedBase)
		if err != nil {
			return Brief{}, err
		}
		brief.Diff = diffSummary
	}

	return brief, nil
}

func runScreenerInProcess(ctx context.Context, repoRoot, baseRev string) ([]screener.Candidate, error) {
	var outBuf, errBuf bytes.Buffer
	rc := screener.Run(ctx, []string{"--base", baseRev, "--repo-root", repoRoot}, &outBuf, &errBuf)
	if rc != 0 {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = fmt.Sprintf("exit %d", rc)
		}
		return nil, fmt.Errorf("CHECK-01 screener failed: %s", msg)
	}
	var cands []screener.Candidate
	if err := json.Unmarshal(outBuf.Bytes(), &cands); err != nil {
		return nil, fmt.Errorf("parse CHECK-01 screener output: %w", err)
	}
	if cands == nil {
		cands = []screener.Candidate{}
	}
	return cands, nil
}

func readRegularFileSafe(path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.Base(abs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func collectDiffSummary(ctx context.Context, repoRoot, resolvedBase string) (*DiffSummary, error) {
	statOut, err := runGitOutput(ctx, repoRoot, "diff", "--stat", "--end-of-options", resolvedBase, "--")
	if err != nil {
		return nil, fmt.Errorf("git diff --stat: %w", err)
	}
	nameStatusOut, err := runGitOutput(ctx, repoRoot, "diff", "--name-status", "--end-of-options", resolvedBase, "--")
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status: %w", err)
	}
	files := []DiffFile{}
	for _, line := range strings.Split(strings.TrimSpace(nameStatusOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[len(parts)-1])
		files = append(files, DiffFile{Status: status, Path: path})
	}
	return &DiffSummary{
		Base:  resolvedBase,
		Files: files,
		Stat:  strings.TrimRight(statOut, "\r\n"),
	}, nil
}

func resolveGitCommit(ctx context.Context, repoRoot, rev string) (string, error) {
	out, err := runGitOutput(ctx, repoRoot, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("invalid --base revision %q: %w", rev, err)
	}
	return strings.TrimSpace(out), nil
}

func resolveGitCommitQuiet(ctx context.Context, repoRoot, rev string) (string, bool) {
	out, err := runGitOutput(ctx, repoRoot, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", false
	}
	sha := strings.TrimSpace(out)
	return sha, sha != ""
}

func runGitOutput(ctx context.Context, repoRoot string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	fullArgs := append([]string{"git"}, args...)
	stdout, stderr, code, err := execx.Combined(cctx, execx.Cmd{
		Args: fullArgs,
		Dir:  repoRoot,
	})
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = fmt.Sprintf("git exited %d", code)
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout, nil
}

// RenderBriefMarkdown formats a focused adversarial review packet in Markdown.
func RenderBriefMarkdown(b Brief) string {
	var sb strings.Builder
	sb.WriteString("# LOOP-04 Adversarial Review Brief\n\n")
	fmt.Fprintf(&sb, "- **Schema**: `%s`\n", b.SchemaVersion)
	fmt.Fprintf(&sb, "- **Generated At**: `%s`\n", b.GeneratedAt)
	if b.BaseCommit != "" {
		fmt.Fprintf(&sb, "- **Base Commit**: `%s` (`%s`)\n", b.BaseCommit, b.BaseRev)
	}
	if b.HeadCommit != "" {
		fmt.Fprintf(&sb, "- **Head Commit**: `%s`\n", b.HeadCommit)
	}
	sb.WriteString("\n## Review Guardrails\n\n")
	for _, rule := range b.Rules {
		fmt.Fprintf(&sb, "- %s\n", rule)
	}

	sb.WriteString("\n## Focus Areas\n\n")
	sb.WriteString("| ID | Focus Area | Targets | CORPUS |\n")
	sb.WriteString("|---|---|---|---|\n")
	for _, item := range b.FocusAreas {
		targets := "-"
		if len(item.TargetPaths) > 0 {
			targets = "`" + strings.Join(item.TargetPaths, "`, `") + "`"
		}
		cids := "-"
		if len(item.CorpusIDs) > 0 {
			cids = strings.Join(item.CorpusIDs, ", ")
		}
		fmt.Fprintf(&sb, "| `%s` | %s | %s | %s |\n", item.ID, item.Title, targets, cids)
	}

	sb.WriteString("\n## CORPUS-01 Summary\n\n")
	if len(b.CorpusSummary) == 0 {
		sb.WriteString("_No CORPUS-01 cases loaded._\n")
	} else {
		sb.WriteString("| ID | Language | Category | Title |\n")
		sb.WriteString("|---|---|---|---|\n")
		for _, c := range b.CorpusSummary {
			fmt.Fprintf(&sb, "| `%s` | `%s` | `%s` | %s |\n", c.ID, c.Language, c.Category, c.Title)
		}
	}

	sb.WriteString("\n## CHECK-01 Screener Candidates\n\n")
	if len(b.ScreenerCandidates) == 0 {
		sb.WriteString("_No CHECK-01 behavior-change candidates detected (or `--base` not provided)._\n")
	} else {
		sb.WriteString("| ID | Kind | File | Function | Before | After |\n")
		sb.WriteString("|---|---|---|---|---|---|\n")
		for _, c := range b.ScreenerCandidates {
			fmt.Fprintf(&sb, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
				c.ID, c.Kind, c.File, c.Function, sanitizeTSVCell(c.Before), sanitizeTSVCell(c.After))
		}
	}

	if b.Diff != nil {
		sb.WriteString("\n## Diff File Stats (No Raw Diff Dump)\n\n")
		if len(b.Diff.Files) == 0 {
			sb.WriteString("_No changed files relative to base._\n")
		} else {
			for _, f := range b.Diff.Files {
				fmt.Fprintf(&sb, "- `%s` `%s`\n", f.Status, f.Path)
			}
		}
		if b.Diff.Stat != "" {
			sb.WriteString("\n```text\n")
			sb.WriteString(b.Diff.Stat)
			sb.WriteString("\n```\n")
		}
	}

	return sb.String()
}

func isSymlink(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func ensureNoSymlinkPath(target string) error {
	if strings.TrimSpace(target) == "" || strings.ContainsRune(target, 0) {
		return errors.New("output path must be non-empty")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	cur := filepath.Clean(abs)
	var stack []string
	for {
		stack = append(stack, cur)
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	tmpRoot := filepath.Clean(os.TempDir())
	for i := len(stack) - 1; i >= 0; i-- {
		p := stack[i]
		st, lerr := os.Lstat(p)
		if lerr != nil {
			if errors.Is(lerr, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat %q: %w", p, lerr)
		}
		if st.Mode()&os.ModeSymlink != 0 {
			if p == tmpRoot && i > 0 {
				continue
			}
			return fmt.Errorf("refusing to traverse or overwrite symlink: %s", p)
		}
	}
	return nil
}

func writeFileInDirSafe(dir, name string, data []byte) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open root %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()
	if st, lerr := root.Lstat(name); lerr == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to overwrite symlink: %s", filepath.Join(dir, name))
		}
		if st.IsDir() {
			return fmt.Errorf("output path is a directory: %s", filepath.Join(dir, name))
		}
	} else if !errors.Is(lerr, os.ErrNotExist) {
		return fmt.Errorf("lstat %s: %w", filepath.Join(dir, name), lerr)
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fsperm.File)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Join(dir, name), err)
	}
	_, wErr := f.Write(data)
	cErr := f.Close()
	if wErr != nil {
		return fmt.Errorf("write %s: %w", filepath.Join(dir, name), wErr)
	}
	if cErr != nil {
		return fmt.Errorf("close %s: %w", filepath.Join(dir, name), cErr)
	}
	return nil
}

func writeSafeFile(outPath string, data []byte) error {
	if err := ensureNoSymlinkPath(outPath); err != nil {
		return err
	}
	abs, err := filepath.Abs(outPath)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	if st, err := os.Lstat(abs); err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to overwrite symlink: %s", abs)
		}
		if st.IsDir() {
			return fmt.Errorf("output path is a directory: %s", abs)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat output path: %w", err)
	}
	parentDir := filepath.Dir(abs)
	if err := os.MkdirAll(parentDir, fsperm.Dir); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := ensureNoSymlinkPath(abs); err != nil {
		return err
	}
	return writeFileInDirSafe(parentDir, filepath.Base(abs), data)
}
