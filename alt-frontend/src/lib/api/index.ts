// Main API entry point - provides backward compatibility with original api.ts
// Updated: Added getArticlesWithCursor for article pagination
import { ApiClient, defaultApiConfig } from "./core/ApiClient";
import { CacheManager, defaultCacheConfig } from "./cache/CacheManager";
import { AuthInterceptor, LoginBanner } from "./auth";
import { FeedsApi } from "./feeds/FeedsApi";
import { DesktopApi } from "./desktop/DesktopApi";

// Re-export types for external use
export type { CursorResponse } from "@/schema/common";

// Re-export errors for backward compatibility
export { ApiError as ApiClientError } from "./core/ApiError";

// Re-export server fetch utility
export { serverFetch } from "./utils/serverFetch";

// Create singleton instances for backward compatibility
const cacheManager = new CacheManager(defaultCacheConfig);
const loginBanner = new LoginBanner();
const authInterceptor = new AuthInterceptor({
  onAuthRequired: () => loginBanner.show(),
});

export const apiClient = new ApiClient(
  defaultApiConfig,
  cacheManager,
  authInterceptor,
);

const feedsApiInstance = new FeedsApi(apiClient);
const desktopApiInstance = new DesktopApi(apiClient, feedsApiInstance);

// Combine all APIs into the main feedsApi object for backward compatibility
export const feedsApi = {
  // Health check
  checkHealth: feedsApiInstance.checkHealth.bind(feedsApiInstance),

  // Cursor-based APIs
  getFeedsWithCursor: feedsApiInstance.getFeedsWithCursor.bind(feedsApiInstance),
  getFavoriteFeedsWithCursor: feedsApiInstance.getFavoriteFeedsWithCursor.bind(feedsApiInstance),
  getReadFeedsWithCursor: feedsApiInstance.getReadFeedsWithCursor.bind(feedsApiInstance),
  getArticlesWithCursor: feedsApiInstance.getArticlesWithCursor.bind(feedsApiInstance),

  // Legacy pagination methods
  getFeeds: feedsApiInstance.getFeeds.bind(feedsApiInstance),
  getFeedsPage: feedsApiInstance.getFeedsPage.bind(feedsApiInstance),
  getAllFeeds: feedsApiInstance.getAllFeeds.bind(feedsApiInstance),
  getSingleFeed: feedsApiInstance.getSingleFeed.bind(feedsApiInstance),

  // Feed management
  registerRssFeed: feedsApiInstance.registerRssFeed.bind(feedsApiInstance),
  registerFavoriteFeed:
    feedsApiInstance.registerFavoriteFeed.bind(feedsApiInstance),
  updateFeedReadStatus:
    feedsApiInstance.updateFeedReadStatus.bind(feedsApiInstance),

  // Article summaries
  getArticleSummary: feedsApiInstance.getArticleSummary.bind(feedsApiInstance),
  getFeedDetails: feedsApiInstance.getFeedDetails.bind(feedsApiInstance),
  archiveContent: feedsApiInstance.archiveContent.bind(feedsApiInstance),
  summarizeArticle: feedsApiInstance.summarizeArticle.bind(feedsApiInstance),

  // Feed content on the fly
  getFeedContentOnTheFly:
    feedsApiInstance.getFeedContentOnTheFly.bind(feedsApiInstance),
  // Search
  searchArticles: feedsApiInstance.searchArticles.bind(feedsApiInstance),
  searchFeeds: feedsApiInstance.searchFeeds.bind(feedsApiInstance),

  // Statistics
  getFeedStats: feedsApiInstance.getFeedStats.bind(feedsApiInstance),
  getFeedStatsSSR: feedsApiInstance.getFeedStatsSSR.bind(feedsApiInstance),
  getTodayUnreadCount:
    feedsApiInstance.getTodayUnreadCount.bind(feedsApiInstance),

  // Tags
  fetchFeedTags: feedsApiInstance.fetchFeedTags.bind(feedsApiInstance),

  // Prefetch methods
  prefetchFeeds: feedsApiInstance.prefetchFeeds.bind(feedsApiInstance),
  prefetchFavoriteFeeds:
    feedsApiInstance.prefetchFavoriteFeeds.bind(feedsApiInstance),
  prefetchReadFeeds: feedsApiInstance.prefetchReadFeeds.bind(feedsApiInstance),

  // Desktop API methods
  getDesktopFeeds: desktopApiInstance.getDesktopFeeds.bind(desktopApiInstance),
  getTestFeeds: desktopApiInstance.getTestFeeds.bind(desktopApiInstance),
  toggleFavorite: desktopApiInstance.toggleFavorite.bind(desktopApiInstance),
  toggleBookmark: desktopApiInstance.toggleBookmark.bind(desktopApiInstance),
  getRecentActivity:
    desktopApiInstance.getRecentActivity.bind(desktopApiInstance),
  getWeeklyStats: desktopApiInstance.getWeeklyStats.bind(desktopApiInstance),

  // Cache management
  clearCache: feedsApiInstance.clearCache.bind(feedsApiInstance),
};
