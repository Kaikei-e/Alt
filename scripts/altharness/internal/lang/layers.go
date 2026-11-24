package lang

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// RunLayers is .claude/skills/clean-architecture/scripts/check_layers.sh.
func RunLayers(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return RunLayersWithStdin(ctx, args, os.Stdin, stdout, stderr)
}

// RunLayersWithStdin runs the layer boundary checker with an explicit stdin reader.
func RunLayersWithStdin(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	if len(args) == 0 {
		return layerUsage(stderr)
	}
	mode := ""
	service := ""
	target := ""
	switch args[0] {
	case "--file":
		if len(args) != 2 {
			return layerUsage(stderr)
		}
		mode, target = "file", args[1]
	case "--changed":
		if len(args) != 1 {
			return layerUsage(stderr)
		}
		mode = "changed"
	case "--hook":
		if len(args) != 1 {
			return layerUsage(stderr)
		}
		mode = "hook"
	default:
		if strings.HasPrefix(args[0], "-") || len(args) != 1 {
			return layerUsage(stderr)
		}
		mode, service = "dir", args[0]
	}
	ledger, rc := loadKnownLedger(stderr)
	if rc != 0 {
		return rc
	}
	switch mode {
	case "dir":
		info, err := os.Stat(service)
		if err != nil || !info.IsDir() {
			fmt.Fprintf(stderr, "Error: '%s' not found\n", service)
			return 2
		}
		abs := service
		if resolved, err := filepath.Abs(service); err == nil {
			abs = resolved
		}
		repo := resolveLayerRoot(abs)
		rep := scanLayers(ctx, abs, repo, "", "", ledger)
		for _, line := range rep.clean {
			fmt.Fprintln(stdout, line)
		}
		for _, line := range rep.violations {
			fmt.Fprintln(stdout, line)
		}
		for _, line := range rep.warns {
			fmt.Fprintln(stdout, line)
		}
		for _, line := range ledger.knownLines() {
			fmt.Fprintln(stdout, line)
		}
		for _, line := range fixLines(rep.rules) {
			fmt.Fprintln(stdout, line)
		}
		if rep.count > 0 {
			return 1
		}
		return 0
	case "file":
		rep := checkOneFile(ctx, target, ledger)
		for _, line := range rep.collected {
			fmt.Fprintln(stdout, line)
		}
		for _, line := range fixLines(rep.rules) {
			fmt.Fprintln(stdout, line)
		}
		if rep.count > 0 {
			return 1
		}
		return 0
	case "changed":
		root := resolveLayerRoot(".")
		out, errText, code, err := capture(ctx, root, nil, "git", "-c", "core.quotePath=false", "-C", root, "status", "--porcelain", "-uall")
		if err != nil || code != 0 {
			fmt.Fprintf(stderr, "error: git status failed: %s\n", strings.TrimSpace(errText))
			return 2
		}
		var reps []layerReport
		if strings.TrimSpace(out) != "" {
			cwd, _ := os.Getwd()
			for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				if len(line) < 4 {
					continue
				}
				xy := line[:2]
				raw := line[3:]
				if strings.Contains(xy, "R") || strings.Contains(xy, "C") {
					if i := strings.LastIndex(raw, " -> "); i >= 0 {
						raw = raw[i+4:]
					}
				}
				raw = strings.Trim(raw, `"`)
				file := raw
				if cwd != root {
					file = filepath.Join(root, raw)
				}
				reps = append(reps, checkOneFile(ctx, file, ledger))
			}
		}
		merged := mergeReports(reps)
		for _, line := range merged.collected {
			fmt.Fprintln(stdout, line)
		}
		for _, line := range fixLines(merged.rules) {
			fmt.Fprintln(stdout, line)
		}
		if merged.count > 0 {
			return 1
		}
		return 0
	case "hook":
		if stdin == nil {
			return 0
		}
		data, err := io.ReadAll(stdin)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			return 0
		}
		file := extractHookFilePath(data)
		if file == "" {
			return 0
		}
		rep := checkOneFile(ctx, file, ledger)
		if rep.count > 0 {
			for _, line := range rep.collected {
				fmt.Fprintln(stderr, line)
			}
			for _, line := range fixLines(rep.rules) {
				fmt.Fprintln(stderr, line)
			}
			fmt.Fprintln(stderr, "See .claude/skills/clean-architecture/SKILL.md section 9.")
			return 2
		}
		return 0
	default:
		return layerUsage(stderr)
	}
}

func layerUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "Usage: check_layers.sh <service-dir> | --file <path> | --changed | --hook")
	return 2
}

// extractHookFilePath supports both Claude/Codex (tool_input.file_path)
// and Antigravity agy (toolCall.args.Path / toolCall.args.TargetFile) JSON payloads.
func extractHookFilePath(data []byte) string {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return ""
	}
	if tool, ok := doc["tool_input"].(map[string]any); ok {
		if file, ok := tool["file_path"].(string); ok && file != "" {
			return file
		}
	}
	if tc, ok := doc["toolCall"].(map[string]any); ok {
		if args, ok := tc["args"].(map[string]any); ok {
			for _, key := range []string{"Path", "TargetFile", "file_path"} {
				if file, ok := args[key].(string); ok && file != "" {
					return file
				}
			}
		}
	}
	return ""
}

type knownEntry struct {
	rule, prefix, substr, reason string
	count                        int
}

type knownLedger struct{ entries []knownEntry }

func (k *knownLedger) hit(rule, rel, text string) bool {
	if k == nil {
		return false
	}
	for i := range k.entries {
		e := &k.entries[i]
		if e.rule == rule && strings.HasPrefix(rel, e.prefix) && (e.substr == "*" || strings.Contains(text, e.substr)) {
			e.count++
			return true
		}
	}
	return false
}

func (k *knownLedger) totalHits() int {
	if k == nil {
		return 0
	}
	total := 0
	for _, e := range k.entries {
		total += e.count
	}
	return total
}

func (k *knownLedger) knownLines() []string {
	if k == nil {
		return nil
	}
	var lines []string
	for _, e := range k.entries {
		if e.count > 0 {
			lines = append(lines, fmt.Sprintf("KNOWN %s %s (%d lines): %s", e.rule, e.prefix, e.count, e.reason))
		}
	}
	return lines
}

func loadKnownLedger(stderr io.Writer) (*knownLedger, int) {
	path := os.Getenv("CHECK_LAYERS_KNOWN")
	if path == "" {
		if root, err := repoRoot(); err == nil {
			for _, cand := range []string{
				filepath.Join(root, ".claude", "skills", "clean-architecture", "scripts", "known_violations.tsv"),
				filepath.Join(root, ".agents", "skills", "clean-architecture", "scripts", "known_violations.tsv"),
			} {
				if st, err := os.Stat(cand); err == nil && !st.IsDir() {
					path = cand
					break
				}
			}
		}
	}
	if path == "" {
		return &knownLedger{}, 0
	}
	f, err := os.Open(path)
	if err != nil {
		return &knownLedger{}, 0
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return &knownLedger{}, 0
	}
	var ledger knownLedger
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
			fmt.Fprintf(stderr, "check_layers.sh: malformed ledger line: %s\n", line)
			return nil, 2
		}
		ledger.entries = append(ledger.entries, knownEntry{rule: parts[0], prefix: parts[1], substr: parts[2], reason: parts[3]})
	}
	return &ledger, 0
}

func resolveLayerRoot(ref string) string {
	if env := os.Getenv("CHECK_LAYERS_ROOT"); env != "" {
		if abs, err := filepath.Abs(env); err == nil {
			return abs
		}
		return env
	}
	out, err := exec.Command("git", "-C", ref, "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			if abs, err := filepath.Abs(s); err == nil {
				return abs
			}
			return s
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ref
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		return abs
	}
	return cwd
}

