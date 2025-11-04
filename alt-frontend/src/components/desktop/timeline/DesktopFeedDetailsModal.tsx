"use client";

import { Box, Dialog, Flex, HStack, IconButton, Portal, Spinner, Text } from "@chakra-ui/react";
import type { CSSObject } from "@emotion/react";
import { Archive, ExternalLink, Sparkles, X } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { feedsApi } from "@/lib/api";
import type { FeedContentOnTheFlyResponse, FetchArticleSummaryResponse } from "@/schema/feed";
import { DesktopRenderFeedDetails } from "./DesktopRenderFeedDetails";

interface DesktopFeedDetailsModalProps {
  isOpen: boolean;
  onClose: () => void;
  feedLink: string;
  feedTitle: string;
  feedId: string;
}

const scrollAreaStyles: CSSObject = {
  "&::-webkit-scrollbar": {
    width: "6px",
  },
  "&::-webkit-scrollbar-track": {
    background: "rgba(255, 255, 255, 0.04)",
    borderRadius: "8px",
  },
  "&::-webkit-scrollbar-thumb": {
    background: "rgba(255, 255, 255, 0.25)",
    borderRadius: "8px",
  },
  "&::-webkit-scrollbar-thumb:hover": {
    background: "rgba(255, 255, 255, 0.35)",
  },
};

const summaryContainerStyles: CSSObject = {
  background: "rgba(255, 255, 255, 0.05)",
  border: "1px solid rgba(255, 255, 255, 0.12)",
  borderRadius: "16px",
};

