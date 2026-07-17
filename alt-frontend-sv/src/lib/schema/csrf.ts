import * as v from "valibot";

export const CsrfTokenResponseSchema = v.object({
	csrf_token: v.optional(v.nullable(v.string())),
});

export type CsrfTokenResponse = v.InferOutput<typeof CsrfTokenResponseSchema>;

export function parseCsrfToken(data: unknown): string | null {
	const result = v.safeParse(CsrfTokenResponseSchema, data);
	if (!result.success) return null;
	return result.output.csrf_token ?? null;
}
