"use client";

import React from "react";
import { UnsummarizedFeedStatsSummary } from "@/schema/feedStats";
import { setupSSEWithReconnect } from "@/lib/apiSse";
import { Flex, Text, Box } from "@chakra-ui/react";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { useEffect, useState, useRef } from "react";
import { useSSEProgress } from "@/hooks/useSSEProgress";
import { FiRss, FiFileText, FiLayers } from "react-icons/fi";
import { SSEProgressBar } from "@/components/mobile/stats/SSEProgressBar";
import { StatCard } from "@/components/mobile/stats/StatCard";

// Type guard for validating numeric amounts
const isValidAmount = (value: unknown): value is number => {
  return (
    typeof value === "number" && !isNaN(value) && value >= 0 && isFinite(value)
  );
};

export default function FeedsStatsPage() {
  const [feedAmount, setFeedAmount] = useState(0);
  const [unsummarizedArticlesAmount, setUnsummarizedArticlesAmount] =
    useState(0);
  const [totalArticlesAmount, setTotalArticlesAmount] = useState(0);
  const [isConnected, setIsConnected] = useState(false);
  const [retryCount, setRetryCount] = useState(0);
  const eventSourceRef = useRef<EventSource | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);
  const lastDataReceivedRef = useRef<number>(Date.now());

  // Progress tracking for SSE updates (5-second cycle)
  const { progress, reset: resetProgress } = useSSEProgress(5000);

  // Connection health check using ref to avoid re-creating interval
  useEffect(() => {
    const healthCheck = setInterval(() => {
      const now = Date.now();
      const timeSinceLastData = now - lastDataReceivedRef.current;
      const readyState =
        eventSourceRef.current?.readyState ?? EventSource.CLOSED;

      // Consider connected based on connection state and recent data
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
  }, []); // No dependencies needed since we use ref

  useEffect(() => {
    let isMounted = true; // Race condition prevention

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
        if (!isMounted) return; // Prevent updates after unmount

        try {
          // Handle feed amount with validation
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
          // Handle unsummarized articles with validation
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
          // Handle total articles with validation
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
          // Only update connection state if it's actually changed
          setIsConnected((prev) => {
            if (prev !== true) {
              return true;
            }
            return prev;
          });
          setRetryCount((prev) => {
            if (prev !== 0) {
              return 0;
            }
            return prev;
          });
          resetProgress(); // Reset progress bar on new data
        }
      },
      () => {
        // Handle SSE connection error with retry tracking
        if (isMounted) {
          setIsConnected((prev) => (prev !== false ? false : prev));
          setRetryCount((prev) => {
            const newCount = prev + 1;
            return newCount;
          });
        }
      },
      3, // Max 3 reconnect attempts
      () => {
        // Handle SSE connection opened - update last data received time
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
    cleanupRef.current = cleanup;

    return () => {
      isMounted = false; // Prevent race conditions
      cleanup();
    };
  }, []); // 🔧 FIX: Remove resetProgress from dependencies to prevent infinite SSE reconnections // Only resetProgress dependency needed

  return (
    <Box
      minH="100vh"
      minHeight="100dvh"
      background="var(--vaporwave-bg)"
      position="relative"
      overflowX="hidden"
      pt="env(safe-area-inset-top)"
      pb="env(safe-area-inset-bottom)"
    >
      {/* SSE Progress Bar */}
      <SSEProgressBar
        progress={progress}
        isVisible={isConnected}
        onComplete={resetProgress}
      />

      <Box p={5} maxW="container.sm" mx="auto" pt={8} overflowX="hidden">
        {/* Header */}
        <Box mb={8} textAlign="center">
          <Text
            fontSize="2xl"
            fontWeight="bold"
            color="var(--vaporwave-cyan)"
            textShadow="0 0 20px var(--vaporwave-cyan)"
            mb={2}
          >
            Feeds Statistics
          </Text>

          {/* Connection Status */}
          <Flex align="center" justify="center" gap={2}>
            <Box
              w={2}
              h={2}
              borderRadius="full"
              bg={isConnected ? "#4caf50" : "#e53935"}
              transition="background-color 0.3s ease"
            />
            <Text
              fontSize="sm"
              color={
                isConnected
                  ? "var(--vaporwave-green)"
                  : retryCount > 0
                    ? "var(--vaporwave-yellow)"
                    : "var(--vaporwave-magenta)"
              }
              textShadow={
                isConnected
                  ? "0 0 8px var(--vaporwave-green)"
                  : retryCount > 0
                    ? "0 0 8px var(--vaporwave-yellow)"
                    : "0 0 8px var(--vaporwave-magenta)"
              }
            >
              {isConnected
                ? "Connected"
                : retryCount > 0
                  ? `Reconnecting (${retryCount}/3)`
                  : "Disconnected"}
            </Text>
          </Flex>
        </Box>

        {/* Statistics Cards */}
        <Flex direction="column" gap={6}>
          <StatCard
            label="TOTAL FEEDS"
            value={feedAmount}
            description="RSS feeds being monitored"
            icon={FiRss}
          />

          <StatCard
            label="TOTAL ARTICLES"
            value={totalArticlesAmount}
            description="All articles across RSS feeds"
            icon={FiFileText}
          />

          <StatCard
            label="UNSUMMARIZED ARTICLES"
            value={unsummarizedArticlesAmount}
            description="Articles waiting for AI summarization"
            icon={FiLayers}
          />
        </Flex>
      </Box>

      <FloatingMenu />
    </Box>
  );
}
