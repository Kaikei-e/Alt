import { ApiClientError, feedApi } from "@/lib/api";
import type { DesktopFeed } from "@/types/desktop-feed";
import { sanitizeFeedContent } from "@/utils/contentSanitizer";

export class DesktopFeedsApi {
  private baseUrl: string;
  private cache: Map<string, unknown> = new Map();

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  async getDesktopFeeds(cursor?: string | null): Promise<{
    feeds: DesktopFeed[];
    nextCursor: string | null;
    hasMore: boolean;
  }> {
    const cacheKey = `desktop-feeds-${cursor || "initial"}`;

    if (this.cache.has(cacheKey)) {
      return this.cache.get(cacheKey) as {
        feeds: DesktopFeed[];
        nextCursor: string | null;
        hasMore: boolean;
      };
    }

    const params = new URLSearchParams();
    if (cursor) params.append("cursor", cursor);

    const response = await fetch(`${this.baseUrl}/api/backend/feeds/desktop?${params}`, {
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
    });

    if (!response.ok) {
      throw new ApiClientError(
        `Failed to fetch desktop feeds: ${response.statusText}`,
        response.status
      );
    }

    const data = await response.json();

    // Transform API response to DesktopFeed format
    const transformedData = {
      feeds: data.feeds.map(this.transformFeedData),
      nextCursor: data.nextCursor,
      hasMore: data.hasMore,
    };

    // Cache for 2 minutes
    this.cache.set(cacheKey, transformedData);
    setTimeout(() => this.cache.delete(cacheKey), 2 * 60 * 1000);

    return transformedData;
  }

  private transformFeedData(apiData: Record<string, unknown>): DesktopFeed {
    const source = (apiData.source as Record<string, unknown>) || {};
    const engagement = (apiData.engagement as Record<string, unknown>) || {};

    // Sanitize content before processing
    const sanitizedContent = sanitizeFeedContent({
      title: String(apiData.title || ""),
      description: String(apiData.description || ""),
      author: String(apiData.author || ""),
      link: String(apiData.link || ""),
    });

    return {
      id: String(apiData.id || ""),
      title: sanitizedContent.title,
      description: sanitizedContent.description,
      link: sanitizedContent.link,
      published: String(apiData.published || ""),
      metadata: {
        source: {
          id: String(source.id || ""),
          name: String(source.name || ""),
          icon: this.getSourceIcon(String(source.name || "")),
          reliability: Number(source.reliability) || 8.0,
          category: String(source.category || "general"),
          unreadCount: Number(source.unreadCount) || 0,
          avgReadingTime: Number(source.avgReadingTime) || 5,
        },
        readingTime: this.estimateReadingTime(String(apiData.description || "")),
        engagement: {
          // views: Number(engagement.views) || 0,      // Removed: SNS element
          // comments: Number(engagement.comments) || 0,// Removed: SNS element
          likes: Number(engagement.likes) || 0,
          bookmarks: Number(engagement.bookmarks) || 0,
        },
        tags: Array.isArray(apiData.tags) ? (apiData.tags as string[]) : [],
        relatedCount: Number(apiData.relatedCount) || 0,
        publishedAt: this.formatRelativeTime(String(apiData.published || "")),
        author: sanitizedContent.author || undefined,
        summary: apiData.summary
          ? sanitizeFeedContent({
              title: "",
              description: String(apiData.summary),
              author: "",
              link: "",
            }).description
          : this.generateSummary(sanitizedContent.description),
        priority: this.calculatePriority(apiData),
        category: String(apiData.category || "general"),
        difficulty: String(apiData.difficulty || "intermediate") as
          | "beginner"
          | "intermediate"
          | "advanced",
      },
      isRead: Boolean(apiData.isRead),
      isFavorited: Boolean(apiData.isFavorited),
      isBookmarked: Boolean(apiData.isBookmarked),
      readingProgress: apiData.readingProgress ? Number(apiData.readingProgress) : undefined,
    };
  }

  private getSourceIcon(sourceName: string): string {
    const iconMap: Record<string, string> = {
      TechCrunch: "📰",
      "Hacker News": "🔥",
      Medium: "📝",
      "Dev.to": "💻",
      GitHub: "🐙",
      "Stack Overflow": "📚",
      Reddit: "🤖",
    };
    return iconMap[sourceName] || "📄";
  }

  private estimateReadingTime(content: string): number {
    const wordsPerMinute = 200;
    const wordCount = content.split(/\s+/).length;
    return Math.max(1, Math.ceil(wordCount / wordsPerMinute));
  }

  private calculatePriority(feedData: Record<string, unknown>): "high" | "medium" | "low" {
    const engagement = (feedData.engagement as Record<string, unknown>) || {};
    // Calculate priority based on RSS-specific engagement (likes, bookmarks)
    const score = (Number(engagement.likes) || 0) * 0.5 + (Number(engagement.bookmarks) || 0) * 2;

    if (score > 50) return "high";
    if (score > 10) return "medium";
    return "low";
  }

  private formatRelativeTime(date: string): string {
    const now = new Date();
    const publishedDate = new Date(date);
    const diffHours = Math.floor((now.getTime() - publishedDate.getTime()) / (1000 * 60 * 60));

    if (diffHours < 1) return "Just now";
    if (diffHours < 24) return `${diffHours} hours ago`;
    if (diffHours < 48) return "Yesterday";
    return `${Math.floor(diffHours / 24)} days ago`;
  }

  private generateSummary(description: string): string {
    return description.length > 150 ? description.substring(0, 147) + "..." : description;
  }

  async markAsRead(feedId: string): Promise<void> {
    // 既存のfeedApiを利用
    await feedApi.updateFeedReadStatus(feedId);
    this.invalidateFeedCaches();
  }

  async toggleFavorite(feedId: string, isFavorited: boolean): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/backend/feeds/${feedId}/favorite`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ isFavorited }),
    });

    if (!response.ok) {
      throw new ApiClientError("Failed to toggle favorite", response.status);
    }
  }

  async toggleBookmark(feedId: string, isBookmarked: boolean): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/backend/feeds/${feedId}/bookmark`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ isBookmarked }),
    });

    if (!response.ok) {
      throw new ApiClientError("Failed to toggle bookmark", response.status);
    }
  }

  private invalidateFeedCaches(): void {
    for (const key of this.cache.keys()) {
      if (key.startsWith("desktop-feeds-")) {
        this.cache.delete(key);
      }
    }
  }
}

import { PUBLIC_API_BASE_URL } from "@/lib/env.public";

export const desktopFeedsApi = new DesktopFeedsApi(PUBLIC_API_BASE_URL || "http://localhost:8080");
