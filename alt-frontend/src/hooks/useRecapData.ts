import { useCallback, useEffect, useState } from "react";
import { recapApi } from "@/lib/api";
import type { RecapSummary } from "@/schema/recap";

export const useRecapData = () => {
  const [data, setData] = useState<RecapSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [isInitialLoading, setIsInitialLoading] = useState(true);

  const fetchData = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const recap = await recapApi.get7DaysRecap();
      setData(recap);
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

