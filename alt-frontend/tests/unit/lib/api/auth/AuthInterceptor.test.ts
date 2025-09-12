import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
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
  let mockOnAuthRequired: ReturnType<typeof vi.fn>;

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
      // @ts-ignore
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

      expect(vi.mocked(sessionStorage.setItem)).toHaveBeenCalledWith(
        "alt:recheck-whoami",
        "1",
      );
      expect(mockFetch).toHaveBeenCalledWith("/api/auth/recheck", {
        credentials: "include",
        signal: expect.any(AbortSignal),
      });
      expect(result).toBe(mockOriginalResponse);
    });

    it("should not recheck if already done in session", async () => {
      vi.mocked(sessionStorage.getItem).mockReturnValue("1");

      const mockResponse = new Response('{"error": "Unauthorized"}', {
        status: 401,
        headers: { "Content-Type": "application/json" },
      });

      await interceptor.intercept(mockResponse, "https://api.test.com/data");

      expect(mockFetch).not.toHaveBeenCalled();
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
