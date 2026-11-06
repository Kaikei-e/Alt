package mtlsedge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/compose"
)

const (
	sentinelSecretEnv     = "SENTINEL_SECRET_CANDIDATE_ENV_995"
	sentinelSecretDB      = "SENTINEL_SECRET_DB_PASS_995"
	sentinelSecretJWT     = "SENTINEL_SECRET_JWT_KEY_995"
	sentinelSecretStderr  = "SENTINEL_SECRET_STDERR_LEAK_995"
	expectedCallerDigest  = "ghcr.io/example/pre-processor@sha256:1010101010101010101010101010101010101010101010101010101010101010"
	expectedReceiverImage = "ghcr.io/example/news-creator@sha256:2020202020202020202020202020202020202020202020202020202020202020"
)

var baselineInputEnviron = []string{
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
	"UNRELATED_SHELL_VAR=ignored",
}

var expectedFrozenChildEnv = []string{
	"DOCKER_HOST=unix:///var/run/docker.sock",
	"DOCKER_CONTEXT=default",
	"DOCKER_CONFIG=/tmp/docker-cfg",
	"PATH=/usr/bin:/bin",
	"HOME=/tmp/clean-home",
	"COMPOSE_PROJECT_NAME=alt",
}

type fakeExitError struct {
	code int
}

func (e *fakeExitError) Error() string {
	return "exit error"
}

func (e *fakeExitError) ExitCode() int {
	return e.code
}

type recordedCall struct {
	WorkDir string
	Env     []string
	Cmd     string
	Args    []string
}

type fakeRunner struct {
	t                    *testing.T
	calls                []recordedCall
	dockerUnreachable    bool
	contextErr           error
	composeVerErr        error
	composeConfigStdout  []byte
	composeConfigStderr  string
	composeConfigErr     error
	callerContainerIDs   string
	receiverContainerIDs string
	callerInspectJSON    []byte
	receiverInspectJSON  []byte
	callerImageJSON      []byte
	callerImageErr       error
	receiverImageJSON    []byte
	receiverImageErr     error
	probeStderr          string
	probeErr             error
	onComposeConfig      func(call recordedCall)
}

func newBaselineFakeRunner(t *testing.T) *fakeRunner {
	t.Helper()
	return &fakeRunner{
		t:                    t,
		composeConfigStdout:  mustReadFixture(t, "compose_config_valid.json"),
		callerContainerIDs:   "1111111111111111111111111111111111111111111111111111111111111111\n",
		receiverContainerIDs: "2222222222222222222222222222222222222222222222222222222222222222\n",
		callerInspectJSON:    mustReadFixture(t, "container_inspect_caller.json"),
		receiverInspectJSON:  mustReadFixture(t, "container_inspect_receiver.json"),
		callerImageJSON:      mustReadFixture(t, "image_inspect_caller.json"),
		receiverImageJSON:    mustReadFixture(t, "image_inspect_receiver.json"),
	}
}

func (f *fakeRunner) factory() ExecutorFactory {
	return func(workDir string, env []string) compose.Executor {
		envCopy := append([]string(nil), env...)
		return &fakeExecutorInstance{
			runner:  f,
			workDir: workDir,
			env:     envCopy,
		}
	}
}

type fakeExecutorInstance struct {
	runner  *fakeRunner
	workDir string
	env     []string
}

func (e *fakeExecutorInstance) Run(ctx context.Context, cmd string, args []string) error {
	var stdout, stderr bytes.Buffer
	return e.RunWithPipes(ctx, cmd, args, &stdout, &stderr)
}

