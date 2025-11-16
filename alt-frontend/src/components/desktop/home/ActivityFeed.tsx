"use client";

import { Box, HStack, Icon, Text, VStack } from "@chakra-ui/react";
import { Activity } from "lucide-react";
import type React from "react";
import { ActivityItem } from "./ActivityItem";

interface ActivityFeedProps {
  activities: Array<{
    id: number;
    type: "new_feed" | "ai_summary" | "bookmark" | "read";
    title: string;
    time: string;
  }>;
  maxHeight?: string;
  isLoading?: boolean;
}

export const ActivityFeed: React.FC<ActivityFeedProps> = ({
  activities,
  maxHeight = "400px",
  isLoading = false,
}) => {
  return (
    <Box
      data-testid="activity-feed"
      className="glass"
      p={6}
      borderRadius="var(--radius-xl)"
      border="1px solid var(--surface-border)"
      h={maxHeight}
      display="flex"
      flexDirection="column"
    >
      <HStack
        data-testid="activity-header"
        mb={4}
        align="center"
        justify="space-between"
      >
        <HStack>
          <Icon as={Activity} color="var(--alt-primary)" boxSize={5} />
          <Text fontSize="lg" fontWeight="semibold" color="var(--text-primary)">
            Recent Activity
          </Text>
        </HStack>
      </HStack>

      <Box flex="1" overflow="auto">
        {isLoading ? (
          <Box data-testid="loading-state" textAlign="center" py={8}>
            <Text color="var(--text-muted)">Loading activities...</Text>
          </Box>
        ) : activities.length === 0 ? (
          <Box data-testid="empty-state" textAlign="center" py={8}>
            <Text color="var(--text-muted)">No recent activity</Text>
          </Box>
        ) : (
          <VStack gap={2} align="stretch">
            {activities.map((activity) => (
              <ActivityItem
                key={activity.id}
                type={activity.type}
                title={activity.title}
                time={activity.time}
              />
            ))}
          </VStack>
        )}
      </Box>
    </Box>
  );
};

export default ActivityFeed;