type layerRule struct {
	kind, rule, lang, inc, exc, pattern string
	re                                  *regexp.Regexp
}

type layerReport struct {
	violations []string
	warns      []string
	collected  []string
	clean      []string
	rules      []string
	count      int
}

func scanLayers(ctx context.Context, service, repo, filterAbs, display string, ledger *knownLedger) layerReport {
	var rep layerReport
	files, walkErr := walkSource(service)
	if walkErr != nil {
		rep.violations = append(rep.violations, fmt.Sprintf("ERROR: failed to scan %s: %v", service, walkErr))
		rep.count++
		return rep
	}
	hasGo := false
	hasPy := false
	for _, f := range files {
		switch filepath.Ext(f.rel) {
		case ".go":
			hasGo = true
		case ".py":
			hasPy = true
		}
	}
	if hasGo {
		rules := []layerRule{
			{"VIOLATION", "rest-imports-driver-gateway-or-pgx", "go", "handler", "!*_test.go", `"([^"]+/(driver|gateway)(/[^"]*)?|github\.com/jackc/pgx[^"]*)"`, nil},
			{"VIOLATION", "rest-imports-driver-gateway-or-pgx", "go", "rest", "!*_test.go", `"([^"]+/(driver|gateway)(/[^"]*)?|github\.com/jackc/pgx[^"]*)"`, nil},
			{"VIOLATION", "usecase-imports-forbidden-layers-or-sql", "go", "usecase", "!*_test.go", `"([^"]+/(driver|gateway|rest|handler)(/[^"]*)?|database/sql|github\.com/jackc/pgx[^"]*)"`, nil},
			{"WARN", "usecase-imports-otel", "go", "usecase", "!*_test.go", `"go\.opentelemetry\.io[^"]*"`, nil},
			{"VIOLATION", "port-imports-forbidden-layers-or-sql", "go", "port", "!*_test.go", `"([^"]+/(driver|gateway|rest|handler|usecase)(/[^"]*)?|database/sql|github\.com/jackc/pgx[^"]*)"`, nil},
			{"VIOLATION", "port-imports-net-http", "go", "port", "!*_test.go", `"net/http"`, nil},
			{"VIOLATION", "port-imports-otel", "go", "port", "!*_test.go", `"go\.opentelemetry\.io[^"]*"`, nil},
			{"VIOLATION", "gateway-imports-usecase-or-handler", "go", "gateway", "!*_test.go", `"([^"]+/(usecase|rest|handler)(/[^"]*)?)"`, nil},
			{"VIOLATION", "driver-imports-upward", "go", "driver", "!*_test.go", `"([^"]+/(usecase|gateway|rest|handler)(/[^"]*)?)"`, nil},
			{"VIOLATION", "driver-implements-port-skips-gateway", "go", "driver", "!*_test.go", `"([^"]+/port(/[^"]*)?)"`, nil},
			{"VIOLATION", "driver-imports-domain", "go", "driver", "!*_test.go", `"([^"]+/domain(/[^"]*)?)"`, nil},
			{"VIOLATION", "domain-imports-sibling-layers", "go", "domain", "!*_test.go", `"([^"]+/(usecase|port|gateway|driver|rest|handler)(/[^"]*)?)"`, nil},
			{"VIOLATION", "domain-imports-sql", "go", "domain", "!*_test.go", `"(database/sql|github\.com/jackc/pgx[^"]*)"`, nil},
			{"VIOLATION", "domain-imports-net-http", "go", "domain", "!*_test.go", `"net/http"`, nil},
			{"VIOLATION", "domain-imports-otel", "go", "domain", "!*_test.go", `"go\.opentelemetry\.io[^"]*"`, nil},
		}
		groups := []struct {
			label string
			dirs  []string
			from  int
			to    int
		}{
			{"[clean] Go rest/handler: clean", []string{"handler", "rest"}, 0, 2},
			{"[clean] Go usecase: clean", []string{"usecase"}, 2, 4},
			{"[clean] Go port: clean", []string{"port"}, 4, 7},
			{"[clean] Go gateway: clean", []string{"gateway"}, 7, 8},
			{"[clean] Go driver: clean", []string{"driver"}, 8, 11},
			{"[clean] Go domain: clean", []string{"domain"}, 11, 15},
		}
		for _, g := range groups {
			before := rep.count
			beforeKnown := ledger.totalHits()
			for _, rule := range rules[g.from:g.to] {
				applyRule(ctx, &rep, files, service, repo, filterAbs, display, ledger, rule)
			}
			if display == "" && filterAbs == "" && rep.count == before && ledger.totalHits() == beforeKnown && hasLayerFiles(files, g.dirs, ".go", false) {
				rep.clean = append(rep.clean, g.label)
			}
		}
	}
	if hasPy {
		rules := []layerRule{
			{"VIOLATION", "py-handler-imports-driver-or-gateway", "py", "handler", "!*test*", `^(from|import)\s+.*(\.driver|\.gateway)`, nil},
			{"VIOLATION", "py-handler-imports-driver-or-gateway", "py", "routers", "!*test*", `^(from|import)\s+.*(\.driver|\.gateway)`, nil},
			{"VIOLATION", "py-usecase-imports-infra-or-outer", "py", "usecase", "!*test*", `^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler)`, nil},
			{"VIOLATION", "py-port-imports-infra", "py", "port", "!*test*", `^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler)`, nil},
			{"VIOLATION", "py-driver-imports-upward", "py", "driver", "!*test*", `^(from|import)\s+.*(\.usecase|\.handler|\.gateway)`, nil},
			{"VIOLATION", "py-driver-imports-port-skips-gateway", "py", "driver", "!*test*", `^(from|import)\s+.*\.port`, nil},
			{"VIOLATION", "py-driver-imports-domain", "py", "driver", "!*test*", `^(from|import)\s+.*\.domain`, nil},
			{"VIOLATION", "py-domain-imports-infra-or-layers", "py", "domain", "!*test*", `^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler|\.usecase|\.port)`, nil},
		}
		groups := []struct {
			label string
			dirs  []string
			from  int
			to    int
		}{
			{"[clean] Python handler/routers: clean", []string{"handler", "routers"}, 0, 2},
			{"[clean] Python usecase: clean", []string{"usecase"}, 2, 3},
			{"[clean] Python port: clean", []string{"port"}, 3, 4},
			{"[clean] Python driver: clean", []string{"driver"}, 4, 7},
			{"[clean] Python domain: clean", []string{"domain"}, 7, 8},
		}
		for _, g := range groups {
			before := rep.count
			beforeKnown := ledger.totalHits()
			for _, rule := range rules[g.from:g.to] {
				applyRule(ctx, &rep, files, service, repo, filterAbs, display, ledger, rule)
			}
			if display == "" && filterAbs == "" && rep.count == before && ledger.totalHits() == beforeKnown && hasLayerFiles(files, g.dirs, ".py", true) {
				rep.clean = append(rep.clean, g.label)
			}
		}
	}
	return rep
}

