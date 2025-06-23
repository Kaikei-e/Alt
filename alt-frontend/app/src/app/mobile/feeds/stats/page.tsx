"use client";

import { FeedStatsSummary } from "@/schema/feedStats";
import { feedsApiSse } from "@/lib/apiSse";
import { Flex, Text, Box } from "@chakra-ui/react";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { useEffect, useState, useRef } from "react";
import { useSSEProgress } from "@/hooks/useSSEProgress";
import { FiRss, FiFileText } from "react-icons/fi";
import { SSEProgressBar } from "@/components/mobile/stats/SSEProgressBar";
import { StatCard } from "@/components/mobile/stats/StatCard";

export default function FeedsStatsPage() {
  const [feedAmount, setFeedAmount] = useState(0);
  const [unsummarizedArticlesAmount, setUnsummarizedArticlesAmount] = useState(0);
  const [isConnected, setIsConnected] = useState(false);
  const [lastDataReceived, setLastDataReceived] = useState<number>(Date.now());
  const eventSourceRef = useRef<{
    close: () => void;
    getReadyState: () => number;
  } | null>(null);

  // Progress tracking for SSE updates (5-second cycle)
  const { progress, reset: resetProgress } = useSSEProgress(5000);

  // Connection health check
  useEffect(() => {
    const healthCheck = setInterval(() => {
      const timeSinceLastData = Date.now() - lastDataReceived;
      const readyState = eventSourceRef.current?.getReadyState() ?? EventSource.CLOSED;

      // Consider connected if:
      // 1. EventSource is in OPEN state AND
      // 2. We've received data within the last 10 seconds (2x the expected interval)
      const shouldBeConnected = readyState === EventSource.OPEN && timeSinceLastData < 10000;

      setIsConnected(shouldBeConnected);
    }, 1000); // Check every second

    return () => clearInterval(healthCheck);
  }, [lastDataReceived]);

  useEffect(() => {
    const sseConnection = feedsApiSse.getFeedsStats(
      (data: FeedStatsSummary) => {
        // Update data
        if (data.feed_amount?.amount !== undefined) {
          setFeedAmount(data.feed_amount.amount);
        }
        if (data.summarized_feed?.amount !== undefined) {
          setUnsummarizedArticlesAmount(data.summarized_feed.amount);
        }

        // Update last data received timestamp
        setLastDataReceived(Date.now());
        resetProgress(); // Reset progress bar on new data
      },
      (event) => {
        console.error("SSE connection error:", event);
        // Don't immediately set to disconnected - let the health check handle it
        // This prevents flickering when there are temporary connection issues
      },
    );

    eventSourceRef.current = sseConnection;

    return () => {
      eventSourceRef.current?.close();
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
          fontSize="2xl"
          fontWeight="bold"
          color="var(--vaporwave-pink)"
          mb={6}
          textAlign="center"
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
            {isConnected ? "Connected" : "Disconnected"}
          </Text>
        </Flex>

        {/* Stats Cards */}
        <Flex direction="column" gap={4}>
          <StatCard
            icon={FiRss}
            label="Total Feeds"
            value={feedAmount}
            description="RSS feeds being monitored"
          />

          <StatCard
            icon={FiFileText}
            label="Unsummarized Articles"
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
