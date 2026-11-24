// Package hygiene is the CHECK-06 worktree scan: zero-byte truncations and
// untracked ELF/Mach-O/PE artifacts. An empty candidate set is not success
// when git itself fails; that path exits 2.
package hygiene

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	exitOK         = 0
	exitViolations = 1
	exitUsage      = 2
)

var excludedPrefixes = []string{"stopped-using-k8s/", "scripts/harness/corpus/"}

var defaultGlobs = []string{"**/__init__.py", "**/.gitkeep", "**/py.typed"}

var machoSingle = map[string]struct{}{
	"\xfe\xed\xfa\xce": {},
	"\xce\xfa\xed\xfe": {},
	"\xfe\xed\xfa\xcf": {},
	"\xcf\xfa\xed\xfe": {},
}

// Finding is one hygiene violation.
type Finding struct {
	Code   string
	Path   string
	Detail string
}

// Run is the CLI matching check_worktree_hygiene.py.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	repo := ""
	allow := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--repo-root":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "ERROR: --repo-root requires a value")
				return exitUsage
			}
			i++
			repo = args[i]
		case "--allowlist":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "ERROR: --allowlist requires a value")
				return exitUsage
			}
			i++
			allow = args[i]
		case "-h", "--help":
			fmt.Fprintln(stdout, "usage: hygiene [--repo-root dir] [--allowlist file]")
			return 0
		default:
			fmt.Fprintf(stderr, "ERROR: unknown argument %q\n", args[i])
			return exitUsage
		}
	}
	if repo == "" {
		repo = defaultRoot(ctx)
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return exitUsage
	}
	var allowPath string
	if allow != "" {
		allowPath, err = filepath.Abs(allow)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return exitUsage
		}
	}
	findings, stats, err := Check(ctx, abs, allowPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return exitUsage
	}
	if len(findings) > 0 {
		for _, f := range findings {
			fmt.Fprintf(stdout, "[%s] %s: %s\n", f.Code, f.Path, f.Detail)
		}
		return exitViolations
	}
	fmt.Fprintf(stdout, "OK: worktree hygiene clean (tracked=%d, untracked=%d, binary_candidates=%d)\n",
		stats.Tracked, stats.Untracked, stats.BinaryCandidates)
	return exitOK
}

// Stats counts what the scan inspected.
type Stats struct {
	Tracked          int
	Untracked        int
	BinaryCandidates int
}

// Check scans repoRoot. allowlist may be empty to use the repo ledger.
func Check(ctx context.Context, repoRoot, allowlist string) ([]Finding, Stats, error) {
	if err := verifyGit(ctx, repoRoot); err != nil {
		return nil, Stats{}, err
	}
	regexes, err := loadAllowlist(repoRoot, allowlist)
	if err != nil {
		return nil, Stats{}, err
	}
	tracked, err := listTracked(ctx, repoRoot)
	if err != nil {
		return nil, Stats{}, err
	}
	untracked, err := listZ(ctx, repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, Stats{}, err
	}
	staged, err := listZ(ctx, repoRoot, "diff", "--cached", "--name-only", "--diff-filter=AM", "-z")
	if err != nil {
		return nil, Stats{}, err
	}
	stagedAdded, err := listZ(ctx, repoRoot, "diff", "--cached", "--name-only", "--diff-filter=A", "-z")
	if err != nil {
		return nil, Stats{}, err
	}
	var findings []Finding
	checkTrackedZero(ctx, repoRoot, tracked, &findings)
	checkUntrackedZero(repoRoot, untracked, regexes, &findings)
	checkStagedZero(repoRoot, stagedAdded, regexes, &findings)
	nBin := checkBinaries(repoRoot, untracked, staged, &findings)
	return findings, Stats{Tracked: len(tracked), Untracked: len(untracked), BinaryCandidates: nBin}, nil
}

func verifyGit(ctx context.Context, repoRoot string) error {
	st, err := os.Stat(repoRoot)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("repository root is not a directory: %s", repoRoot)
	}
	out, err := git(ctx, repoRoot, nil, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("not inside a git worktree: %s", repoRoot)
	}
	return nil
}

