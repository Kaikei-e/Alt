package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/config"
	"github.com/alt-project/altctl/internal/mtlsedge"
	"github.com/alt-project/altctl/internal/output"
)

const (
	cmdSentinelEnv    = "SENTINEL_CMD_CANDIDATE_SECRET_995"
	cmdSentinelDB     = "SENTINEL_SECRET_DB_PASS_995"
	cmdSentinelJWT    = "SENTINEL_SECRET_JWT_KEY_995"
	cmdSentinelStderr = "SENTINEL_CMD_STDERR_SECRET_995"
)

var cmdBaselineEnviron = []string{
	"COMPOSE_ENV_FILES=/tmp/evil.env",
	"COMPOSE_FILE=/tmp/evil.yaml",
	"COMPOSE_PROFILES=evil",
	"COMPOSE_PROJECT_NAME=evil",
	"DOCKER_TLS_VERIFY=1",
	"DOCKER_CERT_PATH=/tmp/certs",
	"DOCKER_HOST=unix:///var/run/docker.sock",
	"DOCKER_CONTEXT=default",
	"DOCKER_CONFIG=/tmp/docker-cfg",
	"PATH=/usr/bin:/bin",
	"HOME=/tmp/clean-home",
}

var cmdExpectedFrozenEnv = []string{
	"DOCKER_HOST=unix:///var/run/docker.sock",
	"DOCKER_CONTEXT=default",
	"DOCKER_CONFIG=/tmp/docker-cfg",
	"PATH=/usr/bin:/bin",
	"HOME=/tmp/clean-home",
	"COMPOSE_PROJECT_NAME=alt",
}

type cmdExitErr struct {
	code int
}

func (e *cmdExitErr) Error() string { return "exit error" }
func (e *cmdExitErr) ExitCode() int { return e.code }

type cmdRecordedCall struct {
	WorkDir string
	Env     []string
	Args    []string
}

type cmdFakeRunner struct {
	t                    *testing.T
	dockerUnreachable    bool
	composeConfigStdout  []byte
	composeConfigStderr  string
	composeConfigErr     error
	callerContainerIDs   string
	receiverContainerIDs string
	callerInspectJSON    []byte
	receiverInspectJSON  []byte
	callerImageJSON      []byte
	receiverImageJSON    []byte
	probeStderr          string
	probeErr             error
	calls                []cmdRecordedCall
	onComposeConfig      func(workDir string, args []string, env []string)
}

func newCmdFakeRunner(t *testing.T) *cmdFakeRunner {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "internal", "mtlsedge", "testdata", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		return b
	}
	return &cmdFakeRunner{
		t:                    t,
		composeConfigStdout:  read("compose_config_valid.json"),
		callerContainerIDs:   "1111111111111111111111111111111111111111111111111111111111111111\n",
		receiverContainerIDs: "2222222222222222222222222222222222222222222222222222222222222222\n",
		callerInspectJSON:    read("container_inspect_caller.json"),
		receiverInspectJSON:  read("container_inspect_receiver.json"),
		callerImageJSON:      read("image_inspect_caller.json"),
		receiverImageJSON:    read("image_inspect_receiver.json"),
	}
}

func (r *cmdFakeRunner) factory() mtlsedge.ExecutorFactory {
	return func(workDir string, env []string) compose.Executor {
		return &cmdFakeExec{runner: r, workDir: workDir, env: append([]string(nil), env...)}
	}
}

type cmdFakeExec struct {
	runner  *cmdFakeRunner
	workDir string
	env     []string
}

func (e *cmdFakeExec) Run(ctx context.Context, c string, args []string) error {
	var out, errBuf bytes.Buffer
	return e.RunWithPipes(ctx, c, args, &out, &errBuf)
}

