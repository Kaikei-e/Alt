import { BackendFeedItem } from "@/schema/feed";
import { FeedSearchResult } from "@/schema/search";

export const transformFeedSearchResult = (
  feedSearchResult: FeedSearchResult,
): BackendFeedItem[] => {
  return feedSearchResult.results.map((result) => ({
    title: result.title,
    description: result.description,
    link: result.link,
    published: result.published,
    authors: result.authors,
  }));
};