type sourceFile struct {
	abs string
	rel string
}

func walkSource(service string) ([]sourceFile, error) {
	var files []sourceFile
	err := filepath.WalkDir(service, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".py" {
			return nil
		}
		rel, err := filepath.Rel(service, path)
		if err != nil {
			return err
		}
		files = append(files, sourceFile{abs: path, rel: filepath.ToSlash(rel)})
		return nil
	})
	return files, err
}

func applyRule(ctx context.Context, rep *layerReport, files []sourceFile, service, repo, filterAbs, display string, ledger *knownLedger, rule layerRule) {
	if ctx.Err() != nil {
		return
	}
	re, err := regexp.Compile(rule.pattern)
	if err != nil {
		return
	}
	for _, file := range files {
		if !layerMatch(file.rel, rule) {
			continue
		}
		if filterAbs != "" && filepath.Clean(file.abs) != filepath.Clean(filterAbs) {
			continue
		}
		data, err := os.ReadFile(file.abs)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !re.MatchString(line) {
				continue
			}
			shown := file.abs
			if !filepath.IsAbs(service) {
				shown = filepath.Join(service, filepath.FromSlash(file.rel))
			}
			text := fmt.Sprintf("%s:%d:%s", shown, i+1, line)
			relRepo := file.rel
			if strings.HasPrefix(file.abs, repo+string(os.PathSeparator)) {
				if rel, err := filepath.Rel(repo, file.abs); err == nil {
					relRepo = filepath.ToSlash(rel)
				}
			}
			if rule.kind == "VIOLATION" {
				if ledger.hit(rule.rule, relRepo, line) {
					continue
				}
				rep.count++
				recordRule(rep, rule.rule)
				if display == "" && filterAbs == "" {
					rep.violations = append(rep.violations, "VIOLATION "+rule.rule+" "+text)
				} else {
					name := display
					if name == "" {
						name = file.abs
					}
					rep.collected = append(rep.collected, fmt.Sprintf("VIOLATION %s %s:%d:%s", rule.rule, name, i+1, line))
				}
			} else if display == "" && filterAbs == "" {
				rep.warns = append(rep.warns, "WARN "+rule.rule+" "+text)
			}
		}
	}
}

