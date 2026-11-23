package execx

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunEcho(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	err := Run(context.Background(), Cmd{Args: []string{"echo", "hi"}}, &out, &errb)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "hi\n" {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestMissingBinary(t *testing.T) {
	t.Parallel()
	_, _, code, err := Combined(context.Background(), Cmd{Args: []string{"alt-harness-missing-binary-xyz"}})
	if err == nil || code != 127 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := Run(ctx, Cmd{Args: []string{"sleep", "5"}}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()
	if err := Run(context.Background(), Cmd{}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
}
