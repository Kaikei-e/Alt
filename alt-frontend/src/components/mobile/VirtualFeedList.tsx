"use client";

import { Flex } from "@chakra-ui/react";
import type { Feed } from "@/schema/feed";
import FeedCard from "./FeedCard";

interface VirtualFeedListProps {
  /**
   * Feed items that should be rendered (already filtered for read status).
   */
  feeds: Feed[];
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
    <Flex direction="column" gap={4} data-testid="virtual-feed-list">
      {feeds.map((feed: Feed, index: number) => (
        <div key={feed.link} data-testid={`virtual-feed-item-${index}`}>
          <FeedCard
            feed={feed}
            isReadStatus={readFeeds.has(feed.link)}
            setIsReadStatus={() => onMarkAsRead(feed.link)}
          />
        </div>
      ))}
    </Flex>
  );
};

export default VirtualFeedList;
