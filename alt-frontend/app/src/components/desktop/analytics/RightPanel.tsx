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
    <Box className="glass" borderRadius="var(--radius-xl)" overflow="hidden">
      {/* Header with Theme Toggle */}
      <Flex
        justify="space-between"
        align="center"
        p={3}
        bg="var(--surface-bg)"
        borderBottom="1px solid var(--surface-border)"
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
          _hover={{
            bg:
              activeTab === "analytics"
                ? "var(--accent-primary)"
                : "var(--surface-hover)",
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
          _hover={{
            bg:
              activeTab === "actions"
                ? "var(--accent-primary)"
                : "var(--surface-hover)",
          }}
        >
          ⚡ Actions
        </Button>
      </HStack>

      {/* Tab Content */}
      <Box overflowY="scroll">
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
    </Box>
  );
};
