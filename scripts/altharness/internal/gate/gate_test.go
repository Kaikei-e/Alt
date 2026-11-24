package gate

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunOrderAndFailClosed(t *testing.T) {
	t.Parallel()
	var seq atomic.Int32
	steps := []Step{
		{Name: "a", Parallel: true, Run: func(context.Context) (int, error) {
			seq.Add(1)
			time.Sleep(30 * time.Millisecond)
			return 0, nil
		}},
		{Name: "b", Parallel: true, Run: func(context.Context) (int, error) {
			seq.Add(1)
			return 1, errors.New("b broke")
		}},
		{Name: "c", Parallel: false, Run: func(context.Context) (int, error) {
			if seq.Load() < 2 {
				t.Errorf("serial step overlapped parallel group")
			}
			return 0, nil
		}},
		{Name: "missing"},
	}
	rep, err := Run(context.Background(), steps, Options{Workers: 2, DefaultTimeout: time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Failed != 2 {
		t.Fatalf("failed=%d, want 2", rep.Failed)
	}
	if rep.Results[0].Name != "a" || rep.Results[1].Code != 1 || rep.Results[3].Code != 2 {
		t.Fatalf("results=%+v", rep.Results)
	}
	var buf bytes.Buffer
	FormatSummary(&buf, rep)
	text := buf.String()
	if !(bytes.Contains(buf.Bytes(), []byte("PASS: a")) && bytes.Contains(buf.Bytes(), []byte("FAIL: b")) && bytes.Contains(buf.Bytes(), []byte("FAIL: missing"))) {
		t.Fatalf("summary:\n%s", text)
	}
	a := bytes.Index(buf.Bytes(), []byte("PASS: a"))
	b := bytes.Index(buf.Bytes(), []byte("FAIL: b"))
	c := bytes.Index(buf.Bytes(), []byte("PASS: c"))
	if !(a < b && b < c) {
		t.Fatalf("summary not in catalog order:\n%s", text)
	}
}

func TestTimeoutFailsClosed(t *testing.T) {
	t.Parallel()
	steps := []Step{{
		Name:    "slow",
		Timeout: 20 * time.Millisecond,
		Run: func(ctx context.Context) (int, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(time.Second):
				return 0, nil
			}
		},
	}}
	rep, err := Run(context.Background(), steps, Options{Workers: 1})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Results[0].Code == 0 || !rep.Results[0].TimedOut {
		t.Fatalf("result=%+v", rep.Results[0])
	}
}

func TestNilContext(t *testing.T) {
	t.Parallel()
	_, err := Run(nil, nil, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}
