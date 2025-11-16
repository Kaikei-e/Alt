"use client";

import { Box, Flex, HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import type React from "react";
import type { TrendingTopic } from "@/types/analytics";

interface TrendingTopicsProps {
  topics: TrendingTopic[];
  isLoading: boolean;
}

export const TrendingTopics: React.FC<TrendingTopicsProps> = ({
  topics,
  isLoading,
}) => {
  if (isLoading) {
    return (
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Text
          fontSize="sm"
          fontWeight="bold"
          color="var(--text-primary)"
          mb={3}
        >
          🔥 Trending Topics
        </Text>
        <Flex justify="center" align="center" h="60px">
          <Spinner color="var(--accent-primary)" size="sm" />
        </Flex>
      </Box>
    );
  }

  const getTrendColor = (trend: string) => {
    switch (trend) {
      case "up":
        return "var(--alt-success)";
      case "down":
        return "var(--alt-error)";
      case "stable":
        return "var(--alt-warning)";
      default:
        return "var(--text-secondary)";
    }
  };

  const getTrendIcon = (trend: string) => {
    switch (trend) {
      case "up":
        return "📈";
      case "down":
        return "📉";
      case "stable":
        return "➡️";
      default:
        return "⚪";
    }
  };

  return (
    <Box className="glass" p={4} borderRadius="var(--radius-lg)">
      <Text fontSize="sm" fontWeight="bold" color="var(--text-primary)" mb={3}>
        🔥 Trending Topics
      </Text>

      <VStack gap={2} align="stretch">
        {topics.slice(0, 6).map((topic, index) => (
          <HStack
            key={topic.tag}
            justify="space-between"
            p={2}
            bg="var(--surface-bg)"
            borderRadius="var(--radius-md)"
            border="1px solid var(--surface-border)"
            _hover={{
              bg: "var(--surface-hover)",
              transform: "translateX(2px)",
            }}
            transition="all var(--transition-speed) ease"
            cursor="pointer"
          >
            <HStack gap={2}>
              <Text
                fontSize="xs"
                color="var(--text-muted)"
                fontWeight="bold"
                minW="16px"
              >
                #{index + 1}
              </Text>
              <VStack gap={0} align="start">
                <HStack gap={1}>
                  <Box
                    w="6px"
                    h="6px"
                    bg={topic.color}
                    borderRadius="var(--radius-full)"
                  />
                  <Text
                    fontSize="xs"
                    color="var(--text-primary)"
                    fontWeight="medium"
                  >
                    #{topic.tag}
                  </Text>
                </HStack>
                <Text fontSize="xs" color="var(--text-muted)">
                  {topic.count} articles
                </Text>
              </VStack>
            </HStack>

            <HStack gap={1}>
              <Text fontSize="xs" color={getTrendColor(topic.trend)}>
                {getTrendIcon(topic.trend)}
              </Text>
              <Text
                fontSize="xs"
                color={getTrendColor(topic.trend)}
                fontWeight="medium"
              >
                {topic.trendValue > 0 ? "+" : ""}
                {topic.trendValue}%
              </Text>
            </HStack>
          </HStack>
        ))}
      </VStack>
    </Box>
  );
};
