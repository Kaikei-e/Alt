"use client";

import { Box, Text } from "@chakra-ui/react";
import ActivityItem from "@/components/desktop/home/ActivityItem";

export default function ActivityItemTest() {
  return (
    <Box p={8} bg="var(--app-bg)" minH="100vh">
      <Text fontSize="2xl" mb={6} textAlign="center">
        ActivityItem Component Test
      </Text>
      <Box maxW="400px" mx="auto">
        <ActivityItem
          type="new_feed"
          title="New feed added: TechCrunch"
          time="just now"
        />
      </Box>
    </Box>
  );
}