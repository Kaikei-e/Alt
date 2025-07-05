import { useState, useEffect } from 'react';
import { SourceAnalytic } from '@/types/analytics';

export const useSourceAnalytics = () => {
  const [sources, setSources] = useState<SourceAnalytic[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const fetchSources = async () => {
      try {
        setIsLoading(true);
        const response = await fetch('/api/analytics/source-analytics');
        const data = await response.json();
        setSources(data);
      } catch (err) {
        setError(err as Error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchSources();

    // リアルタイム更新のためのSSE接続
    const eventSource = new EventSource('/api/analytics/source-analytics-sse');
    
    eventSource.onmessage = (event) => {
      const updatedSources = JSON.parse(event.data);
      setSources(updatedSources);
    };

    eventSource.onerror = (error) => {
      console.error('SSE connection error for source analytics:', error);
    };

    return () => {
      eventSource.close();
    };
  }, []);

  return { sources, isLoading, error };
};