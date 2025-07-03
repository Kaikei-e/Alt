"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
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
import { feedsApi } from "@/lib/api";
import { FeedStatsSummary } from "@/schema/feedStats";
import { AnimatedNumber } from "@/components/mobile/stats/AnimatedNumber";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { ThemeToggle } from "@/components/ThemeToggle";
import css from "./page.module.css";

export default function Home() {
  const router = useRouter();
  const [stats, setStats] = useState<FeedStatsSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Performance optimization: Preload /mobile/feeds route for instant navigation
  useEffect(() => {
    router.prefetch("/mobile/feeds");
  }, [router]);

  useEffect(() => {
    const fetchStats = async () => {
      try {
        setIsLoading(true);
        const data = await feedsApi.getFeedStats();
        setStats(data);
      } catch (err) {
        setError("Unable to load statistics");
        console.error("Error fetching stats:", err);
      } finally {
        setIsLoading(false);
      }
    };

    fetchStats();
  }, []);

  return (
    <Box
      minH="100vh"
      minHeight="100dvh"
      bg="var(--app-bg)"
      position="relative"
      pt="env(safe-area-inset-top)"
      pb="env(safe-area-inset-bottom)"
      background="var(--alt-gradient-bg)"
    >
      <Box
        as="a"
        position="absolute"
        top="-40px"
        left="6px"
        bg="var(--gradient-bg)"
        color="var(--text-primary)"
        p={2}
        borderRadius="xs"
        zIndex={1000}
        textDecoration="none"
        transition="top 0.2s ease"
        _focus={{
          top: "6px",
          outline: "2px solid",
          outlineColor: "accent.default",
        }}
        onClick={(e) => {
          e.preventDefault();
          const mainContent = document.getElementById("main-content");
          if (mainContent) {
            mainContent.setAttribute("tabindex", "-1");
            mainContent.focus();
          }
        }}
      >
        Skip to main content
      </Box>

      {/* Theme Toggle Button */}
      <Box
        position="absolute"
        top="env(safe-area-inset-top, 16px)"
        right="16px"
        zIndex={100}
      >
        <ThemeToggle size="md" />
      </Box>
      {/* Main Content */}
      <Box
        id="main-content"
        as="main"
        position="relative"
        zIndex={1}
        maxW="container.sm"
        mx="auto"
        px={4}
        py={6}
        width="100%"
        maxWidth="100%"
        role="main"
        aria-label="Alt RSS Reader Home Page"
      >
        {/* Hero Section - Refined & Elegant */}
        <VStack as="section" gap={2} mb={6} textAlign="center">
          <Text
            as="h1"
            fontSize="3xl"
            fontFamily="heading"
            lineHeight="1.2"
            fontWeight="bold"
            bgClip="text"
            color="var(--alt-primary)"
            mb={1}
          >
            Alt
          </Text>
          <Text
            color="var(--alt-text-muted)"
            fontSize="md"
            fontFamily="body"
            maxW="300px"
            lineHeight="1.5"
          >
            AI-powered RSS reader with modern aesthetics
          </Text>
        </VStack>

        {/* Navigation Section - Refined Glass Card */}
        <Box as="nav" aria-label="Main navigation" mb={6}>
          <ChakraLink
            as={NextLink}
            href="/mobile/feeds"
            data-testid="nav-card"
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-glass-border)"
            backdropFilter="blur(12px) saturate(1.2)"
            p={4}
            borderRadius="lg"
            color="fg.default"
            cursor="pointer"
            transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
            minH="44px"
            w="full"
            textAlign="left"
            textDecoration="none"
            display="block"
            position="relative"
            overflow="hidden"
            _hover={{
              transform: "translateY(-1px)",
              bg: "var(--alt-glass)",
              borderColor: "var(--alt-glass-border)",
              boxShadow: "0 4px 20px var(--alt-glass-shadow)",
              textDecoration: "none",
            }}
            _focus={{
              outline: "2px solid",
              outlineColor: "var(--alt-primary)",
              outlineOffset: "2px",
            }}
            _active={{
              transform: "translateY(0)",
            }}
          >
            {/* Subtle gradient overlay */}
            <Box
              position="absolute"
              top={0}
              left={0}
              right={0}
              bottom={0}
              bgGradient="linear(45deg, transparent 0%, var(--alt-glass) 50%, transparent 100%)"
              opacity={0}
              transition="opacity 0.2s ease"
              _hover={{ opacity: 1 }}
            />

            <Flex
              align="center"
              justify="space-between"
              position="relative"
              zIndex={1}
            >
              <HStack gap={3}>
                <Box
                  color="var(--alt-primary)"
                  p={2}
                  borderRadius="sm"
                  bg="var(--alt-glass)"
                >
                  <Rss size={18} />
                </Box>
                <VStack align="start" gap={0}>
                  <Text
                    fontSize="lg"
                    fontWeight="semibold"
                    fontFamily="heading"
                    color="var(--text-primary)"
                  >
                    Browse Feeds
                  </Text>
                  <Text
                    fontSize="xs"
                    color="var(--text-primary)"
                    fontFamily="body"
                  >
                    Explore RSS subscriptions
                  </Text>
                </VStack>
              </HStack>

              <Box color="var(--alt-primary)" opacity={0.6}>
                <ArrowRight size={16} />
              </Box>
            </Flex>
          </ChakraLink>
        </Box>

        {/* Dashboard Section - Refined Stats Grid */}
        <VStack as="section" gap={3} mb={6} aria-labelledby="dashboard-heading">
          <Text
            id="dashboard-heading"
            as="h2"
            fontSize="lg"
            fontWeight="semibold"
            color="var(--text-primary)"
            fontFamily="heading"
            alignSelf="flex-start"
            mb={1}
          >
            Dashboard
          </Text>

          {isLoading ? (
            <VStack
              gap={3}
              w="full"
              aria-live="polite"
              aria-label="Loading statistics"
            >
              {[1, 2].map((i) => (
                <Box
                  key={i}
                  data-testid="stats-loading"
                  bg="var(--alt-glass)"
                  border="1px solid"
                  borderColor="var(--alt-glass-border)"
                  backdropFilter="blur(8px)"
                  p={4}
                  borderRadius="md"
                  w="full"
                  opacity="0.7"
                  aria-label={`Loading statistic ${i}`}
                >
                  <Flex direction="column" gap={2}>
                    <Box h="12px" bg="var(--alt-glass)" borderRadius="xs" />
                    <Box h="24px" bg="var(--alt-glass)" borderRadius="sm" />
                    <Box h="10px" bg="var(--alt-glass)" borderRadius="xs" />
                  </Flex>
                </Box>
              ))}
            </VStack>
          ) : error ? (
            <Box
              bg="var(--alt-glass)"
              border="1px solid"
              borderColor="var(--alt-error)"
              backdropFilter="blur(8px)"
              p={4}
              borderRadius="md"
              w="full"
              textAlign="center"
              role="alert"
              aria-live="assertive"
            >
              <Text color="semantic.error" fontFamily="body" fontSize="sm">
                {error}
              </Text>
            </Box>
          ) : (
            <VStack gap={3} w="full" aria-live="polite">
              {/* Total Feeds Card */}
              <Box
                data-testid="stat-card"
                bg="var(--alt-glass)"
                border="1px solid"
                borderColor="var(--alt-secondary)"
                backdropFilter="blur(10px) saturate(1.1)"
                w="full"
                p={4}
                borderRadius="md"
                transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
                position="relative"
                overflow="hidden"
                role="region"
                aria-labelledby="total-feeds-label"
                _hover={{
                  transform: "translateY(-1px)",
                  bg: "var(--alt-glass)",
                  borderColor: "var(--alt-tertiary)",
                  boxShadow: "0 8px 25px var(--alt-glass-shadow)",
                }}
              >
                {/* Subtle accent border */}
                <Box
                  position="absolute"
                  top={0}
                  left={0}
                  right={0}
                  h="1px"
                  bgGradient="linear(to-r, transparent, var(--alt-glass), transparent)"
                />

                <Flex direction="column" gap={2}>
                  <Flex align="center" gap={2}>
                    <Box
                      color="var(--alt-primary)"
                      p={1}
                      borderRadius="xs"
                      bg="var(--alt-glass)"
                    >
                      <Rss size={14} />
                    </Box>
                    <Text
                      id="total-feeds-label"
                      fontSize="xs"
                      textTransform="uppercase"
                      color="var(--text-primary)"
                      letterSpacing="wide"
                      fontFamily="body"
                      fontWeight="medium"
                    >
                      Total Feeds
                    </Text>
                  </Flex>

                  <Box data-testid="animated-number">
                    <AnimatedNumber
                      value={stats?.feed_amount?.amount || 0}
                      duration={800}
                      textProps={{
                        fontSize: "2xl",
                        fontWeight: "bold",
                        color: "var(--text-primary)",
                        fontFamily: "heading",
                      }}
                    />
                  </Box>

                  <Text
                    fontSize="xs"
                    color="var(--text-primary)"
                    lineHeight="1.4"
                    fontFamily="body"
                  >
                    RSS feeds monitored
                  </Text>
                </Flex>
              </Box>

              {/* AI Summarized Feeds Card */}
              <Box
                data-testid="stat-card"
                bg="var(--alt-glass)"
                border="1px solid"
                borderColor="var(--alt-secondary)"
                backdropFilter="blur(10px) saturate(1.1)"
                w="full"
                p={4}
                borderRadius="md"
                transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
                position="relative"
                overflow="hidden"
                role="region"
                aria-labelledby="ai-feeds-label"
                _hover={{
                  transform: "translateY(-1px)",
                  bg: "var(--alt-glass)",
                  borderColor: "var(--alt-tertiary)",
                  boxShadow: "0 8px 25px var(--alt-glass-shadow)",
                }}
              >
                {/* Subtle accent border */}
                <Box
                  position="absolute"
                  top={0}
                  left={0}
                  right={0}
                  h="1px"
                  bgGradient="linear(to-r, transparent, var(--accent-primary), transparent)"
                />

                <Flex direction="column" gap={2}>
                  <Flex align="center" gap={2}>
                    <Box
                      color="var(--alt-primary)"
                      p={1}
                      borderRadius="xs"
                      bg="var(--alt-glass)"
                    >
                      <FileText size={14} />
                    </Box>
                    <Text
                      id="ai-feeds-label"
                      fontSize="xs"
                      textTransform="uppercase"
                      color="var(--text-primary)"
                      letterSpacing="wide"
                      fontFamily="body"
                      fontWeight="medium"
                    >
                      AI Summarized
                    </Text>
                  </Flex>

                  <Box data-testid="animated-number">
                    <AnimatedNumber
                      value={stats?.summarized_feed?.amount || 0}
                      duration={800}
                      textProps={{
                        fontSize: "2xl",
                        fontWeight: "bold",
                        color: "var(--text-primary)",
                        fontFamily: "heading",
                      }}
                    />
                  </Box>

                  <Text
                    fontSize="xs"
                    color="var(--text-primary)"
                    lineHeight="1.4"
                    fontFamily="body"
                  >
                    Feeds processed by AI
                  </Text>
                </Flex>
              </Box>
            </VStack>
          )}
        </VStack>
      </Box>

      {/* Floating Menu */}
      <FloatingMenu />
    </Box>
  );
}
