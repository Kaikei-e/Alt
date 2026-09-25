package test_utils

import (
	"runtime"
	"time"
)

// Utility functions for performance calculations
func getMemoryUsage() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

func calculateMemoryDelta(initial, final runtime.MemStats) MemoryUsage {
	return MemoryUsage{
		AllocMB:      float64(final.Alloc-initial.Alloc) / 1024 / 1024,
		TotalAllocMB: float64(final.TotalAlloc-initial.TotalAlloc) / 1024 / 1024,
		SysMB:        float64(final.Sys-initial.Sys) / 1024 / 1024,
		NumGC:        final.NumGC - initial.NumGC,
	}
}

func calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}

	return total / time.Duration(len(durations))
}

func calculateMin(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	min := durations[0]
	for _, d := range durations[1:] {
		if d < min {
			min = d
		}
	}

	return min
}

func calculateMax(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	max := durations[0]
	for _, d := range durations[1:] {
		if d > max {
			max = d
		}
	}

	return max
}

func calculatePercentile(durations []time.Duration, percentile int) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Simple percentile calculation (for production, use a proper sorting algorithm)
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// Bubble sort (simple but not efficient for large datasets)
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	index := (percentile * len(sorted)) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}
