package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const provisionScript = "../scripts/provision-consumer-group.sh"

// fakeRedisCLI records its argv and REDISCLI_AUTH so the test can see how the
// provisioning script authenticates without a live redis-streams.
const fakeRedisCLI = `#!/usr/bin/env bash
printf '%s\n' "$@" > "${FAKE_REDIS_CLI_ARGS}"
printf '%s' "${REDISCLI_AUTH:-}" > "${FAKE_REDIS_CLI_AUTH}"
echo OK
`

type provisionRun struct {
	output string
	err    error
	args   []string
	auth   string
	called bool
}

func runProvisionScript(t *testing.T, env map[string]string, args ...string) provisionRun {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required to exercise the provisioning script")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "redis-cli"), []byte(fakeRedisCLI), 0o700); err != nil { // #nosec G306 -- the fake has to be executable
		t.Fatal(err)
	}
	argsFile := filepath.Join(dir, "args")
	authFile := filepath.Join(dir, "auth")

	cmd := exec.Command("bash", append([]string{provisionScript}, args...)...) // #nosec G204 -- runs this repo's own script
	cmd.Env = []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"FAKE_REDIS_CLI_ARGS=" + argsFile,
		"FAKE_REDIS_CLI_AUTH=" + authFile,
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()

	run := provisionRun{output: string(out), err: err}
	if rawArgs, readErr := os.ReadFile(argsFile); readErr == nil { // #nosec G304 -- path under t.TempDir()
		run.called = true
		run.args = strings.Split(strings.TrimRight(string(rawArgs), "\n"), "\n")
	}
	if rawAuth, readErr := os.ReadFile(authFile); readErr == nil { // #nosec G304 -- path under t.TempDir()
		run.auth = string(rawAuth)
	}
	return run
}

func writePasswordFile(t *testing.T, password string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "redis_streams_password")
	if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hasPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

// redis-streams disables the default user; only the `streams` ACL user can
// create the consumer group, and its password must never be echoed or put on
// redis-cli's command line.
func assertAuthenticatedAsStreams(t *testing.T, run provisionRun, password string) {
	t.Helper()
	if run.err != nil {
		t.Fatalf("script failed: %v\n%s", run.err, run.output)
	}
	if !run.called {
		t.Fatalf("redis-cli was never invoked:\n%s", run.output)
	}
	if !hasPair(run.args, "--user", "streams") {
		t.Errorf("redis-cli must authenticate as the streams ACL user, got args %q", run.args)
	}
	if run.auth != password {
		t.Errorf("REDISCLI_AUTH must carry the password file's content")
	}
	if strings.Contains(strings.Join(run.args, " "), password) {
		t.Errorf("the password must not appear on redis-cli's command line")
	}
	if strings.Contains(run.output, password) {
		t.Errorf("the password must never be echoed, got output %q", run.output)
	}
}

func TestProvisionScript_AuthenticatesAsStreamsUser(t *testing.T) {
	password := "streams-password-value"

	run := runProvisionScript(t, map[string]string{
		"REDIS_URL":           "redis://redis-streams:6379",
		"REDIS_PASSWORD_FILE": writePasswordFile(t, password),
	})

	assertAuthenticatedAsStreams(t, run, password)
	if !hasPair(run.args, "-h", "redis-streams") || !hasPair(run.args, "-p", "6379") {
		t.Errorf("redis-cli must target redis-streams:6379, got args %q", run.args)
	}
}

// compose writes the user into the URL (redis://streams@redis-streams:6379).
// redis-cli -u reads a bare userinfo as the password, so the script must not
// hand that URL to redis-cli as-is.
func TestProvisionScript_UserInURLIsNotAPassword(t *testing.T) {
	password := "streams-password-value"

	run := runProvisionScript(t, map[string]string{
		"REDIS_URL":           "redis://streams@redis-streams:6379",
		"REDIS_PASSWORD_FILE": writePasswordFile(t, password),
	})

	assertAuthenticatedAsStreams(t, run, password)
	for _, arg := range run.args {
		if strings.Contains(arg, "@") {
			t.Errorf("redis-cli must not receive a URL with userinfo, got %q", arg)
		}
	}
}

func TestProvisionScript_PasswordFileArgument(t *testing.T) {
	password := "streams-password-value"

	run := runProvisionScript(t, map[string]string{
		"REDIS_URL": "redis://redis-streams:6379",
	}, "--password-file", writePasswordFile(t, password))

	assertAuthenticatedAsStreams(t, run, password)
}

func TestProvisionScript_MissingPasswordFileFailsBeforeRedis(t *testing.T) {
	run := runProvisionScript(t, map[string]string{
		"REDIS_URL": "redis://redis-streams:6379",
	})

	if run.err == nil {
		t.Fatalf("script must fail without a password file, got output %q", run.output)
	}
	if run.called {
		t.Errorf("redis-cli must not be invoked unauthenticated, got args %q", run.args)
	}
	if !strings.Contains(run.output, "REDIS_PASSWORD_FILE") {
		t.Errorf("the error must name REDIS_PASSWORD_FILE, got %q", run.output)
	}
}

func TestProvisionScript_UnreadablePasswordFileFails(t *testing.T) {
	run := runProvisionScript(t, map[string]string{
		"REDIS_URL":           "redis://redis-streams:6379",
		"REDIS_PASSWORD_FILE": filepath.Join(t.TempDir(), "missing"),
	})

	if run.err == nil {
		t.Fatalf("script must fail on an unreadable password file, got output %q", run.output)
	}
	if run.called {
		t.Errorf("redis-cli must not be invoked, got args %q", run.args)
	}
}