func (e *cmdFakeExec) RunWithOutput(ctx context.Context, c string, args []string) ([]byte, error) {
	var out, errBuf bytes.Buffer
	err := e.RunWithPipes(ctx, c, args, &out, &errBuf)
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (e *cmdFakeExec) RunWithPipes(_ context.Context, _ string, args []string, stdout, stderr io.Writer) error {
	e.runner.calls = append(e.runner.calls, cmdRecordedCall{
		WorkDir: e.workDir,
		Env:     append([]string(nil), e.env...),
		Args:    append([]string(nil), args...),
	})
	if len(args) >= 1 && args[0] == "info" {
		if e.runner.dockerUnreachable {
			if stderr != nil {
				_, _ = io.WriteString(stderr, "Cannot connect "+cmdSentinelStderr)
			}
			return errors.New("unreachable")
		}
		if stdout != nil {
			_, _ = io.WriteString(stdout, "{}\n")
		}
		return nil
	}
	if len(args) >= 2 && args[0] == "context" && args[1] == "show" {
		if stdout != nil {
			_, _ = io.WriteString(stdout, "default\n")
		}
		return nil
	}
	if len(args) >= 3 && args[0] == "compose" && args[1] == "version" {
		if stdout != nil {
			_, _ = io.WriteString(stdout, "5.4.0\n")
		}
		return nil
	}
	if len(args) >= 1 && args[0] == "compose" {
		if e.runner.onComposeConfig != nil {
			e.runner.onComposeConfig(e.workDir, args, e.env)
		}
		if stdout != nil {
			_, _ = stdout.Write(e.runner.composeConfigStdout)
		}
		if stderr != nil && e.runner.composeConfigStderr != "" {
			_, _ = io.WriteString(stderr, e.runner.composeConfigStderr)
		}
		if e.runner.composeConfigErr != nil {
			return e.runner.composeConfigErr
		}
		return nil
	}
	if len(args) >= 1 && args[0] == "ps" {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "com.docker.compose.service=pre-processor") && stdout != nil {
			_, _ = io.WriteString(stdout, e.runner.callerContainerIDs)
		}
		if strings.Contains(joined, "com.docker.compose.service=news-creator") && stdout != nil {
			_, _ = io.WriteString(stdout, e.runner.receiverContainerIDs)
		}
		return nil
	}
	if len(args) >= 3 && args[0] == "container" && args[1] == "inspect" {
		target := args[len(args)-1]
		if strings.HasPrefix(target, "1111") && stdout != nil {
			_, _ = stdout.Write(e.runner.callerInspectJSON)
		}
		if strings.HasPrefix(target, "2222") && stdout != nil {
			_, _ = stdout.Write(e.runner.receiverInspectJSON)
		}
		return nil
	}
	if len(args) >= 3 && args[0] == "image" && args[1] == "inspect" {
		target := args[len(args)-1]
		if strings.Contains(target, "aaaa") && stdout != nil {
			_, _ = stdout.Write(e.runner.callerImageJSON)
		}
		if strings.Contains(target, "bbbb") && stdout != nil {
			_, _ = stdout.Write(e.runner.receiverImageJSON)
		}
		return nil
	}
	if len(args) >= 1 && args[0] == "run" {
		if stderr != nil && e.runner.probeStderr != "" {
			_, _ = io.WriteString(stderr, e.runner.probeStderr)
		}
		return e.runner.probeErr
	}
	return nil
}

