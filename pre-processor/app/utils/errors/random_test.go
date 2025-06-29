package errors

import "testing"

func TestSecureRandomFloat64_Range(t *testing.T) {
	for i := 0; i < 10; i++ {
		v := SecureRandomFloat64()
		if v < 0 || v > 1 {
			t.Fatalf("value out of range: %f", v)
		}
	}
}
