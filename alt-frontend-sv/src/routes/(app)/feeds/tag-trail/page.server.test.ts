import { beforeEach, describe, expect, it, vi } from "vitest";

const {
	createServerTransport,
	createServerTransportWithToken,
	fetchRandomFeed,
} = vi.hoisted(() => ({
	createServerTransport: vi.fn(),
	createServerTransportWithToken: vi.fn(),
	fetchRandomFeed: vi.fn(),
}));

vi.mock("#lib/connect/transport-server.js", () => ({
	createServerTransport,
	createServerTransportWithToken,
}));

vi.mock("#lib/connect/articles.js", () => ({
	fetchRandomFeed,
}));

import { load } from "./+page.server";

describe("tag-trail +page.server load", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("reuses locals.backendToken via createServerTransportWithToken when present", async () => {
		const fakeTransport = {};
		const fakeFetch = vi.fn();
		const fakeFeed = { id: "feed-1", title: "Tech News", tags: [] };
		createServerTransportWithToken.mockReturnValue(fakeTransport);
		fetchRandomFeed.mockResolvedValue(fakeFeed);

		const result = await load({
			request: new Request("http://localhost/feeds/tag-trail", {
				headers: { cookie: "ory_kratos_session=abc" },
			}),
			fetch: fakeFetch,
			locals: { backendToken: "cached-token" },
		} as unknown as Parameters<typeof load>[0]);

		expect(createServerTransportWithToken).toHaveBeenCalledWith(
			"cached-token",
			fakeFetch,
		);
		expect(createServerTransport).not.toHaveBeenCalled();
		expect(result).toEqual({ initialFeed: fakeFeed });
	});

	it("falls back to createServerTransport when locals.backendToken is null", async () => {
		const fakeTransport = {};
		const fakeFetch = vi.fn();
		const fakeFeed = { id: "feed-2", title: "World News", tags: [] };
		createServerTransport.mockResolvedValue(fakeTransport);
		fetchRandomFeed.mockResolvedValue(fakeFeed);

		const result = await load({
			request: new Request("http://localhost/feeds/tag-trail", {
				headers: { cookie: "ory_kratos_session=abc" },
			}),
			fetch: fakeFetch,
			locals: { backendToken: null },
		} as unknown as Parameters<typeof load>[0]);

		expect(createServerTransport).toHaveBeenCalledWith(
			"ory_kratos_session=abc",
			fakeFetch,
		);
		expect(createServerTransportWithToken).not.toHaveBeenCalled();
		expect(result).toEqual({ initialFeed: fakeFeed });
	});
});
