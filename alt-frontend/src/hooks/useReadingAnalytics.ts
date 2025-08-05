import { useState, useEffect } from "react";
import { ReadingAnalytics } from "@/types/analytics";
import { mockAnalytics } from "@/data/mockAnalyticsData";

export const useReadingAnalytics = () => {
  const [analytics, setAnalytics] = useState<ReadingAnalytics | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const fetchAnalytics = async () => {
      try {
        setIsLoading(true);
        // Use mock data for now - can be replaced with real API later
        await new Promise((resolve) => setTimeout(resolve, 500)); // Simulate loading
        setAnalytics(mockAnalytics);
      } catch (err) {
        setError(err as Error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchAnalytics();

    // TODO: SSEリアルタイム更新 - will be implemented when API is ready
    // const eventSource = new EventSource('/api/analytics/sse');
    //
    // eventSource.onmessage = (event) => {
    //   const updatedData = JSON.parse(event.data);
    //   setAnalytics(updatedData);
    // };
    //
    // eventSource.onerror = (error) => {
    //   console.error('SSE connection error:', error);
    // };
    //
    // return () => {
    //   eventSource.close();
    // };
  }, []);

  return { analytics, isLoading, error };
};
