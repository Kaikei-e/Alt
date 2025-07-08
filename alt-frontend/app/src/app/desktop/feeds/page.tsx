"use client";

import React, { Suspense } from "react";
import { Box, Text } from "@chakra-ui/react";
import { DesktopFeedsLayout } from "@/components/desktop/layout/DesktopFeedsLayout";
import { DesktopHeader } from "@/components/desktop/layout/DesktopHeader";
import { DesktopSidebar } from "@/components/desktop/layout/DesktopSidebar";
import DesktopTimeline from "@/components/desktop/timeline/DesktopTimeline";

// Basic stats for header
const mockStats = {
  totalUnread: 86,
  totalFeeds: 8,
  readToday: 12,
  weeklyAverage: 45,
};

const mockFeedSources = [
  {
    id: "techcrunch",
    name: "TechCrunch",
    icon: "📰",
    unreadCount: 12,
    category: "tech",
  },
  {
    id: "hackernews",
    name: "Hacker News",
    icon: "🔥",
    unreadCount: 8,
    category: "tech",
  },
];

// Loading fallback
const LoadingFallback = () => (
  <Box
    h="100vh"
    display="flex"
    alignItems="center"
    justifyContent="center"
    bg="var(--app-bg)"
  >
    <Box
      className="glass"
      p={8}
      borderRadius="var(--radius-xl)"
      textAlign="center"
    >
      <div
        style={{
          width: "32px",
          height: "32px",
          border: "3px solid var(--surface-border)",
          borderTop: "3px solid var(--accent-primary)",
          borderRadius: "50%",
          animation: "spin 1s linear infinite",
          margin: "0 auto 16px",
        }}
      />
      <Text color="var(--text-primary)" fontSize="lg">
        Loading Alt Feeds...
      </Text>
    </Box>
  </Box>
);

function DesktopFeedsContent() {
  return (
    <DesktopFeedsLayout
      header={
        <DesktopHeader
          totalUnread={mockStats.totalUnread}
          searchQuery=""
          onSearchChange={() => {}}
          currentTheme={"vaporwave"}
          onThemeToggle={() => {}}
        />
      }
      sidebar={
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={{
            sources: [],
            timeRange: "all",
            readStatus: "all",
            tags: [],
            priority: "all",
          }}
          onFilterChange={() => {}}
          onClearAll={() => {}}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={() => {}}
        />
      }
    >
      <DesktopTimeline />
    </DesktopFeedsLayout>
  );
}

export default function DesktopFeedsPage() {
  return (
    <Suspense fallback={<LoadingFallback />}>
      <DesktopFeedsContent />
    </Suspense>
  );
}
