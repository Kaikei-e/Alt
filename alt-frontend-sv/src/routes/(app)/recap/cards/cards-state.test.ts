import { describe, expect, it } from "vitest";
import {
	buildMockDegradedRecapCardsResponse,
	buildMockEmptyRecapCardsResponse,
	buildMockRecapCardsResponse,
} from "../../../../../tests/e2e/fixtures/factories";
import { determineCardsPageState, formatJobWindow } from "./cards-state";

describe("Topic Cards Page State Machine", () => {
	it("returns 'loading' when isLoading is true", () => {
		const state = determineCardsPageState({
			isLoading: true,
			error: null,
			data: null,
		});
		expect(state).toBe("loading");
	});

	it("returns 'error' when error is present", () => {
		const state = determineCardsPageState({
			isLoading: false,
			error: new Error("Network failure"),
			data: null,
		});
		expect(state).toBe("error");
	});

	it("returns 'empty' when data has null job", () => {
		const emptyData = buildMockEmptyRecapCardsResponse();
		const state = determineCardsPageState({
			isLoading: false,
			error: null,
			data: emptyData,
		});
		expect(state).toBe("empty");
	});

	it("returns 'empty' when cards array is empty even if job exists", () => {
		const state = determineCardsPageState({
			isLoading: false,
			error: null,
			data: {
				job: {
					jobId: "123",
					kickedAt: "2026-09-22T00:00:00Z",
					from: "2026-09-19T00:00:00Z",
					to: "2026-09-22T00:00:00Z",
					paramsVersion: "cards-v0.2",
					cardsSelected: 0,
					degraded: false,
				},
				cards: [],
			},
		});
		expect(state).toBe("empty");
	});

	it("returns 'degraded' when job.degraded is true and cards are present", () => {
		const degradedData = buildMockDegradedRecapCardsResponse();
		const state = determineCardsPageState({
			isLoading: false,
			error: null,
			data: degradedData,
		});
		expect(state).toBe("degraded");
	});

	it("returns 'populated' when job is not degraded and cards are present", () => {
		const populatedData = buildMockRecapCardsResponse();
		const state = determineCardsPageState({
			isLoading: false,
			error: null,
			data: populatedData,
		});
		expect(state).toBe("populated");
	});
});

describe("formatJobWindow", () => {
	it("formats from and to into a readable date range", () => {
		const from = "2026-09-19T17:00:00Z";
		const to = "2026-09-22T17:00:00Z";
		const expectedFrom = new Date(from).toLocaleDateString("en-US", {
			month: "short",
			day: "numeric",
			year: "numeric",
		});
		const expectedTo = new Date(to).toLocaleDateString("en-US", {
			month: "short",
			day: "numeric",
			year: "numeric",
		});
		const formatted = formatJobWindow(from, to);
		expect(formatted).toBe(`${expectedFrom} – ${expectedTo}`);
	});

	it("handles same year or cross-year dates gracefully", () => {
		const formatted = formatJobWindow(
			"2025-12-30T00:00:00Z",
			"2026-01-02T00:00:00Z",
		);
		expect(formatted).toContain("Dec 30");
		expect(formatted).toContain("Jan 2");
	});

	it("returns empty string if invalid dates are passed", () => {
		expect(formatJobWindow("", "")).toBe("");
	});
});
