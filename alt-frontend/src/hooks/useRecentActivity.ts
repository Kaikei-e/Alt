"use client";

import { useState, useEffect } from "react";
import { feedsApi } from "@/lib/api";
import { ActivityData } from "@/types/desktop";
import { getRelativeTime } from "@/lib/utils/time";
import { useAuth } from "@/contexts/auth-context";

export interface UseRecentActivityReturn {
  activities: ActivityData[];
  isLoading: boolean;
  error: string | null;
}

export const useRecentActivity = (
  limit: number = 10,
): UseRecentActivityReturn => {
  const [activities, setActivities] = useState<ActivityData[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { isAuthenticated } = useAuth();

  useEffect(() => {
    const fetchActivities = async () => {
      try {
        setIsLoading(true);
        setError(null);

        // Only fetch if authenticated to prevent 401 retry loops
        if (!isAuthenticated) {
          setActivities([]);
          return;
        }

        const activityData = await feedsApi.getRecentActivity(limit);

        // Transform the API response to the expected format
        const transformedActivities: ActivityData[] = activityData.map(
          (activity) => ({
            id: activity.id,
            type: activity.type,
            title: activity.title,
            time: getRelativeTime(activity.timestamp),
          }),
        );

        setActivities(transformedActivities);
      } catch {
        setError("Failed to fetch recent activity");
        setActivities([]);
      } finally {
        setIsLoading(false);
      }
    };

    fetchActivities();
  }, [limit, isAuthenticated]);

  return {
    activities,
    isLoading,
    error,
  };
};
