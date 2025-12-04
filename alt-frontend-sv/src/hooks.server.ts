import type { Handle } from "@sveltejs/kit";
import { redirect } from "@sveltejs/kit";
import { ory } from "$lib/ory";

const PUBLIC_ROUTES = [
	/\/auth(\/|$)/,
	/\/api(\/|$)/,
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
	const { url } = event;
	const pathname = url.pathname;

	// Check if the route is public
	// SvelteKit automatically handles basePath, so we can use pathname directly
	// The pathname will be like /sv/login, and we check against patterns
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
		// /sv/ へのアクセスの場合は、/sv/home を return_to として設定（ループを防ぐ）
		let returnTo: string;
		if (pathname === "/sv" || pathname === "/sv/") {
			returnTo = encodeURIComponent(`${url.origin}/sv/home`);
		} else {
			returnTo = encodeURIComponent(`${pathname}${url.search}`);
		}
		// Redirect to login page - explicitly include basePath to ensure correct routing
		// SvelteKit's redirect() should add basePath automatically, but we include it explicitly to be safe
		throw redirect(303, `/sv/login?return_to=${returnTo}`);
	}

	return resolveEvent(event);
};
