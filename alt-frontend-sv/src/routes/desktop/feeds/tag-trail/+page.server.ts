import type { ServerLoad } from "@sveltejs/kit";
import { createServerTransport } from "$lib/connect/transport-server";
import { fetchRandomFeed } from "$lib/connect/articles";

export const load: ServerLoad = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		// Use Connect-RPC for server-side fetch (ADR-174)
		// This includes tags in the response (generated on-the-fly if not in DB)
		const transport = await createServerTransport(cookieHeader);
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
