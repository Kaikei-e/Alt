import type { ServerLoad } from "@sveltejs/kit";
import { getFeedLinks } from "$lib/api";
import type { FeedLink } from "$lib/schema/feedLink";

interface PageData {
	feedLinks: FeedLink[];
	error?: string;
}

export const load: ServerLoad = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const feedLinks = await getFeedLinks(cookieHeader);

		return {
			feedLinks,
		} satisfies PageData;
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("[SettingsFeeds] Failed to load feed links:", {
			message: errorMessage,
			cookieHeader: cookieHeader ? "present" : "missing",
		});

		return {
			feedLinks: [],
			error: "Failed to load feed links",
		} satisfies PageData;
	}
};
