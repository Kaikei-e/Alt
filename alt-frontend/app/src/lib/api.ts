import {
  BackendFeedItem,
  Feed,
  FeedDetails,
  FeedURLPayload,
} from "@/schema/feed";
import { FeedSearchResult } from "@/schema/search";
import { Article } from "@/schema/article";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost/api";

export type ApiResponse<T> = {
  data: T;
};

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
  private baseUrl: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private cache = new Map<string, CacheEntry<any>>();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private pendingRequests = new Map<string, Promise<any>>();

  constructor(baseUrl: string = API_BASE_URL) {
    this.baseUrl = baseUrl;
  }

  private getCacheKey(endpoint: string, method: string = "GET"): string {
    return `${method}:${endpoint}`;
  }

  private isValidCache<T>(entry: CacheEntry<T>): boolean {
    return Date.now() - entry.timestamp < entry.ttl;
  }

  private setCache<T>(key: string, data: T, ttlMinutes: number = 5): void {
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
      const requestPromise = fetch(`${this.baseUrl}${endpoint}`, {
        method: "GET",
        headers: {
          "Content-Type": "application/json",
          // Add performance headers
          "Cache-Control": "max-age=300", // 5 minutes client cache
          "Accept-Encoding": "gzip, deflate, br",
        },
        // Use keep-alive for connection reuse
        keepalive: true,
      }).then(async (response) => {
        if (!response.ok) {
          throw new Error(
            `API request failed: ${response.status} ${response.statusText}`,
          );
        }
        return response.json();
      });

      this.pendingRequests.set(cacheKey, requestPromise);

      const data = await requestPromise;

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
      const response = await fetch(`${this.baseUrl}${endpoint}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Accept-Encoding": "gzip, deflate, br",
        },
        body: JSON.stringify(data),
        keepalive: true,
      });

      if (!response.ok) {
        throw new Error(
          `API request failed: ${response.status} ${response.statusText}`,
        );
      }

      const result = await response.json();

      // Invalidate related cache entries after POST
      this.invalidateCache();

      if (result.error) {
        throw new Error(result.error);
      }

      return result as T;
    } catch (error) {
      throw error;
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

export const feedsApi = {
  async checkHealth(): Promise<{ status: string }> {
    return apiClient.get("/v1/health", 1); // Short cache for health checks
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
    const prefetchPromises = pages.map(
      (page) => this.getFeedsPage(page).catch(() => {

      }),
    );
    await Promise.all(prefetchPromises);
  },

  async searchFeeds(query: string): Promise<FeedSearchResult> {
    try {
      const response = await fetch(`${API_BASE_URL}/v1/feeds/search`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ query }),
      });

      if (!response.ok) {
        throw new Error(`API request failed: ${response.status} ${response.statusText}`);
      }

      const result = await response.json();

      // Backend returns array directly, so wrap it in expected structure
      if (Array.isArray(result)) {
        return { results: result, error: null };
      }

      // If already in expected format, return as is
      return result;
    } catch (error) {
      return {
        results: [],
        error: error instanceof Error ? error.message : "Search failed"
      };
    }
  },

  // Clear cache method
  clearCache(): void {
    apiClient.clearCache();
  },
};
