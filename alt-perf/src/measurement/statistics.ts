/**
 * Statistical analysis module for performance measurements
 * Provides functions for calculating statistics, detecting outliers,
 * and determining measurement stability.
 */

/**
 * Confidence interval result
 */
export interface ConfidenceInterval {
  lower: number;
  upper: number;
  level: number;
}

/**
 * Statistical summary of a dataset
 */
export interface StatisticalSummary {
  count: number;
  mean: number;
  median: number;
  min: number;
  max: number;
  stdDev: number;
  variance: number;
  p75: number;
  p90: number;
  p95: number;
  p99: number;
  confidenceInterval: ConfidenceInterval;
  outliers: number[];
  isStable: boolean;
}

/**
 * T-distribution critical values for common confidence levels
 * Key: degrees of freedom, Value: { 0.90: t-value, 0.95: t-value, 0.99: t-value }
 */
const T_CRITICAL_VALUES: Record<number, Record<number, number>> = {
  1: { 0.90: 6.314, 0.95: 12.706, 0.99: 63.657 },
  2: { 0.90: 2.920, 0.95: 4.303, 0.99: 9.925 },
  3: { 0.90: 2.353, 0.95: 3.182, 0.99: 5.841 },
  4: { 0.90: 2.132, 0.95: 2.776, 0.99: 4.604 },
  5: { 0.90: 2.015, 0.95: 2.571, 0.99: 4.032 },
  6: { 0.90: 1.943, 0.95: 2.447, 0.99: 3.707 },
  7: { 0.90: 1.895, 0.95: 2.365, 0.99: 3.499 },
  8: { 0.90: 1.860, 0.95: 2.306, 0.99: 3.355 },
  9: { 0.90: 1.833, 0.95: 2.262, 0.99: 3.250 },
  10: { 0.90: 1.812, 0.95: 2.228, 0.99: 3.169 },
  15: { 0.90: 1.753, 0.95: 2.131, 0.99: 2.947 },
  20: { 0.90: 1.725, 0.95: 2.086, 0.99: 2.845 },
  25: { 0.90: 1.708, 0.95: 2.060, 0.99: 2.787 },
  30: { 0.90: 1.697, 0.95: 2.042, 0.99: 2.750 },
  40: { 0.90: 1.684, 0.95: 2.021, 0.99: 2.704 },
  50: { 0.90: 1.676, 0.95: 2.009, 0.99: 2.678 },
  100: { 0.90: 1.660, 0.95: 1.984, 0.99: 2.626 },
  1000: { 0.90: 1.646, 0.95: 1.962, 0.99: 2.581 },
};

/**
 * Get the t-critical value for given degrees of freedom and confidence level
 */
function getTCriticalValue(df: number, level: number): number {
  // Find the closest df in the table
  const dfs = Object.keys(T_CRITICAL_VALUES)
    .map(Number)
    .sort((a, b) => a - b);

  let closestDf = dfs[0];
  for (const tableDf of dfs) {
    if (tableDf <= df) {
      closestDf = tableDf;
    } else {
      break;
    }
  }

  const values = T_CRITICAL_VALUES[closestDf];
  // Find closest confidence level
  if (level in values) {
    return values[level];
  }
  // Default to 0.95 if not found
  return values[0.95] ?? 1.96;
}

/**
 * Calculate percentile value from sorted array
 */
function percentile(sortedValues: number[], p: number): number {
  if (sortedValues.length === 0) return 0;
  if (sortedValues.length === 1) return sortedValues[0];

  const index = (p / 100) * (sortedValues.length - 1);
  const lower = Math.floor(index);
  const upper = Math.ceil(index);

  if (lower === upper) {
    return sortedValues[lower];
  }

  const fraction = index - lower;
  return sortedValues[lower] + fraction * (sortedValues[upper] - sortedValues[lower]);
}

/**
 * Calculate comprehensive statistics for a dataset
 * Uses Welford's algorithm for numerical stability
 */
