package ratchet

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	exitClean      = 0
	exitViolations = 1
	exitUsage      = 2
)

const helpText = `usage: check_ratchet.py [-h] [--repo-root REPO_ROOT]
                        [--suppressions-file SUPPRESSIONS_FILE] [--update]
                        [--max-judgment-rules MAX_JUDGMENT_RULES]
                        [--findings-json FINDINGS_JSON]
                        [--ruff-json RUFF_JSON]
                        [--golangci-sarif GOLANGCI_SARIF]
                        [--clippy-json CLIPPY_JSON] [--deno-json DENO_JSON]
                        [--biome-json BIOME_JSON] [--project-dir PROJECT_DIR]
                        [--require-findings] [--baselines-file BASELINES_FILE]
                        [--update-fingerprints]

Enforce inline suppression ratchet and block-new rule fingerprint baselines.

options:
  -h, --help            show this help message and exit
  --repo-root REPO_ROOT
                        Repository root directory (default: detected repo
                        root).
  --suppressions-file SUPPRESSIONS_FILE
                        Path to suppressions.toml (default: <repo-
                        root>/scripts/lint/ratchet/suppressions.toml).
  --update              Regenerate suppressions.toml from current repository
                        counts.
  --max-judgment-rules MAX_JUDGMENT_RULES
                        Set [governance].max_judgment_rules when running
                        --update.
  --findings-json FINDINGS_JSON
                        Path to generic/Ruff findings JSON for block-new
                        fingerprint checking.
  --ruff-json RUFF_JSON
                        Path to ` + "`" + `ruff check --output-format json` + "`" + ` report.
  --golangci-sarif GOLANGCI_SARIF
                        Path to ` + "`" + `golangci-lint` + "`" + ` / ` + "`" + `gosec` + "`" + ` SARIF 2.1.0 report.
  --clippy-json CLIPPY_JSON
                        Path to ` + "`" + `cargo clippy --message-format=json` + "`" + ` NDJSON
                        report.
  --deno-json DENO_JSON
                        Path to ` + "`" + `deno lint --json` + "`" + ` report.
  --biome-json BIOME_JSON
                        Path to ` + "`" + `biome lint --reporter=json` + "`" + ` report.
  --project-dir PROJECT_DIR
                        Optional project directory for resolving project-
                        relative finding paths.
  --require-findings    Fail closed (exit 2) if 0 findings were parsed from
                        the report.
  --baselines-file BASELINES_FILE
                        Path to rule_baselines.toml (default: <repo-
                        root>/scripts/lint/ratchet/rule_baselines.toml).
  --update-fingerprints
                        Update rule_baselines.toml from findings report
                        (lowers baseline when violations are removed).
`

func usageOnly() string {
	i := strings.Index(helpText, "\n\n")
	if i < 0 {
		return helpText
	}
	return helpText[:i+1]
}

type optKind int

const (
	kindHelp optKind = iota
	kindBool
	kindPath
	kindInt
)

type optDef struct {
	name string
	kind optKind
	dest string
}

var optionOrder = []optDef{
	{name: "-h", kind: kindHelp, dest: "help"},
	{name: "--help", kind: kindHelp, dest: "help"},
	{name: "--repo-root", kind: kindPath, dest: "repo"},
	{name: "--suppressions-file", kind: kindPath, dest: "suppressions"},
	{name: "--update", kind: kindBool, dest: "update"},
	{name: "--max-judgment-rules", kind: kindInt, dest: "max"},
	{name: "--findings-json", kind: kindPath, dest: "generic"},
	{name: "--ruff-json", kind: kindPath, dest: "ruff"},
	{name: "--golangci-sarif", kind: kindPath, dest: "golangci-sarif"},
	{name: "--clippy-json", kind: kindPath, dest: "clippy-json"},
	{name: "--deno-json", kind: kindPath, dest: "deno-json"},
	{name: "--biome-json", kind: kindPath, dest: "biome-json"},
	{name: "--project-dir", kind: kindPath, dest: "project"},
	{name: "--require-findings", kind: kindBool, dest: "require"},
	{name: "--baselines-file", kind: kindPath, dest: "baselines"},
	{name: "--update-fingerprints", kind: kindBool, dest: "updatefp"},
}

var negNumber = regexp.MustCompile(`^-\d+$|^-\d*\.\d+$`)

