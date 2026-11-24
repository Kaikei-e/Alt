package ratchet

import (
	"alt/scripts/altharness/internal/lint"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type findingItem struct {
	rule       string
	relFile    string
	line       int
	lineSHA256 string
}

type fpKey struct {
	rule string
	file string
	sha  string
}

func lineFingerprint(line string) string {
	sum := sha256.Sum256([]byte(normalizeSourceLine(line)))
	return hex.EncodeToString(sum[:])
}

func readTextStrict(path string) (string, error) {
	b, err := readOS(path)
	if err != nil {
		return "", err
	}
	return decodeUTF8Strict(b)
}

func readOS(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s", pyOSError(err))
	}
	return b, nil
}

func gitBin() (string, error) {
	p, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git")
	}
	return p, nil
}

func gitLSFiles(ctx context.Context, repoRoot string) ([]string, error) {
	bin, err := gitBin()
	if err != nil {
		return nil, err
	}
	argv := []string{bin, "-C", repoRoot, "ls-files", "--cached", "--others", "--exclude-standard"}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return nil, cmdFail(argv, err)
	}
	seen := map[string]struct{}{}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		files = append(files, line)
	}
	sort.Strings(files)
	return files, nil
}

func cmdFail(argv []string, err error) error {
	if ee, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("Command '%s' returned non-zero exit status %d.", pyListRepr(argv), ee.ExitCode())
	}
	return err
}

