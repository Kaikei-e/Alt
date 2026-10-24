import type { ServerLoad } from "@sveltejs/kit";
import { fetchRandomFeed } from "#lib/connect/articles.js";
import {
	createServerTransport,
	createServerTransportWithToken,
} from "#lib/connect/transport-server.js";

export const load: ServerLoad = async ({ request, fetch, locals }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		// Use Connect-RPC for server-side fetch (ADR-174)
		// This includes tags in the response (generated on-the-fly if not in DB)
		const transport = locals?.backendToken
			? createServerTransportWithToken(locals.backendToken, fetch)
			: await createServerTransport(cookieHeader, fetch);
		const feed = await fetchRandomFeed(transport);
		return {
			initialFeed: feed,
		};
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Failed to load initial feed for Tag Trail:", {
			message: errorMessage,
			cookieHeader: cookieHeader ? "present" : "missing",
		});

		return {
			initialFeed: null,
			error: "Failed to load initial feed",
		};
	}
};
