package verifyclaims

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	toolchainChannelRE = regexp.MustCompile(`(?m)^\s*channel\s*=\s*"([^"]+)"`)
	rustAllowAttrRE    = regexp.MustCompile(`#(!?)\[\s*allow\s*\(([^)]*)\)\s*\]`)
)

type recapClippyOptions struct {
	repoRoot    string
	projectRel  string
	evidenceDir string
	format      string
	cargoBin    string // optional test override
}

// AllowEntry records one #[allow(...)] or #![allow(...)] occurrence.
type AllowEntry struct {
	File  string   `json:"file"`
	Line  int      `json:"line"`
	Kind  string   `json:"kind"` // "inner" (#![allow(...)]) or "outer" (#[allow(...)])
	Rules []string `json:"rules"`
	Raw   string   `json:"raw"`
}

// RecapClippyResult is the structured VERIFY-06 evidence record for recap-worker clippy.
type RecapClippyResult struct {
	Valid               bool           `json:"valid"`
	ReproCommand        string         `json:"repro_command"`
	ProjectDir          string         `json:"project_dir"`
	ToolchainFile       string         `json:"toolchain_file"`
	ToolchainChannel    string         `json:"toolchain_channel"`
	CargoVersion        string         `json:"cargo_version"`
	ClippyVersion       string         `json:"clippy_version"`
	ExitCode            int            `json:"exit_code"`
	WarningCount        int            `json:"warning_count"`
	ErrorCount          int            `json:"error_count"`
	AllowInventoryTotal int            `json:"allow_inventory_total"`
	AllowInnerCount     int            `json:"allow_inner_count"`
	AllowOuterCount     int            `json:"allow_outer_count"`
	AllowByRule         map[string]int `json:"allow_by_rule"`
	StdoutSHA256        string         `json:"stdout_sha256"`
	InventorySHA256     string         `json:"inventory_sha256"`
	EvidenceDir         string         `json:"evidence_dir,omitempty"`
	Errors              []Issue        `json:"errors"`
}

type cargoCompilerMsg struct {
	Reason  string `json:"reason"`
	Message *struct {
		Level    string `json:"level"`
		Message  string `json:"message"`
		Rendered string `json:"rendered"`
	} `json:"message"`
}

