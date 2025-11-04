import { describe, expect, test } from "vitest";
import { securityHeaders } from "@/config/security";

describe("Security Headers", () => {
  test("should include Content-Security-Policy header", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["Content-Security-Policy"]).toBeDefined();
    expect(headers["Content-Security-Policy"]).toContain("default-src 'self'");
  });

  test("should include X-Frame-Options header", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["X-Frame-Options"]).toBe("DENY");
  });

  test("should include X-Content-Type-Options header", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["X-Content-Type-Options"]).toBe("nosniff");
  });

  test("should include Referrer-Policy header", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["Referrer-Policy"]).toBe("strict-origin-when-cross-origin");
  });

  test("should include Strict-Transport-Security header", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["Strict-Transport-Security"]).toBe(
      "max-age=31536000; includeSubDomains; preload"
    );
  });

  test("should include X-XSS-Protection header set to 0", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["X-XSS-Protection"]).toBe("0");
  });

  test("should include Permissions-Policy header", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["Permissions-Policy"]).toBe(
      "camera=(), microphone=(), geolocation=(), payment=()"
    );
  });

  test("should include Cross-Origin-Opener-Policy header", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["Cross-Origin-Opener-Policy"]).toBe("same-origin");
  });

  test("should include Cross-Origin-Embedder-Policy header", () => {
    const headers = securityHeaders("test-nonce");
    expect(headers["Cross-Origin-Embedder-Policy"]).toBe("unsafe-none");
  });
});
