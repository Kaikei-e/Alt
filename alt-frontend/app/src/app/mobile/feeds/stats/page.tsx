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
  return typeof value === 'number' && !isNaN(value) && value >= 0 && isFinite(value);
};

export default function FeedsStatsPage() {
  const [feedAmount, setFeedAmount] = useState(0);
  const [unsummarizedArticlesAmount, setUnsummarizedArticlesAmount] = useState(0);
  const [totalArticlesAmount, setTotalArticlesAmount] = useState(0);
  const [isConnected, setIsConnected] = useState(false);
  const [lastDataReceived, setLastDataReceived] = useState<number>(Date.now());
  const [retryCount, setRetryCount] = useState(0);
  const eventSourceRef = useRef<EventSource | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);

  // Progress tracking for SSE updates (5-second cycle)
  const { progress, reset: resetProgress } = useSSEProgress(5000);

  // Connection health check
  useEffect(() => {
    const healthCheck = setInterval(() => {
      const timeSinceLastData = Date.now() - lastDataReceived;
      const readyState = eventSourceRef.current?.readyState ?? EventSource.CLOSED;

      // Consider connected if we're receiving data regularly
      const isReceivingData = timeSinceLastData < 15000; // 15s timeout
      const isConnectionOpen = readyState === EventSource.OPEN;

      setIsConnected(isReceivingData && isConnectionOpen);
    }, 1000); // Check every second

    return () => clearInterval(healthCheck);
  }, [lastDataReceived]);

  useEffect(() => {
    let isMounted = true; // Race condition prevention

    // SSE endpoint configuration
    const apiBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL || 'http://localhost:8080/api';
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
          console.error('Error handling feed amount:', error);
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
          console.error('Error handling unsummarized articles:', error);
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
          console.error('Error handling total articles:', error);
        }

        // Update connection state and reset retry count on successful data
        if (isMounted) {
          const now = Date.now();
          setLastDataReceived(now);
          setIsConnected(true);
          setRetryCount(0);
          resetProgress(); // Reset progress bar on new data
        }
      },
      () => {
        // Handle SSE connection error with retry tracking
        if (isMounted) {
          setIsConnected(false);
          setRetryCount(prev => {
            const newCount = prev + 1;
            console.log(`SSE connection error, retry count: ${newCount}`);
            return newCount;
          });
        }
      },
      3, // Max 3 reconnect attempts
      () => {
        // Handle SSE connection opened - update last data received time
        if (isMounted) {
          const now = Date.now();
          setLastDataReceived(now);
          setIsConnected(true);
          setRetryCount(0);
        }
      }
    );

    // Update the event source reference for health checks
    eventSourceRef.current = eventSource;
    cleanupRef.current = cleanup;

    return () => {
      isMounted = false; // Prevent race conditions
      cleanup();
    };
  }, [resetProgress]);

  return (
    <Box
      minH="100vh"
      minHeight="100dvh"
      background="var(--vaporwave-bg)"
      position="relative"
      pt="env(safe-area-inset-top)"
      pb="env(safe-area-inset-bottom)"
    >
      {/* SSE Progress Bar */}
      <SSEProgressBar
        progress={progress}
        isVisible={isConnected}
        onComplete={resetProgress}
      />

      <Box p={5} maxW="container.sm" mx="auto" pt={8}>
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
              color={isConnected ? "var(--vaporwave-green)" : retryCount > 0 ? "var(--vaporwave-yellow)" : "var(--vaporwave-magenta)"}
              textShadow={isConnected ? "0 0 8px var(--vaporwave-green)" : retryCount > 0 ? "0 0 8px var(--vaporwave-yellow)" : "0 0 8px var(--vaporwave-magenta)"}
            >
              {isConnected ? "Connected" : retryCount > 0 ? `Reconnecting (${retryCount}/3)` : "Disconnected"}
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
