import {
  BackendFeedItem,
  Feed,
  FeedDetails,
  FeedURLPayload,
} from "@/schema/feed";
import { FeedSearchResult } from "@/schema/search";
import { Article } from "@/schema/article";
import { FeedStatsSummary } from "@/schema/feedStats";
import { UnreadCount } from "@/schema/unread";
import {
  ApiConfig,
  defaultApiConfig,
  CacheConfig,
  defaultCacheConfig,
} from "@/lib/config";
import { CursorResponse, MessageResponse } from "@/schema/common";
import { DesktopFeedsResponse } from "@/types/desktop-feed";
import { ActivityResponse, WeeklyStats } from "@/types/desktop";
import { mockDesktopFeeds } from "@/data/mockDesktopFeeds";

// Re-export types for external use
export type { CursorResponse } from "@/schema/common";

export class ApiClientError extends Error {
  public readonly status?: number;
  public readonly code?: string;

  constructor(message: string, status?: number, code?: string) {
    super(message);
    this.name = "ApiClientError";
    this.status = status;
    this.code = code;
  }
}

// Cache interface for performance optimization
interface CacheEntry<T> {
  data: T;
  timestamp: number;
  ttl: number;
}

// Remove duplicate message interface - use MessageResponse from common

class ApiClient {
  private config: ApiConfig;
  private cacheConfig: CacheConfig;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private cache = new Map<string, CacheEntry<any>>();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private pendingRequests = new Map<string, Promise<any>>();
  private cleanupTimer?: NodeJS.Timeout;

  constructor(
    apiConfig: ApiConfig = defaultApiConfig,
    cacheConfig: CacheConfig = defaultCacheConfig,
  ) {
    this.config = apiConfig;
    this.cacheConfig = cacheConfig;
    this.startCacheCleanup();
  }

  private getCacheKey(endpoint: string, method: string = "GET"): string {
    return `${method}:${endpoint}`;
  }

  private isValidCache<T>(entry: CacheEntry<T>): boolean {
    return Date.now() - entry.timestamp < entry.ttl;
  }

  private setCache<T>(
    key: string,
    data: T,
    ttlMinutes: number = this.cacheConfig.defaultTtl / (60 * 1000),
  ): void {
    // Implement cache size limit
    if (this.cache.size >= this.cacheConfig.maxSize) {
      this.evictOldestEntry();
    }

    this.cache.set(key, {
      data,
      timestamp: Date.now(),
      ttl: ttlMinutes * 60 * 1000,
    });
  }

  private getFromCache<T>(key: string): T | null {
    const entry = this.cache.get(key);
    if (entry && this.isValidCache(entry)) {
      return entry.data;
    }
    this.cache.delete(key);
    return null;
  }

  async get<T>(endpoint: string, cacheTtl: number = 5): Promise<T> {
    const cacheKey = this.getCacheKey(endpoint);

    // Check cache first
    const cachedData = this.getFromCache<T>(cacheKey);
    if (cachedData) {
      return cachedData;
    }

    // Check for pending request to avoid duplicate calls
    if (this.pendingRequests.has(cacheKey)) {
      return this.pendingRequests.get(cacheKey);
    }

    try {
      const responsePromise = this.makeRequest(
        `${this.config.baseUrl}${endpoint}`,
        {
          method: "GET",
          headers: {
            "Content-Type": "application/json",
            "Cache-Control": "max-age=300",
            "Accept-Encoding": "gzip, deflate, br",
          },
          keepalive: true,
        },
      ).then((response) => response.json());

      this.pendingRequests.set(cacheKey, responsePromise);

      const data = await responsePromise;

      // Cache the result
      this.setCache(cacheKey, data, cacheTtl);

      // Remove from pending requests
      this.pendingRequests.delete(cacheKey);

      return data;
    } catch (error) {
      this.pendingRequests.delete(cacheKey);
      throw error;
    }
  }

