import { ApiError } from "./ApiError";
import { CacheManager, defaultCacheConfig } from "../cache/CacheManager";
import { AuthInterceptor, LoginBanner } from "../auth";
import { authAPI } from "../auth-client";

export interface ApiConfig {
  baseUrl: string;
  requestTimeout: number;
}

export const defaultApiConfig: ApiConfig = {
  baseUrl:
    typeof window === "undefined"
      ? process.env.API_URL || "http://localhost:8080" // SSR: 内向き
      : "/api/backend", // Client: 外向き（Nginx書き換え）
  requestTimeout: 120000, // 2 minutes
};

export class ApiClient {
  private config: ApiConfig;
  private cacheManager: CacheManager;
  private authInterceptor: AuthInterceptor;
  private pendingRequests = new Map<string, Promise<any>>();
  private loginBanner: LoginBanner;
  constructor(
    config: ApiConfig = defaultApiConfig,
    cacheManager?: CacheManager,
    authInterceptor?: AuthInterceptor,
  ) {
    this.config = config;
    this.cacheManager = cacheManager || new CacheManager(defaultCacheConfig);
    this.loginBanner = new LoginBanner();
    this.authInterceptor =
      authInterceptor ||
      new AuthInterceptor({
        onAuthRequired: () => this.loginBanner.show(),
      });
  }

  async get<T>(endpoint: string, cacheTtl: number = 5): Promise<T> {
    const cacheKey = this.cacheManager.getCacheKey(endpoint);

    if (process.env.NODE_ENV === "development") {
    }

    // Check cache first
    const cachedData = this.cacheManager.get<T>(cacheKey);
    if (cachedData) {
      if (process.env.NODE_ENV === "development") {
      }
      return cachedData;
    }

    // Check for pending request to avoid duplicate calls
    if (this.pendingRequests.has(cacheKey)) {
      if (process.env.NODE_ENV === "development") {
      }
      return this.pendingRequests.get(cacheKey);
    }

    if (process.env.NODE_ENV === "development") {
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
      ).then(async (response) => {
        const interceptedResponse = await this.authInterceptor.intercept(
          response,
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
        );
        return interceptedResponse.json();
      });

      this.pendingRequests.set(cacheKey, responsePromise);

      const data = await responsePromise;

      // Cache the result
      this.cacheManager.set(cacheKey, data, cacheTtl);

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
      // Get CSRF token for state-changing operations
      const csrfToken = await authAPI.getCSRFToken();

      const headers: Record<string, string> = {
        "Content-Type": "application/json",
        "Accept-Encoding": "gzip, deflate, br",
      };

      // Add CSRF token if available
      if (csrfToken) {
        headers["X-CSRF-Token"] = csrfToken;
      } else if (process.env.NODE_ENV === "development") {
      }

      const response = await this.makeRequest(
        `${this.config.baseUrl}${endpoint}`,
        {
          method: "POST",
          headers,
          body: JSON.stringify(data),
          keepalive: true,
        },
      );

      const interceptedResponse = await this.authInterceptor.intercept(
        response,
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

      const result = await interceptedResponse.json();

      // Invalidate related cache entries after POST
      this.cacheManager.invalidate();

      if (result.error) {
        throw new ApiError(result.error);
      }

      return result as T;
    } catch (error) {
      if (error instanceof ApiError) {
        throw error;
      }
      throw new ApiError(
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

    // SSR Cookie handling
    let enhancedOptions = { ...options };
    if (typeof window === "undefined") {
      try {
        const { cookies } = await import("next/headers");
        const cookieStore = await cookies();
        const cookieHeader = cookieStore.toString();

        enhancedOptions.headers = {
          ...enhancedOptions.headers,
          cookie: cookieHeader,
        };
      } catch (error) {
        // Ignore import errors in non-Next.js environments
      }
    }

    try {
      const identityHeaders = await this.resolveIdentityHeaders();
      if (Object.keys(identityHeaders).length > 0) {
        const mergedHeaders: Record<string, string> = {};
        const existing = enhancedOptions.headers;

        if (existing instanceof Headers) {
          existing.forEach((value, key) => {
            mergedHeaders[key] = value;
          });
        } else if (Array.isArray(existing)) {
          for (const [key, value] of existing) {
            mergedHeaders[key] = value as string;
          }
        } else if (existing && typeof existing === "object") {
          Object.assign(mergedHeaders, existing as Record<string, string>);
        }

        enhancedOptions.headers = {
          ...mergedHeaders,
          ...identityHeaders,
        };
      }
    } catch {
      // Ignore header enrichment errors to avoid blocking the request outright.
    }

    try {
      const response = await fetch(url, {
        ...enhancedOptions,
        credentials: "include",
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      // Don't throw error for 401s - let auth interceptor handle them
      if (!response.ok && response.status !== 401) {
        throw new ApiError(
          `API request failed: ${response.status} ${response.statusText}`,
          response.status,
        );
      }

      return response;
    } catch (error) {
      clearTimeout(timeoutId);
      if (error instanceof DOMException && error.name === "AbortError") {
        throw new ApiError("Request timeout", 408);
      }
      throw error;
    }
  }

  clearCache(): void {
    this.cacheManager.clear();
    this.pendingRequests.clear();
  }

  destroy(): void {
    this.cacheManager.destroy();
    this.clearCache();
  }

  private async resolveIdentityHeaders(): Promise<Record<string, string>> {
    try {
      let headers: Record<string, string> = {};

      if (typeof window !== "undefined") {
        headers = (await authAPI.getSessionHeaders()) ?? {};
      } else {
        const { getServerSessionHeaders } = await import(
          "../../auth/server-headers"
        );
        headers = (await getServerSessionHeaders()) ?? {};
      }

      // Development logging to help debug authentication issues
      if (process.env.NODE_ENV === "development") {
        if (Object.keys(headers).length > 0) {
        } else {
        }
      }

      return headers;
    } catch (error) {
      // 本番環境ではログを出力しない（セキュリティ上の理由）
      if (process.env.NODE_ENV === "development") {
        console.error("[ApiClient] Failed to resolve identity headers", error);
      }
      return {};
    }
  }
}
