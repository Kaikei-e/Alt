package loopcorrect

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"alt/scripts/altharness/internal/corpus"
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/guards"
)

func cmdScaffoldCorpus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_ = ctx
	p, err := parseFlags(args,
		map[string]bool{
			"--class-id":  true,
			"--title":     true,
			"--language":  true,
			"--category":  true,
			"--repo-root": true,
			"--format":    true,
		},
		map[string]bool{
			"--write": true,
		},
	)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}
	if p.help {
		writeUsage(stdout)
		return exitOK
	}
	if len(p.positionals) > 0 {
		writef(stderr, "error: unrecognized arguments: %s\n", strings.Join(p.positionals, " "))
		return exitUsage
	}
	format, err := parseFormatFlag(p)
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitUsage
	}

	classID := strings.TrimSpace(p.values["--class-id"])
	title := strings.TrimSpace(p.values["--title"])
	language := strings.TrimSpace(p.values["--language"])
	category := strings.TrimSpace(p.values["--category"])

	if classID == "" || title == "" || language == "" || category == "" {
		writeln(stderr, "error: scaffold-corpus requires --class-id, --title, --language, and --category")
		return exitUsage
	}
	if strings.ContainsAny(classID, "\t\r\n") || strings.ContainsAny(title, "\t\r\n") {
		writeln(stderr, "error: --class-id and --title must be single-line strings")
		return exitUsage
	}
	if _, ok := corpus.ValidLanguages[language]; !ok {
		writef(stderr, "error: invalid --language %q (valid: %s)\n", language, strings.Join(sortedKeys(corpus.ValidLanguages), ", "))
		return exitUsage
	}
	if _, ok := corpus.ValidCategories[category]; !ok {
		writef(stderr, "error: invalid --category %q (valid: %s)\n", category, strings.Join(sortedKeys(corpus.ValidCategories), ", "))
		return exitUsage
	}

	repoRoot := guards.ResolveRepoRoot(p.values["--repo-root"], "")
	report, err := ScaffoldCorpusCase(repoRoot, classID, title, language, category, p.bools["--write"])
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return exitFail
	}

	if format == "json" {
		raw, err := marshalPrettyJSON(report)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return exitFail
		}
		_, _ = stdout.Write(raw)
		return exitOK
	}

	mode := "dry-run (pass --write to create files)"
	if report.Written {
		mode = "written"
	}
	writef(stdout, "Scaffold CORPUS-01 case %s [%s]\n", report.CorpusID, mode)
	writef(stdout, "  meta   : %s\n", report.MetaPath)
	writef(stdout, "  before : %s\n", report.BeforePath)
	writef(stdout, "  after  : %s\n", report.AfterPath)
	writef(stdout, "\n%s", report.MetaTOML)
	return exitOK
}

// ScaffoldCorpusCase computes the next unused C-NN corpus ID and generates
// meta.toml plus before/after file paths. Files are created on disk only when
// writeFiles is true, and existing C-NN directories are never overwritten.
func ScaffoldCorpusCase(repoRoot, classID, title, language, category string, writeFiles bool) (ScaffoldReport, error) {
	corpusRoot := filepath.Join(repoRoot, corpusDirRel)
	if isSymlink(corpusRoot) {
		return ScaffoldReport{}, fmt.Errorf("corpus directory must not be a symlink: %s", corpusRoot)
	}
	nextID, err := nextCorpusID(corpusRoot)
	if err != nil {
		return ScaffoldReport{}, err
	}
	beforeName, afterName := snippetFilenames(language)
	caseDir := filepath.Join(corpusRoot, nextID)
	metaPath := filepath.Join(caseDir, "meta.toml")
	beforePath := filepath.Join(caseDir, beforeName)
	afterPath := filepath.Join(caseDir, afterName)

	metaTOML := renderScaffoldMetaTOML(nextID, classID, title, language, category, beforeName, afterName)

	report := ScaffoldReport{
		CorpusID:   nextID,
		ClassID:    classID,
		Title:      title,
		Language:   language,
		Category:   category,
		Dir:        displayRelPath(repoRoot, caseDir),
		MetaPath:   displayRelPath(repoRoot, metaPath),
		BeforePath: displayRelPath(repoRoot, beforePath),
		AfterPath:  displayRelPath(repoRoot, afterPath),
		Written:    false,
		MetaTOML:   metaTOML,
	}

	if !writeFiles {
		return report, nil
	}

	if err := os.MkdirAll(corpusRoot, fsperm.Dir); err != nil {
		return ScaffoldReport{}, fmt.Errorf("create corpus root %s: %w", corpusRoot, err)
	}
	if _, err := os.Lstat(caseDir); err == nil {
		return ScaffoldReport{}, fmt.Errorf("refusing to overwrite existing corpus directory: %s", caseDir)
	}
	// os.Mkdir fails atomically with EEXIST if another process created caseDir.
	if err := os.Mkdir(caseDir, fsperm.Dir); err != nil {
		return ScaffoldReport{}, fmt.Errorf("create corpus directory %s: %w", caseDir, err)
	}

	caseRoot, err := os.OpenRoot(caseDir)
	if err != nil {
		return ScaffoldReport{}, fmt.Errorf("open corpus directory %s: %w", caseDir, err)
	}
	defer func() { _ = caseRoot.Close() }()

	beforeStub, afterStub := snippetStubs(language, nextID, classID)
	if err := writeExclusiveFile(caseRoot, "meta.toml", []byte(metaTOML)); err != nil {
		return ScaffoldReport{}, err
	}
	if err := writeExclusiveFile(caseRoot, beforeName, beforeStub); err != nil {
		return ScaffoldReport{}, err
	}
	if err := writeExclusiveFile(caseRoot, afterName, afterStub); err != nil {
		return ScaffoldReport{}, err
	}

	report.Written = true
	return report, nil
}

