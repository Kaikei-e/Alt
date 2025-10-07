"use client";

import { Box, Text } from "@chakra-ui/react";
import ActivityFeed from "@/components/desktop/home/ActivityFeed";

export default function ActivityFeedTest() {
  const mockActivities = [
    {
      id: 1,
      type: "new_feed" as const,
      title: "New feed added: TechCrunch",
      time: "just now",
    },
    {
      id: 2,
      type: "read" as const,
      title: "Article read: React 19 Release",
      time: "1 hour ago",
    },
  ];

  return (
    <Box p={8} bg="var(--app-bg)" minH="100vh">
      <Text fontSize="2xl" mb={6} textAlign="center">
        ActivityFeed Component Test
      </Text>
      <Box maxW="400px" mx="auto">
        <ActivityFeed activities={mockActivities} />
      </Box>
    </Box>
  );
}
