import { useState, useEffect } from "react";
import { TrendingTopic } from "@/types/analytics";
import { mockTrendingTopics } from "@/data/mockAnalyticsData";

export const useTrendingTopics = () => {
  const [topics, setTopics] = useState<TrendingTopic[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const fetchTopics = async () => {
      try {
        setIsLoading(true);
        // Use mock data for now - can be replaced with real API later
        await new Promise((resolve) => setTimeout(resolve, 300)); // Simulate loading
        setTopics(mockTrendingTopics);
      } catch (err) {
        setError(err as Error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchTopics();

    // TODO: リアルタイム更新のためのSSE接続 - will be implemented when API is ready
    // const eventSource = new EventSource('/api/analytics/trending-topics-sse');
    //
    // eventSource.onmessage = (event) => {
    //   const updatedTopics = JSON.parse(event.data);
    //   setTopics(updatedTopics);
    // };
    //
    // eventSource.onerror = (error) => {
    //   console.error('SSE connection error for trending topics:', error);
    // };
    //
    // return () => {
    //   eventSource.close();
    // };
  }, []);

  return { topics, isLoading, error };
};
