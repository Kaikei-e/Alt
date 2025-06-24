"use client";

import { useEffect, useState, lazy, Suspense } from "react";
import { useRouter } from "next/navigation";
import { Box, Flex, Text, VStack, HStack } from "@chakra-ui/react";
import { FiRss, FiFileText } from "react-icons/fi";
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
      {/* Skip Link for Accessibility (DESIGN_LANGUAGE.md) */}
      <Box
        as="a"
        position="absolute"
        top="-40px"
        left="6px"
        bg="bg.canvas"
        color="fg.default"
        p={2}
        borderRadius="sm"
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
        p={5}
        width="100%"
        maxWidth="100%"
        padding="20px"
        role="main"
        aria-label="Alt RSS Reader Home Page"
      >
        {/* Hero Section - TASK.md Compliant */}
        <VStack as="section" gap={4} mb={8} textAlign="center">
          <Text
            as="h1"
            fontSize="4xl"
            fontFamily="heading"
            lineHeight="1.1"
            fontWeight="bold"
            bgGradient="vaporwave"
            bgClip="text"
            className={css.gradientText}
          >
            Welcome to Alt
          </Text>
          <Text
            color="fg.muted"
            fontSize="lg"
            fontFamily="body"
          >
            Your AI-powered RSS reader with a vaporwave twist
          </Text>
        </VStack>

        {/* Navigation Section - Primary CTA (TASK.md) */}
        <Box as="nav" aria-label="Main navigation" mb={8}>
          <Box
            as="a"
            data-testid="nav-card"
            bg="bg.subtle"
            border="1px solid"
            borderColor="border.default"
            backdropFilter="blur(16px)"
            p={6}
            borderRadius="xl"
            color="fg.default"
            cursor="pointer"
            transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
            minH="44px"
            w="full"
            textAlign="left"
            textDecoration="none"
            display="block"
            className={`${css.glass} touch-target`}
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
              transform: "translateY(-2px)",
              boxShadow: "0 8px 32px rgba(255, 0, 110, 0.15)",
              bg: "border.hover",
              borderColor: "accent.default",
              textDecoration: "none",
            }}
            _focus={{
              outline: "2px solid",
              outlineColor: "accent.default",
              outlineOffset: "2px",
            }}
            _focusVisible={{
              outline: "2px solid",
              outlineColor: "accent.default",
              outlineOffset: "2px",
            }}
          >
            <HStack gap={4}>
              <Box color="accent.default">
                <FiRss size={24} />
              </Box>
              <VStack align="start" gap={1}>
                <Text
                  fontSize="xl"
                  fontWeight="bold"
                  fontFamily="heading"
                  color="fg.default"
                >
                  Browse Feeds
                </Text>
                <Text
                  fontSize="sm"
                  color="fg.muted"
                  fontFamily="body"
                >
                  Explore your RSS subscriptions
                </Text>
              </VStack>
            </HStack>
          </Box>
        </Box>

        {/* Dashboard Statistics Section - TASK.md */}
        <VStack as="section" gap={4} mb={8} aria-labelledby="dashboard-heading">
          <Text
            id="dashboard-heading"
            as="h2"
            fontSize="2xl"
            fontWeight="bold"
            color="accent.emphasized"
            fontFamily="heading"
            mb={2}
          >
            Dashboard
          </Text>

          {isLoading ? (
            <VStack gap={4} w="full" aria-live="polite" aria-label="Loading statistics">
              {/* Loading Skeletons - TASK.md Specification */}
              <Box
                data-testid="stats-loading"
                bg="bg.subtle"
                border="1px solid"
                borderColor="border.default"
                backdropFilter="blur(16px)"
                p={6}
                borderRadius="xl"
                w="full"
                className={`${css.glass} ${css.loadingSkeleton}`}
                opacity="0.8"
                transition="opacity 0.3s ease"
                aria-label="Loading feed statistics"
              >
                <Flex direction="column" gap={3}>
                  <Box h="20px" bg="rgba(255, 255, 255, 0.1)" borderRadius="md" />
                  <Box h="32px" bg="rgba(255, 0, 110, 0.2)" borderRadius="md" />
                  <Box h="16px" bg="rgba(255, 255, 255, 0.08)" borderRadius="sm" />
                </Flex>
              </Box>
              <Box
                bg="bg.subtle"
                border="1px solid"
                borderColor="border.default"
                backdropFilter="blur(16px)"
                p={6}
                borderRadius="xl"
                w="full"
                className={`${css.glass} ${css.loadingSkeleton}`}
                opacity="0.8"
                transition="opacity 0.3s ease"
              >
                <Flex direction="column" gap={3}>
                  <Box h="20px" bg="rgba(255, 255, 255, 0.1)" borderRadius="md" />
                  <Box h="32px" bg="rgba(139, 56, 236, 0.2)" borderRadius="md" />
                  <Box h="16px" bg="rgba(255, 255, 255, 0.08)" borderRadius="sm" />
                </Flex>
              </Box>
            </VStack>
          ) : error ? (
            <Box
              bg="bg.subtle"
              border="1px solid"
              borderColor="border.default"
              backdropFilter="blur(16px)"
              p={6}
              borderRadius="xl"
              w="full"
              textAlign="center"
              className={css.glass}
              role="alert"
              aria-live="assertive"
            >
              <Text
                color="semantic.error"
                fontFamily="body"
                fontSize="md"
              >
                {error}
              </Text>
            </Box>
          ) : (
            <VStack gap={4} w="full" aria-live="polite">
              <Box
                data-testid="stat-card"
                bg="bg.subtle"
                border="1px solid"
                borderColor="border.default"
                backdropFilter="blur(16px)"
                w="full"
                p={6}
                borderRadius="xl"
                transition="all 0.3s ease"
                className={css.glass}
                role="region"
                aria-labelledby="total-feeds-label"
                _hover={{
                  transform: "translateY(-2px) scale(1.02)",
                  boxShadow: "0 20px 40px rgba(255, 0, 110, 0.3)",
                }}
              >
                <Flex direction="column" gap={3}>
                  <Flex align="center" gap={2}>
                    <Box color="accent.default">
                      <FiRss size={20} />
                    </Box>
                    <Text
                      id="total-feeds-label"
                      fontSize="sm"
                      textTransform="uppercase"
                      color="fg.subtle"
                      letterSpacing="wider"
                      fontFamily="body"
                    >
                      Total Feeds
                    </Text>
                  </Flex>
                  <Box data-testid="animated-number">
                    <AnimatedNumber
                      value={stats?.feed_amount?.amount || 0}
                      duration={800}
                      textProps={{
                        fontSize: "3xl",
                        fontWeight: "bold",
                        color: "fg.default",
                        fontFamily: "heading",
                      }}
                    />
                  </Box>
                  <Text
                    fontSize="sm"
                    color="fg.muted"
                    lineHeight="1.5"
                    fontFamily="body"
                  >
                    RSS feeds being monitored
                  </Text>
                </Flex>
              </Box>

              <Box
                data-testid="stat-card"
                bg="bg.subtle"
                border="1px solid"
                borderColor="border.default"
                backdropFilter="blur(16px)"
                w="full"
                p={6}
                borderRadius="xl"
                transition="all 0.3s ease"
                className={css.glass}
                role="region"
                aria-labelledby="ai-feeds-label"
                _hover={{
                  transform: "translateY(-2px) scale(1.02)",
                  boxShadow: "0 20px 40px rgba(139, 56, 236, 0.3)",
                }}
              >
                <Flex direction="column" gap={3}>
                  <Flex align="center" gap={2}>
                    <Box color="accent.emphasized">
                      <FiFileText size={20} />
                    </Box>
                    <Text
                      id="ai-feeds-label"
                      fontSize="sm"
                      textTransform="uppercase"
                      color="fg.subtle"
                      letterSpacing="wider"
                      fontFamily="body"
                    >
                      AI-Summarized Feeds
                    </Text>
                  </Flex>
                  <Box data-testid="animated-number">
                    <AnimatedNumber
                      value={stats?.summarized_feed?.amount || 0}
                      duration={800}
                      textProps={{
                        fontSize: "3xl",
                        fontWeight: "bold",
                        color: "fg.default",
                        fontFamily: "heading",
                      }}
                    />
                  </Box>
                  <Text
                    fontSize="sm"
                    color="fg.muted"
                    lineHeight="1.5"
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
