"use client";

import React from "react";
import { Box, HStack, VStack, Text } from "@chakra-ui/react";

interface StatsFooterProps {
  stats: {
    weeklyReads: number;
    aiProcessed: number;
    bookmarks: number;
  };
}

export const StatsFooter: React.FC<StatsFooterProps> = ({ stats }) => {
  return (
    <Box
      data-testid="stats-footer"
      pt={4}
      borderTop="1px solid var(--surface-border)"
      mt={4}
    >
      <HStack gap={6} justify="space-between">
        <VStack gap={1} align="start">
          <Text fontSize="xs" color="var(--text-muted)">
            Weekly Reads
          </Text>
          <Text fontSize="sm" fontWeight="semibold" color="var(--text-primary)">
            {stats.weeklyReads}
          </Text>
        </VStack>

        <VStack gap={1} align="start">
          <Text fontSize="xs" color="var(--text-muted)">
            AI Processed
          </Text>
          <Text fontSize="sm" fontWeight="semibold" color="var(--text-primary)">
            {stats.aiProcessed}
          </Text>
        </VStack>

        <VStack gap={1} align="start">
          <Text fontSize="xs" color="var(--text-muted)">
            Bookmarks
          </Text>
          <Text fontSize="sm" fontWeight="semibold" color="var(--text-primary)">
            {stats.bookmarks}
          </Text>
        </VStack>
      </HStack>
    </Box>
  );
};

export default StatsFooter;
