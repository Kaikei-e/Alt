"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/contexts/auth-context";
import { feedApi } from "@/lib/api";
import type { FeedStatsSummary } from "@/schema/feedStats";
import { useTodayUnreadCount } from "./useTodayUnreadCount";

export interface UseHomeStatsReturn {
  // 基本統計
  feedStats: FeedStatsSummary | null;
  isLoadingStats: boolean;
  statsError: string | null;

  // 未読記事数
  unreadCount: number;

  // 追加統計（計算値）
  extraStats: {
    weeklyReads: number;
    aiProcessed: number;
    bookmarks: number;
  };

  // リフレッシュ関数
  refreshStats: () => Promise<void>;
}

export const useHomeStats = (): UseHomeStatsReturn => {
  const [feedStats, setFeedStats] = useState<FeedStatsSummary | null>(null);
  const [isLoadingStats, setIsLoadingStats] = useState(true);
  const [statsError, setStatsError] = useState<string | null>(null);
  const { isAuthenticated } = useAuth();

  const { count: unreadCount } = useTodayUnreadCount();

  const fetchStats = useCallback(async () => {
    try {
      setIsLoadingStats(true);
      setStatsError(null);

      // Only fetch if authenticated to prevent 401 retry loops
      if (!isAuthenticated) {
        setFeedStats(null);
        return;
      }

      const stats = await feedApi.getFeedStats();
      setFeedStats(stats);
    } catch {
      setStatsError("Failed to fetch feed stats");
      setFeedStats(null);
    } finally {
      setIsLoadingStats(false);
    }
  }, [isAuthenticated]);

  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  const refreshStats = useCallback(async () => {
    await fetchStats();
  }, [fetchStats]);

  // 計算された追加統計
  const extraStats = {
    weeklyReads: 45, // Mock calculation - will be replaced with real logic
    aiProcessed: feedStats?.summarized_feed?.amount || 0,
    bookmarks: 12, // Mock calculation - will be replaced with real logic
  };

  return {
    feedStats,
    isLoadingStats,
    statsError,
    unreadCount,
    extraStats,
    refreshStats,
  };
};
