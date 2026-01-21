import { describe, it, expect } from "vitest";
import {
	calculateDurationSecs,
	calculateStageDurations,
	calculateJobMetrics,
	formatDurationWithUnits,
	getPerformanceLabel,
	calculateBarWidth,
} from "./stageMetrics";
import type { StatusTransition } from "$lib/schema/dashboard";

describe("stageMetrics", () => {
	describe("calculateDurationSecs", () => {
		it("calculates duration between two timestamps correctly", () => {
			const start = "2025-01-20T10:00:00Z";
			const end = "2025-01-20T10:05:30Z";
			expect(calculateDurationSecs(start, end)).toBe(330); // 5 min 30 sec
		});

		it("returns 0 for same timestamps", () => {
			const time = "2025-01-20T10:00:00Z";
			expect(calculateDurationSecs(time, time)).toBe(0);
		});

		it("returns 0 for negative duration", () => {
			const start = "2025-01-20T10:05:00Z";
			const end = "2025-01-20T10:00:00Z";
			expect(calculateDurationSecs(start, end)).toBe(0);
		});
	});

	describe("calculateStageDurations", () => {
		it("returns pending stages for empty history", () => {
			const result = calculateStageDurations([], "2025-01-20T10:00:00Z", "pending");
			expect(result).toHaveLength(8); // PIPELINE_STAGES.length
			expect(result.every((s) => s.status === "pending")).toBe(true);
		});

		it("calculates duration for completed stages", () => {
			const history: StatusTransition[] = [
				{
					id: 1,
					status: "running",
					stage: "fetch",
					transitioned_at: "2025-01-20T10:00:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 2,
					status: "completed",
					stage: "fetch",
					transitioned_at: "2025-01-20T10:01:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 3,
					status: "running",
					stage: "preprocess",
					transitioned_at: "2025-01-20T10:01:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 4,
					status: "completed",
					stage: "preprocess",
					transitioned_at: "2025-01-20T10:03:00Z",
					reason: null,
					actor: "system",
				},
			];

			const result = calculateStageDurations(history, "2025-01-20T10:00:00Z", "running");

			const fetchStage = result.find((s) => s.stage === "fetch");
			const preprocessStage = result.find((s) => s.stage === "preprocess");

			expect(fetchStage?.durationSecs).toBe(60); // 1 minute
			expect(fetchStage?.status).toBe("completed");
			expect(preprocessStage?.durationSecs).toBe(120); // 2 minutes
			expect(preprocessStage?.status).toBe("completed");
		});

		it("infers completion from next stage start", () => {
			const history: StatusTransition[] = [
				{
					id: 1,
					status: "running",
					stage: "fetch",
					transitioned_at: "2025-01-20T10:00:00Z",
					reason: null,
					actor: "system",
				},
				// No explicit completion for fetch, but preprocess starts
				{
					id: 2,
					status: "running",
					stage: "preprocess",
					transitioned_at: "2025-01-20T10:02:00Z",
					reason: null,
					actor: "system",
				},
			];

			const result = calculateStageDurations(history, "2025-01-20T10:00:00Z", "running");

			const fetchStage = result.find((s) => s.stage === "fetch");
			expect(fetchStage?.durationSecs).toBe(120); // 2 minutes (inferred from next stage)
			expect(fetchStage?.status).toBe("completed");
		});

		it("infers timing for missing stages when job is completed", () => {
			// This tests the scenario where a stage (like "evidence") doesn't have
			// explicit status history entries, but the job completed successfully.
			// The function should infer timing from surrounding stages.
			const history: StatusTransition[] = [
				{
					id: 1,
					status: "running",
					stage: "fetch",
					transitioned_at: "2025-01-20T10:00:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 2,
					status: "running",
					stage: "preprocess",
					transitioned_at: "2025-01-20T10:01:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 3,
					status: "running",
					stage: "dedup",
					transitioned_at: "2025-01-20T10:02:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 4,
					status: "running",
					stage: "genre",
					transitioned_at: "2025-01-20T10:03:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 5,
					status: "running",
					stage: "select",
					transitioned_at: "2025-01-20T10:04:00Z",
					reason: null,
					actor: "system",
				},
				// NOTE: No "evidence" stage entry - simulating the bug scenario
				{
					id: 6,
					status: "running",
					stage: "dispatch",
					transitioned_at: "2025-01-20T10:05:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 7,
					status: "running",
					stage: "persist",
					transitioned_at: "2025-01-20T10:06:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 8,
					status: "completed",
					stage: "persist",
					transitioned_at: "2025-01-20T10:07:00Z",
					reason: null,
					actor: "system",
				},
			];

			const result = calculateStageDurations(history, "2025-01-20T10:00:00Z", "completed");

			// All stages should be marked as completed for a completed job
			const completedStages = result.filter((s) => s.status === "completed");
			expect(completedStages).toHaveLength(8);

			// Evidence stage should have inferred timing from select (end) and dispatch (start)
			const evidenceStage = result.find((s) => s.stage === "evidence");
			expect(evidenceStage?.status).toBe("completed");
			// Evidence should infer: start from select's end, end from dispatch's start
			// Since select doesn't have explicit end, select's end is inferred from dispatch's start
			// So evidence start = select's end = dispatch's start = 10:05:00
			// And evidence end = dispatch's start = 10:05:00
			// This means duration = 0 for evidence (which is expected since it's instantaneous)
			expect(evidenceStage?.durationSecs).toBe(0);
		});

		it("correctly counts all stages as completed when job is completed", () => {
			// Test that verifies the Stages "5/8" vs "8/8" fix
			const history: StatusTransition[] = [
				{
					id: 1,
					status: "running",
					stage: "fetch",
					transitioned_at: "2025-01-20T10:00:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 2,
					status: "running",
					stage: "preprocess",
					transitioned_at: "2025-01-20T10:01:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 3,
					status: "running",
					stage: "dedup",
					transitioned_at: "2025-01-20T10:02:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 4,
					status: "running",
					stage: "genre",
					transitioned_at: "2025-01-20T10:03:00Z",
					reason: null,
					actor: "system",
				},
				// Missing: select, evidence (simulating incomplete status history)
				{
					id: 5,
					status: "running",
					stage: "dispatch",
					transitioned_at: "2025-01-20T10:05:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 6,
					status: "running",
					stage: "persist",
					transitioned_at: "2025-01-20T10:06:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 7,
					status: "completed",
					stage: "persist",
					transitioned_at: "2025-01-20T10:07:00Z",
					reason: null,
					actor: "system",
				},
			];

			const result = calculateStageDurations(history, "2025-01-20T10:00:00Z", "completed");

			// Even with missing status history for select and evidence,
			// all 8 stages should be marked as completed when job is completed
			const completedCount = result.filter((s) => s.status === "completed").length;
			expect(completedCount).toBe(8);

			// Verify select stage is marked as completed even without explicit history
			const selectStage = result.find((s) => s.stage === "select");
			expect(selectStage?.status).toBe("completed");

			// Verify evidence stage is marked as completed even without explicit history
			const evidenceStage = result.find((s) => s.stage === "evidence");
			expect(evidenceStage?.status).toBe("completed");
		});
	});

	describe("calculateJobMetrics", () => {
		it("calculates performance ratio correctly", () => {
			const history: StatusTransition[] = [
				{
					id: 1,
					status: "running",
					stage: "fetch",
					transitioned_at: "2025-01-20T10:00:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 2,
					status: "completed",
					stage: "fetch",
					transitioned_at: "2025-01-20T10:05:00Z",
					reason: null,
					actor: "system",
				},
			];

			const result = calculateJobMetrics(
				history,
				"2025-01-20T10:00:00Z",
				"completed",
				300, // 5 minutes total
				250, // 4:10 average
			);

			expect(result.totalDurationSecs).toBe(300);
			expect(result.performanceRatio).toBeCloseTo(1.2); // slower than average
		});

		it("returns null ratio when average is not available", () => {
			const result = calculateJobMetrics(
				[],
				"2025-01-20T10:00:00Z",
				"completed",
				300,
				null,
			);

			expect(result.performanceRatio).toBeNull();
		});
	});

	describe("formatDurationWithUnits", () => {
		it("formats seconds only", () => {
			expect(formatDurationWithUnits(45)).toBe("45s");
		});

		it("formats minutes and seconds", () => {
			expect(formatDurationWithUnits(125)).toBe("2m 5s");
		});

		it("formats minutes only when no remainder", () => {
			expect(formatDurationWithUnits(120)).toBe("2m");
		});

		it("formats hours and minutes", () => {
			expect(formatDurationWithUnits(3725)).toBe("1h 2m");
		});

		it("returns dash for zero", () => {
			expect(formatDurationWithUnits(0)).toBe("-");
		});
	});

	describe("getPerformanceLabel", () => {
		it("returns green for fast performance", () => {
			expect(getPerformanceLabel(0.5)).toEqual({ label: "Fast", color: "green" });
			expect(getPerformanceLabel(0.8)).toEqual({ label: "Fast", color: "green" });
		});

		it("returns amber for normal performance", () => {
			expect(getPerformanceLabel(0.9)).toEqual({ label: "Normal", color: "amber" });
			expect(getPerformanceLabel(1.0)).toEqual({ label: "Normal", color: "amber" });
			expect(getPerformanceLabel(1.2)).toEqual({ label: "Normal", color: "amber" });
		});

		it("returns red for slow performance", () => {
			expect(getPerformanceLabel(1.3)).toEqual({ label: "Slow", color: "red" });
			expect(getPerformanceLabel(2.0)).toEqual({ label: "Slow", color: "red" });
		});

		it("returns gray for null ratio", () => {
			expect(getPerformanceLabel(null)).toEqual({ label: "-", color: "gray" });
		});
	});

	describe("calculateBarWidth", () => {
		it("calculates percentage correctly", () => {
			expect(calculateBarWidth(50, 100)).toBe(50);
			expect(calculateBarWidth(75, 100)).toBe(75);
		});

		it("returns 0 for zero values", () => {
			expect(calculateBarWidth(0, 100)).toBe(0);
			expect(calculateBarWidth(50, 0)).toBe(0);
		});

		it("returns 100 for max value", () => {
			expect(calculateBarWidth(100, 100)).toBe(100);
		});
	});
});