export function calculateStatistics(values: number[]): StatisticalSummary {
  if (values.length === 0) {
    return {
      count: 0,
      mean: 0,
      median: 0,
      min: 0,
      max: 0,
      stdDev: 0,
      variance: 0,
      p75: 0,
      p90: 0,
      p95: 0,
      p99: 0,
      confidenceInterval: { lower: 0, upper: 0, level: 0.95 },
      outliers: [],
      isStable: true,
    };
  }

  if (values.length === 1) {
    const value = values[0];
    return {
      count: 1,
      mean: value,
      median: value,
      min: value,
      max: value,
      stdDev: 0,
      variance: 0,
      p75: value,
      p90: value,
      p95: value,
      p99: value,
      confidenceInterval: { lower: value, upper: value, level: 0.95 },
      outliers: [],
      isStable: true,
    };
  }

  // Welford's online algorithm for mean and variance
  let mean = 0;
  let m2 = 0;
  let min = Infinity;
  let max = -Infinity;

  for (let i = 0; i < values.length; i++) {
    const value = values[i];
    const delta = value - mean;
    mean += delta / (i + 1);
    const delta2 = value - mean;
    m2 += delta * delta2;

    if (value < min) min = value;
    if (value > max) max = value;
  }

  // Sample variance and standard deviation
  const variance = m2 / (values.length - 1);
  const stdDev = Math.sqrt(variance);

  // Sort for median and percentiles
  const sorted = [...values].sort((a, b) => a - b);

  // Median
  const mid = Math.floor(sorted.length / 2);
  const median =
    sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid];

  // Percentiles
  const p75 = percentile(sorted, 75);
  const p90 = percentile(sorted, 90);
  const p95 = percentile(sorted, 95);
  const p99 = percentile(sorted, 99);

  // Confidence interval
  const ci = calculateConfidenceInterval(mean, stdDev, values.length, 0.95);

  // Outliers
  const outliers = detectOutliers(values);

  // Stability check
  const stable = isStable(values);

  return {
    count: values.length,
    mean,
    median,
    min,
    max,
    stdDev,
    variance,
    p75,
    p90,
    p95,
    p99,
    confidenceInterval: ci,
    outliers,
    isStable: stable,
  };
}

/**
 * Calculate confidence interval for the mean
 * Uses t-distribution for small samples
 */
export function calculateConfidenceInterval(
  mean: number,
  stdDev: number,
  n: number,
  level: number = 0.95
): ConfidenceInterval {
  if (n <= 1 || stdDev === 0) {
    return { lower: mean, upper: mean, level };
  }

  const df = n - 1;
  const tValue = getTCriticalValue(df, level);
  const standardError = stdDev / Math.sqrt(n);
  const margin = tValue * standardError;

  return {
    lower: mean - margin,
    upper: mean + margin,
    level,
  };
}

/**
 * Detect outliers using the IQR method
 * Outliers are values outside Q1 - 1.5*IQR and Q3 + 1.5*IQR
 */
export function detectOutliers(values: number[]): number[] {
  if (values.length < 4) {
    return [];
  }

  const sorted = [...values].sort((a, b) => a - b);

  // Calculate Q1 and Q3
  const q1 = percentile(sorted, 25);
  const q3 = percentile(sorted, 75);
  const iqr = q3 - q1;

  const lowerBound = q1 - 1.5 * iqr;
  const upperBound = q3 + 1.5 * iqr;

  return values.filter((v) => v < lowerBound || v > upperBound);
}

/**
 * Check if measurements are stable based on coefficient of variation
 * CV = stdDev / mean
 * Measurements are considered stable if CV is below the threshold
 */
export function isStable(values: number[], cvThreshold: number = 0.15): boolean {
  if (values.length <= 1) {
    return true;
  }

  const stats = calculateStatisticsBasic(values);

  if (stats.mean === 0) {
    return stats.stdDev === 0;
  }

  const cv = stats.stdDev / Math.abs(stats.mean);
  return cv < cvThreshold;
}

/**
 * Basic statistics calculation (mean and stdDev only)
 * Used internally to avoid circular dependency
 */
function calculateStatisticsBasic(values: number[]): { mean: number; stdDev: number } {
  if (values.length === 0) {
    return { mean: 0, stdDev: 0 };
  }

  if (values.length === 1) {
    return { mean: values[0], stdDev: 0 };
  }

  let mean = 0;
  let m2 = 0;

  for (let i = 0; i < values.length; i++) {
    const delta = values[i] - mean;
    mean += delta / (i + 1);
    const delta2 = values[i] - mean;
    m2 += delta * delta2;
  }

  const variance = m2 / (values.length - 1);
  const stdDev = Math.sqrt(variance);

  return { mean, stdDev };
}
