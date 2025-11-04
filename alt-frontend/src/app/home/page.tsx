"use client";

import { Box, Button, Grid, Heading, HStack, Text, VStack } from "@chakra-ui/react";
import { BarChart3, LogOut, Rss, Settings, Users } from "lucide-react";
import NextLink from "next/link";
import { useAuth } from "@/contexts/auth-context";

export default function HomePage() {
  const { logout } = useAuth();

  const handleLogout = async () => {
    try {
      await logout();
      window.location.href = "/public/landing";
    } catch (error) {
      console.error("Logout failed:", error);
    }
  };
  return (
    <Box minH="100vh" p={6} bg="var(--app-bg)" role="main" aria-label="Alt RSS Reader Dashboard">
      <VStack gap={8} maxW="1200px" mx="auto">
        {/* Header */}
        <HStack w="full" justify="space-between" align="center">
          <Heading as="h1" size="xl" color="var(--alt-primary)" fontFamily="heading">
            Alt Dashboard
          </Heading>
          <HStack gap={4}>
            <NextLink href="/desktop/settings" passHref>
              <Button
                variant="ghost"
                size="md"
                color="var(--text-primary)"
                _hover={{ bg: "var(--alt-glass)" }}
              >
                <HStack gap={2}>
                  <Settings size={18} />
                  設定
                </HStack>
              </Button>
            </NextLink>
            <Button
              variant="outline"
              size="md"
              borderColor="var(--alt-primary)"
              color="var(--alt-primary)"
              _hover={{ bg: "var(--alt-glass)" }}
              onClick={handleLogout}
            >
              <HStack gap={2}>
                <LogOut size={18} />
                ログアウト
              </HStack>
            </Button>
          </HStack>
        </HStack>

        {/* Welcome Message */}
        <Box textAlign="center" py={8}>
          <Text fontSize="2xl" color="var(--text-primary)" fontFamily="body" mb={4}>
            Alt RSS Reader へようこそ！
          </Text>
          <Text
            fontSize="lg"
            color="var(--alt-text-muted)"
            fontFamily="body"
            maxW="600px"
            mx="auto"
            lineHeight="1.6"
          >
            AI-powered な RSS リーダーで、効率的に情報収集を始めましょう。
          </Text>
        </Box>

        {/* Quick Actions Grid */}
        <Grid templateColumns={{ base: "1fr", md: "repeat(3, 1fr)" }} gap={6} w="full">
          {/* Feed Management */}
          <Box
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-secondary)"
            backdropFilter="blur(10px)"
            p={6}
            borderRadius="lg"
            _hover={{
              transform: "translateY(-2px)",
              boxShadow: "0 8px 25px var(--alt-glass-shadow)",
            }}
            transition="all 0.2s ease"
          >
            <VStack gap={4} align="center">
              <Box color="var(--alt-primary)" p={3} borderRadius="full" bg="var(--alt-glass)">
                <Rss size={32} />
              </Box>
              <VStack gap={2} textAlign="center">
                <Text
                  fontSize="xl"
                  fontWeight="semibold"
                  color="var(--text-primary)"
                  fontFamily="heading"
                >
                  フィード管理
                </Text>
                <Text
                  fontSize="sm"
                  color="var(--alt-text-muted)"
                  fontFamily="body"
                  lineHeight="1.5"
                >
                  RSS フィードの追加・管理
                </Text>
              </VStack>
              <NextLink href="/desktop/feeds" passHref>
                <Button
                  bg="var(--alt-primary)"
                  color="white"
                  size="md"
                  w="full"
                  _hover={{ opacity: 0.9 }}
                >
                  フィード一覧
                </Button>
              </NextLink>
            </VStack>
          </Box>

          {/* Article Reading */}
          <Box
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-secondary)"
            backdropFilter="blur(10px)"
            p={6}
            borderRadius="lg"
            _hover={{
              transform: "translateY(-2px)",
              boxShadow: "0 8px 25px var(--alt-glass-shadow)",
            }}
            transition="all 0.2s ease"
          >
            <VStack gap={4} align="center">
              <Box color="var(--alt-primary)" p={3} borderRadius="full" bg="var(--alt-glass)">
                <BarChart3 size={32} />
              </Box>
              <VStack gap={2} textAlign="center">
                <Text
                  fontSize="xl"
                  fontWeight="semibold"
                  color="var(--text-primary)"
                  fontFamily="heading"
                >
                  記事閲覧
                </Text>
                <Text
                  fontSize="sm"
                  color="var(--alt-text-muted)"
                  fontFamily="body"
                  lineHeight="1.5"
                >
                  AI 要約付き記事を読む
                </Text>
              </VStack>
              <NextLink href="/desktop/articles" passHref>
                <Button
                  bg="var(--alt-primary)"
                  color="white"
                  size="md"
                  w="full"
                  _hover={{ opacity: 0.9 }}
                >
                  記事一覧
                </Button>
              </NextLink>
            </VStack>
          </Box>

          {/* Mobile Version */}
          <Box
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-secondary)"
            backdropFilter="blur(10px)"
            p={6}
            borderRadius="lg"
            _hover={{
              transform: "translateY(-2px)",
              boxShadow: "0 8px 25px var(--alt-glass-shadow)",
            }}
            transition="all 0.2s ease"
          >
            <VStack gap={4} align="center">
              <Box color="var(--alt-primary)" p={3} borderRadius="full" bg="var(--alt-glass)">
                <Users size={32} />
              </Box>
              <VStack gap={2} textAlign="center">
                <Text
                  fontSize="xl"
                  fontWeight="semibold"
                  color="var(--text-primary)"
                  fontFamily="heading"
                >
                  モバイル版
                </Text>
                <Text
                  fontSize="sm"
                  color="var(--alt-text-muted)"
                  fontFamily="body"
                  lineHeight="1.5"
                >
                  モバイル最適化 UI
                </Text>
              </VStack>
              <NextLink href="/mobile/feeds" passHref>
                <Button
                  bg="var(--alt-primary)"
                  color="white"
                  size="md"
                  w="full"
                  _hover={{ opacity: 0.9 }}
                >
                  モバイル版
                </Button>
              </NextLink>
            </VStack>
          </Box>
        </Grid>

        {/* Footer */}
        <Box
          textAlign="center"
          py={8}
          borderTop="1px solid"
          borderColor="var(--alt-glass-border)"
          w="full"
        >
          <Text fontSize="sm" color="var(--text-primary)" opacity={0.6} fontFamily="body">
            Alt RSS Reader - AI-powered RSS management
          </Text>
        </Box>
      </VStack>
    </Box>
  );
}
