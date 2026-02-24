/**
 * SSRF validator tests
 * Tests for src/lib/server/ssrf-validator.ts
 */

import { describe, expect, it } from "vitest";
import {
  SSRFValidationError,
  validateUrlForSSRF,
} from "../../../src/lib/server/ssrf-validator";

describe("server-only ssrf-validator", () => {
  describe("validateUrlForSSRF", () => {
    it("should allow valid public HTTPS URLs", () => {
      expect(() => {
        validateUrlForSSRF("https://example.com/article");
      }).not.toThrow();
    });

    it("should allow valid public HTTP URLs", () => {
      expect(() => {
        validateUrlForSSRF("http://example.com/article");
      }).not.toThrow();
    });

    it("should block private IP addresses (10.x)", () => {
      expect(() => {
        validateUrlForSSRF("http://10.0.0.1/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block private IP addresses (172.16-31.x)", () => {
      expect(() => {
        validateUrlForSSRF("http://172.16.0.1/article");
      }).toThrow(SSRFValidationError);

      expect(() => {
        validateUrlForSSRF("http://172.31.255.255/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block private IP addresses (192.168.x)", () => {
      expect(() => {
        validateUrlForSSRF("http://192.168.1.1/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block loopback addresses (127.x)", () => {
      expect(() => {
        validateUrlForSSRF("http://127.0.0.1/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block localhost", () => {
      expect(() => {
        validateUrlForSSRF("http://localhost/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block link-local addresses (169.254.x)", () => {
      expect(() => {
        validateUrlForSSRF("http://169.254.169.254/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block metadata endpoints (AWS/GCP)", () => {
      expect(() => {
        validateUrlForSSRF("http://169.254.169.254/latest/meta-data");
      }).toThrow(SSRFValidationError);

      expect(() => {
        validateUrlForSSRF("http://metadata.google.internal/computeMetadata");
      }).toThrow(SSRFValidationError);
    });

    it("should block internal domains (.local)", () => {
      expect(() => {
        validateUrlForSSRF("http://internal.local/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block internal domains (.internal)", () => {
      expect(() => {
        validateUrlForSSRF("http://service.internal/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block internal domains (.corp)", () => {
      expect(() => {
        validateUrlForSSRF("http://api.corp/article");
      }).toThrow(SSRFValidationError);
    });

    it("should block non-HTTP/HTTPS schemes", () => {
      expect(() => {
        validateUrlForSSRF("javascript:alert(1)");
      }).toThrow(SSRFValidationError);

      expect(() => {
        validateUrlForSSRF("file:///etc/passwd");
      }).toThrow(SSRFValidationError);
    });

    it("should block non-standard ports", () => {
      expect(() => {
        validateUrlForSSRF("http://example.com:22/article");
      }).toThrow(SSRFValidationError);

      expect(() => {
        validateUrlForSSRF("http://example.com:3306/article");
      }).toThrow(SSRFValidationError);
    });

    it("should allow standard web ports", () => {
      expect(() => {
        validateUrlForSSRF("http://example.com:80/article");
      }).not.toThrow();

      expect(() => {
        validateUrlForSSRF("https://example.com:443/article");
      }).not.toThrow();

      expect(() => {
        validateUrlForSSRF("http://example.com:8080/article");
      }).not.toThrow();

      expect(() => {
        validateUrlForSSRF("https://example.com:8443/article");
      }).not.toThrow();
    });

    it("should throw SSRFValidationError with proper type", () => {
      try {
        validateUrlForSSRF("http://127.0.0.1/article");
        expect.fail("Should have thrown");
      } catch (error) {
        expect(error).toBeInstanceOf(SSRFValidationError);
        if (error instanceof SSRFValidationError) {
          expect(error.type).toBe("LOOPBACK_BLOCKED");
          expect(error.message).toContain("Loopback");
        }
      }
    });

    it("should reject invalid URL format", () => {
      expect(() => {
        validateUrlForSSRF("not-a-url");
      }).toThrow(SSRFValidationError);
    });

    it("should reject empty URL", () => {
      expect(() => {
        validateUrlForSSRF("");
      }).toThrow(SSRFValidationError);
    });
  });
});
