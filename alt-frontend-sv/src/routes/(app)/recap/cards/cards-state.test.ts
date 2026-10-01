import { describe, expect, it } from "vitest";
import {
	buildMockDegradedRecapCardsResponse,
	buildMockEmptyRecapCardsResponse,
	buildMockRecapCardsResponse,
} from "../../../../../tests/e2e/fixtures/factories";
import {
	computeWindowLabel,
	determineCardsPageState,
	formatCardTimestamp,
	formatJobWindow,
	getCardsEmptyMessage,
	getTopicCardsNotice,
} from "./cards-state";

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

describe("computeWindowLabel", () => {
	it("returns '72-hour window' for a 72-hour window", () => {
		const from = "2026-09-19T17:00:00Z";
		const to = "2026-09-22T17:00:00Z";
		expect(computeWindowLabel(from, to)).toBe("72-hour window");
	});

	it("returns '24-hour window' for a 24-hour window", () => {
		const from = "2026-09-21T17:00:00Z";
		const to = "2026-09-22T17:00:00Z";
		expect(computeWindowLabel(from, to)).toBe("24-hour window");
	});

	it("returns '1-hour window' for a 1-hour window", () => {
		const from = "2026-09-22T16:00:00Z";
		const to = "2026-09-22T17:00:00Z";
		expect(computeWindowLabel(from, to)).toBe("1-hour window");
	});

	it("returns empty string when dates are invalid or missing", () => {
		expect(computeWindowLabel("", "")).toBe("");
		expect(computeWindowLabel("invalid", "dates")).toBe("");
		expect(
			computeWindowLabel("2026-09-22T00:00:00Z", "2026-09-20T00:00:00Z"),
		).toBe("");
	});
});

describe("formatCardTimestamp", () => {
	it("formats valid ISO timestamp to locale date/time string", () => {
		const formatted = formatCardTimestamp("2026-09-22T17:00:00Z");
		expect(formatted).toBeTruthy();
		// Should contain month and day
		expect(formatted).toMatch(/[A-Za-z]{3}\s+\d+/);
	});

	it("returns empty string on empty or invalid timestamp", () => {
		expect(formatCardTimestamp("")).toBe("");
		expect(formatCardTimestamp("invalid")).toBe("");
	});
});

describe("getCardsEmptyMessage", () => {
	it("returns 'No topic cards were selected in the latest run.' when hasJob is true", () => {
		expect(getCardsEmptyMessage(true)).toBe(
			"No topic cards were selected in the latest run.",
		);
	});

	it("returns 'Topic cards will appear here after the first daily run.' when hasJob is false", () => {
		expect(getCardsEmptyMessage(false)).toBe(
			"Topic cards will appear here after the first daily run.",
		);
	});
});

