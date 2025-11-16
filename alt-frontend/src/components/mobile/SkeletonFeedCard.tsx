import { keyframes } from "@emotion/react";
import { Box, Flex, Skeleton, SkeletonCircle } from "@chakra-ui/react";
import { memo } from "react";

type SkeletonFeedCardProps = {
  variant?: "list" | "swipe";
  reduceMotion?: boolean;
};

const peekFloat = keyframes`
  0% { transform: translateY(0); opacity: 0.35; }
  50% { transform: translateY(6px); opacity: 0.55; }
  100% { transform: translateY(0); opacity: 0.35; }
`;

const ListSkeleton = () => (
  <Box
    data-testid="skeleton-feed-card"
    bg="var(--accent-gradient)"
    borderRadius="18px"
    mb="16px"
    p="2px"
  >
    <Box
      w="100%"
      p="20px"
      borderRadius="16px"
      bg="#1a1a2e"
      opacity={0.6}
      display="flex"
      flexDir="column"
      gap={4}
    >
      <Skeleton height="24px" width="80%" borderRadius="4px" />
      <Flex direction="column" gap={3}>
        <Skeleton height="16px" width="100%" borderRadius="4px" />
        <Skeleton height="16px" width="90%" borderRadius="4px" />
        <Skeleton height="16px" width="70%" borderRadius="4px" />
      </Flex>
      <Flex justify="space-between" align="center" gap={4}>
        <Skeleton height="32px" width="120px" borderRadius="16px" />
        <Skeleton height="32px" width="100px" borderRadius="16px" />
      </Flex>
    </Box>
  </Box>
);

const SwipeSkeletonCard = ({ reduceMotion }: { reduceMotion: boolean }) => (
  <Box position="relative" w="100%" maxW="28rem" mx="auto" py={4}>
    <Box
      aria-hidden="true"
      border="1px dashed var(--alt-glass-border)"
      borderRadius="1.25rem"
      position="absolute"
      insetX="1.5rem"
      top="1.25rem"
      bottom="-0.25rem"
      pointerEvents="none"
      style={
        reduceMotion
          ? { opacity: 0.4 }
          : {
              animation: `${peekFloat} 3.2s ease-in-out infinite`,
            }
      }
    />
    <Box
      data-testid="swipe-skeleton-card"
      borderRadius="1.25rem"
      border="1px solid var(--alt-glass-border)"
      bg="rgba(8, 8, 18, 0.85)"
      backdropFilter="blur(18px)"
      px={{ base: 5, md: 6 }}
      py={{ base: 6, md: 7 }}
      boxShadow="0 20px 45px rgba(0, 0, 0, 0.45)"
    >
      <Flex justify="space-between" align="center" mb={5}>
        <SkeletonCircle size="12" />
        <Skeleton height="20px" width="32%" borderRadius="999px" />
      </Flex>
      <Skeleton height="28px" width="82%" borderRadius="md" mb={4} />
      <Flex direction="column" gap={3}>
        {Array.from({ length: 4 }).map((_, index) => (
          <Skeleton
            // eslint-disable-next-line react/no-array-index-key
            key={`swipe-skeleton-line-${index}`}
            height="16px"
            width={`${90 - index * 10}%`}
            borderRadius="4px"
          />
        ))}
      </Flex>
      <Flex justify="space-between" align="center" mt={6} gap={4}>
        <Skeleton height="40px" width="48%" borderRadius="lg" />
        <Skeleton height="40px" width="40%" borderRadius="lg" />
      </Flex>
    </Box>
  </Box>
);

const SkeletonFeedCard = memo(function SkeletonFeedCard({
  variant = "list",
  reduceMotion = false,
}: SkeletonFeedCardProps) {
  if (variant === "swipe") {
    return <SwipeSkeletonCard reduceMotion={reduceMotion} />;
  }

  return <ListSkeleton />;
});

export default SkeletonFeedCard;
