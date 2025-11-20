"use client";

import { Box, Flex, Text } from "@chakra-ui/react";
import ErrorState from "@/app/mobile/feeds/_components/ErrorState";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import EmptyMorningState from "@/components/morning/EmptyMorningState";
import { MorningUpdateList } from "@/components/morning/MorningUpdateList";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { useMorningUpdates } from "@/hooks/useMorningUpdates";

export default function MorningLetterUpdatesPage() {
  const { data, isInitialLoading, error, retry } = useMorningUpdates();

  // Initial loading skeleton
  if (isInitialLoading) {
    return (
      <Box minH="100dvh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100dvh"
          data-testid="morning-letter-skeleton-container"
        >
          <Flex direction="column" gap={4}>
            {Array.from({ length: 5 }).map((_, index) => (
              <SkeletonFeedCard key={`skeleton-${index}`} />
            ))}
          </Flex>
        </Box>
        <FloatingMenu />
      </Box>
    );
  }

  // Error state
  if (error) {
    return <ErrorState error={error} onRetry={retry} isLoading={false} />;
  }

  // Empty state
  if (!data || data.length === 0) {
    return (
      <Box minH="100dvh" position="relative">
        <EmptyMorningState />
        <FloatingMenu />
      </Box>
    );
  }

  const today = new Date();
  const formattedDate = today.toLocaleDateString("en-US", {
    month: "long",
    day: "numeric",
    year: "numeric",
  });

  return (
    <Box minH="100dvh" position="relative">
      <Box
        p={5}
        maxW="container.sm"
        mx="auto"
        overflowY="auto"
        overflowX="hidden"
        height="100vh"
        data-testid="morning-letter-scroll-container"
        bg="var(--app-bg)"
      >
        {/* Header */}
        <Box mb={6}>
          <Text
            fontSize="2xl"
            fontWeight="bold"
            color="var(--accent-primary)"
            mb={2}
            bgGradient="var(--accent-gradient)"
            bgClip="text"
          >
            Morning Letter
          </Text>
          <Text fontSize="sm" color="var(--text-secondary)">
            {formattedDate}
          </Text>
        </Box>

        {/* Section: Overnight Updates */}
        <Box mb={6}>
          <Text
            fontSize="lg"
            fontWeight="semibold"
            color="var(--text-primary)"
            mb={4}
          >
            Overnight Updates
          </Text>
          <MorningUpdateList updates={data} />
        </Box>

        {/* Footer spacing */}
        <Box h={20} />
      </Box>

      <FloatingMenu />
    </Box>
  );
}

