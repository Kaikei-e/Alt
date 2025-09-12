"use client";

import { useState, useEffect } from "react";
import { feedsApi } from "@/lib/api";
import { getStartOfLocalDayUTC } from "@/lib/utils/time";
import { useAuth } from "@/contexts/auth-context";

export const useTodayUnreadCount = () => {
  const [count, setCount] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const { isAuthenticated } = useAuth();

  useEffect(() => {
    const fetchCount = async () => {
      try {
        setIsLoading(true);

        // Only fetch if authenticated to prevent 401 retry loops
        if (!isAuthenticated) {
          setCount(0);
          return;
        }

        const since = getStartOfLocalDayUTC().toISOString();
        const result = await feedsApi.getTodayUnreadCount(since);
        setCount(result.count);
      } catch {
        setCount(0);
      } finally {
        setIsLoading(false);
      }
    };
    fetchCount();
  }, [isAuthenticated]);

  return { count, isLoading };
};
