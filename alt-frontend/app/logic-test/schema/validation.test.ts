import { describe, expect, it } from "vitest";
import * as v from "valibot";
import { searchQuerySchema } from "@/schema/validation/searchQuery";
import { articleSearchQuerySchema } from "@/schema/validation/articleSearchQuery";

describe("Schema Validation", () => {
  describe("searchQuerySchema", () => {
    it("should validate correct search queries", () => {
      const validQueries = [
        { query: "javascript" },
        { query: "react hooks" },
        { query: "a".repeat(100) }, // max length
        { query: "ai" }, // min length
      ];

      validQueries.forEach((query) => {
        const result = v.safeParse(searchQuerySchema, query);
        expect(result.success).toBe(true);
        if (result.success) {
          expect(result.output).toEqual(query);
        }
      });
    });

    it("should trim whitespace from queries", () => {
      const query = { query: "  javascript  " };
      const result = v.safeParse(searchQuerySchema, query);

      expect(result.success).toBe(true);
      if (result.success) {
        expect(result.output.query).toBe("javascript");
      }
    });

    it("should reject empty queries", () => {
      const invalidQueries = [
        { query: "" },
        { query: "   " }, // only whitespace
      ];

      invalidQueries.forEach((query) => {
        const result = v.safeParse(searchQuerySchema, query);
        expect(result.success).toBe(false);
        if (!result.success) {
          expect(result.issues[0].message).toBe("Please enter a search query");
        }
      });
    });

    it("should reject queries that are too short", () => {
      const query = { query: "a" };
      const result = v.safeParse(searchQuerySchema, query);

      expect(result.success).toBe(false);
      if (!result.success) {
        expect(result.issues[0].message).toBe(
          "Search query must be at least 2 characters",
        );
      }
    });

    it("should reject queries that are too long", () => {
      const query = { query: "a".repeat(101) };
      const result = v.safeParse(searchQuerySchema, query);

      expect(result.success).toBe(false);
      if (!result.success) {
        expect(result.issues[0].message).toBe(
          "Search query must be at most 100 characters",
        );
      }
    });

    it("should reject non-string queries", () => {
      const invalidQueries = [
        { query: 123 },
        { query: null },
        { query: undefined },
        { query: {} },
        { query: [] },
      ];

      invalidQueries.forEach((query) => {
        const result = v.safeParse(searchQuerySchema, query as any);
        expect(result.success).toBe(false);
        if (!result.success) {
          expect(result.issues[0].message).toBe("Please enter a search query");
        }
      });
    });

    it("should handle special characters in queries", () => {
      const specialQueries = [
        { query: "C++" },
        { query: "React.js" },
        { query: "Node@latest" },
        { query: "API & SDK" },
        { query: "GraphQL/REST" },
        { query: "test-driven-development" },
        { query: "Hello, World!" },
        { query: "émojis 🚀" },
      ];

      specialQueries.forEach((query) => {
        const result = v.safeParse(searchQuerySchema, query);
        expect(result.success).toBe(true);
        if (result.success) {
          expect(result.output).toEqual(query);
        }
      });
    });
  });

  describe("articleSearchQuerySchema", () => {
    it("should validate correct article search queries", () => {
      const validQueries = [
        { query: "machine learning" },
        { query: "web development" },
        { query: "a".repeat(100) },
        { query: "ai" },
      ];

      validQueries.forEach((query) => {
        const result = v.safeParse(articleSearchQuerySchema, query);
        expect(result.success).toBe(true);
        if (result.success) {
          expect(result.output).toEqual(query);
        }
      });
    });

    it("should trim whitespace from article queries", () => {
      const query = { query: "  react tutorial  " };
      const result = v.safeParse(articleSearchQuerySchema, query);

      expect(result.success).toBe(true);
      if (result.success) {
        expect(result.output.query).toBe("react tutorial");
      }
    });

    it("should reject empty article queries", () => {
      const invalidQueries = [{ query: "" }, { query: "   " }];

      invalidQueries.forEach((query) => {
        const result = v.safeParse(articleSearchQuerySchema, query);
        expect(result.success).toBe(false);
        if (!result.success) {
          expect(result.issues[0].message).toBe("Please enter a search query");
        }
      });
    });

    it("should reject article queries that are too short", () => {
      const query = { query: "x" };
      const result = v.safeParse(articleSearchQuerySchema, query);

      expect(result.success).toBe(false);
      if (!result.success) {
        expect(result.issues[0].message).toBe(
          "Search query must be at least 2 characters",
        );
      }
    });

    it("should reject article queries that are too long", () => {
      const query = { query: "x".repeat(101) };
      const result = v.safeParse(articleSearchQuerySchema, query);

      expect(result.success).toBe(false);
      if (!result.success) {
        expect(result.issues[0].message).toBe(
          "Search query must be at most 100 characters",
        );
      }
    });
  });

  describe("Edge cases and error handling", () => {
    it("should handle missing query field", () => {
      const invalidData = {};
      const result = v.safeParse(searchQuerySchema, invalidData);

      expect(result.success).toBe(false);
      if (!result.success) {
        expect(result.issues).toHaveLength(1);
      }
    });

    it("should handle multiple validation errors", () => {
      const invalidData = { query: 123, extra: "field" };
      const result = v.safeParse(searchQuerySchema, invalidData as any);

      expect(result.success).toBe(false);
    });

    it("should provide detailed error information", () => {
      const query = { query: "" };
      const result = v.safeParse(searchQuerySchema, query);

      expect(result.success).toBe(false);
      if (!result.success) {
        expect(result.issues[0]).toMatchObject({
          message: expect.any(String),
          path: expect.any(Array),
        });
      }
    });

    it("should validate boundary conditions", () => {
      // Exactly 2 characters (minimum)
      const minQuery = { query: "ab" };
      const minResult = v.safeParse(searchQuerySchema, minQuery);
      expect(minResult.success).toBe(true);

      // Exactly 100 characters (maximum)
      const maxQuery = { query: "a".repeat(100) };
      const maxResult = v.safeParse(searchQuerySchema, maxQuery);
      expect(maxResult.success).toBe(true);

      // 1 character (below minimum)
      const belowMinQuery = { query: "a" };
      const belowMinResult = v.safeParse(searchQuerySchema, belowMinQuery);
      expect(belowMinResult.success).toBe(false);

      // 101 characters (above maximum)
      const aboveMaxQuery = { query: "a".repeat(101) };
      const aboveMaxResult = v.safeParse(searchQuerySchema, aboveMaxQuery);
      expect(aboveMaxResult.success).toBe(false);
    });

    it("should handle Unicode characters correctly", () => {
      const unicodeQueries = [
        { query: "javascript 🚀" },
        { query: "café programming" },
        { query: "ñoño development" },
        { query: "中文编程" },
        { query: "🔥 hot topics 🔥" },
      ];

      unicodeQueries.forEach((query) => {
        const result = v.safeParse(searchQuerySchema, query);
        expect(result.success).toBe(true);
        if (result.success) {
          expect(result.output).toEqual(query);
        }
      });
    });
  });
});
