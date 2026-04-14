import { describe, expect, test } from "vitest";
import {
	deriveRunStatusKind,
	RUN_STATUS_LABELS,
	type RunStatusKind,
} from "./runStatusPill";

describe("deriveRunStatusKind", () => {
	test("idle: no versions and no run", () => {
		expect(
			deriveRunStatusKind({
				runStatus: null,
				pendingUpdate: false,
				currentVersion: 0,
			}),
		).toBe<RunStatusKind>("idle");
	});

	test("ready: report has versions and no active run and no pending refresh", () => {
		expect(
			deriveRunStatusKind({
				runStatus: null,
				pendingUpdate: false,
				currentVersion: 3,
			}),
		).toBe<RunStatusKind>("ready");
	});

	test("generating: pending run", () => {
		expect(
			deriveRunStatusKind({
				runStatus: "pending",
				pendingUpdate: false,
				currentVersion: 3,
			}),
		).toBe<RunStatusKind>("generating");
	});

	test("generating: running run", () => {
		expect(
			deriveRunStatusKind({
				runStatus: "running",
				pendingUpdate: false,
				currentVersion: 3,
			}),
		).toBe<RunStatusKind>("generating");
	});

	test("completed: pendingUpdate flag is set", () => {
		expect(
			deriveRunStatusKind({
				runStatus: null,
				pendingUpdate: true,
				currentVersion: 3,
			}),
		).toBe<RunStatusKind>("completed");
	});

	test("completed: succeeded status even before banner acknowledgement", () => {
		expect(
			deriveRunStatusKind({
				runStatus: "succeeded",
				pendingUpdate: false,
				currentVersion: 3,
			}),
		).toBe<RunStatusKind>("completed");
	});

	test("failed: failed run overrides ready/pending computation", () => {
		expect(
			deriveRunStatusKind({
				runStatus: "failed",
				pendingUpdate: false,
				currentVersion: 3,
			}),
		).toBe<RunStatusKind>("failed");
	});

	test("cancelled: cancelled run", () => {
		expect(
			deriveRunStatusKind({
				runStatus: "cancelled",
				pendingUpdate: false,
				currentVersion: 3,
			}),
		).toBe<RunStatusKind>("cancelled");
	});

	test("generating wins over pendingUpdate (re-run started after previous completion)", () => {
		expect(
			deriveRunStatusKind({
				runStatus: "running",
				pendingUpdate: true,
				currentVersion: 3,
			}),
		).toBe<RunStatusKind>("generating");
	});
});

describe("RUN_STATUS_LABELS", () => {
	test("every kind has a label", () => {
		const kinds: RunStatusKind[] = [
			"idle",
			"ready",
			"generating",
			"completed",
			"failed",
			"cancelled",
		];
		for (const kind of kinds) {
			expect(RUN_STATUS_LABELS[kind]).toBeTruthy();
		}
	});

	test("labels are English functional words", () => {
		expect(RUN_STATUS_LABELS.generating).toBe("Generating");
		expect(RUN_STATUS_LABELS.completed).toBe("Updated");
		expect(RUN_STATUS_LABELS.failed).toBe("Failed");
		expect(RUN_STATUS_LABELS.ready).toBe("Ready");
	});
});
