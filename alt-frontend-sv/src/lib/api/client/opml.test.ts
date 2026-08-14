import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$app/paths", () => ({ base: "" }));

const IMPORT_RESULT = {
	total: 2,
	imported: 2,
	skipped: 0,
	failed: 0,
};

function jsonResponse(body: unknown, status: number, statusText: string) {
	return new Response(JSON.stringify(body), {
		status,
		statusText,
		headers: { "content-type": "application/json" },
	});
}

function opmlFile() {
	return new File(['<opml version="2.0"></opml>'], "feeds.opml", {
		type: "text/x-opml",
	});
}

/**
 * Stand-in for this BFF's /api/auth/csrf plus the guarded write routes the
 * client hits. auth-hub mints a new HMAC token on every call and the BFF
 * mirrors it into the browser-wide `csrf_token` cookie
 * (src/lib/server/auth.ts), so only the most recently issued token ever
 * validates — a module-private token cache therefore 403s as soon as any
 * other client write has re-issued the shared token.
 */
function createFakeBff() {
	let issued = 0;
	let cookieToken: string | null = null;
	let rotateBeforeNextWrite = false;
	const issuedTokens: string[] = [];

	function issueToken(): string {
		issued += 1;
		cookieToken = `csrf-token-${issued}`;
		issuedTokens.push(cookieToken);
		return cookieToken;
	}

	/**
	 * Models another client write landing while a write is already in flight.
	 * The importer's window is the whole multipart upload — the longest write
	 * in the app — so this is the ordinary case, not a narrow race.
	 */
	function rotateDuringNextWrite() {
		rotateBeforeNextWrite = true;
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
			if (!provided || provided !== cookieToken) {
				return jsonResponse({ error: "invalid csrf token" }, 403, "Forbidden");
			}

			if (url.endsWith("/api/v1/rss-feed-link/import/opml")) {
				return jsonResponse(IMPORT_RESULT, 200, "OK");
			}
			return jsonResponse({ ok: true }, 200, "OK");
		},
	);

	return { fetchImpl, issuedTokens, rotateDuringNextWrite };
}

describe("importOPMLClient CSRF handling", () => {
	let bff: ReturnType<typeof createFakeBff>;

	beforeEach(() => {
		vi.resetModules();
		bff = createFakeBff();
		vi.stubGlobal("fetch", bff.fetchImpl);
	});

	it("still imports after an unrelated write has re-issued the shared token", async () => {
		const { importOPMLClient } = await import("./opml");
		const { callClientAPI } = await import("./core");

		await expect(importOPMLClient(opmlFile())).resolves.toEqual(IMPORT_RESULT);

		// Any other state-changing action (mark-as-read here) rotates the
		// browser-wide cookie out from under whatever the importer is holding.
		await expect(
			callClientAPI("/v1/feeds/read", { method: "POST", body: "{}" }),
		).resolves.toEqual({ ok: true });

		await expect(importOPMLClient(opmlFile())).resolves.toEqual(IMPORT_RESULT);
	});

	it("replays the import once when a write rotates the token mid-upload", async () => {
		const { importOPMLClient } = await import("./opml");

		bff.rotateDuringNextWrite();

		await expect(importOPMLClient(opmlFile())).resolves.toEqual(IMPORT_RESULT);
	});

	it("shares one issued token with a concurrent write", async () => {
		const { importOPMLClient } = await import("./opml");
		const { callClientAPI } = await import("./core");

		await expect(
			Promise.all([
				importOPMLClient(opmlFile()),
				callClientAPI("/v1/feeds/read", { method: "POST", body: "{}" }),
			]),
		).resolves.toEqual([IMPORT_RESULT, { ok: true }]);
		expect(bff.issuedTokens).toHaveLength(1);
	});
});
