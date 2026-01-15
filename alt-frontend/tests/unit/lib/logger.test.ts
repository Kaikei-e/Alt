/**
 * Tests for structured logger (ADR 98/99 compliance)
 */

import { describe, it, expect, beforeEach, afterEach, vi, type MockInstance } from "vitest";
import { Logger } from "@/lib/logger";

describe("Logger", () => {
  let infoSpy: MockInstance;
  let errorSpy: MockInstance;
  let warnSpy: MockInstance;

  beforeEach(() => {
    // In browser environment (JSDOM), logger uses console.info for info level
    infoSpy = vi.spyOn(console, "info").mockImplementation(() => {});
    errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    vi.spyOn(console, "log").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("basic logging", () => {
    it("should log info messages", () => {
      const logger = new Logger();
      logger.info("test message");

      expect(infoSpy).toHaveBeenCalledTimes(1);
    });

    it("should log error messages", () => {
      const logger = new Logger();
      logger.error("error message");

      expect(errorSpy).toHaveBeenCalledTimes(1);
    });

    it("should log warn messages", () => {
      const logger = new Logger();
      logger.warn("warning message");

      expect(warnSpy).toHaveBeenCalledTimes(1);
    });
  });

  describe("business context (ADR 98/99)", () => {
    it("should include alt.feed.id when set", () => {
      const logger = new Logger().withFeedId("feed-123");
      logger.info("test");

      // In browser mode, second argument is the full entry object
      expect(infoSpy).toHaveBeenCalled();
      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["alt.feed.id"]).toBe("feed-123");
    });

    it("should include alt.article.id when set", () => {
      const logger = new Logger().withArticleId("article-456");
      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["alt.article.id"]).toBe("article-456");
    });

    it("should include alt.job.id when set", () => {
      const logger = new Logger().withJobId("job-789");
      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["alt.job.id"]).toBe("job-789");
    });

    it("should include alt.processing.stage when set", () => {
      const logger = new Logger().withProcessingStage("rendering");
      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["alt.processing.stage"]).toBe("rendering");
    });

    it("should include alt.ai.pipeline when set", () => {
      const logger = new Logger().withAIPipeline("alt-frontend");
      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["alt.ai.pipeline"]).toBe("alt-frontend");
    });

    it("should chain multiple context values", () => {
      const logger = new Logger()
        .withFeedId("feed-123")
        .withArticleId("article-456")
        .withProcessingStage("loading");

      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["alt.feed.id"]).toBe("feed-123");
      expect(entry["alt.article.id"]).toBe("article-456");
      expect(entry["alt.processing.stage"]).toBe("loading");
    });
  });

  describe("exception logging", () => {
    it("should include error details", () => {
      const logger = new Logger();
      const error = new Error("test error");

      logger.exception("operation failed", error);

      const entry = errorSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["error_name"]).toBe("Error");
      expect(entry["error_message"]).toBe("test error");
      expect(entry["error_stack"]).toBeDefined();
    });
  });

  describe("duration logging", () => {
    it("should include operation and duration_ms", () => {
      const logger = new Logger();
      logger.logDuration("fetch_articles", 150);

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["operation"]).toBe("fetch_articles");
      expect(entry["duration_ms"]).toBe(150);
    });
  });

  describe("log entry format", () => {
    it("should include timestamp", () => {
      const logger = new Logger();
      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["timestamp"]).toBeDefined();
      // ISO timestamp format
      expect(typeof entry["timestamp"]).toBe("string");
    });

    it("should include level", () => {
      const logger = new Logger();
      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["level"]).toBe("info");
    });

    it("should include message", () => {
      const logger = new Logger();
      logger.info("my message");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["message"]).toBe("my message");
    });

    it("should include service name", () => {
      const logger = new Logger();
      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["service"]).toBe("alt-frontend");
    });

    it("should include environment as browser in JSDOM", () => {
      const logger = new Logger();
      logger.info("test");

      const entry = infoSpy.mock.calls[0][1] as Record<string, unknown>;
      expect(entry["environment"]).toBe("browser");
    });
  });
});
