/**
 * Auth middleware - session validation and backend token retrieval
 * with short-term cache to avoid per-request Kratos round-trips.
 */
import { createHash } from "node:crypto";
import { ory } from "$lib/server/ory";
import { env } from "$env/dynamic/private";
import type { Identity, Session } from "@ory/client";

const AUTH_HUB_URL = env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";
const AUTH_HUB_TIMEOUT_MS = 3000;

// Name of the Kratos session cookie (see kratos/kratos.yml `session.cookie`).
const SESSION_COOKIE_NAME = "ory_kratos_session";

export interface SessionResult {
	session: Session | null;
	user: Identity | null;
	backendToken: string | null;
}

// Short-term session cache (30 seconds TTL)
// Key: SHA-256 hash of the session cookie value, Value: { result, expiresAt }
const SESSION_CACHE_TTL_MS = 30_000;
const sessionCache = new Map<
	string,
	{ result: SessionResult; expiresAt: number }
>();

// Extracts the session cookie's value from a raw `Cookie` header by parsing
// individual cookie pairs, not by taking a positional substring — another
// cookie sorting before it must not shift or truncate the value we key on.
function extractSessionCookieValue(cookieHeader: string): string | null {
	for (const pair of cookieHeader.split(";")) {
		const eqIndex = pair.indexOf("=");
		if (eqIndex === -1) continue;
		const name = pair.slice(0, eqIndex).trim();
		if (name === SESSION_COOKIE_NAME) {
			return pair.slice(eqIndex + 1).trim();
		}
	}
	return null;
}

// Cache key is a hash of the actual session cookie value, never a raw header
// substring: two different users' cookie headers must never collide just
// because an unrelated cookie happens to sort first in the header.
function getCacheKey(cookieHeader: string): string | null {
	const sessionValue = extractSessionCookieValue(cookieHeader);
	if (!sessionValue) {
		return null;
	}
	return createHash("sha256").update(sessionValue).digest("hex");
}

function pruneExpiredEntries(): void {
	const now = Date.now();
	for (const [key, entry] of sessionCache) {
		if (entry.expiresAt <= now) {
			sessionCache.delete(key);
		}
	}
}

export async function validateSession(
	cookie: string | null,
): Promise<SessionResult> {
	if (!cookie) {
		return { session: null, user: null, backendToken: null };
	}

	// Requests without an actual session cookie (e.g. only unrelated cookies)
	// are never cached — there is nothing safe to key them on.
	const cacheKey = getCacheKey(cookie);
	const now = Date.now();

	// Check cache
	if (cacheKey) {
		const cached = sessionCache.get(cacheKey);
		if (cached && cached.expiresAt > now) {
			return cached.result;
		}
	}

	// Fetch session and backend token in parallel
	const [sessionResponse, backendToken] = await Promise.all([
		ory.toSession({ cookie }),
		fetchBackendToken(cookie),
	]);

	const result: SessionResult = {
		session: sessionResponse.data,
		user: sessionResponse.data.identity ?? null,
		backendToken,
	};

	// Cache the result
	if (cacheKey) {
		sessionCache.set(cacheKey, {
			result,
			expiresAt: now + SESSION_CACHE_TTL_MS,
		});

		// Periodically prune expired entries (keep cache size bounded)
		if (sessionCache.size > 100) {
			pruneExpiredEntries();
		}
	}

	return result;
}

async function fetchBackendToken(cookie: string): Promise<string | null> {
	try {
		const response = await fetch(`${AUTH_HUB_URL}/session`, {
			headers: { cookie },
			signal: AbortSignal.timeout(AUTH_HUB_TIMEOUT_MS),
		});
		if (!response.ok) {
			return null;
		}
		return response.headers.get("X-Alt-Backend-Token");
	} catch {
		return null;
	}
}
