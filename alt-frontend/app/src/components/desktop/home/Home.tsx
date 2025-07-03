"use client";

import NextLink from "next/link";
import {
  Box,
  Flex,
  Text,
  VStack,
  HStack,
  Link as ChakraLink,
} from "@chakra-ui/react";
import { Rss, FileText, ArrowRight } from "lucide-react";
import { ThemeToggle } from "@/components/ThemeToggle";
import { AnimatedNumber } from "@/components/mobile/stats/AnimatedNumber";
import css from "@/app/page.module.css";

export default function DesktopHome() {
  const stats = {
    feed_amount: { amount: 1234 },
    summarized_feed: { amount: 567 },
  };

  return (
    <Box minH="100vh" bg="var(--app-bg)" position="relative" p={8}>
      {/* Theme Toggle */}
      <Box position="absolute" top={4} right={4} zIndex={100}>
        <ThemeToggle size="md" />
      </Box>

      <Flex
        as="main"
        id="main-content"
        maxW="6xl"
        mx="auto"
        pt={20}
        gap={16}
        align="flex-start"
      >
        {/* Left column: hero and navigation */}
        <VStack align="flex-start" spacing={6} flex="1">
          <VStack align="flex-start" spacing={2}>
            <Text
              as="h1"
              fontSize="4xl"
              fontWeight="bold"
              fontFamily="heading"
              bgGradient="var(--alt-gradient-primary)"
              bgClip="text"
              className={css.gradientText}
            >
              Alt
            </Text>
            <Text
              color="var(--alt-text-muted)"
              fontSize="lg"
              fontFamily="body"
              maxW="420px"
            >
              AI-powered RSS reader with modern aesthetics
            </Text>
          </VStack>

          <Box as="nav" aria-label="Main navigation" w="full">
            <ChakraLink
              as={NextLink}
              href="/mobile/feeds"
              bg="var(--alt-glass)"
              border="1px solid"
              borderColor="var(--alt-glass-border)"
              backdropFilter="blur(12px) saturate(1.2)"
              p={6}
              borderRadius="lg"
              display="block"
              textDecoration="none"
              _hover={{
                transform: "translateY(-2px)",
                boxShadow: "0 8px 25px var(--alt-glass-shadow)",
              }}
              _focus={{
                outline: "2px solid var(--alt-primary)",
                outlineOffset: "2px",
              }}
            >
              <Flex align="center" justify="space-between">
                <HStack gap={4}>
                  <Box
                    color="var(--alt-primary)"
                    p={2}
                    borderRadius="sm"
                    bg="var(--alt-glass)"
                  >
                    <Rss size={20} />
                  </Box>
                  <VStack align="start" spacing={0}>
                    <Text
                      fontSize="xl"
                      fontWeight="semibold"
                      fontFamily="heading"
                      color="var(--text-primary)"
                    >
                      Browse Feeds
                    </Text>
                    <Text
                      fontSize="sm"
                      color="var(--text-primary)"
                      fontFamily="body"
                    >
                      Explore RSS subscriptions
                    </Text>
                  </VStack>
                </HStack>
                <Box color="var(--alt-primary)" opacity={0.6}>
                  <ArrowRight size={18} />
                </Box>
              </Flex>
            </ChakraLink>
          </Box>
        </VStack>

        {/* Right column: stats */}
        <VStack align="stretch" spacing={6} flex="1" minW="280px">
          <Box
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-secondary)"
            backdropFilter="blur(10px) saturate(1.1)"
            p={6}
            borderRadius="lg"
            role="region"
            aria-labelledby="desktop-total-feeds-label"
            _hover={{
              transform: "translateY(-2px)",
              boxShadow: "0 8px 25px var(--alt-glass-shadow)",
            }}
          >
            <Flex direction="column" gap={3}>
              <Flex align="center" gap={2}>
                <Box
                  color="var(--alt-primary)"
                  p={1}
                  borderRadius="xs"
                  bg="var(--alt-glass)"
                >
                  <Rss size={16} />
                </Box>
                <Text
                  id="desktop-total-feeds-label"
                  fontSize="sm"
                  textTransform="uppercase"
                  color="var(--alt-text-primary)"
                  letterSpacing="wide"
                  fontFamily="body"
                  fontWeight="medium"
                >
                  Total Feeds
                </Text>
              </Flex>
              <AnimatedNumber
                value={stats.feed_amount.amount}
                duration={800}
                textProps={{
                  fontSize: "3xl",
                  fontWeight: "bold",
                  color: "var(--alt-text-primary)",
                  fontFamily: "heading",
                }}
              />
              <Text
                fontSize="sm"
                color="var(--alt-text-primary)"
                fontFamily="body"
              >
                RSS feeds monitored
              </Text>
            </Flex>
          </Box>

          <Box
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-secondary)"
            backdropFilter="blur(10px) saturate(1.1)"
            p={6}
            borderRadius="lg"
            role="region"
            aria-labelledby="desktop-ai-feeds-label"
            _hover={{
              transform: "translateY(-2px)",
              boxShadow: "0 8px 25px var(--alt-glass-shadow)",
            }}
          >
            <Flex direction="column" gap={3}>
              <Flex align="center" gap={2}>
                <Box
                  color="var(--alt-primary)"
                  p={1}
                  borderRadius="xs"
                  bg="var(--alt-glass)"
                >
                  <FileText size={16} />
                </Box>
                <Text
                  id="desktop-ai-feeds-label"
                  fontSize="sm"
                  textTransform="uppercase"
                  color="var(--alt-text-primary)"
                  letterSpacing="wide"
                  fontFamily="body"
                  fontWeight="medium"
                >
                  AI Summarized
                </Text>
              </Flex>
              <AnimatedNumber
                value={stats.summarized_feed.amount}
                duration={800}
                textProps={{
                  fontSize: "3xl",
                  fontWeight: "bold",
                  color: "var(--alt-text-primary)",
                  fontFamily: "heading",
                }}
              />
              <Text
                fontSize="sm"
                color="var(--alt-text-primary)"
                fontFamily="body"
              >
                Feeds processed by AI
              </Text>
            </Flex>
          </Box>
        </VStack>
      </Flex>
    </Box>
  );
}
