"use client";

import {
  Box,
  Flex,
  Text,
  HStack,
  VStack,
  IconButton,
  Badge,
} from "@chakra-ui/react";
import { useRef, useState, useCallback, useMemo, useEffect } from "react";
import { Heart, Bookmark, Clock, ExternalLink, Eye } from "lucide-react";
import { feedsApi } from "@/lib/api";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import { Feed } from "@/schema/feed";
import { DesktopFeed } from "@/types/desktop-feed";
import { FeedTag } from "@/types/feed-tags";
const PAGE_SIZE = 20;

// Transform Feed to DesktopFeed
const transformFeedToDesktopFeed = (feed: Feed): DesktopFeed => {
  return {
    ...feed,
    metadata: {
      source: {
        id: "rss",
        name: "RSS Feed",
        icon: "📰",
        reliability: 0.8,
        category: "general",
        unreadCount: 0,
        avgReadingTime: 5,
      },
      readingTime: Math.max(1, Math.ceil(feed.description.length / 200)),
      engagement: {
        likes: Math.floor(Math.random() * 50),
        bookmarks: Math.floor(Math.random() * 20),
      },
      tags: [],
      relatedCount: 0,
      publishedAt: feed.published,
      priority: "medium" as const,
      category: "general",
      difficulty: "intermediate" as const,
    },
    isRead: false,
    isFavorited: false,
    isBookmarked: false,
  };
};

// Format time ago
const formatTimeAgo = (dateString: string) => {
  const now = new Date();
  const date = new Date(dateString);
  const diffInSeconds = Math.floor((now.getTime() - date.getTime()) / 1000);

  if (diffInSeconds < 60) return "just now";
  if (diffInSeconds < 3600) return `${Math.floor(diffInSeconds / 60)}m ago`;
  if (diffInSeconds < 86400) return `${Math.floor(diffInSeconds / 3600)}h ago`;
  return `${Math.floor(diffInSeconds / 86400)}d ago`;
};

