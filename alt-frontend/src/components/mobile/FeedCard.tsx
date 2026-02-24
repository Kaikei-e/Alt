import { Box, Button, Flex, Text } from "@chakra-ui/react";
import { SquareArrowOutUpRight } from "lucide-react";
import Link from "next/link";
import { type KeyboardEvent, useCallback, useMemo } from "react";
import { truncateFeedDescription } from "@/lib/utils/textUtils";
import type { RenderFeed } from "@/schema/feed";
import { FeedDetails } from "./FeedDetails";

type FeedCardProps = {
  feed: RenderFeed;
  isReadStatus: boolean;
  setIsReadStatus: (feedLink: string) => void; // 親のonMarkAsReadを呼び出すため、feedLinkを渡す
};

const FeedCard = function FeedCard({
  feed,
  isReadStatus,
  setIsReadStatus,
}: FeedCardProps) {
  // Memoize expensive description truncation calculation
  const _truncatedDescription = useMemo(
    () => truncateFeedDescription(feed.description),
    [feed.description],
  );

  const handleReadStatus = useCallback(
    (url: string) => {
      // API呼び出しは親コンポーネントで行うため、コールバックのみ実行
      setIsReadStatus(url);
      // setIsReadStatusは親のonMarkAsReadを呼び出す
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

  return (
    // Gradient border container with hover effects
    <Box
      p="2px"
      borderRadius="18px"
      /* Use solid border instead of gradient */
      border="2px solid var(--surface-border)"
      transition="transform 0.3s ease-in-out, box-shadow 0.3s ease-in-out"
      cursor="pointer"
      data-testid="feed-card-container"
    >
      <Box
        className="glass"
        w="full"
        p={4}
        borderRadius="16px"
        data-testid="feed-card"
        role="article"
        tabIndex={0}
        onKeyDown={handleKeyDown}
        aria-label={`Feed: ${feed.title}`}
        bg="var(--surface-bg)"
      >
        <Flex direction="column" gap={2}>
          {/* Title as link */}
          <Flex direction="row" align="center" gap={2}>
            <Box
              display="flex"
              alignItems="center"
              justifyContent="center"
              width="24px"
              height="24px"
              flexShrink={0}
              data-testid={`feed-link-icon-${feed.id}`}
            >
              <SquareArrowOutUpRight color="var(--alt-primary)" size={16} />
            </Box>
            <Link
              href={feed.normalizedUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={`Open ${feed.title} in external link`}
            >
              <Text
                fontSize="sm"
                fontWeight="semibold"
                color="var(--accent-primary)"
                _hover={{ textDecoration: "underline" }}
                lineHeight="1.4"
                wordBreak="break-word"
              >
                {feed.title}
              </Text>
            </Link>
          </Flex>

          {/* Description - Use server-generated excerpt if available, otherwise fallback to truncated description */}
          <Text
            fontSize="xs"
            color="var(--text-primary)"
            lineHeight="1.5"
            wordBreak="break-word"
            css={{
              display: "-webkit-box",
              WebkitLineClamp: 3,
              WebkitBoxOrient: "vertical",
              overflow: "hidden",
            }}
          >
            {feed.excerpt}
          </Text>

          {/* Author name (if available) */}
          {feed.author && (
            <Text
              fontSize="xs"
              color="var(--text-secondary)"
              fontStyle="italic"
            >
              by {feed.author}
            </Text>
          )}

          {/* Bottom section with button and details */}
          <Flex justify="space-between" align="center" mt={3} gap={3}>
            <Button
              flex={1}
              size="sm"
              borderRadius="full"
              bg="var(--alt-primary)"
              color="var(--text-primary)"
              fontWeight="bold"
              px={4}
              minHeight="44px"
              fontSize="sm"
              border="1px solid rgba(255, 255, 255, 0.2)"
              onClick={() => handleReadStatus(feed.link)}
              _hover={{
                bg: "var(--accent-gradient)",
                transform: "scale(1.05)",
                boxShadow: "0 4px 12px var(--accent-primary)",
              }}
              _active={{
                transform: "scale(0.98)",
              }}
              transition="all 0.2s ease"
              aria-label={`Mark ${feed.title} as read`}
              data-testid="mark-as-read-button"
            >
              Mark as read
            </Button>

            <FeedDetails feedURL={feed.link} feedTitle={feed.title} />
          </Flex>
        </Flex>
      </Box>
    </Box>
  );
};

export default FeedCard;
