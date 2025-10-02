import { ApiClient } from "../core/ApiClient";
import { CursorApi } from "./CursorApi";
import { serverFetch } from "../utils/serverFetch";
import {
  BackendFeedItem,
  SanitizedFeed,
  FeedDetails,
  FeedURLPayload,
  FetchArticleSummaryResponse,
  sanitizeFeed,
  FeedContentOnTheFlyResponse,
} from "@/schema/feed";
import { FeedSearchResult } from "@/schema/search";
import { Article } from "@/schema/article";
import { FeedStatsSummary } from "@/schema/feedStats";
import { UnreadCount } from "@/schema/unread";
import { MessageResponse } from "@/schema/common";
import { FeedTags } from "@/types/feed-tags";
import { ApiError } from "../core/ApiError";

export class FeedsApi {
  private feedsCursorApi: CursorApi<BackendFeedItem, SanitizedFeed>;
  private favoritesCursorApi: CursorApi<BackendFeedItem, SanitizedFeed>;
  private readCursorApi: CursorApi<BackendFeedItem, SanitizedFeed>;

  // Cursor-based API functions
  public getFeedsWithCursor: (cursor?: string, limit?: number) => Promise<any>;
  public getFavoriteFeedsWithCursor: (
    cursor?: string,
    limit?: number,
  ) => Promise<any>;
  public getReadFeedsWithCursor: (
    cursor?: string,
    limit?: number,
  ) => Promise<any>;

  constructor(private apiClient: ApiClient) {
    const transformFeedItem = (item: BackendFeedItem): SanitizedFeed => {
      return sanitizeFeed(item);
    };

    this.feedsCursorApi = new CursorApi(
      apiClient,
      "/v1/feeds/fetch/cursor",
      transformFeedItem,
      5,
    );

    this.favoritesCursorApi = new CursorApi(
      apiClient,
      "/v1/feeds/fetch/favorites/cursor",
      transformFeedItem,
      10,
    );

    this.readCursorApi = new CursorApi(
      apiClient,
      "/v1/feeds/fetch/viewed/cursor",
      transformFeedItem,
      10,
    );

    // Initialize the cursor functions after the CursorApi instances are created
    this.getFeedsWithCursor = this.feedsCursorApi.createFunction();
    this.getFavoriteFeedsWithCursor = this.favoritesCursorApi.createFunction();
    this.getReadFeedsWithCursor = this.readCursorApi.createFunction();
  }

  // Health check
  async checkHealth(): Promise<{ status: string }> {
    return this.apiClient.get("/v1/health", 1);
  }

  // Legacy pagination methods
  async getFeeds(
    page: number = 1,
    pageSize: number = 10,
  ): Promise<SanitizedFeed[]> {
    const limit = page * pageSize;
    const response = await this.apiClient.get<BackendFeedItem[]>(
      `/v1/feeds/fetch/limit/${limit}`,
      10,
    );

    if (Array.isArray(response)) {
      return response.map(sanitizeFeed);
    }
    return [];
  }

  async getFeedsPage(page: number = 0): Promise<SanitizedFeed[]> {
    const response = await this.apiClient.get<BackendFeedItem[]>(
      `/v1/feeds/fetch/page/${page}`,
      10,
    );

    if (Array.isArray(response)) {
      return response.map(sanitizeFeed);
    }
    return [];
  }

  async getAllFeeds(): Promise<SanitizedFeed[]> {
    const response = await this.apiClient.get<BackendFeedItem[]>(
      "/v1/feeds/fetch/list",
      15,
    );

    if (Array.isArray(response)) {
      return response.map(sanitizeFeed);
    }
    return [];
  }

  async getSingleFeed(): Promise<SanitizedFeed> {
    const response = await this.apiClient.get<BackendFeedItem>(
      "/v1/feeds/fetch/single",
      5,
    );
    return sanitizeFeed(response);
  }

  // Feed management
  async registerRssFeed(url: string): Promise<MessageResponse> {
    return this.apiClient.post("/v1/rss-feed-link/register", { url });
  }

  async registerFavoriteFeed(url: string): Promise<MessageResponse> {
    return this.apiClient.post("/v1/feeds/register/favorite", { url });
  }

