package verifyclaims

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ociDigestRE = regexp.MustCompile(`^sha256:([0-9a-f]{64})$`)

type plectoWITOptions struct {
	repoRoot    string
	baseCommit  string
	artifactDir string
	beforeWasm  string
	afterWasm   string
	evidenceDir string
	format      string
}

// ShippedArtifactEvidence captures OCI WASM blob inspection if present.
type ShippedArtifactEvidence struct {
	ArtifactDir   string `json:"artifact_dir"`
	ManifestHash  string `json:"manifest_sha256"`
	WasmBlobHash  string `json:"wasm_blob_sha256"`
	WasmBlobBytes int64  `json:"wasm_blob_bytes"`
	WITSHA256     string `json:"wit_sha256"`
	WITEqualHead  bool   `json:"wit_equal_head"`
}

// PlectoWITResult is the structured VERIFY-06 evidence record for plecto ABI equivalence.
type PlectoWITResult struct {
	Valid                   bool                     `json:"valid"`
	Mode                    string                   `json:"mode"`
	ReproCommand            string                   `json:"repro_command"`
	BaseCommit              string                   `json:"base_commit,omitempty"`
	HeadCommit              string                   `json:"head_commit,omitempty"`
	WasmToolsVersion        string                   `json:"wasm_tools_version"`
	RustcVersion            string                   `json:"rustc_version,omitempty"`
	BaseCoreWasmSHA256      string                   `json:"base_core_wasm_sha256,omitempty"`
	HeadCoreWasmSHA256      string                   `json:"head_core_wasm_sha256,omitempty"`
	BaseCoreWasmBytes       int64                    `json:"base_core_wasm_bytes,omitempty"`
	HeadCoreWasmBytes       int64                    `json:"head_core_wasm_bytes,omitempty"`
	BaseComponentWasmSHA256 string                   `json:"base_component_wasm_sha256"`
	HeadComponentWasmSHA256 string                   `json:"head_component_wasm_sha256"`
	BaseComponentWasmBytes  int64                    `json:"base_component_wasm_bytes"`
	HeadComponentWasmBytes  int64                    `json:"head_component_wasm_bytes"`
	BaseWITSHA256           string                   `json:"base_wit_sha256"`
	HeadWITSHA256           string                   `json:"head_wit_sha256"`
	WITEqual                bool                     `json:"wit_equal"`
	WasmBytesEqual          bool                     `json:"wasm_bytes_equal"`
	WorldWITValid           bool                     `json:"world_wit_valid,omitempty"`
	ShippedArtifact         *ShippedArtifactEvidence `json:"shipped_artifact,omitempty"`
	EvidenceDir             string                   `json:"evidence_dir,omitempty"`
	ExitCode                int                      `json:"exit_code"`
	Errors                  []Issue                  `json:"errors"`
}

type ociIndex struct {
	Manifests []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
	} `json:"manifests"`
}

