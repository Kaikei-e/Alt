import { beforeEach, describe, expect, it, vi } from "vitest";

const { getFeedsWithCursor } = vi.hoisted(() => ({
	getFeedsWithCursor: vi.fn(),
}));

vi.mock("#lib/api.js", () => ({
	getFeedsWithCursor,
}));

import { GET } from "./+server";

function makeEvent(search = "") {
	const url = new URL(
		`http://localhost/api/v1/feeds/fetch/cursor${search}`,
	);
	return {
		request: new Request(url),
		url,
		locals: { backendToken: "locals-token" },
	} as unknown as Parameters<typeof GET>[0];
}

describe("GET /api/v1/feeds/fetch/cursor", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getFeedsWithCursor.mockImplementation(
			async (_cookie: string, _cursor: string | undefined, limit: number) => {
				if (Number.isNaN(limit)) {
					throw new Error("invalid limit");
				}
				return { data: [], next_cursor: null, has_more: false };
			},
		);
	});

	it("returns 400 when limit is non-numeric", async () => {
		const res = await GET(makeEvent("?limit=abc"));

		expect(res.status).toBe(400);
		expect(getFeedsWithCursor).not.toHaveBeenCalled();
	});

	it("returns 400 when limit is non-positive", async () => {
		const resZero = await GET(makeEvent("?limit=0"));
		expect(resZero.status).toBe(400);

		const resNeg = await GET(makeEvent("?limit=-5"));
		expect(resNeg.status).toBe(400);

		expect(getFeedsWithCursor).not.toHaveBeenCalled();
	});

	it("clamps oversized limit to 100", async () => {
		const res = await GET(makeEvent("?limit=100000"));

		expect(res.status).toBe(200);
		expect(getFeedsWithCursor).toHaveBeenCalledWith(
			"",
			undefined,
			100,
			"locals-token",
		);
	});

	it("defaults limit to 20 when omitted", async () => {
		const res = await GET(makeEvent());

		expect(res.status).toBe(200);
		expect(getFeedsWithCursor).toHaveBeenCalledWith(
			"",
			undefined,
			20,
			"locals-token",
		);
	});
});
