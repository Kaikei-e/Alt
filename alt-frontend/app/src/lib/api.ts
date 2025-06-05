import { Feed } from "@/schema/feed";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost/api";

export type ApiResponse<T> = {
  data: T;
};

// Backend response types
interface BackendFeedItem {
  title: string;
  description: string;
  link: string;
  links?: string[];
  published?: string;
  author?: {
    name: string;
  };
  authors?: Array<{
    name: string;
  }>;
}

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string = API_BASE_URL) {
    this.baseUrl = baseUrl;
  }

  async get<T>(endpoint: string): Promise<T> {
    try {
      const response = await fetch(`${this.baseUrl}${endpoint}`, {
        method: "GET",
        headers: {
          "Content-Type": "application/json",
        },
      });

      if (!response.ok) {
        throw new Error(
          `API request failed: ${response.status} ${response.statusText}`,
        );
      }

      const data = await response.json();
      return data;
    } catch (error) {
      throw error;
    }
  }

  async post<T>(endpoint: string, data: Record<string, unknown>): Promise<T> {
    try {
      const response = await fetch(`${this.baseUrl}${endpoint}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(data),
      });

      if (!response.ok) {
        throw new Error(
          `API request failed: ${response.status} ${response.statusText}`,
        );
      }

      return response.json();
    } catch (error) {
      throw error;
    }
  }
}

export const apiClient = new ApiClient();

export const feedsApi = {
  async checkHealth(): Promise<{ status: string }> {
    return apiClient.get("/v1/health");
  },

  async getFeeds(page: number = 1, pageSize: number = 20): Promise<Feed[]> {
    const limit = page * pageSize;
    const response = await apiClient.get<BackendFeedItem[]>(
      `/v1/feeds/fetch/limit/${limit}`,
    );

    if (Array.isArray(response)) {
      return response.map((item: BackendFeedItem) => ({
        id: item.link || "",
        title: item.title || "",
        description: item.description || "",
        link: item.link || "",
        published: item.published || "",
      }));
    }

    return [];
  },

  async getFeedsPage(page: number = 0): Promise<Feed[]> {
    const response = await apiClient.get<BackendFeedItem[]>(
      `/v1/feeds/fetch/page/${page}`,
    );

    if (Array.isArray(response)) {
      return response.map((item: BackendFeedItem) => ({
        id: item.link || "",
        title: item.title || "",
        description: item.description || "",
        link: item.link || "",
        published: item.published || "",
      }));
    }

    return [];
  },

  async getAllFeeds(): Promise<Feed[]> {
    const response = await apiClient.get<BackendFeedItem[]>(
      "/v1/feeds/fetch/list",
    );

    if (Array.isArray(response)) {
      return response.map((item: BackendFeedItem) => ({
        id: item.link || "",
        title: item.title || "",
        description: item.description || "",
        link: item.link || "",
        published: item.published || "",
      }));
    }

    return [];
  },

  async getSingleFeed(): Promise<Feed> {
    return apiClient.get("/v1/feeds/fetch/single");
  },

  async registerRssFeed(url: string): Promise<{ message: string }> {
    return apiClient.post("/v1/rss-feed-link/register", { url });
  },
};
