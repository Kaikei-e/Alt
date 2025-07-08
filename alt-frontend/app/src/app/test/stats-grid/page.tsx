"use client";

import { Box, Text } from "@chakra-ui/react";
import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { Rss, FileText, Zap } from "lucide-react";
import StatsGrid from "@/components/desktop/home/StatsGrid";

function StatsGridTestContent() {
  const searchParams = useSearchParams();
  if (!searchParams) {
    return <Text>No search params</Text>;
  }

  const isLoading = searchParams.get("loading") === "true";
  const hasError = searchParams.get("error") === "true";

  const mockStats = [
    {
      id: "total-feeds",
      icon: Rss,
      label: "Total Feeds",
      value: 42,
      trend: "+12%",
      trendLabel: "from last week",
      color: "primary" as const,
    },
    {
      id: "ai-processed",
      icon: Zap,
      label: "AI Processed",
      value: 156,
      trend: "+89%",
      trendLabel: "efficiency boost",
      color: "secondary" as const,
    },
    {
      id: "unread-articles",
      icon: FileText,
      label: "Unread Articles",
      value: 23,
      trend: "+5%",
      trendLabel: "new today",
      color: "tertiary" as const,
    },
  ];

  return (
    <Box p={8} minH="100vh" bg="var(--app-bg)">
      <StatsGrid
        stats={mockStats}
        isLoading={isLoading}
        error={hasError ? "Test error message" : null}
      />
    </Box>
  );
}

export default function StatsGridTestPage() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <StatsGridTestContent />
    </Suspense>
  );
}
