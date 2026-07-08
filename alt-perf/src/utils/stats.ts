/**
 * Statistical utility functions for performance measurement
 */

// Bounds-checked array read; the callers below only ever pass indices that
// are mathematically within range, but noUncheckedIndexedAccess can't prove
// that statically, so this makes the invariant explicit instead of using a
// non-null assertion.
function at(values: number[], index: number): number {
  const value = values[index];
  if (value === undefined) {
    throw new Error(`Index ${index} out of bounds (length ${values.length})`);
  }
  return value;
}

/**
 * Calculate median of an array of numbers
 */
export function calculateMedian(values: number[]): number {
  if (values.length === 0) return 0;

  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);

  if (sorted.length % 2 === 0) {
    return (at(sorted, mid - 1) + at(sorted, mid)) / 2;
  }
  return at(sorted, mid);
}

/**
 * Calculate percentile (e.g., P90, P95)
 */
export function calculatePercentile(values: number[], percentile: number): number {
  if (values.length === 0) return 0;
  if (percentile < 0 || percentile > 100) {
    throw new Error("Percentile must be between 0 and 100");
  }

  const sorted = [...values].sort((a, b) => a - b);
  const index = (percentile / 100) * (sorted.length - 1);
  const lower = Math.floor(index);
  const upper = Math.ceil(index);

  if (lower === upper) {
    return at(sorted, lower);
  }

  const weight = index - lower;
  return at(sorted, lower) * (1 - weight) + at(sorted, upper) * weight;
}

/**
 * Calculate P90 (90th percentile)
 */
export function calculateP90(values: number[]): number {
  return calculatePercentile(values, 90);
}

/**
 * Calculate mean (average)
 */
export function calculateMean(values: number[]): number {
  if (values.length === 0) return 0;
  return values.reduce((sum, v) => sum + v, 0) / values.length;
}

/**
 * Calculate standard deviation
 */
export function calculateStdDev(values: number[]): number {
  if (values.length === 0) return 0;

  const mean = calculateMean(values);
  const squaredDiffs = values.map((v) => Math.pow(v - mean, 2));
  const variance = calculateMean(squaredDiffs);

  return Math.sqrt(variance);
}

/**
 * Discard outliers using IQR (Interquartile Range) method
 * Returns values within [Q1 - threshold*IQR, Q3 + threshold*IQR]
 * Default threshold is 1.5 (standard for mild outliers)
 */
export function discardOutliers(values: number[], threshold: number = 1.5): number[] {
  if (values.length < 4) return values;

  const sorted = [...values].sort((a, b) => a - b);
  const q1 = calculatePercentile(sorted, 25);
  const q3 = calculatePercentile(sorted, 75);
  const iqr = q3 - q1;

  const lowerBound = q1 - threshold * iqr;
  const upperBound = q3 + threshold * iqr;

  return values.filter((v) => v >= lowerBound && v <= upperBound);
}

/**
 * Calculate statistics summary for a set of measurements
 */
export interface StatsSummary {
  count: number;
  mean: number;
  median: number;
  p90: number;
  stdDev: number;
  min: number;
  max: number;
}

export function calculateStats(values: number[]): StatsSummary {
  if (values.length === 0) {
    return {
      count: 0,
      mean: 0,
      median: 0,
      p90: 0,
      stdDev: 0,
      min: 0,
      max: 0,
    };
  }

  return {
    count: values.length,
    mean: calculateMean(values),
    median: calculateMedian(values),
    p90: calculateP90(values),
    stdDev: calculateStdDev(values),
    min: Math.min(...values),
    max: Math.max(...values),
  };
}
