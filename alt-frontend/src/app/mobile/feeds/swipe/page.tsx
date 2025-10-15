"use client";

import {
  Box,
  Button,
  Flex,
  Text,
  VStack,
  HStack,
  Link,
  Spinner,
} from "@chakra-ui/react";
import type { CSSObject } from "@emotion/react";
import { motion, AnimatePresence, useMotionValue, animate } from "framer-motion";
import { useDrag } from "@use-gesture/react";
import useSWRInfinite from "swr/infinite";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  JSX,
} from "react";
import { feedsApi } from "@/lib/api";
import { Sparkles, SquareArrowOutUpRight, BookOpen, BotMessageSquare } from "lucide-react";
import { CursorResponse } from "@/schema/common";
import { Feed } from "@/schema/feed";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import EmptyFeedState from "@/components/mobile/EmptyFeedState";
import ErrorState from "../_components/ErrorState";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

const PAGE_SIZE = 20;
const SWIPE_DISTANCE = 80;
const SWIPE_VELOCITY = 0.5;
const SWIPE_DURATION = 250;
const PREFETCH_THRESHOLD = 10;
const DISMISS_DELAY = 140;
const INITIAL_PAGE_COUNT = 3;

const MotionBox = motion.div;

const scrollAreaStyles: CSSObject = {
  "&::-webkit-scrollbar": {
    width: "4px",
  },
  "&::-webkit-scrollbar-track": {
    background: "transparent",
    borderRadius: "2px",
  },
  "&::-webkit-scrollbar-thumb": {
    background: "rgba(255, 255, 255, 0.2)",
    borderRadius: "2px",
  },
  "&::-webkit-scrollbar-thumb:hover": {
    background: "rgba(255, 255, 255, 0.3)",
  },
};

type SwrKey = readonly ["mobile-feed-swipe", string | undefined, number];

const canonicalize = (url: string) => {
  try {
    const parsed = new URL(url);
    parsed.hash = "";
    [
      "utm_source",
      "utm_medium",
      "utm_campaign",
      "utm_term",
      "utm_content",
    ].forEach((param) => parsed.searchParams.delete(param));
    if (parsed.pathname !== "/" && parsed.pathname.endsWith("/")) {
      parsed.pathname = parsed.pathname.slice(0, -1);
    }
    return parsed.toString();
  } catch {
    return url;
  }
};

const getKey = (
  pageIndex: number,
  previousPageData: CursorResponse<Feed> | null,
): SwrKey | null => {
  if (previousPageData && !previousPageData.next_cursor) {
    return null;
  }

  if (pageIndex === 0) {
    return ["mobile-feed-swipe", undefined, PAGE_SIZE];
  }

  const cursor = previousPageData?.next_cursor ?? undefined;
  return ["mobile-feed-swipe", cursor, PAGE_SIZE];
};

const fetchPage = async (
  _: string,
  cursor: string | undefined,
  limit: number,
): Promise<CursorResponse<Feed>> => {
  return feedsApi.getFeedsWithCursor(cursor, limit);
};