func layerMatch(rel string, rule layerRule) bool {
	ext := ".go"
	if rule.lang == "py" {
		ext = ".py"
	}
	if filepath.Ext(rel) != ext {
		return false
	}
	if !underComponent(rel, rule.inc) {
		return false
	}
	base := filepath.Base(rel)
	switch rule.exc {
	case "!*_test.go":
		if strings.HasSuffix(base, "_test.go") {
			return false
		}
	case "!*test*":
		if strings.Contains(rel, "test") {
			return false
		}
	}
	return true
}

func underComponent(rel, name string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts[:len(parts)-1] {
		if part == name {
			return true
		}
	}
	return false
}

func hasLayerFiles(files []sourceFile, dirs []string, ext string, pyTest bool) bool {
	for _, file := range files {
		if filepath.Ext(file.rel) != ext {
			continue
		}
		base := filepath.Base(file.rel)
		if ext == ".go" && strings.HasSuffix(base, "_test.go") {
			continue
		}
		if pyTest && strings.Contains(file.rel, "test") {
			continue
		}
		for _, dir := range dirs {
			if underComponent(file.rel, dir) {
				return true
			}
		}
	}
	return false
}

func recordRule(rep *layerReport, rule string) {
	for _, existing := range rep.rules {
		if existing == rule {
			return
		}
	}
	rep.rules = append(rep.rules, rule)
}

func checkOneFile(ctx context.Context, file string, ledger *knownLedger) layerReport {
	if _, err := os.Stat(file); err != nil {
		return layerReport{}
	}
	base := filepath.Base(file)
	marker := ""
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return layerReport{}
	case strings.HasSuffix(base, ".go"):
		marker = "go.mod"
	case strings.HasSuffix(base, ".py"):
		if strings.Contains(base, "test") {
			return layerReport{}
		}
		marker = "pyproject.toml"
	default:
		return layerReport{}
	}
	dir := filepath.Dir(file)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	service := ""
	for {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			service = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if service == "" {
		return layerReport{}
	}
	absFile := file
	if abs, err := filepath.Abs(file); err == nil {
		absFile = abs
	}
	return scanLayers(ctx, service, resolveLayerRoot(service), absFile, file, ledger)
}

func mergeReports(reps []layerReport) layerReport {
	var out layerReport
	for _, rep := range reps {
		out.count += rep.count
		out.collected = append(out.collected, rep.collected...)
		for _, rule := range rep.rules {
			recordRule(&out, rule)
		}
	}
	return out
}

