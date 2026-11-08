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
		getArticlesByTag.mockResolvedValue({ articles: [], has_more: false });
	});

	it.each(["abc", "10abc", "1e3", " 10", "-0"])(
		'returns 400 when limit is "%s"',
		async (rawLimit) => {
			const res = await GET(
				makeEvent(`?tag_id=tag-1&limit=${encodeURIComponent(rawLimit)}`),
			);

			expect(res.status).toBe(400);
			expect(getArticlesByTag).not.toHaveBeenCalled();
		},
	);

	it("returns 400 when limit is non-positive", async () => {
		const resZero = await GET(makeEvent("?tag_id=tag-1&limit=0"));
		expect(resZero.status).toBe(400);

		const resNeg = await GET(makeEvent("?tag_id=tag-1&limit=-5"));
		expect(resNeg.status).toBe(400);

		expect(getArticlesByTag).not.toHaveBeenCalled();
	});

	it("clamps oversized limit to 99", async () => {
		const res = await GET(makeEvent("?tag_id=tag-1&limit=100000"));

		expect(res.status).toBe(200);
		expect(getArticlesByTag).toHaveBeenCalledWith("", "tag-1", undefined, 99);
	});

	it("defaults limit to 20 when omitted", async () => {
		const res = await GET(makeEvent("?tag_id=tag-1"));

		expect(res.status).toBe(200);
		expect(getArticlesByTag).toHaveBeenCalledWith("", "tag-1", undefined, 20);
	});
});
