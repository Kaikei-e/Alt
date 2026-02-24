import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { User } from "@/types/auth";
import { AuthAPIClient } from "../../../../src/lib/api/auth-client";

// Mock @ory/client
const mockToSession = vi.hoisted(() => vi.fn());
vi.mock("@/lib/ory/client", () => {
  return {
    oryClient: {
      toSession: mockToSession,
    },
  };
});

interface UserPreferences {
  theme?: "light" | "dark" | "system";
  language?: string;
  [key: string]: any;
}

type MockHeaders = {
  entries: () => IterableIterator<[string, string]>;
};

interface MockFetchResponse<T = unknown> {
  ok: boolean;
  status: number;
  statusText: string;
  json: () => Promise<T>;
  headers: MockHeaders;
}

const toIterator = <T>(input: T[]): IterableIterator<T> =>
  input[Symbol.iterator]() as IterableIterator<T>;

const createMockHeaders = (data: Record<string, string> = {}): MockHeaders => ({
  entries: () => toIterator(Object.entries(data)),
});

const createMockResponse = <T = unknown>(
  overrides: Partial<MockFetchResponse<T>> = {},
): MockFetchResponse<T> => ({
  ok: overrides.ok ?? true,
  status: overrides.status ?? 200,
  statusText: overrides.statusText ?? "OK",
  json: overrides.json ?? (() => Promise.resolve({} as T)),
  headers: overrides.headers ?? createMockHeaders(),
});

const originalLocation = window.location;

const restoreWindowLocation = () => {
  Object.defineProperty(window, "location", {
    configurable: true,
    enumerable: true,
    value: originalLocation,
    writable: true,
  });
};

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Mock console methods to avoid noise in tests
const consoleSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

