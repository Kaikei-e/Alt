package canary

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type tautoCanaryConfig struct {
	tautoBin string
	workDir  string
}

const (
	tautoFalseManifest = "package {\n  name = \"false-tote\"\n  version = \"0.1.0\"\n}\n"
	tautoFalseSource   = "pub theorem bogus(): Eq(Nat, 0, 1) {\n  Refl\n}\n"

	tautoHoleManifest = "package {\n  name = \"hole-tote\"\n  version = \"0.1.0\"\n}\n"
	tautoHoleSource   = "pub theorem with_hole(): Eq(Nat, 0, 0) {\n  ?h\n}\n"

	tautoAxiomManifest = "package {\n  name = \"axiom-tote\"\n  version = \"0.1.0\"\n}\n"
	tautoAxiomSource   = "pub axiom trust_me(): Eq(Nat, 0, 1)\n"
)

func runTautoCanary(ctx context.Context, st streams, cfg tautoCanaryConfig) int {
	tautoBin := cfg.tautoBin
	if tautoBin == "" {
		if override := strings.TrimSpace(os.Getenv("TAUTO_BIN")); override != "" {
			if found := look(override); found != "" {
				tautoBin = found
			} else {
				st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (tauto): TAUTO_BIN %q not found or not executable", override))
				return exitFail
			}
		}
	}
	if tautoBin == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			localBin := filepath.Join(home, ".local", "bin", "tauto")
			if info, statErr := os.Stat(localBin); statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
				tautoBin = localBin
			}
		}
	}
	if tautoBin == "" {
		tautoBin = look("tauto")
	}
	if tautoBin == "" {
		st.error("ERROR: CANARY AUDIT FAILED (tauto): tauto executable not found in PATH")
		return exitFail
	}
	code := withWorkDir(st, cfg.workDir, "alt-tauto-canary-", "tauto", func(canaryDir string) int {
		if !checkTautoFalseTheorem(ctx, st, tautoBin, canaryDir) {
			return exitFail
		}
		if !checkTautoHole(ctx, st, tautoBin, canaryDir) {
			return exitFail
		}
		if !checkTautoAxiom(ctx, st, tautoBin, canaryDir) {
			return exitFail
		}
		return exitOK
	})
	if code != exitOK {
		return code
	}
	st.info("canary audit passed (tauto): false-theorem (exit 1), hole (check exit 3, lint exit 1), axiom (proof-status axioms=[trust_me])")
	return exitOK
}

func writeTautoCanaryTote(root, name, manifest, src string) (string, error) {
	toteDir := filepath.Join(root, name)
	srcDir := filepath.Join(toteDir, "src")
	if err := os.MkdirAll(srcDir, fsperm.Dir); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(toteDir, "Tauto.eure"), []byte(manifest), fsperm.File); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.taut"), []byte(src), fsperm.File); err != nil {
		return "", err
	}
	return toteDir, nil
}

func checkTautoFalseTheorem(ctx context.Context, st streams, tautoBin, canaryDir string) bool {
	falseDir, err := writeTautoCanaryTote(canaryDir, "false-tote", tautoFalseManifest, tautoFalseSource)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (tauto/false-theorem): %s", err.Error()))
		return false
	}
	res := runCmd(ctx, 0, tautoBin, []string{"--offline", "check", "--summary", "--timeout", "30", falseDir}, canaryDir, nil)
	if res.err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (tauto/false-theorem): execution error: %s", res.err.Error()))
		return false
	}
	if res.code != 1 {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (tauto/false-theorem): expected exit 1, got exit %d (output: %s)",
			res.code, snippet(res.combined()),
		))
		return false
	}
	return true
}

func checkTautoHole(ctx context.Context, st streams, tautoBin, canaryDir string) bool {
	holeDir, err := writeTautoCanaryTote(canaryDir, "hole-tote", tautoHoleManifest, tautoHoleSource)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (tauto/hole): %s", err.Error()))
		return false
	}
	checkRes := runCmd(ctx, 0, tautoBin, []string{"--offline", "check", "--summary", "--timeout", "30", holeDir}, canaryDir, nil)
	if checkRes.err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (tauto/hole-check): execution error: %s", checkRes.err.Error()))
		return false
	}
	if checkRes.code != 3 {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (tauto/hole-check): expected exit 3, got exit %d (output: %s)",
			checkRes.code, snippet(checkRes.combined()),
		))
		return false
	}
	lintRes := runCmd(ctx, 0, tautoBin, []string{"lint", "--json", holeDir}, canaryDir, nil)
	if lintRes.err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (tauto/hole-lint): execution error: %s", lintRes.err.Error()))
		return false
	}
	if lintRes.code != 1 {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (tauto/hole-lint): expected exit 1, got exit %d (output: %s)",
			lintRes.code, snippet(lintRes.combined()),
		))
		return false
	}
	var diags []any
	if err := json.Unmarshal([]byte(strings.TrimSpace(lintRes.stdout)), &diags); err != nil || len(diags) == 0 {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (tauto/hole-lint): expected non-empty JSON diagnostic array, got stdout=%s",
			snippet(lintRes.stdout),
		))
		return false
	}
	return true
}

func checkTautoAxiom(ctx context.Context, st streams, tautoBin, canaryDir string) bool {
	axiomDir, err := writeTautoCanaryTote(canaryDir, "axiom-tote", tautoAxiomManifest, tautoAxiomSource)
	if err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (tauto/axiom): %s", err.Error()))
		return false
	}
	srcDir := filepath.Join(axiomDir, "src")
	res := runCmd(ctx, 0, tautoBin, []string{"--offline", "proof-status", "--json", srcDir}, canaryDir, nil)
	if res.err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (tauto/axiom): execution error: %s", res.err.Error()))
		return false
	}
	if res.code != 0 && res.code != 1 {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (tauto/axiom): expected exit 0 or 1 from proof-status --json, got exit %d (output: %s)",
			res.code, snippet(res.combined()),
		))
		return false
	}
	var payload struct {
		Axioms []string `json:"axioms"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.stdout)), &payload); err != nil {
		st.error(fmt.Sprintf(
			"ERROR: CANARY AUDIT FAILED (tauto/axiom): invalid proof-status JSON: %s (stdout: %s)",
			err.Error(), snippet(res.stdout),
		))
		return false
	}
	for _, ax := range payload.Axioms {
		if ax == "trust_me" {
			return true
		}
	}
	st.error(fmt.Sprintf(
		"ERROR: CANARY AUDIT FAILED (tauto/axiom): proof-status JSON axioms missing 'trust_me' (got %s)",
		pyList(payload.Axioms),
	))
	return false
}
