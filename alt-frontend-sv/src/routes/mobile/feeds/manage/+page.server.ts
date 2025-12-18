import type { ServerLoad } from "@sveltejs/kit";
import { getFeedLinks } from "$lib/api";

interface PageData {
  feedLinks: Array<{ id: string; url: string }>;
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
    console.error("[MobileFeedsManage] Failed to load feed links:", {
      message: errorMessage,
      cookieHeader: cookieHeader ? "present" : "missing",
    });

    return {
      feedLinks: [],
      error: "Failed to load feed links",
    } satisfies PageData;
  }
};