  async post<T>(endpoint: string, data: Record<string, unknown>): Promise<T> {
    try {
      const response = await this.makeRequest(
        `${this.config.baseUrl}${endpoint}`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Accept-Encoding": "gzip, deflate, br",
          },
          body: JSON.stringify(data),
          keepalive: true,
        },
      );

      const result = await response.json();

      // Invalidate related cache entries after POST
      this.invalidateCache();

      if (result.error) {
        throw new ApiClientError(result.error);
      }

      return result as T;
    } catch (error) {
      if (error instanceof ApiClientError) {
        throw error;
      }
      throw new ApiClientError(
        error instanceof Error ? error.message : "Unknown error occurred",
      );
    }
  }

  private async makeRequest(
    url: string,
    options: RequestInit,
  ): Promise<Response> {
    const controller = new AbortController();
    const timeoutId = setTimeout(
      () => controller.abort(),
      this.config.requestTimeout,
    );

    try {
      const response = await fetch(url, {
        ...options,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        throw new ApiClientError(
          `API request failed: ${response.status} ${response.statusText}`,
          response.status,
        );
      }

      return response;
    } catch (error) {
      clearTimeout(timeoutId);
      if (error instanceof DOMException && error.name === "AbortError") {
        throw new ApiClientError("Request timeout", 408);
      }
      throw error;
    }
  }

  private evictOldestEntry(): void {
    const oldestKey = Array.from(this.cache.keys())[0];
    if (oldestKey) {
      this.cache.delete(oldestKey);
    }
  }

  private startCacheCleanup(): void {
    this.cleanupTimer = setInterval(() => {
      this.cleanupExpiredEntries();
    }, this.cacheConfig.cleanupInterval);
  }

  private cleanupExpiredEntries(): void {
    for (const [key, entry] of this.cache.entries()) {
      if (!this.isValidCache(entry)) {
        this.cache.delete(key);
      }
    }
  }

  // Clear cache when data changes
  private invalidateCache(): void {
    this.cache.clear();
  }

  // Public method to clear cache if needed
  clearCache(): void {
    this.cache.clear();
    this.pendingRequests.clear();
  }

  // Cleanup method for proper resource management
  destroy(): void {
    if (this.cleanupTimer) {
      clearInterval(this.cleanupTimer);
    }
    this.clearCache();
  }
}

export const apiClient = new ApiClient();

// Generic cursor-based API factory function
type CursorFetchFunction<T> = (
  cursor?: string,
  limit?: number,
) => Promise<CursorResponse<T>>;

/**
 * Creates a standardized cursor-based API function
 * @param endpoint - API endpoint path
 * @param transformer - Function to transform backend data to frontend format
 * @param defaultCacheTtl - Default cache TTL in minutes
 * @returns Cursor-based fetch function
 */
function createCursorApi<BackendType, FrontendType>(
  endpoint: string,
  transformer: (item: BackendType) => FrontendType,
  defaultCacheTtl: number = 10,
): CursorFetchFunction<FrontendType> {
  return async (
    cursor?: string,
    limit: number = 20,
  ): Promise<CursorResponse<FrontendType>> => {
    // Validate limit constraints
    if (limit < 1 || limit > 100) {
      throw new Error("Limit must be between 1 and 100");
    }

    const params = new URLSearchParams();
    params.set("limit", limit.toString());
    if (cursor) {
      params.set("cursor", cursor);
    }

    // Use different cache TTL based on context
    const cacheTtl = cursor ? defaultCacheTtl + 5 : defaultCacheTtl;
    const response = await apiClient.get<CursorResponse<BackendType>>(
      `${endpoint}?${params.toString()}`,
      cacheTtl,
    );

    // Guard against null or malformed responses
    if (!response || !Array.isArray(response.data)) {
      return {
        data: [],
        next_cursor: null,
      };
    }

    // Transform backend items to frontend format
    const transformedData = response.data.map(transformer);

    return {
      data: transformedData,
      next_cursor: response.next_cursor,
    };
  };
}

