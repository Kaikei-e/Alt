import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
	GetTrailResponseSchema,
	FootprintSchema,
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
					},
				],
				nextCursor: "cursor_abc",
				hasMore: true,
				generatedAt: "2026-06-10T12:00:00Z",
			});

			expect(response.footprints).toHaveLength(1);
			expect(response.footprints[0].verb).toBe("read");
			expect(response.footprints[0].tags).toEqual(["rust", "async"]);
			expect(response.hasMore).toBe(true);
		});

		it("round-trips through proto serialization", () => {
			const original = create(GetTrailResponseSchema, {
				footprints: [
					{ footprintKey: "ask:1", verb: "asked", itemKey: "article:2", title: "Q" },
				],
				nextCursor: "",
				hasMore: false,
			});
			const binary = toBinary(GetTrailResponseSchema, original);
			const deserialized = fromBinary(GetTrailResponseSchema, binary);

			expect(deserialized.footprints).toHaveLength(1);
			expect(deserialized.footprints[0].verb).toBe("asked");
			expect(deserialized.hasMore).toBe(false);
		});
	});

	describe("Footprint", () => {
		it("validates each canonical verb", () => {
			for (const verb of ["read", "asked", "returned", "listened", "dismissed"]) {
				const fp = create(FootprintSchema, {
					footprintKey: `${verb}:1`,
					verb,
					itemKey: "article:1",
				});
				expect(fp.verb).toBe(verb);
			}
		});
	});
});