func loadAllowlist(repoRoot, explicit string) ([]*regexp.Regexp, error) {
	var globs []string
	path := explicit
	if path == "" {
		candidate := filepath.Join(repoRoot, "scripts", "lint", "empty_file_allowlist.toml")
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
		}
	} else if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("allowlist file not found: %s", path)
	}
	if path == "" {
		globs = append(globs, defaultGlobs...)
	} else {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse allowlist %s: %w", path, err)
		}
		var doc map[string]any
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("failed to parse allowlist %s: %w", path, err)
		}
		section, _ := doc["untracked_empty"].(map[string]any)
		rawGlobs, _ := section["globs"].([]any)
		if len(rawGlobs) == 0 {
			return nil, fmt.Errorf("allowlist %s must define [untracked_empty].globs as a list of strings", path)
		}
		for _, item := range rawGlobs {
			s, ok := item.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("allowlist %s must define [untracked_empty].globs as a list of strings", path)
			}
			globs = append(globs, s)
		}
	}
	out := make([]*regexp.Regexp, 0, len(globs))
	for _, g := range globs {
		out = append(out, globToRegex(g))
	}
	return out, nil
}

func globToRegex(pattern string) *regexp.Regexp {
	pat := strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pat); {
		if strings.HasPrefix(pat[i:], "**/") {
			b.WriteString("(?:.+/)?")
			i += 3
			continue
		}
		if strings.HasPrefix(pat[i:], "/**") && i+3 == len(pat) {
			b.WriteString("/.+")
			i += 3
			continue
		}
		if strings.HasPrefix(pat[i:], "**") {
			b.WriteString(".*")
			i += 2
			continue
		}
		switch pat[i] {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(pat[i : i+1]))
		}
		i++
	}
	b.WriteByte('$')
	return regexp.MustCompile(b.String())
}

func listTracked(ctx context.Context, repoRoot string) ([]string, error) {
	out, err := git(ctx, repoRoot, nil, "ls-files", "-s", "-z")
	if err != nil {
		return nil, fmt.Errorf("git ls-files failed: %s", strings.TrimSpace(err.Error()))
	}
	seen := map[string]struct{}{}
	var files []string
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		meta, path, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok {
			continue
		}
		mode, _, _ := bytes.Cut(meta, []byte{' '})
		if string(mode) != "100644" && string(mode) != "100755" {
			continue
		}
		rel := string(path)
		if excluded(rel) {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		files = append(files, rel)
	}
	return files, nil
}

func listZ(ctx context.Context, repoRoot string, args ...string) ([]string, error) {
	out, err := git(ctx, repoRoot, nil, args...)
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(err.Error()))
	}
	var paths []string
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		rel := string(entry)
		if !excluded(rel) {
			paths = append(paths, rel)
		}
	}
	return paths, nil
}

