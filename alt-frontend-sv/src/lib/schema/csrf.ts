import * as v from "valibot";

// This BFF's own public /api/auth/csrf response shape, returned to the
// browser client (src/lib/api/client/core.ts, src/lib/api/client/opml.ts).
export const CsrfTokenResponseSchema = v.object({
	csrf_token: v.optional(v.nullable(v.string())),
});

export type CsrfTokenResponse = v.InferOutput<typeof CsrfTokenResponseSchema>;

export function parseCsrfToken(data: unknown): string | null {
	const result = v.safeParse(CsrfTokenResponseSchema, data);
	if (!result.success) return null;
	return result.output.csrf_token ?? null;
}

// auth-hub's POST /csrf response shape (auth-hub/internal/adapter/handler/csrf.go).
// Distinct from CsrfTokenResponseSchema above — that one is this BFF's own
// contract with its browser client, not auth-hub's wire format.
export const AuthHubCsrfResponseSchema = v.object({
	data: v.object({
		csrf_token: v.string(),
	}),
});

export function parseAuthHubCsrfToken(data: unknown): string | null {
	const result = v.safeParse(AuthHubCsrfResponseSchema, data);
	if (!result.success) return null;
	return result.output.data.csrf_token;
}
