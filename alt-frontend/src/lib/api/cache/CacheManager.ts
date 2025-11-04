export interface CacheEntry<T> {
  data: T;
  timestamp: number;
  ttl: number;
}

export interface CacheConfig {
  maxSize: number;
  defaultTtl: number; // in milliseconds
  cleanupInterval: number; // in milliseconds
}

export const defaultCacheConfig: CacheConfig = {
  maxSize: 100,
  defaultTtl: 5 * 60 * 1000, // 5 minutes
  cleanupInterval: 60 * 1000, // 1 minute
};

export class CacheManager {
  private cache = new Map<string, CacheEntry<any>>();
  private config: CacheConfig;
  private cleanupTimer?: NodeJS.Timeout;

  constructor(config: CacheConfig = defaultCacheConfig) {
    this.config = config;
    this.startCacheCleanup();
  }

  getCacheKey(endpoint: string, method: string = "GET"): string {
    return `${method}:${endpoint}`;
  }

  private isValidCache<T>(entry: CacheEntry<T>): boolean {
    return Date.now() - entry.timestamp < entry.ttl;
  }

  set<T>(key: string, data: T, ttlMinutes: number = this.config.defaultTtl / (60 * 1000)): void {
    // Implement cache size limit
    if (this.cache.size >= this.config.maxSize) {
      this.evictOldestEntry();
    }

    this.cache.set(key, {
      data,
      timestamp: Date.now(),
      ttl: ttlMinutes * 60 * 1000,
    });
  }

  get<T>(key: string): T | null {
    const entry = this.cache.get(key);
    if (entry && this.isValidCache(entry)) {
      return entry.data;
    }
    if (entry) {
      this.cache.delete(key);
    }
    return null;
  }

  has(key: string): boolean {
    const entry = this.cache.get(key);
    if (entry && this.isValidCache(entry)) {
      return true;
    }
    if (entry) {
      this.cache.delete(key);
    }
    return false;
  }

  size(): number {
    return this.cache.size;
  }

  private evictOldestEntry(): void {
    const oldestKey = Array.from(this.cache.keys())[0];
    if (oldestKey) {
      this.cache.delete(oldestKey);
    }
  }

  private startCacheCleanup(): void {
    this.cleanupTimer = setInterval(() => {
      this.cleanupExpiredEntries();
    }, this.config.cleanupInterval);
  }

  private cleanupExpiredEntries(): void {
    for (const [key, entry] of this.cache.entries()) {
      if (!this.isValidCache(entry)) {
        this.cache.delete(key);
      }
    }
  }

  clear(): void {
    this.cache.clear();
  }

  invalidate(): void {
    this.clear();
  }

  destroy(): void {
    if (this.cleanupTimer) {
      clearInterval(this.cleanupTimer);
    }
    this.clear();
  }
}