func nextCorpusID(corpusRoot string) (string, error) {
	entries, err := os.ReadDir(corpusRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "C-01", nil
		}
		return "", fmt.Errorf("read corpus directory %s: %w", corpusRoot, err)
	}
	maxNum := 0
	for _, ent := range entries {
		m := corpusIDRE.FindStringSubmatch(ent.Name())
		if m == nil {
			continue
		}
		n, convErr := strconv.Atoi(m[1])
		if convErr == nil && n > maxNum {
			maxNum = n
		}
	}
	return fmt.Sprintf("C-%02d", maxNum+1), nil
}

func snippetFilenames(language string) (string, string) {
	ext := "txt"
	switch language {
	case "go":
		ext = "go"
	case "python":
		ext = "py"
	case "rust":
		ext = "rs"
	case "typescript":
		ext = "ts"
	case "shell":
		ext = "sh"
	case "binary":
		ext = "bin"
	}
	return "before." + ext, "after." + ext
}

func snippetStubs(language, corpusID, classID string) ([]byte, []byte) {
	if language == "binary" {
		return []byte("BEFORE-STUB-" + corpusID + "\n"), []byte("AFTER-STUB-" + corpusID + "\n")
	}
	comment := "//"
	if language == "python" || language == "shell" {
		comment = "#"
	}
	before := fmt.Sprintf("%s %s (%s): TODO paste minimal BEFORE (clean/safe) snippet.\n", comment, corpusID, classID)
	after := fmt.Sprintf("%s %s (%s): TODO paste minimal AFTER (reproducing real past mistake) snippet.\n", comment, corpusID, classID)
	return []byte(before), []byte(after)
}

func renderScaffoldMetaTOML(corpusID, classID, title, language, category, beforeName, afterName string) string {
	notes := fmt.Sprintf("Scaffolded by LOOP-07 /correct for recurring class %s (%s). Replace placeholders with real mistake context and wire a detection check.", classID, title)
	sourceRef := fmt.Sprintf(".harness/runs/main/decisions.tsv#class=%s", classID)
	var b strings.Builder
	fmt.Fprintf(&b, "id = %s\n", strconv.Quote(corpusID))
	fmt.Fprintf(&b, "title = %s\n", strconv.Quote(title))
	fmt.Fprintf(&b, "category = %s\n", strconv.Quote(category))
	fmt.Fprintf(&b, "language = %s\n", strconv.Quote(language))
	fmt.Fprintf(&b, "before_file = %s\n", strconv.Quote(beforeName))
	fmt.Fprintf(&b, "after_file = %s\n", strconv.Quote(afterName))
	b.WriteString("support_files = []\n")
	b.WriteString("evaluation_mode = \"diff\"\n\n")
	b.WriteString("[origin]\n")
	b.WriteString("branch = \"TODO-fill-branch\"\n")
	b.WriteString("base_commit = \"0000000\"\n")
	b.WriteString("files = [\"TODO/path/to/affected/file\"]\n")
	b.WriteString("commits = [\"worktree-uncommitted\"]\n")
	fmt.Fprintf(&b, "source_ref = %s\n", strconv.Quote(sourceRef))
	fmt.Fprintf(&b, "notes = %s\n\n", strconv.Quote(notes))
	b.WriteString("[detection]\n")
	b.WriteString("planned_checks = [\"LOOP-07\"]\n")
	b.WriteString("active_checks = []\n")
	b.WriteString("currently_caught = false\n")
	b.WriteString("current_method = \"unassigned\"\n")
	b.WriteString("expected_signal = \"\"\n")
	return b.String()
}

func writeExclusiveFile(root *os.Root, name string, data []byte) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fsperm.File)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	return nil
}
