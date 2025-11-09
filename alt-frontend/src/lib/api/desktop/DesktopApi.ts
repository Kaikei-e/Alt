import { mockDesktopFeeds } from "@/data/mockDesktopFeeds";
import type { MessageResponse } from "@/schema/common";
import { type BackendFeedItem, sanitizeFeed } from "@/schema/feed";
import type { ActivityResponse, WeeklyStats } from "@/types/desktop";
import type { DesktopFeed, DesktopFeedsResponse } from "@/types/desktop-feed";
import type { ApiClient } from "../core/ApiClient";
import type { FeedApi } from "../feeds/FeedApi";

export class DesktopApi {
  constructor(
    private apiClient: ApiClient,
    private feedsApi: FeedApi
  ) {}

  // Desktop Feed Methods
  async getDesktopFeeds(cursor?: string | null): Promise<DesktopFeedsResponse> {
    try {
      const params = new URLSearchParams();
      params.set("limit", "20");
      if (cursor) {
        params.set("cursor", cursor);
      }

      const response = await this.apiClient.get<{
        data: unknown[];
        next_cursor: string | null;
      }>(`/v1/feeds/fetch/cursor?${params.toString()}`, 10);

      const transformedFeeds = (response.data || []).map((item) =>
        sanitizeFeed(item as BackendFeedItem)
      );

      return {
        feeds: transformedFeeds,
        nextCursor: response.next_cursor,
        hasMore: response.next_cursor !== null,
        totalCount: transformedFeeds.length,
      };
    } catch (error) {
      console.error("getDesktopFeeds error:", error);
      return {
        feeds: [],
        nextCursor: null,
        hasMore: false,
        totalCount: 0,
      };
    }
  }

  // Test-compatible cursor API for E2E tests
  async getTestFeeds(
    cursor?: string | null
  ): Promise<{ data: DesktopFeed[]; next_cursor: string | null }> {
    return new Promise((resolve) => {
      const pageSize = 20;
      const startIndex = cursor ? parseInt(cursor) : 0;
      const endIndex = startIndex + pageSize;
      const paginatedFeeds = mockDesktopFeeds.slice(startIndex, endIndex);

      resolve({
        data: paginatedFeeds,
        next_cursor: endIndex < mockDesktopFeeds.length ? endIndex.toString() : null,
      });
    });
  }

  // Mock methods for development/testing
  async toggleFavorite(_feedId: string, isFavorited: boolean): Promise<MessageResponse> {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          message: isFavorited ? "Added to favorites" : "Removed from favorites",
        });
      }, 200);
    });
  }

  async toggleBookmark(_feedId: string, isBookmarked: boolean): Promise<MessageResponse> {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          message: isBookmarked ? "Added to bookmarks" : "Removed from bookmarks",
        });
      }, 200);
    });
  }

  async getRecentActivity(limit: number = 10): Promise<ActivityResponse[]> {
    return new Promise((resolve) => {
      setTimeout(() => {
        const mockActivities: ActivityResponse[] = [
          {
            id: 1,
            type: "new_feed",
            title: "Added TechCrunch RSS feed",
            timestamp: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
          },
          {
            id: 2,
            type: "ai_summary",
            title: "AI summary generated for 5 articles",
            timestamp: new Date(Date.now() - 4 * 60 * 60 * 1000).toISOString(),
          },
          {
            id: 3,
            type: "bookmark",
            title: 'Bookmarked "Introduction to React 19"',
            timestamp: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
          },
          {
            id: 4,
            type: "read",
            title: 'Read "Modern JavaScript Features"',
            timestamp: new Date(Date.now() - 48 * 60 * 60 * 1000).toISOString(),
          },
        ];

        resolve(mockActivities.slice(0, limit));
      }, 300);
    });
  }

  async getWeeklyStats(): Promise<WeeklyStats> {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          weeklyReads: 45,
          aiProcessed: 18,
          bookmarks: 12,
        });
      }, 200);
    });
  }
}
