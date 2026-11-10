import { describe, expect, it } from "vitest";
import { convertProtoSurfaceStatus } from "#lib/connect/recap.js";
import type { RecapSurfaceStatus as ProtoRecapSurfaceStatus } from "#lib/gen/alt/recap/v2/surface_status_pb.js";
import {
	evaluateSurfaceStatus,
	formatSurfaceTimestamp,
	getDegradationNoticeText,
} from "./surface-status";

function expectedLocaleTimestamp(iso: string): string {
	return new Date(iso).toLocaleString("en-US", {
		month: "short",
		day: "numeric",
		hour: "numeric",
		minute: "2-digit",
	});
}

describe("formatSurfaceTimestamp", () => {
	it("formats valid ISO timestamp to locale date/time string", () => {
		const iso = "2026-09-22T17:00:00Z";
		expect(formatSurfaceTimestamp(iso)).toBe(expectedLocaleTimestamp(iso));
	});

	it("returns empty string on empty or invalid timestamp", () => {
		expect(formatSurfaceTimestamp("")).toBe("");
		expect(formatSurfaceTimestamp("invalid")).toBe("");
	});
});

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
		const expectedFail = expectedLocaleTimestamp(failIso);
		const expectedGenerated = expectedLocaleTimestamp(generatedIso);

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
		const expectedFail = expectedLocaleTimestamp(failIso);

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
		const expectedGenerated = expectedLocaleTimestamp(generatedIso);

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
	] as const)("prioritises stale warning over %s status even when generatedAt is empty", (status) => {
		const generatedIso = "2026-09-20T12:00:00Z";
		const expectedGenerated = expectedLocaleTimestamp(generatedIso);

		const withGenerated = evaluateSurfaceStatus({
			generatedAt: generatedIso,
			windowStart: "2026-09-17T12:00:00Z",
			windowEnd: "2026-09-20T12:00:00Z",
			isStale: true,
			staleAfter: "2026-09-21T18:00:00Z",
			latestRun: {
				jobId: `run-stale-${status}`,
				status,
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:01:00Z",
			},
			degradationReasons: [],
		});

		expect(withGenerated.statusNotice).toEqual({
			tone: "warning",
			text: `This was last updated ${expectedGenerated}; a newer update has not completed.`,
		});

		const withoutGenerated = evaluateSurfaceStatus({
			generatedAt: "",
			windowStart: "",
			windowEnd: "",
			isStale: true,
			staleAfter: "",
			latestRun: {
				jobId: `run-stale-empty-${status}`,
				status,
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:01:00Z",
			},
			degradationReasons: [],
		});

		expect(withoutGenerated.statusNotice).toEqual({
			tone: "warning",
			text: "This edition is out of date; a newer update has not completed.",
		});
	});

	it.each([
		"running",
		"pending",
	] as const)("returns info notice when latestRun.status is %s and surface is not stale", (status) => {
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

	it("maps degradationReasons using detail alone for extractive_genres and per-code sentence with parenthesised detail for other codes", () => {
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
					code: "few_cards",
					detail: "3 cards selected",
				},
				{
					code: "placeholder_output",
					detail: "",
				},
			],
		});

		expect(result.degradationNotices).toEqual([
			"9 of 19 genres are extractive excerpts, not AI summaries.",
			"Fewer topic cards than usual met the quality threshold (3 cards selected).",
			"The AI draft had empty or placeholder sections, so this letter shows extracted excerpts instead.",
		]);
	});
});

describe("getDegradationNoticeText", () => {
	it.each([
		[
			"extractive_genres",
			"Some genres use extractive excerpts instead of AI summaries.",
		],
		["few_cards", "Fewer topic cards than usual met the quality threshold."],
		["pulse_fallback", "Topics were chosen with relaxed selection criteria."],
		[
			"placeholder_output",
			"The AI draft had empty or placeholder sections, so this letter shows extracted excerpts instead.",
		],
		[
			"duplicate_bullets",
			"The AI draft repeated points across sections, so this letter shows extracted excerpts instead.",
		],
		[
			"low_japanese_ratio",
			"The AI draft was not mostly in Japanese, so this letter shows extracted excerpts instead.",
		],
		[
			"stale_recap_input",
			"Built from a 3-day recap older than the freshness window.",
		],
		["recap_unavailable", "Built without a 3-day recap."],
		["generation_failed", "This edition was produced in degraded mode."],
	] as const)("returns exact text for code %s when detail is empty", (code, expected) => {
		expect(getDegradationNoticeText({ code, detail: "" })).toBe(expected);
	});

	it("returns detail sentence alone with a trailing period for extractive_genres when detail is non-empty", () => {
		expect(
			getDegradationNoticeText({
				code: "extractive_genres",
				detail: "9 of 19 genres are extractive excerpts, not AI summaries",
			}),
		).toBe("9 of 19 genres are extractive excerpts, not AI summaries.");

		expect(
			getDegradationNoticeText({
				code: "extractive_genres",
				detail: "9 of 19 genres are extractive excerpts, not AI summaries.",
			}),
		).toBe("9 of 19 genres are extractive excerpts, not AI summaries.");
	});

	it("appends detail in parentheses after per-code text for other codes when detail is non-empty", () => {
		expect(
			getDegradationNoticeText({
				code: "few_cards",
				detail: "3 cards selected",
			}),
		).toBe(
			"Fewer topic cards than usual met the quality threshold (3 cards selected).",
		);

		expect(
			getDegradationNoticeText({
				code: "pulse_fallback",
				detail: "fallback level 2",
			}),
		).toBe(
			"Topics were chosen with relaxed selection criteria (fallback level 2).",
		);
	});
});

describe("convertProtoSurfaceStatus", () => {
	it("maps an unknown latestRun.status to latestRun: null and keeps the rest of the status", () => {
		const proto = {
			$typeName: "alt.recap.v2.RecapSurfaceStatus",
			generatedAt: "2026-09-22T17:00:00Z",
			windowStart: "2026-09-19T17:00:00Z",
			windowEnd: "2026-09-22T17:00:00Z",
			isStale: true,
			staleAfter: "2026-09-23T05:00:00Z",
			latestRun: {
				$typeName: "alt.recap.v2.SurfaceRun",
				jobId: "run-unknown",
				status: "unknown_status",
				kickedAt: "2026-09-22T17:01:00Z",
				updatedAt: "2026-09-22T17:02:00Z",
			},
			degradationReasons: [
				{
					$typeName: "alt.recap.v2.DegradationReason",
					code: "few_cards",
					detail: "3 cards selected",
				},
			],
		} as unknown as ProtoRecapSurfaceStatus;

		expect(convertProtoSurfaceStatus(proto)).toEqual({
			generatedAt: "2026-09-22T17:00:00Z",
			windowStart: "2026-09-19T17:00:00Z",
			windowEnd: "2026-09-22T17:00:00Z",
			isStale: true,
			staleAfter: "2026-09-23T05:00:00Z",
			latestRun: null,
			degradationReasons: [
				{
					code: "few_cards",
					detail: "3 cards selected",
				},
			],
		});
	});
});
