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
	"/sv/desktop/feeds": "/sv/feeds",
	"/sv/mobile/feeds": "/sv/feeds",
	// Batch A
	"/sv/desktop/augur": "/sv/augur",
	"/sv/mobile/retrieve/ask-augur": "/sv/augur",
	"/sv/desktop/recap/morning-letter": "/sv/recap/morning-letter",
	"/sv/mobile/recap/morning-letter": "/sv/recap/morning-letter",
	"/sv/desktop/feeds/tag-trail": "/sv/feeds/tag-trail",
	"/sv/mobile/feeds/tag-trail": "/sv/feeds/tag-trail",
	"/sv/desktop/feeds/favorites": "/sv/feeds/favorites",
	// Batch B
	"/sv/desktop/feeds/search": "/sv/feeds/search",
	"/sv/mobile/feeds/search": "/sv/feeds/search",
	"/sv/desktop/feeds/viewed": "/sv/feeds/viewed",
	"/sv/mobile/feeds/viewed": "/sv/feeds/viewed",
	// Batch C
	"/sv/desktop/recap/evening-pulse": "/sv/recap/evening-pulse",
	"/sv/mobile/recap/evening-pulse": "/sv/recap/evening-pulse",
	"/sv/desktop/recap": "/sv/recap",
	"/sv/mobile/recap/3days": "/sv/recap",
	"/sv/mobile/recap/7days": "/sv/recap?window=7",
	"/sv/desktop/recap/job-status": "/sv/recap/job-status",
	"/sv/mobile/recap/job-status": "/sv/recap/job-status",
	// Batch D
	"/sv/desktop/settings/feeds": "/sv/settings/feeds",
	"/sv/mobile/feeds/manage": "/sv/settings/feeds",
	"/sv/desktop/stats": "/sv/stats",
	"/sv/mobile/feeds/stats": "/sv/stats",
	// Batch E
	"/sv/mobile/feeds/swipe": "/sv/feeds/swipe",
	"/sv/desktop": "/sv/dashboard",
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
