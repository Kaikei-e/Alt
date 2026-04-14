package logger

import (
	"strings"
	"testing"
)

func TestHashQuery_StableAndShort(t *testing.T) {
	t.Parallel()

	a := HashQuery("プログラミング")
	b := HashQuery("プログラミング")
	if a != b {
		t.Fatalf("hash not stable: %s vs %s", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("want 16 hex chars (8-byte prefix), got %d", len(a))
	}
	if strings.Contains(a, "プログラミング") {
		t.Fatalf("hash must not contain plaintext")
	}
}

func TestHashQuery_EmptyInputHasDistinctMarker(t *testing.T) {
	t.Parallel()
	// Empty queries should still hash (to a stable sentinel) rather than
	// leaking the original value or panicking.
	h := HashQuery("")
	if h == "" {
		t.Fatal("empty input should produce a non-empty hash marker")
	}
}

func TestHashQuery_DifferentInputsDiffer(t *testing.T) {
	t.Parallel()
	if HashQuery("alpha") == HashQuery("beta") {
		t.Fatal("different inputs must hash to different values")
	}
}
