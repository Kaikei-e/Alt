"use client";

import type React from "react";
import { useEffect, useState } from "react";
import { ErrorBoundary } from "react-error-boundary";
import type { Feed } from "@/schema/feed";
import {
  FeatureFlagManager,
  shouldUseVirtualization,
} from "@/utils/featureFlags";
import DesktopTimeline from "./DesktopTimeline";
import { VirtualDesktopTimeline } from "./VirtualDesktopTimeline";

interface VirtualDesktopTimelineImplProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedId: string) => void;
  onToggleFavorite: (feedId: string) => void;
  onToggleBookmark: (feedId: string) => void;
  onReadLater: (feedId: string) => void;
  onViewArticle: (feedId: string) => void;
}

const DesktopTimelineFallback: React.FC<
  VirtualDesktopTimelineImplProps
> = () => {
  return <DesktopTimeline />;
};

export const VirtualDesktopTimelineImpl: React.FC<
  VirtualDesktopTimelineImplProps
> = (props) => {
  const [useVirtualization, setUseVirtualization] = useState(false);
  const [containerHeight, setContainerHeight] = useState(800);
  const [virtualizationError, setVirtualizationError] = useState(false);
  const { feeds } = props;

  // Dynamic container height calculation
  useEffect(() => {
    const calculateHeight = () => {
      const headerHeight = 80;
      const footerHeight = 60;
      const padding = 40;
      const calculatedHeight = Math.max(
        600,
        window.innerHeight - headerHeight - footerHeight - padding,
      );
      setContainerHeight(calculatedHeight);
    };

    calculateHeight();
    window.addEventListener("resize", calculateHeight);
    return () => window.removeEventListener("resize", calculateHeight);
  }, []);

  // Virtualization enable/disable logic
  useEffect(() => {
    const flags = FeatureFlagManager.getInstance().getFlags();
    const desktopVirtualizationEnabled =
      flags.enableDesktopVirtualization !== false;
    const shouldVirtualize =
      shouldUseVirtualization(feeds.length, flags) &&
      desktopVirtualizationEnabled &&
      !virtualizationError;

    // Enable virtualization for desktop when >= 100 items
    setUseVirtualization(shouldVirtualize && feeds.length >= 100);
  }, [feeds.length, virtualizationError]);

  // Error handling
  const handleVirtualizationError = (error: Error) => {
    console.error("Desktop virtualization error:", error);
    setVirtualizationError(true);
    setUseVirtualization(false);

    // Disable desktop virtualization in feature flags
    FeatureFlagManager.getInstance().updateFlags({
      enableDesktopVirtualization: false,
    });
  };

  // Use fallback when virtualization is disabled
  if (!useVirtualization) {
    return <DesktopTimelineFallback {...props} />;
  }

  // Use virtualization with error boundary
  return (
    <ErrorBoundary
      FallbackComponent={({ error }) => {
        handleVirtualizationError(error);
        return <DesktopTimelineFallback {...props} />;
      }}
      onError={handleVirtualizationError}
    >
      <VirtualDesktopTimeline
        {...props}
        containerHeight={containerHeight}
        enableDynamicSizing={true}
        overscan={2}
      />
    </ErrorBoundary>
  );
};
