import { createHmac } from "node:crypto";
import { env } from "./env.js";

/** Default test user UUID: standard RFC 4122 nil-adjacent deterministic UUID. */
export const TEST_USER_ID = "00000000-0000-0000-0000-000000000001";
/** Secondary test user UUID for cross-tenant document seeding in staging. */
export const SECONDARY_USER_ID = "00000000-0000-0000-0000-000000000002";

/**
 * Resolves the HS256 signing secret for the test backend token.
 * Uses the requiredSecretFile-backed env.backendTokenSecret directly.
 */
export function resolveBackendTokenSecret(): string {
	return env.backendTokenSecret;
}

function base64UrlEncode(input: string | Buffer): string {
	const buf = typeof input === "string" ? Buffer.from(input, "utf8") : input;
	return buf.toString("base64url");
}

export interface MintTokenOptions {
	userId?: string;
	issuer?: string;
	audience?: string;
	expiresInSeconds?: number;
	secret?: string;
	iat?: number;
	exp?: number;
}

/**
 * Mints a valid short-lived HS256 X-Alt-Backend-Token JWT for E2E testing.
 */
export function mintBackendToken(options: MintTokenOptions = {}): string {
	const secret = options.secret ?? resolveBackendTokenSecret();
	const userId = options.userId ?? TEST_USER_ID;
	const issuer = options.issuer ?? "auth-hub";
	const audience = options.audience ?? "alt-backend";
	const now = options.iat ?? Math.floor(Date.now() / 1000);
	const exp = options.exp ?? now + (options.expiresInSeconds ?? 900); // 15-minute short-lived exp

	const header = {
		alg: "HS256",
		typ: "JWT",
	};
	const payload = {
		iss: issuer,
		aud: audience,
		sub: userId,
		iat: now,
		exp: exp,
	};

	const encodedHeader = base64UrlEncode(JSON.stringify(header));
	const encodedPayload = base64UrlEncode(JSON.stringify(payload));
	const signingInput = `${encodedHeader}.${encodedPayload}`;

	const signature = createHmac("sha256", secret).update(signingInput).digest("base64url");

	return `${signingInput}.${signature}`;
}

export const BACKEND_TOKEN_HEADER = "X-Alt-Backend-Token";
