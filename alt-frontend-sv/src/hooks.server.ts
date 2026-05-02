import type { Handle, HandleServerError } from "@sveltejs/kit";
import { redirect } from "@sveltejs/kit";
import {
	isPublicRoute,
	isApiRoute,
	isStreamEndpoint,
} from "$lib/server/route-guard";
import { validateSession } from "$lib/server/auth-middleware";
import { classifyOryError } from "$lib/server/error-classifier";
import {
	buildApiErrorResponse,
	buildRedirectUrl,
} from "$lib/server/response-builder";
import { resolveResponsiveRedirect } from "$lib/server/redirect-resolver";

const resolveOptions = {
	filterSerializedResponseHeaders: (name: string) => name === "content-type",
};

export const handle: Handle = async ({ event, resolve: resolveEvent }) => {
	const { url } = event;
	const pathname = url.pathname;

	// Redirect old path-based routes to unified responsive routes (preserving query params)
	const redirectTarget = resolveResponsiveRedirect(pathname, url.search);
	if (redirectTarget) {
		throw redirect(301, redirectTarget);
	}

	const isPublic = isPublicRoute(pathname);

	// Fast path: public routes without cookies skip auth entirely
	if (isPublic && !event.request.headers.get("cookie")) {
		event.locals.session = null;
		event.locals.user = null;
		event.locals.backendToken = null;
		return resolveEvent(event, resolveOptions);
	}

	event.locals.backendToken = null;

	try {
		const cookie = event.request.headers.get("cookie");
		const result = await validateSession(cookie);
		event.locals.session = result.session;
		event.locals.user = result.user;
		event.locals.backendToken = result.backendToken;
	} catch (error) {
		event.locals.session = null;
		event.locals.user = null;

		const classified = classifyOryError(error);

		console.warn("[hooks.server] Session validation failed", {
			pathname,
			status: classified.status,
			error: classified.message,
			...classified.safeLogInfo,
		});

		if (isApiRoute(pathname) && !isPublic) {
			return buildApiErrorResponse({
				status: classified.status,
				isStreamEndpoint: isStreamEndpoint(pathname),
			});
		}

		if (isPublic) {
			return resolveEvent(event, resolveOptions);
		}

		throw redirect(303, buildRedirectUrl(pathname, url.origin));
	}

	return resolveEvent(event, resolveOptions);
};

// handleError captures every uncaught exception thrown from load functions and
// server hooks, emitting a structured JSON line to stderr so docker logs can be
// tailed and grepped. The production frontend container runs node adapter with
// production NODE_ENV which otherwise swallows raw console.error from load
// functions; this handler restores that visibility.
export const handleError: HandleServerError = ({
	error,
	event,
	status,
	message,
}) => {
	const errInfo =
		error instanceof Error
			? {
					name: error.name,
					message: error.message,
					stack: error.stack,
				}
			: { message: String(error) };
	const cause = (error as { cause?: unknown })?.cause;
	console.error(
		JSON.stringify({
			level: "error",
			source: "sveltekit-handleError",
			ts: new Date().toISOString(),
			path: event.url.pathname,
			query: event.url.search || undefined,
			method: event.request.method,
			status,
			message,
			error: errInfo,
			cause: cause === undefined ? undefined : String(cause),
			userAgent: event.request.headers.get("user-agent") || undefined,
			remote: event.getClientAddress?.(),
		}),
	);
	return { message: "Internal error" };
};
