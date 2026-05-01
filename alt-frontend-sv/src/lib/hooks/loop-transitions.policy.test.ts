import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { canTransition } from "./loop-transitions";

/**
 * Conformance test pinning the FE allowlist (loop-transitions.ts) to the
 * canonical policy at proto/alt/knowledge/loop/v1/loop_transition_policy.json.
 *
 * If you add or remove an edge, edit the policy JSON first; this test will
 * walk you to the implementation that needs to match.
 *
 * ADR-000876 — single-source-of-truth for the OODA transition matrix.
 */

interface PolicyEdge {
	from: string;
	to: string;
	triggers?: string[];
}

interface TransitionPolicy {
	version: number;
	allowed_edges: PolicyEdge[];
	same_stage_triggers: string[];
	forbidden_edges: PolicyEdge[];
}

function loadPolicy(): TransitionPolicy {
	// Walk up from this test until we find proto/alt/knowledge/loop/v1/.
	// Mirrors the strategy used by transition_policy_test.go in alt-backend.
	const here = fileURLToPath(new URL(".", import.meta.url));
	let dir = here;
	for (let i = 0; i < 12; i++) {
		try {
			const path = `${dir}/proto/alt/knowledge/loop/v1/loop_transition_policy.json`;
			const body = readFileSync(path, "utf-8");
			return JSON.parse(body) as TransitionPolicy;
		} catch {
			// step up
		}
		dir = `${dir}/..`;
	}
	throw new Error(
		"could not locate proto/alt/knowledge/loop/v1/loop_transition_policy.json",
	);
}

const policy = loadPolicy();

type Stage = "observe" | "orient" | "decide" | "act";
const ALL_STAGES: Stage[] = ["observe", "orient", "decide", "act"];

describe("loop-transitions ↔ loop_transition_policy.json conformance", () => {
	it("uses policy version 1", () => {
		expect(policy.version).toBe(1);
	});

	it("accepts every allowed_edges entry", () => {
		for (const edge of policy.allowed_edges) {
			expect(canTransition(edge.from as Stage, edge.to as Stage)).toBe(true);
		}
	});

	it("rejects every forbidden_edges entry", () => {
		for (const edge of policy.forbidden_edges) {
			expect(canTransition(edge.from as Stage, edge.to as Stage)).toBe(false);
		}
	});

	it("matches the policy exactly — no extra allow / no missing reject", () => {
		const allowed = new Set(
			policy.allowed_edges.map((e) => `${e.from}->${e.to}`),
		);
		for (const from of ALL_STAGES) {
			for (const to of ALL_STAGES) {
				if (from === to) continue;
				const expected = allowed.has(`${from}->${to}`);
				expect(canTransition(from, to)).toBe(expected);
			}
		}
	});

	it("declares the same-stage triggers the BE classifier accepts", () => {
		expect(policy.same_stage_triggers).toEqual(
			expect.arrayContaining(["defer", "recheck", "archive", "mark_reviewed"]),
		);
	});
});
