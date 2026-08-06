import { createHmac, timingSafeEqual } from "node:crypto";

/**
 * JWS compact-form decoding and HS256 verification.
 *
 * `12-validate-jwt-shape.hurl` could only reach the claims through Hurl's
 * filter chain — `header … split "." nth 1 base64UrlSafeDecode decode "utf-8"
 * contains "\"iss\":\"auth-hub\""` — a substring match against the serialized
 * JSON. That passes for a token that merely *mentions* the issuer anywhere,
 * including inside an unrelated claim, and it cannot express "aud is exactly
 * this one audience" or "exp is TTL seconds after iat" at all. Worst of all it
 * says nothing about the signature: a token signed with the wrong key, or with
 * `alg: none`, decodes identically and passes every one of those asserts.
 *
 * Verifying the MAC here is what makes this an assertion about the contract
 * that actually matters. alt-backend accepts the `X-Alt-Backend-Token` header
 * only if it verifies against the same shared HS256 secret
 * (`e2e/fixtures/staging-secrets/alt_backend_token_secret.txt`, mounted into
 * both services by compose.staging.yaml). If auth-hub ever signs with a
 * different key, every authenticated request into the platform 401s in
 * production while this suite would still have reported green.
 *
 * These live here rather than in `_shared/` because auth-hub is the only
 * issuer in the fleet; if a second suite ever needs them they should move.
 */

export type DecodedJwt = {
	readonly header: Record<string, unknown>;
	readonly claims: Record<string, unknown>;
	/** `<header>.<payload>` — the bytes HMAC-SHA256 is computed over. */
	readonly signingInput: string;
	readonly signature: Buffer;
};

function decodeSegment(segment: string, what: string): Record<string, unknown> {
	let text: string;
	try {
		text = Buffer.from(segment, "base64url").toString("utf8");
	} catch (error) {
		const detail = error instanceof Error ? error.message : String(error);
		throw new Error(`JWT ${what} is not valid base64url: ${detail}`);
	}

	let parsed: unknown;
	try {
		parsed = JSON.parse(text);
	} catch {
		throw new Error(`JWT ${what} is not JSON: ${text.slice(0, 200)}`);
	}
	if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
		throw new Error(`JWT ${what} is not a JSON object: ${text.slice(0, 200)}`);
	}
	return parsed as Record<string, unknown>;
}

/** Splits and decodes a compact JWS. Throws — with the offending token — otherwise. */
export function decodeJwt(token: string): DecodedJwt {
	const parts = token.split(".");
	if (parts.length !== 3) {
		throw new Error(
			`expected a 3-segment compact JWS, got ${parts.length} segment(s): ` +
				`${token.slice(0, 80)}…`,
		);
	}
	const [rawHeader, rawPayload, rawSignature] = parts as [string, string, string];
	return {
		header: decodeSegment(rawHeader, "header"),
		claims: decodeSegment(rawPayload, "payload"),
		signingInput: `${rawHeader}.${rawPayload}`,
		signature: Buffer.from(rawSignature, "base64url"),
	};
}

/**
 * True when `token` carries a valid HS256 MAC over its own signing input.
 *
 * Constant-time on purpose. Not because a test harness has a timing-attack
 * surface, but because `timingSafeEqual` throws on a length mismatch, which
 * turns "the signature is 20 bytes because the service switched to a truncated
 * MAC" into a named failure rather than a quiet `false`.
 */
export function verifyHS256(token: string, secret: string): boolean {
	const decoded = decodeJwt(token);
	const expected = createHmac("sha256", secret).update(decoded.signingInput).digest();
	if (decoded.signature.length !== expected.length) {
		throw new Error(
			`JWT signature is ${decoded.signature.length} bytes; HMAC-SHA256 produces ` +
				`${expected.length}. The token was not signed with HS256.`,
		);
	}
	return timingSafeEqual(decoded.signature, expected);
}

/** Reads a claim that must be a string, naming the claim when it is not. */
export function stringClaim(decoded: DecodedJwt, name: string): string {
	const value = decoded.claims[name];
	if (typeof value !== "string") {
		throw new Error(
			`JWT claim "${name}" should be a string, got ${JSON.stringify(value)}. ` +
				`claims: ${JSON.stringify(decoded.claims)}`,
		);
	}
	return value;
}

/** Reads a claim that must be a number (`iat`, `exp`, `nbf`). */
export function numberClaim(decoded: DecodedJwt, name: string): number {
	const value = decoded.claims[name];
	if (typeof value !== "number" || !Number.isFinite(value)) {
		throw new Error(
			`JWT claim "${name}" should be a numeric date, got ${JSON.stringify(value)}. ` +
				`claims: ${JSON.stringify(decoded.claims)}`,
		);
	}
	return value;
}

/**
 * Normalises `aud` to a list.
 *
 * `jwt.ClaimStrings` marshals a single audience as a one-element array in
 * golang-jwt v5, but RFC 7519 permits the bare-string form and a library swap
 * would silently change it. Accepting both here keeps the *assertion* about
 * the audience value rather than about its JSON encoding.
 */
export function audiences(decoded: DecodedJwt): string[] {
	const value = decoded.claims["aud"];
	if (typeof value === "string") return [value];
	if (Array.isArray(value) && value.every((entry) => typeof entry === "string")) {
		return value as string[];
	}
	throw new Error(
		`JWT claim "aud" should be a string or a list of strings, got ${JSON.stringify(value)}`,
	);
}
