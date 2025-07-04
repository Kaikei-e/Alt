"use client";

import { useState, useEffect } from "react";
import { feedsApi } from "@/lib/api";
import { getStartOfLocalDayUTC } from "@/lib/utils/time";

export const useTodayUnreadCount = () => {
  const [count, setCount] = useState(0);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchCount = async () => {
      try {
        setIsLoading(true);
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
  }, []);

  return { count, isLoading };
};
