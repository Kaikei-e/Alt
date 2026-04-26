import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@connectrpc/connect", () => ({
	createClient: vi.fn(),
}));

vi.mock("$lib/gen/alt/knowledge/loop/v1/knowledge_loop_pb", () => ({
	KnowledgeLoopService: {},
	DismissState: { ACTIVE: 1, DEFERRED: 2, DISMISSED: 3, COMPLETED: 4 },
	LoopPriority: { CRITICAL: 1, CONTINUING: 2, CONFIRM: 3, REFERENCE: 4 },
	LoopStage: { OBSERVE: 1, ORIENT: 2, DECIDE: 3, ACT: 4 },
	RenderDepthHint: { FLAT: 1, LIGHT: 2, STRONG: 3, CRITICAL: 4 },
	ServiceQuality: { FULL: 1, DEGRADED: 2, FALLBACK: 3 },
	SurfaceBucket: { NOW: 1, CONTINUE: 2, CHANGED: 3, REVIEW: 4 },
	TransitionTrigger: { USER_TAP: 1, DWELL: 2, KEYBOARD: 3, PROGRAMMATIC: 4 },
	WhyKind: { SOURCE: 1, PATTERN: 2, RECALL: 3, CHANGE: 4 },
	DecisionIntent: {
		UNSPECIFIED: 0,
		OPEN: 1,
		ASK: 2,
		SAVE: 3,
		COMPARE: 4,
		REVISIT: 5,
		SNOOZE: 6,
	},
	ActTargetType: {
		UNSPECIFIED: 0,
		ARTICLE: 1,
		ASK: 2,
		RECAP: 3,
		DIFF: 4,
		CLUSTER: 5,
	},
}));

import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import { getKnowledgeLoop } from "./knowledge_loop";

type MockClient = {
	getKnowledgeLoop: ReturnType<typeof vi.fn>;
};

function makeTs(iso: string): { seconds: bigint; nanos: number } {
	const ms = new Date(iso).getTime();
	return { seconds: BigInt(Math.floor(ms / 1000)), nanos: 0 };
}

describe("knowledge_loop mapProtoEntry — PR-L1 OODA decide/act payload", () => {
	let mockTransport: Transport;
	let mockClient: MockClient;

	beforeEach(() => {
		mockTransport = {} as Transport;
		mockClient = {
			getKnowledgeLoop: vi.fn(),
		};
		(createClient as unknown as ReturnType<typeof vi.fn>).mockReturnValue(
			mockClient as never,
		);
	});

	async function fetchWith(entry: Record<string, unknown>) {
		mockClient.getKnowledgeLoop.mockResolvedValue({
			foregroundEntries: [entry],
			surfaces: [],
			sessionState: undefined,
			overallServiceQuality: 1,
			generatedAt: makeTs("2026-04-23T10:00:00Z"),
			projectionSeqHiwater: 100n,
		});
		const result = await getKnowledgeLoop(mockTransport, "default");
		return result.foregroundEntries[0];
	}

	function baseProtoEntry(overrides: Record<string, unknown> = {}) {
		return {
			entryKey: "article:42",
			sourceItemKey: "article:42",
			proposedStage: 1,
			surfaceBucket: 1,
			projectionRevision: 1n,
			projectionSeqHiwater: 100n,
			sourceEventSeq: 100n,
			freshnessAt: makeTs("2026-04-23T09:50:00Z"),
			dismissState: 1,
			renderDepthHint: 1,
			loopPriority: 4,
			whyPrimary: { kind: 1, text: "New summary", evidenceRefs: [] },
			artifactVersionRef: {},
			whyEvidenceRefs: [],
			changeSummary: undefined,
			continueContext: undefined,
			decisionOptions: [],
			actTargets: [],
			...overrides,
		};
	}

	it("carries change_summary summary + changedFields + previousEntryKey", async () => {
		const mapped = await fetchWith(
			baseProtoEntry({
				changeSummary: {
					summary: "Title tightened",
					changedFields: ["title", "summary"],
					previousEntryKey: "article:old",
				},
			}),
		);
		expect(mapped.changeSummary).toEqual({
			summary: "Title tightened",
			changedFields: ["title", "summary"],
			previousEntryKey: "article:old",
		});
	});

	it("carries continue_context summary + recentActionLabels + lastInteractedAt", async () => {
		const mapped = await fetchWith(
			baseProtoEntry({
				continueContext: {
					summary: "Read 3m ago",
					recentActionLabels: ["scroll_30pct"],
					lastInteractedAt: makeTs("2026-04-20T09:15:00Z"),
				},
			}),
		);
		expect(mapped.continueContext?.summary).toBe("Read 3m ago");
		expect(mapped.continueContext?.recentActionLabels).toEqual([
			"scroll_30pct",
		]);
		expect(mapped.continueContext?.lastInteractedAt).toBe(
			"2026-04-20T09:15:00.000Z",
		);
	});

	it("maps decision_options array with intent enum + optional label", async () => {
		const mapped = await fetchWith(
			baseProtoEntry({
				decisionOptions: [
					{ actionId: "open", intent: 1, label: "Open source" },
					{ actionId: "ask", intent: 2 },
					{ actionId: "save", intent: 3 },
					{ actionId: "dismiss", intent: 6 },
				],
			}),
		);
		expect(mapped.decisionOptions).toEqual([
			{ actionId: "open", intent: "open", label: "Open source" },
			{ actionId: "ask", intent: "ask" },
			{ actionId: "save", intent: "save" },
			{ actionId: "dismiss", intent: "snooze" },
		]);
	});

	it("maps act_targets with target_type enum + optional route", async () => {
		const mapped = await fetchWith(
			baseProtoEntry({
				actTargets: [
					{
						targetType: 1,
						targetRef: "article:42",
						route: "/feeds/article:42",
					},
					{ targetType: 2, targetRef: "entry:42" },
				],
			}),
		);
		expect(mapped.actTargets).toEqual([
			{
				targetType: "article",
				targetRef: "article:42",
				route: "/feeds/article:42",
			},
			{ targetType: "ask", targetRef: "entry:42" },
		]);
	});

	it("leaves the 4 optional fields empty when the proto omits them", async () => {
		const mapped = await fetchWith(baseProtoEntry());
		expect(mapped.changeSummary).toBeUndefined();
		expect(mapped.continueContext).toBeUndefined();
		expect(mapped.decisionOptions).toEqual([]);
		expect(mapped.actTargets).toEqual([]);
	});
});
