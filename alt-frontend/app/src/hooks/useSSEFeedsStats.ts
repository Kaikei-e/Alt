import { useState, useEffect, useRef } from "react";
import { setupSSEWithReconnect } from "@/lib/apiSse";
import { UnsummarizedFeedStatsSummary } from "@/schema/feedStats";

// Type guard for validating numeric amounts
const isValidAmount = (value: unknown): value is number => {
  return (
    typeof value === "number" && !isNaN(value) && value >= 0 && isFinite(value)
  );
};

export const useSSEFeedsStats = () => {
  const [feedAmount, setFeedAmount] = useState(0);
  const [unsummarizedArticlesAmount, setUnsummarizedArticlesAmount] =
    useState(0);
  const [totalArticlesAmount, setTotalArticlesAmount] = useState(0);
  const [isConnected, setIsConnected] = useState(false);
  const [retryCount, setRetryCount] = useState(0);
  const eventSourceRef = useRef<EventSource | null>(null);
  const lastDataReceivedRef = useRef<number>(Date.now());

  // Connection health check
  useEffect(() => {
    const healthCheck = setInterval(() => {
      const now = Date.now();
      const timeSinceLastData = now - lastDataReceivedRef.current;
      const readyState =
        eventSourceRef.current?.readyState ?? EventSource.CLOSED;

      // Backend sends data every 5s, so 15s timeout gives buffer for network delays
      const isReceivingData = timeSinceLastData < 15000; // 15s timeout (3x backend interval)
      const isConnectionOpen = readyState === EventSource.OPEN;

      // Connection is healthy if open AND receiving data regularly
      const shouldBeConnected = isConnectionOpen && isReceivingData;

      // Only update state if it actually changed to prevent unnecessary re-renders
      setIsConnected((prev) => {
        if (prev !== shouldBeConnected) {
          return shouldBeConnected;
        }
        return prev;
      });
    }, 5000); // Check every 5 seconds to reduce overhead

    return () => clearInterval(healthCheck);
  }, []);

  // SSE connection setup - ONLY runs once
  useEffect(() => {
    let isMounted = true;

    // SSE endpoint configuration
    const apiBaseUrl =
      process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080/api";
    const sseUrl = `${apiBaseUrl}/v1/sse/feeds/stats`;

    // Set initial disconnected state
    setIsConnected(false);
    setRetryCount(0);

    const { eventSource, cleanup } = setupSSEWithReconnect(
      sseUrl,
      (data: UnsummarizedFeedStatsSummary) => {
        if (!isMounted) return;

        try {
          // Handle feed amount
          if (data.feed_amount?.amount !== undefined) {
            const amount = data.feed_amount.amount;
            if (isValidAmount(amount)) {
              setFeedAmount(amount);
            } else {
              setFeedAmount(0);
            }
          }
        } catch (error) {
          console.error("Error handling feed amount:", error);
        }

        try {
          // Handle unsummarized articles
          if (data.unsummarized_feed?.amount !== undefined) {
            const amount = data.unsummarized_feed.amount;
            if (isValidAmount(amount)) {
              setUnsummarizedArticlesAmount(amount);
            } else {
              setUnsummarizedArticlesAmount(0);
            }
          }
        } catch (error) {
          console.error("Error handling unsummarized articles:", error);
        }

        try {
          // Handle total articles
          const totalArticlesAmount = data.total_articles?.amount ?? 0;
          if (isValidAmount(totalArticlesAmount)) {
            setTotalArticlesAmount(totalArticlesAmount);
          } else {
            setTotalArticlesAmount(0);
          }
        } catch (error) {
          console.error("Error handling total articles:", error);
        }

        // Update connection state and reset retry count on successful data
        if (isMounted) {
          const now = Date.now();
          lastDataReceivedRef.current = now;

          setIsConnected((prev) => (prev !== true ? true : prev));
          setRetryCount((prev) => (prev !== 0 ? 0 : prev));
        }
      },
      () => {
        // Handle SSE connection error
        if (isMounted) {
          setIsConnected((prev) => (prev !== false ? false : prev));
          setRetryCount((prev) => prev + 1);
        }
      },
      3, // Max 3 reconnect attempts
      () => {
        // Handle SSE connection opened
        if (isMounted) {
          const now = Date.now();
          lastDataReceivedRef.current = now;
          setIsConnected((prev) => (prev !== true ? true : prev));
          setRetryCount((prev) => (prev !== 0 ? 0 : prev));
        }
      },
    );

    // Update the event source reference for health checks
    eventSourceRef.current = eventSource;

    return () => {
      isMounted = false;
      cleanup();
    };
  }, []); // ✅ EMPTY dependency array - only runs once

  return {
    feedAmount,
    unsummarizedArticlesAmount,
    totalArticlesAmount,
    isConnected,
    retryCount,
  };
};