func runRecapClippy(ctx context.Context, opts recapClippyOptions, stdout, stderr io.Writer) int {
	projectAbs, unsafeReason, err := resolveSafeRepoPath(opts.repoRoot, opts.projectRel)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR [UNSAFE_PROJECT_DIR] %s: %s\n", opts.projectRel, unsafeReason)
		return exitUsage
	}
	if st, err := os.Stat(filepath.Join(projectAbs, "Cargo.toml")); err != nil || st.IsDir() {
		fmt.Fprintf(stderr, "error: %s is not a Rust project directory (missing Cargo.toml)\n", opts.projectRel)
		return exitUsage
	}
	toolchainPath, ok := findRustToolchain(projectAbs, opts.repoRoot)
	if !ok {
		fmt.Fprintf(stderr, "error: rust-toolchain.toml not found in %s or ancestors up to repo root\n", opts.projectRel)
		return exitInvalid
	}
	tcRaw, err := os.ReadFile(toolchainPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: read %s: %v\n", toolchainPath, err)
		return exitInvalid
	}
	m := toolchainChannelRE.FindStringSubmatch(string(tcRaw))
	if len(m) < 2 {
		fmt.Fprintf(stderr, "error: channel missing in %s\n", toolchainPath)
		return exitInvalid
	}
	channel := m[1]

	cargoBin := opts.cargoBin
	if cargoBin == "" {
		var lookErr error
		cargoBin, lookErr = exec.LookPath("cargo")
		if lookErr != nil {
			fmt.Fprintln(stderr, "error: cargo not found in PATH")
			return exitInvalid
		}
	}

	var evDir string
	if opts.evidenceDir != "" {
		var evErr error
		evDir, evErr = ensureSafeEvidenceDir(opts.evidenceDir)
		if evErr != nil {
			fmt.Fprintf(stderr, "error: %v\n", evErr)
			return exitUsage
		}
	}

	tcRel, _ := filepath.Rel(opts.repoRoot, toolchainPath)
	res := RecapClippyResult{
		ReproCommand:     fmt.Sprintf("(cd %s && cargo +%s clippy --offline --all-targets -- -D warnings)", opts.projectRel, channel),
		ProjectDir:       opts.projectRel,
		ToolchainFile:    filepath.ToSlash(tcRel),
		ToolchainChannel: channel,
		AllowByRule:      map[string]int{},
		EvidenceDir:      evDir,
		Errors:           []Issue{},
	}

	toolchainArg := "+" + channel
	if vOut, _, vc, vErr := runExec(ctx, projectAbs, nil, 15*time.Second, cargoBin, toolchainArg, "--version"); vErr == nil && vc == 0 {
		res.CargoVersion = strings.TrimSpace(string(vOut))
	}
	if cOut, _, cc, cErr := runExec(ctx, projectAbs, nil, 15*time.Second, cargoBin, toolchainArg, "clippy", "--version"); cErr == nil && cc == 0 {
		res.ClippyVersion = strings.TrimSpace(string(cOut))
	}

	entries, invText, invErr := inventoryRustAllows(projectAbs)
	if invErr != nil {
		res.Errors = append(res.Errors, Issue{Code: "ALLOW_INVENTORY_ERROR", Path: opts.projectRel, Message: invErr.Error()})
	} else {
		res.AllowInventoryTotal = len(entries)
		for _, e := range entries {
			if e.Kind == "inner" {
				res.AllowInnerCount++
			} else {
				res.AllowOuterCount++
			}
			for _, rule := range e.Rules {
				res.AllowByRule[rule]++
			}
		}
		res.InventorySHA256 = sha256Hex([]byte(invText))
	}

	clippyOut, clippyErr, exitCode, execErr := runExec(
		ctx,
		projectAbs,
		nil,
		defaultCommandTimeout,
		cargoBin, toolchainArg, "clippy", "--offline", "--all-targets", "--message-format=json", "--", "-D", "warnings",
	)
	if execErr != nil {
		res.ExitCode = -1
		res.Errors = append(res.Errors, Issue{Code: "CLIPPY_EXEC_ERROR", Path: opts.projectRel, Message: execErr.Error()})
	} else {
		res.ExitCode = exitCode
		res.StdoutSHA256 = sha256Hex(clippyOut)
		warnings, errs, rendered := parseClippyJSONMessages(clippyOut)
		res.WarningCount = warnings
		res.ErrorCount = errs
		if exitCode != 0 {
			res.Errors = append(res.Errors, Issue{
				Code:    "CLIPPY_FAILED",
				Path:    opts.projectRel,
				Message: fmt.Sprintf("cargo +%s clippy exited %d (warnings=%d, errors=%d)", channel, exitCode, warnings, errs),
			})
		}
		if warnings > 0 {
			res.Errors = append(res.Errors, Issue{
				Code:    "CLIPPY_WARNINGS_NONZERO",
				Path:    opts.projectRel,
				Message: fmt.Sprintf("cargo clippy emitted %d compiler warning(s), expected 0", warnings),
			})
		}
		if errs > 0 && exitCode == 0 {
			res.Errors = append(res.Errors, Issue{
				Code:    "CLIPPY_ERRORS_NONZERO",
				Path:    opts.projectRel,
				Message: fmt.Sprintf("cargo clippy emitted %d compiler error(s)", errs),
			})
		}
		if evDir != "" {
			var logBuf bytes.Buffer
			fmt.Fprintf(&logBuf, "# command: %s\n", res.ReproCommand)
			fmt.Fprintf(&logBuf, "# exit_code: %d warnings: %d errors: %d\n", exitCode, warnings, errs)
			if len(rendered) > 0 {
				logBuf.WriteString("\n# rendered diagnostics:\n")
				for _, r := range rendered {
					logBuf.WriteString(r)
					if !strings.HasSuffix(r, "\n") {
						logBuf.WriteByte('\n')
					}
				}
			}
			if len(clippyErr) > 0 {
				logBuf.WriteString("\n# stderr:\n")
				logBuf.Write(clippyErr)
			}
			_ = writeEvidenceFile(evDir, "clippy.log", logBuf.Bytes())
		}
	}

	res.Valid = len(res.Errors) == 0 && res.ExitCode == 0 && res.WarningCount == 0 && res.ErrorCount == 0
	if evDir != "" {
		_ = writeEvidenceFile(evDir, "allow_inventory.txt", []byte(invText))
		if raw, err := writeJSONBytes(res); err == nil {
			_ = writeEvidenceFile(evDir, "summary.json", raw)
		}
	}

	if opts.format == "json" {
		raw, err := writeJSONBytes(res)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		if res.Valid {
			return exitOK
		}
		return exitInvalid
	}
	if res.Valid {
		fmt.Fprintf(stdout, "OK: recap-clippy verified (channel=%s, warnings=%d, errors=%d, allows=%d [inner=%d, outer=%d])\n",
			res.ToolchainChannel, res.WarningCount, res.ErrorCount, res.AllowInventoryTotal, res.AllowInnerCount, res.AllowOuterCount)
		return exitOK
	}
	for _, iss := range res.Errors {
		fmt.Fprintf(stderr, "ERROR [%s] %s: %s\n", iss.Code, iss.Path, iss.Message)
	}
	return exitInvalid
}