func discoverDirectoryTargets(ctx context.Context, repoRoot string, tracked []string) ([]string, error) {
	runSh := filepath.Join(repoRoot, "scripts", "lint", "run.sh")
	if isRegularFile(runSh) {
		var out bytes.Buffer
		if code := lint.Run(ctx, repoRoot, []string{"--list"}, &out, io.Discard); code != 0 {
			return nil, fmt.Errorf("in-process lint target discovery failed with exit status %d", code)
		}
		set := map[string]struct{}{}
		for _, t := range strings.Split(out.String(), "\n") {
			if strings.TrimSpace(t) == "" {
				continue
			}
			name := strings.TrimRight(strings.TrimSpace(t), "/")
			if isDir(filepath.Join(repoRoot, strings.TrimSpace(t))) {
				set[name] = struct{}{}
			}
		}
		return sortedKeys(set), nil
	}
	set := map[string]struct{}{}
	for _, rel := range tracked {
		if strings.HasPrefix(rel, "stopped-using-k8s/") || strings.HasPrefix(rel, "scripts/harness/corpus/") {
			continue
		}
		base := rel
		dir := ""
		if i := strings.LastIndex(rel, "/"); i >= 0 {
			base = rel[i+1:]
			dir = rel[:i]
		}
		if _, ok := markerNames[base]; ok && dir != "" {
			set[dir] = struct{}{}
		}
	}
	return sortedKeys(set), nil
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isSafeRepoFile(full, repoRoot string) bool {
	if isSymlink(full) {
		return false
	}
	resolved, err := resolvePath(full)
	if err != nil {
		return false
	}
	repoResolved, err := resolvePath(repoRoot)
	if err != nil {
		return false
	}
	if _, ok := relTo(repoResolved, resolved); !ok {
		return false
	}
	return isRegularFile(full)
}

func collectRepoCounts(ctx context.Context, repoRoot string) (map[string]map[string]int, error) {
	tracked, err := gitLSFiles(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	targets, err := discoverDirectoryTargets(ctx, repoRoot, tracked)
	if err != nil {
		return nil, err
	}
	totals := map[string]map[string]int{}
	for _, rel := range tracked {
		if isExcludedSourcePath(rel) {
			continue
		}
		full := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if !isSafeRepoFile(full, repoRoot) {
			continue
		}
		b, err := readOS(full)
		if err != nil {
			return nil, err
		}
		text := decodeUTF8Replace(b)
		fileCounts := countFileSuppressions(rel, text)
		if len(fileCounts) == 0 {
			continue
		}
		target := resolveTarget(rel, targets)
		bucket := totals[target]
		if bucket == nil {
			bucket = map[string]int{}
			totals[target] = bucket
		}
		for kind, n := range fileCounts {
			bucket[kind] += n
		}
	}
	out := map[string]map[string]int{}
	for target, bucket := range totals {
		sum := 0
		kinds := map[string]int{}
		for k, n := range bucket {
			if n > 0 {
				kinds[k] = n
				sum += n
			}
		}
		if sum > 0 {
			out[target] = kinds
		}
	}
	return out, nil
}

func parseFileURI(raw string) (path, host string, ok bool) {
	if !strings.HasPrefix(raw, "file://") {
		return "", "", false
	}
	rest := strings.TrimPrefix(raw, "file://")
	if strings.HasPrefix(rest, "/") {
		p, err := url.PathUnescape(rest)
		if err != nil {
			p = rest
		}
		return p, "", true
	}
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", rest, true
	}
	host = rest[:slash]
	p, err := url.PathUnescape(rest[slash:])
	if err != nil {
		p = rest[slash:]
	}
	return p, host, true
}

func resolveFindingRel(raw, repoRoot, projectDir string) (string, error) {
	if raw == "" || strings.Contains(raw, "\x00") {
		return "", fmt.Errorf("invalid empty or NUL finding path: %s", pyReprString(raw))
	}
	candidate := raw
	if strings.HasPrefix(candidate, "file://") {
		p, host, _ := parseFileURI(candidate)
		if host != "" && host != "localhost" {
			return "", fmt.Errorf("unsupported non-local file URI in finding path: %s", pyReprString(raw))
		}
		candidate = p
	}
	normalized := strings.ReplaceAll(candidate, "\\", "/")
	if hasDotDot(normalized) {
		return "", fmt.Errorf("finding path must not contain '..': %s", pyReprString(raw))
	}
	if strings.HasPrefix(normalized, "/") {
		return validateContained(normalized, repoRoot, raw)
	}
	if projectDir != "" {
		projFull := filepath.Join(projectDir, filepath.FromSlash(normalized))
		if isSymlink(projFull) || pathExists(projFull) {
			return validateContained(projFull, repoRoot, raw)
		}
		repoResolved, err := resolvePath(repoRoot)
		if err != nil {
			return "", err
		}
		projResolved, err := resolvePath(projectDir)
		if err != nil {
			return "", err
		}
		if projRel, ok := relTo(repoResolved, projResolved); ok && projRel != "." && projRel != "" {
			projParts := strings.Split(projRel, "/")
			pParts := strings.Split(normalized, "/")
			if len(pParts) >= len(projParts) && samePrefix(pParts, projParts) {
				repoFull := filepath.Join(repoRoot, filepath.FromSlash(normalized))
				if isSymlink(repoFull) || pathExists(repoFull) {
					return validateContained(repoFull, repoRoot, raw)
				}
			}
		}
		return validateContained(projFull, repoRoot, raw)
	}
	full := filepath.Join(repoRoot, filepath.FromSlash(normalized))
	return validateContained(full, repoRoot, raw)
}

func samePrefix(parts, prefix []string) bool {
	for i := range prefix {
		if parts[i] != prefix[i] {
			return false
		}
	}
	return true
}

func validateContained(full, repoRoot, raw string) (string, error) {
	if isSymlink(full) {
		return "", fmt.Errorf("finding path must not be a symlink: %s", pyReprString(raw))
	}
	resolved, err := resolvePath(full)
	if err != nil {
		return "", err
	}
	repoResolved, err := resolvePath(repoRoot)
	if err != nil {
		return "", err
	}
	rel, ok := relTo(repoResolved, resolved)
	if !ok {
		return "", fmt.Errorf("finding path escapes repository root: %s", pyReprString(raw))
	}
	cursor := repoResolved
	if rel != "." {
		for _, part := range strings.Split(rel, "/") {
			if part == "" {
				continue
			}
			cursor = cursor + "/" + part
			if isSymlink(cursor) {
				return "", fmt.Errorf("finding path traverses a symlink: %s", pyReprString(raw))
			}
		}
	}
	return rel, nil
}

type lineCache map[string][]string

func extractSourceLine(repoRoot, relFile string, lineNo int, override *string, cache lineCache) (string, error) {
	if override != nil {
		return *override, nil
	}
	if relFile == "" || strings.Contains(relFile, "\x00") || strings.HasPrefix(relFile, "/") || hasDotDot(relFile) {
		return "", fmt.Errorf("invalid relative finding path: %s", pyReprString(relFile))
	}
	full := filepath.Join(repoRoot, filepath.FromSlash(relFile))
	if _, err := validateContained(full, repoRoot, relFile); err != nil {
		return "", err
	}
	if !isRegularFile(full) {
		return "", fmt.Errorf("finding source file does not exist: %s", pyReprString(relFile))
	}
	lines, ok := cache[relFile]
	if !ok {
		b, err := readOS(full)
		if err != nil {
			return "", err
		}
		lines = splitLines(decodeUTF8Replace(b))
		cache[relFile] = lines
	}
	if lineNo < 1 {
		return "", fmt.Errorf("finding line %d out of range for %s (1..%d)", lineNo, pyReprString(relFile), len(lines))
	}
	if len(lines) == 0 {
		if lineNo == 1 {
			return "", nil
		}
		return "", fmt.Errorf("finding line %d out of range for empty file %s", lineNo, pyReprString(relFile))
	}
	if lineNo > len(lines) {
		return "", fmt.Errorf("finding line %d out of range for %s (1..%d)", lineNo, pyReprString(relFile), len(lines))
	}
	return lines[lineNo-1], nil
}

func buildFinding(repoRoot, projectDir, rule, fileRaw string, line int, override *string, cache lineCache) (findingItem, error) {
	rel, err := resolveFindingRel(fileRaw, repoRoot, projectDir)
	if err != nil {
		return findingItem{}, err
	}
	src, err := extractSourceLine(repoRoot, rel, line, override, cache)
	if err != nil {
		return findingItem{}, err
	}
	return findingItem{
		rule:       strings.TrimSpace(rule),
		relFile:    rel,
		line:       line,
		lineSHA256: lineFingerprint(src),
	}, nil
}

func orChain(entry jval, keys ...string) jval {
	cur := jval{kind: jNull}
	for _, k := range keys {
		v, ok := entry.field(k)
		if !ok {
			v = jval{kind: jNull}
		}
		if !cur.truthy() {
			cur = v
		}
		if cur.truthy() {
			return cur
		}
	}
	return cur
}

func loadFindingsJSON(text, repoRoot, projectDir string) ([]findingItem, error) {
	raw, err := parseJSON(text)
	if err != nil {
		return nil, err
	}
	var items []jval
	if raw.kind == jObj {
		f, ok := raw.field("findings")
		if ok && f.kind == jArr {
			items = f.arr
		} else {
			return nil, fmt.Errorf("findings JSON must be a list or an object containing a 'findings' list")
		}
	} else if raw.kind == jArr {
		items = raw.arr
	} else {
		return nil, fmt.Errorf("findings JSON must be a list or an object containing a 'findings' list")
	}
	cache := lineCache{}
	var out []findingItem
	for _, entry := range items {
		if entry.kind != jObj {
			continue
		}
		ruleV := orChain(entry, "rule", "code")
		if ruleV.kind == jNull && entry.has("filename") && entry.has("location") {
			ruleV = jval{kind: jStr, s: "syntax-error"}
		}
		fileV := orChain(entry, "file", "filename", "path")
		lineV, hasLine := entry.field("line")
		if !hasLine || lineV.kind == jNull {
			if loc, ok := entry.field("location"); ok && loc.kind == jObj {
				if row, ok := loc.field("row"); ok {
					lineV = row
				}
			}
		}
		rule, rok := ruleV.asString()
		fileRaw, fok := fileV.asString()
		line, lok := lineV.asInt()
		if !rok || !fok || !lok {
			return nil, fmt.Errorf("invalid finding entry (expected rule/code, file/filename, line/location.row): %s", entry.repr())
		}
		var override *string
		if src, ok := entry.field("source_line"); ok && src.kind == jStr {
			s := src.s
			override = &s
		}
		item, err := buildFinding(repoRoot, projectDir, rule, fileRaw, int(line), override, cache)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func loadGolangciSARIF(text, repoRoot, projectDir string) ([]findingItem, error) {
	raw, err := parseJSON(text)
	if err != nil {
		return nil, err
	}
	runs, ok := raw.field("runs")
	if raw.kind != jObj || !ok || runs.kind != jArr {
		return nil, fmt.Errorf("SARIF payload must be an object with a 'runs' list")
	}
	cache := lineCache{}
	var out []findingItem
	for _, run := range runs.arr {
		if run.kind != jObj {
			continue
		}
		results, _ := run.field("results")
		if results.kind != jArr {
			continue
		}
		for _, result := range results.arr {
			if result.kind != jObj {
				continue
			}
			ruleID, _ := result.field("ruleId")
			rule, rok := ruleID.asString()
			if !rok || strings.TrimSpace(rule) == "" {
				return nil, fmt.Errorf("SARIF result missing ruleId: %s", result.repr())
			}
			rule = strings.TrimSpace(rule)
			if rule == "gosec" {
				msgText := ""
				if msg, ok := result.field("message"); ok && msg.kind == jObj {
					if t, ok := msg.field("text"); ok {
						if s, ok := t.asString(); ok {
							msgText = s
						}
					}
				}
				if m := gosecMsgRule.FindStringSubmatch(strings.TrimSpace(msgText)); m != nil {
					rule = m[1]
				}
			}
			locs, _ := result.field("locations")
			if locs.kind != jArr || len(locs.arr) == 0 {
				return nil, fmt.Errorf("SARIF result missing locations: %s", result.repr())
			}
			loc0 := locs.arr[0]
			var phys jval
			if loc0.kind == jObj {
				phys, _ = loc0.field("physicalLocation")
			}
			var art, reg jval
			if phys.kind == jObj {
				art, _ = phys.field("artifactLocation")
				reg, _ = phys.field("region")
			}
			var uri string
			var start int64
			uok, sok := false, false
			if art.kind == jObj {
				if u, ok := art.field("uri"); ok {
					uri, uok = u.asString()
				}
			}
			if reg.kind == jObj {
				if s, ok := reg.field("startLine"); ok {
					start, sok = s.asInt()
				}
			}
			if !uok || !sok {
				return nil, fmt.Errorf("SARIF location missing artifactLocation.uri or region.startLine: %s", result.repr())
			}
			item, err := buildFinding(repoRoot, projectDir, rule, uri, int(start), nil, cache)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
	}
	return out, nil
}

func loadClippyJSON(text, repoRoot, projectDir string) ([]findingItem, error) {
	cache := lineCache{}
	var out []findingItem
	for _, rawLine := range splitLines(text) {
		stripped := strings.TrimSpace(rawLine)
		if stripped == "" || !strings.HasPrefix(stripped, "{") {
			continue
		}
		obj, err := parseJSON(stripped)
		if err != nil {
			return nil, err
		}
		if obj.kind != jObj {
			continue
		}
		reason, _ := obj.field("reason")
		rs, _ := reason.asString()
		if rs != "compiler-message" {
			continue
		}
		msg, ok := obj.field("message")
		if !ok || msg.kind != jObj {
			continue
		}
		level, _ := msg.field("level")
		lv, _ := level.asString()
		if lv != "error" && lv != "warning" {
			continue
		}
		spans, _ := msg.field("spans")
		codeObj, _ := msg.field("code")
		var codeVal jval
		if codeObj.kind == jObj {
			codeVal, _ = codeObj.field("code")
		}
		codeStr, _ := codeVal.asString()
		spanList := []jval{}
		if spans.kind == jArr {
			spanList = spans.arr
		}
		if !codeVal.truthy() && len(spanList) == 0 {
			continue
		}
		rule := "rustc-error"
		if codeVal.kind == jStr && strings.TrimSpace(codeStr) != "" {
			rule = codeStr
		}
		var primary jval
		found := false
		for _, sp := range spanList {
			if sp.kind != jObj {
				continue
			}
			if flag, ok := sp.field("is_primary"); ok && flag.truthy() {
				primary = sp
				found = true
				break
			}
		}
		if !found {
			if len(spanList) > 0 && spanList[0].kind == jObj {
				primary = spanList[0]
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("clippy compiler-message missing span: %s", msg.repr())
		}
		fn, _ := primary.field("file_name")
		ls, _ := primary.field("line_start")
		fileName, fok := fn.asString()
		line, lok := ls.asInt()
		if !fok || !lok {
			return nil, fmt.Errorf("clippy span missing file_name or line_start: %s", primary.repr())
		}
		item, err := buildFinding(repoRoot, projectDir, rule, fileName, int(line), nil, cache)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func loadDenoJSON(text, repoRoot, projectDir string) ([]findingItem, error) {
	raw, err := parseJSON(text)
	if err != nil {
		return nil, err
	}
	if raw.kind != jObj {
		return nil, fmt.Errorf("deno lint JSON must be an object")
	}
	errs, _ := raw.field("errors")
	if errs.kind == jArr && len(errs.arr) > 0 {
		return nil, fmt.Errorf("deno lint reported %d parse/config error(s): %s", len(errs.arr), errs.arr[0].repr())
	}
	diags, ok := raw.field("diagnostics")
	if !ok || diags.kind == jNull {
		diags = jval{kind: jArr}
	}
	if diags.kind != jArr {
		return nil, fmt.Errorf("deno lint JSON 'diagnostics' must be a list")
	}
	cache := lineCache{}
	var out []findingItem
	for _, diag := range diags.arr {
		if diag.kind != jObj {
			continue
		}
		code, _ := diag.field("code")
		filename, _ := diag.field("filename")
		rng, _ := diag.field("range")
		var start jval
		if rng.kind == jObj {
			start, _ = rng.field("start")
		}
		var lineV jval
		if start.kind == jObj {
			lineV, _ = start.field("line")
		}
		cs, cok := code.asString()
		fs, fok := filename.asString()
		ln, lok := lineV.asInt()
		if !cok || !fok || !lok {
			return nil, fmt.Errorf("invalid deno lint diagnostic: %s", diag.repr())
		}
		item, err := buildFinding(repoRoot, projectDir, cs, fs, int(ln), nil, cache)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func biomeLine(repoRoot, relFile string, loc jval) (int, error) {
	if start, ok := loc.field("start"); ok && start.kind == jObj {
		if line, ok := start.field("line"); ok {
			if n, ok := line.asInt(); ok {
				if n < 1 {
					return 1, nil
				}
				return int(n), nil
			}
		}
	}
	if span, ok := loc.field("span"); ok && span.kind == jArr && len(span.arr) > 0 {
		if off, ok := span.arr[0].asInt(); ok {
			if off < 0 {
				off = 0
			}
			if src, ok := loc.field("sourceCode"); ok && src.kind == jStr {
				b := []byte(src.s)
				if int(off) > len(b) {
					off = int64(len(b))
				}
				return bytesCountNL(b[:off]) + 1, nil
			}
			full := filepath.Join(repoRoot, filepath.FromSlash(relFile))
			if _, err := validateContained(full, repoRoot, relFile); err != nil {
				return 0, err
			}
			if isRegularFile(full) {
				b, err := readOS(full)
				if err != nil {
					return 0, err
				}
				if int(off) > len(b) {
					off = int64(len(b))
				}
				return bytesCountNL(b[:off]) + 1, nil
			}
		}
	}
	return 1, nil
}

func bytesCountNL(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func loadBiomeJSON(text, repoRoot, projectDir string) ([]findingItem, error) {
	raw, err := parseJSON(text)
	if err != nil {
		return nil, err
	}
	diags, ok := raw.field("diagnostics")
	if raw.kind != jObj || !ok || diags.kind != jArr {
		return nil, fmt.Errorf("biome JSON must be an object with a 'diagnostics' list")
	}
	cache := lineCache{}
	var out []findingItem
	for _, diag := range diags.arr {
		if diag.kind != jObj {
			continue
		}
		cat, _ := diag.field("category")
		loc, _ := diag.field("location")
		cs, cok := cat.asString()
		if !cok || loc.kind != jObj {
			continue
		}
		pathField, _ := loc.field("path")
		var fileRaw string
		switch pathField.kind {
		case jObj:
			if f, ok := pathField.field("file"); ok {
				fileRaw, _ = f.asString()
			}
		case jStr:
			fileRaw = pathField.s
		}
		if strings.TrimSpace(fileRaw) == "" {
			return nil, fmt.Errorf("biome diagnostic missing location.path.file: %s", diag.repr())
		}
		rel, err := resolveFindingRel(fileRaw, repoRoot, projectDir)
		if err != nil {
			return nil, err
		}
		lineNo, err := biomeLine(repoRoot, rel, loc)
		if err != nil {
			return nil, err
		}
		src, err := extractSourceLine(repoRoot, rel, lineNo, nil, cache)
		if err != nil {
			return nil, err
		}
		out = append(out, findingItem{
			rule:       strings.TrimSpace(cs),
			relFile:    rel,
			line:       lineNo,
			lineSHA256: lineFingerprint(src),
		})
	}
	return out, nil
}

func loadFindingsFile(path string) (string, error) {
	return readTextStrict(path)
}
