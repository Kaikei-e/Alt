import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { ApiClient } from "./ApiClient";
import { CacheManager } from "../cache/CacheManager";
import { AuthInterceptor } from "../auth/AuthInterceptor";
import { ApiError } from "./ApiError";

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Mock sessionStorage
Object.defineProperty(window, 'sessionStorage', {
  value: {
    getItem: vi.fn(),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  },
  writable: true
});

describe("ApiClient", () => {
  let apiClient: ApiClient;
  let mockCacheManager: CacheManager;
  let mockAuthInterceptor: AuthInterceptor;

  beforeEach(() => {
    mockCacheManager = new CacheManager();
    mockAuthInterceptor = new AuthInterceptor();
    apiClient = new ApiClient({
      baseUrl: "https://api.test.com",
      requestTimeout: 5000
    }, mockCacheManager, mockAuthInterceptor);

    vi.clearAllMocks();
    mockFetch.mockClear();
  });

  afterEach(() => {
    apiClient.destroy();
    mockCacheManager.destroy();
  });

  describe("GET requests", () => {
    it("should make successful GET request", async () => {
      const responseData = { data: "test" };
      mockFetch.mockResolvedValueOnce(new Response(JSON.stringify(responseData), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      }));

      const result = await apiClient.get("/test");

      expect(mockFetch).toHaveBeenCalledWith(
        "https://api.test.com/test",
        expect.objectContaining({
          method: "GET",
          headers: expect.objectContaining({
            "Content-Type": "application/json"
          }),
          credentials: "include"
        })
      );
      expect(result).toEqual(responseData);
    });

    it("should use cached data on subsequent requests", async () => {
      const responseData = { data: "test" };
      mockFetch.mockResolvedValueOnce(new Response(JSON.stringify(responseData), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      }));

      // First request
      await apiClient.get("/test");
      // Second request (should use cache)
      const result = await apiClient.get("/test");

      expect(mockFetch).toHaveBeenCalledTimes(1);
      expect(result).toEqual(responseData);
    });

    it("should handle request timeout", async () => {
      const timeoutClient = new ApiClient({
        baseUrl: "https://api.test.com",
        requestTimeout: 100 // Very short timeout
      }, mockCacheManager, mockAuthInterceptor);
      
      // Mock fetch to throw AbortError after timeout
      mockFetch.mockImplementation(() => 
        new Promise((_, reject) => 
          setTimeout(() => {
            const error = new DOMException("Operation was aborted", "AbortError");
            reject(error);
          }, 150)
        )
      );

      await expect(timeoutClient.get("/test")).rejects.toThrow("Request timeout");
      
      timeoutClient.destroy();
    }, 1000);

    it("should handle network errors", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      await expect(apiClient.get("/test")).rejects.toThrow("Network error");
    });

    it("should handle HTTP error responses", async () => {
      mockFetch.mockResolvedValueOnce(new Response("Not Found", {
        status: 404,
        statusText: "Not Found"
      }));

      await expect(apiClient.get("/test")).rejects.toThrow(ApiError);
    });
  });

  describe("POST requests", () => {
    it("should make successful POST request", async () => {
      const requestData = { name: "test" };
      const responseData = { id: 1, name: "test" };
      mockFetch.mockResolvedValueOnce(new Response(JSON.stringify(responseData), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      }));

      const result = await apiClient.post("/test", requestData);

      expect(mockFetch).toHaveBeenCalledWith(
        "https://api.test.com/test",
        expect.objectContaining({
          method: "POST",
          headers: expect.objectContaining({
            "Content-Type": "application/json"
          }),
          body: JSON.stringify(requestData),
          credentials: "include"
        })
      );
      expect(result).toEqual(responseData);
    });

    it("should invalidate cache after POST", async () => {
      const spy = vi.spyOn(mockCacheManager, 'invalidate');
      
      mockFetch.mockResolvedValueOnce(new Response('{"success": true}', {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      }));

      await apiClient.post("/test", { data: "test" });

      expect(spy).toHaveBeenCalled();
    });

    it("should handle API error responses", async () => {
      mockFetch.mockResolvedValueOnce(new Response('{"error": "Validation failed"}', {
        status: 400,
        headers: { 'Content-Type': 'application/json' }
      }));

      await expect(apiClient.post("/test", {})).rejects.toThrow(ApiError);
    });
  });

  describe("authentication handling", () => {
    it("should handle 401 responses through auth interceptor", async () => {
      const mockIntercept = vi.spyOn(mockAuthInterceptor, 'intercept');
      mockIntercept.mockResolvedValueOnce(new Response('{"authRequired": true}', {
        status: 401,
        headers: { 'Content-Type': 'application/json' }
      }));

      mockFetch.mockResolvedValueOnce(new Response('{"error": "Unauthorized"}', {
        status: 401,
        headers: { 'Content-Type': 'application/json' }
      }));

      const result = await apiClient.get("/protected");

      expect(mockIntercept).toHaveBeenCalled();
      expect(result).toEqual({ authRequired: true });
    });
  });

  describe("request deduplication", () => {
    it("should deduplicate identical concurrent GET requests", async () => {
      const responseData = { data: "test" };
      mockFetch.mockResolvedValue(new Response(JSON.stringify(responseData), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      }));

      // Make two concurrent requests
      const [result1, result2] = await Promise.all([
        apiClient.get("/test"),
        apiClient.get("/test")
      ]);

      expect(mockFetch).toHaveBeenCalledTimes(1);
      expect(result1).toEqual(responseData);
      expect(result2).toEqual(responseData);
    });
  });

  describe("SSR cookie handling", () => {
    it("should handle server-side cookie forwarding", async () => {
      // Mock server-side environment
      const originalWindow = global.window;
      // @ts-ignore
      delete global.window;

      // Mock Next.js cookies
      const mockCookies = vi.fn().mockResolvedValue({
        toString: () => 'session=abc123; token=def456'
      });
      
      vi.doMock('next/headers', () => ({
        cookies: mockCookies
      }));

      mockFetch.mockResolvedValueOnce(new Response('{"data": "test"}', {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      }));

      await apiClient.get("/test");

      expect(mockFetch).toHaveBeenCalledWith(
        "https://api.test.com/test",
        expect.objectContaining({
          headers: expect.objectContaining({
            cookie: 'session=abc123; token=def456'
          })
        })
      );

      // Restore window
      global.window = originalWindow;
    });
  });

  describe("cleanup", () => {
    it("should clear cache and pending requests on destroy", () => {
      const cacheSpy = vi.spyOn(mockCacheManager, 'clear');
      
      apiClient.destroy();

      expect(cacheSpy).toHaveBeenCalled();
    });
  });
});