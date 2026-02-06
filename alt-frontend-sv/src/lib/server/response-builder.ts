/**
 * Response builder - constructs API error responses and redirect URLs
 */

interface ApiErrorOptions {
	status: number;
	isStreamEndpoint: boolean;
}

export function buildApiErrorResponse(options: ApiErrorOptions): Response {
	const { status, isStreamEndpoint } = options;
	const errorLabel = status === 403 ? "Forbidden" : "Authentication required";

	return new Response(
		JSON.stringify({
			error: errorLabel,
			message: "Session validation failed",
		}),
		{
			status,
			headers: {
				"Content-Type": "application/json",
				"Cache-Control": "no-cache",
				...(isStreamEndpoint && {
					"X-Accel-Buffering": "no",
					Connection: "close",
				}),
			},
		},
	);
}

export function buildRedirectUrl(pathname: string, origin: string): string {
	if (pathname === "/sv" || pathname === "/sv/") {
		return `/sv/login?return_to=${encodeURIComponent(`${origin}/sv/home`)}`;
	}
	return `/sv/login?return_to=${encodeURIComponent(pathname)}`;
}
