package test_utils

import (
	"testing"
)

// BenchmarkHelper provides utilities for Go benchmark tests
type BenchmarkHelper struct {
	b *testing.B
}

func NewBenchmarkHelper(b *testing.B) *BenchmarkHelper {
	return &BenchmarkHelper{b: b}
}

func (bh *BenchmarkHelper) RunParallelBenchmark(operation func(pb *testing.PB)) {
	bh.b.ResetTimer()
	bh.b.RunParallel(operation)
}

func (bh *BenchmarkHelper) MeasureMemoryAllocations(operation func()) {
	bh.b.ResetTimer()
	bh.b.ReportAllocs()

	for i := 0; i < bh.b.N; i++ {
		operation()
	}
}

func (bh *BenchmarkHelper) BenchmarkWithSetup(setup func(), operation func(), teardown func()) {
	for i := 0; i < bh.b.N; i++ {
		bh.b.StopTimer()
		setup()
		bh.b.StartTimer()

		operation()

		bh.b.StopTimer()
		teardown()
	}
}
