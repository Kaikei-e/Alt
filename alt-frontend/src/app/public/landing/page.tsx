export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";
export const revalidate = 0;

import NextLink from "next/link";
import {
  Box,
  Flex,
  Text,
  VStack,
  HStack,
  Link as ChakraLink,
  Button,
} from "@chakra-ui/react";
import { Rss, FileText, ArrowRight, LogIn, UserPlus } from "lucide-react";
import { AnimatedNumber } from "@/components/mobile/stats/AnimatedNumber";
import { ThemeToggle } from "@/components/ThemeToggle";
import { serverFetch } from "@/lib/server-fetch";

// Fetch public platform stats from alt-backend
async function getPlatformStats() {
  try {
    return await serverFetch<{
      feed_amount: { amount: number };
      summarized_feed: { amount: number };
    }>("/v1/platform/stats");
  } catch (err) {
    console.error("Error fetching platform stats:", err);
    return { feed_amount: { amount: 0 }, summarized_feed: { amount: 0 } };
  }
}

export default async function PublicLandingPage() {
  const stats = await getPlatformStats();

  return (
    <Box
      minH="100vh"
      p={4}
      bg="var(--app-bg)"
      position="relative"
      role="main"
      aria-label="Alt RSS Reader Public Landing Page"
    >
      {/* Theme Toggle Button */}
      <Box position="absolute" top="16px" right="16px" zIndex={100}>
        <ThemeToggle size="md" />
      </Box>

      {/* Hero Section */}
      <VStack as="section" gap={4} mb={8} textAlign="center" pt={16}>
        <Text
          as="h1"
          fontSize="4xl"
          fontFamily="heading"
          lineHeight="1.2"
          fontWeight="bold"
          bgClip="text"
          color="var(--alt-primary)"
          mb={2}
        >
          Alt
        </Text>
        <Text
          color="var(--alt-text-muted)"
          fontSize="lg"
          fontFamily="body"
          maxW="500px"
          lineHeight="1.6"
          mb={4}
        >
          AI-powered RSS reader with modern aesthetics. Join thousands of users
          managing their RSS feeds intelligently.
        </Text>

        {/* Authentication Buttons */}
        <HStack gap={4} flexWrap="wrap" justifyContent="center">
          <NextLink href="/auth/login" prefetch={false} passHref>
            <Button
              as="a"
              bg="var(--alt-primary)"
              color="white"
              size="lg"
              px={8}
              py={6}
              borderRadius="full"
              fontWeight="semibold"
              aria-label="ログインページへ移動"
              _hover={{
                bg: "var(--alt-primary)",
                transform: "translateY(-2px)",
                boxShadow: "0 8px 25px var(--alt-glass-shadow)",
              }}
              _active={{
                transform: "translateY(0)",
              }}
              transition="all 0.2s ease"
            >
              <HStack gap={2}>
                <LogIn size={18} aria-hidden="true" />
                <span>ログイン</span>
              </HStack>
            </Button>
          </NextLink>
          <NextLink href="/api/auth/register" prefetch={false} passHref>
            <Button
              as="a"
              variant="outline"
              borderColor="var(--alt-primary)"
              color="var(--alt-primary)"
              size="lg"
              px={8}
              py={6}
              borderRadius="full"
              fontWeight="semibold"
              aria-label="新規登録ページへ移動"
              _hover={{
                bg: "var(--alt-glass)",
                transform: "translateY(-2px)",
                boxShadow: "0 8px 25px var(--alt-glass-shadow)",
              }}
              _active={{
                transform: "translateY(0)",
              }}
              transition="all 0.2s ease"
            >
              <HStack gap={2}>
                <UserPlus size={18} aria-hidden="true" />
                <span>新規登録</span>
              </HStack>
            </Button>
          </NextLink>
        </HStack>
      </VStack>

      {/* Platform Statistics Section */}
      <VStack as="section" gap={6} mb={8} maxW="800px" mx="auto">
        <Text
          as="h2"
          fontSize="2xl"
          fontWeight="semibold"
          color="var(--text-primary)"
          fontFamily="heading"
          textAlign="center"
          mb={4}
        >
          プラットフォーム統計
        </Text>

        <HStack gap={6} flexWrap="wrap" justifyContent="center" w="full">
          {/* Total Feeds Card */}
          <Box
            data-testid="stat-card"
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-secondary)"
            backdropFilter="blur(10px) saturate(1.1)"
            minW="250px"
            p={6}
            borderRadius="lg"
            transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
            position="relative"
            overflow="hidden"
            textAlign="center"
            _hover={{
              transform: "translateY(-2px)",
              boxShadow: "0 8px 25px var(--alt-glass-shadow)",
            }}
          >
            <VStack gap={3}>
              <Box
                color="var(--alt-primary)"
                p={3}
                borderRadius="full"
                bg="var(--alt-glass)"
              >
                <Rss size={24} />
              </Box>
              <VStack gap={1}>
                <Text
                  fontSize="sm"
                  textTransform="uppercase"
                  color="var(--text-primary)"
                  letterSpacing="wide"
                  fontFamily="body"
                  fontWeight="medium"
                  opacity={0.8}
                >
                  Total RSS Feeds
                </Text>
                <AnimatedNumber
                  value={stats?.feed_amount?.amount || 0}
                  duration={1200}
                  textProps={{
                    fontSize: "3xl",
                    fontWeight: "bold",
                    color: "var(--text-primary)",
                    fontFamily: "heading",
                  }}
                />
                <Text
                  fontSize="sm"
                  color="var(--text-primary)"
                  opacity={0.7}
                  fontFamily="body"
                >
                  現在監視中のフィード数
                </Text>
              </VStack>
            </VStack>
          </Box>

          {/* AI Summarized Feeds Card */}
          <Box
            data-testid="stat-card"
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-secondary)"
            backdropFilter="blur(10px) saturate(1.1)"
            minW="250px"
            p={6}
            borderRadius="lg"
            transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
            position="relative"
            overflow="hidden"
            textAlign="center"
            _hover={{
              transform: "translateY(-2px)",
              boxShadow: "0 8px 25px var(--alt-glass-shadow)",
            }}
          >
            <VStack gap={3}>
              <Box
                color="var(--alt-primary)"
                p={3}
                borderRadius="full"
                bg="var(--alt-glass)"
              >
                <FileText size={24} />
              </Box>
              <VStack gap={1}>
                <Text
                  fontSize="sm"
                  textTransform="uppercase"
                  color="var(--text-primary)"
                  letterSpacing="wide"
                  fontFamily="body"
                  fontWeight="medium"
                  opacity={0.8}
                >
                  AI Processed
                </Text>
                <AnimatedNumber
                  value={stats?.summarized_feed?.amount || 0}
                  duration={1200}
                  textProps={{
                    fontSize: "3xl",
                    fontWeight: "bold",
                    color: "var(--text-primary)",
                    fontFamily: "heading",
                  }}
                />
                <Text
                  fontSize="sm"
                  color="var(--text-primary)"
                  opacity={0.7}
                  fontFamily="body"
                >
                  AI要約済みフィード数
                </Text>
              </VStack>
            </VStack>
          </Box>
        </HStack>
      </VStack>

      {/* Footer */}
      <Box
        as="footer"
        textAlign="center"
        py={8}
        borderTop="1px solid"
        borderColor="var(--alt-glass-border)"
        mt={16}
      >
        <Text
          fontSize="sm"
          color="var(--text-primary)"
          opacity={0.6}
          fontFamily="body"
        >
          © 2025 Alt RSS Reader. Built with modern web technologies.
        </Text>
      </Box>
    </Box>
  );
}
