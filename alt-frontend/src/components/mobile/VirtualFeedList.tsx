"use client";

import { Flex } from "@chakra-ui/react";
import type { RenderFeed } from "@/schema/feed";
import FeedCard from "./FeedCard";

interface VirtualFeedListProps {
  /**
   * Feed items that should be rendered (already filtered for read status).
   */
  feeds: RenderFeed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
}

export const VirtualFeedList: React.FC<VirtualFeedListProps> = ({
  feeds,
  readFeeds,
  onMarkAsRead,
}) => {
  if (feeds.length === 0) {
    return null;
  }

  return (
    <Flex
      direction="column"
      gap={4}
      data-testid="virtual-feed-list"
      mt={2}
      style={
        {
          contentVisibility: "auto",
          containIntrinsicSize: "800px",
        } as React.CSSProperties
      }
    >
      {feeds.map((feed: RenderFeed, index: number) => (
        <div key={feed.link} data-testid={`virtual-feed-item-${index}`}>
          <FeedCard
            feed={feed}
            isReadStatus={readFeeds.has(feed.normalizedUrl)}
            setIsReadStatus={() => onMarkAsRead(feed.normalizedUrl)}
          />
        </div>
      ))}
    </Flex>
  );
};

export default VirtualFeedList;
