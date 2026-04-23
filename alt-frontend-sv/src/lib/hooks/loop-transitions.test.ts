import { describe, it, expect } from "vitest";
import type { LoopStageName } from "$lib/connect/knowledge_loop";
import { canTransition, transitionReason } from "./loop-transitions";

const stages: LoopStageName[] = ["observe", "orient", "decide", "act"];

describe("canTransition — ADR-000831 §7 allowlist", () => {
	it.each([
		["observe", "orient"],
		["observe", "decide"],
		["orient", "decide"],
		["decide", "act"],
		["act", "observe"],
	] as const)("allows %s → %s", (from, to) => {
		expect(canTransition(from, to)).toBe(true);
	});

	it.each([
		["observe", "act"],
		["act", "act"],
		["act", "orient"],
		["act", "decide"],
		["decide", "observe"],
		["orient", "observe"],
		["decide", "orient"],
	] as const)("forbids %s → %s", (from, to) => {
		expect(canTransition(from, to)).toBe(false);
	});

	it("forbids same-stage idempotent transition", () => {
		for (const s of stages) {
			expect(canTransition(s, s)).toBe(false);
		}
	});

	it("transitionReason returns a short human-readable string for forbidden pairs", () => {
		expect(transitionReason("observe", "act")).toMatch(/not available/i);
		expect(transitionReason("observe", "orient")).toBe("");
	});
});
