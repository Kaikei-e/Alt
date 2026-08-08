package errorhandler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
)

// captureHandler records every slog.Record so tests can assert on level and
// attributes instead of parsing formatted output.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) recordsAt(level slog.Level) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if r.Level == level {
			out = append(out, r)
		}
	}
	return out
}

func attrValue(r slog.Record, key string) (string, bool) {
	var found string
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value.String()
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func newCapturingLogger() (*slog.Logger, *captureHandler) {
	h := &captureHandler{}
	return slog.New(h), h
}

// productionShapedError reproduces the wrap chain from the real incident:
// handler -> usecase ("track home action: %w") -> sovereign driver
// ("sovereign AppendKnowledgeUserEvent: %w") -> *connect.Error from the wire.
func productionShapedError(code connect.Code, message string) error {
	upstream := connect.NewError(code, errors.New(message))
	driverWrapped := fmt.Errorf("sovereign AppendKnowledgeUserEvent: %w", upstream)
	return fmt.Errorf("track home action: %w", driverWrapped)
}

// TestHandleUpstreamError_PreservesCallerClassCodes pins the core defect: an
// upstream Connect code that the Connect protocol maps to a 4xx status is the
// caller's fault, not ours. Collapsing it into CodeInternal both lies about
// who broke and burns the 5xx dependency-failure budget that downstream
// circuit breakers count.
func TestHandleUpstreamError_PreservesCallerClassCodes(t *testing.T) {
	t.Parallel()

	codes := []connect.Code{
		connect.CodeInvalidArgument,
		connect.CodeNotFound,
		connect.CodePermissionDenied,
		connect.CodeUnauthenticated,
		connect.CodeResourceExhausted,
		connect.CodeFailedPrecondition,
		connect.CodeAlreadyExists,
		connect.CodeAborted,
		connect.CodeOutOfRange,
	}

	for _, code := range codes {
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()
			logger, capture := newCapturingLogger()

			connectErr := HandleUpstreamError(
				context.Background(),
				logger,
				productionShapedError(code, "dedupe_key is required"),
				"TrackHomeAction",
			)

			if connectErr == nil {
				t.Fatal("expected a connect error, got nil")
			}
			if connectErr.Code() != code {
				t.Fatalf("expected upstream code %v to be preserved, got %v", code, connectErr.Code())
			}

			if errs := capture.recordsAt(slog.LevelError); len(errs) != 0 {
				t.Fatalf("caller-class upstream error must not log at ERROR, got %d ERROR records", len(errs))
			}

			warns := capture.recordsAt(slog.LevelWarn)
			if len(warns) != 1 {
				t.Fatalf("expected exactly 1 WARN record, got %d", len(warns))
			}

			gotCode, ok := attrValue(warns[0], "upstream_code")
			if !ok || gotCode != code.String() {
				t.Errorf("expected upstream_code=%q attribute, got %q (present=%v)", code.String(), gotCode, ok)
			}
			gotOp, ok := attrValue(warns[0], "operation")
			if !ok || gotOp != "TrackHomeAction" {
				t.Errorf("expected operation=TrackHomeAction attribute, got %q (present=%v)", gotOp, ok)
			}
			// The caller bug must stay findable: the upstream message is the
			// only thing that says WHICH field was malformed.
			gotMsg, ok := attrValue(warns[0], "upstream_message")
			if !ok || gotMsg != "dedupe_key is required" {
				t.Errorf("expected verbatim upstream_message, got %q (present=%v)", gotMsg, ok)
			}
			if _, hasCause := attrValue(warns[0], "cause"); !hasCause {
				t.Error("expected the full wrap chain under a cause attribute so the caller bug is traceable")
			}
		})
	}
}

// TestHandleUpstreamError_KeepsServerClassCodesInternal pins Critical Rule 8:
// honest degradation must not become quiet degradation. A genuine upstream
// fault stays 5xx and stays loud.
func TestHandleUpstreamError_KeepsServerClassCodesInternal(t *testing.T) {
	t.Parallel()

	codes := []connect.Code{
		connect.CodeInternal,
		connect.CodeUnknown,
		connect.CodeUnavailable,
		connect.CodeDataLoss,
		connect.CodeDeadlineExceeded,
		connect.CodeUnimplemented,
		connect.CodeCanceled,
	}

	for _, code := range codes {
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()
			logger, capture := newCapturingLogger()

			connectErr := HandleUpstreamError(
				context.Background(),
				logger,
				productionShapedError(code, "upstream exploded"),
				"TrackHomeAction",
			)

			if connectErr.Code() != connect.CodeInternal {
				t.Fatalf("expected %v to stay CodeInternal, got %v", code, connectErr.Code())
			}
			if warns := capture.recordsAt(slog.LevelWarn); len(warns) != 0 {
				t.Fatalf("server-class upstream error must not be downgraded to WARN, got %d WARN records", len(warns))
			}
			errRecords := capture.recordsAt(slog.LevelError)
			if len(errRecords) != 1 {
				t.Fatalf("expected exactly 1 ERROR record, got %d", len(errRecords))
			}
			if id, ok := attrValue(errRecords[0], "error_id"); !ok || id == "" {
				t.Error("internal faults must keep their error_id for log correlation")
			}
		})
	}
}

// TestHandleUpstreamError_NonConnectErrorStaysInternal pins the
// "anything unclassifiable stays 5xx" half of the contract.
func TestHandleUpstreamError_NonConnectErrorStaysInternal(t *testing.T) {
	t.Parallel()
	logger, capture := newCapturingLogger()

	connectErr := HandleUpstreamError(
		context.Background(),
		logger,
		errors.New("database connection failed"),
		"TrackHomeAction",
	)

	if connectErr.Code() != connect.CodeInternal {
		t.Fatalf("expected CodeInternal for a plain error, got %v", connectErr.Code())
	}
	if len(capture.recordsAt(slog.LevelError)) != 1 {
		t.Fatal("a plain error must still log at ERROR")
	}
}

// TestHandleUpstreamError_CodedErrorWithoutMessage guards the
// connect.NewError(code, nil) shape, whose Message() is empty.
func TestHandleUpstreamError_CodedErrorWithoutMessage(t *testing.T) {
	t.Parallel()
	logger, capture := newCapturingLogger()

	connectErr := HandleUpstreamError(
		context.Background(),
		logger,
		fmt.Errorf("lookup lens: %w", connect.NewError(connect.CodeNotFound, nil)),
		"ListLenses",
	)

	if connectErr.Code() != connect.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %v", connectErr.Code())
	}
	if len(capture.recordsAt(slog.LevelWarn)) != 1 {
		t.Fatal("expected a WARN record for a message-less caller-class upstream error")
	}
}

// TestHandleUpstreamError_ClientMessageDoesNotLeakWrapChain keeps the existing
// security posture of this package: the internal wrap chain is log-only.
func TestHandleUpstreamError_ClientMessageDoesNotLeakWrapChain(t *testing.T) {
	t.Parallel()
	logger, _ := newCapturingLogger()

	connectErr := HandleUpstreamError(
		context.Background(),
		logger,
		productionShapedError(connect.CodeInvalidArgument, "dedupe_key is required"),
		"TrackHomeAction",
	)

	msg := connectErr.Message()
	if msg == "" {
		t.Fatal("expected a non-empty client-facing message")
	}
	for _, leak := range []string{"sovereign AppendKnowledgeUserEvent", "track home action:"} {
		if strings.Contains(msg, leak) {
			t.Errorf("client message leaked internal wrap chain %q: %s", leak, msg)
		}
	}
}
