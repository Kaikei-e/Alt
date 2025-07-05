"use client";

import { Box } from "@chakra-ui/react";
import { Rss } from "lucide-react";
import StatsCard from "@/components/desktop/home/StatsCard";

export default function StatsCardTestPage() {
  return (
    <Box p={8} minH="100vh" bg="var(--app-bg)">
      <StatsCard
        icon={Rss}
        label="Total Feeds"
        value={42}
        trend="+12%"
        trendLabel="from last week"
        color="primary"
        isLoading={false}
      />
    </Box>
  );
}