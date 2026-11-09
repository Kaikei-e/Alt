import { describe, expect, it } from "vitest";
import {
	evaluateSurfaceStatus,
	formatSurfaceTimestamp,
	getDegradationNoticeText,
} from "./surface-status";

describe("evaluateSurfaceStatus", () => {
	it("returns isCurrent false and no notices when surfaceStatus is absent", () => {
		expect(evaluateSurfaceStatus(undefined)).toEqual({
			isCurrent: false,
			statusNotice: null,
			degradationNotices: [],
		});
		expect(evaluateSurfaceStatus(null)).toEqual({
			isCurrent: false,
			statusNotice: null,
			degradationNotices: [],
		});
	});

	it("returns isCurrent true and no notices when fresh and completed without degradation", () => {
		const result = evaluateSurfaceStatus({
			generatedAt: "2026-09-22T17:00:00Z",
			windowStart: "2026-09-19T17:00:00Z",
			windowEnd: "2026-09-22T17:00:00Z",
			isStale: false,
			staleAfter: "2026-09-23T23:00:00Z",
			latestRun: {
				jobId: "run-1",
				status: "completed",
				kickedAt: "2026-09-22T17:00:00Z",
				updatedAt: "2026-09-22T17:05:00Z",
			},
			degradationReasons: [],
		});

		expect(result).toEqual({
			isCurrent: true,
			statusNotice: null,
			degradationNotices: [],
		});
	});

	it("returns failed notice with fail time and generatedAt when latestRun failed", () => {
		const failIso = "2026-09-22T18:02:00Z";
		const generatedIso = "2026-09-22T12:00:00Z";
		const expectedFail = formatSurfaceTimestamp(failIso);
		const expectedGenerated = formatSurfaceTimestamp(generatedIso);

		const result = evaluateSurfaceStatus({
			generatedAt: generatedIso,
			windowStart: "2026-09-19T12:00:00Z",
			windowEnd: "2026-09-22T12:00:00Z",
			isStale: false,
			staleAfter: "2026-09-23T18:00:00Z",
			latestRun: {
				jobId: "run-failed",
				status: "failed",
				kickedAt: "2026-09-22T18:00:00Z",
				updatedAt: failIso,
			},
			degradationReasons: [],
		});

		expect(result.isCurrent).toBe(true);
		expect(result.statusNotice).toEqual({
			tone: "error",
			text: `The latest update failed at ${expectedFail}. Showing results from ${expectedGenerated}.`,
		});
	});

	it("omits Showing results sentence when latestRun failed and generatedAt is empty", () => {
		const failIso = "2026-09-22T18:02:00Z";
		const expectedFail = formatSurfaceTimestamp(failIso);

		const result = evaluateSurfaceStatus({
			generatedAt: "",
			windowStart: "",
			windowEnd: "",
			isStale: true,
			staleAfter: "",
			latestRun: {
				jobId: "run-failed-empty",
				status: "failed",
				kickedAt: "2026-09-22T18:00:00Z",
				updatedAt: failIso,
			},
			degradationReasons: [],
		});

		expect(result.isCurrent).toBe(false);
		expect(result.statusNotice).toEqual({
			tone: "error",
			text: `The latest update failed at ${expectedFail}.`,
		});
	});

	it("returns generic failed text when latestRun failed and timestamps are unparseable", () => {
		const result = evaluateSurfaceStatus({
			generatedAt: "",
			windowStart: "",
			windowEnd: "",
			isStale: true,
			staleAfter: "",
			latestRun: {
				jobId: "run-failed-bad-time",
				status: "failed",
				kickedAt: "not-a-date",
				updatedAt: "",
			},
			degradationReasons: [],
		});

		expect(result.statusNotice).toEqual({
			tone: "error",
			text: "The latest update failed.",
		});
	});

	it("returns stale notice when isStale is true and latestRun is not failed", () => {
		const generatedIso = "2026-09-20T12:00:00Z";
		const expectedGenerated = formatSurfaceTimestamp(generatedIso);

		const result = evaluateSurfaceStatus({
			generatedAt: generatedIso,
			windowStart: "2026-09-17T12:00:00Z",
			windowEnd: "2026-09-20T12:00:00Z",
			isStale: true,
			staleAfter: "2026-09-21T18:00:00Z",
			latestRun: {
				jobId: "run-old",
				status: "completed",
				kickedAt: "2026-09-20T11:55:00Z",
				updatedAt: generatedIso,
			},
			degradationReasons: [],
		});

		expect(result.isCurrent).toBe(false);
		expect(result.statusNotice).toEqual({
			tone: "warning",
			text: `This was last updated ${expectedGenerated}; a newer update has not completed.`,
		});
	});

	it.each([
		"running",
		"pending",
	] as const)("returns info notice when latestRun.status is %s", (status) => {
		const result = evaluateSurfaceStatus({
			generatedAt: "2026-09-22T17:00:00Z",
			windowStart: "2026-09-19T17:00:00Z",
			windowEnd: "2026-09-22T17:00:00Z",
			isStale: false,
			staleAfter: "2026-09-23T23:00:00Z",
			latestRun: {
				jobId: `run-${status}`,
				status,
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:01:00Z",
			},
			degradationReasons: [],
		});

		expect(result.isCurrent).toBe(true);
		expect(result.statusNotice).toEqual({
			tone: "info",
			text: "Update in progress…",
		});
	});

	it("maps degradationReasons using detail when present and fallback text per code when detail is empty", () => {
		const result = evaluateSurfaceStatus({
			generatedAt: "2026-09-22T17:00:00Z",
			windowStart: "",
			windowEnd: "",
			isStale: false,
			staleAfter: "2026-09-23T23:00:00Z",
			degradationReasons: [
				{
					code: "extractive_genres",
					detail: "9 of 19 genres are extractive excerpts, not AI summaries",
				},
				{
					code: "placeholder_output",
					detail: "",
				},
			],
		});

		expect(result.degradationNotices).toEqual([
			"9 of 19 genres are extractive excerpts, not AI summaries",
			getDegradationNoticeText({ code: "placeholder_output", detail: "" }),
		]);
		expect(result.degradationNotices[1]?.length).toBeGreaterThan(0);
	});
});

describe("getDegradationNoticeText", () => {
	it.each([
		"extractive_genres",
		"few_cards",
		"pulse_fallback",
		"placeholder_output",
		"duplicate_bullets",
		"low_japanese_ratio",
		"stale_recap_input",
		"recap_unavailable",
		"generation_failed",
	])("returns non-empty English fallback text for code %s when detail is empty", (code) => {
		const text = getDegradationNoticeText({ code, detail: "" });
		expect(text.length).toBeGreaterThan(5);
		expect(text).not.toBe(code);
	});

	it("prefers detail when provided", () => {
		expect(
			getDegradationNoticeText({
				code: "few_cards",
				detail: "1 cards selected",
			}),
		).toBe("1 cards selected");
	});
});
