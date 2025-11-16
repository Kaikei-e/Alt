/**
 * @vitest-environment node
 */

import { NextRequest } from "next/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { middleware } from "../../src/middleware";

// Mock environment variables
process.env.NEXT_PUBLIC_APP_ORIGIN = "https://curionoah.com";
process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL = "https://curionoah.com/ory";

describe("middleware", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("public paths", () => {
    it("should allow access to root path", async () => {
      const request = new NextRequest("https://curionoah.com/");
      const response = await middleware(request);

      expect(response.status).toBe(200);
    });

    it("should allow access to Kratos proxy paths", async () => {
      const request = new NextRequest(
        "https://curionoah.com/ory/self-service/registration/browser",
      );
      const response = await middleware(request);

      expect(response.status).toBe(200);
    });

    it("should allow access to auth paths", async () => {
      const request = new NextRequest("https://curionoah.com/auth/login");
      const response = await middleware(request);

      expect(response.status).toBe(200);
    });

    it("should allow access to api paths", async () => {
      const request = new NextRequest("https://curionoah.com/api/backend/test");
      const response = await middleware(request);

      expect(response.status).toBe(200);
    });

    it("should NOT allow unauthenticated access to /api/debug/** paths", async () => {
      const request = new NextRequest(
        "https://curionoah.com/api/debug/cookies",
      );
      const response = await middleware(request);

      // Should redirect to landing page (303) when unauthenticated
      expect(response.status).toBe(303);
      const location = response.headers.get("location");
      expect(location).toContain("/public/landing");
      expect(location).toContain("return_to=");
    });

    it("should allow access to _next paths", async () => {
      const request = new NextRequest(
        "https://curionoah.com/_next/static/test.js",
      );
      const response = await middleware(request);

      expect(response.status).toBe(200);
    });

    it("should allow access to static files", async () => {
      const request = new NextRequest("https://curionoah.com/favicon.ico");
      const response = await middleware(request);

      expect(response.status).toBe(200);
    });
  });

  describe("authenticated access", () => {
    it("should allow access when ory_kratos_session cookie exists", async () => {
      const request = new NextRequest("https://curionoah.com/desktop/home");
      request.cookies.set("ory_kratos_session", "test-session-value");

      // Mock auth-hub session validation
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
      });

      const response = await middleware(request);

      expect(response.status).toBe(200);
    });

    it("should allow authenticated access to /api/debug/cookies", async () => {
      const request = new NextRequest(
        "https://curionoah.com/api/debug/cookies",
      );
      request.cookies.set("ory_kratos_session", "test-session-value");

      // Mock auth-hub session validation
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
      });

      const response = await middleware(request);

      expect(response.status).toBe(200);
    });
  });

  describe("unauthenticated access", () => {
    it("should redirect to Kratos when no session cookie exists", async () => {
      const request = new NextRequest("https://curionoah.com/desktop/home");

      const response = await middleware(request);

      expect(response.status).toBe(303);
      const location = response.headers.get("location");
      expect(location).toContain("/ory/self-service/login/browser");
    });
  });

  describe("unauthenticated access without guard cookie", () => {
    it("should redirect to Kratos login flow with return_to parameter", async () => {
      const request = new NextRequest(
        "https://curionoah.com/desktop/home?test=123",
      );

      const response = await middleware(request);

      expect(response.status).toBe(303);

      const location = response.headers.get("location");
      expect(location).toContain("/ory/self-service/login/browser");
      expect(location).toContain(
        "return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fhome%3Ftest%3D123",
      );
    });

    it("should handle paths with search parameters correctly", async () => {
      const request = new NextRequest(
        "https://curionoah.com/desktop/feeds?category=tech&page=2",
      );

      const response = await middleware(request);

      const location = response.headers.get("location");
      expect(location).toContain("/ory/self-service/login/browser");
      expect(location).toContain(
        "return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Ffeeds%3Fcategory%3Dtech%26page%3D2",
      );
    });

    it("should not redirect when coming from login flow (prevent redirect loop)", async () => {
      const request = new NextRequest("https://curionoah.com/home");
      request.headers.set(
        "referer",
        "https://curionoah.com/auth/login?flow=abc123",
      );

      const response = await middleware(request);

      expect(response.status).toBe(200);
    });

    it("should not redirect when coming from Kratos (prevent redirect loop)", async () => {
      const request = new NextRequest("https://curionoah.com/home");
      request.headers.set(
        "referer",
        "https://curionoah.com/ory/self-service/login/browser",
      );

      const response = await middleware(request);

      expect(response.status).toBe(200);
    });
  });

  describe("edge cases", () => {
    it("should handle empty search parameters", async () => {
      const request = new NextRequest("https://curionoah.com/desktop/home?");

      const response = await middleware(request);

      const location = response.headers.get("location");
      expect(location).toContain("/ory/self-service/login/browser");
      expect(location).toContain(
        "return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fhome",
      );
    });

    it("should handle paths without search parameters", async () => {
      const request = new NextRequest("https://curionoah.com/desktop/settings");

      const response = await middleware(request);

      const location = response.headers.get("location");
      expect(location).toContain("/ory/self-service/login/browser");
      expect(location).toContain(
        "return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fsettings",
      );
    });
  });
});
