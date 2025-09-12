import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { CacheManager } from "../../../../../src/lib/api/cache/CacheManager";

describe("CacheManager", () => {
  let cacheManager: CacheManager;

  beforeEach(() => {
    vi.useFakeTimers();
    cacheManager = new CacheManager();
  });

  afterEach(() => {
    vi.useRealTimers();
    cacheManager.destroy();
  });

  describe("cache key generation", () => {
    it("should generate cache key with default GET method", () => {
      const key = cacheManager.getCacheKey("/api/test");
      expect(key).toBe("GET:/api/test");
    });

    it("should generate cache key with custom method", () => {
      const key = cacheManager.getCacheKey("/api/test", "POST");
      expect(key).toBe("POST:/api/test");
    });
  });

  describe("cache operations", () => {
    it("should set and get cached data", () => {
      const testData = { test: "data" };
      cacheManager.set("test-key", testData, 5);

      const retrieved = cacheManager.get("test-key");
      expect(retrieved).toEqual(testData);
    });

    it("should return null for non-existent key", () => {
      const retrieved = cacheManager.get("non-existent");
      expect(retrieved).toBeNull();
    });

    it("should return null for expired data", () => {
      const testData = { test: "data" };
      cacheManager.set("test-key", testData, 0.001); // Very short TTL

      vi.advanceTimersByTime(100); // Advance time beyond TTL

      const retrieved = cacheManager.get("test-key");
      expect(retrieved).toBeNull();
    });

    it("should remove expired entries automatically", () => {
      const testData = { test: "data" };
      cacheManager.set("test-key", testData, 0.001);

      vi.advanceTimersByTime(100);
      cacheManager.get("test-key"); // This should trigger cleanup

      expect(cacheManager.has("test-key")).toBe(false);
    });
  });

  describe("cache size management", () => {
    it("should evict oldest entry when max size reached", () => {
      const maxSize = 2;
      const smallCacheManager = new CacheManager({
        maxSize,
        defaultTtl: 60000,
        cleanupInterval: 60000,
      });

      smallCacheManager.set("key1", "data1", 5);
      smallCacheManager.set("key2", "data2", 5);
      smallCacheManager.set("key3", "data3", 5); // Should evict key1

      expect(smallCacheManager.has("key1")).toBe(false);
      expect(smallCacheManager.has("key2")).toBe(true);
      expect(smallCacheManager.has("key3")).toBe(true);

      smallCacheManager.destroy();
    });
  });

  describe("cache invalidation", () => {
    it("should clear all cache entries", () => {
      cacheManager.set("key1", "data1", 5);
      cacheManager.set("key2", "data2", 5);

      cacheManager.clear();

      expect(cacheManager.has("key1")).toBe(false);
      expect(cacheManager.has("key2")).toBe(false);
    });

    it("should invalidate cache (alias for clear)", () => {
      cacheManager.set("key1", "data1", 5);

      cacheManager.invalidate();

      expect(cacheManager.has("key1")).toBe(false);
    });
  });

  describe("cleanup mechanism", () => {
    it("should run periodic cleanup", () => {
      const cleanupInterval = 5000;
      const cleanupManager = new CacheManager({
        maxSize: 100,
        defaultTtl: 60000,
        cleanupInterval,
      });

      cleanupManager.set("key1", "data1", 0.001); // Very short TTL

      vi.advanceTimersByTime(100); // Expire the entry
      vi.advanceTimersByTime(cleanupInterval); // Trigger cleanup

      expect(cleanupManager.has("key1")).toBe(false);

      cleanupManager.destroy();
    });

    it("should stop cleanup timer on destroy", () => {
      const clearIntervalSpy = vi.spyOn(global, "clearInterval");

      cacheManager.destroy();

      expect(clearIntervalSpy).toHaveBeenCalled();
    });
  });

  describe("cache statistics", () => {
    it("should report cache size", () => {
      cacheManager.set("key1", "data1", 5);
      cacheManager.set("key2", "data2", 5);

      expect(cacheManager.size()).toBe(2);
    });

    it("should check if key exists", () => {
      cacheManager.set("key1", "data1", 5);

      expect(cacheManager.has("key1")).toBe(true);
      expect(cacheManager.has("key2")).toBe(false);
    });
  });

  describe("configuration", () => {
    it("should use default configuration", () => {
      const manager = new CacheManager();

      expect(manager.size()).toBe(0);

      manager.destroy();
    });

    it("should use custom configuration", () => {
      const config = {
        maxSize: 10,
        defaultTtl: 30000,
        cleanupInterval: 10000,
      };

      const manager = new CacheManager(config);

      expect(manager.size()).toBe(0);

      manager.destroy();
    });
  });
});