type cli struct {
	submode      string
	repo         *string
	suppressions *string
	update       bool
	maxSet       bool
	max          *big.Int
	inputs       map[string]string
	project      *string
	require      bool
	baselines    *string
	updateFP     bool
}

// Run matches python3 scripts/lint/check_ratchet.py.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	c, code, ok := parseCLI(args, stdout, stderr)
	if !ok {
		return code
	}
	repo := defaultRepoRoot(ctx)
	if c.repo != nil {
		repo = *c.repo
	}
	resolvedRepo, err := resolvePath(repo)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return exitUsage
	}
	var fpKind, fpPath string
	nInputs := 0
	for _, kind := range []string{"generic", "ruff", "golangci-sarif", "clippy-json", "deno-json", "biome-json"} {
		if p, ok := c.inputs[kind]; ok {
			nInputs++
			fpKind, fpPath = kind, p
		}
	}
	if nInputs > 1 {
		fmt.Fprintf(stderr, "ERROR: specify at most one findings input flag\n")
		return exitUsage
	}
	if c.submode == "check-fingerprints" || nInputs > 0 || c.updateFP {
		if nInputs == 0 {
			fmt.Fprintf(stderr, "ERROR: one of --findings-json, --ruff-json, --golangci-sarif, --clippy-json, --deno-json, or --biome-json is required for fingerprint mode\n")
			return exitUsage
		}
		in, err := resolvePath(fpPath)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
			return exitUsage
		}
		baselines := filepath.Join(resolvedRepo, "scripts", "lint", "ratchet", "rule_baselines.toml")
		if c.baselines != nil {
			baselines, err = resolvePath(*c.baselines)
			if err != nil {
				fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
				return exitUsage
			}
		}
		var project string
		if c.project != nil {
			project, err = resolvePath(*c.project)
			if err != nil {
				fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
				return exitUsage
			}
		}
		return runFingerprints(ctx, stdout, stderr, resolvedRepo, fpKind, in, baselines, project, c.project != nil, c.updateFP, c.require)
	}
	sup := filepath.Join(resolvedRepo, "scripts", "lint", "ratchet", "suppressions.toml")
	if c.suppressions != nil {
		sup, err = resolvePath(*c.suppressions)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
			return exitUsage
		}
	}
	return runSuppressions(ctx, stdout, stderr, resolvedRepo, sup, c.update, c.maxSet, c.max)
}

func parseCLI(args []string, stdout, stderr io.Writer) (cli, int, bool) {
	c := cli{submode: "ratchet", inputs: map[string]string{}}
	raw := append([]string(nil), args...)
	if len(raw) > 0 && (raw[0] == "check-fingerprints" || raw[0] == "ratchet") {
		c.submode = raw[0]
		raw = raw[1:]
	}
	if msg := ambiguousScan(raw); msg != "" {
		usageErr(stderr, msg)
		return c, exitUsage, false
	}
	extras := []string{}
	for i := 0; i < len(raw); i++ {
		a := raw[i]
		if a == "--" {
			extras = append(extras, raw[i:]...)
			break
		}
		if isPositional(a) {
			extras = append(extras, a)
			continue
		}
		def, explicit, hasExplicit, unknown := resolveOpt(a)
		if unknown {
			extras = append(extras, a)
			continue
		}
		if def.kind == kindHelp {
			if hasExplicit {
				usageErr(stderr, fmt.Sprintf("argument -h/--help: ignored explicit argument %s", pyReprString(explicit)))
				return c, exitUsage, false
			}
			fmt.Fprint(stdout, helpText)
			return c, exitClean, false
		}
		if def.kind == kindBool {
			if hasExplicit {
				usageErr(stderr, fmt.Sprintf("argument %s: ignored explicit argument %s", def.name, pyReprString(explicit)))
				return c, exitUsage, false
			}
			switch def.dest {
			case "update":
				c.update = true
			case "require":
				c.require = true
			case "updatefp":
				c.updateFP = true
			}
			continue
		}
		val := explicit
		if !hasExplicit {
			if i+1 >= len(raw) || !isPositional(raw[i+1]) {
				usageErr(stderr, fmt.Sprintf("argument %s: expected one argument", def.name))
				return c, exitUsage, false
			}
			i++
			val = raw[i]
		}
		if def.kind == kindInt {
			n, ok := parseCLIInt(val)
			if !ok {
				usageErr(stderr, fmt.Sprintf("argument %s: invalid int value: %s", def.name, pyReprString(val)))
				return c, exitUsage, false
			}
			c.maxSet = true
			c.max = n
			continue
		}
		switch def.dest {
		case "repo":
			c.repo = &val
		case "suppressions":
			c.suppressions = &val
		case "project":
			c.project = &val
		case "baselines":
			c.baselines = &val
		default:
			c.inputs[def.dest] = val
		}
	}
	if len(extras) > 0 {
		usageErr(stderr, "unrecognized arguments: "+strings.Join(extras, " "))
		return c, exitUsage, false
	}
	return c, 0, true
}

