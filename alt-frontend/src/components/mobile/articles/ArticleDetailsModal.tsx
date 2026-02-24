"use client";

import {
  Box,
  Button,
  Dialog,
  Flex,
  HStack,
  Portal,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import type { CSSObject } from "@emotion/react";
import { Sparkles, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { articleApi } from "@/lib/api";
import type { Article } from "@/schema/article";
import type { FeedContentOnTheFlyResponse } from "@/schema/feed";
import { renderingRegistry } from "@/utils/renderingStrategies";

interface ArticleDetailsModalProps {
  article: Article;
  isOpen: boolean;
  onClose: () => void;
}

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

const buildContentStyles = (): CSSObject => ({
  "& img": {
    maxWidth: "100%",
    height: "auto",
    borderRadius: "8px",
    marginTop: "1rem",
    marginBottom: "1rem",
  },
  "& h1, & h2, & h3, & h4, & h5, & h6": {
    fontWeight: "bold",
    marginTop: "1.5rem",
    marginBottom: "0.75rem",
    color: "var(--alt-text-primary)",
  },
  "& p": {
    marginBottom: "1rem",
    lineHeight: 1.8,
    color: "var(--alt-text-primary)",
  },
  "& ul, & ol": {
    marginLeft: "1.5rem",
    marginBottom: "1rem",
    lineHeight: 1.8,
  },
  "& li": {
    marginBottom: "0.5rem",
    color: "var(--alt-text-primary)",
  },
  "& a": {
    color: "var(--alt-primary)",
    textDecoration: "underline",
  },
});

export const ArticleDetailsModal = ({
  article,
  isOpen,
  onClose,
}: ArticleDetailsModalProps) => {
  const [fullContent, setFullContent] =
    useState<FeedContentOnTheFlyResponse | null>(null);
  const [isLoadingContent, setIsLoadingContent] = useState(false);
  const [contentError, setContentError] = useState<string | null>(null);

  const [summary, setSummary] = useState<string | null>(null);
  const [isLoadingSummary, setIsLoadingSummary] = useState(false);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const [isSummarizing, setIsSummarizing] = useState(false);
  const [showSummary, setShowSummary] = useState(false);

  const sanitizedFullContent = useMemo(() => {
    if (!fullContent?.content) {
      return null;
    }

    return renderingRegistry.render(
      fullContent.content,
      undefined,
      article.url,
    );
  }, [article.url, fullContent?.content]);

  const publishedLabel = article.published_at
    ? new Date(article.published_at).toLocaleString()
    : null;

  // Reset state when modal closes
  useEffect(() => {
    if (!isOpen) {
      setShowSummary(false);
    }
  }, [isOpen]);

  // Load content when modal opens
  useEffect(() => {
    if (!isOpen || fullContent) {
      return;
    }

    const loadContent = async () => {
      setIsLoadingContent(true);
      setContentError(null);

      try {
        // Backend already handles: DB check → fetch if not found → save to DB → return content
        // No need to call archiveContent separately
        const contentResponse = await articleApi.getFeedContentOnTheFly({
          feed_url: article.url,
        });

        if (contentResponse.content) {
          setFullContent(contentResponse);
        } else {
          setContentError("記事全文を取得できませんでした");
        }
      } catch (error) {
        console.error("Error fetching content:", error);
        setContentError("記事全文を取得できませんでした");
      } finally {
        setIsLoadingContent(false);
      }
    };

    loadContent();
  }, [isOpen, article.url, article.title, fullContent]);

  const handleToggleSummary = useCallback(async () => {
    if (!showSummary && !summary) {
      setIsLoadingSummary(true);
      setSummaryError(null);

      try {
        const summaryResponse = await articleApi.getArticleSummary(article.url);
        if (
          summaryResponse.matched_articles &&
          summaryResponse.matched_articles.length > 0
        ) {
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

    setShowSummary((prev) => !prev);
  }, [article.url, showSummary, summary]);

  const handleSummarizeNow = useCallback(async () => {
    setIsSummarizing(true);
    setSummaryError(null);

    try {
      const summarizeResponse = await articleApi.summarizeArticle(article.url);

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
  }, [article.url]);

  // Handle escape key
  useEffect(() => {
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && isOpen) {
        onClose();
      }
    };

    if (isOpen) {
      document.addEventListener("keydown", handleEscape);
    }

    return () => {
      document.removeEventListener("keydown", handleEscape);
    };
  }, [isOpen, onClose]);

  return (
    <Dialog.Root open={isOpen} onOpenChange={(e) => !e.open && onClose()}>
      <Portal>
        <Dialog.Backdrop
          bg="rgba(0, 0, 0, 0.6)"
          backdropFilter="blur(8px)"
          data-testid="article-modal-backdrop"
        />
        <Dialog.Positioner>
          <Dialog.Content
            bg="var(--alt-glass)"
            backdropFilter="blur(20px)"
            border="2px solid var(--alt-glass-border)"
            borderRadius="1rem"
            boxShadow="0 12px 40px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.1)"
            mx={4}
            my={8}
            maxW="600px"
            maxH="85vh"
            data-testid="article-modal-content"
          >
            <Dialog.Header
              position="relative"
              zIndex="2"
              borderBottom="1px solid var(--alt-glass-border)"
              px={6}
              py={4}
              bg="rgba(255, 255, 255, 0.03)"
              backdropFilter="blur(20px)"
            >
              <Flex justify="space-between" align="flex-start" gap={4}>
                <Box flex="1">
                  <Dialog.Title
                    fontSize="xl"
                    fontWeight="bold"
                    color="var(--alt-text-primary)"
                    lineHeight="1.4"
                    mb={2}
                  >
                    {article.title}
                  </Dialog.Title>
                  {publishedLabel && (
                    <Text
                      fontSize="sm"
                      color="var(--alt-text-secondary)"
                      mt={2}
                    >
                      {publishedLabel}
                    </Text>
                  )}
                </Box>
                <Dialog.CloseTrigger asChild>
                  <Button
                    size="sm"
                    variant="ghost"
                    color="var(--alt-text-secondary)"
                    _hover={{
                      bg: "rgba(255, 255, 255, 0.1)",
                      color: "var(--alt-text-primary)",
                    }}
                    data-testid="close-modal-button"
                  >
                    <X size={20} />
                  </Button>
                </Dialog.CloseTrigger>
              </Flex>
            </Dialog.Header>

            <Dialog.Body
              px={6}
              py={6}
              maxH="60vh"
              overflowY="auto"
              css={scrollAreaStyles}
              bg="transparent"
            >
              {isLoadingContent ? (
                <HStack justify="center" py={8}>
                  <Spinner size="md" color="var(--alt-primary)" />
                  <Text color="var(--alt-text-secondary)" fontSize="sm">
                    記事を読み込み中...
                  </Text>
                </HStack>
              ) : contentError ? (
                <Text
                  color="var(--alt-text-secondary)"
                  fontSize="sm"
                  textAlign="center"
                  py={8}
                >
                  {contentError}
                </Text>
              ) : sanitizedFullContent ? (
                <Box
                  fontSize="sm"
                  color="var(--alt-text-primary)"
                  lineHeight="1.7"
                  css={buildContentStyles()}
                  data-testid="article-full-content"
                >
                  {sanitizedFullContent}
                </Box>
              ) : (
                <Text
                  fontSize="sm"
                  color="var(--alt-text-primary)"
                  lineHeight="1.7"
                >
                  {article.content}
                </Text>
              )}

              {/* Summary Section */}
              {showSummary && (
                <Box
                  mt={6}
                  p={5}
                  bg="rgba(255, 255, 255, 0.03)"
                  borderRadius="12px"
                  border="1px solid var(--alt-glass-border)"
                  backdropFilter="blur(20px)"
                  data-testid="summary-section"
                >
                  <Text
                    fontSize="xs"
                    color="var(--alt-text-secondary)"
                    fontWeight="bold"
                    mb={3}
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
                      <Text
                        color="var(--alt-text-secondary)"
                        fontSize="xs"
                        textAlign="center"
                      >
                        これには数秒かかる場合があります
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
                      lineHeight="1.8"
                      whiteSpace="pre-wrap"
                    >
                      {summary}
                    </Text>
                  ) : null}
                </Box>
              )}
            </Dialog.Body>

            <Dialog.Footer
              borderTop="1px solid var(--alt-glass-border)"
              px={6}
              py={4}
              display="flex"
              justifyContent="center"
              bg="rgba(255, 255, 255, 0.03)"
              backdropFilter="blur(20px)"
            >
              <Button
                onClick={handleToggleSummary}
                size="md"
                borderRadius="12px"
                bg={showSummary ? "var(--alt-secondary)" : "var(--alt-primary)"}
                color="white"
                fontWeight="bold"
                px={8}
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
                  <Sparkles size={16} />
                  <Text>
                    {isLoadingSummary
                      ? "読込中..."
                      : showSummary
                        ? "要約非表示"
                        : "View AI Summary"}
                  </Text>
                </Flex>
              </Button>
            </Dialog.Footer>
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  );
};