func fixLines(rules []string) []string {
	var lines []string
	for _, rule := range rules {
		lines = append(lines, "FIX "+rule+": "+remediation(rule))
	}
	return lines
}

func remediation(rule string) string {
	switch rule {
	case "rest-imports-driver-gateway-or-pgx":
		return "move database and external calls behind a Usecase and Port, implement that Port in Gateway over Driver, and map Domain errors to transport responses in Handler."
	case "usecase-imports-forbidden-layers-or-sql":
		return "define a narrow Port interface in port/ for the capability, depend on that Port in Usecase, and move concrete Gateway, Driver, or SQL calls into Gateway and Driver."
	case "port-imports-forbidden-layers-or-sql":
		return "keep Port method signatures purely in terms of Domain types and standard library primitives, and move concrete adapter or SQL types into Gateway and Driver."
	case "port-imports-net-http":
		return "replace net/http types in Port signatures with Domain values or plain parameters and keep HTTP transport details inside Handler or Driver."
	case "port-imports-otel":
		return "remove OpenTelemetry types from Port contracts; propagate trace context via context.Context and keep span instrumentation in adapters or Composition Root."
	case "gateway-imports-usecase-or-handler":
		return "Gateway may import only Port, Driver, and Domain; move shared interfaces to port/ and shared data types to domain/ instead of importing Usecase or Handler."
	case "driver-imports-upward":
		return "a Driver performs raw I/O only and must not import Handler, Usecase, or Gateway; move orchestration upward and pass plain parameters into the Driver."
	case "driver-implements-port-skips-gateway":
		return "do not import Port in Driver; return Driver-owned row/payload types from Driver and implement the Port interface in a Gateway that wraps it."
	case "driver-imports-domain":
		return "a Driver owns its own row/payload types; return those and move the mapping to Domain types into the Gateway that calls it."
	case "domain-imports-sibling-layers":
		return "keep Domain restricted to the standard library; move external capability interfaces to port/ and orchestration or adapter logic to Usecase or Gateway."
	case "domain-imports-sql":
		return "remove database/sql and pgx types from Domain models; define row structs in Driver and map between rows and pure Domain types in Gateway."
	case "domain-imports-net-http":
		return "remove net/http types from Domain; keep HTTP transport DTOs in Handler or raw HTTP calls in Driver and map to pure Domain values at the boundary."
	case "domain-imports-otel":
		return "keep Domain free of OpenTelemetry imports; propagate context.Context and put span instrumentation in Handler, Gateway, or Driver."
	case "py-handler-imports-driver-or-gateway":
		return "call a Usecase from the Handler/Router instead of importing Gateway or Driver directly, and wire concrete adapters in the Composition Root."
	case "py-usecase-imports-infra-or-outer":
		return "define a Port protocol/ABC in port/ for the external capability, call that Port from Usecase, and move HTTP, DB, or framework imports into Gateway, Driver, or Handler."
	case "py-port-imports-infra":
		return "keep Port definitions free of infrastructure and web-framework packages, expressing contracts with Domain and standard library types only."
	case "py-driver-imports-upward":
		return "a Driver performs raw I/O only and must not import Handler, Usecase, or Gateway; move higher-level types or orchestration upward and pass plain parameters into Driver."
	case "py-driver-imports-port-skips-gateway":
		return "do not import Port in Driver; return Driver-owned payload/row types from Driver and implement the Port in a Gateway that maps them to Domain models."
	case "py-driver-imports-domain":
		return "a Driver owns its own row/payload types; return those from Driver and move the mapping to Domain models into the Gateway that calls it."
	case "py-domain-imports-infra-or-layers":
		return "keep Domain pure (standard library only); move capability protocols to port/ and move infrastructure or framework dependencies to Handler, Gateway, or Driver."
	default:
		return "move the forbidden cross-layer or infrastructure dependency to the appropriate outer adapter and depend inward via Port and Domain."
	}
}
