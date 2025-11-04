"use client";

import { useCallback, useRef } from "react";
import { feedsApi } from "@/lib/api";
import type { Feed } from "@/schema/feed";

const MAX_CACHE_SIZE = 5;
const PREFETCH_DELAY = 500; // ms
const DISMISSED_CLEANUP_DELAY = 3000; // ms - time to keep dismissed articles in exclusion list

export interface UseArticleContentPrefetchResult {
  triggerPrefetch: () => void;
  getCachedContent: (feedUrl: string) => string | null;
  markAsDismissed: (feedUrl: string) => void;
  contentCacheRef: React.MutableRefObject<Map<string, string | "loading">>;
}

/**
 * Hook for prefetching article content in the background
 *
 * @param feeds - Array of feed items
 * @param activeIndex - Current active feed index
 * @param prefetchAhead - Number of articles to prefetch ahead (default: 2)
 * @returns Methods to trigger prefetch and get cached content
 */
export const useArticleContentPrefetch = (
  feeds: Feed[],
  activeIndex: number,
  prefetchAhead: number = 2
): UseArticleContentPrefetchResult => {
  // Cache for prefetched article content
  const contentCacheRef = useRef<Map<string, string | "loading">>(new Map());
  const prefetchTimeoutsRef = useRef<NodeJS.Timeout[]>([]);

  // Track recently dismissed articles to prevent prefetch race condition
  const dismissedArticlesRef = useRef<Set<string>>(new Set());
  const dismissalTimeoutsRef = useRef<Map<string, NodeJS.Timeout>>(new Map());

  /**
   * Prefetch content for a single article
   */
  const prefetchContent = useCallback(async (feed: Feed) => {
    const feedUrl = feed.link;

    // Skip if article is being dismissed (race condition prevention)
    if (dismissedArticlesRef.current.has(feedUrl)) {
      console.log(`[useArticleContentPrefetch] Skipping dismissed article: ${feedUrl}`);
      return;
    }

    // Skip if already in cache or being prefetched
    if (contentCacheRef.current.has(feedUrl)) {
      return;
    }

    try {
      // Mark as loading to prevent duplicate requests
      contentCacheRef.current.set(feedUrl, "loading");

      // Fetch full article content
      const response = await feedsApi.getFeedContentOnTheFly({
        feed_url: feedUrl,
      });

      // Store content in cache
      if (response.content) {
        contentCacheRef.current.set(feedUrl, response.content);

        // Archive article in background (non-blocking)
        feedsApi.archiveContent(feedUrl, feed.title).catch((err) => {
          console.warn(`[useArticleContentPrefetch] Failed to archive article: ${feedUrl}`, err);
        });
      } else {
        // Remove from cache if no content
        contentCacheRef.current.delete(feedUrl);
      }

      // Clean up old cache entries if size exceeds limit
      if (contentCacheRef.current.size > MAX_CACHE_SIZE) {
        const entries = Array.from(contentCacheRef.current.keys());
        const oldestKey = entries[0];
        contentCacheRef.current.delete(oldestKey);
      }
    } catch (error) {
      // Remove failed prefetch from cache
      contentCacheRef.current.delete(feedUrl);
      console.warn(`[useArticleContentPrefetch] Failed to prefetch content: ${feedUrl}`, error);
    }
  }, []);

  /**
   * Trigger prefetch for next N articles
   */
  const triggerPrefetch = useCallback(() => {
    // Clear any pending timeouts
    prefetchTimeoutsRef.current.forEach((timeout) => clearTimeout(timeout));
    prefetchTimeoutsRef.current = [];

    // Prefetch next N articles with staggered delays
    for (let i = 1; i <= prefetchAhead; i++) {
      const nextFeed = feeds[activeIndex + i];
      if (nextFeed) {
        const timeout = setTimeout(
          () => {
            prefetchContent(nextFeed);
          },
          PREFETCH_DELAY * i // Stagger requests
        );
        prefetchTimeoutsRef.current.push(timeout);
      }
    }
  }, [feeds, activeIndex, prefetchAhead, prefetchContent]);

  /**
   * Get cached content for a feed URL
   * Returns null if not cached or still loading
   */
  const getCachedContent = useCallback((feedUrl: string): string | null => {
    const cached = contentCacheRef.current.get(feedUrl);
    return cached === "loading" ? null : cached || null;
  }, []);

  /**
   * Mark an article as dismissed to prevent prefetch race conditions
   * The article will be excluded from prefetch for DISMISSED_CLEANUP_DELAY ms
   */
  const markAsDismissed = useCallback((feedUrl: string): void => {
    // Add to dismissed set
    dismissedArticlesRef.current.add(feedUrl);

    // Clear any existing timeout for this URL
    const existingTimeout = dismissalTimeoutsRef.current.get(feedUrl);
    if (existingTimeout) {
      clearTimeout(existingTimeout);
    }

    // Schedule cleanup after delay
    const timeout = setTimeout(() => {
      dismissedArticlesRef.current.delete(feedUrl);
      dismissalTimeoutsRef.current.delete(feedUrl);
    }, DISMISSED_CLEANUP_DELAY);

    dismissalTimeoutsRef.current.set(feedUrl, timeout);
  }, []);

  return {
    triggerPrefetch,
    getCachedContent,
    markAsDismissed,
    contentCacheRef,
  };
};
