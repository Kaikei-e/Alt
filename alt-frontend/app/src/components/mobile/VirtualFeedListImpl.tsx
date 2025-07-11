"use client";

import React, { useState, useEffect, useMemo } from 'react';
import { useWindowSize } from '@/hooks/useWindowSize';
import { VirtualFeedListCore } from './VirtualFeedListCore';
import { Feed } from '@/schema/feed';

interface VirtualFeedListImplProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
}

export const VirtualFeedListImpl: React.FC<VirtualFeedListImplProps> = ({
  feeds,
  readFeeds,
  onMarkAsRead
}) => {
  const { height: windowHeight } = useWindowSize();
  const [estimatedItemHeight, setEstimatedItemHeight] = useState(200);

  // 動的に容器高さを計算
  const containerHeight = useMemo(() => {
    // ヘッダー、フッター、パディングを考慮
    const headerHeight = 60;
    const footerHeight = 80;
    const padding = 40;
    return Math.max(400, windowHeight - headerHeight - footerHeight - padding);
  }, [windowHeight]);

  // アイテム高さの自動調整（固定サイズモード）
  useEffect(() => {
    // フィードの内容に基づいて推定高さを調整
    const avgDescriptionLength = feeds.reduce((sum, feed) => 
      sum + feed.description.length, 0
    ) / feeds.length;

    // 説明文の長さに基づいて高さを調整
    const baseHeight = 120; // 最小高さ
    const additionalHeight = Math.min(avgDescriptionLength / 4, 100); // 最大100px追加
    
    setEstimatedItemHeight(baseHeight + additionalHeight);
  }, [feeds]);

  return (
    <VirtualFeedListCore
      feeds={feeds}
      readFeeds={readFeeds}
      onMarkAsRead={onMarkAsRead}
      estimatedItemHeight={estimatedItemHeight}
      containerHeight={containerHeight}
      overscan={5}
    />
  );
};