import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/env/private", () => ({
	BACKEND_REST_URL: "http://backend-rest.test",
	BACKEND_CONNECT_URL: "http://backend-connect.test",
	AUTH_HUB_INTERNAL_URL: "http://auth-hub:8888",
}));

const { updateFeedReadStatus } = vi.hoisted(() => ({
	updateFeedReadStatus: vi.fn(),
}));

vi.mock("#lib/api.js", async (importOriginal) => {
	const actual = await importOriginal<typeof import("#lib/api.js")>();
	return { ...actual, updateFeedReadStatus };
});

import { POST } from "./+server";

function makeEvent(body: BodyInit | null) {
	return {
		request: new Request("http://localhost/api/v1/feeds/read", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				"X-CSRF-Token": "expected-csrf",
				cookie: "csrf_token=expected-csrf",
			},
			body,
		}),
		cookies: {
			get: (name: string) =>
				name === "csrf_token" ? "expected-csrf" : undefined,
		},
		locals: { backendToken: "locals-jwt" },
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /api/v1/feeds/read", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('returns 400 for invalid JSON body "{"', async () => {
		const res = await POST(makeEvent("{"));

		expect(res.status).toBe(400);
		expect(updateFeedReadStatus).not.toHaveBeenCalled();
	});

	it('returns 400 for literal "null" body', async () => {
		const res = await POST(makeEvent("null"));

		expect(res.status).toBe(400);
		expect(updateFeedReadStatus).not.toHaveBeenCalled();
	});

	it("returns 400 for non-string feed_url", async () => {
		const res = await POST(makeEvent(JSON.stringify({ feed_url: 123 })));

		expect(res.status).toBe(400);
		expect(updateFeedReadStatus).not.toHaveBeenCalled();
	});

	it("updates feed read status for valid string feed_url", async () => {
		updateFeedReadStatus.mockResolvedValueOnce(undefined);

		const res = await POST(
			makeEvent(JSON.stringify({ feed_url: "https://example.com/feed.xml" })),
		);

		expect(res.status).toBe(200);
		expect(await res.json()).toEqual({ success: true });
		expect(updateFeedReadStatus).toHaveBeenCalledWith(
			"csrf_token=expected-csrf",
			"https://example.com/feed.xml",
			"locals-jwt",
		);
	});
});