// Desktop-styled Feed Card
const DesktopStyledFeedCard = ({
  feed,
  isRead,
  onMarkAsRead,
}: {
  feed: DesktopFeed;
  isRead: boolean;
  onMarkAsRead: () => void;
}) => {
  const [isHovered, setIsHovered] = useState(false);
  const [isFavorited, setIsFavorited] = useState(feed.isFavorited);
  const [tags, setTags] = useState<FeedTag[]>([]);
  const [showAllTags, setShowAllTags] = useState(false);

  useEffect(() => {
    const fetchTags = async () => {
      try {
        const response = await feedsApi.fetchFeedTags(feed.link);
        setTags(response.tags);
      } catch (error) {
        console.error("Failed to fetch tags for feed:", feed.link, error);
        // Set empty tags on error to prevent UI issues
        setTags([]);
      }
    };
    fetchTags();
  }, [feed.link]);



  const handleViewArticle = useCallback(() => {
    window.open(feed.link, "_blank");
  }, [feed.link]);

  const handleToggleFavorite = useCallback(
    async (e: React.MouseEvent) => {
      e.stopPropagation();

      const res = await feedsApi.registerFavoriteFeed(feed.link);
      if (res.message !== "favorite feed registered") {
        console.error(res.message);
        return;
      }
      setIsFavorited(true);
    },
    [feed.link],
  );

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case "high":
        return "var(--accent-primary)";
      case "medium":
        return "var(--accent-secondary)";
      case "low":
        return "var(--text-muted)";
      default:
        return "var(--text-muted)";
    }
  };

  return (
    <Box
      className="glass"
      p={6}
      borderRadius="var(--radius-lg)"
      border="1px solid var(--surface-border)"
      borderLeftWidth="3px"
      borderLeftColor={getPriorityColor(feed.metadata.priority)}
      cursor="pointer"
      role="article"
      tabIndex={0}
      transition="all var(--transition-smooth) ease"
      opacity={isRead ? 0.7 : 1}
      _hover={{
        transform: "translateY(-2px)",
        boxShadow: "0 8px 25px rgba(0, 0, 0, 0.1)",
        borderColor: "var(--accent-primary)",
      }}
      data-testid={`desktop-feed-card-${feed.id}`}
    >
      {/* Card Header */}
      <Flex justify="space-between" align="center" mb={4}>
        <HStack gap={4}>
          <Text fontSize="xl">{feed.metadata.source.icon}</Text>
          <VStack gap={0} align="start">
            <Text
              fontSize="sm"
              fontWeight="semibold"
              color="var(--text-primary)"
            >
              {feed.metadata.source.name}
            </Text>
            <HStack gap={2} fontSize="xs" color="var(--text-secondary)">
              <Text>{formatTimeAgo(feed.published)}</Text>
              <Text>•</Text>
              <HStack gap={1}>
                <Clock size={12} />
                <Text>{feed.metadata.readingTime} min read</Text>
              </HStack>
            </HStack>
          </VStack>
        </HStack>

        {/* Quick Actions */}
        <HStack
          gap={2}
          opacity={isHovered ? 1 : 0.7}
          transition="opacity var(--transition-speed) ease"
        >
          <IconButton
            aria-label={
              isFavorited ? "Remove from favorites" : "Add to favorites"
            }
            size="sm"
            variant="ghost"
            color={
              isFavorited ? "var(--accent-primary)" : "var(--text-secondary)"
            }
            bg={isFavorited ? "var(--surface-bg)" : "transparent"}
            fill={
              isFavorited ? "var(--accent-primary)" : "var(--text-secondary)"
            }
            onClick={(e) => {
              e.stopPropagation();
              handleToggleFavorite(e);
            }}
          >
            <Heart size={16} />
          </IconButton>
        </HStack>
      </Flex>

      {/* Card Main Content */}
      <VStack gap={4} align="stretch">
        {/* Title */}
        <HStack
          gap={3}
          align="flex-start"
          outline={isHovered ? "2px solid var(--accent-primary)" : "none"}
          rounded={isHovered ? "var(--radius-md)" : "none"}
          p={isHovered ? "2px" : "0"}
          onMouseEnter={() => setIsHovered(true)}
          onMouseLeave={() => setIsHovered(false)}
        >
          <Text fontSize="lg" color={getPriorityColor(feed.metadata.priority)}>
            <IconButton
              aria-label="Open article"
              size="sm"
              variant="ghost"
              color="var(--text-primary)"
              onClick={handleViewArticle}
            >
              <ExternalLink size={16} />
            </IconButton>
          </Text>
          <Text
            fontSize="lg"
            fontWeight="semibold"
            color="var(--text-primary)"
            lineHeight="1.4"
            flex={1}
            onClick={handleViewArticle}
          >
            {feed.title}
          </Text>
        </HStack>

        {/* Description */}
        <Text
          fontSize="sm"
          color="var(--text-secondary)"
          lineHeight="1.6"
          css={{
            display: "-webkit-box",
            WebkitLineClamp: 3,
            WebkitBoxOrient: "vertical",
            overflow: "hidden",
          }}
        >
          {feed.description}
        </Text>

        {/* Tags and Metadata */}
        <HStack justify="space-between" align="center" wrap="wrap">
          <HStack gap={2} wrap="wrap">
            {(showAllTags ? tags : tags.slice(0, 5)).map((tag) => (
              <Badge
                key={tag.id}
                bg="var(--accent-tertiary)"
                color="white"
                fontSize="xs"
                px={2}
                py={1}
                borderRadius="md"
              >
                {tag.name}
              </Badge>
            ))}
            {tags.length > 5 && (
              <Badge
                bg="var(--surface-border)"
                color="var(--text-muted)"
                fontSize="xs"
                px={2}
                py={1}
                borderRadius="md"
                cursor="pointer"
                _hover={{
                  bg: "var(--accent-secondary)",
                  color: "white",
                }}
                onClick={(e) => {
                  e.stopPropagation();
                  setShowAllTags(!showAllTags);
                }}
              >
                {showAllTags ? "Show Less" : `+${tags.length - 5}`}
              </Badge>
            )}
          </HStack>

          <HStack gap={4} fontSize="xs" color="var(--text-muted)">
            <HStack gap={1}>
              <Heart size={12} />
              <Text>{feed.metadata.engagement.likes}</Text>
            </HStack>
            <HStack gap={1}>
              <Bookmark size={12} />
              <Text>{feed.metadata.engagement.bookmarks}</Text>
            </HStack>
            {!isRead && (
              <HStack gap={1}>
                <Eye size={12} />
                <Text>New</Text>
              </HStack>
            )}
          </HStack>
        </HStack>

        {/* Action Bar */}
        <HStack justify="space-between" align="center" pt={2}>
          <HStack gap={2}>
            <Text fontSize="xs" color="var(--text-muted)">
              {new Date(feed.published).toLocaleDateString()}
            </Text>
            <Text fontSize="xs" color="var(--text-muted)">
              •
            </Text>
            <Text fontSize="xs" color="var(--text-muted)">
              Reliability: {Math.round(feed.metadata.source.reliability * 100)}%
            </Text>
          </HStack>

          <HStack gap={2}>
            {!isRead && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onMarkAsRead();
                }}
                style={{
                  padding: "6px 14px",
                  borderRadius: "8px",
                  backgroundColor: "var(--accent-primary)",
                  color: "var(--text-primary)",
                  border: "none",
                  fontSize: "12px",
                  fontWeight: "600",
                  cursor: "pointer",
                  transition: "all 0.2s ease",
                }}
              >
                Mark as Read
              </button>
            )}
            <IconButton
              aria-label="Open article"
              size="sm"
              variant="ghost"
              color="var(--text-secondary)"
              onClick={(e) => {
                e.stopPropagation();
                handleViewArticle();
              }}
            >
              <ExternalLink size={16} />
            </IconButton>
          </HStack>
        </HStack>
      </VStack>
    </Box>
  );
};

