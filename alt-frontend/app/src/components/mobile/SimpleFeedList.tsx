"use client";

import { Flex } from '@chakra-ui/react';
import { Feed } from '@/schema/feed';
import FeedCard from './FeedCard';

interface SimpleFeedListProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
}

export const SimpleFeedList: React.FC<SimpleFeedListProps> = ({
  feeds,
  readFeeds,
  onMarkAsRead,
}) => {
  if (feeds.length === 0) {
    return null;
  }

  return (
    <Flex direction="column" gap={4} data-testid="feed-list-fallback">
      {feeds.map((feed: Feed) => (
        <FeedCard
          key={feed.link}
          feed={feed}
          isReadStatus={readFeeds.has(feed.link)}
          setIsReadStatus={() => onMarkAsRead(feed.link)}
        />
      ))}
    </Flex>
  );
};

export default SimpleFeedList;