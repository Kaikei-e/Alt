import type { Handle, HandleServerError, ServerInit } from "@sveltejs/kit";
import { redirect } from "@sveltejs/kit";
import { building } from "$app/environment";
import {
	isPublicRoute,
	isApiRoute,
	isStreamEndpoint,
} from "$lib/server/route-guard";
import { verifySovereignAdminAuth } from "$lib/server/sovereign-admin";
import { validateSession } from "$lib/server/auth-middleware";
import { classifyOryError } from "$lib/server/error-classifier";
import {
	buildApiErrorResponse,
	buildRedirectUrl,
} from "$lib/server/response-builder";
import { resolveResponsiveRedirect } from "$lib/server/redirect-resolver";
import { classifySafari, extractChunkHash } from "$lib/safari-error-utils";
import {
	applyApiCacheControl,
	applyHtmlCacheControl,
} from "./hooks.server.cache-control";

const resolveOptions = {
	filterSerializedResponseHeaders: (name: string) => name === "content-type",
};

// `init` runs once when the server is created, which is where required runtime
// config has to be proven present. SvelteKit also runs it while prerendering,
// where `building` is true and the build machine holds no runtime secrets, so
// that pass is skipped rather than turned into a build-time secret requirement.
export const init: ServerInit = () => {
	if (building) return;

	verifySovereignAdminAuth();
};

export const handle: Handle = async ({ event, resolve: resolveEvent }) => {
	const { url } = event;
	const pathname = url.pathname;

	// Redirect old path-based routes to unified responsive routes (preserving query params).
	// 303 (See Other) so that iOS Safari does not pin the redirect to its cache
	// the way it does for 301 — old bookmarks must be allowed to follow new
	// mappings on every visit.
	const redirectTarget = resolveResponsiveRedirect(pathname, url.search);
	if (redirectTarget) {
		throw redirect(303, redirectTarget);
	}

	const isPublic = isPublicRoute(pathname);

	// Fast path: public routes without cookies skip auth entirely
	if (isPublic && !event.request.headers.get("cookie")) {
		event.locals.session = null;
		event.locals.user = null;
		event.locals.backendToken = null;
		const response = await resolveEvent(event, resolveOptions);
		applyHtmlCacheControl(response);
		applyApiCacheControl(response, pathname);
		return response;
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
			const response = await resolveEvent(event, resolveOptions);
			applyHtmlCacheControl(response);
			applyApiCacheControl(response, pathname);
			return response;
		}

		throw redirect(303, buildRedirectUrl(pathname, url.origin));
	}

	const response = await resolveEvent(event, resolveOptions);
	applyHtmlCacheControl(response);
	applyApiCacheControl(response, pathname);
	return response;
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
	const userAgent = event.request.headers.get("user-agent") || undefined;
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
			userAgent,
			safariBucket: classifySafari(userAgent),
			chunkHash: extractChunkHash(errInfo.message || ""),
			remote: event.getClientAddress?.(),
		}),
	);
	return { message: "Internal error" };
};
