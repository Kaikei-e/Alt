import type { Article } from "@/schema/article";
import type { MessageResponse } from "@/schema/common";
import {
  type FeedContentOnTheFlyResponse,
  type FeedDetails,
  type FeedURLPayload,
  type FetchArticleSummaryResponse,
} from "@/schema/feed";
import type { ApiClient } from "../core/ApiClient";
import { ApiError } from "../core/ApiError";
import { CursorApi } from "../feeds/CursorApi";

export class ArticleApi {
  private articlesCursorApi: CursorApi<Article, Article>;

  public getArticlesWithCursor: (cursor?: string, limit?: number) => Promise<any>;

  constructor(private apiClient: ApiClient) {
    this.articlesCursorApi = new CursorApi(
      apiClient,
      "/v1/articles/fetch/cursor",
      (item: Article) => item,
      20
    );

    this.getArticlesWithCursor = this.articlesCursorApi.createFunction();
  }

  async getArticleSummary(feedUrl: string): Promise<FetchArticleSummaryResponse> {
    return this.apiClient.post<FetchArticleSummaryResponse>("/v1/feeds/fetch/summary/provided", {
      feed_urls: [feedUrl],
    });
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
      throw new ApiError(error instanceof Error ? error.message : "Failed to fetch feed details");
    }
  }

  async archiveContent(feedUrl: string, title?: string): Promise<MessageResponse> {
    const trimmedTitle = title?.trim();
    const payload: Record<string, unknown> = { feed_url: feedUrl };
    if (trimmedTitle) {
      payload.title = trimmedTitle;
    }

    return this.apiClient.post("/v1/articles/archive", payload);
  }

  async summarizeArticle(
    feedUrl: string
  ): Promise<{ success: boolean; summary: string; article_id: string; feed_url: string }> {
    return this.apiClient.post("/v1/feeds/summarize", { feed_url: feedUrl });
  }

  async getFeedContentOnTheFly(payload: FeedURLPayload): Promise<FeedContentOnTheFlyResponse> {
    const encodedUrl = encodeURIComponent(payload.feed_url);
    // Use shorter timeout (15 seconds) to prevent UI freezing
    // The backend may take 20-44 seconds, but we should fail fast to keep UI responsive
    return this.apiClient.get<FeedContentOnTheFlyResponse>(
      `/v1/articles/fetch/content?url=${encodedUrl}`,
      0, // No cache - always fetch fresh
      15000 // 15 second timeout to prevent UI freezing
    );
  }

  async searchArticles(query: string): Promise<Article[]> {
    const backendResponse = await this.apiClient.get<Article[]>(`/v1/articles/search?q=${query}`);

    return backendResponse;
  }
}