// Transform function to reduce data processing overhead
const transformFeedItem = (item: BackendFeedItem): Feed => ({
  id: item.link || "",
  title: item.title || "",
  description: item.description || "",
  link: item.link || "",
  published: item.published || "",
});

// Remove duplicate CursorResponse - use from common

export const feedsApi = {
  async checkHealth(): Promise<{ status: string }> {
    return apiClient.get("/v1/health", 1); // Short cache for health checks
  },

  getFeedsWithCursor: createCursorApi(
    "/v1/feeds/fetch/cursor",
    transformFeedItem,
    5, // 5 minute cache for regular feeds
  ),

  async getFeeds(page: number = 1, pageSize: number = 10): Promise<Feed[]> {
    const limit = page * pageSize;
    const response = await apiClient.get<BackendFeedItem[]>(
      `/v1/feeds/fetch/limit/${limit}`,
      10, // 10 minute cache for feed data
    );

    if (Array.isArray(response)) {
      return response.map(transformFeedItem);
    }

    return [];
  },

  async getFeedsPage(page: number = 0): Promise<Feed[]> {
    const response = await apiClient.get<BackendFeedItem[]>(
      `/v1/feeds/fetch/page/${page}`,
      10, // 10 minute cache for paginated data
    );

    if (Array.isArray(response)) {
      return response.map(transformFeedItem);
    }

    return [];
  },

  async getAllFeeds(): Promise<Feed[]> {
    const response = await apiClient.get<BackendFeedItem[]>(
      "/v1/feeds/fetch/list",
      15, // 15 minute cache for all feeds
    );

    if (Array.isArray(response)) {
      return response.map(transformFeedItem);
    }

    return [];
  },

  async getSingleFeed(): Promise<Feed> {
    return apiClient.get("/v1/feeds/fetch/single", 5);
  },

  async registerRssFeed(url: string): Promise<MessageResponse> {
    return apiClient.post("/v1/rss-feed-link/register", { url });
  },

  async registerFavoriteFeed(url: string): Promise<MessageResponse> {
    return apiClient.post("/v1/feeds/register/favorite", { url });
  },

  async updateFeedReadStatus(url: string): Promise<MessageResponse> {
    return apiClient.post("/v1/feeds/read", { feed_url: url });
  },

  async getFeedDetails(payload: FeedURLPayload): Promise<FeedDetails> {
    return apiClient.post<FeedDetails>(`/v1/feeds/fetch/details`, {
      feed_url: payload.feed_url,
    });
  },

  async searchArticles(query: string): Promise<Article[]> {
    return apiClient.get<Article[]>(`/v1/articles/search?q=${query}`);
  },

  // Method to prefetch data for performance
  async prefetchFeeds(pages: number[] = [0, 1]): Promise<void> {
    const prefetchPromises = pages.map((page) =>
      this.getFeedsPage(page).catch(() => {}),
    );
    await Promise.all(prefetchPromises);
  },

  async searchFeeds(query: string): Promise<FeedSearchResult> {
    try {
      const response = await apiClient.post<
        BackendFeedItem[] | FeedSearchResult
      >("/v1/feeds/search", { query });

      // Backend returns array directly, so wrap it in expected structure
      if (Array.isArray(response)) {
        return { results: response, error: null };
      }

      // If already in expected format, return as is
      return response as FeedSearchResult;
    } catch (error) {
      return {
        results: [],
        error:
          error instanceof ApiClientError ? error.message : "Search failed",
      };
    }
  },

  async getFeedStats(): Promise<FeedStatsSummary> {
    return apiClient.get<FeedStatsSummary>("/v1/feeds/stats", 5); // 5 minute cache for stats
  },

  async getTodayUnreadCount(since: string): Promise<UnreadCount> {
    return apiClient.get<UnreadCount>(
      `/v1/feeds/count/unreads?since=${encodeURIComponent(since)}`,
      1,
    );
  },

  getFavoriteFeedsWithCursor: createCursorApi(
    "/v1/feeds/fetch/favorites/cursor",
    transformFeedItem,
    10, // 10 minute cache for favorite feeds
  ),

  getReadFeedsWithCursor: createCursorApi(
    "/v1/feeds/fetch/viewed/cursor",
    transformFeedItem,
    10, // 10 minute cache for read feeds
  ),

  async prefetchFavoriteFeeds(cursors: string[]): Promise<void> {
    const prefetchPromises = cursors.map((cursor) =>
      this.getFavoriteFeedsWithCursor(cursor).catch(() => {}),
    );
    await Promise.all(prefetchPromises);
  },

  async prefetchReadFeeds(cursors: string[]): Promise<void> {
    const prefetchPromises = cursors.map((cursor) =>
      this.getReadFeedsWithCursor(cursor).catch(() => {}),
    );
    await Promise.all(prefetchPromises);
  },

  // Clear cache method
  clearCache(): void {
    apiClient.clearCache();
  },

  // Desktop Feed Methods
  async getDesktopFeeds(cursor?: string | null): Promise<DesktopFeedsResponse> {
    // For now, return mock data with pagination simulation
    // TODO: Replace with actual API call when backend is ready
    return new Promise((resolve) => {
      // Remove timeout for faster test execution
      const pageSize = 5;
      const startIndex = cursor ? parseInt(cursor) : 0;
      const endIndex = startIndex + pageSize;
      const paginatedFeeds = mockDesktopFeeds.slice(startIndex, endIndex);

      resolve({
        feeds: paginatedFeeds,
        nextCursor: endIndex < mockDesktopFeeds.length ? endIndex.toString() : null,
        hasMore: endIndex < mockDesktopFeeds.length,
        totalCount: mockDesktopFeeds.length
      });
    });
  },

  async toggleFavorite(feedId: string, isFavorited: boolean): Promise<MessageResponse> {
    // Mock API call for favorite toggle
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          message: isFavorited ? 'Added to favorites' : 'Removed from favorites'
        });
      }, 200);
    });
  },

  async toggleBookmark(feedId: string, isBookmarked: boolean): Promise<MessageResponse> {
    // Mock API call for bookmark toggle
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          message: isBookmarked ? 'Added to bookmarks' : 'Removed from bookmarks'
        });
      }, 200);
    });
  },

  async getRecentActivity(limit: number = 10): Promise<ActivityResponse[]> {
    // Mock API call for recent activity
    // TODO: Replace with actual API call when backend is ready
    return new Promise((resolve) => {
      setTimeout(() => {
        const mockActivities: ActivityResponse[] = [
          {
            id: 1,
            type: 'new_feed',
            title: 'Added TechCrunch RSS feed',
            timestamp: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString() // 2 hours ago
          },
          {
            id: 2,
            type: 'ai_summary',
            title: 'AI summary generated for 5 articles',
            timestamp: new Date(Date.now() - 4 * 60 * 60 * 1000).toISOString() // 4 hours ago
          },
          {
            id: 3,
            type: 'bookmark',
            title: 'Bookmarked "Introduction to React 19"',
            timestamp: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString() // 1 day ago
          },
          {
            id: 4,
            type: 'read',
            title: 'Read "Modern JavaScript Features"',
            timestamp: new Date(Date.now() - 48 * 60 * 60 * 1000).toISOString() // 2 days ago
          }
        ];

        resolve(mockActivities.slice(0, limit));
      }, 300); // Simulate network delay
    });
  },

  async getWeeklyStats(): Promise<WeeklyStats> {
    // Mock API call for weekly stats
    // TODO: Replace with actual API call when backend is ready
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          weeklyReads: 45,
          aiProcessed: 18,
          bookmarks: 12
        });
      }, 200);
    });
  },
};