describe("AuthAPIClient", () => {
  let client: AuthAPIClient;

  beforeEach(() => {
    client = new AuthAPIClient();
    mockFetch.mockReset();
    consoleSpy.mockClear();
    consoleErrorSpy.mockClear();
    restoreWindowLocation();
  });

  afterEach(() => {
    mockFetch.mockReset();
    restoreWindowLocation();
  });

  describe("constructor", () => {
    it("should initialize with default base URL", () => {
      const client = new AuthAPIClient();
      expect(client).toBeInstanceOf(AuthAPIClient);
    });

    it("should use environment variable for base URL if available", () => {
      const originalEnv = process.env.NEXT_PUBLIC_AUTH_SERVICE_URL;
      process.env.NEXT_PUBLIC_AUTH_SERVICE_URL = "http://custom-auth:8080";

      const client = new AuthAPIClient();
      expect(client).toBeInstanceOf(AuthAPIClient);

      // Restore original env
      process.env.NEXT_PUBLIC_AUTH_SERVICE_URL = originalEnv;
    });
  });

  describe("initiateLogin", () => {
    it("should throw redirect error (current implementation)", async () => {
      // Mock window.location for test environment
      Object.defineProperty(window, "location", {
        value: { href: "" },
        writable: true,
      });

      await expect(client.initiateLogin()).rejects.toThrow(
        "Login flow initiated via redirect",
      );
    });
  });

  describe("completeLogin", () => {
    it("should throw redirect error (current implementation)", async () => {
      // Mock window.location for test environment
      Object.defineProperty(window, "location", {
        value: { href: "" },
        writable: true,
      });

      await expect(
        client.completeLogin("flow-123", "test@example.com", "password123"),
      ).rejects.toThrow("Login redirected to Kratos");
    });
  });

  describe("initiateRegistration", () => {
    it("should throw redirect error (current implementation)", async () => {
      // Mock window.location for test environment
      Object.defineProperty(window, "location", {
        value: { href: "" },
        writable: true,
      });

      await expect(client.initiateRegistration()).rejects.toThrow(
        "Registration flow initiated via redirect",
      );
    });
  });

  describe("completeRegistration", () => {
    it("should throw redirect error (current implementation)", async () => {
      // Mock window.location for test environment
      Object.defineProperty(window, "location", {
        value: { href: "" },
        writable: true,
      });

      await expect(
        client.completeRegistration(
          "flow-456",
          "newuser@example.com",
          "password123",
          "New User",
        ),
      ).rejects.toThrow("Registration redirected to Kratos");
    });
  });

  describe("logout", () => {
    it("should make POST request to logout endpoint", async () => {
      // Mock CSRF token request first
      mockFetch
        .mockResolvedValueOnce(
          createMockResponse({
            json: () =>
              Promise.resolve({ data: { csrf_token: "csrf-token-123" } }),
          }),
        )
        // Mock actual logout request
        .mockResolvedValueOnce(
          createMockResponse({
            json: () => Promise.resolve({}),
          }),
        );

      await client.logout();

      expect(mockFetch).toHaveBeenCalledTimes(2);
      expect(
        (client as unknown as { sessionHeaders: unknown }).sessionHeaders,
      ).toBeNull();
    });
  });

  describe("getCSRFToken", () => {
    it("should make POST request and return CSRF token", async () => {
      const mockCSRFToken = "csrf-token-123";

      mockFetch.mockResolvedValueOnce(
        createMockResponse({
          json: () => Promise.resolve({ data: { csrf_token: mockCSRFToken } }),
        }),
      );

      const result = await client.getCSRFToken();

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/auth\/csrf$/),
        expect.objectContaining({
          method: "POST",
          credentials: "include",
        }),
      );
      expect(result).toBe(mockCSRFToken);
    });

    it("should return null and log error on server failure", async () => {
      mockFetch.mockResolvedValueOnce(
        createMockResponse({
          ok: false,
          status: 500,
          statusText: "Internal Server Error",
          headers: createMockHeaders(),
        }),
      );

      const result = await client.getCSRFToken();

      expect(result).toBeNull();
      // Check for console.error since the actual implementation uses console.error for server failures
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        "🚨 CSRF token request failed:",
        expect.objectContaining({
          status: 500,
          statusText: "Internal Server Error",
        }),
      );
    });
  });

  describe("updateProfile", () => {
    it("should make PUT request with profile data and return updated User", async () => {
      const mockUser: User = {
        id: "user-123",
        tenantId: "tenant-456",
        email: "test@example.com",
        name: "Updated Name",
        role: "user",
        createdAt: "2025-01-15T10:00:00Z",
      };

      // Mock CSRF token request first
      mockFetch
        .mockResolvedValueOnce(
          createMockResponse({
            json: () =>
              Promise.resolve({ data: { csrf_token: "csrf-token-123" } }),
          }),
        )
        // Mock actual profile update request
        .mockResolvedValueOnce(
          createMockResponse({
            json: () => Promise.resolve({ data: mockUser }),
          }),
        );

      const profileUpdate = { name: "Updated Name" };
      const result = await client.updateProfile(profileUpdate);

      expect(result).toEqual(mockUser);
    });
  });

  describe("getUserSettings", () => {
    it("should make GET request and return user settings", async () => {
      const mockSettings = { theme: "dark", language: "en" };

      mockFetch.mockResolvedValueOnce(
        createMockResponse({
          json: () => Promise.resolve({ data: mockSettings }),
        }),
      );

      const result = await client.getUserSettings();

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/auth\/settings$/),
        expect.objectContaining({
          method: "GET",
          credentials: "include",
        }),
      );
      expect(result).toEqual(mockSettings);
    });
  });

  describe("updateUserSettings", () => {
    it("should make PUT request with settings data", async () => {
      // Mock CSRF token request first
      mockFetch
        .mockResolvedValueOnce(
          createMockResponse({
            json: () =>
              Promise.resolve({ data: { csrf_token: "csrf-token-123" } }),
          }),
        )
        // Mock actual settings update request
        .mockResolvedValueOnce(
          createMockResponse({
            json: () => Promise.resolve({}),
          }),
        );

      const settings: UserPreferences = { theme: "light", language: "ja" };
      await client.updateUserSettings(settings);

      expect(mockFetch).toHaveBeenCalledTimes(2);
    });
  });

  describe("CSRF token integration", () => {
    it("should add CSRF token to unsafe HTTP methods", async () => {
      const mockCSRFToken = "csrf-token-123";

      // Mock CSRF token request
      mockFetch
        .mockResolvedValueOnce(
          createMockResponse({
            json: () =>
              Promise.resolve({ data: { csrf_token: mockCSRFToken } }),
          }),
        )
        // Mock actual request
        .mockResolvedValueOnce(
          createMockResponse({
            json: () => Promise.resolve({}),
          }),
        );

      await client.logout();

      // Should have made CSRF token request first
      expect(mockFetch).toHaveBeenNthCalledWith(
        1,
        expect.stringMatching(/\/api\/auth\/csrf$/),
        expect.objectContaining({ method: "POST" }),
      );

      // Should have made logout request with CSRF token
      expect(mockFetch).toHaveBeenNthCalledWith(
        2,
        expect.stringMatching(/\/api\/auth\/logout$/),
        expect.objectContaining({
          headers: expect.objectContaining({
            "X-CSRF-Token": mockCSRFToken,
          }),
        }),
      );
    });

    it("should proceed without CSRF token if retrieval fails", async () => {
      // Mock CSRF token failure
      mockFetch
        .mockResolvedValueOnce(
          createMockResponse({
            ok: false,
            status: 500,
            statusText: "Internal Server Error",
          }),
        )
        // Mock actual request
        .mockResolvedValueOnce(
          createMockResponse({
            json: () => Promise.resolve({}),
          }),
        );

      await client.logout();

      // Should still make the logout request without CSRF token
      expect(mockFetch).toHaveBeenNthCalledWith(
        2,
        expect.stringMatching(/\/api\/auth\/logout$/),
        expect.objectContaining({
          method: "POST",
        }),
      );
    });
  });
});
