"use client";

import { Box } from "@chakra-ui/react";
import ActivityItem from "@/components/desktop/home/ActivityItem";

export default function ActivityItemTestPage() {
  return (
    <Box p={8} minH="100vh" bg="var(--app-bg)">
      <ActivityItem
        type="new_feed"
        title="TechCrunch added"
        time="2 min ago"
      />
    </Box>
  );
}