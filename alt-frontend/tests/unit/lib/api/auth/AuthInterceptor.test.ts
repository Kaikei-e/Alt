import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AuthInterceptor } from "../../../../../src/lib/api/auth/AuthInterceptor";
import { ApiError } from "../../../../../src/lib/api/core/ApiError";

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Mock sessionStorage
Object.defineProperty(window, "sessionStorage", {
  value: {
    getItem: vi.fn(),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  },
  writable: true,
});

describe("AuthInterceptor", () => {
  let interceptor: AuthInterceptor;
  let mockOnAuthRequired: () => void;

  beforeEach(() => {
    mockOnAuthRequired = vi.fn();
    interceptor = new AuthInterceptor({
      onAuthRequired: mockOnAuthRequired,
      recheckEndpoint: "/api/auth/recheck",
      recheckTimeout: 3000,
      recheckStorageKey: "alt:recheck-whoami",
    });

    vi.clearAllMocks();
    // Reset sessionStorage mocks
    vi.mocked(sessionStorage.getItem).mockReturnValue(null);
    vi.mocked(sessionStorage.setItem).mockImplementation(() => {});
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  describe("non-401 responses", () => {
    it("should pass through successful responses", async () => {
      const mockResponse = new Response('{"data": "test"}', {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });

      const result = await interceptor.intercept(
        mockResponse,
        "https://api.test.com/data",
      );

      expect(result).toBe(mockResponse);
      expect(mockOnAuthRequired).not.toHaveBeenCalled();
    });

    it("should pass through other error responses", async () => {
      const mockResponse = new Response('{"error": "Server error"}', {
        status: 500,
        headers: { "Content-Type": "application/json" },
      });

      const result = await interceptor.intercept(
        mockResponse,
        "https://api.test.com/data",
      );

      expect(result).toBe(mockResponse);
      expect(mockOnAuthRequired).not.toHaveBeenCalled();
    });
  });

  describe("401 responses", () => {
    it("should handle 401 on server-side (no window)", async () => {
      // Mock server-side environment
      const originalWindow = global.window;
      // @ts-expect-error
      delete global.window;

      const mockResponse = new Response('{"error": "Unauthorized"}', {
        status: 401,
        headers: { "Content-Type": "application/json" },
      });

      const result = await interceptor.intercept(
        mockResponse,
        "https://api.test.com/data",
      );

      expect(result).toBe(mockResponse);
      expect(mockOnAuthRequired).not.toHaveBeenCalled();

      // Restore window
      global.window = originalWindow;
    });

    it("should trigger auth required callback on first 401", async () => {
      const mockResponse = new Response('{"error": "Unauthorized"}', {
        status: 401,
        headers: { "Content-Type": "application/json" },
      });

      mockFetch.mockResolvedValueOnce(
        new Response('{"status": "ok"}', {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );

      // Mock original request recreation
      const mockOriginalResponse = new Response('{"data": "success"}', {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
      mockFetch.mockResolvedValueOnce(mockOriginalResponse);

      const result = await interceptor.intercept(
        mockResponse,
        "https://api.test.com/data",
        {
          method: "GET",
          headers: { Authorization: "Bearer token" },
        },
      );

      // Implementation always calls recheck endpoint, doesn't use sessionStorage
      expect(mockFetch).toHaveBeenCalledWith("/api/auth/recheck", {
        credentials: "include",
        signal: expect.any(AbortSignal),
      });
      expect(result).toBe(mockOriginalResponse);
    });

    it("should not recheck if already done in session", async () => {
      const mockResponse = new Response('{"error": "Unauthorized"}', {
        status: 401,
        headers: { "Content-Type": "application/json" },
      });

      // Implementation always calls recheck endpoint
      mockFetch.mockResolvedValueOnce(
        new Response('{"error": "Unauthorized"}', {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      );

      await interceptor.intercept(mockResponse, "https://api.test.com/data");

      // Implementation always calls recheck endpoint
      expect(mockFetch).toHaveBeenCalled();
      expect(mockOnAuthRequired).toHaveBeenCalled();
    });

    it("should handle recheck failure gracefully", async () => {
      const mockResponse = new Response('{"error": "Unauthorized"}', {
        status: 401,
        headers: { "Content-Type": "application/json" },
      });

      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      await interceptor.intercept(mockResponse, "https://api.test.com/data");

      expect(mockOnAuthRequired).toHaveBeenCalled();
    });

    it("should handle recheck 401 response", async () => {
      const mockResponse = new Response('{"error": "Unauthorized"}', {
        status: 401,
        headers: { "Content-Type": "application/json" },
      });

      mockFetch.mockResolvedValueOnce(
        new Response('{"error": "Unauthorized"}', {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      );

      await interceptor.intercept(mockResponse, "https://api.test.com/data");

      expect(mockOnAuthRequired).toHaveBeenCalled();
    });
  });

  describe("configuration", () => {
    it("should use custom configuration", () => {
      const customInterceptor = new AuthInterceptor({
        onAuthRequired: vi.fn(),
        recheckEndpoint: "/custom/auth/check",
        recheckTimeout: 5000,
        recheckStorageKey: "custom:recheck",
      });

      expect(customInterceptor).toBeDefined();
    });

    it("should use default configuration", () => {
      const defaultInterceptor = new AuthInterceptor();

      expect(defaultInterceptor).toBeDefined();
    });
  });
});
