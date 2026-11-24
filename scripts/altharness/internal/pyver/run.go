package pyver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alt/scripts/altharness/internal/tomlx"
)

const reexecEnv = "CHECK_PYTHON_VERSIONS_REEXEC"

const usageText = `usage: check_python_versions.py [-h] [--repo-root REPO_ROOT] [--project-dir PROJECT_DIR] [--pyproject PYPROJECT] [--source SOURCE]

Validate single-source Python target version config and Dockerfile parity.
`

// Run is the CLI for scripts/lint/check_python_versions.py.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	opts, errText, code := parseArgs(args)
	if errText != "" {
		fmt.Fprint(stderr, errText)
		return code
	}
	if opts.help {
		fmt.Fprint(stdout, usageText)
		return exitClean
	}
	repo := opts.repoRoot
	if repo == "" {
		repo = defaultRoot(ctx)
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitUsage
	}
	repo = absRepo

	required := highestRequired(opts, repo)
	rt, stop := selectRuntime(ctx, required, stderr)
	if stop != 0 {
		return stop
	}

	if opts.pyproject != "" && opts.projectDir != "" {
		fmt.Fprint(stderr, "error: --pyproject and --project-dir are mutually exclusive\n")
		return exitUsage
	}
	if len(opts.sources) > 0 && opts.pyproject == "" {
		fmt.Fprint(stderr, "error: --source requires --pyproject\n")
		return exitUsage
	}

	violations, usage, err := collect(ctx, opts, repo, rt)
	if usage != "" {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitViolations
	}
	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Fprintln(stderr, v.String())
		}
		fmt.Fprintf(stderr, "\nFound %d Python version configuration violation(s).\n", len(violations))
		return exitViolations
	}
	fmt.Fprintln(stdout, "OK: Python version configuration check passed.")
	return exitClean
}

type cliOpts struct {
	help       bool
	repoRoot   string
	projectDir string
	pyproject  string
	sources    []string
}

func parseArgs(args []string) (cliOpts, string, int) {
	var opts cliOpts
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name := arg
		val := ""
		hasVal := false
		if eq := strings.IndexByte(arg, '='); eq > 0 && strings.HasPrefix(arg, "--") {
			name = arg[:eq]
			val = arg[eq+1:]
			hasVal = true
		}
		switch name {
		case "-h", "--help":
			opts.help = true
		case "--repo-root", "--project-dir", "--pyproject", "--source":
			if !hasVal {
				if i+1 >= len(args) {
					return cliOpts{}, fmt.Sprintf("%scheck_python_versions.py: error: argument %s: expected one argument\n", usageText, name), exitUsage
				}
				i++
				val = args[i]
			}
			switch name {
			case "--repo-root":
				opts.repoRoot = val
			case "--project-dir":
				opts.projectDir = val
			case "--pyproject":
				opts.pyproject = val
			case "--source":
				opts.sources = append(opts.sources, val)
			}
		default:
			return cliOpts{}, fmt.Sprintf("%scheck_python_versions.py: error: unrecognized arguments: %s\n", usageText, arg), exitUsage
		}
	}
	return opts, "", 0
}

func defaultRoot(ctx context.Context) string {
	if gitBin, err := exec.LookPath("git"); err == nil {
		cmd := exec.CommandContext(ctx, gitBin, "rev-parse", "--show-toplevel")
		if out, err := cmd.Output(); err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				return s
			}
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func highestRequired(opts cliOpts, repo string) *[2]int {
	if opts.pyproject != "" {
		if len(opts.sources) == 0 {
			return nil
		}
		return extractFloorQuiet(opts.pyproject)
	}
	if opts.projectDir != "" {
		dir := opts.projectDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(repo, dir)
		}
		return extractFloorQuiet(filepath.Join(dir, "pyproject.toml"))
	}
	files, err := discoverRepoPyprojects(context.Background(), repo)
	if err != nil {
		return nil
	}
	var floors [][2]int
	for _, pp := range files {
		if floor := extractFloorQuiet(pp); floor != nil {
			floors = append(floors, *floor)
		}
	}
	return maxFloor(floors)
}