func excluded(rel string) bool {
	for _, p := range excludedPrefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

func checkTrackedZero(ctx context.Context, repoRoot string, tracked []string, findings *[]Finding) {
	var zeros []string
	for _, rel := range tracked {
		st, err := os.Lstat(filepath.Join(repoRoot, rel))
		if err != nil || !st.Mode().IsRegular() || st.Size() != 0 {
			continue
		}
		zeros = append(zeros, rel)
	}
	sizes := headSizes(ctx, repoRoot, zeros)
	for _, rel := range zeros {
		if sizes[rel] > 0 {
			*findings = append(*findings, Finding{
				Code:   "hygiene.zero-byte-tracked",
				Path:   rel,
				Detail: fmt.Sprintf("tracked file was %d bytes in HEAD but is now 0 bytes in the working tree", sizes[rel]),
			})
		}
	}
}

func headSizes(ctx context.Context, repoRoot string, rels []string) map[string]int {
	sizes := map[string]int{}
	if len(rels) == 0 || !hasHead(ctx, repoRoot) {
		return sizes
	}
	var batch []string
	for _, rel := range rels {
		if strings.ContainsAny(rel, "\n\r") {
			out, err := git(ctx, repoRoot, nil, "cat-file", "-s", "HEAD:"+rel)
			if err == nil {
				s := strings.TrimSpace(string(out))
				var n int
				if _, err := fmt.Sscan(s, &n); err == nil {
					sizes[rel] = n
				}
			}
			continue
		}
		batch = append(batch, rel)
	}
	if len(batch) == 0 {
		return sizes
	}
	var stdin bytes.Buffer
	for _, p := range batch {
		fmt.Fprintf(&stdin, "HEAD:%s\n", p)
	}
	out, err := git(ctx, repoRoot, stdin.Bytes(), "cat-file", "--batch-check=%(objecttype) %(objectsize)")
	if err != nil {
		return sizes
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for i, rel := range batch {
		if i >= len(lines) {
			break
		}
		parts := strings.Fields(lines[i])
		if len(parts) == 2 && parts[0] == "blob" {
			var n int
			if _, err := fmt.Sscan(parts[1], &n); err == nil {
				sizes[rel] = n
			}
		}
	}
	return sizes
}

func hasHead(ctx context.Context, repoRoot string) bool {
	_, err := git(ctx, repoRoot, nil, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	return err == nil
}

func checkUntrackedZero(repoRoot string, files []string, rx []*regexp.Regexp, findings *[]Finding) {
	for _, rel := range files {
		st, err := os.Lstat(filepath.Join(repoRoot, rel))
		if err != nil || !st.Mode().IsRegular() || st.Size() != 0 {
			continue
		}
		if matchAny(rx, rel) {
			continue
		}
		*findings = append(*findings, Finding{
			Code: "hygiene.zero-byte-untracked", Path: rel,
			Detail: "untracked 0-byte file is not matched by scripts/lint/empty_file_allowlist.toml",
		})
	}
}

func checkStagedZero(repoRoot string, files []string, rx []*regexp.Regexp, findings *[]Finding) {
	for _, rel := range files {
		if matchAny(rx, rel) {
			continue
		}
		st, err := os.Lstat(filepath.Join(repoRoot, rel))
		if err != nil || !st.Mode().IsRegular() || st.Size() != 0 {
			continue
		}
		*findings = append(*findings, Finding{
			Code: "hygiene.zero-byte-staged", Path: rel,
			Detail: "newly staged 0-byte file is not matched by scripts/lint/empty_file_allowlist.toml",
		})
	}
}

func matchAny(rx []*regexp.Regexp, rel string) bool {
	for _, r := range rx {
		if r.MatchString(rel) {
			return true
		}
	}
	return false
}

func checkBinaries(repoRoot string, untracked, staged []string, findings *[]Finding) int {
	sources := map[string][]string{}
	for _, rel := range untracked {
		sources[rel] = append(sources[rel], "untracked")
	}
	for _, rel := range staged {
		sources[rel] = append(sources[rel], "staged")
	}
	keys := make([]string, 0, len(sources))
	for k := range sources {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, rel := range keys {
		fmtName := detectBinary(filepath.Join(repoRoot, rel))
		if fmtName == "" {
			continue
		}
		*findings = append(*findings, Finding{
			Code: "hygiene.binary-artifact", Path: rel,
			Detail: fmt.Sprintf("detected %s binary artifact (%s)", fmtName, strings.Join(sources[rel], "+")),
		})
	}
	return len(sources)
}

func detectBinary(path string) string {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() < 4 {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	header := make([]byte, 64)
	n, _ := f.Read(header)
	header = header[:n]
	if bytes.HasPrefix(header, []byte{0x7f, 'E', 'L', 'F'}) {
		return "ELF"
	}
	if len(header) >= 4 {
		if _, ok := machoSingle[string(header[:4])]; ok {
			return "Mach-O"
		}
	}
	if bytes.HasPrefix(header, []byte{0xca, 0xfe, 0xba, 0xbe}) && len(header) >= 8 {
		nfat := binary.BigEndian.Uint32(header[4:8])
		if nfat >= 1 && nfat <= 32 {
			return "Mach-O universal"
		}
	}
	if bytes.HasPrefix(header, []byte{'M', 'Z'}) && st.Size() >= 64 && len(header) >= 64 {
		e := binary.LittleEndian.Uint32(header[0x3C:0x40])
		if int64(e) <= st.Size()-4 {
			if _, err := f.Seek(int64(e), io.SeekStart); err == nil {
				sig := make([]byte, 4)
				if _, err := io.ReadFull(f, sig); err == nil && bytes.Equal(sig, []byte{'P', 'E', 0, 0}) {
					return "PE"
				}
			}
		}
	}
	return ""
}

func git(ctx context.Context, repo string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return out, err
	}
	return out, nil
}

func defaultRoot(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