func usageErr(stderr io.Writer, msg string) {
	fmt.Fprint(stderr, usageOnly())
	fmt.Fprintf(stderr, "check_ratchet.py: error: %s\n", msg)
}

func ambiguousScan(args []string) string {
	for _, a := range args {
		if a == "--" {
			break
		}
		if isPositional(a) {
			continue
		}
		if msg := ambiguousMessage(a); msg != "" {
			return msg
		}
	}
	return ""
}

func ambiguousMessage(arg string) string {
	if strings.HasPrefix(arg, "--") {
		prefix := arg
		if i := strings.IndexByte(arg, '='); i >= 0 {
			prefix = arg[:i]
		}
		if _, ok := exactOpt(prefix); ok {
			return ""
		}
		var names []string
		for _, d := range optionOrder {
			if strings.HasPrefix(d.name, "--") && strings.HasPrefix(d.name, prefix) {
				names = append(names, d.name)
			}
		}
		if len(names) > 1 {
			return "ambiguous option: " + arg + " could match " + strings.Join(names, ", ")
		}
	}
	return ""
}

func exactOpt(name string) (optDef, bool) {
	for _, d := range optionOrder {
		if d.name == name {
			return d, true
		}
	}
	return optDef{}, false
}

func isPositional(arg string) bool {
	if arg == "" || arg == "--" {
		return arg != "--"
	}
	if !strings.HasPrefix(arg, "-") {
		return true
	}
	return negNumber.MatchString(arg)
}

func resolveOpt(arg string) (optDef, string, bool, bool) {
	if d, ok := exactOpt(arg); ok {
		return d, "", false, false
	}
	if name, explicit, ok := strings.Cut(arg, "="); ok {
		if d, found := exactOpt(name); found {
			return d, explicit, true, false
		}
	}
	if strings.HasPrefix(arg, "-h") && !strings.HasPrefix(arg, "--") {
		if strings.HasPrefix(arg, "-h=") {
			return optDef{name: "-h/--help", kind: kindHelp}, arg[3:], true, false
		}
		rest := arg[2:]
		if rest != "" && rest[0] == '-' {
			return optDef{name: "-h/--help", kind: kindHelp}, rest, true, false
		}
		return optDef{name: "-h/--help", kind: kindHelp}, "", false, false
	}
	if strings.HasPrefix(arg, "--") {
		prefix, explicit, has := strings.Cut(arg, "=")
		var match optDef
		n := 0
		for _, d := range optionOrder {
			if strings.HasPrefix(d.name, "--") && strings.HasPrefix(d.name, prefix) {
				match = d
				n++
			}
		}
		if n == 1 {
			return match, explicit, has, false
		}
		return optDef{}, "", false, true
	}
	return optDef{}, "", false, true
}

func parseCLIInt(s string) (*big.Int, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil, false
	}
	sign := 1
	switch t[0] {
	case '+':
		t = t[1:]
	case '-':
		sign = -1
		t = t[1:]
	}
	if t == "" {
		return nil, false
	}
	var b strings.Builder
	prevUS := true
	for _, c := range t {
		if c == '_' {
			if prevUS {
				return nil, false
			}
			prevUS = true
			continue
		}
		if c < '0' || c > '9' {
			return nil, false
		}
		prevUS = false
		b.WriteRune(c)
	}
	if prevUS || b.Len() == 0 {
		return nil, false
	}
	n := new(big.Int)
	if _, ok := n.SetString(b.String(), 10); !ok {
		return nil, false
	}
	if sign < 0 {
		n.Neg(n)
	}
	return n, true
}

