import type { BackendFeedItem } from "@/schema/feed";

export type FeedSearchResult = {
  results: BackendFeedItem[];
  error: string | null;
};