func findRustToolchain(projectAbs, repoRoot string) (string, bool) {
	dir := filepath.Clean(projectAbs)
	root := filepath.Clean(repoRoot)
	for {
		cand := filepath.Join(dir, "rust-toolchain.toml")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, true
		}
		if dir == root {
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func parseClippyJSONMessages(raw []byte) (warnings, errs int, rendered []string) {
	for _, line := range bytes.Split(raw, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			continue
		}
		var msg cargoCompilerMsg
		if err := json.Unmarshal(trimmed, &msg); err != nil {
			continue
		}
		if msg.Reason != "compiler-message" || msg.Message == nil {
			continue
		}
		switch msg.Message.Level {
		case "warning":
			warnings++
			if msg.Message.Rendered != "" {
				rendered = append(rendered, msg.Message.Rendered)
			}
		case "error", "error: internal compiler error":
			errs++
			if msg.Message.Rendered != "" {
				rendered = append(rendered, msg.Message.Rendered)
			}
		}
	}
	return warnings, errs, rendered
}

func inventoryRustAllows(projectAbs string) ([]AllowEntry, string, error) {
	var entries []AllowEntry
	roots := []string{"src", "tests", "benches"}
	for _, sub := range roots {
		base := filepath.Join(projectAbs, sub)
		st, err := os.Lstat(base)
		if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		err = filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.Type()&os.ModeSymlink != 0 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if d.Name() == "target" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".rs") || !d.Type().IsRegular() {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(projectAbs, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			for idx, line := range normalizeLines(string(raw)) {
				// Ignore pure line comments.
				codePart := line
				if commentIdx := strings.Index(codePart, "//"); commentIdx >= 0 {
					codePart = codePart[:commentIdx]
				}
				matches := rustAllowAttrRE.FindAllStringSubmatch(codePart, -1)
				for _, m := range matches {
					kind := "outer"
					if m[1] == "!" {
						kind = "inner"
					}
					var rules []string
					for _, part := range strings.Split(m[2], ",") {
						rule := strings.TrimSpace(part)
						if rule != "" {
							rules = append(rules, rule)
						}
					}
					entries = append(entries, AllowEntry{
						File:  relSlash,
						Line:  idx + 1,
						Kind:  kind,
						Rules: rules,
						Raw:   strings.TrimSpace(m[0]),
					})
				}
			}
			return nil
		})
		if err != nil {
			return nil, "", err
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].File != entries[j].File {
			return entries[i].File < entries[j].File
		}
		if entries[i].Line != entries[j].Line {
			return entries[i].Line < entries[j].Line
		}
		return entries[i].Raw < entries[j].Raw
	})

	var buf strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&buf, "%s:%d:%s:%s\n", e.File, e.Line, e.Kind, e.Raw)
	}
	return entries, buf.String(), nil
}
