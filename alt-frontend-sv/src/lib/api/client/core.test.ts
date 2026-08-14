import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$app/paths", () => ({ base: "" }));

function jsonResponse(body: unknown, status: number, statusText: string) {
	return new Response(JSON.stringify(body), {
		status,
		statusText,
		headers: { "content-type": "application/json" },
	});
}

/**
 * Stand-in for this BFF's /api/auth/csrf plus one guarded write route.
 * auth-hub mints a new HMAC token on every call and the BFF mirrors it into
 * the browser-wide `csrf_token` cookie (src/lib/server/auth.ts), so only the
 * most recently issued token ever validates — that is what makes a per-tab
 * token cache reject every write once a second tab has loaded.
 */
function createFakeBff() {
	let issued = 0;
	let cookieToken: string | null = null;
	let rotateBeforeNextWrite = false;
	const issuedTokens: string[] = [];
	const writeTokens: (string | null)[] = [];

	/** Models any tab hitting GET /api/auth/csrf: new token, cookie overwritten. */
	function issueToken(): string {
		issued += 1;
		cookieToken = `csrf-token-${issued}`;
		issuedTokens.push(cookieToken);
		return cookieToken;
	}

	const fetchImpl = vi.fn(
		async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = typeof input === "string" ? input : String(input);

			if (url.endsWith("/api/auth/csrf")) {
				return jsonResponse({ csrf_token: issueToken() }, 200, "OK");
			}

			const method = (init?.method ?? "GET").toUpperCase();
			if (method === "GET") {
				return jsonResponse({ ok: true }, 200, "OK");
			}

			if (rotateBeforeNextWrite) {
				rotateBeforeNextWrite = false;
				issueToken();
			}

			const headers = (init?.headers ?? {}) as Record<string, string>;
			const provided = headers["X-CSRF-Token"] ?? null;
			writeTokens.push(provided);

			if (!provided || provided !== cookieToken) {
				return jsonResponse({ error: "invalid csrf token" }, 403, "Forbidden");
			}
			return jsonResponse({ ok: true }, 200, "OK");
		},
	);

	return {
		fetchImpl,
		issueToken,
		issuedTokens,
		writeTokens,
		currentCookieToken: () => cookieToken,
		rotateOnNextWrite: () => {
			rotateBeforeNextWrite = true;
		},
	};
}

describe("client CSRF token handling", () => {
	let bff: ReturnType<typeof createFakeBff>;

	beforeEach(() => {
		vi.resetModules();
		bff = createFakeBff();
		vi.stubGlobal("fetch", bff.fetchImpl);
	});

	it("keeps writes working after another tab has re-issued the shared token", async () => {
		const { callClientAPI } = await import("./core");

		await expect(
			callClientAPI("/v1/feeds/read", { method: "POST", body: "{}" }),
		).resolves.toEqual({ ok: true });

		// A second tab opens and fetches its own token; the cookie now holds it.
		bff.issueToken();

		await expect(
			callClientAPI("/v1/feeds/read", { method: "POST", body: "{}" }),
		).resolves.toEqual({ ok: true });
	});

	it("hands out-of-band callers a token that matches the current cookie", async () => {
		const { getClientCSRFToken } = await import("./core");

		await getClientCSRFToken();
		bff.issueToken();

		await expect(getClientCSRFToken()).resolves.toBe(bff.currentCookieToken());
	});

	it("retries once when the token is rotated between issuance and the write", async () => {
		const { callClientAPI } = await import("./core");

		bff.rotateOnNextWrite();

		await expect(
			callClientAPI("/v1/feeds/read", { method: "POST", body: "{}" }),
		).resolves.toEqual({ ok: true });
		expect(bff.writeTokens).toHaveLength(2);
	});

	it("shares one issued token across concurrent writes", async () => {
		const { callClientAPI } = await import("./core");

		await expect(
			Promise.all([
				callClientAPI("/v1/feeds/read", { method: "POST", body: "{}" }),
				callClientAPI("/v1/feeds/read", { method: "POST", body: "{}" }),
				callClientAPI("/v1/feeds/read", { method: "POST", body: "{}" }),
			]),
		).resolves.toEqual([{ ok: true }, { ok: true }, { ok: true }]);
		expect(bff.issuedTokens).toHaveLength(1);
	});

	it("does not fetch a token for read-only requests", async () => {
		const { callClientAPI } = await import("./core");

		await expect(callClientAPI("/v1/stats")).resolves.toEqual({ ok: true });
		expect(bff.issuedTokens).toHaveLength(0);
	});
});
