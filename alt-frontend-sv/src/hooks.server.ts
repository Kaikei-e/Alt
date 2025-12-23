import type { Handle } from "@sveltejs/kit";
import { redirect } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { ory } from "$lib/ory";

const PUBLIC_ROUTES = [
	/\/auth(\/|$)/,
	/\/health(\/|$)/,
	// Note: /sv/api/ and /api/ are NOT public - they require authentication
	// Each API endpoint checks authentication individually, but hooks.server.ts
	// also validates session and returns 401 for unauthenticated API requests
	/\/login(\/|$)/,
	/\/register(\/|$)/,
	/\/logout(\/|$)/,
	/\/recovery(\/|$)/,
	/\/verification(\/|$)/,
	/\/error(\/|$)/,
	/\/public\/landing(\/|$)/,
	/\/landing$/,
	/\/favicon\.ico$/,
	/\/icon\.svg$/,
	/\/test(\/|$)/,
];

export const handle: Handle = async ({ event, resolve: resolveEvent }) => {
	console.log("[hooks] Incoming request:", event.url.pathname);
	console.log("[hooks] KRATOS_INTERNAL_URL:", env.KRATOS_INTERNAL_URL);
	console.log("[hooks] AUTH_HUB_INTERNAL_URL:", env.AUTH_HUB_INTERNAL_URL);
	console.log("[hooks] BACKEND_BASE_URL:", env.BACKEND_BASE_URL);
	const start = performance.now();
	const { url } = event;
	const pathname = url.pathname;

	// Check if the route is public
	// SvelteKit automatically handles basePath, so we can use pathname directly
	// The pathname will be like /sv/login, and we check against patterns
	// Note: pathname includes basePath, so /sv/api/... matches /\/api\// pattern
	const isPublic = PUBLIC_ROUTES.some((pattern) => pattern.test(pathname));

	// Debug logging for API routes
	if (pathname.includes("/api/") || pathname.includes("summarize")) {
		console.log("[hooks.server] Request received", {
			pathname,
			isPublic,
			hasCookie: !!event.request.headers.get("cookie"),
			method: event.request.method,
		});
	}

	// Check if this is an SSE/streaming endpoint
	const isStreamEndpoint =
		pathname.includes("/stream") || pathname.includes("/sse");

	// Validate session
	try {
		const cookie = event.request.headers.get("cookie");
		if (cookie) {
			const { data: session } = await ory.toSession({ cookie });
			event.locals.session = session;
			event.locals.user = session.identity ?? null;
		} else {
			event.locals.session = null;
			event.locals.user = null;
		}
	} catch (error) {
		// Session is invalid or expired
		event.locals.session = null;
		event.locals.user = null;

		// Determine error type and status code
		// @ory/client SDK may return ApiError with statusCode property
		let errorStatus = 401;
		let errorMessage = error instanceof Error ? error.message : String(error);

		// Check for statusCode in error object (ApiError from @ory/client)
		if (error && typeof error === "object") {
			const errorObj = error as Record<string, unknown>;
			// Check for statusCode property (common in API error objects)
			if (typeof errorObj.statusCode === "number") {
				errorStatus = errorObj.statusCode;
			}
			// Check for response.status (some SDKs use this)
			else if (errorObj.response && typeof errorObj.response === "object") {
				const response = errorObj.response as Record<string, unknown>;
				if (typeof response.status === "number") {
					errorStatus = response.status;
				}
			}
			// Check error message for status codes
			else if (
				errorMessage.includes("403") ||
				errorMessage.includes("Forbidden")
			) {
				errorStatus = 403;
			}

			// Log safe error information (exclude sensitive data)
			// Extract only non-sensitive properties for logging
			const safeErrorInfo: Record<string, unknown> = {
				name: errorObj.name,
				statusCode: errorObj.statusCode,
				code: errorObj.code,
			};

			// Safely extract response status if available (without full response body)
			const errorResponse =
				(errorObj.response as Record<string, unknown> | undefined);
			if (errorResponse) {
				safeErrorInfo.responseStatus = errorResponse.status;
				safeErrorInfo.responseStatusText = errorResponse.statusText;
				// Log the error data if it exists, it might contain the Kratos reason
				if (errorResponse.data) {
					safeErrorInfo.responseData = JSON.stringify(errorResponse.data).substring(
						0,
						500,
					);
				}
			}

			console.warn("[hooks.server] Session validation error details", {
				pathname,
				errorType: error instanceof Error ? error.constructor.name : typeof error,
				errorMessage: errorMessage.substring(0, 200),
				errorStatus,
				errorInfo: safeErrorInfo,
				hasCookie: !!event.request.headers.get("cookie"),
				cookieLength: event.request.headers.get("cookie")?.length || 0,
				isStreamEndpoint,
			});
		} else {
			// Fallback: check error message for status codes
			if (errorMessage.includes("403") || errorMessage.includes("Forbidden")) {
				errorStatus = 403;
			}
			console.warn("[hooks.server] Session validation failed", {
				pathname,
				error: errorMessage.substring(0, 200),
				errorType: typeof error,
				status: errorStatus,
				hasCookie: !!event.request.headers.get("cookie"),
				cookieLength: event.request.headers.get("cookie")?.length || 0,
				isStreamEndpoint,
			});
		}

		// For API endpoints, return JSON error instead of letting SvelteKit handle it
		// This prevents HTML error pages for API requests
		if (pathname.startsWith("/sv/api/") || pathname.startsWith("/api/")) {
			const isPublic = PUBLIC_ROUTES.some((pattern) => pattern.test(pathname));
			// Only return error if route is not public (public routes don't need auth)
			if (!isPublic) {
				// For SSE endpoints, ensure we return proper error format
				// The endpoint handler will handle SSE-specific error formatting if needed
				return new Response(
					JSON.stringify({
						error: errorStatus === 403 ? "Forbidden" : "Authentication required",
						message: "Session validation failed",
					}),
					{
						status: errorStatus,
						headers: {
							"Content-Type": "application/json",
							"Cache-Control": "no-cache",
							// For SSE endpoints, add headers to prevent buffering
							...(isStreamEndpoint && {
								"X-Accel-Buffering": "no",
								Connection: "close",
							}),
						},
					},
				);
			}
		}

		let returnTo: string;
		if (pathname === "/sv" || pathname === "/sv/") {
			returnTo = encodeURIComponent(`${url.origin}/sv/home`);
		} else {
			returnTo = encodeURIComponent(`${pathname}${url.search}`);
		}
		throw redirect(303, `/sv/login?return_to=${returnTo}`);
	}

	return resolveEvent(event, {
		filterSerializedResponseHeaders: (name) => {
			return name === "content-type";
		},
	});
};