func maxFloor(floors [][2]int) *[2]int {
	if len(floors) == 0 {
		return nil
	}
	best := floors[0]
	for _, f := range floors[1:] {
		if floorLess(best, f) {
			best = f
		}
	}
	return &best
}

func extractFloorQuiet(path string) *[2]int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var data map[string]any
	if err := tomlx.Unmarshal(raw, &data); err != nil {
		return nil
	}
	project, _ := asMap(data["project"])
	if project == nil {
		return nil
	}
	req, ok := project["requires-python"].(string)
	if !ok {
		return nil
	}
	return extractRequiresPythonFloor(req)
}

func selectRuntime(ctx context.Context, required *[2]int, stderr io.Writer) (pyRuntime, int) {
	bin, ver, err := currentPython(ctx)
	rt := pyRuntime{bin: bin, major: ver[0], minor: ver[1]}
	if required == nil {
		return rt, 0
	}
	if err != nil {
		ver = [2]int{0, 0}
		rt = pyRuntime{}
	}
	if !floorLess(ver, *required) && rt.bin != "" {
		return rt, 0
	}
	if os.Getenv(reexecEnv) == "1" {
		fmt.Fprintf(stderr, "error: Python %d.%d is older than required floor %d.%d even after re-exec; cannot verify AST syntax.\n",
			ver[0], ver[1], required[0], required[1])
		return rt, exitViolations
	}
	alt := findPythonForFloor(required[0], required[1])
	if alt == "" {
		shown := ver
		if rt.bin == "" && err != nil {
			shown = ver
		}
		fmt.Fprintf(stderr, "error: Python %d.%d is older than required floor %d.%d and python%d.%d was not found in PATH or ~/.local/bin; refusing to skip AST syntax verification.\n",
			shown[0], shown[1], required[0], required[1], required[0], required[1])
		return rt, exitViolations
	}
	altVer, aerr := pythonVersion(ctx, alt)
	if aerr != nil || floorLess(altVer, *required) {
		use := altVer
		if aerr != nil {
			use = ver
		}
		fmt.Fprintf(stderr, "error: Python %d.%d is older than required floor %d.%d even after re-exec; cannot verify AST syntax.\n",
			use[0], use[1], required[0], required[1])
		return rt, exitViolations
	}
	return pyRuntime{bin: alt, major: altVer[0], minor: altVer[1]}, 0
}

func currentPython(ctx context.Context) (string, [2]int, error) {
	for _, name := range []string{"python3", "python"} {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		ver, err := pythonVersion(ctx, bin)
		if err != nil {
			continue
		}
		return bin, ver, nil
	}
	return "", [2]int{}, fmt.Errorf("python3 was not found")
}

func pythonVersion(ctx context.Context, bin string) ([2]int, error) {
	cmd := exec.CommandContext(ctx, bin, "-c", "import sys; print(sys.version_info[0], sys.version_info[1])")
	out, err := cmd.Output()
	if err != nil {
		return [2]int{}, err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return [2]int{}, fmt.Errorf("bad version from %s", bin)
	}
	return [2]int{atoi(fields[0]), atoi(fields[1])}, nil
}

func findPythonForFloor(major, minor int) string {
	name := fmt.Sprintf("python%d.%d", major, minor)
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	local := filepath.Join(home, ".local", "bin", name)
	st, err := os.Stat(local)
	if err != nil || st.IsDir() {
		return ""
	}
	if st.Mode()&0o111 == 0 {
		return ""
	}
	return local
}

type workPiece struct {
	viols []Violation
	unit  *sourceUnit
}

