"use client";

import React, { useState } from "react";
import { Box, VStack, HStack, Button, Flex } from "@chakra-ui/react";
import { ReadingAnalytics } from "./ReadingAnalytics";
import { TrendingTopics } from "./TrendingTopics";
import { QuickActions } from "./QuickActions";
import { SourceAnalytics } from "./SourceAnalytics";
import { BookmarksList } from "./BookmarksList";
import { ReadingQueue } from "./ReadingQueue";
import { ThemeToggle } from "@/components/ThemeToggle";
import { useReadingAnalytics } from "@/hooks/useReadingAnalytics";
import { useTrendingTopics } from "@/hooks/useTrendingTopics";
import { useSourceAnalytics } from "@/hooks/useSourceAnalytics";

export const RightPanel: React.FC = () => {
  const [activeTab, setActiveTab] = useState<"analytics" | "actions">(
    "analytics",
  );
  const { analytics, isLoading: analyticsLoading } = useReadingAnalytics();
  const { topics, isLoading: topicsLoading } = useTrendingTopics();
  const { sources, isLoading: sourcesLoading } = useSourceAnalytics();

  return (
    <Flex
      direction="column"
      h="100vh"
      className="glass"
      borderRadius="var(--radius-xl)"
      overflow="hidden"
    >
      {/* Header with Theme Toggle */}
      <Flex
        justify="space-between"
        align="center"
        p={3}
        bg="var(--surface-bg)"
        borderBottom="1px solid var(--surface-border)"
        flexShrink={0}
      >
        <Box fontSize="sm" color="var(--text-muted)" fontWeight="medium">
          Analytics
        </Box>
        <ThemeToggle size="sm" />
      </Flex>

      {/* Tab Headers */}
      <HStack
        bg="var(--surface-bg)"
        borderBottom="1px solid var(--surface-border)"
        gap={0}
        flexShrink={0}
      >
        <Button
          flex={1}
          variant="ghost"
          size="sm"
          borderRadius={0}
          color={
            activeTab === "analytics"
              ? "var(--text-primary)"
              : "var(--text-secondary)"
          }
          bg={
            activeTab === "analytics" ? "var(--accent-primary)" : "transparent"
          }
          fontSize="sm"
          fontWeight="medium"
          onClick={() => setActiveTab("analytics")}
          transition="all 0.2s ease"
          _hover={{
            bg:
              activeTab === "analytics"
                ? "var(--accent-primary)"
                : "var(--surface-bg)",
            opacity: activeTab === "analytics" ? 1 : 0.8,
          }}
        >
          📊 Analytics
        </Button>
        <Button
          flex={1}
          variant="ghost"
          size="sm"
          borderRadius={0}
          color={
            activeTab === "actions"
              ? "var(--text-primary)"
              : "var(--text-secondary)"
          }
          bg={activeTab === "actions" ? "var(--accent-primary)" : "transparent"}
          fontSize="sm"
          fontWeight="medium"
          onClick={() => setActiveTab("actions")}
          transition="all 0.2s ease"
          _hover={{
            bg:
              activeTab === "actions"
                ? "var(--accent-primary)"
                : "var(--surface-bg)",
            opacity: activeTab === "actions" ? 1 : 0.8,
          }}
        >
          ⚡ Actions
        </Button>
      </HStack>

      {/* Tab Content - Scrollable */}
      <Box
        flex={1}
        overflowY="auto"
        overflowX="hidden"
        css={{
          "&::-webkit-scrollbar": {
            width: "6px",
          },
          "&::-webkit-scrollbar-track": {
            background: "var(--surface-secondary)",
            borderRadius: "3px",
          },
          "&::-webkit-scrollbar-thumb": {
            background: "var(--accent-primary)",
            borderRadius: "3px",
            opacity: 0.7,
          },
          "&::-webkit-scrollbar-thumb:hover": {
            opacity: 1,
          },
        }}
      >
        {activeTab === "analytics" && (
          <VStack gap={4} p={4} align="stretch">
            <ReadingAnalytics
              analytics={analytics}
              isLoading={analyticsLoading}
            />

            <TrendingTopics topics={topics} isLoading={topicsLoading} />

            <SourceAnalytics sources={sources} isLoading={sourcesLoading} />
          </VStack>
        )}

        {activeTab === "actions" && (
          <VStack gap={4} p={4} align="stretch">
            <QuickActions />
            <BookmarksList />
            <ReadingQueue />
          </VStack>
        )}
      </Box>
    </Flex>
  );
};
