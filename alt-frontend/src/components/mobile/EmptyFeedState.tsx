"use client";

import { Box, Button, Flex, Text, VStack } from "@chakra-ui/react";
import { Plus, Rss, Sparkles } from "lucide-react";
import Link from "next/link";

/**
 * EmptyFeedState component displays a user-friendly empty state
 * when there are no feeds available. It provides clear guidance
 * and a call-to-action to register the first feed.
 */
export default function EmptyFeedState() {
  return (
    <Flex
      role="region"
      aria-label="Empty feed state"
      direction="column"
      justify="center"
      align="center"
      minH="70vh"
      p={6}
      textAlign="center"
    >
      <VStack gap={8} maxW="400px">
        {/* Animated Icon */}
        <Box
          data-testid="empty-state-icon"
          position="relative"
          w="120px"
          h="120px"
          display="flex"
          alignItems="center"
          justifyContent="center"
        >
          {/* Background circle with gradient */}
          <Box
            position="absolute"
            w="100%"
            h="100%"
            bg="var(--alt-glass)"
            borderRadius="full"
            border="2px solid var(--alt-glass-border)"
            css={{
              "@keyframes pulse": {
                "0%, 100%": {
                  transform: "scale(1)",
                  opacity: 0.8,
                },
                "50%": {
                  transform: "scale(1.05)",
                  opacity: 1,
                },
              },
              animation: "pulse 3s ease-in-out infinite",
            }}
          />

          {/* RSS Icon */}
          <Box position="relative" zIndex={1}>
            <Rss
              size={56}
              color="var(--alt-text-secondary)"
              strokeWidth={1.5}
            />
          </Box>

          {/* Sparkle decorations */}
          <Box
            position="absolute"
            top="10px"
            right="10px"
            color="var(--accent-primary)"
            css={{
              "@keyframes twinkle": {
                "0%, 100%": { opacity: 0.3, transform: "rotate(0deg)" },
                "50%": { opacity: 1, transform: "rotate(180deg)" },
              },
              animation: "twinkle 4s ease-in-out infinite",
            }}
          >
            <Sparkles size={20} />
          </Box>
        </Box>

        {/* Heading */}
        <VStack gap={3}>
          <Text
            as="h2"
            fontSize={{ base: "2xl", md: "3xl" }}
            fontWeight="bold"
            color="var(--alt-text-primary)"
            bgGradient="var(--accent-gradient)"
            bgClip="text"
          >
            No Feeds Yet
          </Text>

          <Text
            fontSize="md"
            color="var(--alt-text-secondary)"
            lineHeight="1.6"
            px={4}
          >
            Start by adding your first RSS feed to begin reading articles from
            your favorite sources.
          </Text>
        </VStack>

        {/* Call to Action Button */}
        <Link href="/mobile/feeds/register" style={{ width: "100%" }}>
          <Button
            size="lg"
            w="100%"
            borderRadius="16px"
            bgGradient="linear(to-r, #FF416C, #FF4B2B)"
            color="white"
            fontWeight="bold"
            _hover={{
              transform: "translateY(-2px)",
              boxShadow: "0 8px 25px rgba(255, 65, 108, 0.4)",
            }}
            _active={{
              transform: "translateY(0)",
            }}
            transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
          >
            <Flex align="center" gap={2}>
              <Plus size={20} />
              <Text>Add Your First Feed</Text>
            </Flex>
          </Button>
        </Link>

        {/* Secondary helpful text */}
        <Box
          mt={4}
          p={4}
          bg="var(--alt-glass)"
          borderRadius="12px"
          border="1px solid var(--alt-glass-border)"
          w="100%"
        >
          <Text fontSize="sm" color="var(--alt-text-secondary)">
            <strong>Tip:</strong> You can find RSS feeds from your favorite
            blogs, news sites, or podcasts. Look for the RSS icon or
            &quot;/feed&quot; in the URL.
          </Text>
        </Box>
      </VStack>
    </Flex>
  );
}
