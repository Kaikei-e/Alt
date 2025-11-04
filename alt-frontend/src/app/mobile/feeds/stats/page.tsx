"use client";

import { Box, Flex, Text } from "@chakra-ui/react";
import { FileText, Layers, Rss } from "lucide-react";
import React from "react";
import { StatCard } from "@/components/mobile/stats/StatCard";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { useSSEFeedsStats } from "@/hooks/useSSEFeedsStats";

export default function FeedsStatsPage() {
  // Use the dedicated SSE hook for feeds stats
  const { feedAmount, unsummarizedArticlesAmount, totalArticlesAmount, isConnected, retryCount } =
    useSSEFeedsStats();

  return (
    <Box
      minH="100vh"
      minHeight="100dvh"
      background="var(--app-bg)"
      position="relative"
      overflowX="hidden"
      pt="env(safe-area-inset-top)"
      pb="env(safe-area-inset-bottom)"
    >
      <Box p={5} maxW="container.sm" mx="auto" pt={8} overflowX="hidden">
        {/* Header */}
        <Box mb={8} textAlign="center">
          <Text
            fontSize="2xl"
            fontWeight="bold"
            color="var(--alt-primary)"
            textShadow="0 0 20px var(--text-primary)"
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
              bg={isConnected ? "var(--alt-success)" : "var(--alt-error)"}
              transition="background-color 0.3s ease"
            />
            <Text
              fontSize="sm"
              color={
                isConnected
                  ? "var(--text-primary)"
                  : retryCount > 0
                    ? "var(--text-primary)"
                    : "var(--text-primary)"
              }
              textShadow={
                isConnected
                  ? "0 0 8px var(--alt-success)"
                  : retryCount > 0
                    ? "0 0 8px var(--alt-warning)"
                    : "0 0 8px var(--alt-error)"
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
        <Flex direction="column" gap={6} overflowY="scroll">
          <StatCard
            label="TOTAL FEEDS"
            value={feedAmount}
            description="RSS feeds being monitored"
            icon={Rss}
          />

          <StatCard
            label="TOTAL ARTICLES"
            value={totalArticlesAmount}
            description="All articles across RSS feeds"
            icon={FileText}
          />

          <StatCard
            label="UNSUMMARIZED ARTICLES"
            value={unsummarizedArticlesAmount}
            description="Articles waiting for AI summarization"
            icon={Layers}
          />
        </Flex>
      </Box>

      <FloatingMenu />
    </Box>
  );
}
