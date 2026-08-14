import { beforeEach, describe, expect, it, vi } from "vitest";

import { fallback } from "./+server";

// This proxy concatenates the catch-all straight onto a fixed service prefix,
// so the prefix is only as strong as the segment appended to it. SvelteKit
// percent-decodes the rest param before we see it, and `fetch`'s URL parser
// then collapses `..` and treats `\` as a separator — an authenticated
// non-admin sending `..%2F..%2Fv1%2Faggregate` walks straight out of the
// AdminMonitorService prefix onto any BFF endpoint with their token attached.
function makeEvent(path: string, token: string | null = "backend-token") {
	return {
		request: new Request("http://localhost/api/v2/admin-monitor", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: "{}",
		}),
		params: { path },
		locals: { backendToken: token },
	} as unknown as Parameters<typeof fallback>[0];
}

describe("POST /api/v2/alt.admin_monitor.v1.AdminMonitorService/[...path]", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => new Response("{}", { status: 200 })),
		);
	});

	it.each([
		["encoded slash traversal", "../../v1/aggregate"],
		["encoded backslash traversal", "..\\..\\v1\\aggregate"],
		["traversal after a valid-looking method", "Watch\\..\\..\\v1\\aggregate"],
		["extra path segment", "Watch/Extra"],
		["query string smuggled into the method", "Watch?admin=1"],
		["fragment smuggled into the method", "Watch#/v1/aggregate"],
		["still-encoded slash", "Watch%2F..%2Fv1"],
		["empty method", ""],
	])("refuses %s", async (_label, method) => {
		const res = await fallback(makeEvent(method));

		expect(res.status).toBe(404);
		expect(globalThis.fetch).not.toHaveBeenCalled();
	});

	it("proxies a well-formed Connect method to the BFF", async () => {
		const res = await fallback(makeEvent("Watch"));

		expect(res.status).toBe(200);
		expect(vi.mocked(globalThis.fetch).mock.calls[0]?.[0]).toBe(
			"http://alt-butterfly-facade:9250/alt.admin_monitor.v1.AdminMonitorService/Watch",
		);
	});

	it("still requires a backend token", async () => {
		const res = await fallback(makeEvent("Watch", null));

		expect(res.status).toBe(401);
		expect(globalThis.fetch).not.toHaveBeenCalled();
	});
});
