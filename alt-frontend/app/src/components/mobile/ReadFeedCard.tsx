import { Flex, Text, Box } from "@chakra-ui/react";
import { Feed } from "@/schema/feed";
import Link from "next/link";
import { memo } from "react";
import { FeedDetails } from "./FeedDetails";
import { truncateFeedDescription } from "@/lib/utils/textUtils";

type ReadFeedCardProps = {
  feed: Feed;
};

const ReadFeedCard = memo(function ReadFeedCard({ feed }: ReadFeedCardProps) {
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
      data-testid="read-feed-card-container"
    >
      <Box
        className="glass"
        w="full"
        p={5}
        borderRadius="16px"
        data-testid="read-feed-card"
        role="article"
        aria-label={`Read feed: ${feed.title}`}
        bg="var(--alt-bg-primary, #1a1a2e)"
      >
        <Flex direction="column" gap={2}>
          {/* Title as link */}
          <Link
            href={feed.link}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={`Open read feed ${feed.title} in external link`}
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

          {/* Bottom section with read status and details */}
          <Flex justify="space-between" align="center" mt={2}>
            <Box
              px={4}
              py={2}
              borderRadius="full"
              bg="rgba(255, 255, 255, 0.1)"
              border="1px solid rgba(255, 255, 255, 0.2)"
              minHeight="44px"
              minWidth="120px"
              display="flex"
              alignItems="center"
              justifyContent="center"
            >
              <Text
                fontSize="sm"
                color="rgba(255, 255, 255, 0.7)"
                fontWeight="medium"
              >
                Already Read
              </Text>
            </Box>

            <FeedDetails feedURL={feed.link} />
          </Flex>
        </Flex>
      </Box>
    </Box>
  );
});

export default ReadFeedCard;