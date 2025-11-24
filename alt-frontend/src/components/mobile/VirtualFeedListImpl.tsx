"use client";

import type React from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useWindowSize } from "@/hooks/useWindowSize";
import type { RenderFeed } from "@/schema/feed";
import { FeatureFlagManager } from "@/utils/featureFlags";
import { DynamicVirtualFeedList } from "./DynamicVirtualFeedList";
import { VirtualFeedListCore } from "./VirtualFeedListCore";

interface VirtualFeedListImplProps {
  feeds: RenderFeed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
}

export const VirtualFeedListImpl: React.FC<VirtualFeedListImplProps> = ({
  feeds,
  readFeeds,
  onMarkAsRead,
}) => {
  const { height: windowHeight } = useWindowSize();
  const [useDynamicSizing, setUseDynamicSizing] = useState(false);
  const [dynamicSizingError, setDynamicSizingError] = useState(false);

  // 動的サイズ調整の有効/無効判定
  useEffect(() => {
    const flags = FeatureFlagManager.getInstance().getFlags();
    const enableDynamic =
      flags.enableDynamicSizing !== false && !dynamicSizingError;

    // フィード数が少ない場合は動的サイズ調整を無効化
    if (feeds.length < 100) {
      setUseDynamicSizing(false);
      return;
    }

    // コンテンツの変動が大きい場合のみ動的サイズ調整を有効化
    const hasVariableContent = feeds.some(
      (feed) => feed.description.length > 500 || feed.title.length > 100,
    );

    setUseDynamicSizing(enableDynamic && hasVariableContent);
  }, [feeds, dynamicSizingError]);

  // 動的サイズ調整エラー処理
  const handleDynamicSizingError = useCallback((error: Error) => {
    console.error("Dynamic sizing error:", error);
    setDynamicSizingError(true);
    setUseDynamicSizing(false);

    // フィーチャーフラグを無効化
    FeatureFlagManager.getInstance().updateFlags({
      enableDynamicSizing: false,
    });
  }, []);

  // 動的に容器高さを計算
  const containerHeight = useMemo(() => {
    const headerHeight = 60;
    const footerHeight = 80;
    const padding = 40;
    return Math.max(400, windowHeight - headerHeight - footerHeight - padding);
  }, [windowHeight]);

  // Dynamic Sizing使用時
  if (useDynamicSizing) {
    return (
      <DynamicVirtualFeedList
        feeds={feeds}
        readFeeds={readFeeds}
        onMarkAsRead={onMarkAsRead}
        containerHeight={containerHeight}
        overscan={3} // Dynamic Sizingでは少なめに設定
        onMeasurementError={handleDynamicSizingError}
      />
    );
  }

  // 固定サイズ使用時
  return (
    <VirtualFeedListCore
      feeds={feeds}
      readFeeds={readFeeds}
      onMarkAsRead={onMarkAsRead}
      estimatedItemHeight={estimateItemHeight(feeds)}
      containerHeight={containerHeight}
      overscan={5}
    />
  );
};

// 固定サイズ推定ヘルパー関数
function estimateItemHeight(feeds: RenderFeed[]): number {
  if (feeds.length === 0) return 200;

  const avgDescriptionLength =
    feeds.reduce((sum, feed) => sum + feed.description.length, 0) /
    feeds.length;

  const baseHeight = 120;
  const additionalHeight = Math.min(avgDescriptionLength / 4, 100);

  return baseHeight + additionalHeight;
}