export const DesktopFeedDetailsModal = ({
  isOpen,
  onClose,
  feedLink,
  feedTitle,
  feedId,
}: DesktopFeedDetailsModalProps) => {
  const [content, setContent] = useState<FeedContentOnTheFlyResponse | null>(null);
  const [articleSummary, setArticleSummary] = useState<FetchArticleSummaryResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasFetched, setHasFetched] = useState(false);

  const [isArchiving, setIsArchiving] = useState(false);
  const [isArchived, setIsArchived] = useState(false);
  const [isSummarizing, setIsSummarizing] = useState(false);
  const [summary, setSummary] = useState<string | null>(null);
  const [summaryError, setSummaryError] = useState<string | null>(null);

  useEffect(() => {
    setContent(null);
    setArticleSummary(null);
    setError(null);
    setSummary(null);
    setSummaryError(null);
    setIsArchived(false);
    setHasFetched(false);
  }, [feedLink, feedTitle]);

  useEffect(() => {
    if (!isOpen || hasFetched) {
      return;
    }

    let isCancelled = false;

    const fetchDetails = async () => {
      setIsLoading(true);
      setError(null);

      const summaryPromise = feedsApi.getArticleSummary(feedLink).catch((err) => {
        console.error("Failed to fetch article summary:", err);
        return null;
      });

      const contentPromise = feedsApi
        .getFeedContentOnTheFly({ feed_url: feedLink })
        .catch((err) => {
          console.error("Failed to fetch article content:", err);
          return null;
        });

      try {
        const [summaryResponse, contentResponse] = await Promise.all([
          summaryPromise,
          contentPromise,
        ]);

        if (isCancelled) {
          return;
        }

        const hasSummary =
          !!summaryResponse?.matched_articles && summaryResponse.matched_articles.length > 0;
        const hasContent = !!contentResponse?.content && contentResponse.content.trim().length > 0;

        if (hasSummary) {
          setArticleSummary(summaryResponse);
        }

        if (hasContent) {
          setContent(contentResponse);
          try {
            await feedsApi.archiveContent(feedLink, feedTitle);
            if (!isCancelled) {
              setIsArchived(true);
            }
          } catch (archiveErr) {
            console.warn("Auto-archive failed:", archiveErr);
          }
        }

        if (!hasSummary && !hasContent) {
          setError("Unable to load article details");
        }
      } finally {
        if (!isCancelled) {
          setHasFetched(true);
          setIsLoading(false);
        }
      }
    };

    void fetchDetails();

    return () => {
      isCancelled = true;
    };
  }, [feedLink, feedTitle, hasFetched, isOpen]);

  const handleArchive = useCallback(async () => {
    if (isArchiving) {
      return;
    }
    setIsArchiving(true);

    try {
      await feedsApi.archiveContent(feedLink, feedTitle);
      setIsArchived(true);
    } catch (err) {
      console.error("Failed to archive feed:", err);
    } finally {
      setIsArchiving(false);
    }
  }, [feedLink, feedTitle, isArchiving]);

  const handleSummarize = useCallback(async () => {
    if (isSummarizing) {
      return;
    }
    setIsSummarizing(true);
    setSummaryError(null);

    try {
      const result = await feedsApi.summarizeArticle(feedLink);
      const trimmed = result.summary?.trim();

      if (trimmed) {
        setSummary(trimmed);
      } else {
        setSummaryError("AI summary is unavailable for this article.");
      }
    } catch (err) {
      console.error("Failed to generate AI summary:", err);
      setSummaryError("Failed to generate AI summary. Please try again.");
    } finally {
      setIsSummarizing(false);
    }
  }, [feedLink, isSummarizing]);

  const activeFeedDetails = useMemo(() => {
    if (content) {
      return content;
    }
    if (articleSummary) {
      return articleSummary;
    }
    return null;
  }, [articleSummary, content]);

  return (
    <Dialog.Root
      open={isOpen}
      onOpenChange={(details) => {
        if (!details.open) {
          onClose();
        }
      }}
    >
      <Portal>
        <Dialog.Backdrop bg="rgba(5, 10, 25, 0.65)" backdropFilter="blur(14px)" />
        <Dialog.Positioner display="flex" alignItems="center" justifyContent="center">
          <Dialog.Content
            maxW="900px"
            w="94vw"
            bg="var(--app-bg)"
            borderRadius="var(--radius-xl)"
            border="1px solid var(--surface-border)"
            boxShadow="0 30px 70px rgba(0, 0, 0, 0.35)"
            overflow="hidden"
            data-testid={`desktop-feed-details-modal-${feedId}`}
          >
            <Dialog.Header px={6} py={5} borderBottom="1px solid rgba(255, 255, 255, 0.06)">
              <Flex align="center" justify="space-between" gap={4}>
                <Link
                  href={feedLink}
                  target="_blank"
                  rel="noopener noreferrer"
                  color="var(--text-primary)"
                  data-testid={`desktop-feed-details-link-${feedId}`}
                  style={{
                    textDecoration: "none",
                    transition: "color 0.2s ease",
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.color = "var(--accent-primary)";
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.color = "var(--text-primary)";
                  }}
                >
                  <HStack fontWeight="semibold" fontSize="xl" gap={2}>
                    <ExternalLink size={18} />
                    <Text
                      as="span"
                      style={{
                        display: "-webkit-box",
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: "vertical",
                        overflow: "hidden",
                      }}
                    >
                      {feedTitle}
                    </Text>
                  </HStack>
                </Link>
                <Dialog.CloseTrigger asChild>
                  <IconButton
                    aria-label="Close feed details"
                    size="sm"
                    variant="ghost"
                    color="var(--text-secondary)"
                    borderRadius="full"
                  >
                    <X size={16} />
                  </IconButton>
                </Dialog.CloseTrigger>
              </Flex>
            </Dialog.Header>

            <Dialog.Body px={0} py={0}>
              <Box
                px={6}
                py={7}
                maxH="75vh"
                overflowY="auto"
                css={scrollAreaStyles}
                data-testid={`desktop-feed-details-scroll-${feedId}`}
              >
                {isLoading && !activeFeedDetails ? (
                  <Flex align="center" justify="center" minH="180px" direction="column" gap={3}>
                    <Spinner color="var(--accent-primary)" size="lg" />
                    <Text color="var(--text-secondary)" fontSize="sm">
                      Fetching article...
                    </Text>
                  </Flex>
                ) : (
                  <DesktopRenderFeedDetails
                    feedDetails={activeFeedDetails}
                    isLoading={isLoading}
                    error={error}
                    articleUrl={feedLink}
                  />
                )}

                {summary && (
                  <Box mt={5} px={5} py={4} css={summaryContainerStyles}>
                    <Text
                      fontSize="sm"
                      color="var(--text-secondary)"
                      fontWeight="bold"
                      textTransform="uppercase"
                      letterSpacing="0.08em"
                      mb={2}
                    >
                      AI Summary
                    </Text>
                    <Text color="var(--text-primary)" fontSize="md" lineHeight="1.8">
                      {summary}
                    </Text>
                  </Box>
                )}

                {summaryError && (
                  <Box
                    mt={5}
                    px={5}
                    py={4}
                    borderRadius="16px"
                    border="1px solid rgba(255, 99, 71, 0.4)"
                    bg="rgba(255, 99, 71, 0.08)"
                  >
                    <Text color="var(--text-primary)" fontSize="sm">
                      {summaryError}
                    </Text>
                  </Box>
                )}
              </Box>
            </Dialog.Body>

            <Dialog.Footer px={6} py={5} borderTop="1px solid rgba(255, 255, 255, 0.06)">
              <HStack gap={3} w="100%">
                <Box
                  as="button"
                  flex="1"
                  onClick={isArchiving ? undefined : handleArchive}
                  px={4}
                  py={3}
                  borderRadius="md"
                  border="1px solid rgba(255, 255, 255, 0.35)"
                  bg={isArchived ? "rgba(255, 255, 255, 0.18)" : "rgba(255, 255, 255, 0.08)"}
                  color={isArchived ? "var(--accent-secondary)" : "rgba(255, 255, 255, 0.9)"}
                  cursor={isArchiving ? "not-allowed" : "pointer"}
                  opacity={isArchiving ? 0.6 : 1}
                  transition="all 0.2s ease"
                  display="flex"
                  alignItems="center"
                  justifyContent="center"
                  gap={2}
                  data-testid={`desktop-feed-details-archive-${feedId}`}
                  _hover={{
                    bg: isArchiving ? undefined : "rgba(255, 255, 255, 0.22)",
                    transform: isArchiving ? undefined : "translateY(-1px)",
                  }}
                  _active={{
                    transform: isArchiving ? undefined : "translateY(0)",
                  }}
                >
                  <Archive size={18} />
                  <Text fontSize="sm" fontWeight="medium">
                    {isArchiving ? "Archiving..." : isArchived ? "Archived" : "Archive"}
                  </Text>
                </Box>

                <Box
                  as="button"
                  flex="1"
                  onClick={isSummarizing || !!summary ? undefined : handleSummarize}
                  px={4}
                  py={3}
                  borderRadius="md"
                  border="1px solid rgba(255, 255, 255, 0.35)"
                  bg={summary ? "rgba(255, 255, 255, 0.18)" : "rgba(255, 255, 255, 0.08)"}
                  color={summary ? "var(--accent-primary)" : "rgba(255, 255, 255, 0.9)"}
                  cursor={isSummarizing || summary ? "not-allowed" : "pointer"}
                  opacity={isSummarizing ? 0.6 : 1}
                  transition="all 0.2s ease"
                  display="flex"
                  alignItems="center"
                  justifyContent="center"
                  gap={2}
                  data-testid={`desktop-feed-details-ai-${feedId}`}
                  _hover={{
                    bg: isSummarizing || summary ? undefined : "rgba(255, 255, 255, 0.22)",
                    transform: isSummarizing || summary ? undefined : "translateY(-1px)",
                  }}
                  _active={{
                    transform: isSummarizing || summary ? undefined : "translateY(0)",
                  }}
                >
                  {isSummarizing ? (
                    <>
                      <Spinner size="sm" color="var(--accent-primary)" />
                      <Text fontSize="sm" fontWeight="medium">
                        Generating...
                      </Text>
                    </>
                  ) : summary ? (
                    <>
                      <Sparkles size={18} />
                      <Text fontSize="sm" fontWeight="medium">
                        Summary Ready
                      </Text>
                    </>
                  ) : (
                    <>
                      <Sparkles size={18} />
                      <Text fontSize="sm" fontWeight="medium">
                        Summarize
                      </Text>
                    </>
                  )}
                </Box>
              </HStack>
            </Dialog.Footer>
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  );
};

export default DesktopFeedDetailsModal;
