import { useEffect, useState } from "react";
import { mockSourceAnalytics } from "@/data/mockAnalyticsData";
import type { SourceAnalytic } from "@/types/analytics";

export const useSourceAnalytics = () => {
  const [sources, setSources] = useState<SourceAnalytic[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const fetchSources = async () => {
      try {
        setIsLoading(true);
        // Use mock data for now - can be replaced with real API later
        await new Promise((resolve) => setTimeout(resolve, 400)); // Simulate loading
        setSources(mockSourceAnalytics);
      } catch (err) {
        setError(err as Error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchSources();

    // TODO: リアルタイム更新のためのSSE接続 - will be implemented when API is ready
    // const eventSource = new EventSource('/api/analytics/source-analytics-sse');
    //
    // eventSource.onmessage = (event) => {
    //   const updatedSources = JSON.parse(event.data);
    //   setSources(updatedSources);
    // };
    //
    // eventSource.onerror = (error) => {
    //   console.error('SSE connection error for source analytics:', error);
    // };
    //
    // return () => {
    //   eventSource.close();
    // };
  }, []);

  return { sources, isLoading, error };
};
