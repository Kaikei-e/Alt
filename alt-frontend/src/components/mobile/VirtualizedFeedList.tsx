"use client";

import { Box, Text } from "@chakra-ui/react";
import type React from "react";
import { useCallback, useEffect, useState } from "react";
import { ErrorBoundary } from "react-error-boundary";
import type { Feed } from "@/schema/feed";
import {
  FeatureFlagManager,
  shouldUseVirtualization,
} from "@/utils/featureFlags";
import { SimpleFeedList } from "./SimpleFeedList";
import { VirtualFeedListImpl } from "./VirtualFeedListImpl";

interface VirtualizedFeedListProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
}

// Error boundary fallback component
const VirtualizationErrorFallback: React.FC<{
  error: Error;
  resetErrorBoundary: () => void;
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
}> = ({ error, feeds, readFeeds, onMarkAsRead }) => {
  useEffect(() => {
    console.error(
      "Virtualization failed, falling back to simple rendering:",
      error,
    );

    // Disable virtualization in feature flags
    FeatureFlagManager.getInstance().updateFlags({
      enableVirtualization: false,
    });
  }, [error]);

  return (
    <Box>
      {process.env.NODE_ENV === "development" && (
        <Box
          bg="red.100"
          border="1px solid"
          borderColor="red.300"
          p={2}
          mb={4}
          borderRadius="md"
        >
          <Text fontSize="sm" color="red.800">
            仮想化でエラーが発生しました。通常表示にフォールバックします。
          </Text>
          <Text fontSize="xs" color="red.600">
            {error.message}
          </Text>
        </Box>
      )}
      <SimpleFeedList
        feeds={feeds}
        readFeeds={readFeeds}
        onMarkAsRead={onMarkAsRead}
      />
    </Box>
  );
};

export const VirtualizedFeedList: React.FC<VirtualizedFeedListProps> = (
  props,
) => {
  const [useVirtualization, setUseVirtualization] = useState(false);
  const [errorCount, setErrorCount] = useState(0);
  const { feeds } = props;

  useEffect(() => {
    const flags = FeatureFlagManager.getInstance().getFlags();
    const shouldVirtualize = shouldUseVirtualization(feeds.length, flags);

    // Error count limit - disable after 3 consecutive errors
    if (errorCount >= 3) {
      setUseVirtualization(false);
      return;
    }

    setUseVirtualization(shouldVirtualize);
  }, [feeds.length, errorCount]);

  const handleVirtualizationError = useCallback(() => {
    setErrorCount((prev) => prev + 1);
    setUseVirtualization(false);
  }, []);

  // Use simple implementation when virtualization is disabled
  if (!useVirtualization) {
    return <SimpleFeedList {...props} />;
  }

  // Use virtualization with error boundary
  return (
    <ErrorBoundary
      FallbackComponent={(errorProps) => (
        <VirtualizationErrorFallback {...errorProps} {...props} />
      )}
      onError={handleVirtualizationError}
      onReset={() => setErrorCount(0)}
    >
      <VirtualFeedListImpl {...props} />
    </ErrorBoundary>
  );
};

export default VirtualizedFeedList;
