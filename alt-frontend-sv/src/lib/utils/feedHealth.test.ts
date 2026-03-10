import { describe, it, expect } from "vitest";
import type { FeedLink, FeedHealthStatus } from "$lib/schema/feedLink";
import {
	classifyFeedHealth,
	getHealthColor,
	getHealthLabel,
	summarizeHealth,
} from "./feedHealth";

function makeFeed(
	overrides: Partial<FeedLink> & { healthStatus: FeedHealthStatus },
): FeedLink {
	return {
		id: "test-id",
		url: "https://example.com/feed.xml",
		consecutiveFailures: 0,
		lastFailureReason: "",
		isActive: true,
		...overrides,
	};
}

describe("classifyFeedHealth", () => {
	it("returns the feed healthStatus directly", () => {
		expect(classifyFeedHealth(makeFeed({ healthStatus: "healthy" }))).toBe(
			"healthy",
		);
		expect(classifyFeedHealth(makeFeed({ healthStatus: "warning" }))).toBe(
			"warning",
		);
		expect(classifyFeedHealth(makeFeed({ healthStatus: "error" }))).toBe(
			"error",
		);
		expect(classifyFeedHealth(makeFeed({ healthStatus: "inactive" }))).toBe(
			"inactive",
		);
		expect(classifyFeedHealth(makeFeed({ healthStatus: "unknown" }))).toBe(
			"unknown",
		);
	});
});

describe("getHealthColor", () => {
	it("returns green for healthy", () => {
		expect(getHealthColor("healthy")).toBe("#22c55e");
	});
	it("returns amber for warning", () => {
		expect(getHealthColor("warning")).toBe("#f59e0b");
	});
	it("returns red for error", () => {
		expect(getHealthColor("error")).toBe("#ef4444");
	});
	it("returns gray-400 for inactive", () => {
		expect(getHealthColor("inactive")).toBe("#9ca3af");
	});
	it("returns gray-500 for unknown", () => {
		expect(getHealthColor("unknown")).toBe("#6b7280");
	});
});

describe("getHealthLabel", () => {
	it.each([
		["healthy", "Healthy"],
		["warning", "Warning"],
		["error", "Error"],
		["inactive", "Inactive"],
		["unknown", "Unknown"],
	] as [FeedHealthStatus, string][])(
		"returns %s for %s status",
		(status, label) => {
			expect(getHealthLabel(status)).toBe(label);
		},
	);
});

describe("summarizeHealth", () => {
	it("counts all statuses correctly", () => {
		const feeds: FeedLink[] = [
			makeFeed({ id: "1", healthStatus: "healthy" }),
			makeFeed({ id: "2", healthStatus: "healthy" }),
			makeFeed({ id: "3", healthStatus: "warning" }),
			makeFeed({ id: "4", healthStatus: "error" }),
			makeFeed({ id: "5", healthStatus: "inactive" }),
			makeFeed({ id: "6", healthStatus: "unknown" }),
		];

		const result = summarizeHealth(feeds);
		expect(result).toEqual({
			healthy: 2,
			warning: 1,
			error: 1,
			inactive: 1,
			unknown: 1,
		});
	});

	it("returns all zeros for empty array", () => {
		expect(summarizeHealth([])).toEqual({
			healthy: 0,
			warning: 0,
			error: 0,
			inactive: 0,
			unknown: 0,
		});
	});
});