describe("getTopicCardsNotice", () => {
	it("returns null when latestRun is missing", () => {
		expect(getTopicCardsNotice({})).toBeNull();
		expect(
			getTopicCardsNotice({
				job: { kickedAt: "2026-09-22T17:00:00Z" },
				latestRun: null,
			}),
		).toBeNull();
	});

	it("returns null when latestRun status is completed", () => {
		const notice = getTopicCardsNotice({
			job: { kickedAt: "2026-09-22T17:00:00Z" },
			latestRun: {
				jobId: "run-1",
				status: "completed",
				kickedAt: "2026-09-22T17:00:00Z",
				updatedAt: "2026-09-22T17:05:00Z",
			},
		});
		expect(notice).toBeNull();
	});

	it("returns null when latestRun status is an unknown value", () => {
		const notice = getTopicCardsNotice({
			job: { kickedAt: "2026-09-22T17:00:00Z" },
			latestRun: {
				jobId: "run-unknown",
				status: "unknown_status",
				kickedAt: "2026-09-22T17:00:00Z",
				updatedAt: "2026-09-22T17:05:00Z",
			},
		});
		expect(notice).toBeNull();
	});

	it("returns running info notice with time when latestRun status is running", () => {
		const notice = getTopicCardsNotice({
			job: null,
			latestRun: {
				jobId: "run-running",
				status: "running",
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:01:00Z",
			},
		});
		expect(notice).not.toBeNull();
		expect(notice?.tone).toBe("info");
		expect(notice?.text).toContain("Update in progress (started ");
		expect(notice?.text).toMatch(/\)\.$/);
	});

	it("returns pending info notice with time when latestRun status is pending", () => {
		const notice = getTopicCardsNotice({
			job: null,
			latestRun: {
				jobId: "run-pending",
				status: "pending",
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:00:30Z",
			},
		});
		expect(notice).not.toBeNull();
		expect(notice?.tone).toBe("info");
		expect(notice?.text).toContain("Update in progress (started ");
		expect(notice?.text).toMatch(/\)\.$/);
	});

	it("drops time part and prints 'Update in progress.' when kickedAt is missing or unparseable", () => {
		const noticeMissing = getTopicCardsNotice({
			job: null,
			latestRun: {
				jobId: "run-running",
				status: "running",
				kickedAt: "",
				updatedAt: "",
			},
		});
		expect(noticeMissing).toEqual({
			tone: "info",
			text: "Update in progress.",
		});

		const noticeUnparseable = getTopicCardsNotice({
			job: null,
			latestRun: {
				jobId: "run-pending",
				status: "pending",
				kickedAt: "not-a-date",
				updatedAt: "",
			},
		});
		expect(noticeUnparseable).toEqual({
			tone: "info",
			text: "Update in progress.",
		});
	});

	it("returns failed error notice without job when job is null", () => {
		const notice = getTopicCardsNotice({
			job: null,
			latestRun: {
				jobId: "run-failed",
				status: "failed",
				kickedAt: "2026-09-22T18:00:00Z",
				updatedAt: "2026-09-22T18:02:00Z",
			},
		});
		expect(notice).not.toBeNull();
		expect(notice?.tone).toBe("error");
		expect(notice?.text).toContain("The latest update failed at ");
		expect(notice?.text).not.toContain("Showing cards from");
	});

	it("returns failed error notice without time when updatedAt and kickedAt are missing or unparseable", () => {
		const notice = getTopicCardsNotice({
			job: null,
			latestRun: {
				jobId: "run-failed",
				status: "failed",
				kickedAt: "unparseable",
				updatedAt: "",
			},
		});
		expect(notice).toEqual({
			tone: "error",
			text: "The latest update failed.",
		});
	});

	it("returns failed error notice with cards context when job exists and cards.length > 0", () => {
		const notice = getTopicCardsNotice({
			job: { kickedAt: "2026-09-22T12:00:00Z" },
			cards: [{ id: "card-1" }],
			latestRun: {
				jobId: "run-failed",
				status: "failed",
				kickedAt: "2026-09-22T18:00:00Z",
				updatedAt: "2026-09-22T18:02:00Z",
			},
		});
		expect(notice).not.toBeNull();
		expect(notice?.tone).toBe("error");
		expect(notice?.text).toContain("The latest update failed at ");
		expect(notice?.text).toContain(" Showing cards from ");
	});

	it("does not append 'Showing cards from' when job exists but cards is empty", () => {
		const notice = getTopicCardsNotice({
			job: { kickedAt: "2026-09-22T12:00:00Z" },
			cards: [],
			latestRun: {
				jobId: "run-failed",
				status: "failed",
				kickedAt: "2026-09-22T18:00:00Z",
				updatedAt: "2026-09-22T18:02:00Z",
			},
		});
		expect(notice).not.toBeNull();
		expect(notice?.tone).toBe("error");
		expect(notice?.text).toContain("The latest update failed at ");
		expect(notice?.text).not.toContain("Showing cards from");
	});

	it("shows failure notice whenever latestRun.status is failed without timestamp compare", () => {
		const notice = getTopicCardsNotice({
			job: { kickedAt: "2026-09-22T19:00:00Z" },
			cards: [{ id: "card-1" }],
			latestRun: {
				jobId: "run-failed-older-timestamp",
				status: "failed",
				kickedAt: "2026-09-22T18:00:00Z",
				updatedAt: "2026-09-22T18:02:00Z",
			},
		});
		expect(notice).not.toBeNull();
		expect(notice?.tone).toBe("error");
		expect(notice?.text).toContain("The latest update failed at ");
		expect(notice?.text).toContain(" Showing cards from ");
	});

	it("returns null when job exists with zero cards and no failure/running run", () => {
		const notice = getTopicCardsNotice({
			job: { kickedAt: "2026-09-22T12:00:00Z" },
			cards: [],
			latestRun: null,
		});
		expect(notice).toBeNull();
	});
});