type ociManifest struct {
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

func runPlectoWIT(ctx context.Context, opts plectoWITOptions, stdout, stderr io.Writer) int {
	wasmToolsBin, err := exec.LookPath("wasm-tools")
	if err != nil {
		fmt.Fprintln(stderr, "error: wasm-tools not found in PATH")
		return exitInvalid
	}
	verOut, _, verCode, verErr := runExec(ctx, "", nil, 15*time.Second, wasmToolsBin, "--version")
	if verErr != nil || verCode != 0 {
		fmt.Fprintf(stderr, "error: failed to run wasm-tools --version: %v\n", verErr)
		return exitInvalid
	}
	wasmToolsVer := strings.TrimSpace(string(verOut))

	var evDir string
	if opts.evidenceDir != "" {
		var evErr error
		evDir, evErr = ensureSafeEvidenceDir(opts.evidenceDir)
		if evErr != nil {
			fmt.Fprintf(stderr, "error: %v\n", evErr)
			return exitUsage
		}
	}

	var logBuf bytes.Buffer
	res := PlectoWITResult{
		WasmToolsVersion: wasmToolsVer,
		EvidenceDir:      evDir,
		Errors:           []Issue{},
	}

	var baseWIT, headWIT string
	if opts.beforeWasm != "" && opts.afterWasm != "" {
		res.Mode = "direct-wasm"
		res.ReproCommand = fmt.Sprintf("./scripts/altharness/alt-harness verify-claims plecto-wit --before-wasm %s --after-wasm %s", opts.beforeWasm, opts.afterWasm)
		bHash, bSize, err := sha256File(opts.beforeWasm)
		if err != nil {
			fmt.Fprintf(stderr, "error: read --before-wasm: %v\n", err)
			return exitUsage
		}
		aHash, aSize, err := sha256File(opts.afterWasm)
		if err != nil {
			fmt.Fprintf(stderr, "error: read --after-wasm: %v\n", err)
			return exitUsage
		}
		res.BaseComponentWasmSHA256 = bHash
		res.BaseComponentWasmBytes = bSize
		res.HeadComponentWasmSHA256 = aHash
		res.HeadComponentWasmBytes = aSize
		res.WasmBytesEqual = bHash == aHash

		var witErr error
		baseWIT, witErr = extractComponentWIT(ctx, wasmToolsBin, opts.beforeWasm, &logBuf)
		if witErr != nil {
			res.Errors = append(res.Errors, Issue{Code: "WASM_TOOLS_ERROR", Path: opts.beforeWasm, Message: witErr.Error()})
			return finalizePlectoWIT(res, baseWIT, headWIT, "", logBuf.String(), stdout, stderr, opts.format)
		}
		headWIT, witErr = extractComponentWIT(ctx, wasmToolsBin, opts.afterWasm, &logBuf)
		if witErr != nil {
			res.Errors = append(res.Errors, Issue{Code: "WASM_TOOLS_ERROR", Path: opts.afterWasm, Message: witErr.Error()})
			return finalizePlectoWIT(res, baseWIT, headWIT, "", logBuf.String(), stdout, stderr, opts.format)
		}
		return finishPlectoComparison(res, baseWIT, headWIT, "", logBuf.String(), stdout, stderr, opts.format)
	}

	res.Mode = "repo-revision-build"
	res.ReproCommand = fmt.Sprintf("./scripts/altharness/alt-harness verify-claims plecto-wit --base-commit %s", opts.baseCommit)

	gitBin, err := exec.LookPath("git")
	if err != nil {
		fmt.Fprintln(stderr, "error: git not found in PATH")
		return exitInvalid
	}
	cargoBin, err := exec.LookPath("cargo")
	if err != nil {
		fmt.Fprintln(stderr, "error: cargo not found in PATH")
		return exitInvalid
	}

	baseSHA, err := gitRevParse(ctx, gitBin, opts.repoRoot, opts.baseCommit)
	if err != nil {
		res.Errors = append(res.Errors, Issue{Code: "INVALID_BASE_COMMIT", Path: "$.base_commit", Message: err.Error()})
		return finalizePlectoWIT(res, "", "", "", logBuf.String(), stdout, stderr, opts.format)
	}
	headSHA, err := gitRevParse(ctx, gitBin, opts.repoRoot, "HEAD")
	if err != nil {
		res.Errors = append(res.Errors, Issue{Code: "INVALID_HEAD_COMMIT", Path: "HEAD", Message: err.Error()})
		return finalizePlectoWIT(res, "", "", "", logBuf.String(), stdout, stderr, opts.format)
	}
	res.BaseCommit = baseSHA
	res.HeadCommit = headSHA

	plectoDir := filepath.Join(opts.repoRoot, "plecto")
	if st, err := os.Stat(filepath.Join(plectoDir, "Cargo.toml")); err != nil || st.IsDir() {
		res.Errors = append(res.Errors, Issue{Code: "MISSING_PLECTO_CRATE", Path: "plecto/Cargo.toml", Message: "plecto/Cargo.toml not found"})
		return finalizePlectoWIT(res, "", "", "", logBuf.String(), stdout, stderr, opts.format)
	}

	// Verify plecto/wit parses cleanly with wasm-tools.
	witDir := filepath.Join(plectoDir, "wit")
	if witOut, witErrOut, witCode, witErr := runExec(ctx, opts.repoRoot, nil, 30*time.Second, wasmToolsBin, "component", "wit", witDir); witErr == nil && witCode == 0 && len(witOut) > 0 {
		res.WorldWITValid = true
	} else {
		res.Errors = append(res.Errors, Issue{
			Code:    "INVALID_WORLD_WIT",
			Path:    "plecto/wit/world.wit",
			Message: fmt.Sprintf("wasm-tools component wit failed (rc=%d): %s", witCode, strings.TrimSpace(string(witErrOut))),
		})
	}

	if rustcOut, _, rc, rErr := runExec(ctx, plectoDir, nil, 15*time.Second, "rustc", "--version"); rErr == nil && rc == 0 {
		res.RustcVersion = strings.TrimSpace(string(rustcOut))
	}

	tmpDir, err := os.MkdirTemp("", "alt-plecto-wit-")
	if err != nil {
		fmt.Fprintf(stderr, "error: create temp dir: %v\n", err)
		return exitInvalid
	}
	defer os.RemoveAll(tmpDir)

	baseCrateDir := filepath.Join(tmpDir, "base", "plecto")
	headCrateDir := filepath.Join(tmpDir, "head", "plecto")
	sharedTargetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(sharedTargetDir, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: mkdir target: %v\n", err)
		return exitInvalid
	}

	if err := exportGitSubdir(ctx, gitBin, opts.repoRoot, baseSHA, "plecto", filepath.Join(tmpDir, "base")); err != nil {
		res.Errors = append(res.Errors, Issue{Code: "GIT_EXPORT_ERROR", Path: "plecto", Message: err.Error()})
		return finalizePlectoWIT(res, "", "", "", logBuf.String(), stdout, stderr, opts.format)
	}
	if err := copyPlectoWorkingTree(plectoDir, headCrateDir); err != nil {
		res.Errors = append(res.Errors, Issue{Code: "COPY_ERROR", Path: "plecto", Message: err.Error()})
		return finalizePlectoWIT(res, "", "", "", logBuf.String(), stdout, stderr, opts.format)
	}

	baseCore, baseComp, err := buildPlectoComponent(ctx, cargoBin, wasmToolsBin, baseCrateDir, sharedTargetDir, filepath.Join(tmpDir, "base_out"), &logBuf)
	if err != nil {
		res.Errors = append(res.Errors, Issue{Code: "PLECTO_BUILD_ERROR", Path: "base:" + baseSHA, Message: err.Error()})
		return finalizePlectoWIT(res, "", "", "", logBuf.String(), stdout, stderr, opts.format)
	}
	headCore, headComp, err := buildPlectoComponent(ctx, cargoBin, wasmToolsBin, headCrateDir, sharedTargetDir, filepath.Join(tmpDir, "head_out"), &logBuf)
	if err != nil {
		res.Errors = append(res.Errors, Issue{Code: "PLECTO_BUILD_ERROR", Path: "head:" + headSHA, Message: err.Error()})
		return finalizePlectoWIT(res, "", "", "", logBuf.String(), stdout, stderr, opts.format)
	}

	res.BaseCoreWasmSHA256, res.BaseCoreWasmBytes, _ = sha256File(baseCore)
	res.HeadCoreWasmSHA256, res.HeadCoreWasmBytes, _ = sha256File(headCore)
	res.BaseComponentWasmSHA256, res.BaseComponentWasmBytes, _ = sha256File(baseComp)
	res.HeadComponentWasmSHA256, res.HeadComponentWasmBytes, _ = sha256File(headComp)
	res.WasmBytesEqual = res.BaseComponentWasmSHA256 == res.HeadComponentWasmSHA256

	baseWIT, err = extractComponentWIT(ctx, wasmToolsBin, baseComp, &logBuf)
	if err != nil {
		res.Errors = append(res.Errors, Issue{Code: "WASM_TOOLS_ERROR", Path: "base.wasm", Message: err.Error()})
		return finalizePlectoWIT(res, baseWIT, "", "", logBuf.String(), stdout, stderr, opts.format)
	}
	headWIT, err = extractComponentWIT(ctx, wasmToolsBin, headComp, &logBuf)
	if err != nil {
		res.Errors = append(res.Errors, Issue{Code: "WASM_TOOLS_ERROR", Path: "head.wasm", Message: err.Error()})
		return finalizePlectoWIT(res, baseWIT, headWIT, "", logBuf.String(), stdout, stderr, opts.format)
	}

	// Optional OCI shipped artifact inspection if present at plecto/artifacts/stale-chunk-heal or --artifact-dir.
	artifactDir := opts.artifactDir
	if artifactDir == "" {
		candidate := filepath.Join(plectoDir, "artifacts", "stale-chunk-heal")
		if st, err := os.Stat(filepath.Join(candidate, "index.json")); err == nil && !st.IsDir() {
			artifactDir = candidate
		}
	}
	shippedWIT := ""
	if artifactDir != "" {
		shippedEv, sWIT, sErr := inspectShippedOCIArtifact(ctx, wasmToolsBin, artifactDir, headWIT, &logBuf)
		if sErr != nil {
			res.Errors = append(res.Errors, Issue{Code: "SHIPPED_ARTIFACT_ERROR", Path: artifactDir, Message: sErr.Error()})
		} else {
			res.ShippedArtifact = shippedEv
			shippedWIT = sWIT
			if !shippedEv.WITEqualHead {
				res.Errors = append(res.Errors, Issue{
					Code:    "PLECTO_SHIPPED_WIT_MISMATCH",
					Path:    artifactDir,
					Message: "shipped OCI WASM artifact WIT differs from HEAD component WIT",
				})
			}
		}
	}

	return finishPlectoComparison(res, baseWIT, headWIT, shippedWIT, logBuf.String(), stdout, stderr, opts.format)
}

func finishPlectoComparison(res PlectoWITResult, baseWIT, headWIT, shippedWIT, logText string, stdout, stderr io.Writer, format string) int {
	res.BaseWITSHA256 = sha256Hex([]byte(baseWIT))
	res.HeadWITSHA256 = sha256Hex([]byte(headWIT))
	res.WITEqual = baseWIT == headWIT
	if !res.WITEqual {
		res.Errors = append(res.Errors, Issue{
			Code:    "PLECTO_WIT_MISMATCH",
			Path:    "wit",
			Message: fmt.Sprintf("before/after component WIT mismatch (base=%s head=%s)", res.BaseWITSHA256[:12], res.HeadWITSHA256[:12]),
		})
	}
	return finalizePlectoWIT(res, baseWIT, headWIT, shippedWIT, logText, stdout, stderr, format)
}

func finalizePlectoWIT(res PlectoWITResult, baseWIT, headWIT, shippedWIT, logText string, stdout, stderr io.Writer, format string) int {
	res.Valid = len(res.Errors) == 0 && res.WITEqual
	if res.Valid {
		res.ExitCode = exitOK
	} else {
		res.ExitCode = exitInvalid
	}

	if res.EvidenceDir != "" {
		_ = writeEvidenceFile(res.EvidenceDir, "base.wit", []byte(baseWIT))
		_ = writeEvidenceFile(res.EvidenceDir, "head.wit", []byte(headWIT))
		_ = writeEvidenceFile(res.EvidenceDir, "wit.diff", []byte(simpleLineDiff(baseWIT, headWIT)))
		_ = writeEvidenceFile(res.EvidenceDir, "command.log", []byte(logText))
		if shippedWIT != "" {
			_ = writeEvidenceFile(res.EvidenceDir, "shipped.wit", []byte(shippedWIT))
		}
		if raw, err := writeJSONBytes(res); err == nil {
			_ = writeEvidenceFile(res.EvidenceDir, "summary.json", raw)
		}
	}

	if format == "json" {
		raw, err := writeJSONBytes(res)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitUsage
		}
		_, _ = stdout.Write(raw)
		return res.ExitCode
	}
	if res.Valid {
		fmt.Fprintf(stdout, "OK: plecto-wit verified (wit_equal=%v, wasm_bytes_equal=%v, base_wit=%s, head_wit=%s, base_comp=%s, head_comp=%s)\n",
			res.WITEqual, res.WasmBytesEqual, shortHash(res.BaseWITSHA256), shortHash(res.HeadWITSHA256),
			shortHash(res.BaseComponentWasmSHA256), shortHash(res.HeadComponentWasmSHA256))
		return exitOK
	}
	for _, iss := range res.Errors {
		fmt.Fprintf(stderr, "ERROR [%s] %s: %s\n", iss.Code, iss.Path, iss.Message)
	}
	return exitInvalid
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func extractComponentWIT(ctx context.Context, wasmToolsBin, wasmPath string, logBuf *bytes.Buffer) (string, error) {
	if strings.HasPrefix(strings.TrimSpace(wasmPath), "-") {
		return "", fmt.Errorf("unsafe wasm path starting with '-': %q", wasmPath)
	}
	fmt.Fprintf(logBuf, "$ wasm-tools component wit -- %s\n", wasmPath)
	out, errOut, code, err := runExec(ctx, "", nil, 30*time.Second, wasmToolsBin, "component", "wit", "--", wasmPath)
	if err != nil {
		return "", fmt.Errorf("exec wasm-tools: %w", err)
	}
	if code != 0 {
		fmt.Fprintf(logBuf, "stderr:\n%s\n", string(errOut))
		return "", fmt.Errorf("wasm-tools component wit exited %d: %s", code, strings.TrimSpace(string(errOut)))
	}
	return string(out), nil
}

func gitRevParse(ctx context.Context, gitBin, repoRoot, rev string) (string, error) {
	out, errOut, code, err := runExec(ctx, repoRoot, nil, 15*time.Second, gitBin, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("unable to resolve revision %q: %s", rev, strings.TrimSpace(string(errOut)))
	}
	return strings.TrimSpace(string(out)), nil
}

func exportGitSubdir(ctx context.Context, gitBin, repoRoot, rev, subdir, destRoot string) error {
	out, errOut, code, err := runExec(ctx, repoRoot, nil, 30*time.Second, gitBin, "ls-tree", "-r", "--name-only", "-z", rev, "--", subdir)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("git ls-tree %s:%s failed: %s", rev, subdir, strings.TrimSpace(string(errOut)))
	}
	for _, entry := range bytes.Split(out, []byte{0}) {
		rel := strings.TrimSpace(string(entry))
		if rel == "" {
			continue
		}
		if strings.HasPrefix(rel, "/") || strings.Contains(rel, "..") || strings.Contains(rel, `\`) {
			return fmt.Errorf("unsafe git tree path: %q", rel)
		}
		blob, bErr, bCode, err := runExec(ctx, repoRoot, nil, 30*time.Second, gitBin, "show", "--end-of-options", rev+":"+rel)
		if err != nil {
			return err
		}
		if bCode != 0 {
			return fmt.Errorf("git show %s:%s failed: %s", rev, rel, strings.TrimSpace(string(bErr)))
		}
		dest := filepath.Join(destRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), fsperm.Dir); err != nil {
			return err
		}
		if err := os.WriteFile(dest, blob, fsperm.File); err != nil {
			return err
		}
	}
	return nil
}

func copyPlectoWorkingTree(srcDir, destDir string) error {
	skipDirs := map[string]bool{"target": true, "artifacts": true, ".plecto": true, ".git": true}
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(destDir, fsperm.Dir)
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(destDir, rel), fsperm.Dir)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destDir, rel), raw, fsperm.File)
	})
}

func buildPlectoComponent(ctx context.Context, cargoBin, wasmToolsBin, crateDir, targetDir, outDir string, logBuf *bytes.Buffer) (string, string, error) {
	if err := os.MkdirAll(outDir, fsperm.Dir); err != nil {
		return "", "", err
	}
	manifest := filepath.Join(crateDir, "Cargo.toml")
	fmt.Fprintf(logBuf, "$ CARGO_TARGET_DIR=%s cargo --offline build --manifest-path %s --target wasm32-unknown-unknown --release\n", targetDir, manifest)
	out, errOut, code, err := runExec(
		ctx,
		crateDir,
		[]string{"CARGO_TARGET_DIR=" + targetDir},
		defaultCommandTimeout,
		cargoBin, "--offline", "build", "--manifest-path", manifest, "--target", "wasm32-unknown-unknown", "--release",
	)
	if len(out) > 0 {
		logBuf.Write(out)
	}
	if len(errOut) > 0 {
		logBuf.Write(errOut)
	}
	if err != nil {
		return "", "", fmt.Errorf("cargo build exec error: %w", err)
	}
	if code != 0 {
		return "", "", fmt.Errorf("cargo build failed (rc=%d): %s", code, tailString(string(errOut), 400))
	}

	builtCore := filepath.Join(targetDir, "wasm32-unknown-unknown", "release", "alt_stale_chunk_heal_filter.wasm")
	rawCore, err := os.ReadFile(builtCore)
	if err != nil {
		return "", "", fmt.Errorf("read built core wasm: %w", err)
	}
	coreCopy := filepath.Join(outDir, "core.wasm")
	if err := os.WriteFile(coreCopy, rawCore, fsperm.File); err != nil {
		return "", "", err
	}

	compPath := filepath.Join(outDir, "component.wasm")
	fmt.Fprintf(logBuf, "$ wasm-tools component new %s -o %s\n", coreCopy, compPath)
	cOut, cErrOut, cCode, cErr := runExec(ctx, "", nil, 60*time.Second, wasmToolsBin, "component", "new", coreCopy, "-o", compPath)
	if len(cOut) > 0 {
		logBuf.Write(cOut)
	}
	if len(cErrOut) > 0 {
		logBuf.Write(cErrOut)
	}
	if cErr != nil {
		return "", "", fmt.Errorf("wasm-tools component new exec error: %w", cErr)
	}
	if cCode != 0 {
		return "", "", fmt.Errorf("wasm-tools component new failed (rc=%d): %s", cCode, strings.TrimSpace(string(cErrOut)))
	}
	return coreCopy, compPath, nil
}

func inspectShippedOCIArtifact(ctx context.Context, wasmToolsBin, artifactDir, headWIT string, logBuf *bytes.Buffer) (*ShippedArtifactEvidence, string, error) {
	indexPath := filepath.Join(artifactDir, "index.json")
	if isSymlink(indexPath) {
		return nil, "", fmt.Errorf("refusing symlink index.json in %s", artifactDir)
	}
	rawIdx, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, "", fmt.Errorf("read index.json: %w", err)
	}
	var idx ociIndex
	if err := json.Unmarshal(rawIdx, &idx); err != nil {
		return nil, "", fmt.Errorf("parse index.json: %w", err)
	}
	if len(idx.Manifests) == 0 {
		return nil, "", fmt.Errorf("index.json has no manifests")
	}
	mMatch := ociDigestRE.FindStringSubmatch(idx.Manifests[0].Digest)
	if mMatch == nil {
		return nil, "", fmt.Errorf("invalid OCI manifest digest: %q", idx.Manifests[0].Digest)
	}
	manifestHex := mMatch[1]
	manifestBlob := filepath.Join(artifactDir, "blobs", "sha256", manifestHex)
	if isSymlink(manifestBlob) {
		return nil, "", fmt.Errorf("refusing symlink manifest blob: %s", manifestBlob)
	}
	rawManifest, err := os.ReadFile(manifestBlob)
	if err != nil {
		return nil, "", fmt.Errorf("read manifest blob: %w", err)
	}
	if actual := sha256Hex(rawManifest); actual != manifestHex {
		return nil, "", fmt.Errorf("manifest digest mismatch: expected %s got %s", manifestHex, actual)
	}
	var man ociManifest
	if err := json.Unmarshal(rawManifest, &man); err != nil {
		return nil, "", fmt.Errorf("parse manifest blob: %w", err)
	}
	wasmHex := ""
	for _, layer := range man.Layers {
		if layer.MediaType == "application/wasm" {
			lMatch := ociDigestRE.FindStringSubmatch(layer.Digest)
			if lMatch == nil {
				return nil, "", fmt.Errorf("invalid OCI wasm layer digest: %q", layer.Digest)
			}
			wasmHex = lMatch[1]
			break
		}
	}
	if wasmHex == "" {
		return nil, "", fmt.Errorf("no application/wasm layer found in OCI manifest %s", manifestHex)
	}
	wasmBlob := filepath.Join(artifactDir, "blobs", "sha256", wasmHex)
	if isSymlink(wasmBlob) {
		return nil, "", fmt.Errorf("refusing symlink wasm blob: %s", wasmBlob)
	}
	actualWasmHash, wasmBytes, err := sha256File(wasmBlob)
	if err != nil {
		return nil, "", fmt.Errorf("read wasm blob: %w", err)
	}
	if actualWasmHash != wasmHex {
		return nil, "", fmt.Errorf("wasm blob digest mismatch: expected %s got %s", wasmHex, actualWasmHash)
	}
	sWIT, err := extractComponentWIT(ctx, wasmToolsBin, wasmBlob, logBuf)
	if err != nil {
		return nil, "", err
	}
	return &ShippedArtifactEvidence{
		ArtifactDir:   artifactDir,
		ManifestHash:  manifestHex,
		WasmBlobHash:  actualWasmHash,
		WasmBlobBytes: wasmBytes,
		WITSHA256:     sha256Hex([]byte(sWIT)),
		WITEqualHead:  sWIT == headWIT,
	}, sWIT, nil
}

func simpleLineDiff(a, b string) string {
	if a == b {
		return ""
	}
	al := normalizeLines(a)
	bl := normalizeLines(b)
	var out strings.Builder
	maxLen := len(al)
	if len(bl) > maxLen {
		maxLen = len(bl)
	}
	for i := 0; i < maxLen; i++ {
		var av, bv string
		hasA := i < len(al)
		hasB := i < len(bl)
		if hasA {
			av = al[i]
		}
		if hasB {
			bv = bl[i]
		}
		if hasA && hasB && av == bv {
			continue
		}
		if hasA {
			fmt.Fprintf(&out, "-L%d: %s\n", i+1, av)
		}
		if hasB {
			fmt.Fprintf(&out, "+L%d: %s\n", i+1, bv)
		}
	}
	return out.String()
}

func tailString(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[len(s)-limit:]
}
