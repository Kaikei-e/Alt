import {
  BackendFeedItem,
  Feed,
  FeedDetails,
  FeedURLPayload,
} from "@/schema/feed";
import { FeedSearchResult } from "@/schema/search";
import { Article } from "@/schema/article";
import { FeedStatsSummary } from "@/schema/feedStats";
import {
  ApiConfig,
  defaultApiConfig,
  CacheConfig,
  defaultCacheConfig,
} from "@/lib/config";

export type ApiResponse<T> = {
  data: T;
};

export type ApiError = {
  message: string;
  status?: number;
  code?: string;
};

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

interface message {
  message: string;
}

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

// Transform function to reduce data processing overhead
const transformFeedItem = (item: BackendFeedItem): Feed => ({
  id: item.link || "",
  title: item.title || "",
  description: item.description || "",
  link: item.link || "",
  published: item.published || "",
});

export type CursorResponse<T> = {
  data: T[];
  next_cursor: string | null;
};

export const feedsApi = {
  async checkHealth(): Promise<{ status: string }> {
    return apiClient.get("/v1/health", 1); // Short cache for health checks
  },

  async getFeedsWithCursor(cursor?: string, limit: number = 20): Promise<CursorResponse<Feed>> {
    // Validate limit constraints
    if (limit < 1 || limit > 100) {
      throw new Error("Limit must be between 1 and 100");
    }

    const params = new URLSearchParams();
    params.set("limit", limit.toString());
    if (cursor) {
      params.set("cursor", cursor);
    }

    // Use different cache TTL based on whether it's first page or not
    const cacheTtl = cursor ? 15 : 5; // 15 min for subsequent pages, 5 min for first page
    const response = await apiClient.get<CursorResponse<BackendFeedItem>>(
      `/v1/feeds/fetch/cursor?${params.toString()}`,
      cacheTtl
    );

    // Transform backend feed items to frontend format
    const transformedData = response.data.map(transformFeedItem);

    return {
      data: transformedData,
      next_cursor: response.next_cursor,
    };
  },

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

  async registerRssFeed(url: string): Promise<message> {
    return apiClient.post("/v1/rss-feed-link/register", { url });
  },

  async updateFeedReadStatus(url: string): Promise<message> {
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
      this.getFeedsPage(page).catch(() => { }),
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

  // Clear cache method
  clearCache(): void {
    apiClient.clearCache();
  },
};
