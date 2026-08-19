package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"alt/internal/pki"
)

func cohortMainFiles(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	appRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return []string{
		filepath.Join(appRoot, "cmd", "backend", "main.go"),
		filepath.Join(appRoot, "cmd", "harvester", "main.go"),
		filepath.Join(appRoot, "cmd", "datahub", "main.go"),
	}
}

func TestCohortMainsWireStartEnrollment(t *testing.T) {
	for _, path := range cohortMainFiles(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "bootstrap.StartEnrollment") {
			t.Errorf("%s does not call bootstrap.StartEnrollment", path)
		}
	}
}

func TestNotifierMainWiresStartEnrollmentBeforeDataHubClient(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "cmd", "notifier", "main.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	enroll := strings.Index(src, "bootstrap.StartEnrollment")
	diCall := strings.Index(src, "di.NewNotifierComponents")
	if enroll < 0 {
		t.Fatalf("%s does not call bootstrap.StartEnrollment", path)
	}
	if diCall < 0 {
		t.Fatalf("%s does not call di.NewNotifierComponents", path)
	}
	if enroll > diCall {
		t.Fatal("enrollment must start before NewNotifierComponents (data-hub mTLS client)")
	}
	if !strings.Contains(src, "StartEnrollment(ctx, rt, serviceName)") {
		t.Fatal("StartEnrollment must be called with serviceName (alt-notifier)")
	}
}

func TestStartEnrollment_DisabledNotifier(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", pki.ModeDisabled)
	rt := &Runtime{Log: slog.Default()}
	if err := StartEnrollment(context.Background(), rt, "alt-notifier"); err != nil {
		t.Fatal(err)
	}
	if len(rt.shutdownHooks) != 0 {
		t.Fatalf("disabled enrollment must not register a loop")
	}
}

func TestStartEnrollment_Disabled(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", pki.ModeDisabled)
	rt := &Runtime{Log: slog.Default()}
	if err := StartEnrollment(context.Background(), rt, "alt-backend"); err != nil {
		t.Fatal(err)
	}
	if len(rt.shutdownHooks) != 0 {
		t.Fatalf("disabled enrollment must not register a loop")
	}
}