func defaultRepoRoot(ctx context.Context) string {
	if bin, err := exec.LookPath("git"); err == nil {
		cmd := exec.CommandContext(ctx, bin, "rev-parse", "--show-toplevel")
		if out, err := cmd.Output(); err == nil {
			s := strings.TrimSpace(string(out))
			if s != "" {
				if r, err := resolvePath(s); err == nil {
					return r
				}
				return s
			}
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	if r, err := resolvePath(cwd); err == nil {
		return r
	}
	return cwd
}

type countDelta struct {
	target   string
	kind     string
	recorded int
	actual   int
}

func runSuppressions(ctx context.Context, stdout, stderr io.Writer, repo, sup string, update, maxSet bool, max *big.Int) int {
	actual, err := collectRepoCounts(ctx, repo)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to collect suppression counts: %s\n", err.Error())
		return exitUsage
	}
	if update {
		gov := map[string]any{}
		if isRegularFile(sup) {
			_, g, err := loadSuppressions(sup)
			if err != nil {
				fmt.Fprintf(stderr, "ERROR: failed to parse existing %s: %s\n", sup, err.Error())
				return exitUsage
			}
			gov = g
		}
		if maxSet {
			gov["max_judgment_rules"] = max
		}
		if err := writeFile(sup, renderSuppressions(actual, gov)); err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to collect suppression counts: %s\n", err.Error())
			return exitUsage
		}
		fmt.Fprintf(stdout, "Updated %s (%d targets, %d total inline suppressions).\n", sup, len(actual), grandTotal(actual))
		return exitClean
	}
	if !isRegularFile(sup) {
		fmt.Fprintf(stderr, "ERROR: ratchet baseline not found: %s (run `./scripts/altharness/alt-harness ratchet --update` to create it)\n", sup)
		return exitUsage
	}
	recorded, _, err := loadSuppressions(sup)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to parse %s: %s\n", sup, err.Error())
		return exitUsage
	}
	inc, dec := compareCounts(actual, recorded)
	for _, d := range inc {
		fmt.Fprintf(stderr, "%s: [ratchet.increase] %s increased from %d to %d (+%d)\n", d.target, d.kind, d.recorded, d.actual, d.actual-d.recorded)
	}
	for _, d := range dec {
		fmt.Fprintf(stderr, "%s: [ratchet.decrease-update-required] %s decreased from %d to %d (-%d); run `./scripts/altharness/alt-harness ratchet --update` in the same commit\n", d.target, d.kind, d.recorded, d.actual, d.recorded-d.actual)
	}
	if len(inc)+len(dec) > 0 {
		fmt.Fprintf(stderr, "\nFAIL: suppression ratchet mismatch (%d increase(s), %d decrease(s)).\n", len(inc), len(dec))
		return exitViolations
	}
	fmt.Fprintf(stdout, "OK: suppression ratchet clean (%d targets, %d total inline suppressions).\n", len(actual), grandTotal(actual))
	return exitClean
}

func grandTotal(m map[string]map[string]int) int {
	n := 0
	for _, kinds := range m {
		for _, c := range kinds {
			n += c
		}
	}
	return n
}

