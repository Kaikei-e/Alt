import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
	BranchSchema,
	EmitTrailOutcomeRequestSchema,
	FootprintSchema,
	GetItemBranchesRequestSchema,
	GetItemBranchesResponseSchema,
	GetTrailRequestSchema,
	GetTrailResponseSchema,
	ResolveBranchRequestSchema,
} from "$lib/gen/alt/knowledge_trail/v1/knowledge_trail_pb";

describe("Knowledge Trail API Contract", () => {
	describe("GetTrailResponse", () => {
		it("conforms to proto schema with footprint fields", () => {
			const response = create(GetTrailResponseSchema, {
				footprints: [
					{
						footprintKey: "open:article:1",
						verb: "read",
						itemKey: "article:1",
						title: "io_uring basics",
						excerpt: "completion queues…",
						tags: ["rust", "async"],
						note: "2nd visit",
						occurredAt: "2026-06-10T09:12:00Z",
						wear: "deep",
						contactCount: 2,
						firstOccurredAt: "2026-06-01T08:00:00Z",
					},
				],
				nextCursor: "cursor_abc",
				hasMore: true,
				generatedAt: "2026-06-10T12:00:00Z",
			});

			expect(response.footprints).toHaveLength(1);
			expect(response.footprints[0]!.verb).toBe("read");
			expect(response.footprints[0]!.tags).toEqual(["rust", "async"]);
			expect(response.footprints[0]!.wear).toBe("deep");
			// D24 collapse fields: repeated contacts arrive as a count, not rows.
			expect(response.footprints[0]!.contactCount).toBe(2);
			expect(response.footprints[0]!.firstOccurredAt).toBe(
				"2026-06-01T08:00:00Z",
			);
			expect(response.hasMore).toBe(true);
		});

		it("carries the theme-lens filter on the request", () => {
			const req = create(GetTrailRequestSchema, {
				filterTags: ["rust"],
				limit: 20,
			});
			expect(req.filterTags).toEqual(["rust"]);
		});

		it("round-trips through proto serialization", () => {
			const original = create(GetTrailResponseSchema, {
				footprints: [
					{
						footprintKey: "ask:1",
						verb: "asked",
						itemKey: "article:2",
						title: "Q",
					},
				],
				nextCursor: "",
				hasMore: false,
			});
			const binary = toBinary(GetTrailResponseSchema, original);
			const deserialized = fromBinary(GetTrailResponseSchema, binary);

			expect(deserialized.footprints).toHaveLength(1);
			expect(deserialized.footprints[0]!.verb).toBe("asked");
			expect(deserialized.hasMore).toBe(false);
		});
	});

	describe("Footprint", () => {
		it("validates each canonical verb", () => {
			for (const verb of [
				"read",
				"asked",
				"returned",
				"listened",
				"dismissed",
			]) {
				const fp = create(FootprintSchema, {
					footprintKey: `${verb}:1`,
					verb,
					itemKey: "article:1",
				});
				expect(fp.verb).toBe(verb);
			}
		});
	});

	describe("Branch", () => {
		it("carries the mandatory four-tuple (kind, why, evidence, confidence)", () => {
			const branch = create(BranchSchema, {
				branchKey: "cluster:u:article:z",
				anchorItemKey: "article:a",
				relationKind: "cluster",
				why: "Joins a topic you follow — shares rust.",
				evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
				confidence: "corroborated",
				targetItemKey: "article:z",
				targetTitle: "Async Rust",
			});
			expect(branch.relationKind).toBe("cluster");
			expect(branch.why).not.toBe("");
			expect(branch.evidenceRefs).toHaveLength(1);
			expect(branch.confidence).toBe("corroborated");
		});

		it("accepts each canonical relation kind", () => {
			for (const kind of [
				"continuation",
				"cluster",
				"contradiction",
				"inquiry",
			]) {
				const b = create(BranchSchema, {
					branchKey: `${kind}:1`,
					relationKind: kind,
				});
				expect(b.relationKind).toBe(kind);
			}
		});
	});

	describe("EmitTrailOutcomeRequest", () => {
		it("carries the taken branch, the opened item, and raw dwell only", () => {
			const req = create(EmitTrailOutcomeRequestSchema, {
				branchKey: "cluster:u:article:z",
				itemKey: "article:z",
				dwellMs: 42000n,
			});
			expect(req.branchKey).toBe("cluster:u:article:z");
			expect(req.itemKey).toBe("article:z");
			expect(req.dwellMs).toBe(42000n);
			// D18: the request has no outcome classification field — raw dwell only.
			expect("outcome" in req).toBe(false);
		});

		it("round-trips dwell through proto serialization", () => {
			const original = create(EmitTrailOutcomeRequestSchema, {
				branchKey: "b",
				itemKey: "article:1",
				dwellMs: 1n,
			});
			const deserialized = fromBinary(
				EmitTrailOutcomeRequestSchema,
				toBinary(EmitTrailOutcomeRequestSchema, original),
			);
			expect(deserialized.dwellMs).toBe(1n);
		});
	});

	describe("ResolveBranchRequest", () => {
		it("carries branch_key, resolution and the idempotency id", () => {
			const req = create(ResolveBranchRequestSchema, {
				branchKey: "cluster:u:article:z",
				resolution: "taken",
				clientResolutionId: "01938e82-7c00-7a7b-9b10-0123456789ab",
			});
			expect(req.branchKey).toBe("cluster:u:article:z");
			expect(req.resolution).toBe("taken");
			expect(req.clientResolutionId).not.toBe("");
		});

		// D28: the one-tap dismiss reason is optional scrutability, never required.
		it("carries the optional one-tap dismiss reason", () => {
			const req = create(ResolveBranchRequestSchema, {
				branchKey: "cluster:u:article:z",
				resolution: "dismissed",
				clientResolutionId: "01938e82-7c00-7a7b-9b10-0123456789ab",
				dismissReason: "not_following_topic",
			});
			expect(req.dismissReason).toBe("not_following_topic");
		});

		it("defaults dismiss_reason to empty for a plain dismiss", () => {
			const req = create(ResolveBranchRequestSchema, {
				branchKey: "cluster:u:article:z",
				resolution: "dismissed",
				clientResolutionId: "01938e82-7c00-7a7b-9b10-0123456789ab",
			});
			expect(req.dismissReason).toBe("");
		});
	});

	// D26: the patch-exit surface — the article page's read-end shows at most
	// 1-2 branches anchored on the item the user just finished reading.
	describe("GetItemBranchesRequest/Response", () => {
		it("requests the branches anchored on one item with a limit", () => {
			const req = create(GetItemBranchesRequestSchema, {
				itemKey: "article:a1",
				limit: 2,
			});
			expect(req.itemKey).toBe("article:a1");
			expect(req.limit).toBe(2);
		});

		it("returns branches carrying the mandatory four-tuple", () => {
			const res = create(GetItemBranchesResponseSchema, {
				branches: [
					{
						branchKey: "cluster:u:article:z",
						anchorItemKey: "article:a1",
						relationKind: "cluster",
						why: "Joins a topic you follow.",
						evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
						confidence: "plausible",
						targetItemKey: "article:z",
						targetTitle: "Async Rust",
					},
				],
			});
			expect(res.branches).toHaveLength(1);
			expect(res.branches[0]!.anchorItemKey).toBe("article:a1");
			expect(res.branches[0]!.why).not.toBe("");
		});

		it("round-trips through proto serialization", () => {
			const original = create(GetItemBranchesResponseSchema, {
				branches: [
					{
						branchKey: "b",
						anchorItemKey: "article:a1",
						relationKind: "inquiry",
						targetItemKey: "article:z",
					},
				],
			});
			const deserialized = fromBinary(
				GetItemBranchesResponseSchema,
				toBinary(GetItemBranchesResponseSchema, original),
			);
			expect(deserialized.branches).toHaveLength(1);
			expect(deserialized.branches[0]!.relationKind).toBe("inquiry");
		});
	});
});
