"use client";

import { Box, Button, Flex, Icon, Text } from "@chakra-ui/react";
import { AlertTriangle } from "lucide-react";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import EmptyMorningState from "@/components/morning/EmptyMorningState";
import { MorningUpdateList } from "@/components/morning/MorningUpdateList";
import { useMorningUpdates } from "@/hooks/useMorningUpdates";

export default function MorningLetterUpdatesPage() {
  const { data, isInitialLoading, error, retry, isLoading } =
    useMorningUpdates();

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

  // Empty state (only show if no data and no error, or if error but no cached data)
  const hasData = data && data.length > 0;
  const showEmptyState = !hasData && !error;

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
        {/* Error Banner - Show at top if error exists, but don't block the view */}
        {error && (
          <Box
            borderRadius="md"
            mb={4}
            p={3}
            bg="red.50"
            border="1px solid"
            borderColor="red.200"
          >
            <Flex alignItems="center" mb={2}>
              <Icon as={AlertTriangle} boxSize="16px" color="red.500" mr={2} />
              <Text fontSize="sm" fontWeight="semibold" color="red.700">
                更新の取得に失敗しました
              </Text>
            </Flex>
            <Text fontSize="xs" mb={3} color="gray.700">
              {error.message || "ネットワークエラーが発生しました"}
            </Text>
            <Button
              size="sm"
              colorScheme="red"
              onClick={retry}
              loading={isLoading}
              loadingText="再試行中..."
            >
              再試行
            </Button>
          </Box>
        )}

        {/* Empty State - Only show if no data and no error */}
        {showEmptyState && <EmptyMorningState />}

        {/* Content - Show if we have data */}
        {hasData && (
          <>
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
          </>
        )}
      </Box>

      <FloatingMenu />
    </Box>
  );
}
