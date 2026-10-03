import { createHash, createHmac } from "node:crypto";
import type { APIRequestContext } from "@playwright/test";
import { requiredSecretFile } from "../../_shared/env.js";

export function fixtureUserId(label: string): string {
	const hex = createHash("sha256").update(`e2e-search:${label}`).digest("hex");
	return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-4${hex.slice(13, 16)}-a${hex.slice(17, 20)}-${hex.slice(20, 32)}`;
}

export function fixtureToken(userId: string, expiresIn = 300): string {
	const encode = (value: unknown) => Buffer.from(JSON.stringify(value)).toString("base64url");
	const unsigned = `${encode({ alg: "HS256", typ: "JWT" })}.${encode({
		sub: userId, tenant_id: userId, iss: "alt-staging-auth-hub", aud: "alt-backend",
		exp: Math.floor(Date.now() / 1000) + expiresIn,
	})}`;
	const signature = createHmac("sha256", requiredSecretFile("STAGING_JWT_KEY_FILE")).update(unsigned).digest("base64url");
	return `${unsigned}.${signature}`;
}

/** Positive fixture callers sign their own UUID; explicit headers stay intact.
 * Authorization negatives use the unwrapped `bare` client or explicit headers.
 * This helper never changes a URL, body, user identifier, or token supplied by a test.
 */
export function withSearchFixtureAuth(api: APIRequestContext, baseURL: string): APIRequestContext {
	type Options = NonNullable<Parameters<APIRequestContext["get"]>[1]>;
	return new Proxy(api, {
		get(target, property) {
			const original = Reflect.get(target, property);
			if (!["get", "post", "fetch"].includes(String(property))) {
				return typeof original === "function" ? original.bind(target) : original;
			}
			return (url: string, options: Options = {}) => {
				const parsed = new URL(url, baseURL);
				const supplied = Object.keys(options.headers ?? {}).some((name) => ["authorization", "x-alt-backend-token"].includes(name.toLowerCase()));
				let owner: unknown;
				if (parsed.pathname === "/v1/search") {
					owner = parsed.searchParams.get("user_id");
					if (typeof options.params === "object" && !(options.params instanceof URLSearchParams)) owner = options.params["user_id"] ?? owner;
					if (options.params instanceof URLSearchParams) owner = options.params.get("user_id") ?? owner;
				} else if (parsed.pathname === "/services.search.v2.SearchService/SearchArticles" && typeof options.data === "object" && options.data !== null && !Buffer.isBuffer(options.data)) {
					const body = options.data as Record<string, unknown>;
					owner = body["userId"] ?? body["user_id"];
				}
				if (parsed.origin === new URL(baseURL).origin && !supplied && typeof owner === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(owner)) {
					return original.call(target, url, { ...options, headers: { ...options.headers, Authorization: `Bearer ${fixtureToken(owner)}` } });
				}
				return original.call(target, url, options);
			};
		},
	});
}
