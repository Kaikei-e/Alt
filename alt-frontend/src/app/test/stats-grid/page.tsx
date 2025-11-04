"use client";

import { Box, Text } from "@chakra-ui/react";
import { Bookmark, TrendingUp, Zap } from "lucide-react";
import StatsGrid from "@/components/desktop/home/StatsGrid";

export default function StatsGridTest() {
  const mockStats = [
    {
      id: "1",
      icon: TrendingUp,
      label: "Weekly Reads",
      value: 156,
      trend: "+12%",
      trendLabel: "from last week",
      color: "primary" as const,
    },
    {
      id: "2",
      icon: Zap,
      label: "AI Processed",
      value: 78,
      trend: "+5%",
      trendLabel: "this week",
      color: "secondary" as const,
    },
    {
      id: "3",
      icon: Bookmark,
      label: "Bookmarks",
      value: 42,
      trend: "-3%",
      trendLabel: "from last week",
      color: "tertiary" as const,
    },
  ];

  return (
    <Box p={8} bg="var(--app-bg)" minH="100vh">
      <Text fontSize="2xl" mb={6} textAlign="center">
        StatsGrid Component Test
      </Text>
      <Box maxW="800px" mx="auto">
        <StatsGrid stats={mockStats} />
      </Box>
    </Box>
  );
}