func setupMTLSCommandProject(t *testing.T) (string, string, []byte, []byte, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	liveEnv := []byte("POSTGRES_PASSWORD=" + cmdSentinelDB + "\n")
	if err := os.WriteFile(filepath.Join(root, ".env"), liveEnv, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".altctl.yaml"), []byte("defaults:\n  stacks: [core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	compDir := filepath.Join(root, "compose")
	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.env", filepath.Join(compDir, ".env")); err != nil {
		t.Fatal(err)
	}
	composeFiles := map[string][]byte{
		"compose.yaml": []byte("name: alt\ninclude:\n  - ai.yaml\n"),
		"ai.yaml":      []byte("services:\n  pre-processor:\n    image: repo/pre:${IMAGE_TAG:-main}\n    environment:\n      - MTLS_ENFORCE=${PRE_PROCESSOR_MTLS_ENFORCE:-true}\n  news-creator:\n    environment:\n      - MTLS_ALLOWED_PEERS=${NEWS_CREATOR_MTLS_ALLOWED_PEERS:-pre-processor}\n"),
	}
	for name, b := range composeFiles {
		if err := os.WriteFile(filepath.Join(compDir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	candBytes := []byte("POSTGRES_PASSWORD=" + cmdSentinelEnv + "\n")
	cand := filepath.Join(t.TempDir(), "candidate.env")
	if err := os.WriteFile(cand, candBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	projectDir = root
	cfg = &config.Config{
		Project: config.ProjectConfig{Root: root},
		Compose: config.ComposeConfig{Dir: "compose"},
	}
	dryRun = false
	quiet = false
	_ = rootCmd.PersistentFlags().Set("dry-run", "false")
	_ = mtlsValidateCmd.Flags().Set("env-file", "")
	_ = mtlsValidateCmd.Flags().Set("json", "false")
	_ = mtlsObserveCmd.Flags().Set("expect-caller-image", "")
	_ = mtlsObserveCmd.Flags().Set("expect-receiver-image", "")
	_ = mtlsObserveCmd.Flags().Set("json", "false")

	t.Cleanup(func() {
		newMTLSExecutor = mtlsedge.DefaultExecutorFactory
		mtlsEnviron = os.Environ
		projectDir = ""
		dryRun = false
		_ = rootCmd.PersistentFlags().Set("dry-run", "false")
	})

	return root, cand, candBytes, liveEnv, composeFiles
}

func assertCmdOutputNoSecret(t *testing.T, stdout, stderr string) {
	t.Helper()
	for _, s := range []string{cmdSentinelEnv, cmdSentinelDB, cmdSentinelJWT, cmdSentinelStderr} {
		if strings.Contains(stdout, s) {
			t.Errorf("stdout leaked sentinel secret %q: %s", s, stdout)
		}
		if strings.Contains(stderr, s) {
			t.Errorf("stderr leaked sentinel secret %q: %s", s, stderr)
		}
	}
}

func TestMTLSCmd_ValidateAndObserveEndToEnd(t *testing.T) {
	t.Run("validate baseline json passes with exit 0 and staging layout verified", func(t *testing.T) {
		root, cand, candBytes, liveEnv, composeFiles := setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		var stageDirSeen string
		runner.onComposeConfig = func(workDir string, args []string, env []string) {
			stageDirSeen = workDir
			wantArgs := []string{"compose", "-f", filepath.Join(workDir, "compose", "compose.yaml"), "--env-file", filepath.Join(workDir, ".env"), "-p", "alt", "config", "--format", "json"}
			if !reflect.DeepEqual(args, wantArgs) {
				t.Errorf("compose config argv = %v, want %v", args, wantArgs)
			}
			if strings.HasPrefix(filepath.Clean(args[2]), filepath.Clean(root)+string(filepath.Separator)) {
				t.Errorf("-f path %q is inside project root %q", args[2], root)
			}
			if strings.HasPrefix(filepath.Clean(args[4]), filepath.Clean(root)+string(filepath.Separator)) {
				t.Errorf("--env-file path %q is inside project root %q", args[4], root)
			}
			compDirInfo, err := os.Lstat(filepath.Join(workDir, "compose"))
			if err != nil || !compDirInfo.IsDir() || compDirInfo.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("staged compose must be a real directory (not symlink), info=%v err=%v", compDirInfo, err)
			}
			link, err := os.Readlink(filepath.Join(workDir, "compose", ".env"))
			if err != nil || link != "../.env" {
				t.Errorf("staged compose/.env link = %q, err = %v", link, err)
			}
			gotViaCompEnv, err := os.ReadFile(filepath.Join(workDir, "compose", ".env"))
			if err != nil || !bytes.Equal(gotViaCompEnv, candBytes) || bytes.Equal(gotViaCompEnv, liveEnv) {
				t.Fatalf("staged compose/.env did not resolve to candidate bytes")
			}
			for name, want := range composeFiles {
				p := filepath.Join(workDir, "compose", name)
				fi, err := os.Lstat(p)
				if err != nil || !fi.Mode().IsRegular() {
					t.Errorf("staged compose/%s not regular file", name)
				}
				got, _ := os.ReadFile(p)
				if !bytes.Equal(got, want) {
					t.Errorf("staged compose/%s bytes differ", name)
				}
			}
		}
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string { return append([]string(nil), cmdBaselineEnviron...) }

		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{"mtls", "validate", "--env-file", cand, "--json"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("expected nil error (exit 0), got %v", err)
		}
		if stageDirSeen == "" {
			t.Fatal("expected staging dir to be inspected during compose config")
		}
		if _, err := os.Stat(stageDirSeen); !os.IsNotExist(err) {
			t.Errorf("expected staging dir %s to be removed afterwards", stageDirSeen)
		}
		for _, c := range runner.calls {
			if !reflect.DeepEqual(c.Env, cmdExpectedFrozenEnv) {
				t.Errorf("call %v env = %v, want %v", c.Args, c.Env, cmdExpectedFrozenEnv)
			}
		}
		assertCmdOutputNoSecret(t, outBuf.String(), errBuf.String())

		if !strings.Contains(outBuf.String(), `"edge": "pre-processor->news-creator"`) {
			t.Errorf("expected unescaped edge arrow in JSON output, got:\n%s", outBuf.String())
		}
		var rawMap map[string]any
		if err := json.Unmarshal(outBuf.Bytes(), &rawMap); err != nil {
			t.Fatalf("unmarshal raw json: %v", err)
		}
		if _, exists := rawMap["candidate_validate"]; exists {
			t.Errorf("unexpected top-level candidate_validate key in JSON")
		}

		var ev mtlsedge.Evidence
		if err := json.Unmarshal(outBuf.Bytes(), &ev); err != nil {
			t.Fatalf("unmarshal evidence JSON: %v", err)
		}
		if ev.SchemaVersion != 1 || ev.Edge != "pre-processor->news-creator" {
			t.Errorf("unexpected header in evidence: %+v", ev)
		}
		if ev.Stages.CandidateValidate == nil || ev.Stages.CandidateValidate.Status != mtlsedge.StatusPass {
			t.Errorf("expected candidate_validate pass, got %+v", ev.Stages.CandidateValidate)
		}
	})

	t.Run("stale allowlist override fails candidate render and leaves live files byte-identical", func(t *testing.T) {
		root, cand, _, liveEnv, composeFiles := setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		runner.composeConfigStdout = []byte(`{
  "services": {
    "pre-processor": {
      "environment": {
        "CERT_SUBJECT": "pre-processor",
        "MTLS_CA_FILE": "/trust/ca-bundle.pem",
        "MTLS_CERT_FILE": "/certs/svc-cert.pem",
        "MTLS_ENFORCE": "true",
        "MTLS_KEY_FILE": "/certs/svc-key.pem",
        "NEWS_CREATOR_HOST": "https://news-creator:9443"
      }
    },
    "news-creator": {
      "environment": {
        "CERT_SANS": "news-creator,localhost",
        "INBOUND_MTLS": "true",
        "MTLS_ALLOWED_PEERS": "recap-worker",
        "PEER_IDENTITY_STRICT": "true"
      }
    }
  }
}`)
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string { return append([]string(nil), cmdBaselineEnviron...) }

		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{"mtls", "validate", "--env-file", cand, "--json"})

		err := rootCmd.Execute()
		var cliErr *output.CLIError
		if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitGeneral {
			t.Fatalf("expected ExitGeneral (1), got %v", err)
		}
		assertCmdOutputNoSecret(t, outBuf.String(), errBuf.String())

		gotEnv, _ := os.ReadFile(filepath.Join(root, ".env"))
		if !bytes.Equal(gotEnv, liveEnv) {
			t.Error("live .env modified")
		}
		for name, want := range composeFiles {
			got, _ := os.ReadFile(filepath.Join(root, "compose", name))
			if !bytes.Equal(got, want) {
				t.Errorf("live compose/%s modified", name)
			}
		}
	})

	t.Run("shell override fails environment freeze in validate without invoking compose", func(t *testing.T) {
		_, cand, _, _, _ := setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string {
			return []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "NEWS_CREATOR_MTLS_ALLOWED_PEERS=" + cmdSentinelEnv}
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{"mtls", "validate", "--env-file", cand, "--json"})

		err := rootCmd.Execute()
		var cliErr *output.CLIError
		if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitGeneral {
			t.Fatalf("expected ExitGeneral (1), got %v", err)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("expected 0 compose/docker calls, got %d", len(runner.calls))
		}
		assertCmdOutputNoSecret(t, outBuf.String(), errBuf.String())
	})

	t.Run("observe baseline json passes with exit 0 and pins frozen env and compose -f argv", func(t *testing.T) {
		root, _, _, _, _ := setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string { return append([]string(nil), cmdBaselineEnviron...) }

		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{
			"mtls", "observe",
			"--expect-caller-image", "ghcr.io/example/pre-processor@sha256:1010101010101010101010101010101010101010101010101010101010101010",
			"--expect-receiver-image", "ghcr.io/example/news-creator@sha256:2020202020202020202020202020202020202020202020202020202020202020",
			"--json",
		})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("expected nil error (exit 0), got %v", err)
		}
		assertCmdOutputNoSecret(t, outBuf.String(), errBuf.String())

		var rawMap map[string]any
		if err := json.Unmarshal(outBuf.Bytes(), &rawMap); err != nil {
			t.Fatalf("unmarshal raw json: %v", err)
		}
		if _, exists := rawMap["live_observation"]; exists {
			t.Errorf("unexpected top-level live_observation key in JSON")
		}

		foundComposeConfig := false
		wantComposeArgs := []string{"compose", "-f", filepath.Join(root, "compose", "compose.yaml"), "--env-file", filepath.Join(root, ".env"), "-p", "alt", "config", "--format", "json"}
		for _, c := range runner.calls {
			if !reflect.DeepEqual(c.Env, cmdExpectedFrozenEnv) {
				t.Errorf("call %v env = %v, want %v", c.Args, c.Env, cmdExpectedFrozenEnv)
			}
			if len(c.Args) >= 2 && c.Args[0] == "compose" && c.Args[1] == "-f" {
				foundComposeConfig = true
				if !reflect.DeepEqual(c.Args, wantComposeArgs) {
					t.Errorf("observe compose config argv = %v, want %v", c.Args, wantComposeArgs)
				}
			}
		}
		if !foundComposeConfig {
			t.Fatal("expected observe to run docker compose -f <root>/compose/compose.yaml -p alt config --format json")
		}

		var ev mtlsedge.Evidence
		if err := json.Unmarshal(outBuf.Bytes(), &ev); err != nil {
			t.Fatalf("unmarshal evidence JSON: %v", err)
		}
		if !ev.RolloutSucceeded {
			t.Error("expected rollout_succeeded = true")
		}
		if ev.Stages.LiveObservation == nil || ev.Stages.LiveObservation.Status != mtlsedge.StatusPass {
			t.Errorf("expected live_observation pass, got %+v", ev.Stages.LiveObservation)
		}
	})

	t.Run("observe shell override fails environment freeze without invoking docker", func(t *testing.T) {
		setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string {
			return []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "IMAGE_TAG=leaky"}
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{"mtls", "observe", "--json"})

		err := rootCmd.Execute()
		var cliErr *output.CLIError
		if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitGeneral {
			t.Fatalf("expected ExitGeneral (1), got %v", err)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("expected 0 docker calls, got %d", len(runner.calls))
		}
	})

	t.Run("observe caller digest mismatch fails with exit 1", func(t *testing.T) {
		setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string { return append([]string(nil), cmdBaselineEnviron...) }

		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{
			"mtls", "observe",
			"--expect-caller-image", "ghcr.io/example/pre-processor@sha256:9999999999999999999999999999999999999999999999999999999999999999",
			"--json",
		})

		err := rootCmd.Execute()
		var cliErr *output.CLIError
		if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitGeneral {
			t.Fatalf("expected ExitGeneral (1), got %v", err)
		}
		var ev mtlsedge.Evidence
		if err := json.Unmarshal(outBuf.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		if ev.RolloutSucceeded {
			t.Error("expected rollout_succeeded = false")
		}
		wantObservedCaller := "ghcr.io/example/pre-processor@sha256:1010101010101010101010101010101010101010101010101010101010101010"
		if ev.Stages.LiveObservation == nil || ev.Stages.LiveObservation.CallerImageDigest != wantObservedCaller {
			t.Errorf("caller_image_digest = %v, want observed %q", ev.Stages.LiveObservation, wantObservedCaller)
		}
	})

	t.Run("observe receiver digest mismatch fails with exit 1", func(t *testing.T) {
		setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string { return append([]string(nil), cmdBaselineEnviron...) }

		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{
			"mtls", "observe",
			"--expect-receiver-image", "ghcr.io/example/news-creator@sha256:8888888888888888888888888888888888888888888888888888888888888888",
			"--json",
		})

		err := rootCmd.Execute()
		var cliErr *output.CLIError
		if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitGeneral {
			t.Fatalf("expected ExitGeneral (1), got %v", err)
		}
		var ev mtlsedge.Evidence
		if err := json.Unmarshal(outBuf.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		if ev.RolloutSucceeded {
			t.Error("expected rollout_succeeded = false")
		}
		wantObservedReceiver := "ghcr.io/example/news-creator@sha256:2020202020202020202020202020202020202020202020202020202020202020"
		if ev.Stages.LiveObservation == nil || ev.Stages.LiveObservation.ReceiverImageDigest != wantObservedReceiver {
			t.Errorf("receiver_image_digest = %v, want observed %q", ev.Stages.LiveObservation, wantObservedReceiver)
		}
	})

	t.Run("observe probe container cannot start returns not_observed and exit 1", func(t *testing.T) {
		setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		runner.probeErr = &cmdExitErr{code: 126}
		runner.probeStderr = "container start error " + cmdSentinelStderr
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string { return append([]string(nil), cmdBaselineEnviron...) }

		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{"mtls", "observe", "--json"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for not_observed probe")
		}
		var cliErr *output.CLIError
		if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitGeneral {
			t.Fatalf("expected ExitGeneral (1), got %v", err)
		}
		assertCmdOutputNoSecret(t, outBuf.String(), errBuf.String())
	})

	t.Run("validate and observe with docker unreachable return exit 3", func(t *testing.T) {
		_, cand, _, _, _ := setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		runner.dockerUnreachable = true
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string { return append([]string(nil), cmdBaselineEnviron...) }

		for _, argv := range [][]string{
			{"mtls", "validate", "--env-file", cand, "--json"},
			{"mtls", "observe", "--json"},
		} {
			var outBuf, errBuf bytes.Buffer
			rootCmd.SetOut(&outBuf)
			rootCmd.SetErr(&errBuf)
			rootCmd.SetArgs(argv)

			err := rootCmd.Execute()
			var cliErr *output.CLIError
			if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitComposeError {
				t.Fatalf("argv %v: expected ExitComposeError (3), got %v", argv, err)
			}
			assertCmdOutputNoSecret(t, outBuf.String(), errBuf.String())
		}
	})

	t.Run("dry-run flag and malformed expect-image flags return exit 2 before any docker call", func(t *testing.T) {
		_, cand, _, _, _ := setupMTLSCommandProject(t)
		runner := newCmdFakeRunner(t)
		newMTLSExecutor = runner.factory()
		mtlsEnviron = func() []string { return append([]string(nil), cmdBaselineEnviron...) }

		badCases := [][]string{
			{"mtls", "validate", "--dry-run", "--env-file", cand},
			{"mtls", "validate"},
			{"mtls", "observe", "--dry-run"},
			{"mtls", "observe", "--expect-caller-image", "sha256:1010101010101010101010101010101010101010101010101010101010101010"},
			{"mtls", "observe", "--expect-receiver-image", "ghcr.io/example/news-creator@sha256:ABCDEF"},
		}
		for _, argv := range badCases {
			_ = rootCmd.PersistentFlags().Set("dry-run", "false")
			_ = mtlsValidateCmd.Flags().Set("env-file", "")
			_ = mtlsObserveCmd.Flags().Set("expect-caller-image", "")
			_ = mtlsObserveCmd.Flags().Set("expect-receiver-image", "")
			var outBuf, errBuf bytes.Buffer
			rootCmd.SetOut(&outBuf)
			rootCmd.SetErr(&errBuf)
			rootCmd.SetArgs(argv)

			err := rootCmd.Execute()
			var cliErr *output.CLIError
			if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitUsageError {
				t.Fatalf("argv %v: expected ExitUsageError (2), got %v", argv, err)
			}
		}
		if len(runner.calls) != 0 {
			t.Fatalf("expected 0 docker calls on usage errors, got %d", len(runner.calls))
		}
	})
}
