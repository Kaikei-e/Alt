import { Button, Flex, Spinner, Text, Box } from "@chakra-ui/react";
import { Feed } from "@/schema/feed";
import Link from "next/link";
import { feedsApi } from "@/lib/api";
import { useState, useCallback, memo } from "react";

type FeedCardProps = {
  feed: Feed;
  isReadStatus: boolean;
  setIsReadStatus: (isReadStatus: boolean) => void;
};

const FeedCard = memo(function FeedCard({
  feed,
  isReadStatus,
  setIsReadStatus,
}: FeedCardProps) {
  const [isLoading, setIsLoading] = useState(false);

  const handleReadStatus = useCallback(
    async (url: string) => {
      try {
        setIsLoading(true);
        await feedsApi.updateFeedReadStatus(url);
        setIsReadStatus(true);
        setIsLoading(false);
      } catch (error) {
        console.error("Error updating feed read status", error);
        setIsLoading(false);
      }
    },
    [setIsReadStatus],
  );

  const onMarkAsRead = useCallback(() => {
    handleReadStatus(feed.link);
  }, [handleReadStatus, feed.link]);

  if (isLoading) {
    return (
      <Flex justify="center" align="center" p={8}>
        <Spinner
          size="lg"
          color="pink.400"
        />
      </Flex>
    );
  }

  if (isReadStatus) {
    return null;
  }

  // Truncate description more efficiently
  const truncatedDescription =
    feed.description.length > 300
      ? `${feed.description.slice(0, 300)}...`
      : feed.description;

  return (
    <Box
      width="100%"
      p={5}
      data-testid="feed-card"
    >
      <Flex
        flexDirection="column"
        gap={4}
      >
        {/* Title */}
        <Text
          fontSize="lg"
          fontWeight="bold"
          color="#ff006e"
          lineHeight="1.3"
        >
          <Link href={feed.link} target="_blank">
            {feed.title}
          </Link>
        </Text>

        {/* Description */}
        <Text
          fontSize="sm"
          color="rgba(255, 255, 255, 0.8)"
          lineHeight="1.5"
        >
          {truncatedDescription}
        </Text>

        {/* Bottom section with button and date */}
        <Flex
          flexDirection="row"
          justifyContent="space-between"
          alignItems="center"
          pt={2}
        >
          <Button
            onClick={onMarkAsRead}
            size="sm"
            borderRadius="full"
            bg="linear-gradient(45deg, #ff006e, #8338ec)"
            color="white"
            fontWeight="bold"
            px={4}
            disabled={isLoading}
            _hover={{
              bg: "linear-gradient(45deg, #e6005c, #7129d4)",
              transform: "translateY(-1px)",
            }}
            _active={{
              transform: "translateY(0px)",
            }}
            _disabled={{
              opacity: 0.6,
            }}
            transition="all 0.2s ease"
            border="1px solid rgba(255, 255, 255, 0.2)"
          >
            Mark as read
          </Button>

          <Text
            fontSize="xs"
            color="rgba(255, 255, 255, 0.6)"
            fontWeight="medium"
            textAlign="right"
            ml={3}
          >
            {feed.published}
          </Text>
        </Flex>
      </Flex>
    </Box>
  );
});

export default FeedCard;