func collect(ctx context.Context, opts cliOpts, repo string, rt pyRuntime) ([]Violation, string, error) {
	if opts.pyproject != "" {
		return collectExplicit(ctx, opts.pyproject, opts.sources, repo, rt)
	}
	if opts.projectDir != "" {
		dir := opts.projectDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(repo, dir)
		}
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			return nil, fmt.Sprintf("error: project directory not found: %s", dir), nil
		}
		res, err := checkProject(dir, repo, rt)
		if err != nil {
			return nil, "", err
		}
		return finishPieces(ctx, rt, piecesFromProject(res))
	}
	files, err := discoverRepoPyprojects(ctx, repo)
	if err != nil {
		return nil, "", err
	}
	var pieces []workPiece
	for _, pp := range files {
		res, err := checkProject(filepath.Dir(pp), repo, rt)
		if err != nil {
			return nil, "", err
		}
		pieces = append(pieces, piecesFromProject(res)...)
	}
	return finishPieces(ctx, rt, pieces)
}

func piecesFromProject(res projectResult) []workPiece {
	var pieces []workPiece
	if len(res.violations) > 0 {
		pieces = append(pieces, workPiece{viols: res.violations})
	}
	for i := range res.sources {
		unit := res.sources[i]
		pieces = append(pieces, workPiece{unit: &unit})
	}
	return pieces
}

func collectExplicit(ctx context.Context, pyproject string, sources []string, repo string, rt pyRuntime) ([]Violation, string, error) {
	if st, err := os.Stat(pyproject); err != nil || st.IsDir() {
		return nil, fmt.Sprintf("error: pyproject file not found: %s", pyproject), nil
	}
	data, parseViolations := parsePyproject(pyproject, repo)
	if data == nil {
		return finishPieces(ctx, rt, []workPiece{{viols: parseViolations}})
	}
	rel := relDisplay(pyproject, repo)
	var violations []Violation
	checkRuffTargetVersion(data, rel, &violations)
	floor := extractAndValidateFloor(data, rel, &violations)
	pieces := []workPiece{}
	if len(violations) > 0 {
		pieces = append(pieces, workPiece{viols: violations})
	}
	if floor == nil {
		return finishPieces(ctx, rt, pieces)
	}
	for _, src := range sources {
		if st, err := os.Stat(src); err != nil || st.IsDir() {
			return nil, fmt.Sprintf("error: source file not found: %s", src), nil
		}
		display := relDisplay(src, repo)
		text, readErr := readSource(src, display)
		if readErr != nil {
			pieces = append(pieces, workPiece{viols: []Violation{*readErr}})
			continue
		}
		unit := checkSourceText(text, display, *floor, rt)
		pieces = append(pieces, workPiece{unit: &unit})
	}
	return finishPieces(ctx, rt, pieces)
}

func finishPieces(ctx context.Context, rt pyRuntime, pieces []workPiece) ([]Violation, string, error) {
	var reqs []astRequest
	var reqIndex []int
	for i, piece := range pieces {
		if piece.unit != nil && piece.unit.needAST {
			reqs = append(reqs, astRequest{
				Src:  piece.unit.text,
				Name: piece.unit.display,
				Maj:  piece.unit.floor[0],
				Min:  piece.unit.floor[1],
			})
			reqIndex = append(reqIndex, i)
		}
	}
	var responses []astResponse
	if len(reqs) > 0 {
		if rt.bin == "" {
			return nil, "", fmt.Errorf("python interpreter is required to verify AST syntax and was not found")
		}
		var err error
		responses, err = astParseBatch(ctx, rt.bin, reqs)
		if err != nil {
			return nil, "", err
		}
	}
	byPiece := map[int]*astResponse{}
	for n, idx := range reqIndex {
		resp := responses[n]
		byPiece[idx] = &resp
	}
	var out []Violation
	for i, piece := range pieces {
		if piece.unit == nil {
			out = append(out, piece.viols...)
			continue
		}
		out = append(out, finishSource(*piece.unit, byPiece[i], rt)...)
	}
	return out, "", nil
}

func execLook(name string) (string, error) {
	return exec.LookPath(name)
}

func gitOutput(ctx context.Context, gitBin, repo string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, gitBin, append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
