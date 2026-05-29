import { describe, it, expect } from "vitest";
import type { LoopStageName } from "$lib/connect/knowledge_loop";
import {
	canTransition,
	transitionReason,
	type TransitionTrigger,
} from "./loop-transitions";

const stages: LoopStageName[] = ["observe", "orient", "decide", "act"];

describe("canTransition — ADR-000831 §7 allowlist", () => {
	it.each([
		["observe", "orient"],
		["observe", "decide"],
		// Boyd implicit guidance & control: observe/orient commit straight to act.
		["observe", "act"],
		["orient", "observe"],
		["orient", "decide"],
		["orient", "act"],
		["decide", "act"],
		["act", "observe"],
	] as const)("allows %s → %s", (from, to) => {
		expect(canTransition(from, to)).toBe(true);
	});

	it.each([
		["act", "act"],
		["act", "orient"],
		["act", "decide"],
		["decide", "observe"],
		["decide", "orient"],
	] as const)("forbids %s → %s", (from, to) => {
		expect(canTransition(from, to)).toBe(false);
	});

	it("forbids same-stage transition when no trigger is supplied", () => {
		for (const s of stages) {
			expect(canTransition(s, s)).toBe(false);
		}
	});

	it("transitionReason returns a short human-readable string for forbidden pairs", () => {
		expect(transitionReason("decide", "observe")).toMatch(/not available/i);
		expect(transitionReason("observe", "orient")).toBe("");
	});
});

describe("canTransition — ADR-000914 same-stage triggers", () => {
	// Mirrors proto/alt/knowledge/loop/v1/loop_transition_policy.json
	// `same_stage_triggers`. Conformance is also exercised by
	// loop-transitions.policy.test.ts.
	const sameStageAllowed: TransitionTrigger[] = [
		"defer",
		"recheck",
		"archive",
		"mark_reviewed",
		"compare",
		"internalize",
		"intent_signal",
	];
	const crossStageOnly: TransitionTrigger[] = [
		"user_tap",
		"dwell",
		"keyboard",
		"programmatic",
	];

	it.each(
		sameStageAllowed,
	)("allows same-stage transition when trigger is %s", (trigger) => {
		for (const s of stages) {
			expect(canTransition(s, s, trigger)).toBe(true);
		}
	});

	it.each(
		crossStageOnly,
	)("forbids same-stage transition when trigger is %s (cross-stage trigger)", (trigger) => {
		for (const s of stages) {
			expect(canTransition(s, s, trigger)).toBe(false);
		}
	});

	it("cross-stage transitions stay gated by the allowlist regardless of trigger", () => {
		for (const trigger of [...sameStageAllowed, ...crossStageOnly]) {
			expect(canTransition("orient", "observe", trigger)).toBe(true);
			expect(canTransition("decide", "observe", trigger)).toBe(false);
		}
	});

	it("transitionReason treats trigger-eligible same-stage as valid", () => {
		expect(transitionReason("orient", "orient", "intent_signal")).toBe("");
		expect(transitionReason("decide", "decide", "compare")).toBe("");
		expect(transitionReason("act", "act", "user_tap")).toMatch(
			/not available/i,
		);
	});
});
