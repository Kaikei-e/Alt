import { json, type RequestHandler } from "@sveltejs/kit";
import { createServerTransport } from "$lib/connect/transport-server";
import { searchFeeds } from "$lib/connect/feeds";

export const POST: RequestHandler = async ({ request, url }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	// Get query parameters for cursor-based pagination
	const queryParam = url.searchParams.get("query");
	const cursorParam = url.searchParams.get("cursor");
	const limitParam = url.searchParams.get("limit");

	// Parse request body
	let body: { query: string; cursor?: number; limit?: number };
	try {
		body = await request.json();
	} catch {
		return json({ error: "Invalid JSON body" }, { status: 400 });
	}

	// Validate query
	const query = queryParam || body.query;
	if (!query || typeof query !== "string") {
		return json({ error: "Query is required" }, { status: 400 });
	}

	// Parse cursor (offset)
	let cursor: number | undefined;
	if (cursorParam) {
		const cursorOffset = parseInt(cursorParam, 10);
		if (!isNaN(cursorOffset)) {
			cursor = cursorOffset;
		}
	} else if (body.cursor !== undefined && body.cursor !== null) {
		cursor = body.cursor;
	}

	// Parse limit
	let limit = 20;
	if (limitParam) {
		const parsedLimit = parseInt(limitParam, 10);
		if (!isNaN(parsedLimit)) {
			limit = parsedLimit;
		}
	} else if (body.limit !== undefined && body.limit !== null) {
		limit = body.limit;
	}

	try {
		// Use Connect-RPC to search feeds
		const transport = await createServerTransport(cookieHeader);
		const response = await searchFeeds(transport, query, cursor, limit);

		// Convert Connect-RPC response to expected format
		return json({
			results: response.data.map((item) => ({
				title: item.title,
				description: item.description,
				link: item.link,
				published: item.published,
				author: item.author ? { name: item.author } : undefined,
			})),
			error: null,
			next_cursor: response.nextCursor,
			has_more: response.hasMore,
		});
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/feeds/search:", {
			message: errorMessage,
			query,
			cursor,
			limit,
		});
		return json({ error: "Internal server error" }, { status: 500 });
	}
};
