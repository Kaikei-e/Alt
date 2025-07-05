"use client";

import React from "react";
import { Flex, Box, Text, Icon } from "@chakra-ui/react";
import { Plus, Zap, Bookmark, TrendingUp } from "lucide-react";

interface ActivityItemProps {
  type: 'new_feed' | 'ai_summary' | 'bookmark' | 'read';
  title: string;
  time: string;
}

export const ActivityItem: React.FC<ActivityItemProps> = ({
  type,
  title,
  time,
}) => {
  const getActivityIcon = (activityType: ActivityItemProps['type']) => {
    switch (activityType) {
      case 'new_feed':
        return Plus;
      case 'ai_summary':
        return Zap;
      case 'bookmark':
        return Bookmark;
      case 'read':
        return TrendingUp;
      default:
        return Plus;
    }
  };

  const ActivityIcon = getActivityIcon(type);

  return (
    <Flex
      data-testid="activity-item"
      align="center"
      gap={3}
      p={3}
      borderRadius="var(--radius-md)"
      _hover={{ bg: "var(--surface-hover)" }}
      transition="all var(--transition-speed) ease"
      cursor="pointer"
    >
      <Box
        w={8}
        h={8}
        borderRadius="full"
        bg="var(--alt-primary)"
        opacity={0.2}
        display="flex"
        alignItems="center"
        justifyContent="center"
      >
        <Icon
          as={ActivityIcon}
          color="var(--alt-primary)"
          boxSize={4}
        />
      </Box>
      <Box flex="1">
        <Text
          fontSize="sm"
          color="var(--text-primary)"
          fontWeight="medium"
        >
          {title}
        </Text>
        <Text fontSize="xs" color="var(--text-muted)">
          {time}
        </Text>
      </Box>
    </Flex>
  );
};

export default ActivityItem;