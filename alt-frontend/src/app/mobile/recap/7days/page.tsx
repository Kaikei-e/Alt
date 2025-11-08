"use client";

import { Box, Flex, Text } from "@chakra-ui/react";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import ErrorState from "@/app/mobile/feeds/_components/ErrorState";
import EmptyFeedState from "@/components/mobile/EmptyFeedState";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import RecapTimeline from "@/components/mobile/recap/RecapTimeline";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { useAuth } from "@/contexts/auth-context";
import { useRecapData } from "@/hooks/useRecapData";

export default function RecapSevenDaysPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { data, isInitialLoading, error, retry } = useRecapData();

  // 認証チェック（Middlewareが主、クライアント側は補助）
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push("/auth/login");
    }
  }, [isAuthenticated, authLoading, router]);

  // Auth loading
  if (authLoading) {
    return (
      <Box minH="100dvh" position="relative">
        <Box p={5} maxW="container.sm" mx="auto" height="100dvh" data-testid="recap-auth-loading">
          <Flex direction="column" gap={4}>
            {Array.from({ length: 3 }).map((_, index) => (
              <SkeletonFeedCard key={`skeleton-${index}`} />
            ))}
          </Flex>
        </Box>
        <FloatingMenu />
      </Box>
    );
  }

  // Initial loading skeleton
  if (isInitialLoading) {
    return (
      <Box minH="100dvh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100dvh"
          data-testid="recap-skeleton-container"
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
  if (!data || data.genres.length === 0) {
    return (
      <Box minH="100dvh" position="relative">
        <EmptyFeedState />
        <FloatingMenu />
      </Box>
    );
  }

  return (
    <Box minH="100dvh" position="relative">
      <Box
        p={5}
        maxW="container.sm"
        mx="auto"
        overflowY="auto"
        overflowX="hidden"
        height="100vh"
        data-testid="recap-scroll-container"
        bg="var(--app-bg)"
      >
        {/* ヘッダー */}
        <Box mb={6}>
          <Text
            fontSize="2xl"
            fontWeight="bold"
            color="var(--accent-primary)"
            mb={2}
            bgGradient="var(--accent-gradient)"
            bgClip="text"
          >
            7 Days Recap
          </Text>
          <Text fontSize="xs" color="var(--text-secondary)">
            Executed: {new Date(data.executedAt).toLocaleString("en-US")}
          </Text>
          <Text fontSize="xs" color="var(--text-secondary)">
            {data.totalArticles.toLocaleString()} articles analyzed
          </Text>
        </Box>

        {/* タイムライン */}
        <RecapTimeline genres={data.genres} />

        {/* フッター余白 */}
        <Box h={20} />
      </Box>

      <FloatingMenu />
    </Box>
  );
}

