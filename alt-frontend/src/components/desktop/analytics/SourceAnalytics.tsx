"use client";

import React, { useState } from "react";
import {
  Box,
  VStack,
  HStack,
  Text,
  Spinner,
  Flex,
  Button,
} from "@chakra-ui/react";
import { Progress } from "@chakra-ui/progress";
import { SourceAnalytic } from "@/types/analytics";

interface SourceAnalyticsProps {
  sources: SourceAnalytic[];
  isLoading: boolean;
}

export const SourceAnalytics: React.FC<SourceAnalyticsProps> = ({
  sources,
  isLoading,
}) => {
  const [sortBy, setSortBy] = useState<
    "articles" | "reliability" | "engagement"
  >("articles");
  const [showAll, setShowAll] = useState(false);

  if (isLoading) {
    return (
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Text
          fontSize="sm"
          fontWeight="bold"
          color="var(--text-primary)"
          mb={3}
        >
          📰 Source Analytics
        </Text>
        <Flex justify="center" align="center" h="80px">
          <Spinner color="var(--accent-primary)" size="sm" />
        </Flex>
      </Box>
    );
  }

  const sortedSources = [...sources].sort((a, b) => {
    switch (sortBy) {
      case "articles":
        return b.totalArticles - a.totalArticles;
      case "reliability":
        return b.reliability - a.reliability;
      case "engagement":
        return b.engagement - a.engagement;
      default:
        return 0;
    }
  });

  const displaySources = showAll ? sortedSources : sortedSources.slice(0, 4);

  return (
    <Box className="glass" p={4} borderRadius="var(--radius-lg)">
      <HStack justify="space-between" mb={3}>
        <Text fontSize="sm" fontWeight="bold" color="var(--text-primary)">
          📰 Source Analytics
        </Text>
        <HStack gap={1}>
          <Button
            size="xs"
            variant={sortBy === "articles" ? "solid" : "ghost"}
            colorScheme={sortBy === "articles" ? "purple" : "gray"}
            onClick={() => setSortBy("articles")}
            fontSize="xs"
          >
            Articles
          </Button>
          <Button
            size="xs"
            variant={sortBy === "reliability" ? "solid" : "ghost"}
            colorScheme={sortBy === "reliability" ? "purple" : "gray"}
            onClick={() => setSortBy("reliability")}
            fontSize="xs"
          >
            Reliability
          </Button>
          <Button
            size="xs"
            variant={sortBy === "engagement" ? "solid" : "ghost"}
            colorScheme={sortBy === "engagement" ? "purple" : "gray"}
            onClick={() => setSortBy("engagement")}
            fontSize="xs"
          >
            Engagement
          </Button>
        </HStack>
      </HStack>

      <VStack gap={3} align="stretch">
        {displaySources.map((source, index) => (
          <Box
            key={source.id}
            p={3}
            bg="var(--surface-bg)"
            borderRadius="var(--radius-md)"
            border="1px solid var(--surface-border)"
          >
            <HStack justify="space-between" mb={2}>
              <HStack gap={2}>
                <Text fontSize="xs" color="var(--text-muted)" fontWeight="bold">
                  #{index + 1}
                </Text>
                <Text fontSize="sm">{source.icon}</Text>
                <VStack gap={0} align="start">
                  <Text
                    fontSize="xs"
                    color="var(--text-primary)"
                    fontWeight="medium"
                  >
                    {source.name}
                  </Text>
                  <Text fontSize="xs" color="var(--text-muted)">
                    {source.category}
                  </Text>
                </VStack>
              </HStack>
            </HStack>

            <VStack gap={2} align="stretch">
              <HStack justify="space-between">
                <Text fontSize="xs" color="var(--text-secondary)">
                  Articles:
                </Text>
                <Text
                  fontSize="xs"
                  color="var(--text-primary)"
                  fontWeight="medium"
                >
                  {source.totalArticles}
                </Text>
              </HStack>

              <HStack justify="space-between">
                <Text fontSize="xs" color="var(--text-secondary)">
                  Reliability:
                </Text>
                <HStack gap={1}>
                  <Progress
                    value={(source.reliability / 10) * 100}
                    size="sm"
                    w="40px"
                    bg="var(--surface-border)"
                    sx={{
                      "& > div": {
                        bg: "var(--alt-success)",
                      },
                    }}
                    borderRadius="var(--radius-full)"
                  />
                  <Text
                    fontSize="xs"
                    color="var(--text-primary)"
                    fontWeight="medium"
                  >
                    {source.reliability}/10
                  </Text>
                </HStack>
              </HStack>

              <HStack justify="space-between">
                <Text fontSize="xs" color="var(--text-secondary)">
                  Engagement:
                </Text>
                <HStack gap={1}>
                  <Progress
                    value={source.engagement}
                    size="sm"
                    w="40px"
                    bg="var(--surface-border)"
                    sx={{
                      "& > div": {
                        bg: "var(--accent-primary)",
                      },
                    }}
                    borderRadius="var(--radius-full)"
                  />
                  <Text
                    fontSize="xs"
                    color="var(--text-primary)"
                    fontWeight="medium"
                  >
                    {source.engagement}%
                  </Text>
                </HStack>
              </HStack>
            </VStack>
          </Box>
        ))}

        {sources.length > 4 && (
          <Button
            size="xs"
            variant="ghost"
            color="var(--accent-primary)"
            onClick={() => setShowAll(!showAll)}
            _hover={{
              bg: "var(--surface-hover)",
            }}
          >
            {showAll ? "Show Less" : `Show All (${sources.length})`}
          </Button>
        )}
      </VStack>
    </Box>
  );
};
