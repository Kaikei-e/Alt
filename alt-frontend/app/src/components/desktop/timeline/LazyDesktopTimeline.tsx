"use client";

import React, { Suspense } from 'react';
import { Box, Text } from '@chakra-ui/react';
import { ErrorBoundary } from 'react-error-boundary';

// Lazy load the desktop timeline component
const DesktopTimeline = React.lazy(() => import('./DesktopTimeline'));

// Loading fallback component
const DesktopTimelineLoading = () => (
  <Box w="100%" minH="0" flex={1} bg="var(--app-bg)">
    <Box
      display="flex"
      alignItems="center"
      justifyContent="center"
      h="100vh"
      p={4}
    >
      <Box
        className="glass"
        p={8}
        borderRadius="var(--radius-xl)"
        textAlign="center"
        maxW="400px"
      >
        <div
          style={{
            width: "32px",
            height: "32px",
            border: "3px solid var(--surface-border)",
            borderTop: "3px solid var(--accent-primary)",
            borderRadius: "50%",
            animation: "spin 1s linear infinite",
            margin: "0 auto 16px",
          }}
        />
        <Text color="var(--text-primary)" fontSize="lg" mb={2}>
          Loading Desktop Timeline...
        </Text>
        <Text color="var(--text-secondary)" fontSize="sm">
          Preparing your personalized feed experience
        </Text>
      </Box>
    </Box>
  </Box>
);

// Error boundary fallback component
const DesktopTimelineError = ({ error, resetErrorBoundary }: { error: Error; resetErrorBoundary: () => void }) => (
  <Box w="100%" minH="0" flex={1} bg="var(--app-bg)">
    <Box
      display="flex"
      alignItems="center"
      justifyContent="center"
      h="100vh"
      p={4}
    >
      <Box
        className="glass"
        p={8}
        borderRadius="var(--radius-xl)"
        textAlign="center"
        maxW="400px"
      >
        <Text fontSize="2xl" mb={4}>⚠️</Text>
        <Text color="var(--text-primary)" fontSize="lg" mb={2}>
          Failed to load timeline
        </Text>
        <Text color="var(--text-secondary)" fontSize="sm" mb={4}>
          {error.message}
        </Text>
        <button
          onClick={resetErrorBoundary}
          style={{
            padding: "10px 20px",
            borderRadius: "var(--radius-md)",
            backgroundColor: "var(--accent-primary)",
            color: "var(--text-primary)",
            border: "none",
            fontSize: "14px",
            fontWeight: "600",
            cursor: "pointer",
            transition: "all 0.2s ease",
          }}
        >
          Try Again
        </button>
      </Box>
    </Box>
  </Box>
);

// Lazy Desktop Timeline component with error boundary
export const LazyDesktopTimeline: React.FC = () => {
  return (
    <ErrorBoundary FallbackComponent={DesktopTimelineError}>
      <Suspense fallback={<DesktopTimelineLoading />}>
        <DesktopTimeline />
      </Suspense>
    </ErrorBoundary>
  );
};

export default LazyDesktopTimeline;