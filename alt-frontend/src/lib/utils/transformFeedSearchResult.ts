import type { FeedSearchResult, SearchFeedItem } from "@/schema/search";

export const transformFeedSearchResult = (
  feedSearchResult: FeedSearchResult | SearchFeedItem[],
): SearchFeedItem[] => {
  // If the response is directly an array (backend returns array directly)
  if (Array.isArray(feedSearchResult)) {
    return feedSearchResult;
  }

  // If it's wrapped in FeedSearchResult structure
  if (
    feedSearchResult &&
    typeof feedSearchResult === "object" &&
    "results" in feedSearchResult
  ) {
    if (Array.isArray(feedSearchResult.results)) {
      return feedSearchResult.results;
    }
  }

  // Fallback to empty array if structure is unexpected
  return [];
};
