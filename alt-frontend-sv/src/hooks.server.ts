import type { Handle } from "@sveltejs/kit";
import { redirect } from "@sveltejs/kit";
import { ory } from "$lib/ory";

const PUBLIC_ROUTES = [
	/^\/auth(\/|$)/,
	/^\/api(\/|$)/,
	/^\/login(\/|$)/,
	/^\/register(\/|$)/,
	/^\/logout(\/|$)/,
	/^\/recovery(\/|$)/,
	/^\/verification(\/|$)/,
	/^\/public\/landing(\/|$)/,
	/^\/landing$/,
	/^\/favicon\.ico$/,
	/^\/icon\.svg$/,
	/^\/test(\/|$)/,
];

export const handle: Handle = async ({ event, resolve }) => {
	const { url } = event;
	const pathname = url.pathname;

	// Check if the route is public
	const isPublic = PUBLIC_ROUTES.some((pattern) => pattern.test(pathname));

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
	} catch (_error) {
		// Session is invalid or expired
		event.locals.session = null;
		event.locals.user = null;
	}

	// Protect routes
	if (!isPublic && !event.locals.session) {
		const returnTo = encodeURIComponent(`${pathname}${url.search}`);
		// Redirect to login page with return_to parameter
		throw redirect(303, `/login?return_to=${returnTo}`);
	}

	return resolve(event);
};
