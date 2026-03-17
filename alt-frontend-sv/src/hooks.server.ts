import type { Handle } from "@sveltejs/kit";
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

const resolveOptions = {
	filterSerializedResponseHeaders: (name: string) => name === "content-type",
};

// Redirect map: old path-based routes -> unified responsive routes
const RESPONSIVE_REDIRECTS: Record<string, string> = {
	// Phase 1 (completed)
	"/desktop/feeds": "/feeds",
	"/mobile/feeds": "/feeds",
	// Batch A
	"/desktop/augur": "/augur",
	"/mobile/retrieve/ask-augur": "/augur",
	"/desktop/recap/morning-letter": "/recap/morning-letter",
	"/mobile/recap/morning-letter": "/recap/morning-letter",
	"/desktop/feeds/tag-trail": "/feeds/tag-trail",
	"/mobile/feeds/tag-trail": "/feeds/tag-trail",
	"/desktop/feeds/tag-verse": "/feeds/tag-verse",
	"/desktop/feeds/favorites": "/feeds/favorites",
	// Batch B
	"/desktop/feeds/search": "/feeds/search",
	"/mobile/feeds/search": "/feeds/search",
	"/desktop/feeds/viewed": "/feeds/viewed",
	"/mobile/feeds/viewed": "/feeds/viewed",
	// Batch C
	"/desktop/recap/evening-pulse": "/recap/evening-pulse",
	"/mobile/recap/evening-pulse": "/recap/evening-pulse",
	"/desktop/recap": "/recap",
	"/mobile/recap/3days": "/recap",
	"/mobile/recap/7days": "/recap?window=7",
	"/desktop/recap/job-status": "/recap/job-status",
	"/mobile/recap/job-status": "/recap/job-status",
	// Batch D
	"/desktop/settings/feeds": "/settings/feeds",
	"/mobile/feeds/manage": "/settings/feeds",
	"/desktop/stats": "/stats",
	"/mobile/feeds/stats": "/stats",
	// Batch E
	"/mobile/feeds/swipe": "/feeds/swipe",
	"/desktop": "/dashboard",
};

export const handle: Handle = async ({ event, resolve: resolveEvent }) => {
	const { url } = event;
	const pathname = url.pathname;

	// Redirect old path-based routes to unified responsive routes
	const redirectTarget = RESPONSIVE_REDIRECTS[pathname];
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
