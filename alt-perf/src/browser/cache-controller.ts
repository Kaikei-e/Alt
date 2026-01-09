/**
 * Browser cache control
 * Uses Chrome DevTools Protocol for cache management
 */
import type { Page } from "@astral/astral";
import { debug, info, warn } from "../utils/logger.ts";

/**
 * Cache configuration options
 */
export interface CacheConfig {
  /** Disable browser cache */
  disableCache: boolean;
  /** Clear cache before navigation */
  clearBefore: boolean;
  /** Clear cache after navigation */
  clearAfter: boolean;
}

/**
 * Default cache configuration
 */
export const DEFAULT_CACHE_CONFIG: CacheConfig = {
  disableCache: false,
  clearBefore: false,
  clearAfter: false,
};

/**
 * Cache controller for managing browser cache via CDP
 */
export class CacheController {
  private isCacheDisabled = false;

  /**
   * Disable browser cache
   * This prevents the browser from using cached resources
   */
  async disableCache(page: Page): Promise<void> {
    debug("Disabling browser cache");

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        // Enable Network domain first
        await cdpSession.send("Network.enable", {});
        // Disable cache
        await cdpSession.send("Network.setCacheDisabled", {
          cacheDisabled: true,
        });

        this.isCacheDisabled = true;
        info("Browser cache disabled");
      } else {
        warn("CDP session not available, cache control skipped");
      }
    } catch (error) {
      warn("Failed to disable cache", { error: String(error) });
      throw error;
    }
  }

  /**
   * Enable browser cache
   */
  async enableCache(page: Page): Promise<void> {
    debug("Enabling browser cache");

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        await cdpSession.send("Network.setCacheDisabled", {
          cacheDisabled: false,
        });

        this.isCacheDisabled = false;
        info("Browser cache enabled");
      }
    } catch (error) {
      warn("Failed to enable cache", { error: String(error) });
    }
  }

  /**
   * Clear browser cache
   */
  async clearCache(page: Page): Promise<void> {
    debug("Clearing browser cache");

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        await cdpSession.send("Network.clearBrowserCache", {});
        info("Browser cache cleared");
      } else {
        warn("CDP session not available, cache clear skipped");
      }
    } catch (error) {
      warn("Failed to clear cache", { error: String(error) });
    }
  }

  /**
   * Clear browser cookies
   */
  async clearCookies(page: Page): Promise<void> {
    debug("Clearing browser cookies");

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        await cdpSession.send("Network.clearBrowserCookies", {});
        info("Browser cookies cleared");
      } else {
        warn("CDP session not available, cookie clear skipped");
      }
    } catch (error) {
      warn("Failed to clear cookies", { error: String(error) });
    }
  }

  /**
   * Clear all browser storage (cache, cookies, localStorage, etc.)
   */
  async clearAll(page: Page): Promise<void> {
    debug("Clearing all browser storage");

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        // Clear cache and cookies
        await this.clearCache(page);
        await this.clearCookies(page);

        // Clear storage via page context
        await page.evaluate(() => {
          localStorage.clear();
          sessionStorage.clear();
        });

        info("All browser storage cleared");
      }
    } catch (error) {
      warn("Failed to clear all storage", { error: String(error) });
    }
  }

  /**
   * Apply cache configuration
   */
  async applyConfig(page: Page, config: Partial<CacheConfig>): Promise<void> {
    const mergedConfig = { ...DEFAULT_CACHE_CONFIG, ...config };

    if (mergedConfig.clearBefore) {
      await this.clearAll(page);
    }

    if (mergedConfig.disableCache) {
      await this.disableCache(page);
    }
  }

  /**
   * Run cleanup after operation
   */
  async cleanup(page: Page, config: Partial<CacheConfig>): Promise<void> {
    const mergedConfig = { ...DEFAULT_CACHE_CONFIG, ...config };

    if (mergedConfig.clearAfter) {
      await this.clearAll(page);
    }
  }

  /**
   * Check if cache is currently disabled
   */
  isCacheCurrentlyDisabled(): boolean {
    return this.isCacheDisabled;
  }

  /**
   * Get CDP session from page (internal helper)
   */
  private async getCDPSession(page: Page): Promise<CDPSession | null> {
    try {
      const session = await (
        page as unknown as { unsafelyGetCDPSession(): Promise<CDPSession> }
      ).unsafelyGetCDPSession?.();
      return session || null;
    } catch {
      return null;
    }
  }
}

/**
 * CDP session interface (minimal)
 */
interface CDPSession {
  send(method: string, params?: Record<string, unknown>): Promise<unknown>;
}

/**
 * Create a cache controller instance
 */
export function createCacheController(): CacheController {
  return new CacheController();
}
