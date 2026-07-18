// Package safeconv provides overflow-safe integer conversions for protobuf and API boundaries.
package safeconv

import (
	"fmt"
	"math"
	"strconv"
)

// Int32 converts v to int32, clamping to the int32 range.
// Use for response counts/lengths where saturation is preferable to failure.
func Int32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// Int32FromInt64 converts v to int32, clamping to the int32 range.
func Int32FromInt64(v int64) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// Int32Exact converts v to int32 or returns an error on overflow.
func Int32Exact(v int) (int32, error) {
	if v > math.MaxInt32 || v < math.MinInt32 {
		return 0, fmt.Errorf("value %d overflows int32", v)
	}
	return int32(v), nil
}

// ParseInt32 parses s as a base-10 int32 (rejects values outside int32).
func ParseInt32(s string) (int32, error) {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(v), nil
}
