import { keyframes } from "@emotion/react";
import { Box, HStack, Icon, Text, VStack } from "@chakra-ui/react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

const arrowDrift = keyframes`
  0% { transform: translateX(-4px); opacity: 0.4; }
  50% { transform: translateX(4px); opacity: 1; }
  100% { transform: translateX(-4px); opacity: 0.4; }
`;

const dotPulse = keyframes`
  0%, 100% { opacity: 0.3; transform: scale(0.9); }
  50% { opacity: 0.9; transform: scale(1); }
`;

const SwipeSkeletonHint = ({
  prefersReducedMotion,
}: {
  prefersReducedMotion: boolean;
}) => (
  <VStack
    gap={3}
    align="center"
    data-testid="swipe-skeleton-hint"
    data-reduced-motion={prefersReducedMotion ? "true" : "false"}
  >
    <Text fontSize="sm" color="var(--alt-text-secondary)">
      スワイプで次の記事へ進めます
    </Text>
    <HStack gap={3} color="var(--alt-primary)">
      <Icon
        as={ChevronLeft}
        boxSize={6}
        opacity={0.9}
        style={
          prefersReducedMotion
            ? undefined
            : {
                animation: `${arrowDrift} 1.6s ease-in-out infinite`,
              }
        }
      />
      {Array.from({ length: 3 }).map((_, index) => (
        <Box
          key={`swipe-dot-${index}`}
          w={2}
          h={2}
          borderRadius="full"
          bg="var(--alt-primary)"
          opacity={0.6}
          style={
            prefersReducedMotion
              ? undefined
              : {
                  animation: `${dotPulse} 1.8s ${(index + 1) * 0.12}s ease-in-out infinite`,
                }
          }
        />
      ))}
      <Icon
        as={ChevronRight}
        boxSize={6}
        opacity={0.9}
        style={
          prefersReducedMotion
            ? undefined
            : {
                animation: `${arrowDrift} 1.6s ease-in-out infinite reverse`,
              }
        }
      />
    </HStack>
  </VStack>
);

export const SwipeFeedSkeleton = ({
  prefersReducedMotion,
}: {
  prefersReducedMotion: boolean;
}) => (
  <Box minH="100dvh" position="relative">
    <Box
      p={5}
      maxW="container.sm"
      mx="auto"
      height="100dvh"
      data-testid="swipe-skeleton-container"
      display="flex"
      alignItems="center"
      justifyContent="center"
    >
      <VStack gap={8} w="100%">
        <SkeletonFeedCard variant="swipe" reduceMotion={prefersReducedMotion} />
        <SwipeSkeletonHint prefersReducedMotion={prefersReducedMotion} />
      </VStack>
    </Box>
    <FloatingMenu />
  </Box>
);

export default SwipeFeedSkeleton;
