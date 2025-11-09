"use client";

import { Box, Button, Heading, HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import Link from "next/link";
import { useState } from "react";
import { articleApi } from "@/lib/api";
import type { BackendFeedItem, FetchArticleSummaryResponse } from "@/schema/feed";

interface SearchResultsProps {
  results: BackendFeedItem[];
  isLoading: boolean;
  searchQuery: string;
  searchTime?: number;
}

interface SearchResultItemProps {
  result: BackendFeedItem;
}

const SearchResultItem = ({ result }: SearchResultItemProps) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const [summary, setSummary] = useState<FetchArticleSummaryResponse | null>(null);
  const [isLoadingSummary, setIsLoadingSummary] = useState(false);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const [isSummarizing, setIsSummarizing] = useState(false);
  const [isDescriptionExpanded, setIsDescriptionExpanded] = useState(false);

  // Check if description is long enough to need truncation
  const descriptionText = result.description || "";
  const shouldTruncateDescription = descriptionText.length > 200;
  const displayDescription = isDescriptionExpanded
    ? descriptionText
    : shouldTruncateDescription
      ? descriptionText.slice(0, 200) + "..."
      : descriptionText;

  const handleToggleSummary = async () => {
    if (!isExpanded && !summary && result.link) {
      setIsLoadingSummary(true);
      setSummaryError(null);

      try {
        const summaryResponse = await articleApi.getArticleSummary(result.link);
        setSummary(summaryResponse);
      } catch (error) {
        console.error("Error fetching summary:", error);
        setSummaryError("要約を取得できませんでした");
      } finally {
        setIsLoadingSummary(false);
      }
    }
    setIsExpanded(!isExpanded);
  };

  const handleSummarizeNow = async () => {
    if (!result.link) return;

    setIsSummarizing(true);
    setSummaryError(null);

    try {
      // Call the summarize API - it returns the summary directly
      const summarizeResponse = await articleApi.summarizeArticle(result.link);

      if (summarizeResponse.success && summarizeResponse.summary) {
        // Create a FetchArticleSummaryResponse from the summarize response
        const summaryData: FetchArticleSummaryResponse = {
          matched_articles: [
            {
              article_url: result.link || "",
              title: result.title,
              author: result.author?.name || result.authors?.[0]?.name,
              content: summarizeResponse.summary,
              content_type: "summary",
              published_at: result.published || new Date().toISOString(),
              fetched_at: new Date().toISOString(),
              source_id: summarizeResponse.article_id,
            },
          ],
          total_matched: 1,
          requested_count: 1,
        };
        setSummary(summaryData);
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
  };

  return (
    <Box
      bg="var(--surface-bg)"
      p={4}
      borderRadius="0"
      border="2px solid var(--surface-border)"
      _hover={{
        bg: "var(--surface-hover)",
        borderColor: "var(--alt-primary)",
        boxShadow: "var(--shadow-md)",
      }}
      transition="all 0.2s ease"
      role="article"
      aria-label={`Search result: ${result.title}`}
    >
      <VStack align="start" gap={3} width="100%">
        <Link
          href={result.link || "#"}
          target="_blank"
          rel="noopener noreferrer"
          style={{ width: "100%" }}
        >
          <Heading
            as="h3"
            size="md"
            color="var(--alt-primary)"
            fontWeight="700"
            _hover={{
              textDecoration: "underline",
              color: "var(--alt-secondary)",
              textDecorationThickness: "2px",
            }}
            lineHeight="1.3"
            letterSpacing="-0.025em"
            wordBreak="break-word"
            overflowWrap="anywhere"
          >
            {result.title}
          </Heading>
        </Link>

        {descriptionText && (
          <VStack align="start" gap={2} width="100%">
            <Text
              color="var(--text-secondary)"
              fontSize="sm"
              lineHeight="1.7"
              wordBreak="break-word"
              overflowWrap="anywhere"
            >
              {displayDescription}
            </Text>
            {shouldTruncateDescription && (
              <Button
                size="xs"
                variant="ghost"
                onClick={(e) => {
                  e.stopPropagation();
                  setIsDescriptionExpanded(!isDescriptionExpanded);
                }}
                color="var(--alt-primary)"
                _hover={{
                  bg: "rgba(255, 255, 255, 0.05)",
                }}
                alignSelf="flex-start"
              >
                {isDescriptionExpanded ? "折りたたむ" : "続きを読む"}
              </Button>
            )}
          </VStack>
        )}

        <HStack gap={2} fontSize="xs" color="var(--text-muted)" flexWrap="wrap">
          {result.published && (
            <Text>
              {new Date(result.published).toLocaleDateString("ja-JP", {
                year: "numeric",
                month: "short",
                day: "numeric",
              })}
            </Text>
          )}
          {result.authors && result.authors.length > 0 && (
            <>
              {result.published && <Text>•</Text>}
              <Text>{result.authors[0].name}</Text>
            </>
          )}
        </HStack>

        <Button
          size="sm"
          variant="outline"
          onClick={handleToggleSummary}
          width="full"
          color="var(--text-primary)"
          borderColor="var(--surface-border)"
          _hover={{
            bg: "var(--surface-hover)",
            borderColor: "var(--alt-primary)",
          }}
        >
          {isExpanded ? "要約を閉じる" : "要約を表示"}
        </Button>

        {isExpanded && (
          <Box
            p={4}
            bg="rgba(255, 255, 255, 0.03)"
            borderRadius="8px"
            border="1px solid var(--surface-border)"
            mt={2}
            width="100%"
          >
            {isLoadingSummary ? (
              <HStack justify="center" py={4}>
                <Spinner size="sm" color="var(--alt-primary)" />
                <Text color="var(--text-secondary)" fontSize="sm">
                  要約を読み込み中...
                </Text>
              </HStack>
            ) : isSummarizing ? (
              <VStack gap={3} py={4}>
                <HStack justify="center">
                  <Spinner size="sm" color="var(--alt-primary)" />
                  <Text color="var(--text-secondary)" fontSize="sm">
                    要約を生成中...
                  </Text>
                </HStack>
                <Text color="var(--text-muted)" fontSize="xs" textAlign="center">
                  これには数秒かかる場合があります
                </Text>
              </VStack>
            ) : summaryError ? (
              <VStack gap={3} width="100%">
                <Text color="var(--text-secondary)" fontSize="sm" textAlign="center">
                  {summaryError}
                </Text>
                {summaryError === "要約を取得できませんでした" && (
                  <Button
                    size="sm"
                    colorScheme="blue"
                    onClick={handleSummarizeNow}
                    width="full"
                    bg="var(--alt-primary)"
                    color="#ffffff"
                    _hover={{
                      bg: "var(--alt-secondary)",
                    }}
                  >
                    ✨ Summarize Immediately
                  </Button>
                )}
              </VStack>
            ) : summary?.matched_articles && summary.matched_articles.length > 0 ? (
              <VStack align="start" gap={2} width="100%">
                <Text
                  fontSize="sm"
                  fontWeight="bold"
                  color="var(--alt-primary)"
                  wordBreak="break-word"
                  overflowWrap="anywhere"
                >
                  {summary.matched_articles[0].title}
                </Text>
                <Text
                  fontSize="sm"
                  color="var(--text-primary)"
                  lineHeight="1.7"
                  whiteSpace="pre-wrap"
                  wordBreak="break-word"
                  overflowWrap="anywhere"
                >
                  {summary.matched_articles[0].content}
                </Text>
              </VStack>
            ) : (
              <VStack gap={3} width="100%">
                <Text color="var(--text-secondary)" fontSize="sm" textAlign="center">
                  この記事の要約はまだありません
                </Text>
                <Button
                  size="sm"
                  colorScheme="blue"
                  onClick={handleSummarizeNow}
                  width="full"
                  bg="var(--alt-primary)"
                  color="#ffffff"
                  _hover={{
                    bg: "var(--alt-secondary)",
                  }}
                >
                  ✨ Summarize Immediately
                </Button>
              </VStack>
            )}
          </Box>
        )}
      </VStack>
    </Box>
  );
};

const LoadingState = () => (
  <Box
    bg="var(--surface-bg)"
    borderRadius="0"
    border="2px solid var(--surface-border)"
    p={8}
    textAlign="center"
    boxShadow="var(--shadow-sm)"
  >
    <VStack gap={4}>
      <Spinner size="lg" color="var(--alt-primary)" />
      <Text color="var(--text-secondary)">Searching feeds...</Text>
    </VStack>
  </Box>
);

const EmptyState = ({ searchQuery }: { searchQuery: string }) => (
  <Box
    bg="var(--surface-bg)"
    borderRadius="0"
    border="2px solid var(--surface-border)"
    p={8}
    textAlign="center"
    boxShadow="var(--shadow-sm)"
  >
    <VStack gap={3}>
      <Text fontSize="2xl" color="var(--text-muted)">
        🔍
      </Text>
      <Text color="var(--text-secondary)" fontWeight="medium">
        No results found
      </Text>
      {searchQuery && (
        <Text color="var(--text-muted)" fontSize="sm">
          No feeds match &quot;{searchQuery}&quot;. Try different keywords.
        </Text>
      )}
    </VStack>
  </Box>
);

const SearchStats = ({ count, searchTime }: { count: number; searchTime?: number }) => (
  <HStack justify="space-between" align="center" mb={4}>
    <Text color="var(--alt-primary)" fontWeight="700" fontSize="lg">
      Search Results ({count})
    </Text>
    {searchTime && (
      <Text color="var(--text-muted)" fontSize="sm">
        Found in {searchTime}ms
      </Text>
    )}
  </HStack>
);

export const SearchResults = ({
  results,
  isLoading,
  searchQuery,
  searchTime,
}: SearchResultsProps) => {
  if (isLoading) {
    return <LoadingState />;
  }

  if (!searchQuery.trim()) {
    return null;
  }

  if (results.length === 0) {
    return <EmptyState searchQuery={searchQuery} />;
  }

  return (
    <Box
      bg="var(--surface-bg)"
      borderRadius="0"
      border="2px solid var(--surface-border)"
      p={4}
      boxShadow="var(--shadow-sm)"
    >
      <SearchStats count={results.length} searchTime={searchTime} />

      <Box as="ul" role="list" aria-label="Search results">
        <VStack gap={4} align="stretch">
          {results.map((result, index) => (
            <Box as="li" key={result.link || `result-${index}`} listStyleType="none">
              <SearchResultItem result={result} />
            </Box>
          ))}
        </VStack>
      </Box>
    </Box>
  );
};

export default SearchResults;