// Desktop-styled Skeleton Card
const DesktopSkeletonCard = () => (
  <Box
    className="glass"
    p={6}
    borderRadius="var(--radius-lg)"
    border="1px solid var(--surface-border)"
    borderLeftWidth="3px"
    borderLeftColor="var(--surface-border)"
    opacity={0.6}
  >
    <Flex justify="space-between" align="center" mb={4}>
      <HStack gap={4}>
        <Box
          width="32px"
          height="32px"
          backgroundColor="var(--surface-bg)"
          borderRadius="50%"
        />
        <VStack gap={1} align="start">
          <Box
            height="16px"
            backgroundColor="var(--surface-bg)"
            borderRadius="4px"
            width="100px"
          />
          <Box
            height="12px"
            backgroundColor="var(--surface-bg)"
            borderRadius="4px"
            width="120px"
          />
        </VStack>
      </HStack>
      <HStack gap={2}>
        <Box
          width="24px"
          height="24px"
          backgroundColor="var(--surface-bg)"
          borderRadius="50%"
        />
        <Box
          width="24px"
          height="24px"
          backgroundColor="var(--surface-bg)"
          borderRadius="50%"
        />
      </HStack>
    </Flex>

    <VStack gap={4} align="stretch">
      <HStack gap={3} align="flex-start">
        <Box
          width="24px"
          height="24px"
          backgroundColor="var(--surface-bg)"
          borderRadius="4px"
        />
        <Box
          height="24px"
          backgroundColor="var(--surface-bg)"
          borderRadius="4px"
          flex={1}
        />
      </HStack>

      <VStack gap={2} align="stretch">
        <Box
          height="16px"
          backgroundColor="var(--surface-bg)"
          borderRadius="4px"
          width="100%"
        />
        <Box
          height="16px"
          backgroundColor="var(--surface-bg)"
          borderRadius="4px"
          width="90%"
        />
        <Box
          height="16px"
          backgroundColor="var(--surface-bg)"
          borderRadius="4px"
          width="75%"
        />
      </VStack>

      <HStack justify="space-between" align="center">
        <HStack gap={3}>
          <Box
            height="20px"
            backgroundColor="var(--surface-bg)"
            borderRadius="4px"
            width="60px"
          />
          <Box
            height="20px"
            backgroundColor="var(--surface-bg)"
            borderRadius="4px"
            width="80px"
          />
        </HStack>
        <HStack gap={2}>
          <Box
            height="18px"
            backgroundColor="var(--surface-bg)"
            borderRadius="4px"
            width="40px"
          />
          <Box
            height="18px"
            backgroundColor="var(--surface-bg)"
            borderRadius="4px"
            width="40px"
          />
        </HStack>
      </HStack>

      <HStack justify="space-between" align="center">
        <Box
          height="12px"
          backgroundColor="var(--surface-bg)"
          borderRadius="4px"
          width="150px"
        />
        <HStack gap={2}>
          <Box
            height="24px"
            backgroundColor="var(--surface-bg)"
            borderRadius="4px"
            width="100px"
          />
          <Box
            width="24px"
            height="24px"
            backgroundColor="var(--surface-bg)"
            borderRadius="50%"
          />
        </HStack>
      </HStack>
    </VStack>
  </Box>
);

