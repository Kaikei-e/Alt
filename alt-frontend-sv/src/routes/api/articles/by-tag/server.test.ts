import { beforeEach, describe, expect, it, vi } from "vitest";

const { getArticlesByTag } = vi.hoisted(() => ({
	getArticlesByTag: vi.fn(),
}));

vi.mock("#lib/api.js", () => ({
	getArticlesByTag,
}));

import { GET } from "./+server";

function makeEvent(search = "") {
	const url = new URL(`http://localhost/api/articles/by-tag${search}`);
	return {
		request: new Request(url),
		url,
	} as unknown as Parameters<typeof GET>[0];
}

describe("GET /api/articles/by-tag", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getArticlesByTag.mockImplementation(
			async (
				_cookie: string,
				_tagId: string,
				_cursor: string | undefined,
				limit: number,
			) => {
				if (Number.isNaN(limit)) {
					throw new Error("invalid limit");
				}
				return { articles: [], has_more: false };
			},
		);
	});

	it("returns 400 when limit is non-numeric", async () => {
		const res = await GET(makeEvent("?tag_id=tag-1&limit=abc"));

		expect(res.status).toBe(400);
		expect(getArticlesByTag).not.toHaveBeenCalled();
	});

	it("returns 400 when limit is non-positive", async () => {
		const resZero = await GET(makeEvent("?tag_id=tag-1&limit=0"));
		expect(resZero.status).toBe(400);

		const resNeg = await GET(makeEvent("?tag_id=tag-1&limit=-5"));
		expect(resNeg.status).toBe(400);

		expect(getArticlesByTag).not.toHaveBeenCalled();
	});

	it("clamps oversized limit to 100", async () => {
		const res = await GET(makeEvent("?tag_id=tag-1&limit=100000"));

		expect(res.status).toBe(200);
		expect(getArticlesByTag).toHaveBeenCalledWith("", "tag-1", undefined, 100);
	});

	it("defaults limit to 20 when omitted", async () => {
		const res = await GET(makeEvent("?tag_id=tag-1"));

		expect(res.status).toBe(200);
		expect(getArticlesByTag).toHaveBeenCalledWith("", "tag-1", undefined, 20);
	});
});