func compareCounts(actual, recorded map[string]map[string]int) (inc, dec []countDelta) {
	seen := map[string]struct{}{}
	for t := range actual {
		seen[t] = struct{}{}
	}
	for t := range recorded {
		seen[t] = struct{}{}
	}
	targets := make([]string, 0, len(seen))
	for t := range seen {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	for _, target := range targets {
		ks := map[string]struct{}{}
		for k := range actual[target] {
			ks[k] = struct{}{}
		}
		for k := range recorded[target] {
			ks[k] = struct{}{}
		}
		kinds := make([]string, 0, len(ks))
		for k := range ks {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			a := actual[target][kind]
			r := recorded[target][kind]
			if a > r {
				inc = append(inc, countDelta{target, kind, r, a})
			} else if a < r {
				dec = append(dec, countDelta{target, kind, r, a})
			}
		}
	}
	return inc, dec
}

type scopeInfo struct {
	all    bool
	prefix string
}

func computeScope(repo, project string, hasProject bool) (scopeInfo, error) {
	if !hasProject {
		return scopeInfo{all: true}, nil
	}
	repoR, err := resolvePath(repo)
	if err != nil {
		return scopeInfo{}, err
	}
	projR, err := resolvePath(project)
	if err != nil {
		return scopeInfo{}, err
	}
	rel, ok := relTo(repoR, projR)
	if !ok {
		return scopeInfo{}, fmt.Errorf("--project-dir escapes repository root: %s", project)
	}
	if rel == "" || rel == "." {
		return scopeInfo{all: true}, nil
	}
	return scopeInfo{prefix: rel}, nil
}

func fileInScope(file string, sc scopeInfo) bool {
	if sc.all {
		return true
	}
	return file == sc.prefix || strings.HasPrefix(file, sc.prefix+"/")
}

func runFingerprints(ctx context.Context, stdout, stderr io.Writer, repo, kind, input, baselines, project string, hasProject, update, require bool) int {
	_ = ctx
	sc, err := computeScope(repo, project, hasProject)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return exitUsage
	}
	findings, code := loadInput(stderr, kind, input, repo, project, hasProject)
	if code != 0 {
		return code
	}
	if require && len(findings) == 0 {
		fmt.Fprintf(stderr, "ERROR: %s report contained 0 parsed findings while linter exited non-zero (fail-closed: refusing to treat unparsed linter failure as clean)\n", kind)
		return exitUsage
	}
	actual := map[fpKey]int{}
	for _, f := range findings {
		actual[fpKey{f.rule, f.relFile, f.lineSHA256}]++
	}
	if update {
		merged := map[fpKey]int{}
		if !sc.all && isRegularFile(baselines) {
			existing, err := loadBaselines(baselines)
			if err != nil {
				fmt.Fprintf(stderr, "ERROR: failed to load baselines TOML: %s\n", err.Error())
				return exitUsage
			}
			for k, n := range existing {
				if !fileInScope(k.file, sc) {
					merged[k] = n
				}
			}
			for k, n := range actual {
				merged[k] += n
			}
		} else {
			merged = actual
		}
		if err := writeFile(baselines, renderBaselines(merged)); err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to load baselines TOML: %s\n", err.Error())
			return exitUsage
		}
		total := 0
		for _, n := range merged {
			total += n
		}
		fmt.Fprintf(stdout, "Updated %s (%d unique fingerprints, %d total findings).\n", baselines, len(merged), total)
		return exitClean
	}
	recorded, err := loadBaselines(baselines)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to load baselines TOML: %s\n", err.Error())
		return exitUsage
	}
	seen := map[fpKey]int{}
	var fresh []findingItem
	for _, item := range findings {
		key := fpKey{item.rule, item.relFile, item.lineSHA256}
		seen[key]++
		if seen[key] > recorded[key] {
			fresh = append(fresh, item)
		}
	}
	if len(fresh) > 0 {
		for _, item := range fresh {
			short := item.lineSHA256
			if len(short) > 12 {
				short = short[:12]
			}
			fmt.Fprintf(stderr, "%s:%d: [ratchet.new-fingerprint] new %s violation (sha256=%s)\n", item.relFile, item.line, item.rule, short)
		}
		fmt.Fprintf(stderr, "\nFAIL: %d new block-new violation(s) not in %s.\n", len(fresh), baselines)
		return exitViolations
	}
	removed := 0
	for key, recN := range recorded {
		if !fileInScope(key.file, sc) {
			continue
		}
		act := actual[key]
		if recN > act {
			removed += recN - act
		}
	}
	if removed > 0 {
		fmt.Fprintf(stdout, "OK: 0 new block-new violations (%d baselined violation(s) resolved; run --update-fingerprints to lower the baseline).\n", removed)
	} else {
		fmt.Fprintf(stdout, "OK: 0 new block-new violations (%d baselined finding(s) matched).\n", len(findings))
	}
	return exitClean
}

func loadInput(stderr io.Writer, kind, path, repo, project string, hasProject bool) ([]findingItem, int) {
	if !isRegularFile(path) {
		fmt.Fprintf(stderr, "ERROR: findings report not found: %s\n", path)
		return nil, exitUsage
	}
	text, err := loadFindingsFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to load %s findings: %s\n", kind, err.Error())
		return nil, exitUsage
	}
	proj := ""
	if hasProject {
		proj = project
	}
	var findings []findingItem
	switch kind {
	case "generic", "ruff":
		findings, err = loadFindingsJSON(text, repo, proj)
	case "golangci-sarif":
		findings, err = loadGolangciSARIF(text, repo, proj)
	case "clippy-json":
		findings, err = loadClippyJSON(text, repo, proj)
	case "deno-json":
		findings, err = loadDenoJSON(text, repo, proj)
	case "biome-json":
		findings, err = loadBiomeJSON(text, repo, proj)
	default:
		err = fmt.Errorf("unknown findings kind %s", kind)
	}
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to load %s findings: %s\n", kind, err.Error())
		return nil, exitUsage
	}
	return findings, 0
}
