import { Box, Flex, Text, Button, HStack, Badge } from "@chakra-ui/react";
import { motion } from "framer-motion";
import { Archive, ExternalLink } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { truncateFeedDescription } from "@/lib/utils/textUtils";
import type { Feed } from "@/schema/feed";
import { articleApi } from "@/lib/api";

type ViewedFeedCardProps = {
  feed: Feed;
};

const MotionBox = motion(Box);

export const ViewedFeedCard = ({ feed }: ViewedFeedCardProps) => {
  const [isArchiving, setIsArchiving] = useState(false);
  const [isArchived, setIsArchived] = useState(false);
  const truncatedDescription = truncateFeedDescription(feed.description);

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
    <MotionBox
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -20 }}
      transition={{ duration: 0.3 }}
      width="100%"
    >
      <Box
        p={4}
        borderRadius="24px"
        bg="rgba(30, 30, 40, 0.7)"
        backdropFilter="blur(20px)"
        border="1px solid rgba(255, 255, 255, 0.1)"
        boxShadow="0 4px 16px 0 rgba(0, 0, 0, 0.2)"
        position="relative"
        overflow="hidden"
        _hover={{
          border: "1px solid rgba(255, 255, 255, 0.2)",
          boxShadow: "0 8px 24px 0 rgba(0, 0, 0, 0.3)",
          transform: "translateY(-2px)",
        }}
        transition="all 0.3s ease"
      >
        {/* Glass reflection effect */}
        <Box
          position="absolute"
          top="0"
          left="0"
          right="0"
          height="40%"
          bg="linear-gradient(180deg, rgba(255,255,255,0.05) 0%, rgba(255,255,255,0) 100%)"
          pointerEvents="none"
        />

        <Flex direction="column" gap={3}>
          {/* Header: Title and Badge */}
          <Flex justify="space-between" align="flex-start" gap={3}>
            <Link href={feed.link} target="_blank" rel="noopener noreferrer" style={{ flex: 1 }}>
              <Text
                fontSize="md"
                fontWeight="bold"
                color="white"
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
              bg="rgba(138, 75, 255, 0.2)"
              color="#D0BCFF"
              px={2}
              py={0.5}
              borderRadius="full"
              fontSize="xs"
              textTransform="none"
              whiteSpace="nowrap"
            >
              Read
            </Badge>
          </Flex>

          {/* Description */}
          <Text
            fontSize="sm"
            color="rgba(255, 255, 255, 0.7)"
            lineHeight="1.5"
            css={{
              display: "-webkit-box",
              WebkitLineClamp: 3,
              WebkitBoxOrient: "vertical",
              overflow: "hidden",
            }}
          >
            {truncatedDescription}
          </Text>

          {/* Footer: Actions and Meta */}
          <Flex justify="space-between" align="center" mt={1}>
            <HStack gap={2}>
              <Button
                size="xs"
                variant="ghost"
                color="rgba(255, 255, 255, 0.6)"
                _hover={{ bg: "rgba(255, 255, 255, 0.1)", color: "white" }}
                onClick={handleArchive}
                disabled={isArchived}
              >
                <Flex align="center" gap={1}>
                  {!isArchived && <Archive size={14} />}
                  <Text>{isArchiving ? "..." : isArchived ? "Archived" : "Archive"}</Text>
                </Flex>
              </Button>
            </HStack>

            <Link href={feed.link} target="_blank" rel="noopener noreferrer">
              <Button
                size="xs"
                variant="ghost"
                color="rgba(255, 255, 255, 0.6)"
                _hover={{ bg: "rgba(255, 255, 255, 0.1)", color: "white" }}
              >
                <Flex align="center" gap={1}>
                  <Text>Open</Text>
                  <ExternalLink size={14} />
                </Flex>
              </Button>
            </Link>
          </Flex>
        </Flex>
      </Box>
    </MotionBox>
  );
};
