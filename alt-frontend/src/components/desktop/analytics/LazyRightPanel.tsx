"use client";

import { Box, HStack, Skeleton, Text, VStack } from "@chakra-ui/react";
import React, { Suspense } from "react";
import { ErrorBoundary } from "react-error-boundary";

// Lazy load the right panel component
const RightPanel = React.lazy(() =>
  import("./RightPanel").then((module) => ({ default: module.RightPanel })),
);

// Loading fallback component
const RightPanelLoading = () => (
  <Box
    w="320px"
    h="100vh"
    bg="var(--surface-primary)"
    borderLeft="1px solid var(--surface-border)"
    p={4}
    overflowY="auto"
  >
    <VStack gap={4} align="stretch">
      {/* Header skeleton */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Skeleton height="20px" width="60%" mb={2} />
        <Skeleton height="14px" width="80%" />
      </Box>

      {/* Analytics skeleton */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Skeleton height="18px" width="50%" mb={3} />
        <VStack gap={2} align="stretch">
          {Array.from({ length: 4 }).map((_, index) => (
            <HStack key={index} justify="space-between">
              <Skeleton height="14px" width="40%" />
              <Skeleton height="14px" width="20%" />
            </HStack>
          ))}
        </VStack>
      </Box>

      {/* Reading queue skeleton */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Skeleton height="18px" width="60%" mb={3} />
        <VStack gap={3} align="stretch">
          {Array.from({ length: 3 }).map((_, index) => (
            <Box key={index}>
              <Skeleton height="16px" width="90%" mb={1} />
              <Skeleton height="12px" width="60%" />
            </Box>
          ))}
        </VStack>
      </Box>

      {/* Trending topics skeleton */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Skeleton height="18px" width="50%" mb={3} />
        <HStack gap={2} wrap="wrap">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton
              key={index}
              height="24px"
              width="60px"
              borderRadius="full"
            />
          ))}
        </HStack>
      </Box>
    </VStack>
  </Box>
);

// Error boundary fallback component
const RightPanelError = ({
  error,
  resetErrorBoundary,
}: {
  error: Error;
  resetErrorBoundary: () => void;
}) => (
  <Box
    w="320px"
    h="100vh"
    bg="var(--surface-primary)"
    borderLeft="1px solid var(--surface-border)"
    p={4}
    display="flex"
    alignItems="center"
    justifyContent="center"
  >
    <Box
      className="glass"
      p={6}
      borderRadius="var(--radius-lg)"
      textAlign="center"
      maxW="280px"
    >
      <Text fontSize="xl" mb={3}>
        ⚠️
      </Text>
      <Text color="var(--text-primary)" fontSize="md" mb={2}>
        Analytics Unavailable
      </Text>
      <Text color="var(--text-secondary)" fontSize="sm" mb={4}>
        {error.message}
      </Text>
      <button
        onClick={resetErrorBoundary}
        style={{
          padding: "8px 16px",
          borderRadius: "var(--radius-md)",
          backgroundColor: "var(--accent-primary)",
          color: "var(--text-primary)",
          border: "none",
          fontSize: "12px",
          fontWeight: "600",
          cursor: "pointer",
          transition: "all 0.2s ease",
        }}
      >
        Retry
      </button>
    </Box>
  </Box>
);

// Lazy Right Panel component with error boundary
export const LazyRightPanel: React.FC = () => {
  return (
    <ErrorBoundary FallbackComponent={RightPanelError}>
      <Suspense fallback={<RightPanelLoading />}>
        <RightPanel />
      </Suspense>
    </ErrorBoundary>
  );
};

export default LazyRightPanel;
