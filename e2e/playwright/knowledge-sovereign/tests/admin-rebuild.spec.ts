import { z } from "zod";
import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";

const rebuildTargetsSchema = z
	.object({
		targets: z
			.array(
				z.object({
					target: z.string().min(1),
					tables: z.array(z.string().min(1)).min(1),
					projector_name: z.string().min(1),
				}),
			)
			.min(2),
	})
	.passthrough();

const rebuildResultSchema = z
	.object({
		target: z.string().min(1),
		tables: z.array(z.string().min(1)).min(1),
		tables_truncated: z.number().int().positive(),
		projector_name: z.string().min(1),
		checkpoint_before: z.number().int().nonnegative(),
	})
	.passthrough();

test.describe("projection rebuild admin endpoints", () => {
	test(
		"GET /admin/projections/rebuild/targets lists allowlisted rebuild targets",
		{ tag: "@contract" },
		async ({ admin }) => {
			const body = await expectJsonStatus(
				await admin.get("/admin/projections/rebuild/targets"),
				200,
				rebuildTargetsSchema,
			);
			const names = body.targets.map((t) => t.target);
			expect(names).toEqual(expect.arrayContaining(["knowledge-home", "knowledge-trail"]));
		},
	);

	test(
		"POST /admin/projections/rebuild rejects unknown or missing targets with 400",
		{ tag: "@contract" },
		async ({ admin }) => {
			const badTarget = await admin.post("/admin/projections/rebuild", {
				data: { target: "knowledge_events" },
			});
			expect(badTarget.status()).toBe(400);

			const emptyTarget = await admin.post("/admin/projections/rebuild", {
				data: {},
			});
			expect(emptyTarget.status()).toBe(400);
		},
	);

	test(
		"POST /admin/projections/rebuild truncates and resets checkpoint for an allowlisted target",
		{ tag: "@contract" },
		async ({ admin }) => {
			const body = await expectJsonStatus(
				await admin.post("/admin/projections/rebuild", {
					data: { target: "knowledge-trail" },
				}),
				200,
				rebuildResultSchema,
			);
			expect(body.target).toBe("knowledge-trail");
			expect(body.projector_name).toBe("knowledge-trail-projector");
			expect(body.tables).toEqual(
				expect.arrayContaining([
					"knowledge_trail_footprints",
					"knowledge_trail_branches",
					"knowledge_trail_act_outcomes",
				]),
			);
			expect(body.tables_truncated).toBe(body.tables.length);
		},
	);
});
