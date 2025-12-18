import { json, type RequestHandler } from "@sveltejs/kit";
import { getFeedLinks } from "$lib/api";

export const GET: RequestHandler = async ({ request }) => {
  const cookieHeader = request.headers.get("cookie") || "";

  try {
    const links = await getFeedLinks(cookieHeader);
    return json(links);
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : String(error);
    console.error("Error in /api/v1/rss-feed-link/list:", errorMessage);
    return json({ error: errorMessage }, { status: 500 });
  }
};


