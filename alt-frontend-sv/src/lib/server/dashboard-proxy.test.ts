import { beforeEach, describe, expect, it, vi } from "vitest";

const { getBackendToken } = vi.hoisted(() => ({
	getBackendToken: vi.fn(),
}));

vi.mock("#lib/api.js", () => ({
	getBackendToken,
}));

import { proxyDashboardGet } from "./dashboard-proxy";

describe("proxyDashboardGet", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("returns 401 Unauthorized without calling fetch when backend token is missing", async () => {
		getBackendToken.mockResolvedValue(null);
		const fetchMock = vi.fn();
		vi.stubGlobal("fetch", fetchMock);

		const res = await proxyDashboardGet(
			"http://recap-worker:9005",
			"/v1/dashboard/job-progress",
			{
				request: new Request(
					"http://localhost/api/v1/dashboard/job-progress",
				),
				url: new URL("http://localhost/api/v1/dashboard/job-progress"),
			},
		);

		expect(res.status).toBe(401);
		expect(await res.json()).toEqual({ error: "Unauthorized" });
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it("forwards request with X-Alt-Backend-Token when token is present", async () => {
		getBackendToken.mockResolvedValue("valid-jwt");
		const fetchMock = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ ok: true }), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
		vi.stubGlobal("fetch", fetchMock);

		const res = await proxyDashboardGet(
			"http://recap-worker:9005",
			"/v1/dashboard/job-stats",
			{
				request: new Request("http://localhost/api/v1/dashboard/job-stats", {
					headers: { cookie: "ory_kratos_session=abc" },
				}),
				url: new URL(
					"http://localhost/api/v1/dashboard/job-stats?window=3600&ignored=1",
				),
			},
			{ allowedParams: ["window"] },
		);

		expect(res.status).toBe(200);
		expect(await res.json()).toEqual({ ok: true });
		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [calledUrl, init] = fetchMock.mock.calls[0]!;
		expect(calledUrl).toBe(
			"http://recap-worker:9005/v1/dashboard/job-stats?window=3600",
		);
		expect(
			(init?.headers as Record<string, string>)["X-Alt-Backend-Token"],
		).toBe("valid-jwt");
	});
});
