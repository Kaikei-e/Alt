"use client";

import { useEffect, useState, lazy, Suspense } from "react";
import { useRouter } from "next/navigation";
import { Box, Flex, Text, VStack, HStack } from "@chakra-ui/react";
import { FiRss, FiFileText, FiArrowRight } from "react-icons/fi";
import { feedsApi } from "@/lib/api";
import { FeedStatsSummary } from "@/schema/feedStats";
import { AnimatedNumber } from "@/components/mobile/stats/AnimatedNumber";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import css from "./page.module.css";

// Lazy load heavy components
const VaporwaveBackground = lazy(() =>
  import("@/components/mobile/stats/VaporwaveBackground").then(mod => ({
    default: mod.VaporwaveBackground
  }))
);

export default function Home() {
  const router = useRouter();
  const [stats, setStats] = useState<FeedStatsSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const handleNavigateToFeeds = () => {
    router.push('/mobile/feeds');
  };

  // Performance optimization: Preload /mobile/feeds route for instant navigation
  useEffect(() => {
    router.prefetch('/mobile/feeds');
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
      bg="bg.canvas"
      position="relative"
      pt="env(safe-area-inset-top)"
      pb="env(safe-area-inset-bottom)"
    >
      <Box
        as="a"
        position="absolute"
        top="-40px"
        left="6px"
        bg="bg.canvas"
        color="fg.default"
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
          const mainContent = document.getElementById('main-content');
          if (mainContent) {
            mainContent.setAttribute('tabindex', '-1');
            mainContent.focus();
          }
        }}
      >
        Skip to main content
      </Box>

      {/* Vaporwave Background */}
      <Suspense fallback={<div data-testid="vaporwave-background-loading" />}>
        <Box position="absolute" inset={0} zIndex={0} data-testid="vaporwave-background">
          <VaporwaveBackground />
        </Box>
      </Suspense>

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
            bgGradient="vaporwave"
            bgClip="text"
            className={css.gradientText}
            mb={1}
          >
            Alt
          </Text>
          <Text
            color="fg.muted"
            fontSize="md"
            fontFamily="body"
            maxW="300px"
            lineHeight="1.5"
          >
            AI-powered RSS reader with vaporwave aesthetics
          </Text>
        </VStack>

        {/* Navigation Section - Refined Glass Card */}
        <Box as="nav" aria-label="Main navigation" mb={6}>
          <Box
            as="a"
            data-testid="nav-card"
            bg="rgba(255, 255, 255, 0.05)"
            border="1px solid"
            borderColor="rgba(255, 255, 255, 0.1)"
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
            onClick={(e) => {
              e.preventDefault();
              handleNavigateToFeeds();
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                handleNavigateToFeeds();
              }
            }}
            tabIndex={0}
            role="link"
            _hover={{
              transform: "translateY(-1px)",
              bg: "rgba(255, 255, 255, 0.08)",
              borderColor: "rgba(255, 0, 110, 0.3)",
              boxShadow: "0 4px 20px rgba(255, 0, 110, 0.1)",
              textDecoration: "none",
            }}
            _focus={{
              outline: "2px solid",
              outlineColor: "accent.default",
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
              bgGradient="linear(45deg, transparent 0%, rgba(255, 0, 110, 0.05) 50%, transparent 100%)"
              opacity={0}
              transition="opacity 0.2s ease"
              _groupHover={{ opacity: 1 }}
            />

            <Flex align="center" justify="space-between" position="relative" zIndex={1}>
              <HStack gap={3}>
                <Box
                  color="accent.default"
                  p={2}
                  borderRadius="sm"
                  bg="rgba(255, 0, 110, 0.1)"
                >
                  <FiRss size={18} />
                </Box>
                <VStack align="start" gap={0}>
                  <Text
                    fontSize="lg"
                    fontWeight="semibold"
                    fontFamily="heading"
                    color="fg.default"
                  >
                    Browse Feeds
                  </Text>
                  <Text
                    fontSize="xs"
                    color="fg.subtle"
                    fontFamily="body"
                  >
                    Explore RSS subscriptions
                  </Text>
                </VStack>
              </HStack>

              <Box color="fg.subtle" opacity={0.6}>
                <FiArrowRight size={16} />
              </Box>
            </Flex>
          </Box>
        </Box>

        {/* Dashboard Section - Refined Stats Grid */}
        <VStack as="section" gap={3} mb={6} aria-labelledby="dashboard-heading">
          <Text
            id="dashboard-heading"
            as="h2"
            fontSize="lg"
            fontWeight="semibold"
            color="fg.default"
            fontFamily="heading"
            alignSelf="flex-start"
            mb={1}
          >
            Dashboard
          </Text>

          {isLoading ? (
            <VStack gap={3} w="full" aria-live="polite" aria-label="Loading statistics">
              {[1, 2].map((i) => (
                <Box
                  key={i}
                  data-testid="stats-loading"
                  bg="rgba(255, 255, 255, 0.03)"
                  border="1px solid"
                  borderColor="rgba(255, 255, 255, 0.08)"
                  backdropFilter="blur(8px)"
                  p={4}
                  borderRadius="md"
                  w="full"
                  opacity="0.7"
                  aria-label={`Loading statistic ${i}`}
                >
                  <Flex direction="column" gap={2}>
                    <Box h="12px" bg="rgba(255, 255, 255, 0.1)" borderRadius="xs" />
                    <Box h="24px" bg="rgba(255, 0, 110, 0.15)" borderRadius="sm" />
                    <Box h="10px" bg="rgba(255, 255, 255, 0.06)" borderRadius="xs" />
                  </Flex>
                </Box>
              ))}
            </VStack>
          ) : error ? (
            <Box
              bg="rgba(255, 255, 255, 0.03)"
              border="1px solid"
              borderColor="rgba(255, 75, 87, 0.2)"
              backdropFilter="blur(8px)"
              p={4}
              borderRadius="md"
              w="full"
              textAlign="center"
              role="alert"
              aria-live="assertive"
            >
              <Text
                color="semantic.error"
                fontFamily="body"
                fontSize="sm"
              >
                {error}
              </Text>
            </Box>
          ) : (
            <VStack gap={3} w="full" aria-live="polite">
              {/* Total Feeds Card */}
              <Box
                data-testid="stat-card"
                bg="rgba(255, 255, 255, 0.04)"
                border="1px solid"
                borderColor="rgba(255, 255, 255, 0.08)"
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
                  bg: "rgba(255, 255, 255, 0.06)",
                  borderColor: "rgba(255, 0, 110, 0.2)",
                  boxShadow: "0 8px 25px rgba(255, 0, 110, 0.1)",
                }}
              >
                {/* Subtle accent border */}
                <Box
                  position="absolute"
                  top={0}
                  left={0}
                  right={0}
                  h="1px"
                  bgGradient="linear(to-r, transparent, rgba(255, 0, 110, 0.5), transparent)"
                />

                <Flex direction="column" gap={2}>
                  <Flex align="center" gap={2}>
                    <Box
                      color="accent.default"
                      p={1}
                      borderRadius="xs"
                      bg="rgba(255, 0, 110, 0.1)"
                    >
                      <FiRss size={14} />
                    </Box>
                    <Text
                      id="total-feeds-label"
                      fontSize="xs"
                      textTransform="uppercase"
                      color="fg.subtle"
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
                        color: "fg.default",
                        fontFamily: "heading",
                      }}
                    />
                  </Box>

                  <Text
                    fontSize="xs"
                    color="fg.muted"
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
                bg="rgba(255, 255, 255, 0.04)"
                border="1px solid"
                borderColor="rgba(255, 255, 255, 0.08)"
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
                  bg: "rgba(255, 255, 255, 0.06)",
                  borderColor: "rgba(131, 56, 236, 0.2)",
                  boxShadow: "0 8px 25px rgba(131, 56, 236, 0.1)",
                }}
              >
                {/* Subtle accent border */}
                <Box
                  position="absolute"
                  top={0}
                  left={0}
                  right={0}
                  h="1px"
                  bgGradient="linear(to-r, transparent, rgba(131, 56, 236, 0.5), transparent)"
                />

                <Flex direction="column" gap={2}>
                  <Flex align="center" gap={2}>
                    <Box
                      color="accent.emphasized"
                      p={1}
                      borderRadius="xs"
                      bg="rgba(131, 56, 236, 0.1)"
                    >
                      <FiFileText size={14} />
                    </Box>
                    <Text
                      id="ai-feeds-label"
                      fontSize="xs"
                      textTransform="uppercase"
                      color="fg.subtle"
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
                        color: "fg.default",
                        fontFamily: "heading",
                      }}
                    />
                  </Box>

                  <Text
                    fontSize="xs"
                    color="fg.muted"
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
