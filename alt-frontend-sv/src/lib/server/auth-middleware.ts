/**
 * Auth middleware - session validation and backend token retrieval
 * with short-term cache to avoid per-request Kratos round-trips.
 */
import { ory } from "$lib/server/ory";
import { env } from "$env/dynamic/private";
import type { Identity, Session } from "@ory/client";

const AUTH_HUB_URL = env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";

export interface SessionResult {
	session: Session | null;
	user: Identity | null;
	backendToken: string | null;
}

// Short-term session cache (30 seconds TTL)
// Key: cookie hash, Value: { result, expiresAt }
const SESSION_CACHE_TTL_MS = 30_000;
const sessionCache = new Map<
	string,
	{ result: SessionResult; expiresAt: number }
>();

function getCacheKey(cookie: string): string {
	// Use first 64 chars of cookie as cache key (session token portion)
	return cookie.substring(0, 64);
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

	const cacheKey = getCacheKey(cookie);
	const now = Date.now();

	// Check cache
	const cached = sessionCache.get(cacheKey);
	if (cached && cached.expiresAt > now) {
		return cached.result;
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
	sessionCache.set(cacheKey, {
		result,
		expiresAt: now + SESSION_CACHE_TTL_MS,
	});

	// Periodically prune expired entries (keep cache size bounded)
	if (sessionCache.size > 100) {
		pruneExpiredEntries();
	}

	return result;
}

async function fetchBackendToken(cookie: string): Promise<string | null> {
	try {
		const response = await fetch(`${AUTH_HUB_URL}/session`, {
			headers: { cookie },
		});
		if (!response.ok) {
			return null;
		}
		return response.headers.get("X-Alt-Backend-Token");
	} catch {
		return null;
	}
}
