import { Button, Flex, Spinner, Text, Box } from "@chakra-ui/react";
import { Feed } from "@/schema/feed";
import Link from "next/link";
import { feedsApi } from "@/lib/api";
import { useState, useCallback, memo, KeyboardEvent } from "react";
import { FeedDetails } from "./FeedDetails";
import { truncateFeedDescription } from "@/lib/utils/textUtils";

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
      } catch (error) {
        console.error("Failed to mark feed as read:", error);
      } finally {
        setIsLoading(false);
      }
    },
    [setIsReadStatus],
  );

  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        handleReadStatus(feed.link);
      }
    },
    [feed.link, handleReadStatus],
  );

  // Don't render if already read
  if (isReadStatus) {
    return null;
  }

  // Truncate description for better UX
  const truncatedDescription = truncateFeedDescription(feed.description);

  return (
    // Gradient border container with hover effects
    <Box
      p="2px"
      borderRadius="18px"
      background="linear-gradient(45deg, rgb(255, 0, 110), rgb(131, 56, 236), rgb(58, 134, 255))"
      transition="transform 0.3s ease-in-out, box-shadow 0.3s ease-in-out"
      _hover={{
        transform: "translateY(-2px)",
        boxShadow: "0 20px 40px rgba(255, 0, 110, 0.3)",
      }}
      cursor="pointer"
      data-testid="feed-card-container"
    >
      <Box
        className="glass"
        w="full"
        p={5}
        borderRadius="16px"
        data-testid="feed-card"
        role="article"
        tabIndex={0}
        onKeyDown={handleKeyDown}
        aria-label={`Feed: ${feed.title}`}
        bg="var(--alt-bg-primary, #1a1a2e)"
      >
        <Flex direction="column" gap={2}>
          {/* Title as link */}
          <Link
            href={feed.link}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={`Open ${feed.title} in external link`}
          >
            <Text
              fontSize="lg"
              fontWeight="semibold"
              color="#ff006e"
              _hover={{ textDecoration: "underline" }}
              lineHeight="1.4"
            >
              {feed.title}
            </Text>
          </Link>

          {/* Description */}
          <Text fontSize="sm" color="rgba(255, 255, 255, 0.8)" lineHeight="1.5">
            {truncatedDescription}
          </Text>

          {/* Bottom section with button and details */}
          <Flex justify="space-between" align="center" mt={2}>
            <Button
              size="sm"
              borderRadius="full"
              bg="linear-gradient(45deg, #ff006e, #8338ec)"
              color="white"
              fontWeight="bold"
              px={4}
              minHeight="44px"
              minWidth="120px"
              fontSize="sm"
              border="1px solid rgba(255, 255, 255, 0.2)"
              disabled={isLoading}
              onClick={() => handleReadStatus(feed.link)}
              _hover={{
                bg: "linear-gradient(45deg, #e6005c, #7129d4)",
                transform: "scale(1.05)",
                boxShadow: "0 4px 12px rgba(255, 0, 110, 0.4)",
              }}
              _active={{
                transform: "scale(0.98)",
              }}
              _disabled={{
                opacity: 0.6,
                cursor: "not-allowed",
              }}
              transition="all 0.2s ease"
              aria-label={`Mark ${feed.title} as read`}
            >
              {isLoading ? <Spinner size="sm" /> : "Mark as read"}
            </Button>

            <FeedDetails feedURL={feed.link} />
          </Flex>
        </Flex>
      </Box>
    </Box>
  );
});

export default FeedCard;
