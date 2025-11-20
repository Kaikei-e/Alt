// Main API entry point - provides backward compatibility with original api.ts
// Updated: Added getArticlesWithCursor for article pagination

import { ArticleApi } from "./articles/ArticleApi";
import { AuthInterceptor, LoginBanner } from "./auth";
import { CacheManager, defaultCacheConfig } from "./cache/CacheManager";
import { ApiClient, defaultApiConfig } from "./core/ApiClient";
import { DesktopApi } from "./desktop/DesktopApi";
import { FeedApi } from "./feeds/FeedApi";
import { MorningApi } from "./morning/MorningApi";
import { RecapApi } from "./recap/RecapApi";

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

const feedApiInstance = new FeedApi(apiClient);
const articleApiInstance = new ArticleApi(apiClient);
const desktopApiInstance = new DesktopApi(apiClient, feedApiInstance);
const recapApiInstance = new RecapApi(apiClient);
const morningApiInstance = new MorningApi(apiClient);

export const feedApi = feedApiInstance;
export const articleApi = articleApiInstance;
export const desktopApi = desktopApiInstance;
export const recapApi = recapApiInstance;
export const morningApi = morningApiInstance;