  async updateFeedReadStatus(url: string): Promise<MessageResponse> {
    return this.apiClient.post("/v1/feeds/read", { feed_url: url });
  }

  // Article summaries
  async getArticleSummary(
    feedUrl: string,
  ): Promise<FetchArticleSummaryResponse> {
    return this.apiClient.post<FetchArticleSummaryResponse>(
      "/v1/feeds/fetch/summary/provided",
      {
        feed_urls: [feedUrl],
      },
    );
  }

  async getFeedDetails(payload: FeedURLPayload): Promise<FeedDetails> {
    try {
      const response = await this.getArticleSummary(payload.feed_url);
      if (response.matched_articles.length > 0) {
        return {
          feed_url: payload.feed_url,
          summary: response.matched_articles[0].content,
        };
      }
      throw new ApiError("No summary found for this article");
    } catch (error) {
      throw new ApiError(
        error instanceof Error ? error.message : "Failed to fetch feed details",
      );
    }
  }

  async archiveContent(
    feedUrl: string,
    title?: string,
  ): Promise<MessageResponse> {
    const trimmedTitle = title?.trim();
    const payload: Record<string, unknown> = { feed_url: feedUrl };
    if (trimmedTitle) {
      payload.title = trimmedTitle;
    }

    return this.apiClient.post("/v1/articles/archive", payload);
  }

  // Article summarization
  async summarizeArticle(
    feedUrl: string,
  ): Promise<{ success: boolean; summary: string; article_id: string }> {
    return this.apiClient.post("/v1/feeds/summarize", { feed_url: feedUrl });
  }

  async getFeedContentOnTheFly(
    payload: FeedURLPayload,
  ): Promise<FeedContentOnTheFlyResponse> {
    const encodedUrl = encodeURIComponent(payload.feed_url);
    return this.apiClient.get<FeedContentOnTheFlyResponse>(
      `/v1/articles/fetch/content?url=${encodedUrl}`,
      10,
    );
  }

  // Search
  async searchArticles(query: string): Promise<Article[]> {
    return this.apiClient.get<Article[]>(`/v1/articles/search?q=${query}`);
  }

  async searchFeeds(query: string): Promise<FeedSearchResult> {
    try {
      const response = await this.apiClient.post<
        BackendFeedItem[] | FeedSearchResult
      >("/v1/feeds/search", { query });

      if (Array.isArray(response)) {
        return { results: response, error: null };
      }
      return response as FeedSearchResult;
    } catch (error) {
      return {
        results: [],
        error: error instanceof ApiError ? error.message : "Search failed",
      };
    }
  }

  // Statistics
  async getFeedStats(): Promise<FeedStatsSummary> {
    return this.apiClient.get<FeedStatsSummary>("/v1/feeds/stats", 5);
  }

  async getFeedStatsSSR(): Promise<FeedStatsSummary> {
    return serverFetch<FeedStatsSummary>("/v1/feeds/stats");
  }

  async getTodayUnreadCount(since: string): Promise<UnreadCount> {
    return this.apiClient.get<UnreadCount>(
      `/v1/feeds/count/unreads?since=${encodeURIComponent(since)}`,
      1,
    );
  }

  // Tags
  async fetchFeedTags(feedUrl: string): Promise<FeedTags> {
    return this.apiClient.post<FeedTags>(`/v1/feeds/tags`, {
      feed_url: feedUrl,
    });
  }

  // Prefetch methods
  async prefetchFeeds(pages: number[] = [0, 1]): Promise<void> {
    const prefetchPromises = pages.map((page) =>
      this.getFeedsPage(page).catch(() => {}),
    );
    await Promise.all(prefetchPromises);
  }

  async prefetchFavoriteFeeds(cursors: string[]): Promise<void> {
    const prefetchPromises = cursors.map((cursor) =>
      this.getFavoriteFeedsWithCursor(cursor).catch(() => {}),
    );
    await Promise.all(prefetchPromises);
  }

  async prefetchReadFeeds(cursors: string[]): Promise<void> {
    const prefetchPromises = cursors.map((cursor) =>
      this.getReadFeedsWithCursor(cursor).catch(() => {}),
    );
    await Promise.all(prefetchPromises);
  }

  // Cache management
  clearCache(): void {
    this.apiClient.clearCache();
  }
}
