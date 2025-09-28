/**
 * @vitest-environment node
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";
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

      const response = await middleware(request);

      expect(response.status).toBe(200);
    });
  });

  describe("unauthenticated access with guard cookie", () => {
    it("should still require login even with redirect guard cookie", async () => {
      const request = new NextRequest("https://curionoah.com/desktop/home");
      request.cookies.set("alt_auth_redirect_guard", "1");

      const response = await middleware(request);

      expect(response.status).toBe(303);
      const location = response.headers.get("location");
      expect(location).toContain("/auth/login");
    });
  });

  describe("unauthenticated access without guard cookie", () => {
    it("should redirect to app login with return_to parameter", async () => {
      const request = new NextRequest("https://curionoah.com/desktop/home?test=123");

      const response = await middleware(request);

      expect(response.status).toBe(303);

      const location = response.headers.get("location");
      expect(location).toContain("/auth/login");
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
      expect(location).toContain("/auth/login");
      expect(location).toContain(
        "return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Ffeeds%3Fcategory%3Dtech%26page%3D2",
      );
    });
  });

  describe("edge cases", () => {
    it("should handle empty search parameters", async () => {
      const request = new NextRequest("https://curionoah.com/desktop/home?");

      const response = await middleware(request);

      const location = response.headers.get("location");
      expect(location).toContain("/auth/login");
      expect(location).toContain(
        "return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fhome",
      );
    });

    it("should handle paths without search parameters", async () => {
      const request = new NextRequest("https://curionoah.com/desktop/settings");

      const response = await middleware(request);

      const location = response.headers.get("location");
      expect(location).toContain("/auth/login");
      expect(location).toContain(
        "return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fsettings",
      );
    });
  });
});
