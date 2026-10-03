package pki

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestManager_DoesNotLogPasswordFile(t *testing.T) {
	passwordFile := "/run/secrets/pki-agent-alt-backend-jwk"
	password := "super-secret-jwk-password"
	leak := fmt.Errorf("pki: provisioner password file %q: %s", passwordFile, password)

	t.Run("enroll", func(t *testing.T) {
		iss := &fakeIssuer{err: leak}
		m, _, _ := newTestManager(t, iss, time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC))
		m.Cfg.PasswordFile = passwordFile
		m.Cfg.RetryAttempts = 2
		var buf bytes.Buffer
		m.Log = slog.New(slog.NewJSONHandler(&buf, nil))
		ctx := withT(context.Background(), t)
		if err := m.Enroll(ctx); err == nil {
			t.Fatal("expected enroll failure")
		}
		logs := buf.String()
		if !strings.Contains(logs, "pki_enrollment") {
			t.Fatalf("expected enrollment logs, got %s", logs)
		}
		if !strings.Contains(logs, `"error_type"`) {
			t.Fatalf("expected error_type, got %s", logs)
		}
		assertNoPasswordFileInLogs(t, logs, passwordFile, "/run/secrets/", password)
	})

	t.Run("rekey", func(t *testing.T) {
		nb := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
		iss := &recordingRekeyIssuer{fakeIssuer: fakeIssuer{notBefore: nb.Add(16 * time.Hour), lifetime: 24 * time.Hour}}
		m, files, _ := newTestManager(t, iss, nb.Add(16*time.Hour))
		m.Cfg.PasswordFile = passwordFile
		ctx := withT(context.Background(), t)
		cert, key := newSelfSignedPEM(t, m.Cfg.Subject, nb, nb.Add(24*time.Hour))
		if err := files.Write(ctx, cert, key); err != nil {
			t.Fatal(err)
		}
		iss.mu.Lock()
		iss.err = leak
		iss.mu.Unlock()
		var buf bytes.Buffer
		m.Log = slog.New(slog.NewJSONHandler(&buf, nil))
		if _, err := m.Tick(ctx); err == nil {
			t.Fatal("expected rekey failure")
		}
		logs := buf.String()
		if !strings.Contains(logs, "pki_enrollment") {
			t.Fatalf("expected enrollment logs, got %s", logs)
		}
		assertNoPasswordFileInLogs(t, logs, passwordFile, "/run/secrets/", password)
	})

	t.Run("run", func(t *testing.T) {
		iss := &fakeIssuer{err: leak}
		m, _, _ := newTestManager(t, iss, time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC))
		m.Cfg.PasswordFile = passwordFile
		m.Cfg.TickInterval = time.Millisecond
		var buf syncBuffer
		m.Log = slog.New(slog.NewJSONHandler(&buf, nil))
		ctx, cancel := context.WithCancel(withT(context.Background(), t))
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = m.Run(ctx)
		}()
		deadline := time.After(2 * time.Second)
		for !strings.Contains(buf.String(), "pki_enrollment_tick_failed") {
			select {
			case <-deadline:
				cancel()
				<-done
				t.Fatalf("timed out waiting for tick log: %s", buf.String())
			case <-time.After(5 * time.Millisecond):
			}
		}
		cancel()
		<-done
		assertNoPasswordFileInLogs(t, buf.String(), passwordFile, "/run/secrets/", password)
	})
}