export default function SwipeFeedsPage(): JSX.Element {
  const [liveRegionMessage, setLiveRegionMessage] = useState("");
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [activeIndex, setActiveIndex] = useState(0);
  const [isSummaryExpanded, setIsSummaryExpanded] = useState(false);
  const [summary, setSummary] = useState<string | null>(null);
  const [isLoadingSummary, setIsLoadingSummary] = useState(false);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const [isSummarizing, setIsSummarizing] = useState(false);
  const [isContentExpanded, setIsContentExpanded] = useState(false);
  const [fullContent, setFullContent] = useState<string | null>(null);
  const [isLoadingContent, setIsLoadingContent] = useState(false);
  const [contentError, setContentError] = useState<string | null>(null);

  const x = useMotionValue(0);
  const animationInFlightRef = useRef(false);
  const prefetchCursorRef = useRef<string | null>(null);
  const liveRegionTimeoutRef = useRef<number | null>(null);

  const { data, error, isLoading, isValidating, size, setSize, mutate } =
    useSWRInfinite(getKey, fetchPage, {
      revalidateOnFocus: false,
      revalidateFirstPage: false,
      parallel: true,
      initialSize: INITIAL_PAGE_COUNT,
    });

  const feeds = useMemo(() => {
    if (!data || data.length === 0) {
      return [] as Feed[];
    }
    return data.flatMap((page) => page?.data ?? []);
  }, [data]);

  const activeFeed = feeds[activeIndex] ?? null;
  const lastPage = data?.[data.length - 1] ?? null;
  const hasMore = Boolean(lastPage?.next_cursor);
  const isInitialLoading = (!data || data.length === 0) && isLoading;

  useEffect(() => {
    if (!statusMessage) {
      return;
    }
    const timeout = window.setTimeout(() => {
      setStatusMessage(null);
    }, 2000);
    return () => window.clearTimeout(timeout);
  }, [statusMessage]);

  useEffect(() => {
    return () => {
      if (liveRegionTimeoutRef.current) {
        window.clearTimeout(liveRegionTimeoutRef.current);
      }
    };
  }, []);

  const schedulePrefetch = useCallback(() => {
    if (!hasMore || !lastPage) {
      prefetchCursorRef.current = null;
      return;
    }

    const nextCursor = lastPage.next_cursor;
    const remaining = feeds.length - activeIndex;

    if (
      nextCursor &&
      remaining <= PREFETCH_THRESHOLD &&
      remaining >= 0 &&
      !isValidating &&
      prefetchCursorRef.current !== nextCursor
    ) {
      prefetchCursorRef.current = nextCursor;
      setSize(size + 1);
    }
  }, [activeIndex, feeds.length, hasMore, isValidating, lastPage, setSize, size]);

  useEffect(() => {
    schedulePrefetch();
  }, [schedulePrefetch]);

  const resetPosition = useCallback(() => {
    animate(x, 0, {
      type: "spring",
      stiffness: 320,
      damping: 30,
    });
  }, [x]);

  const playDismissAnimation = useCallback(
    async (direction: number) => {
      if (animationInFlightRef.current) {
        return;
      }
      animationInFlightRef.current = true;
      const width =
        typeof window !== "undefined" && window.innerWidth
          ? window.innerWidth
          : 480;
      animate(x, direction * width * 1.2, {
        type: "spring",
        stiffness: 240,
        damping: 24,
      });

      await new Promise<void>((resolve) => {
        window.setTimeout(() => {
          x.set(0);
          animationInFlightRef.current = false;
          resolve();
        }, DISMISS_DELAY);
      });
    },
    [x],
  );

  const dismissCurrentFeed = useCallback(
    async (direction: number) => {
      const current = feeds[activeIndex];
      if (!current) {
        return;
      }

      await playDismissAnimation(direction === 0 ? 1 : direction);

      setActiveIndex((prev) => prev + 1);
      // Reset summary and content state when moving to next card
      setIsSummaryExpanded(false);
      setSummary(null);
      setSummaryError(null);
      setIsContentExpanded(false);
      setFullContent(null);
      setContentError(null);

      const canonicalLink = canonicalize(current.link);
      setLiveRegionMessage("Feed marked as read");
      setStatusMessage("Feed marked as read");
      if (liveRegionTimeoutRef.current) {
        window.clearTimeout(liveRegionTimeoutRef.current);
      }
      liveRegionTimeoutRef.current = window.setTimeout(() => {
        setLiveRegionMessage("");
        liveRegionTimeoutRef.current = null;
      }, 1000);

      try {
        await feedsApi.updateFeedReadStatus(canonicalLink);
        await mutate(undefined, { revalidate: false });
      } catch (err) {
        console.error("Failed to mark feed as read", err);
        setActiveIndex((prev) => Math.max(prev - 1, 0));
        setLiveRegionMessage("Failed to mark feed as read");
        setStatusMessage("Failed to mark feed as read");
        if (liveRegionTimeoutRef.current) {
          window.clearTimeout(liveRegionTimeoutRef.current);
        }
        liveRegionTimeoutRef.current = window.setTimeout(() => {
          setLiveRegionMessage("");
          liveRegionTimeoutRef.current = null;
        }, 1500);
        resetPosition();
      }
    },
    [activeIndex, feeds, mutate, playDismissAnimation, resetPosition],
  );

  const handleToggleContent = useCallback(async () => {
    if (!activeFeed?.link) return;

    if (!isContentExpanded && !fullContent) {
      setIsLoadingContent(true);
      setContentError(null);

      try {
        const contentResponse = await feedsApi.getFeedContentOnTheFly({
          feed_url: activeFeed.link,
        });
        if (contentResponse.content) {
          setFullContent(contentResponse.content);

          // Auto-archive article when displaying content
          // This ensures the article exists in DB before summarization
          feedsApi.archiveContent(activeFeed.link, activeFeed.title).catch((err) => {
            console.warn("Failed to auto-archive article:", err);
            // Don't block UI on archive failure
          });
        } else {
          setContentError("記事全文を取得できませんでした");
        }
      } catch (error) {
        console.error("Error fetching content:", error);
        setContentError("記事全文を取得できませんでした");
      } finally {
        setIsLoadingContent(false);
      }
    }
    setIsContentExpanded(!isContentExpanded);
  }, [activeFeed, isContentExpanded, fullContent]);

  const handleToggleSummary = useCallback(async () => {
    if (!activeFeed?.link) return;

    if (!isSummaryExpanded && !summary) {
      setIsLoadingSummary(true);
      setSummaryError(null);

      try {
        const summaryResponse = await feedsApi.getArticleSummary(activeFeed.link);
        if (summaryResponse.matched_articles && summaryResponse.matched_articles.length > 0) {
          setSummary(summaryResponse.matched_articles[0].content);
        } else {
          setSummaryError("要約を取得できませんでした");
        }
      } catch (error) {
        console.error("Error fetching summary:", error);
        setSummaryError("要約を取得できませんでした");
      } finally {
        setIsLoadingSummary(false);
      }
    }
    setIsSummaryExpanded(!isSummaryExpanded);
  }, [activeFeed, isSummaryExpanded, summary]);

  const handleSummarizeNow = useCallback(async () => {
    if (!activeFeed?.link) return;

    setIsSummarizing(true);
    setSummaryError(null);

    try {
      const summarizeResponse = await feedsApi.summarizeArticle(activeFeed.link);

      if (summarizeResponse.success && summarizeResponse.summary) {
        setSummary(summarizeResponse.summary);
        setSummaryError(null);
      } else {
        setSummaryError("要約の生成に失敗しました");
      }
    } catch (error) {
      console.error("Error summarizing article:", error);
      setSummaryError("要約の生成中にエラーが発生しました");
    } finally {
      setIsSummarizing(false);
    }
  }, [activeFeed]);

  const dragHandlers = useDrag(
    ({ down, movement: [mx], velocity: [vx], direction: [dx], last }) => {
      if (!activeFeed) {
        return;
      }

      if (down) {
        x.set(mx);
        return;
      }

      if (last) {
        const shouldDismiss =
          Math.abs(mx) >= SWIPE_DISTANCE || Math.abs(vx) >= SWIPE_VELOCITY;
        if (!shouldDismiss) {
          resetPosition();
          return;
        }

        const direction = dx !== 0 ? dx : Math.sign(mx) || 1;
        dismissCurrentFeed(direction);
      }
    },
    {
      axis: "x",
      swipe: {
        distance: SWIPE_DISTANCE,
        velocity: SWIPE_VELOCITY,
        duration: SWIPE_DURATION,
      },
      pointer: { touch: true },
    },
  );

  const bind = () => {
    const handlers = dragHandlers();
    const { onDrag, onDragStart, onDragEnd, onAnimationStart, ...rest } = handlers as any;
    return rest;
  };

  const retry = useCallback(async () => {
    try {
      await mutate(undefined, { revalidate: true });
    } catch (err) {
      console.error("Retry failed", err);
      throw err;
    }
  }, [mutate]);

  if (isInitialLoading) {
    return (
      <Box minH="100vh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100vh"
          data-testid="swipe-skeleton-container"
        >
          <Flex direction="column" gap={4}>
            {Array.from({ length: 5 }).map((_, index) => (
              <SkeletonFeedCard key={`swipe-skeleton-${index}`} />
            ))}
          </Flex>
        </Box>
        <FloatingMenu />
      </Box>
    );
  }

  if (error) {
    return <ErrorState error={error} onRetry={retry} isLoading={isValidating} />;
  }

  // Show empty state only when all feeds are consumed AND no more available
  if (!activeFeed || activeIndex >= feeds.length) {
    // If there are more pages to load, show loading state instead of empty
    if (hasMore || isValidating) {
      return (
        <Box minH="100vh" position="relative">
          <Box
            p={5}
            maxW="container.sm"
            mx="auto"
            height="100vh"
            data-testid="swipe-skeleton-container"
          >
            <Flex direction="column" gap={4}>
              {Array.from({ length: 5 }).map((_, index) => (
                <SkeletonFeedCard key={`swipe-skeleton-loading-${index}`} />
              ))}
            </Flex>
          </Box>
          <FloatingMenu />
        </Box>
      );
    }

    // Only show empty state when truly no feeds available
    return (
      <Box minH="100vh" position="relative">
        <EmptyFeedState />
        <FloatingMenu />
      </Box>
    );
  }

  return (
    <Box minH="100vh" position="relative">
      <Box
        aria-live="polite"
        aria-atomic="true"
        position="absolute"
        left="-10000px"
        width="1px"
        height="1px"
        overflow="hidden"
      >
        {liveRegionMessage}
      </Box>

      <Flex
        direction="column"
        align="center"
        justify="center"
        h="100dvh"
        px={4}
        style={{
          overscrollBehavior: "contain",
          touchAction: "pan-y",
        }}
      >
        <AnimatePresence initial={false} mode="popLayout">
          {activeFeed && (
            <MotionBox
              key={activeFeed.id}
              {...bind()}
              style={{
                x,
                willChange: "transform",
                position: "relative",
                width: "100%",
                maxWidth: "30rem",
                height: "92dvh",
                background: "var(--alt-glass)",
                color: "var(--alt-text-primary)",
                border: "2px solid var(--alt-glass-border)",
                boxShadow: "0 12px 40px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.1)",
                borderRadius: "1rem",
                padding: "1.5rem",
                backdropFilter: "blur(20px)",
              }}
              initial={{ scale: 0.98, opacity: 0.96 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ opacity: 0 }}
              data-testid="swipe-card"
            >
              <VStack align="stretch" gap={0} h="100%">
                {/* Fixed Header */}
                <Box
                  position="relative"
                  zIndex="2"
                  bg="rgba(255, 255, 255, 0.03)"
                  backdropFilter="blur(20px)"
                  borderBottom="1px solid var(--alt-glass-border)"
                  px={4}
                  py={3}
                  borderTopRadius="1rem"
                >
                  <Text
                    fontSize="sm"
                    color="var(--alt-text-secondary)"
                    mb={2}
                    textTransform="uppercase"
                    letterSpacing="0.08em"
                    bgGradient="var(--accent-gradient)"
                    bgClip="text"
                    fontWeight="semibold"
                  >
                    Swipe to mark as read
                  </Text>

                  {/* Title with External Link */}
                  <Flex align="center" gap={2}>
                    <Link
                      href={activeFeed.link}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label="Open article in new tab"
                      display="flex"
                      alignItems="center"
                      justifyContent="center"
                      color="var(--alt-text-primary)"
                      borderColor="var(--alt-glass-border)"
                      border="1px solid"
                      borderRadius="md"
                      p={2}
                      _hover={{
                        bg: "rgba(255, 255, 255, 0.05)",
                        borderColor: "var(--alt-primary)",
                      }}
                    >
                      <SquareArrowOutUpRight color="var(--alt-primary)" size={20} />
                    </Link>
                    <Text as="h2" fontSize="xl" fontWeight="bold" flex="1">
                      {activeFeed.title}
                    </Text>
                  </Flex>

                  {activeFeed.published && (
                    <Text color="var(--alt-text-secondary)" fontSize="sm" mt={2}>
                      {new Date(activeFeed.published).toLocaleString()}
                    </Text>
                  )}
                </Box>

                {/* Unified Scroll Area */}
                <Box
                  flex="1"
                  overflow="auto"
                  px={4}
                  py={4}
                  bg="transparent"
                  scrollBehavior="smooth"
                  overscrollBehavior="contain"
                  css={scrollAreaStyles}
                  data-testid="unified-scroll-area"
                >
                  {/* 1. Lead Text (Description) - Always visible */}
                  {activeFeed.description && (
                    <Box mb={4}>
                      <Text
                        fontSize="xs"
                        color="var(--alt-text-secondary)"
                        fontWeight="bold"
                        mb={2}
                        textTransform="uppercase"
                        letterSpacing="1px"
                      >
                        概要 / Summary
                      </Text>
                      <Text
                        fontSize="sm"
                        color="var(--alt-text-primary)"
                        lineHeight="1.7"
                      >
                        {activeFeed.description}
                      </Text>
                    </Box>
                  )}

                  {/* 2. Full Article Content - When expanded */}
                  {isContentExpanded && (
                    <Box
                      mb={4}
                      p={4}
                      bg="rgba(255, 255, 255, 0.03)"
                      borderRadius="12px"
                      border="1px solid var(--alt-glass-border)"
                    >
                      <Text
                        fontSize="xs"
                        color="var(--alt-text-secondary)"
                        fontWeight="bold"
                        mb={2}
                        textTransform="uppercase"
                        letterSpacing="1px"
                      >
                        記事全文 / Full Article
                      </Text>
                      {isLoadingContent ? (
                        <HStack justify="center" py={4}>
                          <Spinner size="sm" color="var(--alt-primary)" />
                          <Text color="var(--alt-text-secondary)" fontSize="sm">
                            記事全文を読み込み中...
                          </Text>
                        </HStack>
                      ) : contentError ? (
                        <Text color="var(--alt-text-secondary)" fontSize="sm" textAlign="center">
                          {contentError}
                        </Text>
                      ) : fullContent ? (
                        <Box
                          fontSize="sm"
                          color="var(--alt-text-primary)"
                          lineHeight="1.7"
                          dangerouslySetInnerHTML={{ __html: fullContent }}
                          css={{
                            "& img": {
                              maxWidth: "100%",
                              height: "auto",
                              borderRadius: "8px",
                              margin: "0.5rem 0",
                            },
                            "& a": {
                              color: "var(--alt-primary)",
                              textDecoration: "underline",
                            },
                            "& p": {
                              marginBottom: "0.5rem",
                            },
                            "& h1, & h2, & h3, & h4, & h5, & h6": {
                              fontWeight: "bold",
                              marginTop: "0.75rem",
                              marginBottom: "0.5rem",
                            },
                          }}
                        />
                      ) : null}
                    </Box>
                  )}

                  {/* 3. Summary - When expanded */}
                  {isSummaryExpanded && (
                    <Box
                      p={4}
                      bg="rgba(255, 255, 255, 0.03)"
                      borderRadius="12px"
                      border="1px solid var(--alt-glass-border)"
                    >
                      <Text
                        fontSize="xs"
                        color="var(--alt-text-secondary)"
                        fontWeight="bold"
                        mb={2}
                        textTransform="uppercase"
                        letterSpacing="1px"
                      >
                        日本語要約 / Japanese Summary
                      </Text>
                      {isLoadingSummary ? (
                        <HStack justify="center" py={4}>
                          <Spinner size="sm" color="var(--alt-primary)" />
                          <Text color="var(--alt-text-secondary)" fontSize="sm">
                            要約を読み込み中...
                          </Text>
                        </HStack>
                      ) : isSummarizing ? (
                        <VStack gap={3} py={4}>
                          <HStack justify="center">
                            <Spinner size="sm" color="var(--alt-primary)" />
                            <Text color="var(--alt-text-secondary)" fontSize="sm">
                              要約を生成中...
                            </Text>
                          </HStack>
                          <Text color="var(--alt-text-secondary)" fontSize="xs" textAlign="center">
                            これには数秒かかる場合があります
                          </Text>
                        </VStack>
                      ) : summaryError ? (
                        <VStack gap={3} w="100%">
                          <Text color="var(--alt-text-secondary)" fontSize="sm" textAlign="center">
                            {summaryError}
                          </Text>
                          {summaryError === "要約を取得できませんでした" && (
                            <Button
                              size="sm"
                              onClick={handleSummarizeNow}
                              w="100%"
                              borderRadius="12px"
                              bg="var(--alt-primary)"
                              color="white"
                              _hover={{
                                bg: "var(--alt-secondary)",
                                transform: "translateY(-1px)",
                              }}
                              data-testid="summarize-now-button"
                            >
                              <Flex align="center" gap={2}>
                                <Sparkles size={16} />
                                <Text>今すぐ要約</Text>
                              </Flex>
                            </Button>
                          )}
                        </VStack>
                      ) : summary ? (
                        <Text
                          fontSize="sm"
                          color="var(--alt-text-primary)"
                          lineHeight="1.7"
                          whiteSpace="pre-wrap"
                        >
                          {summary}
                        </Text>
                      ) : (
                        <VStack gap={3} w="100%">
                          <Text color="var(--alt-text-secondary)" fontSize="sm" textAlign="center">
                            この記事の要約はまだありません
                          </Text>
                          <Button
                            size="sm"
                            onClick={handleSummarizeNow}
                            w="100%"
                            borderRadius="12px"
                            bg="var(--alt-primary)"
                            color="white"
                            _hover={{
                              bg: "var(--alt-secondary)",
                              transform: "translateY(-1px)",
                            }}
                            data-testid="summarize-now-button"
                          >
                            <Flex align="center" gap={2}>
                              <Sparkles size={16} />
                              <Text>今すぐ要約</Text>
                            </Flex>
                          </Button>
                        </VStack>
                      )}
                    </Box>
                  )}
                </Box>

                {/* Fixed Footer with Action Buttons */}
                <Box
                  position="relative"
                  zIndex="2"
                  bg="rgba(255, 255, 255, 0.05)"
                  backdropFilter="blur(20px)"
                  borderTop="1px solid var(--alt-glass-border)"
                  px={3}
                  py={3}
                  borderBottomRadius="1rem"
                  data-testid="action-footer"
                >
                  <HStack gap={2} w="100%" justify="space-between">
                    <Button
                      onClick={handleToggleContent}
                      size="sm"
                      flex="1"
                      borderRadius="12px"
                      bg={isContentExpanded ? "var(--alt-secondary)" : "var(--alt-primary)"}
                      color="white"
                      fontWeight="bold"
                      _hover={{
                        filter: "brightness(1.1)",
                        transform: "translateY(-1px)",
                      }}
                      _active={{
                        transform: "translateY(0)",
                      }}
                      transition="all 0.2s ease"
                      disabled={isLoadingContent}
                      data-testid="toggle-content-button"
                    >
                      <Flex align="center" gap={2}>
                        <BookOpen size={16} />
                        <Text fontSize="xs">
                          {isLoadingContent ? "読込中..." : isContentExpanded ? "全文非表示" : "全文表示"}
                        </Text>
                      </Flex>
                    </Button>

                    <Button
                      onClick={handleToggleSummary}
                      size="sm"
                      flex="1"
                      borderRadius="12px"
                      bg={isSummaryExpanded ? "var(--alt-secondary)" : "var(--alt-primary)"}
                      color="white"
                      fontWeight="bold"
                      _hover={{
                        filter: "brightness(1.1)",
                        transform: "translateY(-1px)",
                      }}
                      _active={{
                        transform: "translateY(0)",
                      }}
                      transition="all 0.2s ease"
                      disabled={isLoadingSummary}
                      data-testid="toggle-summary-button"
                    >
                      <Flex align="center" gap={2}>
                        <BotMessageSquare size={16} />
                        <Text fontSize="xs">
                          {isLoadingSummary ? "読込中..." : isSummaryExpanded ? "要約非表示" : "要約"}
                        </Text>
                      </Flex>
                    </Button>

                    <Button
                      type="button"
                      onClick={() => dismissCurrentFeed(1)}
                      size="sm"
                      flex="1"
                      borderRadius="12px"
                      bgGradient="linear(to-r, #FF416C, #FF4B2B)"
                      color="white"
                      fontWeight="bold"
                      _hover={{
                        transform: "translateY(-1px)",
                        boxShadow: "0 4px 12px rgba(255, 65, 108, 0.3)",
                      }}
                      _active={{
                        transform: "translateY(0)",
                      }}
                      transition="all 0.2s ease"
                      data-testid="swipe-card-button"
                    >
                      <Text fontSize="xs">既読</Text>
                    </Button>
                  </HStack>

                  {statusMessage && (
                    <Text
                      fontSize="xs"
                      color="var(--alt-text-secondary)"
                      textAlign="center"
                      mt={2}
                    >
                      {statusMessage}
                    </Text>
                  )}
                </Box>
              </VStack>
            </MotionBox>
          )}
        </AnimatePresence>
      </Flex>

      <FloatingMenu />
    </Box>
  );
}