func (e *fakeExecutorInstance) RunWithOutput(ctx context.Context, cmd string, args []string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	err := e.RunWithPipes(ctx, cmd, args, &stdout, &stderr)
	if err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func (e *fakeExecutorInstance) RunWithPipes(_ context.Context, cmd string, args []string, stdout, stderr io.Writer) error {
	call := recordedCall{
		WorkDir: e.workDir,
		Env:     append([]string(nil), e.env...),
		Cmd:     cmd,
		Args:    append([]string(nil), args...),
	}
	e.runner.calls = append(e.runner.calls, call)

	if len(args) >= 1 && args[0] == "info" {
		if e.runner.dockerUnreachable {
			if stderr != nil {
				_, _ = io.WriteString(stderr, "Cannot connect to the Docker daemon "+sentinelSecretStderr)
			}
			return errors.New("docker unreachable")
		}
		if stdout != nil {
			_, _ = io.WriteString(stdout, "{}\n")
		}
		return nil
	}

	if len(args) >= 2 && args[0] == "context" && args[1] == "show" {
		if e.runner.contextErr != nil {
			return e.runner.contextErr
		}
		if stdout != nil {
			_, _ = io.WriteString(stdout, "default\n")
		}
		return nil
	}

	if len(args) >= 3 && args[0] == "compose" && args[1] == "version" {
		if e.runner.composeVerErr != nil {
			return e.runner.composeVerErr
		}
		if stdout != nil {
			_, _ = io.WriteString(stdout, "5.4.0\n")
		}
		return nil
	}

	if len(args) >= 1 && args[0] == "compose" && containsArg(args, "config") {
		if e.runner.onComposeConfig != nil {
			e.runner.onComposeConfig(call)
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
		if strings.Contains(joined, "com.docker.compose.service=pre-processor") {
			if stdout != nil {
				_, _ = io.WriteString(stdout, e.runner.callerContainerIDs)
			}
			return nil
		}
		if strings.Contains(joined, "com.docker.compose.service=news-creator") {
			if stdout != nil {
				_, _ = io.WriteString(stdout, e.runner.receiverContainerIDs)
			}
			return nil
		}
	}

	if len(args) >= 3 && args[0] == "container" && args[1] == "inspect" {
		target := args[len(args)-1]
		if strings.HasPrefix(target, "1111") {
			if stdout != nil {
				_, _ = stdout.Write(e.runner.callerInspectJSON)
			}
			return nil
		}
		if strings.HasPrefix(target, "2222") {
			if stdout != nil {
				_, _ = stdout.Write(e.runner.receiverInspectJSON)
			}
			return nil
		}
	}

	if len(args) >= 3 && args[0] == "image" && args[1] == "inspect" {
		target := args[len(args)-1]
		if strings.Contains(target, "aaaa") {
			if e.runner.callerImageErr != nil {
				return e.runner.callerImageErr
			}
			if stdout != nil {
				_, _ = stdout.Write(e.runner.callerImageJSON)
			}
			return nil
		}
		if strings.Contains(target, "bbbb") {
			if e.runner.receiverImageErr != nil {
				return e.runner.receiverImageErr
			}
			if stdout != nil {
				_, _ = stdout.Write(e.runner.receiverImageJSON)
			}
			return nil
		}
	}

	if len(args) >= 1 && args[0] == "run" {
		if stderr != nil && e.runner.probeStderr != "" {
			_, _ = io.WriteString(stderr, e.runner.probeStderr)
		}
		if e.runner.probeErr != nil {
			return e.runner.probeErr
		}
		return nil
	}

	return nil
}

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func setupFakeProject(t *testing.T) (projectDir string, candidateEnvPath string, candidateBytes []byte, liveEnvBytes []byte, composeFiles map[string][]byte) {
	t.Helper()
	projectDir = t.TempDir()

	liveEnvBytes = []byte("LIVE_ENV_ONLY=1\nDB_PASSWORD=" + sentinelSecretDB + "\n")
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), liveEnvBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".altctl.yaml"), []byte("defaults:\n  stacks: [core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "scripts", "marker.txt"), []byte("marker"), 0o644); err != nil {
		t.Fatal(err)
	}

	composeDir := filepath.Join(projectDir, "compose")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.env", filepath.Join(composeDir, ".env")); err != nil {
		t.Fatal(err)
	}

	composeRootYAML := []byte("name: alt\ninclude:\n  - base.yaml\n  - ai.yaml\n")
	baseYAML := []byte("services: {}\n")
	aiYAML := []byte(`services:
  news-creator:
    image: ghcr.io/example/news-creator:${IMAGE_TAG:-main}
    env_file:
      - ../.env
    environment:
      - INBOUND_MTLS=true
      - PEER_IDENTITY_STRICT=${NEWS_CREATOR_PEER_IDENTITY_STRICT:-true}
      - CERT_SANS=news-creator,localhost
      - MTLS_ALLOWED_PEERS=${NEWS_CREATOR_MTLS_ALLOWED_PEERS:-recap-worker,acolyte-orchestrator,rag-orchestrator,recap-evaluator,pre-processor}
  pre-processor:
    image: ghcr.io/example/pre-processor:${IMAGE_TAG:-main}
    env_file:
      - ../.env
    environment:
      - MTLS_ENFORCE=${PRE_PROCESSOR_MTLS_ENFORCE:-true}
      - MTLS_CERT_FILE=/certs/svc-cert.pem
      - MTLS_KEY_FILE=/certs/svc-key.pem
      - MTLS_CA_FILE=/trust/ca-bundle.pem
      - CERT_SUBJECT=pre-processor
      - NEWS_CREATOR_HOST=https://news-creator:9443
      - DB_PASSWORD=${POSTGRES_PASSWORD}
`)

	composeFiles = map[string][]byte{
		"compose.yaml": composeRootYAML,
		"base.yaml":    baseYAML,
		"ai.yaml":      aiYAML,
	}
	for name, content := range composeFiles {
		if err := os.WriteFile(filepath.Join(composeDir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	candidateBytes = []byte("POSTGRES_PASSWORD=" + sentinelSecretEnv + "\nPRE_PROCESSOR_MTLS_ENFORCE=true\n")
	candidateEnvPath = filepath.Join(t.TempDir(), "candidate.env")
	if err := os.WriteFile(candidateEnvPath, candidateBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	return projectDir, candidateEnvPath, candidateBytes, liveEnvBytes, composeFiles
}

func assertNoSecretsLeaked(t *testing.T, res *Result) {
	t.Helper()
	rawJSON, err := json.Marshal(res.Evidence)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	text := res.RenderText()
	for _, secret := range []string{sentinelSecretEnv, sentinelSecretDB, sentinelSecretJWT, sentinelSecretStderr} {
		if strings.Contains(string(rawJSON), secret) {
			t.Errorf("JSON evidence leaked sentinel secret %q: %s", secret, string(rawJSON))
		}
		if strings.Contains(text, secret) {
			t.Errorf("RenderText leaked sentinel secret %q: %s", secret, text)
		}
	}
}

func TestValidate_BaselineStagingAndFrozenEnv(t *testing.T) {
	projectDir, candidateEnvPath, candidateBytes, liveEnvBytes, composeFiles := setupFakeProject(t)
	runner := newBaselineFakeRunner(t)

	var capturedStageDir string
	runner.onComposeConfig = func(call recordedCall) {
		capturedStageDir = call.WorkDir
		info, err := os.Stat(capturedStageDir)
		if err != nil {
			t.Fatalf("stat stageDir: %v", err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("stageDir perm = %o, want 0700", info.Mode().Perm())
		}

		// Pin exact compose config argv and verify -f is staged and NOT under projectDir
		wantStagedCompose := filepath.Join(capturedStageDir, "compose", "compose.yaml")
		wantArgs := []string{"compose", "-f", wantStagedCompose, "-p", "alt", "config", "--format", "json"}
		if !reflect.DeepEqual(call.Args, wantArgs) {
			t.Fatalf("compose config argv = %v, want %v", call.Args, wantArgs)
		}
		cleanProject := filepath.Clean(projectDir) + string(filepath.Separator)
		if strings.HasPrefix(filepath.Clean(call.Args[2]), cleanProject) {
			t.Fatalf("compose -f path %q points inside live projectDir %q", call.Args[2], projectDir)
		}

		// 1. <stage>/compose itself is a real directory, NOT a symlink
		composeDirInfo, err := os.Lstat(filepath.Join(capturedStageDir, "compose"))
		if err != nil {
			t.Fatalf("lstat staged compose dir: %v", err)
		}
		if !composeDirInfo.IsDir() || composeDirInfo.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("staged compose must be a real directory (not symlink), got mode %v", composeDirInfo.Mode())
		}

		// 2. stage .env has candidate bytes and 0600 permissions
		stageEnvPath := filepath.Join(capturedStageDir, ".env")
		stageEnvInfo, err := os.Lstat(stageEnvPath)
		if err != nil {
			t.Fatalf("lstat stage .env: %v", err)
		}
		if !stageEnvInfo.Mode().IsRegular() {
			t.Errorf("stage .env must be a regular file, got mode %v", stageEnvInfo.Mode())
		}
		if stageEnvInfo.Mode().Perm() != 0o600 {
			t.Errorf("stage .env perm = %o, want 0600", stageEnvInfo.Mode().Perm())
		}
		gotEnvBytes, err := os.ReadFile(stageEnvPath)
		if err != nil {
			t.Fatalf("read stage .env: %v", err)
		}
		if !bytes.Equal(gotEnvBytes, candidateBytes) {
			t.Errorf("stage .env bytes mismatch")
		}

		// 3. compose/.env is a symlink to ../.env and resolves to candidateBytes (not liveEnvBytes)
		composeEnvLink := filepath.Join(capturedStageDir, "compose", ".env")
		linkTarget, err := os.Readlink(composeEnvLink)
		if err != nil {
			t.Fatalf("readlink compose/.env: %v", err)
		}
		if linkTarget != "../.env" {
			t.Errorf("compose/.env symlink target = %q, want %q", linkTarget, "../.env")
		}
		gotViaComposeEnv, err := os.ReadFile(composeEnvLink)
		if err != nil {
			t.Fatalf("readFile compose/.env: %v", err)
		}
		if !bytes.Equal(gotViaComposeEnv, candidateBytes) || bytes.Equal(gotViaComposeEnv, liveEnvBytes) {
			t.Fatalf("compose/.env did not resolve to candidate bytes")
		}

		// 4. compose files are regular byte-identical copies (never symlinks)
		for name, wantBytes := range composeFiles {
			p := filepath.Join(capturedStageDir, "compose", name)
			fi, err := os.Lstat(p)
			if err != nil {
				t.Fatalf("lstat staged compose/%s: %v", name, err)
			}
			if !fi.Mode().IsRegular() {
				t.Errorf("staged compose/%s must be regular file (not symlink), mode=%v", name, fi.Mode())
			}
			gotBytes, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read staged compose/%s: %v", name, err)
			}
			if !bytes.Equal(gotBytes, wantBytes) {
				t.Errorf("staged compose/%s bytes differ from source", name)
			}
		}

		// 5. other top-level entries are symlinks
		for _, entry := range []string{".altctl.yaml", "scripts"} {
			p := filepath.Join(capturedStageDir, entry)
			fi, err := os.Lstat(p)
			if err != nil {
				t.Fatalf("lstat staged %s: %v", entry, err)
			}
			if fi.Mode()&os.ModeSymlink == 0 {
				t.Errorf("staged top-level %s must be a symlink, got mode %v", entry, fi.Mode())
			}
		}
	}

	res, err := Validate(context.Background(), ValidateOptions{
		ProjectDir:       projectDir,
		CandidateEnvFile: candidateEnvPath,
		Environ:          baselineInputEnviron,
		NewExecutor:      runner.factory(),
	})
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if !res.DockerReachable {
		t.Fatal("expected DockerReachable = true")
	}
	if !res.Passed() {
		t.Fatalf("expected baseline Validate to pass, got %+v", res.Evidence)
	}
	if capturedStageDir == "" {
		t.Fatal("expected compose config to be called in staging dir")
	}
	if _, err := os.Stat(capturedStageDir); !os.IsNotExist(err) {
		t.Errorf("staging dir %s still exists after Validate", capturedStageDir)
	}

	for i, call := range runner.calls {
		if !reflect.DeepEqual(call.Env, expectedFrozenChildEnv) {
			t.Errorf("call[%d] (%v) env = %v, want %v", i, call.Args, call.Env, expectedFrozenChildEnv)
		}
	}

	cv := res.Evidence.Stages.CandidateValidate
	if cv == nil {
		t.Fatal("expected Stages.CandidateValidate to be populated")
	}
	if cv.Status != StatusPass {
		t.Errorf("status = %q, want %q", cv.Status, StatusPass)
	}
	wantSum := sha256.Sum256(candidateBytes)
	if cv.CandidateEnvSHA256 != hex.EncodeToString(wantSum[:]) {
		t.Errorf("candidate_env_sha256 = %q, want %q", cv.CandidateEnvSHA256, hex.EncodeToString(wantSum[:]))
	}
	assertNoSecretsLeaked(t, res)
}

func TestValidate_StaleAllowlistOverrideFailsAndLeavesLiveTreeUntouched(t *testing.T) {
	projectDir, candidateEnvPath, _, liveEnvBytes, composeFiles := setupFakeProject(t)
	runner := newBaselineFakeRunner(t)
	runner.composeConfigStdout = []byte(`{
  "name": "alt",
  "services": {
    "pre-processor": {
      "environment": {
        "CERT_SUBJECT": "pre-processor",
        "MTLS_CA_FILE": "/trust/ca-bundle.pem",
        "MTLS_CERT_FILE": "/certs/svc-cert.pem",
        "MTLS_ENFORCE": "true",
        "MTLS_KEY_FILE": "/certs/svc-key.pem",
        "NEWS_CREATOR_HOST": "https://news-creator:9443",
        "DB_PASSWORD": "` + sentinelSecretDB + `"
      }
    },
    "news-creator": {
      "environment": {
        "CERT_SANS": "news-creator,localhost",
        "INBOUND_MTLS": "true",
        "MTLS_ALLOWED_PEERS": "recap-worker,acolyte-orchestrator,rag-orchestrator",
        "PEER_IDENTITY_STRICT": "true"
      }
    }
  }
}`)

	res, err := Validate(context.Background(), ValidateOptions{
		ProjectDir:       projectDir,
		CandidateEnvFile: candidateEnvPath,
		Environ:          []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
		NewExecutor:      runner.factory(),
	})
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if res.Passed() {
		t.Fatal("expected Validate to fail when pre-processor is missing from MTLS_ALLOWED_PEERS")
	}
	cv := res.Evidence.Stages.CandidateValidate
	if cv == nil || cv.Status != StatusFail {
		t.Fatalf("expected CandidateValidate status=fail, got %+v", cv)
	}
	if !containsString(cv.Reasons, "caller_not_in_allowed_peers:CERT_SUBJECT,MTLS_ALLOWED_PEERS") {
		t.Errorf("expected caller_not_in_allowed_peers:CERT_SUBJECT,MTLS_ALLOWED_PEERS, got %v", cv.Reasons)
	}

	gotLiveEnv, err := os.ReadFile(filepath.Join(projectDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotLiveEnv, liveEnvBytes) {
		t.Error("live .env was modified")
	}
	for name, want := range composeFiles {
		got, err := os.ReadFile(filepath.Join(projectDir, "compose", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("live compose/%s was modified", name)
		}
	}
	assertNoSecretsLeaked(t, res)
}

func TestValidate_ShellOverrideFailsBeforeCompose(t *testing.T) {
	projectDir, candidateEnvPath, _, _, _ := setupFakeProject(t)

	t.Run("non-empty shell override fails closed before compose", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		res, err := Validate(context.Background(), ValidateOptions{
			ProjectDir:       projectDir,
			CandidateEnvFile: candidateEnvPath,
			Environ: []string{
				"PATH=/usr/bin:/bin",
				"HOME=/tmp",
				"NEWS_CREATOR_MTLS_ALLOWED_PEERS=" + sentinelSecretEnv,
				"IMAGE_TAG=override-tag",
			},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Validate error: %v", err)
		}
		if res.Passed() {
			t.Fatal("expected Validate to fail environment freeze on shell override")
		}
		if len(runner.calls) != 0 {
			t.Fatalf("expected zero docker/compose invocations when environment freeze fails, got %d", len(runner.calls))
		}
		cv := res.Evidence.Stages.CandidateValidate
		wantReasons := []string{
			"shell_env_override:IMAGE_TAG",
			"shell_env_override:NEWS_CREATOR_MTLS_ALLOWED_PEERS",
		}
		if cv == nil || cv.Status != StatusFail || !reflect.DeepEqual(cv.Reasons, wantReasons) {
			t.Errorf("got %+v, want reasons %v", cv, wantReasons)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("empty shell value IMAGE_TAG= still trips environment freeze", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		res, err := Validate(context.Background(), ValidateOptions{
			ProjectDir:       projectDir,
			CandidateEnvFile: candidateEnvPath,
			Environ: []string{
				"PATH=/usr/bin:/bin",
				"HOME=/tmp",
				"IMAGE_TAG=",
			},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Validate error: %v", err)
		}
		if res.Passed() {
			t.Fatal("expected Validate to fail environment freeze when IMAGE_TAG= is set but empty")
		}
		if len(runner.calls) != 0 {
			t.Fatalf("expected zero docker/compose invocations, got %d", len(runner.calls))
		}
		cv := res.Evidence.Stages.CandidateValidate
		wantReasons := []string{"shell_env_override:IMAGE_TAG"}
		if cv == nil || !reflect.DeepEqual(cv.Reasons, wantReasons) {
			t.Errorf("reasons = %v, want %v", cv.Reasons, wantReasons)
		}
	})
}

func TestValidate_AsymmetricSNINotInReceiverSANs(t *testing.T) {
	projectDir, candidateEnvPath, _, _, _ := setupFakeProject(t)
	runner := newBaselineFakeRunner(t)
	runner.composeConfigStdout = []byte(`{
  "name": "alt",
  "services": {
    "pre-processor": {
      "environment": {
        "CERT_SUBJECT": "pre-processor",
        "MTLS_CA_FILE": "/trust/ca-bundle.pem",
        "MTLS_CERT_FILE": "/certs/svc-cert.pem",
        "MTLS_ENFORCE": "true",
        "MTLS_KEY_FILE": "/certs/svc-key.pem",
        "NEWS_CREATOR_HOST": "https://news-creator:9443",
        "NEWS_CREATOR_MTLS_SERVER_NAME": "wrong-sni.internal"
      }
    },
    "news-creator": {
      "environment": {
        "CERT_SANS": "news-creator,localhost",
        "INBOUND_MTLS": "true",
        "MTLS_ALLOWED_PEERS": "pre-processor",
        "PEER_IDENTITY_STRICT": "true"
      }
    }
  }
}`)

	res, err := Validate(context.Background(), ValidateOptions{
		ProjectDir:       projectDir,
		CandidateEnvFile: candidateEnvPath,
		Environ:          []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
		NewExecutor:      runner.factory(),
	})
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if res.Passed() {
		t.Fatal("expected Validate to fail when caller SNI is not in receiver CERT_SANS")
	}
	cv := res.Evidence.Stages.CandidateValidate
	if cv == nil || !containsString(cv.Reasons, "sni_not_in_receiver_sans:NEWS_CREATOR_MTLS_SERVER_NAME,CERT_SANS") {
		t.Errorf("expected sni_not_in_receiver_sans:NEWS_CREATOR_MTLS_SERVER_NAME,CERT_SANS, got %+v", cv)
	}
	assertNoSecretsLeaked(t, res)
}

func TestValidate_CallerMTLSEnforceFalseFails(t *testing.T) {
	projectDir, candidateEnvPath, _, _, _ := setupFakeProject(t)
	runner := newBaselineFakeRunner(t)
	runner.composeConfigStdout = []byte(`{
  "name": "alt",
  "services": {
    "pre-processor": {
      "environment": {
        "CERT_SUBJECT": "pre-processor",
        "MTLS_CA_FILE": "/trust/ca-bundle.pem",
        "MTLS_CERT_FILE": "/certs/svc-cert.pem",
        "MTLS_ENFORCE": "false",
        "MTLS_KEY_FILE": "/certs/svc-key.pem",
        "NEWS_CREATOR_HOST": "https://news-creator:9443"
      }
    },
    "news-creator": {
      "environment": {
        "CERT_SANS": "news-creator,localhost",
        "INBOUND_MTLS": "true",
        "MTLS_ALLOWED_PEERS": "pre-processor",
        "PEER_IDENTITY_STRICT": "true"
      }
    }
  }
}`)

	res, err := Validate(context.Background(), ValidateOptions{
		ProjectDir:       projectDir,
		CandidateEnvFile: candidateEnvPath,
		Environ:          []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
		NewExecutor:      runner.factory(),
	})
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if res.Passed() {
		t.Fatal("expected Validate to fail when MTLS_ENFORCE=false")
	}
	cv := res.Evidence.Stages.CandidateValidate
	if cv == nil || !containsString(cv.Reasons, "caller_env_invalid:MTLS_ENFORCE") {
		t.Errorf("expected caller_env_invalid:MTLS_ENFORCE, got %+v", cv)
	}
	assertNoSecretsLeaked(t, res)
}

func TestValidate_ComposeConfigFailures(t *testing.T) {
	projectDir, candidateEnvPath, _, _, _ := setupFakeProject(t)

	t.Run("variable not set stderr fails with variable name", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.composeConfigStderr = "time=\"2026-11-06T00:00:00Z\" level=warning msg=\"The \\\"POSTGRES_PASSWORD\\\" variable is not set. Defaulting to a blank string. " + sentinelSecretStderr + "\"\n"

		res, err := Validate(context.Background(), ValidateOptions{
			ProjectDir:       projectDir,
			CandidateEnvFile: candidateEnvPath,
			Environ:          []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor:      runner.factory(),
		})
		if err != nil {
			t.Fatalf("Validate error: %v", err)
		}
		if res.Passed() {
			t.Fatal("expected Validate to fail when compose stderr warns variable is not set")
		}
		cv := res.Evidence.Stages.CandidateValidate
		if cv == nil || !reflect.DeepEqual(cv.Reasons, []string{"compose_unset_variable:POSTGRES_PASSWORD"}) {
			t.Errorf("reasons = %v, want [compose_unset_variable:POSTGRES_PASSWORD]", cv.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("compose config non-zero exit with valid stdout JSON still fails with compose_config_failed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.composeConfigStderr = "yaml: line 10: syntax error " + sentinelSecretStderr + "\n"
		runner.composeConfigErr = &fakeExitError{code: 15}

		res, err := Validate(context.Background(), ValidateOptions{
			ProjectDir:       projectDir,
			CandidateEnvFile: candidateEnvPath,
			Environ:          []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor:      runner.factory(),
		})
		if err != nil {
			t.Fatalf("Validate error: %v", err)
		}
		if res.Passed() {
			t.Fatal("expected Validate to fail on non-zero compose config exit")
		}
		cv := res.Evidence.Stages.CandidateValidate
		if cv == nil || !reflect.DeepEqual(cv.Reasons, []string{"compose_config_failed"}) {
			t.Errorf("reasons = %v, want [compose_config_failed]", cv.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})
}

func validCallerEnv() map[string]string {
	return map[string]string{
		"MTLS_ENFORCE":                  "true",
		"MTLS_CERT_FILE":                "/certs/svc-cert.pem",
		"MTLS_KEY_FILE":                 "/certs/svc-key.pem",
		"MTLS_CA_FILE":                  "/trust/ca-bundle.pem",
		"NEWS_CREATOR_HOST":             "https://news-creator:9443",
		"NEWS_CREATOR_MTLS_SERVER_NAME": "news-creator",
		"CERT_SUBJECT":                  "pre-processor",
	}
}

func validReceiverEnv() map[string]string {
	return map[string]string{
		"INBOUND_MTLS":         "true",
		"PEER_IDENTITY_STRICT": "true",
		"CERT_SANS":            "news-creator,localhost",
		"MTLS_ALLOWED_PEERS":   "recap-worker,pre-processor,rag-orchestrator",
	}
}

func TestCheckSemanticEdge_Table(t *testing.T) {
	if got := CheckSemanticEdge(validCallerEnv(), validReceiverEnv()); len(got) != 0 {
		t.Fatalf("expected valid baseline envs to pass, got %v", got)
	}
	for _, wantKey := range []string{"CERT_SUBJECT_FILE", "CERT_PATH_FILE", "KEY_PATH_FILE"} {
		if !containsString(CheckedKeys(), wantKey) {
			t.Errorf("CheckedKeys() = %v, missing %q", CheckedKeys(), wantKey)
		}
	}

	cases := []struct {
		name        string
		mutate      func(c, r map[string]string)
		wantReasons []string
	}{
		{
			name:        "MTLS_ENFORCE false",
			mutate:      func(c, _ map[string]string) { c["MTLS_ENFORCE"] = "false" },
			wantReasons: []string{"caller_env_invalid:MTLS_ENFORCE"},
		},
		{
			name:        "MTLS_ENFORCE empty",
			mutate:      func(c, _ map[string]string) { delete(c, "MTLS_ENFORCE") },
			wantReasons: []string{"caller_env_invalid:MTLS_ENFORCE"},
		},
		{
			name:        "MTLS_CERT_FILE wrong path",
			mutate:      func(c, _ map[string]string) { c["MTLS_CERT_FILE"] = "/etc/ssl/cert.pem" },
			wantReasons: []string{"caller_env_invalid:MTLS_CERT_FILE"},
		},
		{
			name:        "MTLS_KEY_FILE wrong path",
			mutate:      func(c, _ map[string]string) { c["MTLS_KEY_FILE"] = "/etc/ssl/key.pem" },
			wantReasons: []string{"caller_env_invalid:MTLS_KEY_FILE"},
		},
		{
			name:        "MTLS_CA_FILE wrong path",
			mutate:      func(c, _ map[string]string) { c["MTLS_CA_FILE"] = "/etc/ssl/ca.pem" },
			wantReasons: []string{"caller_env_invalid:MTLS_CA_FILE"},
		},
		{
			name:        "NEWS_CREATOR_HOST http scheme",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_HOST"] = "http://news-creator:9443" },
			wantReasons: []string{"caller_env_invalid:NEWS_CREATOR_HOST"},
		},
		{
			name:        "NEWS_CREATOR_HOST port 443",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_HOST"] = "https://news-creator:443" },
			wantReasons: []string{"caller_env_invalid:NEWS_CREATOR_HOST"},
		},
		{
			name:        "NEWS_CREATOR_HOST missing port",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_HOST"] = "https://news-creator" },
			wantReasons: []string{"caller_env_invalid:NEWS_CREATOR_HOST"},
		},
		{
			name:        "NEWS_CREATOR_HOST with path",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_HOST"] = "https://news-creator:9443/path" },
			wantReasons: []string{"caller_env_invalid:NEWS_CREATOR_HOST"},
		},
		{
			name:        "NEWS_CREATOR_HOST with userinfo",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_HOST"] = "https://user@news-creator:9443" },
			wantReasons: []string{"caller_env_invalid:NEWS_CREATOR_HOST"},
		},
		{
			name:        "NEWS_CREATOR_HOST with query",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_HOST"] = "https://news-creator:9443?x=1" },
			wantReasons: []string{"caller_env_invalid:NEWS_CREATOR_HOST"},
		},
		{
			name:        "NEWS_CREATOR_HOST with fragment",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_HOST"] = "https://news-creator:9443#frag" },
			wantReasons: []string{"caller_env_invalid:NEWS_CREATOR_HOST"},
		},
		{
			name:        "NEWS_CREATOR_HOST wrong host",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_HOST"] = "https://other-host:9443" },
			wantReasons: []string{"caller_env_invalid:NEWS_CREATOR_HOST"},
		},
		{
			name:        "SNI explicit value not in receiver CERT_SANS",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_MTLS_SERVER_NAME"] = "alt-news" },
			wantReasons: []string{"sni_not_in_receiver_sans:NEWS_CREATOR_MTLS_SERVER_NAME,CERT_SANS"},
		},
		{
			name:        "SNI padded with whitespace fails",
			mutate:      func(c, _ map[string]string) { c["NEWS_CREATOR_MTLS_SERVER_NAME"] = " news-creator " },
			wantReasons: []string{"sni_not_in_receiver_sans:NEWS_CREATOR_MTLS_SERVER_NAME,CERT_SANS"},
		},
		{
			name: "SNI default news-creator not in receiver CERT_SANS substring suffix",
			mutate: func(c, r map[string]string) {
				delete(c, "NEWS_CREATOR_MTLS_SERVER_NAME")
				r["CERT_SANS"] = "news-creator-x,localhost"
			},
			wantReasons: []string{"sni_not_in_receiver_sans:NEWS_CREATOR_MTLS_SERVER_NAME,CERT_SANS"},
		},
		{
			name: "SNI default news-creator not in receiver CERT_SANS substring prefix",
			mutate: func(c, r map[string]string) {
				delete(c, "NEWS_CREATOR_MTLS_SERVER_NAME")
				r["CERT_SANS"] = "xnews-creator"
			},
			wantReasons: []string{"sni_not_in_receiver_sans:NEWS_CREATOR_MTLS_SERVER_NAME,CERT_SANS"},
		},
		{
			name:        "INBOUND_MTLS false",
			mutate:      func(_, r map[string]string) { r["INBOUND_MTLS"] = "false" },
			wantReasons: []string{"receiver_env_invalid:INBOUND_MTLS"},
		},
		{
			name:        "PEER_IDENTITY_STRICT false",
			mutate:      func(_, r map[string]string) { r["PEER_IDENTITY_STRICT"] = "false" },
			wantReasons: []string{"receiver_env_invalid:PEER_IDENTITY_STRICT"},
		},
		{
			name:        "CERT_SUBJECT empty emits both subject and allowed-peers reasons",
			mutate:      func(c, _ map[string]string) { c["CERT_SUBJECT"] = "" },
			wantReasons: []string{"caller_env_invalid:CERT_SUBJECT", "caller_not_in_allowed_peers:CERT_SUBJECT,MTLS_ALLOWED_PEERS"},
		},
		{
			name:        "CERT_SUBJECT padded with whitespace fails",
			mutate:      func(c, _ map[string]string) { c["CERT_SUBJECT"] = " pre-processor " },
			wantReasons: []string{"caller_env_invalid:CERT_SUBJECT", "caller_not_in_allowed_peers:CERT_SUBJECT,MTLS_ALLOWED_PEERS"},
		},
		{
			name: "CERT_SUBJECT wrong service even if present in allowed peers",
			mutate: func(c, r map[string]string) {
				c["CERT_SUBJECT"] = "recap-worker"
				r["MTLS_ALLOWED_PEERS"] = "recap-worker,pre-processor"
			},
			wantReasons: []string{"caller_env_invalid:CERT_SUBJECT"},
		},
		{
			name:        "CERT_SUBJECT_FILE present with path fails",
			mutate:      func(c, _ map[string]string) { c["CERT_SUBJECT_FILE"] = "/run/secrets/subject" },
			wantReasons: []string{"caller_env_invalid:CERT_SUBJECT_FILE"},
		},
		{
			name:        "CERT_SUBJECT_FILE present with empty value fails",
			mutate:      func(c, _ map[string]string) { c["CERT_SUBJECT_FILE"] = "" },
			wantReasons: []string{"caller_env_invalid:CERT_SUBJECT_FILE"},
		},
		{
			name:        "CERT_PATH_FILE present with path fails",
			mutate:      func(c, _ map[string]string) { c["CERT_PATH_FILE"] = "/tmp/other-cert.pem" },
			wantReasons: []string{"caller_env_invalid:CERT_PATH_FILE"},
		},
		{
			name:        "CERT_PATH_FILE present with empty value fails",
			mutate:      func(c, _ map[string]string) { c["CERT_PATH_FILE"] = "" },
			wantReasons: []string{"caller_env_invalid:CERT_PATH_FILE"},
		},
		{
			name:        "KEY_PATH_FILE present with path fails",
			mutate:      func(c, _ map[string]string) { c["KEY_PATH_FILE"] = "/tmp/other-key.pem" },
			wantReasons: []string{"caller_env_invalid:KEY_PATH_FILE"},
		},
		{
			name:        "KEY_PATH_FILE present with empty value fails",
			mutate:      func(c, _ map[string]string) { c["KEY_PATH_FILE"] = "" },
			wantReasons: []string{"caller_env_invalid:KEY_PATH_FILE"},
		},
		{
			name:        "MTLS_ALLOWED_PEERS negative exact match pre-processor-x",
			mutate:      func(_, r map[string]string) { r["MTLS_ALLOWED_PEERS"] = "pre-processor-x" },
			wantReasons: []string{"caller_not_in_allowed_peers:CERT_SUBJECT,MTLS_ALLOWED_PEERS"},
		},
		{
			name:        "MTLS_ALLOWED_PEERS negative exact match xpre-processor",
			mutate:      func(_, r map[string]string) { r["MTLS_ALLOWED_PEERS"] = "xpre-processor" },
			wantReasons: []string{"caller_not_in_allowed_peers:CERT_SUBJECT,MTLS_ALLOWED_PEERS"},
		},
		{
			name:        "MTLS_ALLOWED_PEERS negative exact match pre-processor-x,other",
			mutate:      func(_, r map[string]string) { r["MTLS_ALLOWED_PEERS"] = "pre-processor-x,other" },
			wantReasons: []string{"caller_not_in_allowed_peers:CERT_SUBJECT,MTLS_ALLOWED_PEERS"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validCallerEnv()
			r := validReceiverEnv()
			tc.mutate(c, r)
			got := CheckSemanticEdge(c, r)
			if !reflect.DeepEqual(got, tc.wantReasons) {
				t.Errorf("CheckSemanticEdge() = %v, want %v", got, tc.wantReasons)
			}
		})
	}
}

func TestCollectComposeReferencedVars_ScalarPatterns(t *testing.T) {
	t.Run("scalar and sequence include paths plus string extends", func(t *testing.T) {
		dir := t.TempDir()
		rootFile := filepath.Join(dir, "compose.yaml")
		incFile := filepath.Join(dir, "inc.yaml")
		singleFile := filepath.Join(dir, "single.yaml")
		seqFile := filepath.Join(dir, "seq.yaml")

		rootYAML := `# comment with ${COMMENT_VAR_IGNORED} and $COMMENT_UNBRACED_IGNORED
name: alt
include:
  - inc.yaml
  - path: single.yaml
  - path:
      - seq.yaml
services:
  local_svc:
    extends: base_svc
    environment:
      - ESCAPED=$$ESCAPED_ONE/$${ESCAPED_TWO}
      - NESTED=${OUTER_VAR:-${INNER_VAR}}
`
		incYAML := `services:
  inc_svc:
    environment:
      - REQUIRED=${REQ_VAR:?missing required var}
      - SHORT_DEF=${SHORT_VAR-fallback}
`
		singleYAML := `services:
  single_svc:
    image: repo/single:${SINGLE_PATH_VAR:-latest}
`
		seqYAML := `services:
  seq_svc:
    image: repo/img:$UNBRACED_VAR
`
		if err := os.WriteFile(rootFile, []byte(rootYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(incFile, []byte(incYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(singleFile, []byte(singleYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(seqFile, []byte(seqYAML), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := CollectComposeReferencedVars(rootFile)
		if err != nil {
			t.Fatalf("CollectComposeReferencedVars error: %v", err)
		}
		want := []string{
			"INNER_VAR",
			"OUTER_VAR",
			"REQ_VAR",
			"SHORT_VAR",
			"SINGLE_PATH_VAR",
			"UNBRACED_VAR",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("CollectComposeReferencedVars = %v, want %v", got, want)
		}
	})

	t.Run("include mapping with unknown key fails closed", func(t *testing.T) {
		dir := t.TempDir()
		rootFile := filepath.Join(dir, "compose.yaml")
		rootYAML := `name: alt
include:
  - unknown_key: other.yaml
services: {}
`
		if err := os.WriteFile(rootFile, []byte(rootYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := CollectComposeReferencedVars(rootFile); err == nil {
			t.Fatal("expected error for include mapping without path, got nil")
		}
	})

	t.Run("include mapping with non-string sequence item fails closed", func(t *testing.T) {
		dir := t.TempDir()
		rootFile := filepath.Join(dir, "compose.yaml")
		okFile := filepath.Join(dir, "ok.yaml")
		rootYAML := `name: alt
include:
  - path:
      - ok.yaml
      - nested: invalid
services: {}
`
		if err := os.WriteFile(rootFile, []byte(rootYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(okFile, []byte("services: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := CollectComposeReferencedVars(rootFile); err == nil {
			t.Fatal("expected error for include path sequence decode failure, got nil")
		}
	})

	t.Run("service with extends.file fails closed naming compose file", func(t *testing.T) {
		dir := t.TempDir()
		rootFile := filepath.Join(dir, "compose.yaml")
		incFile := filepath.Join(dir, "inc.yaml")
		extFile := filepath.Join(dir, "ext.yaml")
		if err := os.WriteFile(rootFile, []byte("name: alt\ninclude:\n  - inc.yaml\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		incYAML := `services:
  inc_svc:
    extends:
      file: ext.yaml
      service: ext_svc
`
		if err := os.WriteFile(incFile, []byte(incYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(extFile, []byte("services:\n  ext_svc:\n    image: repo/img:latest\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := CollectComposeReferencedVars(rootFile)
		if err == nil {
			t.Fatal("expected error when service uses extends.file, got nil")
		}
		if !strings.Contains(err.Error(), "inc.yaml") {
			t.Errorf("error %q does not name compose file inc.yaml", err.Error())
		}
	})
}

func TestObserve_BaselinePassesAndPinsExactArgvAndFrozenEnv(t *testing.T) {
	projectDir, _, _, _, _ := setupFakeProject(t)
	runner := newBaselineFakeRunner(t)

	res, err := Observe(context.Background(), ObserveOptions{
		ProjectDir:          projectDir,
		ExpectCallerImage:   expectedCallerDigest,
		ExpectReceiverImage: expectedReceiverImage,
		Environ:             baselineInputEnviron,
		NewExecutor:         runner.factory(),
	})
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if !res.Passed() {
		t.Fatalf("expected Observe baseline to pass, got %+v", res.Evidence)
	}
	if !res.Evidence.RolloutSucceeded {
		t.Errorf("expected rollout_succeeded = true")
	}
	lo := res.Evidence.Stages.LiveObservation
	if lo == nil || lo.Status != StatusPass || lo.Probe != StatusPass {
		t.Fatalf("unexpected LiveObservation: %+v", lo)
	}
	if lo.CallerImageDigest != expectedCallerDigest {
		t.Errorf("caller_image_digest = %q, want %q", lo.CallerImageDigest, expectedCallerDigest)
	}
	if lo.ReceiverImageDigest != expectedReceiverImage {
		t.Errorf("receiver_image_digest = %q, want %q", lo.ReceiverImageDigest, expectedReceiverImage)
	}

	wantComposeFile := filepath.Join(projectDir, "compose", "compose.yaml")
	wantCalls := [][]string{
		{"info", "--format", "{{json .ServerVersion}}"},
		{"context", "show"},
		{"compose", "version", "--short"},
		{"ps", "-q", "--filter", "label=com.docker.compose.project=alt", "--filter", "label=com.docker.compose.service=pre-processor", "--filter", "status=running"},
		{"container", "inspect", "1111111111111111111111111111111111111111111111111111111111111111"},
		{"ps", "-q", "--filter", "label=com.docker.compose.project=alt", "--filter", "label=com.docker.compose.service=news-creator", "--filter", "status=running"},
		{"container", "inspect", "2222222222222222222222222222222222222222222222222222222222222222"},
		{"image", "inspect", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"image", "inspect", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{"compose", "-f", wantComposeFile, "-p", "alt", "config", "--format", "json"},
		{
			"run",
			"--rm",
			"--pull", "never",
			"--network", "container:1111111111111111111111111111111111111111111111111111111111111111",
			"--user", "65532:65532",
			"--mount", "type=volume,src=alt_pre_processor_certs,dst=/certs,ro",
			"--mount", "type=volume,src=alt_pki_trust_bundle,dst=/trust,ro",
			"--entrypoint", "curl",
			"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"-sS",
			"-o", "/dev/null",
			"--max-time", "10",
			"--fail",
			"--cert", "/certs/svc-cert.pem",
			"--key", "/certs/svc-key.pem",
			"--cacert", "/trust/ca-bundle.pem",
			"https://news-creator:9443/queue/status",
		},
	}

	if len(runner.calls) != len(wantCalls) {
		t.Fatalf("Observe call count = %d, want %d", len(runner.calls), len(wantCalls))
	}
	for i, call := range runner.calls {
		if call.Cmd != "docker" {
			t.Errorf("call[%d] cmd = %q, want docker", i, call.Cmd)
		}
		if !reflect.DeepEqual(call.Args, wantCalls[i]) {
			t.Errorf("call[%d] argv = %v\nwant          %v", i, call.Args, wantCalls[i])
		}
		if !reflect.DeepEqual(call.Env, expectedFrozenChildEnv) {
			t.Errorf("call[%d] env = %v, want %v", i, call.Env, expectedFrozenChildEnv)
		}
	}
	assertNoSecretsLeaked(t, res)
}

func TestObserve_ShellOverrideFailsBeforeDocker(t *testing.T) {
	projectDir, _, _, _, _ := setupFakeProject(t)
	runner := newBaselineFakeRunner(t)

	res, err := Observe(context.Background(), ObserveOptions{
		ProjectDir: projectDir,
		Environ: []string{
			"PATH=/usr/bin:/bin",
			"HOME=/tmp",
			"IMAGE_TAG=leaky-tag",
			"NEWS_CREATOR_MTLS_ALLOWED_PEERS=" + sentinelSecretEnv,
		},
		NewExecutor: runner.factory(),
	})
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if res.Passed() || res.Evidence.RolloutSucceeded {
		t.Fatal("expected Observe to fail environment freeze on shell override")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected 0 docker calls when environment freeze fails in Observe, got %d", len(runner.calls))
	}
	lo := res.Evidence.Stages.LiveObservation
	wantReasons := []string{
		"shell_env_override:IMAGE_TAG",
		"shell_env_override:NEWS_CREATOR_MTLS_ALLOWED_PEERS",
	}
	if lo == nil || !reflect.DeepEqual(lo.Reasons, wantReasons) {
		t.Errorf("reasons = %v, want %v", lo.Reasons, wantReasons)
	}
	if lo.CallerImageDigest != StatusNotObserved || lo.ReceiverImageDigest != StatusNotObserved {
		t.Errorf("expected not_observed digests on early return, got caller=%q receiver=%q", lo.CallerImageDigest, lo.ReceiverImageDigest)
	}
	assertNoSecretsLeaked(t, res)
}

func TestObserve_SemanticCheckFailsWhenRenderedAndContainerAgreeOnInvalidValue(t *testing.T) {
	projectDir, _, _, _, _ := setupFakeProject(t)
	runner := newBaselineFakeRunner(t)
	runner.composeConfigStdout = []byte(`{
  "name": "alt",
  "services": {
    "pre-processor": {
      "environment": {
        "CERT_SUBJECT": "pre-processor",
        "MTLS_CA_FILE": "/trust/ca-bundle.pem",
        "MTLS_CERT_FILE": "/certs/svc-cert.pem",
        "MTLS_ENFORCE": "false",
        "MTLS_KEY_FILE": "/certs/svc-key.pem",
        "NEWS_CREATOR_HOST": "https://news-creator:9443",
        "NEWS_CREATOR_MTLS_SERVER_NAME": "news-creator"
      }
    },
    "news-creator": {
      "environment": {
        "CERT_SANS": "news-creator,localhost",
        "INBOUND_MTLS": "true",
        "MTLS_ALLOWED_PEERS": "recap-worker,acolyte-orchestrator,rag-orchestrator,recap-evaluator,pre-processor",
        "PEER_IDENTITY_STRICT": "true"
      }
    }
  }
}`)
	runner.callerInspectJSON = []byte(`[
  {
    "Id": "1111111111111111111111111111111111111111111111111111111111111111",
    "Image": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "Config": {
      "Env": [
        "CERT_SUBJECT=pre-processor",
        "MTLS_CA_FILE=/trust/ca-bundle.pem",
        "MTLS_CERT_FILE=/certs/svc-cert.pem",
        "MTLS_ENFORCE=false",
        "MTLS_KEY_FILE=/certs/svc-key.pem",
        "NEWS_CREATOR_HOST=https://news-creator:9443",
        "NEWS_CREATOR_MTLS_SERVER_NAME=news-creator"
      ]
    },
    "Mounts": [
      { "Type": "volume", "Name": "alt_pre_processor_certs", "Destination": "/certs" },
      { "Type": "volume", "Name": "alt_pki_trust_bundle", "Destination": "/trust" }
    ]
  }
]`)

	res, err := Observe(context.Background(), ObserveOptions{
		ProjectDir:  projectDir,
		Environ:     baselineInputEnviron,
		NewExecutor: runner.factory(),
	})
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if res.Passed() || res.Evidence.RolloutSucceeded {
		t.Fatal("expected Observe to fail semantic check when both rendered and container env have MTLS_ENFORCE=false")
	}
	lo := res.Evidence.Stages.LiveObservation
	if lo == nil || lo.Status != StatusFail {
		t.Fatalf("expected status=fail, got %+v", lo)
	}
	if len(lo.EnvDriftKeys) != 0 {
		t.Errorf("expected no env drift keys, got %v", lo.EnvDriftKeys)
	}
	if !reflect.DeepEqual(lo.Reasons, []string{"caller_env_invalid:MTLS_ENFORCE"}) {
		t.Errorf("reasons = %v, want [caller_env_invalid:MTLS_ENFORCE]", lo.Reasons)
	}
	assertNoSecretsLeaked(t, res)
}

func TestObserve_ComposeConfigFailures(t *testing.T) {
	projectDir, _, _, _, _ := setupFakeProject(t)

	t.Run("variable not set stderr fails with variable name", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.composeConfigStderr = "level=warning msg=\"The \\\"POSTGRES_PASSWORD\\\" variable is not set.\" " + sentinelSecretStderr + "\n"

		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     baselineInputEnviron,
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected Observe to fail when compose config warns variable is not set")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || !reflect.DeepEqual(lo.Reasons, []string{"compose_unset_variable:POSTGRES_PASSWORD"}) {
			t.Errorf("reasons = %v, want [compose_unset_variable:POSTGRES_PASSWORD]", lo.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("compose config non-zero exit with valid stdout JSON fails with compose_config_failed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.composeConfigStderr = "compose failed " + sentinelSecretStderr + "\n"
		runner.composeConfigErr = &fakeExitError{code: 1}

		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     baselineInputEnviron,
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected Observe to fail when compose config exits non-zero")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || !reflect.DeepEqual(lo.Reasons, []string{"compose_config_failed"}) {
			t.Errorf("reasons = %v, want [compose_config_failed]", lo.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})
}

func TestObserve_StaleContainerDigestAndEnvDrift(t *testing.T) {
	projectDir, _, _, _, _ := setupFakeProject(t)

	t.Run("caller digest mismatch", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:        projectDir,
			ExpectCallerImage: "ghcr.io/example/pre-processor@sha256:9999999999999999999999999999999999999999999999999999999999999999",
			Environ:           []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor:       runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected Observe to fail on caller digest mismatch")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || !containsString(lo.Reasons, "caller_image_digest_mismatch") {
			t.Errorf("expected caller_image_digest_mismatch, got %+v", lo)
		}
		if lo.CallerImageDigest != expectedCallerDigest {
			t.Errorf("caller_image_digest = %q, want observed digest %q", lo.CallerImageDigest, expectedCallerDigest)
		}
		if lo.ReceiverImageDigest != expectedReceiverImage {
			t.Errorf("receiver_image_digest = %q, want observed digest %q", lo.ReceiverImageDigest, expectedReceiverImage)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("receiver digest mismatch", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:          projectDir,
			ExpectCallerImage:   expectedCallerDigest,
			ExpectReceiverImage: "ghcr.io/example/news-creator@sha256:8888888888888888888888888888888888888888888888888888888888888888",
			Environ:             []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor:         runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected Observe to fail on receiver digest mismatch")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || !containsString(lo.Reasons, "receiver_image_digest_mismatch") {
			t.Errorf("expected receiver_image_digest_mismatch, got %+v", lo)
		}
		if lo.CallerImageDigest != expectedCallerDigest {
			t.Errorf("caller_image_digest = %q, want observed digest %q", lo.CallerImageDigest, expectedCallerDigest)
		}
		if lo.ReceiverImageDigest != expectedReceiverImage {
			t.Errorf("receiver_image_digest = %q, want observed digest %q", lo.ReceiverImageDigest, expectedReceiverImage)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("caller container env differs from rendered compose env", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerInspectJSON = []byte(`[
  {
    "Id": "1111111111111111111111111111111111111111111111111111111111111111",
    "Image": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "Config": {
      "Env": [
        "CERT_SUBJECT=pre-processor",
        "MTLS_CA_FILE=/trust/ca-bundle.pem",
        "MTLS_CERT_FILE=/certs/svc-cert.pem",
        "MTLS_ENFORCE=true",
        "MTLS_KEY_FILE=/certs/svc-key.pem",
        "NEWS_CREATOR_HOST=https://news-creator:9443",
        "NEWS_CREATOR_MTLS_SERVER_NAME=drifted-sni",
        "DB_PASSWORD=` + sentinelSecretDB + `"
      ]
    },
    "Mounts": [
      { "Type": "volume", "Name": "alt_pre_processor_certs", "Destination": "/certs" },
      { "Type": "volume", "Name": "alt_pki_trust_bundle", "Destination": "/trust" }
    ]
  }
]`)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected Observe to fail on caller container env drift")
		}
		lo := res.Evidence.Stages.LiveObservation
		if !containsString(lo.EnvDriftKeys, "NEWS_CREATOR_MTLS_SERVER_NAME") {
			t.Errorf("expected NEWS_CREATOR_MTLS_SERVER_NAME in env_drift_keys, got %v", lo.EnvDriftKeys)
		}
		if !containsString(lo.Reasons, "env_drift:NEWS_CREATOR_MTLS_SERVER_NAME") {
			t.Errorf("expected env_drift:NEWS_CREATOR_MTLS_SERVER_NAME, got %v", lo.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("receiver container env differs from rendered compose env", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.receiverInspectJSON = []byte(`[
  {
    "Id": "2222222222222222222222222222222222222222222222222222222222222222",
    "Image": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "Config": {
      "Env": [
        "CERT_SANS=news-creator,localhost",
        "INBOUND_MTLS=true",
        "MTLS_ALLOWED_PEERS=recap-worker,acolyte-orchestrator",
        "PEER_IDENTITY_STRICT=true",
        "BACKEND_TOKEN_SECRET=` + sentinelSecretJWT + `"
      ]
    },
    "Mounts": []
  }
]`)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected Observe to fail on receiver container env drift")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil {
			t.Fatal("expected LiveObservation stage")
		}
		if !containsString(lo.EnvDriftKeys, "MTLS_ALLOWED_PEERS") {
			t.Errorf("expected MTLS_ALLOWED_PEERS in env_drift_keys, got %v", lo.EnvDriftKeys)
		}
		if !containsString(lo.Reasons, "env_drift:MTLS_ALLOWED_PEERS") {
			t.Errorf("expected env_drift:MTLS_ALLOWED_PEERS, got %v", lo.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})
}

func TestObserve_ProbeContainerAndImageFailures(t *testing.T) {
	projectDir, _, _, _, _ := setupFakeProject(t)

	t.Run("probe HTTP error exit 22", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.probeStderr = "curl: (22) The requested URL returned error: 403 " + sentinelSecretStderr
		runner.probeErr = &fakeExitError{code: 22}

		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected fail on probe exit 22")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || lo.Status != StatusFail || lo.Probe != StatusFail {
			t.Fatalf("expected status=fail probe=fail, got %+v", lo)
		}
		if !containsString(lo.Reasons, "probe_http_error") {
			t.Errorf("expected reason probe_http_error, got %v", lo.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("probe TLS error exit 60", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.probeStderr = "curl: (60) SSL certificate problem " + sentinelSecretStderr
		runner.probeErr = &fakeExitError{code: 60}

		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected fail on probe exit 60")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || lo.Status != StatusFail || lo.Probe != StatusFail {
			t.Fatalf("expected status=fail probe=fail, got %+v", lo)
		}
		if !containsString(lo.Reasons, "probe_tls_or_connect_error") {
			t.Errorf("expected reason probe_tls_or_connect_error, got %v", lo.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("probe container cannot start exit 125", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.probeStderr = "docker: Error response from daemon " + sentinelSecretStderr
		runner.probeErr = &fakeExitError{code: 125}

		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected not_observed to not count as pass")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || lo.Status != StatusNotObserved || lo.Probe != StatusNotObserved {
			t.Fatalf("expected status=not_observed probe=not_observed, got %+v", lo)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("mixed hard fail and probe container error returns status fail", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.probeErr = &fakeExitError{code: 125}
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:        projectDir,
			ExpectCallerImage: "ghcr.io/example/pre-processor@sha256:9999999999999999999999999999999999999999999999999999999999999999",
			Environ:           []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor:       runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || lo.Status != StatusFail || lo.Probe != StatusNotObserved {
			t.Fatalf("expected status=fail probe=not_observed, got %+v", lo)
		}
	})

	t.Run("missing /certs mount fails closed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerInspectJSON = []byte(`[
  {
    "Id": "1111111111111111111111111111111111111111111111111111111111111111",
    "Image": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "Config": {
      "Env": [
        "CERT_SUBJECT=pre-processor",
        "MTLS_CA_FILE=/trust/ca-bundle.pem",
        "MTLS_CERT_FILE=/certs/svc-cert.pem",
        "MTLS_ENFORCE=true",
        "MTLS_KEY_FILE=/certs/svc-key.pem",
        "NEWS_CREATOR_HOST=https://news-creator:9443",
        "NEWS_CREATOR_MTLS_SERVER_NAME=news-creator",
        "DB_PASSWORD=` + sentinelSecretDB + `"
      ]
    },
    "Mounts": [
      {
        "Type": "volume",
        "Name": "alt_pki_trust_bundle",
        "Destination": "/trust"
      }
    ]
  }
]`)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected missing /certs mount to fail closed")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || lo.Status != StatusFail {
			t.Fatalf("expected status=fail, got %+v", lo)
		}
		if !containsString(lo.Reasons, "caller_mount_missing:/certs,/trust") {
			t.Errorf("expected caller_mount_missing:/certs,/trust, got %v", lo.Reasons)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("zero running containers returns not_observed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerContainerIDs = ""
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected not_observed when 0 caller containers are running")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo.Status != StatusNotObserved || !containsString(lo.Reasons, "caller_container_not_single_running") {
			t.Errorf("unexpected LiveObservation: %+v", lo)
		}
		if lo.CallerImageDigest != StatusNotObserved || lo.ReceiverImageDigest != StatusNotObserved {
			t.Errorf("expected not_observed digests, got caller=%q receiver=%q", lo.CallerImageDigest, lo.ReceiverImageDigest)
		}
	})

	t.Run("multiple running containers returns not_observed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerContainerIDs = "1111\n3333\n"
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected not_observed when 2 caller containers are running")
		}
		if res.Evidence.Stages.LiveObservation.Status != StatusNotObserved {
			t.Errorf("status = %q, want %q", res.Evidence.Stages.LiveObservation.Status, StatusNotObserved)
		}
	})

	t.Run("empty container Id from inspect fails closed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerInspectJSON = []byte(`[{"Id":"","Image":"sha256:aaaa"}]`)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo == nil || lo.Status != StatusFail || !containsString(lo.Reasons, "caller_container_missing_id") {
			t.Fatalf("expected status=fail with caller_container_missing_id, got %+v", lo)
		}
		if lo.CallerImageDigest != StatusNotObserved || lo.ReceiverImageDigest != StatusNotObserved {
			t.Errorf("expected not_observed digests, got caller=%q receiver=%q", lo.CallerImageDigest, lo.ReceiverImageDigest)
		}
	})

	t.Run("image inspect command error fails closed with inspect_failed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerImageErr = errors.New("inspect failed")
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if !containsString(res.Evidence.Stages.LiveObservation.Reasons, "caller_image_inspect_failed") {
			t.Errorf("expected caller_image_inspect_failed, got %v", res.Evidence.Stages.LiveObservation.Reasons)
		}
	})

	t.Run("image inspect invalid JSON fails closed with parse_failed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerImageJSON = []byte(`not-json`)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if !containsString(res.Evidence.Stages.LiveObservation.Reasons, "caller_image_inspect_parse_failed") {
			t.Errorf("expected caller_image_inspect_parse_failed, got %v", res.Evidence.Stages.LiveObservation.Reasons)
		}
	})

	t.Run("missing RepoDigests fails closed with no_repodigest", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerImageJSON = []byte(`[{"Id":"sha256:aaaa","RepoDigests":[]}]`)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected missing RepoDigests to fail closed")
		}
		if !containsString(res.Evidence.Stages.LiveObservation.Reasons, "caller_image_no_repodigest") {
			t.Errorf("expected caller_image_no_repodigest, got %v", res.Evidence.Stages.LiveObservation.Reasons)
		}
	})

	t.Run("empty container Image field ends in fail", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.callerInspectJSON = []byte(`[{"Id":"1111111111111111111111111111111111111111111111111111111111111111","Image":""}]`)
		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected empty container Image to end in fail, not pass")
		}
		if res.Evidence.Stages.LiveObservation.Status != StatusFail {
			t.Errorf("status = %q, want %q", res.Evidence.Stages.LiveObservation.Status, StatusFail)
		}
	})
}

func TestDockerUnreachableAndUnobservedContext(t *testing.T) {
	projectDir, candidateEnvPath, _, _, _ := setupFakeProject(t)

	t.Run("validate docker unreachable", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.dockerUnreachable = true

		res, err := Validate(context.Background(), ValidateOptions{
			ProjectDir:       projectDir,
			CandidateEnvFile: candidateEnvPath,
			Environ:          []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor:      runner.factory(),
		})
		if err != nil {
			t.Fatalf("Validate error: %v", err)
		}
		if res.DockerReachable {
			t.Fatal("expected DockerReachable = false")
		}
		if res.Passed() {
			t.Fatal("expected Passed() = false when docker unreachable")
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("observe docker unreachable", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.dockerUnreachable = true

		res, err := Observe(context.Background(), ObserveOptions{
			ProjectDir:  projectDir,
			Environ:     []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor: runner.factory(),
		})
		if err != nil {
			t.Fatalf("Observe error: %v", err)
		}
		if res.DockerReachable {
			t.Fatal("expected DockerReachable = false")
		}
		if res.Passed() || res.Evidence.RolloutSucceeded {
			t.Fatal("expected Passed() = false when docker unreachable")
		}
		lo := res.Evidence.Stages.LiveObservation
		if lo.CallerImageDigest != StatusNotObserved || lo.ReceiverImageDigest != StatusNotObserved {
			t.Errorf("expected not_observed digests on unreachable, got caller=%q receiver=%q", lo.CallerImageDigest, lo.ReceiverImageDigest)
		}
		assertNoSecretsLeaked(t, res)
	})

	t.Run("unresolved docker context and compose version report not_observed", func(t *testing.T) {
		runner := newBaselineFakeRunner(t)
		runner.contextErr = errors.New("context error")
		runner.composeVerErr = errors.New("version error")

		res, err := Validate(context.Background(), ValidateOptions{
			ProjectDir:       projectDir,
			CandidateEnvFile: candidateEnvPath,
			Environ:          []string{"PATH=/usr/bin:/bin", "HOME=/tmp"},
			NewExecutor:      runner.factory(),
		})
		if err != nil {
			t.Fatalf("Validate error: %v", err)
		}
		if res.Evidence.DockerContext != StatusNotObserved {
			t.Errorf("docker_context = %q, want %q", res.Evidence.DockerContext, StatusNotObserved)
		}
		if res.Evidence.ToolVersions.DockerCompose != StatusNotObserved {
			t.Errorf("tool_versions.docker_compose = %q, want %q", res.Evidence.ToolVersions.DockerCompose, StatusNotObserved)
		}
	})
}

func TestDefaultExecutorFactory_ExactEnvAndDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMPOSE_FILE", "x")
	exec := DefaultExecutorFactory(dir, []string{"A=1"})

	var envOut, envErr bytes.Buffer
	if err := exec.RunWithPipes(context.Background(), "/usr/bin/env", nil, &envOut, &envErr); err != nil {
		t.Fatalf("/usr/bin/env failed: %v (stderr: %s)", err, envErr.String())
	}
	if got := strings.TrimSpace(envOut.String()); got != "A=1" {
		t.Errorf("/usr/bin/env output = %q, want %q", got, "A=1")
	}

	var pwdOut, pwdErr bytes.Buffer
	if err := exec.RunWithPipes(context.Background(), "/bin/pwd", nil, &pwdOut, &pwdErr); err != nil {
		t.Fatalf("/bin/pwd failed: %v (stderr: %s)", err, pwdErr.String())
	}
	if got := strings.TrimSpace(pwdOut.String()); got != dir {
		t.Errorf("/bin/pwd output = %q, want %q", got, dir)
	}
}

func TestValidateAndObserve_RelativeProjectDir(t *testing.T) {
	projectDir, candidateEnvPath, _, _, _ := setupFakeProject(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relProjectDir, err := filepath.Rel(cwd, projectDir)
	if err != nil {
		t.Fatal(err)
	}

	valRunner := newBaselineFakeRunner(t)
	valRes, err := Validate(context.Background(), ValidateOptions{
		ProjectDir:       relProjectDir,
		CandidateEnvFile: candidateEnvPath,
		Environ:          baselineInputEnviron,
		NewExecutor:      valRunner.factory(),
	})
	if err != nil {
		t.Fatalf("Validate with relative ProjectDir error: %v", err)
	}
	if !valRes.Passed() {
		t.Fatalf("expected Validate with relative ProjectDir to pass, got %+v", valRes.Evidence)
	}
	if len(valRunner.calls) == 0 || valRunner.calls[0].WorkDir != projectDir {
		t.Errorf("Validate root executor WorkDir = %q, want absolute %q", valRunner.calls[0].WorkDir, projectDir)
	}

	obsRunner := newBaselineFakeRunner(t)
	obsRes, err := Observe(context.Background(), ObserveOptions{
		ProjectDir:  relProjectDir,
		Environ:     baselineInputEnviron,
		NewExecutor: obsRunner.factory(),
	})
	if err != nil {
		t.Fatalf("Observe with relative ProjectDir error: %v", err)
	}
	if !obsRes.Passed() {
		t.Fatalf("expected Observe with relative ProjectDir to pass, got %+v", obsRes.Evidence)
	}
	wantComposeFile := filepath.Join(projectDir, "compose", "compose.yaml")
	foundCompose := false
	for _, call := range obsRunner.calls {
		if call.WorkDir != projectDir {
			t.Errorf("Observe call %v WorkDir = %q, want absolute %q", call.Args, call.WorkDir, projectDir)
		}
		if len(call.Args) >= 3 && call.Args[0] == "compose" && call.Args[1] == "-f" {
			foundCompose = true
			if call.Args[2] != wantComposeFile {
				t.Errorf("Observe compose -f = %q, want absolute %q", call.Args[2], wantComposeFile)
			}
		}
	}
	if !foundCompose {
		t.Fatal("expected Observe compose config call")
	}
}

func containsString(items []string, target string) bool {
	for _, it := range items {
		if it == target {
			return true
		}
	}
	return false
}
