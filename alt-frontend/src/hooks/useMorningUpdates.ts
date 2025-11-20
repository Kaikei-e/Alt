import { useCallback, useEffect, useState } from "react";
import { morningApi } from "@/lib/api";
import type { MorningUpdate } from "@/schema/morning";

export const useMorningUpdates = () => {
  const [data, setData] = useState<MorningUpdate[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [isInitialLoading, setIsInitialLoading] = useState(true);

  const fetchData = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const updates = await morningApi.getOvernightUpdates();
      setData(updates);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Unknown error"));
    } finally {
      setIsLoading(false);
      setIsInitialLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const retry = useCallback(async () => {
    await fetchData();
  }, [fetchData]);

  return {
    data,
    isLoading,
    isInitialLoading,
    error,
    retry,
  };
};

