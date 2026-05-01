import { describe, it, expect } from "vitest";
import { classifyLoopStreamFrame } from "./loop-stream-frames";
import {
	DismissState,
	LoopPriority,
	LoopStage,
	KnowledgeLoopEntrySchema,
	RenderDepthHint,
	StreamKnowledgeLoopUpdatesResponseSchema,
	SurfaceBucket,
	WhyKind,
	type StreamKnowledgeLoopUpdatesResponse,
} from "$lib/gen/alt/knowledge/loop/v1/knowledge_loop_pb";
import { create } from "@bufbuild/protobuf";

// Minimal shape helpers — we only build the fields the classifier inspects.
// Casting to the generated type is safe: proto-es is a tagged union at runtime
// and the classifier reads only `update.case` + `update.value` + `projectionSeqHiwater`.
function msg(
	update: StreamKnowledgeLoopUpdatesResponse["update"],
	seq = 42n,
): StreamKnowledgeLoopUpdatesResponse {
	return {
		update,
		projectionSeqHiwater: seq,
		$typeName: "alt.knowledge.loop.v1.StreamKnowledgeLoopUpdatesResponse",
	} as StreamKnowledgeLoopUpdatesResponse;
}

describe("classifyLoopStreamFrame", () => {
	it("maps empty update to a heartbeat frame carrying seq hiwater", () => {
		const frame = classifyLoopStreamFrame(
			msg({ case: undefined, value: undefined }, 7n),
		);
		expect(frame).toEqual({ kind: "heartbeat", projectionSeqHiwater: 7n });
	});

	it("maps appended oneof to entry-appended frame", () => {
		const frame = classifyLoopStreamFrame(
			msg({
				case: "appended",
				value: {
					entryKey: "article:42",
					revision: 10n,
					$typeName: "alt.knowledge.loop.v1.EntryAppended",
				},
			}),
		);
		expect(frame?.kind).toBe("appended");
		if (frame?.kind === "appended") {
			expect(frame.entryKey).toBe("article:42");
			expect(frame.revision).toBe(10n);
		}
	});

	it("maps appended inline_entry into frontend entry data", () => {
		const inline = create(KnowledgeLoopEntrySchema, {
			entryKey: "article:42",
			sourceItemKey: "article:42",
			proposedStage: LoopStage.OBSERVE,
			surfaceBucket: SurfaceBucket.NOW,
			projectionRevision: 2n,
			projectionSeqHiwater: 42n,
			sourceEventSeq: 42n,
			freshnessAt: { seconds: 1_777_000_000n, nanos: 0 },
			dismissState: DismissState.ACTIVE,
			renderDepthHint: RenderDepthHint.LIGHT,
			loopPriority: LoopPriority.CRITICAL,
			whyPrimary: {
				kind: WhyKind.SOURCE,
				text: "Ready to read.",
				evidenceRefs: [],
			},
			decisionOptions: [],
			actTargets: [],
		});
		const frame = classifyLoopStreamFrame(
			create(StreamKnowledgeLoopUpdatesResponseSchema, {
				update: {
					case: "appended",
					value: {
						entryKey: "article:42",
						revision: 42n,
						inlineEntry: inline,
					},
				},
				projectionSeqHiwater: 42n,
			}),
		);
		expect(frame?.kind).toBe("appended");
		if (frame?.kind === "appended") {
			expect(frame.inlineEntry?.entryKey).toBe("article:42");
			expect(frame.inlineEntry?.whyPrimary.text).toBe("Ready to read.");
		}
	});

	it("maps revised oneof to entry-revised frame (silent update)", () => {
		const frame = classifyLoopStreamFrame(
			msg({
				case: "revised",
				value: {
					entryKey: "article:42",
					revision: 11n,
					$typeName: "alt.knowledge.loop.v1.EntryRevised",
				},
			}),
		);
		expect(frame?.kind).toBe("revised");
	});

	it("preserves new_entry_key on superseded frame so UI can render the badge link", () => {
		const frame = classifyLoopStreamFrame(
			msg({
				case: "superseded",
				value: {
					entryKey: "article:42",
					newEntryKey: "article:43",
					revision: 12n,
					$typeName: "alt.knowledge.loop.v1.EntrySuperseded",
				},
			}),
		);
		expect(frame?.kind).toBe("superseded");
		if (frame?.kind === "superseded") {
			expect(frame.newEntryKey).toBe("article:43");
		}
	});

	it("maps withdrawn oneof", () => {
		const frame = classifyLoopStreamFrame(
			msg({
				case: "withdrawn",
				value: {
					entryKey: "article:42",
					revision: 13n,
					$typeName: "alt.knowledge.loop.v1.EntryWithdrawn",
				},
			}),
		);
		expect(frame?.kind).toBe("withdrawn");
	});

	it("maps rebalanced oneof to a surface-bucket frame", () => {
		const frame = classifyLoopStreamFrame(
			msg({
				case: "rebalanced",
				value: {
					surfaceBucket: 1, // SURFACE_BUCKET_NOW
					revision: 14n,
					$typeName: "alt.knowledge.loop.v1.SurfaceRebalanced",
				},
			}),
		);
		expect(frame?.kind).toBe("rebalanced");
		if (frame?.kind === "rebalanced") {
			expect(frame.surfaceBucket).toBe(1);
		}
	});

	it("maps streamExpired oneof to an expired frame carrying the reason", () => {
		const frame = classifyLoopStreamFrame(
			msg({
				case: "streamExpired",
				value: {
					reason: "stale",
					$typeName: "alt.knowledge.loop.v1.StreamExpired",
				},
			}),
		);
		expect(frame).toEqual({ kind: "expired", reason: "stale" });
	});
});
