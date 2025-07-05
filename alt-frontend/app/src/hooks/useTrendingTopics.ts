import { useState, useEffect } from 'react';
import { TrendingTopic } from '@/types/analytics';

export const useTrendingTopics = () => {
  const [topics, setTopics] = useState<TrendingTopic[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const fetchTopics = async () => {
      try {
        setIsLoading(true);
        const response = await fetch('/api/analytics/trending-topics');
        const data = await response.json();
        setTopics(data);
      } catch (err) {
        setError(err as Error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchTopics();

    // リアルタイム更新のためのSSE接続
    const eventSource = new EventSource('/api/analytics/trending-topics-sse');
    
    eventSource.onmessage = (event) => {
      const updatedTopics = JSON.parse(event.data);
      setTopics(updatedTopics);
    };

    eventSource.onerror = (error) => {
      console.error('SSE connection error for trending topics:', error);
    };

    return () => {
      eventSource.close();
    };
  }, []);

  return { topics, isLoading, error };
};