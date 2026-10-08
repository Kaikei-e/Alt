import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/paths", () => ({
	resolve: (path: string) => `/${path.replace(/^\//, "")}`,
}));

const getClientCSRFToken = vi.fn();
vi.mock("./core", () => ({
	getClientCSRFToken: () => getClientCSRFToken(),
}));

import { triggerTopicCardsJob } from "./dashboard";

describe("triggerTopicCardsJob", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getClientCSRFToken.mockResolvedValue("mock-csrf-token");
	});

	it("sends POST to /api/v1/generate/topic-cards with CSRF header and returns response", async () => {
		const mockResponse = {
			job_id: "cards-job-123",
			genres: [],
			status: "running",
		};
		const mockFetch = vi.fn().mockResolvedValue({
			ok: true,
			status: 202,
			json: async () => mockResponse,
		});

		const result = await triggerTopicCardsJob(mockFetch);

		expect(mockFetch).toHaveBeenCalledTimes(1);
		const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit];
		expect(url).toBe("/api/v1/generate/topic-cards");
		expect(init?.method).toBe("POST");
		expect(init?.headers).toEqual({
			"Content-Type": "application/json",
			"X-CSRF-Token": "mock-csrf-token",
		});
		expect(init?.body).toBe(JSON.stringify({}));
		expect(result).toEqual(mockResponse);
	});

	it("throws error with status on 409 already running response", async () => {
		const mockFetch = vi.fn().mockResolvedValue({
			ok: false,
			status: 409,
			text: async () =>
				JSON.stringify({ error: "Cards recap job already running" }),
		});

		await expect(triggerTopicCardsJob(mockFetch)).rejects.toMatchObject({
			status: 409,
		});
	});

	it("throws error with status on 503 user not configured response", async () => {
		const mockFetch = vi.fn().mockResolvedValue({
			ok: false,
			status: 503,
			text: async () => JSON.stringify({ error: "Cards user not configured" }),
		});

		await expect(triggerTopicCardsJob(mockFetch)).rejects.toMatchObject({
			status: 503,
		});
	});
});