// Enhanced Infinite Scroll Hook with Root Option
const useEnhancedInfiniteScroll = (
  callback: () => void,
  sentinelRef: React.RefObject<HTMLDivElement | null>,
  scrollContainerRef: React.RefObject<HTMLDivElement | null>,
  resetKey?: number | string,
) => {
  useEffect(() => {
    let observer: IntersectionObserver | null = null;

    const setupObserver = () => {
      const sentinelElement = sentinelRef.current;
      const scrollContainer = scrollContainerRef.current;

      if (!sentinelElement) {
        console.log("🔍 Sentinel element not found, retrying...");
        setTimeout(setupObserver, 100);
        return;
      }

      console.log("🚀 Setting up IntersectionObserver for infinite scroll");

      observer = new IntersectionObserver(
        (entries) => {
          entries.forEach((entry) => {
            console.log("👀 Intersection observed:", {
              isIntersecting: entry.isIntersecting,
              intersectionRatio: entry.intersectionRatio,
              boundingClientRect: entry.boundingClientRect,
            });

            if (entry.isIntersecting) {
              console.log("✅ Sentinel is visible, triggering callback");
              callback();
            }
          });
        },
        {
          root: scrollContainer, // 明示的にスクロールコンテナを指定
          rootMargin: "200px 0px", // より早めにトリガー
          threshold: 0.1,
        },
      );

      observer.observe(sentinelElement);
      console.log("👁️ Observer started watching sentinel element");
    };

    setupObserver();

    return () => {
      if (observer) {
        observer.disconnect();
        console.log("🛑 IntersectionObserver disconnected");
      }
    };
  }, [callback, sentinelRef, scrollContainerRef, resetKey]);
};

