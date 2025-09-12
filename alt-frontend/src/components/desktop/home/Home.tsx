"use client";

import React from "react";
import NextLink from "next/link";
import {
  Box,
  Flex,
  Text,
  VStack,
  HStack,
  Grid,
  GridItem,
  Button,
  Icon,
  Link as ChakraLink,
} from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { feedsApi } from "@/lib/api";
import type { FeedStatsSummary } from "@/schema/feedStats";
import {
  Home,
  Rss,
  FileText,
  Search,
  Settings,
  Plus,
  TrendingUp,
  Clock,
  ArrowRight,
  Activity,
  Bookmark,
  Download,
  Zap,
} from "lucide-react";
import { ThemeToggle } from "@/components/ThemeToggle";
import { AnimatedNumber } from "@/components/mobile/stats/AnimatedNumber";
import Loading from "@/components/mobile/utils/Loading";
import { useTodayUnreadCount } from "@/hooks/useTodayUnreadCount";

export default function DesktopHome() {
  const [feedStats, setFeedStats] = useState<FeedStatsSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const { count: unreadArticles } = useTodayUnreadCount();
  const extraStats = {
    unreadArticles,
    weeklyReads: 156,
    aiProcessed: 78,
    bookmarks: 42,
  };

  useEffect(() => {
    const fetchStats = async () => {
      try {
        setIsLoading(true);
        // TODO.md: 依存データ fetch も cache: 'no-store' に（特に認証に依存するもの）
        feedsApi.clearCache(); // キャッシュクリアで no-store 効果を確保
        // TODO.md: 一時的にコメントアウトして500問題を特定
        // const data = await feedsApi.getFeedStats();
        // setFeedStats(data);

        // Mock data for testing
        setFeedStats({
          feed_amount: { amount: 0 },
          summarized_feed: { amount: 0 },
        });
      } catch (err) {
        setError("Unable to load statistics");
        console.error("Error fetching stats:", err);
      } finally {
        setIsLoading(false);
      }
    };

    fetchStats();
  }, []);

  const recentActivity = [
    { id: 1, type: "new_feed", title: "TechCrunch added", time: "2 min ago" },
    {
      id: 2,
      type: "ai_summary",
      title: "5 articles summarized",
      time: "5 min ago",
    },
    {
      id: 3,
      type: "bookmark",
      title: "Article bookmarked",
      time: "12 min ago",
    },
    { id: 4, type: "read", title: "15 articles read", time: "1 hour ago" },
  ];

  const quickActions = [
    { id: 1, label: "Add Feed", icon: Plus, href: "/mobile/feeds/register" },
    { id: 2, label: "Search", icon: Search, href: "/mobile/articles/search" },
    { id: 3, label: "Browse", icon: Rss, href: "/desktop/feeds" },
    { id: 4, label: "Bookmarks", icon: Bookmark, href: "/mobile/bookmarks" },
  ];

  const sidebarNavItems = [
    {
      id: 1,
      label: "Dashboard",
      icon: Home,
      href: "/desktop/home",
      active: true,
    },
    { id: 2, label: "Feeds", icon: Rss, href: "/desktop/feeds" },
    { id: 3, label: "Articles", icon: FileText, href: "/desktop/articles" },
    { id: 4, label: "Search", icon: Search, href: "/desktop/articles/search" },
    { id: 5, label: "Settings", icon: Settings, href: "/desktop/settings" },
  ];

  return (
    <Box minH="100vh" bg="var(--app-bg)" position="relative">
      {/* Theme Toggle */}
      <Box position="fixed" top={4} right={4} zIndex={1000}>
        <ThemeToggle size="md" />
      </Box>

      <Flex minH="100vh">
        {/* Left Sidebar */}
        <Box
          w="250px"
          minH="100vh"
          p={6}
          className="glass"
          borderRadius="0"
          borderRight="1px solid var(--surface-border)"
          position="fixed"
          left={0}
          top={0}
          bg="var(--surface-bg)"
          backdropFilter="blur(var(--surface-blur))"
        >
          <VStack align="stretch" gap={8} h="full">
            {/* Logo */}
            <Box py={4}>
              <Text
                fontSize="xl"
                fontWeight="bold"
                bgGradient="var(--accent-gradient)"
                bgClip="text"
                className="gradient-text"
              >
                Alt Dashboard
              </Text>
              <Text fontSize="sm" color="var(--text-muted)" mt={1}>
                RSS Management Hub
              </Text>
            </Box>

            {/* Navigation */}
            <VStack align="stretch" gap={2} flex="1">
              {sidebarNavItems.map((item) => (
                <ChakraLink
                  key={item.id}
                  as={NextLink}
                  href={item.href}
                  textDecoration="none"
                  _hover={{ textDecoration: "none" }}
                >
                  <Flex
                    align="center"
                    gap={3}
                    p={4}
                    h="52px"
                    w="full"
                    borderRadius="var(--radius-lg)"
                    bg={item.active ? "var(--surface-hover)" : "transparent"}
                    border="1px solid"
                    borderColor={
                      item.active ? "var(--alt-primary)" : "transparent"
                    }
                    color={
                      item.active ? "var(--alt-primary)" : "var(--text-primary)"
                    }
                    transition="all var(--transition-speed) ease"
                    _hover={{
                      bg: "var(--surface-hover)",
                      transform: "translateX(4px)",
                      borderColor: "var(--alt-primary)",
                    }}
                  >
                    <Icon as={item.icon} boxSize={5} />
                    <Text fontSize="sm" fontWeight="medium">
                      {item.label}
                    </Text>
                  </Flex>
                </ChakraLink>
              ))}
            </VStack>
          </VStack>
        </Box>

        {/* Main Content */}
        <Box flex="1" ml="250px" p={8}>
          <VStack align="stretch" gap={8}>
            {/* Header */}
            <Box>
              <Text
                fontSize="3xl"
                fontWeight="bold"
                color="var(--text-primary)"
                mb={2}
              >
                Dashboard Overview
              </Text>
              <Text color="var(--text-muted)" fontSize="lg">
                Monitor your RSS feeds and AI-powered content insights
              </Text>
            </Box>

            {/* Stats Grid */}
            <Grid templateColumns="repeat(3, 1fr)" gap={6}>
              <GridItem>
                <Box
                  className="glass"
                  p={6}
                  borderRadius="var(--radius-xl)"
                  border="1px solid var(--surface-border)"
                  _hover={{
                    transform: "translateY(-2px)",
                    boxShadow: "0 8px 32px var(--alt-primary)20",
                  }}
                  transition="all var(--transition-speed) ease"
                >
                  <HStack justify="space-between" mb={4}>
                    <Icon as={Rss} color="var(--alt-primary)" boxSize={6} />
                    <Box
                      bg="var(--alt-primary)"
                      borderRadius="full"
                      p={1}
                      opacity={0.1}
                    />
                  </HStack>
                  {isLoading && !error && <Loading isLoading={isLoading} />}
                  {!isLoading && !error && (
                    <AnimatedNumber
                      value={feedStats?.feed_amount?.amount || 0}
                      duration={1000}
                      textProps={{
                        fontSize: "2xl",
                        fontWeight: "bold",
                        color: "var(--text-primary)",
                      }}
                    />
                  )}
                  {error && (
                    <Text fontSize="sm" color="var(--text-muted)" mt={1}>
                      {error}
                    </Text>
                  )}
                  <Text fontSize="sm" color="var(--text-muted)" mt={1}>
                    Total RSS Feeds
                  </Text>
                  <Text fontSize="xs" color="var(--alt-primary)" mt={2}>
                    +12% from last week
                  </Text>
                </Box>
              </GridItem>

              <GridItem>
                <Box
                  className="glass"
                  p={6}
                  borderRadius="var(--radius-xl)"
                  border="1px solid var(--surface-border)"
                  _hover={{
                    transform: "translateY(-2px)",
                    boxShadow: "0 8px 32px var(--alt-secondary)20",
                  }}
                  transition="all var(--transition-speed) ease"
                >
                  <HStack justify="space-between" mb={4}>
                    <Icon as={Zap} color="var(--alt-secondary)" boxSize={6} />
                    <Box
                      bg="var(--alt-secondary)"
                      borderRadius="full"
                      p={1}
                      opacity={0.1}
                    />
                  </HStack>
                  {isLoading && !error && <Loading isLoading={isLoading} />}
                  {!isLoading && !error && (
                    <AnimatedNumber
                      value={feedStats?.summarized_feed?.amount || 0}
                      duration={1200}
                      textProps={{
                        fontSize: "2xl",
                        fontWeight: "bold",
                        color: "var(--text-primary)",
                      }}
                    />
                  )}
                  {error && (
                    <Text fontSize="sm" color="var(--text-muted)" mt={1}>
                      {error}
                    </Text>
                  )}
                  <Text fontSize="sm" color="var(--text-muted)" mt={1}>
                    AI Summarized
                  </Text>
                  <Text fontSize="xs" color="var(--alt-secondary)" mt={2}>
                    +89% efficiency boost
                  </Text>
                </Box>
              </GridItem>

              <GridItem>
                <Box
                  className="glass"
                  p={6}
                  borderRadius="var(--radius-xl)"
                  border="1px solid var(--surface-border)"
                  _hover={{
                    transform: "translateY(-2px)",
                    boxShadow: "0 8px 32px var(--alt-tertiary)20",
                  }}
                  transition="all var(--transition-speed) ease"
                >
                  <HStack justify="space-between" mb={4}>
                    <Icon
                      as={FileText}
                      color="var(--alt-tertiary)"
                      boxSize={6}
                    />
                    <Box
                      bg="var(--alt-tertiary)"
                      borderRadius="full"
                      p={1}
                      opacity={0.1}
                    />
                  </HStack>
                  {isLoading && !error && <Loading isLoading={isLoading} />}
                  {!isLoading && !error && (
                    <AnimatedNumber
                      value={extraStats.unreadArticles}
                      duration={800}
                      textProps={{
                        fontSize: "2xl",
                        fontWeight: "bold",
                        color: "var(--text-primary)",
                      }}
                    />
                  )}
                  {error && (
                    <Text fontSize="sm" color="var(--text-muted)" mt={1}>
                      {error}
                    </Text>
                  )}
                  <Text fontSize="sm" color="var(--text-muted)" mt={1}>
                    Unread Articles
                  </Text>
                  <Text fontSize="xs" color="var(--alt-tertiary)" mt={2}>
                    New today: 23
                  </Text>
                </Box>
              </GridItem>
            </Grid>

            {/* Main Dashboard Grid */}
            <Grid templateColumns="repeat(2, 1fr)" gap={8}>
              {/* Recent Activity */}
              <GridItem>
                <Box
                  className="glass"
                  p={6}
                  borderRadius="var(--radius-xl)"
                  border="1px solid var(--surface-border)"
                  h="400px"
                >
                  <HStack justify="space-between" mb={6}>
                    <HStack>
                      <Icon
                        as={Activity}
                        color="var(--alt-primary)"
                        boxSize={5}
                      />
                      <Text
                        fontSize="lg"
                        fontWeight="semibold"
                        color="var(--text-primary)"
                      >
                        Recent Activity
                      </Text>
                    </HStack>
                    <Icon as={Clock} color="var(--text-muted)" boxSize={4} />
                  </HStack>

                  <VStack align="stretch" gap={4}>
                    {recentActivity.map((activity) => (
                      <Flex
                        key={activity.id}
                        align="center"
                        gap={3}
                        p={3}
                        borderRadius="var(--radius-md)"
                        _hover={{ bg: "var(--surface-hover)" }}
                        transition="all var(--transition-speed) ease"
                      >
                        <Box
                          w={8}
                          h={8}
                          borderRadius="full"
                          bg="var(--alt-primary)"
                          opacity={0.2}
                          display="flex"
                          alignItems="center"
                          justifyContent="center"
                        >
                          <Icon
                            as={
                              activity.type === "new_feed"
                                ? Plus
                                : activity.type === "ai_summary"
                                  ? Zap
                                  : activity.type === "bookmark"
                                    ? Bookmark
                                    : TrendingUp
                            }
                            color="var(--alt-primary)"
                            boxSize={4}
                          />
                        </Box>
                        <Box flex="1">
                          <Text
                            fontSize="sm"
                            color="var(--text-primary)"
                            fontWeight="medium"
                          >
                            {activity.title}
                          </Text>
                          <Text fontSize="xs" color="var(--text-muted)">
                            {activity.time}
                          </Text>
                        </Box>
                      </Flex>
                    ))}
                  </VStack>
                </Box>
              </GridItem>

              {/* Quick Actions */}
              <GridItem>
                <Box
                  className="glass"
                  p={6}
                  borderRadius="var(--radius-xl)"
                  border="1px solid var(--surface-border)"
                  h="400px"
                  display="flex"
                  flexDirection="column"
                >
                  <HStack justify="space-between" mb={6}>
                    <HStack>
                      <Icon as={Zap} color="var(--alt-secondary)" boxSize={5} />
                      <Text
                        fontSize="lg"
                        fontWeight="semibold"
                        color="var(--text-primary)"
                      >
                        Quick Actions
                      </Text>
                    </HStack>
                  </HStack>

                  <Grid
                    templateColumns="repeat(2, 1fr)"
                    gap={4}
                    mb={6}
                    flex="1"
                    alignItems="stretch"
                  >
                    {quickActions.map((action) => (
                      <GridItem key={action.id} display="flex">
                        <ChakraLink
                          as={NextLink}
                          href={action.href}
                          textDecoration="none"
                          _hover={{ textDecoration: "none" }}
                          w="full"
                        >
                          <Button
                            h="90px"
                            w="full"
                            minH="90px"
                            maxH="90px"
                            minW="120px"
                            bg="var(--surface-bg)"
                            border="1px solid var(--surface-border)"
                            borderRadius="var(--radius-lg)"
                            backdropFilter="blur(var(--surface-blur))"
                            display="flex"
                            flexDirection="column"
                            alignItems="center"
                            justifyContent="center"
                            gap={2}
                            color="var(--text-primary)"
                            p={3}
                            _hover={{
                              transform: "translateY(-2px)",
                              borderColor: "var(--alt-primary)",
                              boxShadow: "0 8px 24px rgba(0, 0, 0, 0.15)",
                              bg: "var(--surface-hover)",
                            }}
                            transition="all var(--transition-speed) ease"
                          >
                            <Icon
                              as={action.icon}
                              color="var(--alt-primary)"
                              boxSize={5}
                            />
                            <Text
                              fontSize="sm"
                              fontWeight="medium"
                              color="var(--text-primary)"
                              textAlign="center"
                              lineHeight="1.2"
                            >
                              {action.label}
                            </Text>
                          </Button>
                        </ChakraLink>
                      </GridItem>
                    ))}
                  </Grid>

                  {/* Additional Stats */}
                  <VStack align="stretch" gap={3} mt="auto">
                    <Flex justify="space-between" align="center">
                      <Text fontSize="sm" color="var(--text-muted)">
                        Weekly reads
                      </Text>
                      <Text
                        fontSize="sm"
                        fontWeight="bold"
                        color="var(--alt-primary)"
                      >
                        {extraStats.weeklyReads}
                      </Text>
                    </Flex>
                    <Flex justify="space-between" align="center">
                      <Text fontSize="sm" color="var(--text-muted)">
                        AI processed
                      </Text>
                      <Text
                        fontSize="sm"
                        fontWeight="bold"
                        color="var(--alt-secondary)"
                      >
                        {extraStats.aiProcessed}%
                      </Text>
                    </Flex>
                    <Flex justify="space-between" align="center">
                      <Text fontSize="sm" color="var(--text-muted)">
                        Bookmarks
                      </Text>
                      <Text
                        fontSize="sm"
                        fontWeight="bold"
                        color="var(--alt-tertiary)"
                      >
                        {extraStats.bookmarks}
                      </Text>
                    </Flex>
                  </VStack>
                </Box>
              </GridItem>
            </Grid>

            {/* Bottom Action Bar */}
            <Flex
              justify="space-between"
              align="center"
              p={6}
              className="glass"
              borderRadius="var(--radius-xl)"
              border="1px solid var(--surface-border)"
            >
              <Box>
                <Text
                  fontSize="lg"
                  fontWeight="semibold"
                  color="var(--text-primary)"
                >
                  Ready to explore?
                </Text>
                <Text fontSize="sm" color="var(--text-muted)">
                  Discover new content and manage your feeds
                </Text>
              </Box>
              <HStack gap={4}>
                <Button className="btn-primary">
                  <NextLink href="/desktop/feeds">
                    Browse Feeds
                    <ArrowRight size={16} style={{ marginLeft: "8px" }} />
                  </NextLink>
                </Button>
                <Button className="btn-primary">
                  <NextLink href="/mobile/feeds/register">
                    Add New Feed
                    <Download size={16} style={{ marginLeft: "8px" }} />
                  </NextLink>
                </Button>
              </HStack>
            </Flex>
          </VStack>
        </Box>
      </Flex>
    </Box>
  );
}
