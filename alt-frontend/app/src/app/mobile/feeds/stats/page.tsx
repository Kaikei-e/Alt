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

      // Consider connected if EventSource is in OPEN state
      // Server sends heartbeat every 10s and data every 5s, so allow 20s timeout
      // But during first 15 seconds after page load, be more lenient for initial connection
      const isInitialConnection = Date.now() - lastDataReceived <= 15000;
      const dataTimeout = isInitialConnection ? 15000 : 20000; // 15s initially, then 20s

      const shouldBeConnected = readyState === EventSource.OPEN && timeSinceLastData < dataTimeout;

      setIsConnected(shouldBeConnected);
    }, 1000); // Check every second

    return () => clearInterval(healthCheck);
  }, [lastDataReceived]);

  useEffect(() => {
    let isMounted = true; // Race condition prevention

    const { eventSource, cleanup } = setupSSEWithReconnect(
      `${process.env.NEXT_PUBLIC_API_BASE_URL || 'http://localhost:8080'}/v1/sse/feeds/stats`,
      (data: UnsummarizedFeedStatsSummary) => {
        if (!isMounted) return; // Prevent updates after unmount

        try {
          // Handle feed amount with validation
          if (data.feed_amount?.amount !== undefined) {
            const amount = data.feed_amount.amount;
            if (isValidAmount(amount)) {
              setFeedAmount(amount);
            } else {
              console.warn('Invalid feed_amount:', amount);
              setFeedAmount(0);
            }
          }
        } catch (error) {
          console.error('Error updating feed amount:', error);
        }

        try {
          // Handle unsummarized articles with validation
          if (data.unsummarized_feed?.amount !== undefined) {
            const amount = data.unsummarized_feed.amount;
            if (isValidAmount(amount)) {
              setUnsummarizedArticlesAmount(amount);
            } else {
              console.warn('Invalid unsummarized_feed amount:', amount);
              setUnsummarizedArticlesAmount(0);
            }
          }
        } catch (error) {
          console.error('Error updating unsummarized articles:', error);
        }

        try {
          // Handle total articles with validation
          const totalArticlesAmount = data.total_articles?.amount ?? 0;
          if (isValidAmount(totalArticlesAmount)) {
            setTotalArticlesAmount(totalArticlesAmount);
          } else {
            console.warn('Invalid total_articles amount:', totalArticlesAmount);
            setTotalArticlesAmount(0);
          }
        } catch (error) {
          console.error('Error updating total articles:', error);
        }

        // Update connection state and reset retry count on successful data
        if (isMounted) {
          setLastDataReceived(Date.now());
          setIsConnected(true);
          setRetryCount(0);
          resetProgress(); // Reset progress bar on new data
        }
      },
      () => {
        // Handle SSE connection error with retry tracking
        if (isMounted) {
          setIsConnected(false);
          setRetryCount(prev => prev + 1);
        }
      },
      3, // Max 3 reconnect attempts
      () => {
        // Handle SSE connection opened - update last data received time
        if (isMounted) {
          setLastDataReceived(Date.now());
          setIsConnected(true);
          setRetryCount(0);
        }
      }
    );

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
        <Text
          fontSize="24px"
          fontWeight="700"
          color="#ff006e"
          mb={6}
          textAlign="center"
          lineHeight="1.2"
        >
          Feeds Statistics
        </Text>

        {/* Connection Status */}
        <Flex justify="center" align="center" gap={2} mb={6}>
          <Box
            w={2}
            h={2}
            borderRadius="full"
            bg={isConnected ? "#4caf50" : "#e53935"}
            transition="background-color 0.3s ease"
          />
          <Text fontSize="sm" color="whiteAlpha.800">
            {isConnected
              ? "Connected"
              : retryCount > 0
                ? `Reconnecting... (${retryCount}/3)`
                : "Disconnected"
            }
          </Text>
        </Flex>

        <Flex direction="column" gap={4}>
          <StatCard
            icon={FiRss}
            label="TOTAL FEEDS"
            value={feedAmount}
            description="RSS feeds being monitored"
          />
          <StatCard
            icon={FiLayers}
            label="TOTAL ARTICLES"
            value={totalArticlesAmount}
            description="All articles across RSS feeds"
            data-testid="stat-card-total-articles"
          />
          <StatCard
            icon={FiFileText}
            label="UNSUMMARIZED ARTICLES"
            value={unsummarizedArticlesAmount}
            description="Articles waiting for AI summarization"
          />
        </Flex>

        {/* Footer */}
        <Text
          textAlign="center"
          fontSize="sm"
          color="whiteAlpha.600"
          mt={8}
        >
          Updates every 5 seconds via Server-Sent Events
        </Text>
      </Box>

      <FloatingMenu />
    </Box>
  );
}
