package lang

import (
	"alt/scripts/altharness/internal/fsperm"
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RunObservability is scripts/lint/observability.sh. Tool versions come from
// the compose file. promtool and amtool are cached under
// $XDG_CACHE_HOME/alt-observability-tools and are not skipped when missing.
func RunObservability(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	if _, err := exec.LookPath("python3"); err != nil {
		fmt.Fprintln(stderr, "error: required command not found in PATH: python3")
		return 1
	}
	verOut, verErr, verCode, verRunErr := capture(ctx, root, nil, "python3", "scripts/observability-compose-tool-images.py", "--github-output", "--compose", "compose/observability.yaml")
	if verRunErr != nil || verCode != 0 {
		fmt.Fprintln(stderr, "error: failed to resolve Prometheus/Alertmanager tool versions from compose/observability.yaml")
		if strings.TrimSpace(verErr) != "" {
			fmt.Fprint(stderr, verErr)
		}
		return 1
	}
	prom, am := "", ""
	for _, line := range strings.Split(strings.TrimSpace(verOut), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "prometheus":
			prom = val
		case "alertmanager":
			am = val
		}
	}
	if prom == "" || am == "" {
		fmt.Fprintf(stderr, "error: could not resolve concrete Prometheus and Alertmanager versions from compose/observability.yaml (got prometheus='%s', alertmanager='%s')\n", prom, am)
		return 1
	}
	osName, arch, rc := observPlatform(stderr)
	if rc != 0 {
		return rc
	}
	cache, err := observabilityCache()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	promDir := filepath.Join(cache, "prometheus-"+prom+"-"+osName+"-"+arch)
	amDir := filepath.Join(cache, "alertmanager-"+am+"-"+osName+"-"+arch)
	if rc := ensurePromTool(ctx, stderr, "promtool", "prometheus/prometheus", prom, promDir, osName, arch); rc != 0 {
		return rc
	}
	if rc := ensurePromTool(ctx, stderr, "amtool", "prometheus/alertmanager", am, amDir, osName, arch); rc != 0 {
		return rc
	}
	labels := []string{"compose-tool-images", "observability-validate"}
	codes := []int{
		stepCode(ctx, stdout, stderr, root, nil, "python3", "scripts/observability-compose-tool-images.py", "--compose", "compose/observability.yaml"),
		stepCode(ctx, stdout, stderr, root, []string{"PROMTOOL=" + filepath.Join(promDir, "promtool"), "AMTOOL=" + filepath.Join(amDir, "amtool")}, "bash", "scripts/observability-validate.sh"),
	}
	fail := 0
	for i, label := range labels {
		if codes[i] == 0 {
			fmt.Fprintf(stdout, "PASS: %s\n", label)
			continue
		}
		fmt.Fprintf(stdout, "FAIL: %s\n", label)
		fail = 1
	}
	return fail
}

func observPlatform(stderr io.Writer) (string, string, int) {
	osName := runtime.GOOS
	switch osName {
	case "linux", "darwin":
	default:
		fmt.Fprintf(stderr, "unsupported OS: %s\n", osName)
		return "", "", 1
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		fmt.Fprintf(stderr, "unsupported arch: %s\n", runtime.GOARCH)
		return "", "", 1
	}
	return osName, arch, 0
}

func ensurePromTool(ctx context.Context, stderr io.Writer, tool, repo, version, dest, osName, arch string) int {
	bin := filepath.Join(dest, tool)
	if st, err := os.Stat(bin); err == nil && st.Mode()&0o111 != 0 {
		return 0
	}
	if _, err := exec.LookPath("curl"); err != nil {
		fmt.Fprintln(stderr, "error: required command not found in PATH: curl")
		return 1
	}
	tarball := "alertmanager-" + version + "." + osName + "-" + arch + ".tar.gz"
	if tool == "promtool" {
		tarball = "prometheus-" + version + "." + osName + "-" + arch + ".tar.gz"
	}
	baseURL := "https://github.com/" + repo + "/releases/download/v" + version
	tmp, err := os.MkdirTemp("", "alt-observability-")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, tarball)
	sums := filepath.Join(tmp, "sha256sums.txt")
	if code, err := run(ctx, io.Discard, stderr, tmp, nil, "curl", "-fsSL", "--proto", "=https", "--tlsv1.2", "--retry", "3", "-o", archive, baseURL+"/"+tarball); err != nil || code != 0 {
		fmt.Fprintf(stderr, "error: failed to download %s\n", baseURL+"/"+tarball)
		return 1
	}
	if code, err := run(ctx, io.Discard, stderr, tmp, nil, "curl", "-fsSL", "--proto", "=https", "--tlsv1.2", "--retry", "3", "-o", sums, baseURL+"/sha256sums.txt"); err != nil || code != 0 {
		fmt.Fprintf(stderr, "error: failed to download %s\n", baseURL+"/sha256sums.txt")
		return 1
	}
	if err := verifySHA256File(sums, archive, tarball); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	if err := os.MkdirAll(dest, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	if err := extractTarMember(archive, strings.TrimSuffix(tarball, ".tar.gz")+"/"+tool, bin); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	if err := os.Chmod(bin, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	return 0
}

func verifySHA256File(sumsPath, file, name string) error {
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("sha256sums.txt has no entry for %s", name)
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return err
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if got != want {
		return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", name, want, got)
	}
	return nil
}

func extractTarMember(archive, member, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("archive missing %s", member)
		}
		if err != nil {
			return err
		}
		if hdr.Name != member {
			continue
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fsperm.Dir)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, tr)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
}

func observabilityCache() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		return filepath.Join(xdg, "alt-observability-tools"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("HOME must be set")
	}
	return filepath.Join(home, ".cache", "alt-observability-tools"), nil
}
