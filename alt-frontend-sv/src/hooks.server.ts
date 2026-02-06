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

export const handle: Handle = async ({ event, resolve: resolveEvent }) => {
	const { url } = event;
	const pathname = url.pathname;
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
