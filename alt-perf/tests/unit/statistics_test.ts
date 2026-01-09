/**
 * Statistics module unit tests
 * TDD: These tests define the expected behavior of the statistics module
 */
import { assertEquals, assertAlmostEquals } from "@std/assert";
import { describe, it } from "@std/testing/bdd";
import {
  calculateStatistics,
  calculateConfidenceInterval,
  detectOutliers,
  isStable,
  type StatisticalSummary,
} from "../../src/measurement/statistics.ts";

describe("Statistics Module", () => {
  describe("calculateStatistics", () => {
    it("should calculate correct mean for simple dataset", () => {
      const values = [1, 2, 3, 4, 5];
      const stats = calculateStatistics(values);
      assertEquals(stats.mean, 3);
    });

    it("should calculate correct median for odd-length dataset", () => {
      const values = [1, 2, 3, 4, 5];
      const stats = calculateStatistics(values);
      assertEquals(stats.median, 3);
    });

    it("should calculate correct median for even-length dataset", () => {
      const values = [1, 2, 3, 4];
      const stats = calculateStatistics(values);
      assertEquals(stats.median, 2.5);
    });

    it("should calculate correct percentiles for 100 values", () => {
      const values = Array.from({ length: 100 }, (_, i) => i + 1);
      const stats = calculateStatistics(values);
      // Linear interpolation method
      // p75: index = 0.75 * 99 = 74.25, value = 75 + 0.25 * 1 = 75.25
      assertAlmostEquals(stats.p75, 75.25, 0.01);
      assertAlmostEquals(stats.p90, 90.1, 0.01);
      assertAlmostEquals(stats.p95, 95.05, 0.01);
      assertAlmostEquals(stats.p99, 99.01, 0.01);
    });

    it("should calculate standard deviation correctly", () => {
      const values = [2, 4, 4, 4, 5, 5, 7, 9];
      const stats = calculateStatistics(values);
      // Population stddev = 2.0, sample stddev ~= 2.138
      assertAlmostEquals(stats.stdDev, 2.138, 0.01);
    });

    it("should handle single value", () => {
      const values = [42];
      const stats = calculateStatistics(values);
      assertEquals(stats.mean, 42);
      assertEquals(stats.median, 42);
      assertEquals(stats.stdDev, 0);
      assertEquals(stats.count, 1);
    });

    it("should handle empty array gracefully", () => {
      const values: number[] = [];
      const stats = calculateStatistics(values);
      assertEquals(stats.count, 0);
      assertEquals(stats.mean, 0);
      assertEquals(stats.median, 0);
    });

    it("should calculate variance correctly", () => {
      const values = [2, 4, 4, 4, 5, 5, 7, 9];
      const stats = calculateStatistics(values);
      // Sample variance = stdDev^2
      assertAlmostEquals(stats.variance, 4.571, 0.01);
    });

    it("should calculate min and max correctly", () => {
      const values = [5, 2, 8, 1, 9, 3];
      const stats = calculateStatistics(values);
      assertEquals(stats.min, 1);
      assertEquals(stats.max, 9);
    });
  });

  describe("detectOutliers", () => {
    it("should detect obvious outliers using IQR method", () => {
      const values = [10, 11, 12, 13, 14, 100]; // 100 is outlier
      const outliers = detectOutliers(values);
      assertEquals(outliers.includes(100), true);
      assertEquals(outliers.length, 1);
    });

    it("should return empty array when no outliers", () => {
      const values = [10, 11, 12, 13, 14];
      const outliers = detectOutliers(values);
      assertEquals(outliers.length, 0);
    });

    it("should detect multiple outliers", () => {
      const values = [1, 10, 11, 12, 13, 14, 100];
      const outliers = detectOutliers(values);
      assertEquals(outliers.length, 2);
      assertEquals(outliers.includes(1), true);
      assertEquals(outliers.includes(100), true);
    });

    it("should handle empty array", () => {
      const values: number[] = [];
      const outliers = detectOutliers(values);
      assertEquals(outliers.length, 0);
    });

    it("should handle array with less than 4 elements", () => {
      const values = [1, 2, 3];
      const outliers = detectOutliers(values);
      assertEquals(outliers.length, 0);
    });
  });

  describe("isStable", () => {
    it("should return true for stable measurements (low CV)", () => {
      // CV = stdDev / mean, ~2% here
      const values = [100, 102, 98, 101, 99];
      assertEquals(isStable(values, 0.15), true);
    });

    it("should return false for unstable measurements (high CV)", () => {
      // High variance relative to mean
      const values = [100, 200, 50, 300, 75];
      assertEquals(isStable(values, 0.15), false);
    });

    it("should use default CV threshold of 0.15", () => {
      const stableValues = [100, 102, 98, 101, 99];
      assertEquals(isStable(stableValues), true);

      const unstableValues = [100, 200, 50, 300, 75];
      assertEquals(isStable(unstableValues), false);
    });

    it("should handle single value as stable", () => {
      const values = [42];
      assertEquals(isStable(values), true);
    });

    it("should handle empty array as stable", () => {
      const values: number[] = [];
      assertEquals(isStable(values), true);
    });
  });

  describe("calculateConfidenceInterval", () => {
    it("should calculate 95% CI correctly for n=30", () => {
      // For n=30, t-value ~= 2.045
      const ci = calculateConfidenceInterval(100, 10, 30, 0.95);
      assertAlmostEquals(ci.lower, 96.27, 0.5);
      assertAlmostEquals(ci.upper, 103.73, 0.5);
      assertEquals(ci.level, 0.95);
    });

    it("should calculate wider CI for smaller samples", () => {
      const ciLarge = calculateConfidenceInterval(100, 10, 30, 0.95);
      const ciSmall = calculateConfidenceInterval(100, 10, 5, 0.95);

      const widthLarge = ciLarge.upper - ciLarge.lower;
      const widthSmall = ciSmall.upper - ciSmall.lower;

      assertEquals(widthSmall > widthLarge, true);
    });

    it("should calculate narrower CI for 90% level", () => {
      const ci95 = calculateConfidenceInterval(100, 10, 30, 0.95);
      const ci90 = calculateConfidenceInterval(100, 10, 30, 0.90);

      const width95 = ci95.upper - ci95.lower;
      const width90 = ci90.upper - ci90.lower;

      assertEquals(width90 < width95, true);
    });

    it("should handle zero standard deviation", () => {
      const ci = calculateConfidenceInterval(100, 0, 30, 0.95);
      assertEquals(ci.lower, 100);
      assertEquals(ci.upper, 100);
    });

    it("should handle n=1", () => {
      const ci = calculateConfidenceInterval(100, 10, 1, 0.95);
      // With n=1, CI is undefined/infinite - should return mean
      assertEquals(ci.lower, 100);
      assertEquals(ci.upper, 100);
    });
  });
});
