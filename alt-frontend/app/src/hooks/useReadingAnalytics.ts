import { useState, useEffect } from 'react';
import { ReadingAnalytics } from '@/types/analytics';

export const useReadingAnalytics = () => {
  const [analytics, setAnalytics] = useState<ReadingAnalytics | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const fetchAnalytics = async () => {
      try {
        setIsLoading(true);
        // 既存のSSE統計APIを活用
        const response = await fetch('/api/analytics/reading-stats');
        const data = await response.json();
        setAnalytics(data);
      } catch (err) {
        setError(err as Error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchAnalytics();

    // SSEリアルタイム更新
    const eventSource = new EventSource('/api/analytics/sse');
    
    eventSource.onmessage = (event) => {
      const updatedData = JSON.parse(event.data);
      setAnalytics(updatedData);
    };

    eventSource.onerror = (error) => {
      console.error('SSE connection error:', error);
    };

    return () => {
      eventSource.close();
    };
  }, []);

  return { analytics, isLoading, error };
};