export default function DesktopTimeline() {
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
  const [isRetrying, setIsRetrying] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Use cursor-based pagination hook
  const {
    data: feeds,
    hasMore,
    isLoading,
    error,
    isInitialLoading,
    loadMore,
    refresh,
  } = useCursorPagination(feedsApi.getFeedsWithCursor, {
    limit: PAGE_SIZE,
    autoLoad: true,
  });

  // Transform feeds to desktop format
  const desktopFeeds = useMemo(() => {
    return feeds?.map(transformFeedToDesktopFeed) || [];
  }, [feeds]);

  // Memoize visible feeds to prevent unnecessary recalculations
  const visibleFeeds = useMemo(
    () => desktopFeeds.filter((feed) => !readFeeds.has(feed.id)),
    [desktopFeeds, readFeeds],
  );

  // Handle marking feed as read
  const handleMarkAsRead = useCallback(async (feedId: string) => {
    setReadFeeds((prev) => {
      const newSet = new Set(prev);
      newSet.add(feedId);
      return newSet;
    });
    try {
      await feedsApi.updateFeedReadStatus(feedId);
    } catch (err) {
      console.error("Failed to update feed read status:", err);
    }
  }, []);

  // Retry functionality
  const retryFetch = useCallback(async () => {
    setIsRetrying(true);
    try {
      await refresh();
    } catch (err) {
      console.error("Retry failed:", err);
    } finally {
      setIsRetrying(false);
    }
  }, [refresh]);

  // Handle infinite scroll
  const handleLoadMore = useCallback(() => {
    console.log("🔄 Load more triggered:", {
      hasMore,
      isLoading,
      feedCount: feeds?.length,
    });
    if (hasMore && !isLoading) {
      console.log("✅ Conditions met, calling loadMore");
      loadMore();
    } else {
      console.log("❌ Conditions not met:", { hasMore, isLoading });
    }
  }, [hasMore, isLoading, loadMore, feeds?.length]);

  // Use enhanced infinite scroll
  useEnhancedInfiniteScroll(
    handleLoadMore,
    sentinelRef,
    scrollContainerRef,
    feeds?.length || 0,
  );

  // Debug effect
  useEffect(() => {
    console.log("📊 Timeline state:", {
      feedsCount: feeds?.length || 0,
      visibleFeedsCount: visibleFeeds.length,
      hasMore,
      isLoading,
      isInitialLoading,
    });
  }, [
    feeds?.length,
    visibleFeeds.length,
    hasMore,
    isLoading,
    isInitialLoading,
  ]);

  // Show skeleton loading state
  if (isInitialLoading) {
    return (
      <Box w="100%" minH="0" flex={1} bg="var(--app-bg)">
        <Box
          overflowY="auto"
          overflowX="hidden"
          h="100vh"
          p={6}
          data-testid="desktop-timeline-skeleton"
        >
          <Flex direction="column" gap={6} maxW="1000px" mx="auto">
            {Array.from({ length: 3 }).map((_, index) => (
              <DesktopSkeletonCard key={`skeleton-${index}`} />
            ))}
          </Flex>
        </Box>
      </Box>
    );
  }

  // Show error state
  if (error) {
    return (
      <Box w="100%" minH="0" flex={1} bg="var(--app-bg)">
        <Box
          display="flex"
          alignItems="center"
          justifyContent="center"
          h="100%"
          p={4}
        >
          <Box
            className="glass"
            p={6}
            borderRadius="var(--radius-lg)"
            textAlign="center"
            maxW="400px"
          >
            <Text fontSize="2xl" mb={3}>
              ⚠️
            </Text>
            <Text color="var(--text-primary)" fontSize="lg" mb={3}>
              Failed to load feeds
            </Text>
            <Text color="var(--text-secondary)" fontSize="sm" mb={4}>
              {error.message}
            </Text>
            <button
              onClick={retryFetch}
              disabled={isRetrying}
              style={{
                padding: "10px 20px",
                borderRadius: "var(--radius-md)",
                backgroundColor: "var(--accent-primary)",
                color: "var(--text-primary)",
                border: "none",
                fontSize: "14px",
                fontWeight: "600",
                cursor: isRetrying ? "not-allowed" : "pointer",
                opacity: isRetrying ? 0.6 : 1,
                transition: "all 0.2s ease",
              }}
            >
              {isRetrying ? "Retrying..." : "Retry"}
            </button>
          </Box>
        </Box>
      </Box>
    );
  }

  return (
    <Box w="100%" minH="0" flex={1} bg="var(--app-bg)">
      <Box
        ref={scrollContainerRef}
        overflowY="auto"
        overflowX="hidden"
        h="100vh"
        p={6}
        data-testid="desktop-timeline-container"
        css={{
          scrollBehavior: "smooth",
          "&::-webkit-scrollbar": {
            width: "6px",
          },
          "&::-webkit-scrollbar-track": {
            background: "var(--surface-secondary)",
            borderRadius: "3px",
          },
          "&::-webkit-scrollbar-thumb": {
            background: "var(--accent-primary)",
            borderRadius: "3px",
            opacity: 0.7,
          },
          "&::-webkit-scrollbar-thumb:hover": {
            opacity: 1,
          },
        }}
      >
        {visibleFeeds.length > 0 ? (
          <>
            {/* Feed Cards - Wide desktop styling */}
            <Flex direction="column" gap={6} maxW="1000px" mx="auto">
              {visibleFeeds.map((feed) => (
                <DesktopStyledFeedCard
                  key={feed.id}
                  feed={feed}
                  isRead={readFeeds.has(feed.id)}
                  onMarkAsRead={() => handleMarkAsRead(feed.id)}
                />
              ))}
            </Flex>

            {/* Infinite scroll sentinel */}
            {hasMore && (
              <Box
                ref={sentinelRef}
                h="60px"
                w="100%"
                mt={6}
                display="flex"
                alignItems="center"
                justifyContent="center"
                data-testid="infinite-scroll-sentinel"
                bg="transparent"
                position="relative"
              >
                {isLoading && (
                  <Box
                    className="glass"
                    p={3}
                    borderRadius="var(--radius-md)"
                    display="flex"
                    alignItems="center"
                    gap={2}
                  >
                    <div
                      style={{
                        width: "14px",
                        height: "14px",
                        border: "2px solid var(--surface-border)",
                        borderTop: "2px solid var(--accent-primary)",
                        borderRadius: "50%",
                        animation: "spin 1s linear infinite",
                      }}
                    />
                    <Text color="var(--text-secondary)" fontSize="sm">
                      Loading more...
                    </Text>
                  </Box>
                )}
                {!isLoading && (
                  <Text color="var(--text-muted)" fontSize="xs">
                    Scroll for more feeds...
                  </Text>
                )}
              </Box>
            )}

            {/* No more feeds indicator */}
            {!hasMore && visibleFeeds.length > 0 && (
              <Box
                className="glass"
                p={4}
                borderRadius="var(--radius-lg)"
                maxW="1000px"
                mx="auto"
                mt={4}
                textAlign="center"
              >
                <Text fontSize="lg" mb={1}>
                  📭
                </Text>
                <Text
                  color="var(--text-secondary)"
                  fontSize="sm"
                  fontWeight="medium"
                >
                  You have reached the end of your feed
                </Text>
              </Box>
            )}
          </>
        ) : (
          /* Enhanced empty state */
          <Flex justify="center" align="center" py={16} maxW="1000px" mx="auto">
            <Box
              className="glass"
              p={6}
              borderRadius="var(--radius-lg)"
              textAlign="center"
            >
              <Text fontSize="2xl" mb={3}>
                📰
              </Text>
              <Text color="var(--text-primary)" fontSize="md" mb={2}>
                No feeds available
              </Text>
              <Text color="var(--text-secondary)" fontSize="sm">
                Your feed will appear here once you subscribe to sources
              </Text>
            </Box>
          </Flex>
        )}
      </Box>
    </Box>
  );
}
