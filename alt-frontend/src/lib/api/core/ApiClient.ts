import { AuthInterceptor, LoginBanner } from "../auth";
import { authAPI } from "../auth-client";
import { CacheManager, defaultCacheConfig } from "../cache/CacheManager";
import { ApiError } from "./ApiError";

/**
 * Maps HTTP status codes to user-friendly error messages
 * Following existing patterns from errorHandler.ts and ErrorState.tsx
 */
function getUserFriendlyErrorMessage(
  status: number,
  statusText?: string,
): string {
  switch (status) {
    case 400:
      return "Invalid request. Please check your input and try again.";
    case 401:
      return "Authentication required. Please log in.";
    case 403:
      return "You don't have permission to perform this action.";
    case 404:
      return "The requested resource was not found.";
    case 408:
      return "Request timeout. Please try again later.";
    case 429:
      return "Too many requests. Please try again later.";
    case 500:
      return "We're having some trouble on our end. Please try again later.";
    case 502:
      return "Unable to connect to the service. Please try again later.";
    case 503:
      return "Service temporarily unavailable. Please try again later.";
    case 504:
      return "Server response timeout. Please try again later.";
    default:
      if (status >= 500) {
        return "We're having some trouble on our end. Please try again later.";
      }
      if (status >= 400) {
        return "Invalid request. Please check your input and try again.";
      }
      return "An unexpected error occurred. Please try again later.";
  }
}

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

  /**
   * Resolves the full request URL from an endpoint string.
   * Handles special cases:
   * - Absolute URLs (http:// or https://) are BLOCKED in client-side for SSRF protection
   * - Next.js API routes (/api/...) use app origin (relative in browser, absolute in SSR)
   * - Other endpoints use baseUrl prefix
   */
  private resolveRequestUrl(endpoint: string): string {
    // SSRF Protection: Block absolute URLs in client-side
    // URLs should be passed in POST body and validated server-side
    if (endpoint.startsWith("http://") || endpoint.startsWith("https://")) {
      if (typeof window !== "undefined") {
        // Client-side: reject absolute URLs
        throw new ApiError(
          "Absolute URLs are not allowed. Use relative paths or POST body.",
          400,
          "SSRF_PROTECTION",
        );
      }
      // Server-side: still allow but should be validated by API routes
      // In practice, API routes should validate URLs from POST body
      return endpoint;
    }

    // Next.js API route - use app origin
    // Handles /api/frontend/ (Next.js API routes) and other /api/ paths
    if (endpoint.startsWith("/api/")) {
      if (typeof window !== "undefined") {
        // Browser: use relative path
        return endpoint;
      }

      // SSR: build absolute URL from app origin
      const appOrigin =
        process.env.NEXT_PUBLIC_APP_ORIGIN?.trim() ||
        process.env.NEXT_PUBLIC_APP_URL?.trim() ||
        "http://localhost:3000";

      return `${appOrigin}${endpoint}`;
    }

    // Default: use baseUrl prefix
    return `${this.config.baseUrl}${endpoint}`;
  }

  async get<T>(
    endpoint: string,
    cacheTtl: number = 5,
    timeout?: number,
  ): Promise<T> {
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
      const requestTimeout = timeout ?? this.config.requestTimeout;
      const requestUrl = this.resolveRequestUrl(endpoint);
      const responsePromise = this.makeRequest(
        requestUrl,
        {
          method: "GET",
          headers: {
            "Content-Type": "application/json",
            "Cache-Control": "max-age=300",
            "Accept-Encoding": "gzip, deflate, br",
          },
          keepalive: true,
        },
        requestTimeout,
      ).then(async (response) => {
        const interceptedResponse = await this.authInterceptor.intercept(
          response,
          requestUrl,
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

      const requestUrl = this.resolveRequestUrl(endpoint);
      const response = await this.makeRequest(
        requestUrl,
        {
          method: "POST",
          headers,
          body: JSON.stringify(data),
          keepalive: true,
        },
      );

      const interceptedResponse = await this.authInterceptor.intercept(
        response,
        requestUrl,
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

      // Check for error response (backend returns error in various formats)
      if (result.error || result.code) {
        const errorCode = result.code || "UNKNOWN_ERROR";
        const statusCode = interceptedResponse.status;
        // Prefer backend message if available, otherwise use user-friendly status code message
        const errorMessage =
          result.message ||
          result.error ||
          getUserFriendlyErrorMessage(statusCode);

        // Log technical details for developers (development only)
        if (process.env.NODE_ENV === "development") {
          console.error("[ApiClient] POST request failed", {
            endpoint,
            status: statusCode,
            code: errorCode,
            message: errorMessage,
          });
        }

        throw new ApiError(errorMessage, statusCode, errorCode);
      }

      return result as T;
    } catch (error) {
      if (error instanceof ApiError) {
        throw error;
      }
      // Log technical details for developers (development only)
      if (process.env.NODE_ENV === "development") {
        console.error("[ApiClient] POST request error", { endpoint, error });
      }
      throw new ApiError(
        error instanceof Error
          ? error.message
          : "An unexpected error occurred. Please try again later.",
      );
    }
  }

  async patch<T>(endpoint: string, data: Record<string, unknown>): Promise<T> {
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

      const requestUrl = this.resolveRequestUrl(endpoint);
      const response = await this.makeRequest(
        requestUrl,
        {
          method: "PATCH",
          headers,
          body: JSON.stringify(data),
          keepalive: true,
        },
      );

      const interceptedResponse = await this.authInterceptor.intercept(
        response,
        requestUrl,
        {
          method: "PATCH",
          headers: {
            "Content-Type": "application/json",
            "Accept-Encoding": "gzip, deflate, br",
          },
          body: JSON.stringify(data),
          keepalive: true,
        },
      );

      const result = await interceptedResponse.json();

      // Invalidate related cache entries after PATCH
      this.cacheManager.invalidate();

      // Check for error response (backend returns error in various formats)
      if (result.error || result.code) {
        const errorCode = result.code || "UNKNOWN_ERROR";
        const statusCode = interceptedResponse.status;
        // Prefer backend message if available, otherwise use user-friendly status code message
        const errorMessage =
          result.message ||
          result.error ||
          getUserFriendlyErrorMessage(statusCode);

        // Log technical details for developers (development only)
        if (process.env.NODE_ENV === "development") {
          console.error("[ApiClient] PATCH request failed", {
            endpoint,
            status: statusCode,
            code: errorCode,
            message: errorMessage,
          });
        }

        throw new ApiError(errorMessage, statusCode, errorCode);
      }

      return result as T;
    } catch (error) {
      if (error instanceof ApiError) {
        throw error;
      }
      // Log technical details for developers (development only)
      if (process.env.NODE_ENV === "development") {
        console.error("[ApiClient] PATCH request error", { endpoint, error });
      }
      throw new ApiError(
        error instanceof Error
          ? error.message
          : "An unexpected error occurred. Please try again later.",
      );
    }
  }

  async delete<T>(endpoint: string): Promise<T> {
    try {
      const csrfToken = await authAPI.getCSRFToken();

      const headers: Record<string, string> = {
        "Content-Type": "application/json",
        "Accept-Encoding": "gzip, deflate, br",
      };

      if (csrfToken) {
        headers["X-CSRF-Token"] = csrfToken;
      }

      const requestUrl = this.resolveRequestUrl(endpoint);
      const response = await this.makeRequest(
        requestUrl,
        {
          method: "DELETE",
          headers,
          keepalive: true,
        },
      );

      const interceptedResponse = await this.authInterceptor.intercept(
        response,
        requestUrl,
        {
          method: "DELETE",
          headers: {
            "Content-Type": "application/json",
            "Accept-Encoding": "gzip, deflate, br",
          },
          keepalive: true,
        },
      );

      const result = await interceptedResponse.json();

      if (result.error || result.code) {
        const errorCode = result.code || "UNKNOWN_ERROR";
        const statusCode = interceptedResponse.status;
        const errorMessage =
          result.message || result.error ||
          getUserFriendlyErrorMessage(statusCode);

        if (process.env.NODE_ENV === "development") {
          console.error("[ApiClient] DELETE request failed", {
            endpoint,
            status: statusCode,
            code: errorCode,
            message: errorMessage,
          });
        }

        throw new ApiError(errorMessage, statusCode, errorCode);
      }

      this.cacheManager.invalidate();

      return result as T;
    } catch (error) {
      if (error instanceof ApiError) {
        throw error;
      }
      if (process.env.NODE_ENV === "development") {
        console.error("[ApiClient] DELETE request error", { endpoint, error });
      }
      throw new ApiError(
        error instanceof Error
          ? error.message
          : "An unexpected error occurred. Please try again later.",
      );
    }
  }

  private async makeRequest(
    url: string,
    options: RequestInit,
    timeout?: number,
  ): Promise<Response> {
    const controller = new AbortController();
    const requestTimeout = timeout ?? this.config.requestTimeout;
    const timeoutId = setTimeout(() => controller.abort(), requestTimeout);

    // SSR Cookie handling
    const enhancedOptions = { ...options };
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
        // Try to extract error details from response body
        let errorCode: string | undefined;
        let errorMessage: string | undefined;

        try {
          const errorData = await response.clone().json();
          errorCode = errorData.code;
          // Prefer backend message if available, otherwise use user-friendly fallback
          errorMessage = errorData.message || errorData.error;
        } catch {
          // If JSON parsing fails, we'll use status code-based message
        }

        // Log technical details for developers (development only)
        if (process.env.NODE_ENV === "development") {
          console.error("[ApiClient] API request failed", {
            status: response.status,
            statusText: response.statusText,
            url,
            code: errorCode,
            message: errorMessage,
          });
        }

        // Use backend message if available, otherwise use user-friendly status code message
        const userMessage =
          errorMessage ||
          getUserFriendlyErrorMessage(response.status, response.statusText);

        throw new ApiError(userMessage, response.status, errorCode);
      }

      return response;
    } catch (error) {
      clearTimeout(timeoutId);
      if (error instanceof DOMException && error.name === "AbortError") {
        // Log technical details for developers (development only)
        if (process.env.NODE_ENV === "development") {
          console.error("[ApiClient] Request timeout", {
            url,
            timeout: this.config.requestTimeout,
          });
        }
        throw new ApiError("Request timeout. Please try again later.", 408);
      }
      // If it's already an ApiError, re-throw it
      if (error instanceof ApiError) {
        throw error;
      }
      // For other errors, log and wrap in ApiError
      if (process.env.NODE_ENV === "development") {
        console.error("[ApiClient] Request failed", { url, error });
      }
      throw new ApiError(
        error instanceof Error
          ? error.message
          : "An unexpected error occurred. Please try again later.",
      );
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
