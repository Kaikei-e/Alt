import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { methodNotAllowedSchema, notFoundSchema, openApiSchema } from "../src/schemas.js";

/**
 * Route registration — new coverage, and tag-generator's analogue of
 * alt-backend's `connect-surface.spec.ts`.
 *
 * The Hurl suite touched two of the four routes this service registers. A
 * `@app.post` decorator deleted in a refactor, or a module whose import blew
 * up and left its routes unregistered, would have shown up only as a 404 on
 * whichever caller happened to hit it first in production. That is CLAUDE.md
 * rule 8's failure mode ("DI forgot to wire" being indistinguishable from
 * "intentionally disabled") in its Python form, and the only thing that can
 * tell the two apart from outside is whether the path resolves at all.
 *
 * FastAPI hands us a better instrument than probing paths one at a time:
 * `/openapi.json` is generated from the **live route table**, so it answers
 * "which routes exist" directly rather than by inference from a status code.
 * The 404 control below is what keeps that honest — if an unregistered path
 * and a registered one both answered the same way, none of this would prove
 * anything.
 */

/** Every route `auth_service.py` registers, with the line that registers it. */
const REGISTERED_ROUTES = [
	{ path: "/health", method: "get", source: "auth_service.py:487" },
	{ path: "/api/v1/extract-tags", method: "post", source: "auth_service.py:543" },
	{ path: "/api/v1/generate-tags", method: "post", source: "auth_service.py:453" },
	{ path: "/api/v1/user-preferences", method: "get", source: "auth_service.py:511" },
] as const;

test.describe("route registration", () => {
	test("the OpenAPI document lists every registered route", {
		tag: "@contract",
	}, async ({ api }) => {
		const spec = await expectJsonStatus(await api.get("/openapi.json"), 200, openApiSchema);

		// The identity of the app object itself. A slice that accidentally
		// brought up news-creator or metrics behind the same DNS name would
		// answer every other assertion in this file just fine.
		expect(spec.info.title).toBe("Tag Generator Service");
		expect(spec.info.version).toBe("1.0.0");

		for (const { path, method, source } of REGISTERED_ROUTES) {
			const operations = spec.paths[path];
			expect(
				operations,
				`${path} is absent from the OpenAPI document. It is registered at ` +
					`${source}; absence means the decorator is gone or the module ` +
					`failed to import, not that the request was wrong.`,
			).toBeDefined();
			expect(Object.keys(operations ?? {}), `${path} lost its ${method.toUpperCase()} operation`).toContain(
				method,
			);
		}
	});

	test("an unregistered path 404s — the control for the assertions above", {
		tag: "@contract",
	}, async ({ api }) => {
		// Starlette's own handler, so the body is `{"detail":"Not Found"}` and
		// not a Pydantic error envelope. Asserting the body rather than just
		// the status keeps the distinction visible: a 404 carrying a `detail`
		// *array* would mean a route matched and validation rejected it.
		const response = await api.get("/api/v1/definitely-not-a-route");
		await expectJsonStatus(response, 404, notFoundSchema);
	});

	test("GET /api/v1/extract-tags is 405, not 404", { tag: "@contract" }, async ({ api }) => {
		// The distinction the whole file rests on, stated once directly: the
		// path resolves (405 — the router matched and rejected the verb) even
		// though only POST is registered. A 404 here would mean the route is
		// gone; a 200 would mean someone added a GET handler that bypasses the
		// request model and therefore all of its validation.
		await expectJsonStatus(await api.get("/api/v1/extract-tags"), 405, methodNotAllowedSchema);
	});

	test("POST /health is 405, not 404", { tag: "@contract" }, async ({ api }) => {
		await expectJsonStatus(
			await api.post("/health", { data: {} }),
			405,
			methodNotAllowedSchema,
		);
	});

	test("the interactive docs are served from the same route table", {
		tag: "@contract",
	}, async ({ api }) => {
		// `FastAPI(...)` is constructed without `docs_url=None`
		// (auth_service.py:431), so /docs is part of this service's exposed
		// surface whether or not anyone intended it. Asserting it makes that a
		// decision the next reader can see and revisit, rather than a
		// surprise found by a scanner. If it is ever disabled, this test
		// failing is the correct signal — flip it to expect 404 then.
		await expectStatus(await api.get("/docs"), 200);
	});
});
