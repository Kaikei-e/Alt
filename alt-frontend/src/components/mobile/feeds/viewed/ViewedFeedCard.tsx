import { Box, Flex, Text, Button, HStack, Badge } from "@chakra-ui/react";
import { animated } from "@react-spring/web";
import { Archive, ExternalLink, BookOpen } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { truncateFeedDescription } from "@/lib/utils/textUtils";
import type { RenderFeed } from "@/schema/feed";
import { articleApi } from "@/lib/api";
import { ViewedFeedDetailsModal } from "./ViewedFeedDetailsModal";

type ViewedFeedCardProps = {
  feed: RenderFeed;
};

const AnimatedBox = animated(Box);

export const ViewedFeedCard = ({ feed, style }: ViewedFeedCardProps & { style?: any }) => {
  const [isArchiving, setIsArchiving] = useState(false);
  const [isArchived, setIsArchived] = useState(false);
  const [isModalOpen, setIsModalOpen] = useState(false);
  // Note: feed.excerpt is already generated server-side, so we don't need truncatedDescription
  // Keeping it for backward compatibility but it should not be used
  const _truncatedDescription = truncateFeedDescription(feed.description);

  const handleArchive = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!feed.link || isArchiving || isArchived) return;

    try {
      setIsArchiving(true);
      await articleApi.archiveContent(feed.link, feed.title);
      setIsArchived(true);
    } catch (error) {
      console.error("Failed to archive:", error);
    } finally {
      setIsArchiving(false);
    }
  };

  return (
    <AnimatedBox
      style={{
        ...style,
        width: "100%",
      }}
    >
      <Box
        p={4}
        borderRadius="1rem"
        bg="var(--alt-glass)"
        backdropFilter="blur(20px)"
        border="2px solid var(--alt-glass-border)"
        boxShadow="0 12px 40px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.1)"
        position="relative"
        overflow="hidden"
        _hover={{
          border: "2px solid var(--alt-glass-border)",
          boxShadow: "0 12px 40px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.15)",
          transform: "translateY(-2px)",
        }}
        transition="all 0.3s ease"
      >

        <Flex direction="column" gap={3}>
          {/* Header: Title and Badge */}
          <Flex justify="space-between" align="flex-start" gap={3}>
            <Link href={feed.normalizedUrl} target="_blank" rel="noopener noreferrer" style={{ flex: 1 }}>
              <Text
                fontSize="md"
                fontWeight="bold"
                color="var(--alt-text-primary)"
                lineHeight="1.4"
                fontFamily="var(--font-outfit)"
                css={{
                  display: "-webkit-box",
                  WebkitLineClamp: 2,
                  WebkitBoxOrient: "vertical",
                  overflow: "hidden",
                }}
              >
                {feed.title}
              </Text>
            </Link>
            <Badge
              colorScheme="purple"
              variant="subtle"
              bg="rgba(255, 255, 255, 0.1)"
              color="var(--alt-text-secondary)"
              px={2}
              py={0.5}
              borderRadius="full"
              fontSize="xs"
              textTransform="none"
              whiteSpace="nowrap"
              border="1px solid var(--alt-glass-border)"
            >
              Read
            </Badge>
          </Flex>

          {/* Description - Use server-generated excerpt if available, otherwise fallback to truncated description */}
          <Text
            fontSize="sm"
            color="var(--alt-text-secondary)"
            lineHeight="1.5"
            css={{
              display: "-webkit-box",
              WebkitLineClamp: 3,
              WebkitBoxOrient: "vertical",
              overflow: "hidden",
            }}
          >
            {feed.excerpt}
          </Text>

          {/* Footer: Actions and Meta */}
          <Flex justify="space-between" align="center" mt={1} gap={2} flexWrap="wrap">
            <Button
              size="xs"
              variant="ghost"
              color="var(--alt-text-secondary)"
              _hover={{ bg: "rgba(255, 255, 255, 0.05)", color: "var(--alt-text-primary)" }}
              onClick={(e) => {
                e.stopPropagation();
                setIsModalOpen(true);
              }}
            >
              <Flex align="center" gap={1}>
                <BookOpen size={14} />
                <Text>Details</Text>
              </Flex>
            </Button>

            <HStack gap={2}>
              <Button
                size="xs"
                variant="ghost"
                color="var(--alt-text-secondary)"
                _hover={{ bg: "rgba(255, 255, 255, 0.05)", color: "var(--alt-text-primary)" }}
                onClick={handleArchive}
                disabled={isArchived}
              >
                <Flex align="center" gap={1}>
                  {!isArchived && <Archive size={14} />}
                  <Text>{isArchiving ? "..." : isArchived ? "Archived" : "Archive"}</Text>
                </Flex>
              </Button>

              <Link href={feed.normalizedUrl} target="_blank" rel="noopener noreferrer">
                <Button
                  size="xs"
                  variant="ghost"
                  color="var(--alt-text-secondary)"
                  _hover={{ bg: "rgba(255, 255, 255, 0.05)", color: "var(--alt-text-primary)" }}
                >
                  <Flex align="center" gap={1}>
                    <Text>Open</Text>
                    <ExternalLink size={14} />
                  </Flex>
                </Button>
              </Link>
            </HStack>
          </Flex>
        </Flex>
      </Box>

      {/* Modal */}
      <ViewedFeedDetailsModal
        feedURL={feed.link}
        feedTitle={feed.title}
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
      />
    </AnimatedBox>
  );
};
