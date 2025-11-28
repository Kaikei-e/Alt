"use client";

import {
  Box,
  Button,
  Flex,
  HStack,
  Link,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import type { CSSObject } from "@emotion/react";
import { useDrag } from "@use-gesture/react";
import { animated, useSpring, useTransition } from "@react-spring/web";
import {
  Archive,
  BookOpen,
  BotMessageSquare,
  Sparkles,
  SquareArrowOutUpRight,
} from "lucide-react";
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { articleApi } from "@/lib/api";
import type { RenderFeed } from "@/schema/feed";
import { renderingRegistry } from "@/utils/renderingStrategies";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";

const SWIPE_DISTANCE = 80;
const SWIPE_VELOCITY = 0.5;
const DISMISS_DELAY = 140;

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

type SwipeFeedCardProps = {
  feed: RenderFeed;
  statusMessage: string | null;
  onDismiss: (direction: number) => Promise<void> | void;
  getCachedContent?: (feedUrl: string) => string | null;
  isBusy?: boolean;
  initialArticleContent?: SafeHtmlString;
};

const buildContentStyles = (): CSSObject => ({
  "& img": {
    maxWidth: "100%",
    height: "auto",
    borderRadius: "8px",
    marginBottom: "0.75rem",
  },
  "& h1, & h2, & h3, & h4, & h5, & h6": {
    fontWeight: "bold",
    marginTop: "0.75rem",
    marginBottom: "0.5rem",
  },
  "& p": {
    marginBottom: "0.5rem",
    lineHeight: 1.7,
  },
  "& ul, & ol": {
    marginLeft: "1.25rem",
    marginBottom: "0.75rem",
  },
});

const normalizeDirection = (direction: number) => {
  if (!direction) {
    return 1;
  }
  return direction;
};

// Inner component that handles the drag logic and content state
const CardView = memo(
  ({
    feed,
    statusMessage,
    onDismiss,
    getCachedContent,
    isBusy = false,
    style,
    initialArticleContent,
  }: SwipeFeedCardProps & { style: any }) => {
    const [isSummaryExpanded, setIsSummaryExpanded] = useState(false);
    const [summary, setSummary] = useState<string | null>(null);
    const [isLoadingSummary, setIsLoadingSummary] = useState(false);
    const [summaryError, setSummaryError] = useState<string | null>(null);
    const [isSummarizing, setIsSummarizing] = useState(false);

    const [isContentExpanded, setIsContentExpanded] = useState(false);
    // Initialize with server-fetched content if available
    const [fullContent, setFullContent] = useState<string | null>(
      initialArticleContent ?? null,
    );
    const [isLoadingContent, setIsLoadingContent] = useState(false);
    const [contentError, setContentError] = useState<string | null>(null);

    const [isArchiving, setIsArchiving] = useState(false);
    const [isArchived, setIsArchived] = useState(false);

    const [{ x }, api] = useSpring(() => ({ x: 0 }));
    const animationInFlightRef = useRef(false);

    const sanitizedFullContent = useMemo(() => {
      if (!fullContent) {
        return null;
      }

      return renderingRegistry.render(fullContent, undefined, feed.link);
    }, [feed.link, fullContent]);

    // Auto-fetch article content when card is mounted or feed changes
    useEffect(() => {
      // Skip if content is already loaded (from initialArticleContent or previous fetch)
      if (fullContent) {
        return;
      }

      // Skip if we have initialArticleContent (already set in useState initializer)
      if (initialArticleContent) {
        return;
      }

      // Check cache first if getCachedContent is available
      const cachedContent = getCachedContent?.(feed.link);

      if (cachedContent) {
        // Use cached content instantly
        setFullContent(cachedContent);
        return;
      }

      // Cache miss - fetch in background
      // Don't set loading state to avoid UI flicker
      let isCancelled = false;

      const fetchContent = async () => {
        try {
          const contentResponse = await articleApi.getFeedContentOnTheFly({
            feed_url: feed.link,
          });

          if (isCancelled) {
            return;
          }

          if (contentResponse.content) {
            setFullContent(contentResponse.content);
            // Archive article in background (non-blocking)
            articleApi
              .archiveContent(feed.link, feed.title)
              .catch((err) =>
                console.warn("Failed to auto-archive article:", err),
              );
          }
        } catch (error) {
          // Only log error, don't update UI state
          // User can still click the button to retry
          if (!isCancelled) {
            console.error(
              "[SwipeFeedCard] Error auto-fetching content:",
              error,
            );
          }
        }
      };

      void fetchContent();

      return () => {
        isCancelled = true;
      };
    }, [
      feed.link,
      feed.title,
      fullContent,
      getCachedContent,
      initialArticleContent,
    ]);

    const resetPosition = useCallback(() => {
      api.start({
        x: 0,
        config: { tension: 250, friction: 25 },
      });
    }, [api]);

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

        return new Promise<void>((resolve) => {
          api.start({
            x: direction * width * 1.2,
            config: { tension: 350, friction: 25 },
            onRest: () => {
              animationInFlightRef.current = false;
              resolve();
            },
          });
        });
      },
      [api],
    );

    const handleToggleContent = useCallback(async () => {
      if (!isContentExpanded && !fullContent) {
        // Check cache first if getCachedContent is available
        const cachedContent = getCachedContent?.(feed.link);

        if (cachedContent) {
          // Use cached content instantly
          setFullContent(cachedContent);
          setIsContentExpanded(true);
          return;
        }

        // Cache miss or no cache available - fetch normally
        setIsLoadingContent(true);
        setContentError(null);

        try {
          const contentResponse = await articleApi.getFeedContentOnTheFly({
            feed_url: feed.link,
          });
          if (contentResponse.content) {
            setFullContent(contentResponse.content);
            articleApi
              .archiveContent(feed.link, feed.title)
              .catch((err) =>
                console.warn("Failed to auto-archive article:", err),
              );
          } else {
            setContentError("Could not fetch article content");
          }
        } catch (error) {
          console.error("Error fetching content:", error);
          setContentError("Could not fetch article content");
        } finally {
          setIsLoadingContent(false);
        }
      }

      setIsContentExpanded((prev) => !prev);
    }, [
      feed.link,
      feed.title,
      fullContent,
      isContentExpanded,
      getCachedContent,
    ]);

    const fetchSummary = useCallback(async () => {
      setIsLoadingSummary(true);
      setSummaryError(null);

      try {
        const summaryResponse = await articleApi.getArticleSummary(feed.link);
        if (
          summaryResponse.matched_articles &&
          summaryResponse.matched_articles.length > 0
        ) {
          setSummary(summaryResponse.matched_articles[0].content);
          setSummaryError(null);
        } else {
          setSummaryError("Could not fetch summary");
        }
      } catch (error) {
        console.error("Error fetching summary:", error);
        setSummaryError("Could not fetch summary");
      } finally {
        setIsLoadingSummary(false);
      }
    }, [feed.link]);

    const handleToggleSummary = useCallback(async () => {
      if (!isSummaryExpanded && !summary) {
        await fetchSummary();
      }

      setIsSummaryExpanded((prev) => !prev);
    }, [feed.link, isSummaryExpanded, summary, fetchSummary]);

    const handleSummarizeNow = useCallback(async () => {
      setIsSummarizing(true);
      setSummaryError(null);

      try {
        const summarizeResponse = await articleApi.summarizeArticle(feed.link);
        console.log("[SwipeFeedCard] Summarize response:", summarizeResponse);

        if (summarizeResponse.success && summarizeResponse.summary) {
          console.log(
            "[SwipeFeedCard] Summary received, length:",
            summarizeResponse.summary.length,
          );
          setSummary(summarizeResponse.summary);
          setSummaryError(null);
        } else {
          console.error(
            "[SwipeFeedCard] Invalid response structure:",
            summarizeResponse,
          );
          setSummaryError("Failed to generate the summary");
        }
      } catch (error) {
        console.error("[SwipeFeedCard] Error summarizing article:", error);
        setSummaryError("Failed to generate the summary");
      } finally {
        setIsSummarizing(false);
      }
    }, [feed.link]);

    const handleDismiss = useCallback(
      async (direction: number) => {
        const normalized = normalizeDirection(direction);

        try {
          // Trigger animation and dismissal in parallel
          // We don't await the animation to finish before calling onDismiss
          // This allows the next card to start rendering immediately
          const animationPromise = playDismissAnimation(normalized);

          // Small delay to ensure animation frame has started
          // This prevents the state update from happening before the transform is applied
          await new Promise((resolve) => setTimeout(resolve, 10));

          await onDismiss(normalized);

          // Ensure animation completes (though component might be unmounted by then)
          await animationPromise;
        } catch (error) {
          resetPosition();
          throw error;
        }
      },
      [onDismiss, playDismissAnimation, resetPosition],
    );

    const bind = useDrag(
      ({ down, movement: [mx], velocity: [vx], direction: [dx], last }) => {
        if (down) {
          api.start({ x: mx, immediate: true });
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
          void handleDismiss(direction);
        }
      },
      {
        axis: "x",
        filterTaps: true,
        pointer: { touch: true },
      },
    );

    const hasDescription = Boolean(feed.description);
    const publishedLabel = useMemo(() => {
      // Prefer created_at if available, as it's a reliable ISO timestamp
      if (feed.created_at) {
        try {
          return new Date(feed.created_at).toLocaleString();
        } catch {
          // Fallback to published if created_at parsing fails
        }
      }

      if (!feed.published) {
        return null;
      }
      try {
        return new Date(feed.published).toLocaleString();
      } catch {
        return feed.published;
      }
    }, [feed.published, feed.created_at]);

    return (
      <animated.div
        {...bind()}
        style={{
          ...style,
          x,
          touchAction: "none",
          position: "absolute", // Important for stacking during transition
          width: "100%",
          maxWidth: "30rem",
          height: "95dvh",
          background: "var(--alt-glass)",
          color: "var(--alt-text-primary)",
          border: "2px solid var(--alt-glass-border)",
          boxShadow:
            "0 12px 40px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.1)",
          borderRadius: "1rem",
          padding: "1rem",
          backdropFilter: "blur(20px)",
          willChange: "transform, opacity",
          zIndex: style.zIndex, // Apply zIndex from transition
        }}
        data-testid="swipe-card"
        aria-busy={isBusy}
      >
        <VStack align="stretch" gap={0} h="100%">
          <Box
            position="relative"
            zIndex="2"
            bg="rgba(255, 255, 255, 0.03)"
            backdropFilter="blur(20px)"
            borderBottom="1px solid var(--alt-glass-border)"
            px={2}
            py={2}
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

            <Flex align="center" gap={2}>
              <Link
                href={feed.link}
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
                flex="1"
                minWidth={0}
                _hover={{
                  bg: "rgba(255, 255, 255, 0.05)",
                  borderColor: "var(--alt-primary)",
                }}
              >
                <Box flexShrink={0}>
                  <SquareArrowOutUpRight color="var(--alt-primary)" size={20} />
                </Box>
                <Text
                  as="h2"
                  fontSize="xl"
                  fontWeight="bold"
                  flex="1"
                  wordBreak="break-word"
                  whiteSpace="normal"
                  minWidth={0}
                >
                  {feed.title}
                </Text>
              </Link>
            </Flex>

            {publishedLabel && (
              <Text color="var(--alt-text-secondary)" fontSize="sm" mt={2}>
                {publishedLabel}
              </Text>
            )}
          </Box>

          <Box
            flex="1"
            overflow="auto"
            px={2}
            py={2}
            bg="transparent"
            scrollBehavior="smooth"
            overscrollBehavior="contain"
            css={scrollAreaStyles}
            data-testid="unified-scroll-area"
          >
            {hasDescription && (
              <Box mb={4}>
                <Text
                  fontSize="xs"
                  color="var(--alt-text-secondary)"
                  fontWeight="bold"
                  mb={2}
                  textTransform="uppercase"
                  letterSpacing="1px"
                >
                  Summary
                </Text>
                <Text
                  fontSize="sm"
                  color="var(--alt-text-primary)"
                  lineHeight="1.7"
                >
                  {feed.description}
                </Text>
              </Box>
            )}

            {isContentExpanded && (
              <Box
                mb={4}
                p={4}
                bg="rgba(255, 255, 255, 0.03)"
                borderRadius="12px"
                border="1px solid var(--alt-glass-border)"
                data-testid="content-section"
              >
                <Text
                  fontSize="xs"
                  color="var(--alt-text-secondary)"
                  fontWeight="bold"
                  mb={2}
                  textTransform="uppercase"
                  letterSpacing="1px"
                >
                  Full Article
                </Text>

                {isLoadingContent ? (
                  <HStack justify="center" py={4}>
                    <Spinner size="sm" color="var(--alt-primary)" />
                    <Text color="var(--alt-text-secondary)" fontSize="sm">
                      Loading article content...
                    </Text>
                  </HStack>
                ) : contentError ? (
                  <Text
                    color="var(--alt-text-secondary)"
                    fontSize="sm"
                    textAlign="center"
                  >
                    {contentError}
                  </Text>
                ) : sanitizedFullContent ? (
                  <Box
                    fontSize="sm"
                    color="var(--alt-text-primary)"
                    lineHeight="1.7"
                    css={buildContentStyles()}
                  >
                    {sanitizedFullContent}
                  </Box>
                ) : null}
              </Box>
            )}

            {isSummaryExpanded && (
              <Box
                p={4}
                bg="rgba(255, 255, 255, 0.03)"
                borderRadius="12px"
                border="1px solid var(--alt-glass-border)"
                data-testid="summary-section"
              >
                <Text
                  fontSize="xs"
                  color="var(--alt-text-secondary)"
                  fontWeight="bold"
                  mb={2}
                  textTransform="uppercase"
                  letterSpacing="1px"
                >
                  Summary
                </Text>

                {isLoadingSummary ? (
                  <HStack justify="center" py={4}>
                    <Spinner size="sm" color="var(--alt-primary)" />
                    <Text color="var(--alt-text-secondary)" fontSize="sm">
                      Loading summary...
                    </Text>
                  </HStack>
                ) : isSummarizing ? (
                  <VStack gap={3} py={4}>
                    <HStack justify="center">
                      <Spinner size="sm" color="var(--alt-primary)" />
                      <Text color="var(--alt-text-secondary)" fontSize="sm">
                        Generating summary...
                      </Text>
                    </HStack>
                    <Text
                      color="var(--alt-text-secondary)"
                      fontSize="xs"
                      textAlign="center"
                    >
                      This may take a few seconds
                    </Text>
                  </VStack>
                ) : summaryError ? (
                  <VStack gap={3} w="100%">
                    <Text
                      color="var(--alt-text-secondary)"
                      fontSize="sm"
                      textAlign="center"
                    >
                      {summaryError}
                    </Text>
                    {summaryError === "Could not fetch summary" && (
                      <VStack gap={2} w="100%">
                        <Button
                          size="sm"
                          onClick={fetchSummary}
                          w="100%"
                          borderRadius="12px"
                          bg="var(--alt-primary)"
                          color="white"
                          _hover={{
                            bg: "var(--alt-secondary)",
                            transform: "translateY(-1px)",
                          }}
                          loading={isLoadingSummary}
                          data-testid="retry-summary-button"
                        >
                          <Flex align="center" gap={2}>
                            <Text>Retry</Text>
                          </Flex>
                        </Button>
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
                          disabled={isSummarizing}
                          data-testid="summarize-now-button"
                        >
                          <Flex align="center" gap={2}>
                            {isSummarizing ? (
                              <Spinner size="xs" color="white" />
                            ) : (
                              <Sparkles size={16} />
                            )}
                            <Text>
                              {isSummarizing
                                ? "Generating..."
                                : "Summarize Now"}
                            </Text>
                          </Flex>
                        </Button>
                      </VStack>
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
                    {isSummarizing ? (
                      <Flex align="center" justify="center" gap={3} py={2}>
                        <Spinner size="sm" color="var(--alt-primary)" />
                        <Text
                          color="var(--alt-text-secondary)"
                          fontSize="sm"
                          textAlign="center"
                        >
                          Generating summary...
                        </Text>
                      </Flex>
                    ) : (
                      <>
                        <Text
                          color="var(--alt-text-secondary)"
                          fontSize="sm"
                          textAlign="center"
                        >
                          No summary available for this article
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
                          disabled={isSummarizing}
                          data-testid="summarize-now-button"
                        >
                          <Flex align="center" gap={2}>
                            <Sparkles size={16} />
                            <Text>Summarize Now</Text>
                          </Flex>
                        </Button>
                      </>
                    )}
                  </VStack>
                )}
              </Box>
            )}
          </Box>

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
                bg={
                  isContentExpanded
                    ? "var(--alt-secondary)"
                    : "var(--alt-primary)"
                }
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
                    {isLoadingContent
                      ? "Loading..."
                      : isContentExpanded
                        ? "Hide"
                        : "Article"}
                  </Text>
                </Flex>
              </Button>

              <Button
                onClick={handleToggleSummary}
                size="sm"
                flex="1"
                borderRadius="12px"
                bg={
                  isSummaryExpanded
                    ? "var(--alt-secondary)"
                    : "var(--alt-primary)"
                }
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
                disabled={isLoadingSummary || isSummarizing}
                data-testid="toggle-summary-button"
              >
                <Flex align="center" gap={2}>
                  {isSummarizing ? (
                    <Spinner size="xs" color="white" />
                  ) : (
                    <BotMessageSquare size={16} />
                  )}
                  <Text fontSize="xs">
                    {isSummarizing
                      ? "Generating..."
                      : isLoadingSummary
                        ? "Loading..."
                        : isSummaryExpanded
                          ? "Hide"
                          : "Summary"}
                  </Text>
                </Flex>
              </Button>

              <Button
                type="button"
                onClick={async () => {
                  if (!feed.link) return;
                  try {
                    setIsArchiving(true);
                    await articleApi.archiveContent(feed.link, feed.title);
                    setIsArchived(true);
                  } catch (e) {
                    console.error("Error archiving feed:", e);
                  } finally {
                    setIsArchiving(false);
                  }
                }}
                size="sm"
                flex="1"
                borderRadius="12px"
                bg="var(--alt-primary)"
                color="var(--text-primary)"
                fontWeight="bold"
                border="1px solid rgba(255, 255, 255, 0.2)"
                _hover={{
                  filter: "brightness(1.1)",
                  transform: "translateY(-1px)",
                }}
                _active={{
                  transform: "translateY(0)",
                }}
                transition="all 0.2s ease"
                disabled={isArchiving || isArchived}
                title="Archive"
                data-testid="archive-button"
              >
                <Flex align="center" gap={1}>
                  <Archive size={14} />
                  <Text fontSize="xs">
                    {isArchiving ? "..." : isArchived ? "✓" : "Archive"}
                  </Text>
                </Flex>
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
      </animated.div>
    );
  },
);

const SwipeFeedCard = memo((props: SwipeFeedCardProps) => {
  const transitions = useTransition(props.feed, {
    keys: (item: RenderFeed) => item.id,
    from: { opacity: 0, scale: 0.98, zIndex: 0 },
    enter: { opacity: 1, scale: 1, zIndex: 1 },
    leave: { opacity: 0, pointerEvents: "none", zIndex: 10 },
    config: (item, state) => {
      if (String(state) === "leave") {
        return { tension: 600, friction: 30 };
      }
      return { tension: 500, friction: 28 };
    },
  });

  return (
    <div style={{ position: "relative", width: "100%", height: "95dvh" }}>
      {transitions((style: any, feed: RenderFeed) => (
        <CardView {...props} feed={feed} style={style} />
      ))}
    </div>
  );
});

SwipeFeedCard.displayName = "SwipeFeedCard";

export default SwipeFeedCard;
