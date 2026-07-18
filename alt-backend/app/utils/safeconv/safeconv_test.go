package safeconv

import (
	"math"
	"testing"
)

func TestInt32(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   int
		want int32
	}{
		{0, 0},
		{42, 42},
		{-1, -1},
		{math.MaxInt32, math.MaxInt32},
		{math.MinInt32, math.MinInt32},
		{math.MaxInt32 + 1, math.MaxInt32},
		{math.MinInt32 - 1, math.MinInt32},
	}
	for _, tc := range cases {
		if got := Int32(tc.in); got != tc.want {
			t.Fatalf("Int32(%d)=%d want %d", tc.in, got, tc.want)
		}
	}
}

func TestInt32Exact(t *testing.T) {
	t.Parallel()
	if _, err := Int32Exact(math.MaxInt32 + 1); err == nil {
		t.Fatal("expected overflow error")
	}
	got, err := Int32Exact(7)
	if err != nil || got != 7 {
		t.Fatalf("Int32Exact(7)=%d,%v", got, err)
	}
}

func TestParseInt32(t *testing.T) {
	t.Parallel()
	got, err := ParseInt32("123")
	if err != nil || got != 123 {
		t.Fatalf("ParseInt32(123)=%d,%v", got, err)
	}
	if _, err := ParseInt32("2147483648"); err == nil {
		t.Fatal("expected overflow for MaxInt32+1")
	}
}
