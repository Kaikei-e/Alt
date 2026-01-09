/**
 * Web Vitals module unit tests
 * TDD: Tests for existing vitals.ts functions that need to be exported
 */
import { assertEquals } from "@std/assert";
import { describe, it } from "@std/testing/bdd";
import {
  getRating,
  calculateScore,
  identifyBottlenecks,
  type VitalRating,
  type WebVitalsResult,
} from "../../src/measurement/vitals.ts";

describe("Web Vitals Module", () => {
  describe("getRating", () => {
    it("should return 'good' for values below good threshold", () => {
      const rating = getRating(2000, { good: 2500, poor: 4000 });
      assertEquals(rating, "good");
    });

    it("should return 'good' for value equal to good threshold", () => {
      const rating = getRating(2500, { good: 2500, poor: 4000 });
      assertEquals(rating, "good");
    });

    it("should return 'needs-improvement' for values between thresholds", () => {
      const rating = getRating(3000, { good: 2500, poor: 4000 });
      assertEquals(rating, "needs-improvement");
    });

    it("should return 'needs-improvement' for value equal to poor threshold", () => {
      const rating = getRating(4000, { good: 2500, poor: 4000 });
      assertEquals(rating, "needs-improvement");
    });

    it("should return 'poor' for values above poor threshold", () => {
      const rating = getRating(5000, { good: 2500, poor: 4000 });
      assertEquals(rating, "poor");
    });

    it("should return 'needs-improvement' for null values", () => {
      const rating = getRating(null, { good: 2500, poor: 4000 });
      assertEquals(rating, "needs-improvement");
    });

    it("should return 'needs-improvement' for zero values", () => {
      const rating = getRating(0, { good: 2500, poor: 4000 });
      assertEquals(rating, "needs-improvement");
    });

    it("should handle CLS thresholds correctly", () => {
      assertEquals(getRating(0.05, { good: 0.1, poor: 0.25 }), "good");
      assertEquals(getRating(0.15, { good: 0.1, poor: 0.25 }), "needs-improvement");
      assertEquals(getRating(0.3, { good: 0.1, poor: 0.25 }), "poor");
    });
  });

  describe("calculateScore", () => {
    it("should return 100 for all 'good' ratings", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 1000, rating: "good" },
        inp: { value: 100, rating: "good" },
        cls: { value: 0.05, rating: "good" },
        fcp: { value: 1000, rating: "good" },
        ttfb: { value: 500, rating: "good" },
        timestamp: Date.now(),
      };
      const score = calculateScore(vitals);
      assertEquals(score, 100);
    });

    it("should return 0 for all 'poor' ratings", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 5000, rating: "poor" },
        inp: { value: 600, rating: "poor" },
        cls: { value: 0.5, rating: "poor" },
        fcp: { value: 4000, rating: "poor" },
        ttfb: { value: 2000, rating: "poor" },
        timestamp: Date.now(),
      };
      const score = calculateScore(vitals);
      assertEquals(score, 0);
    });

    it("should return 50 for all 'needs-improvement' ratings", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 3000, rating: "needs-improvement" },
        inp: { value: 300, rating: "needs-improvement" },
        cls: { value: 0.15, rating: "needs-improvement" },
        fcp: { value: 2000, rating: "needs-improvement" },
        ttfb: { value: 1000, rating: "needs-improvement" },
        timestamp: Date.now(),
      };
      const score = calculateScore(vitals);
      assertEquals(score, 50);
    });

    it("should apply weights correctly", () => {
      // LCP good (25%), rest poor
      const vitals: WebVitalsResult = {
        lcp: { value: 1000, rating: "good" },
        inp: { value: 600, rating: "poor" },
        cls: { value: 0.5, rating: "poor" },
        fcp: { value: 4000, rating: "poor" },
        ttfb: { value: 2000, rating: "poor" },
        timestamp: Date.now(),
      };
      const score = calculateScore(vitals);
      // 100 * 25 / 100 = 25
      assertEquals(score, 25);
    });

    it("should handle custom weights", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 1000, rating: "good" },
        inp: { value: 600, rating: "poor" },
        cls: { value: 0.5, rating: "poor" },
        fcp: { value: 4000, rating: "poor" },
        ttfb: { value: 2000, rating: "poor" },
        timestamp: Date.now(),
      };
      const customWeights = { lcp: 100, inp: 0, cls: 0, fcp: 0, ttfb: 0 };
      const score = calculateScore(vitals, customWeights);
      assertEquals(score, 100);
    });
  });

  describe("identifyBottlenecks", () => {
    it("should identify LCP bottleneck", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 5000, rating: "poor" },
        inp: { value: 100, rating: "good" },
        cls: { value: 0.05, rating: "good" },
        fcp: { value: 1000, rating: "good" },
        ttfb: { value: 500, rating: "good" },
        timestamp: Date.now(),
      };
      const bottlenecks = identifyBottlenecks(vitals);
      assertEquals(bottlenecks.some((b) => b.toLowerCase().includes("lcp") || b.toLowerCase().includes("contentful")), true);
      assertEquals(bottlenecks.length, 1);
    });

    it("should identify INP bottleneck", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 1000, rating: "good" },
        inp: { value: 600, rating: "poor" },
        cls: { value: 0.05, rating: "good" },
        fcp: { value: 1000, rating: "good" },
        ttfb: { value: 500, rating: "good" },
        timestamp: Date.now(),
      };
      const bottlenecks = identifyBottlenecks(vitals);
      assertEquals(bottlenecks.some((b) => b.toLowerCase().includes("inp") || b.toLowerCase().includes("interaction")), true);
    });

    it("should identify CLS bottleneck", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 1000, rating: "good" },
        inp: { value: 100, rating: "good" },
        cls: { value: 0.5, rating: "poor" },
        fcp: { value: 1000, rating: "good" },
        ttfb: { value: 500, rating: "good" },
        timestamp: Date.now(),
      };
      const bottlenecks = identifyBottlenecks(vitals);
      assertEquals(bottlenecks.some((b) => b.toLowerCase().includes("cls") || b.toLowerCase().includes("layout")), true);
    });

    it("should identify FCP bottleneck", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 1000, rating: "good" },
        inp: { value: 100, rating: "good" },
        cls: { value: 0.05, rating: "good" },
        fcp: { value: 4000, rating: "poor" },
        ttfb: { value: 500, rating: "good" },
        timestamp: Date.now(),
      };
      const bottlenecks = identifyBottlenecks(vitals);
      assertEquals(bottlenecks.some((b) => b.toLowerCase().includes("fcp") || b.toLowerCase().includes("first")), true);
    });

    it("should identify TTFB bottleneck", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 1000, rating: "good" },
        inp: { value: 100, rating: "good" },
        cls: { value: 0.05, rating: "good" },
        fcp: { value: 1000, rating: "good" },
        ttfb: { value: 2000, rating: "poor" },
        timestamp: Date.now(),
      };
      const bottlenecks = identifyBottlenecks(vitals);
      assertEquals(bottlenecks.some((b) => b.toLowerCase().includes("ttfb") || b.toLowerCase().includes("byte")), true);
    });

    it("should identify multiple bottlenecks", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 5000, rating: "poor" },
        inp: { value: 600, rating: "poor" },
        cls: { value: 0.5, rating: "poor" },
        fcp: { value: 4000, rating: "poor" },
        ttfb: { value: 2000, rating: "poor" },
        timestamp: Date.now(),
      };
      const bottlenecks = identifyBottlenecks(vitals);
      assertEquals(bottlenecks.length, 5);
    });

    it("should return empty array when no bottlenecks", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 1000, rating: "good" },
        inp: { value: 100, rating: "good" },
        cls: { value: 0.05, rating: "good" },
        fcp: { value: 1000, rating: "good" },
        ttfb: { value: 500, rating: "good" },
        timestamp: Date.now(),
      };
      const bottlenecks = identifyBottlenecks(vitals);
      assertEquals(bottlenecks.length, 0);
    });

    it("should not flag 'needs-improvement' as bottleneck", () => {
      const vitals: WebVitalsResult = {
        lcp: { value: 3000, rating: "needs-improvement" },
        inp: { value: 300, rating: "needs-improvement" },
        cls: { value: 0.15, rating: "needs-improvement" },
        fcp: { value: 2000, rating: "needs-improvement" },
        ttfb: { value: 1000, rating: "needs-improvement" },
        timestamp: Date.now(),
      };
      const bottlenecks = identifyBottlenecks(vitals);
      assertEquals(bottlenecks.length, 0);
    });
  });
});
