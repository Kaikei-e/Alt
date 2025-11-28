"use client";

import { Box, Button, HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import Link from "next/link";
import { memo, useState } from "react";
import { articleApi } from "@/lib/api";
import type { FetchArticleSummaryResponse } from "@/schema/feed";
import type { SearchFeedItem } from "@/schema/search";
import { SearchResultsVirtualList } from "./SearchResultsVirtualList";

interface SearchResultsProps {
  results: SearchFeedItem[];
  isLoading: boolean;
  searchQuery: string;
  searchTime?: number;
}

interface SearchResultItemProps {
  result: SearchFeedItem;
}

export const SearchResultItem = memo(({ result }: SearchResultItemProps) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const [summary, setSummary] = useState<FetchArticleSummaryResponse | null>(
    null,
  );
  const [isLoadingSummary, setIsLoadingSummary] = useState(false);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const [isSummarizing, setIsSummarizing] = useState(false);
  const [isDescriptionExpanded, setIsDescriptionExpanded] = useState(false);

  // Check if description is long enough to need truncation
  // Trim to check if there's actual content (not just whitespace)
  const descriptionText = (result.description || "").trim();
  const hasDescription = descriptionText.length > 0;
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
        setSummaryError("Failed to fetch summary");
      } finally {
        setIsLoadingSummary(false);
      }
    }
    setIsExpanded(!isExpanded);
  };

  const handleSummarizeNow = async () => {
    if (!result.link) return;

    setIsSummarizing(true);
    try {
      const summaryResponse = await articleApi.getArticleSummary(result.link);
      setSummary(summaryResponse);
      setIsExpanded(true);
    } catch (error) {
      console.error("Error generating summary:", error);
      setSummaryError("Failed to fetch summary");
    } finally {
      setIsSummarizing(false);
    }
  };

  return (
    <Box
      bg="var(--bg-glass)"
      backdropFilter="blur(12px)"
      borderRadius="24px"
      border="1px solid var(--border-glass)"
      boxShadow="var(--shadow-glass)"
      p={5}
      transition="all 0.3s ease"
      _hover={{
        transform: "translateY(-2px)",
        boxShadow: "0 8px 30px rgba(0,0,0,0.12)",
        borderColor: "var(--accent-primary)",
      }}
    >
      <VStack align="stretch" gap={3}>
        <Link href={result.link || "#"} target="_blank">
          <Text
            fontSize="md"
            fontWeight="600"
            color="var(--text-primary)"
            _hover={{ color: "var(--alt-primary)" }}
            transition="color 0.2s"
            lineHeight="1.4"
          >
            {result.title}
          </Text>
        </Link>

        {result.published && (
          <HStack
            justify="space-between"
            fontSize="xs"
            color="var(--text-secondary)"
          >
            <Text>
              {result.author?.name || result.authors?.[0]?.name || "Unknown"}
            </Text>
            <Text>{new Date(result.published).toLocaleDateString()}</Text>
          </HStack>
        )}

        {hasDescription && (
          <Box>
            <Text
              color="var(--text-secondary)"
              lineHeight="1.6"
              {...(isDescriptionExpanded ? {} : { isTruncated: true })}
            >
              {displayDescription}
            </Text>
            {shouldTruncateDescription && (
              <Button
                size="xs"
                variant="ghost"
                onClick={() => setIsDescriptionExpanded(!isDescriptionExpanded)}
                mt={2}
                width="100%"
              >
                {isDescriptionExpanded ? "Show less" : "Read more"}
              </Button>
            )}
          </Box>
        )}

        {isExpanded && (
          <Box mt={3}>
            {isLoadingSummary ? (
              <HStack justify="center" py={4}>
                <Spinner size="sm" color="var(--alt-primary)" />
                <Text color="var(--text-secondary)" fontSize="sm">
                  Loading summary...
                </Text>
              </HStack>
            ) : isSummarizing ? (
              <VStack gap={3} py={4}>
                <HStack justify="center">
                  <Spinner size="sm" color="var(--alt-primary)" />
                  <Text color="var(--text-secondary)" fontSize="sm">
                    Generating summary...
                  </Text>
                </HStack>
                <Text
                  color="var(--text-muted)"
                  fontSize="xs"
                  textAlign="center"
                >
                  This may take a few seconds
                </Text>
              </VStack>
            ) : summaryError ? (
              <VStack gap={3} width="100%">
                <Text
                  color="var(--text-secondary)"
                  fontSize="sm"
                  textAlign="center"
                >
                  {summaryError}
                </Text>
                {summaryError === "Failed to fetch summary" && (
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
            ) : summary?.matched_articles &&
              summary.matched_articles.length > 0 ? (
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
                <Text
                  color="var(--text-secondary)"
                  fontSize="sm"
                  textAlign="center"
                >
                  No summary available for this article
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

        <HStack gap={2} mt={3}>
          <Button
            size="xs"
            variant="outline"
            onClick={handleToggleSummary}
            width="full"
            bg="var(--bg-surface)"
            borderColor="var(--border-glass)"
            color="var(--text-primary)"
            _hover={{
              bg: "var(--alt-primary-alpha)",
              borderColor: "var(--alt-primary)",
            }}
          >
            {isExpanded ? "Hide summary" : "Show summary"}
          </Button>
        </HStack>
      </VStack>
    </Box>
  );
});

SearchResultItem.displayName = "SearchResultItem";

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
    bg="var(--bg-glass)"
    backdropFilter="blur(12px)"
    borderRadius="24px"
    border="1px solid var(--border-glass)"
    p={8}
    textAlign="center"
    boxShadow="var(--shadow-glass)"
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

const SearchStats = ({
  count,
  searchTime,
}: {
  count: number;
  searchTime?: number;
}) => (
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

export const SearchResults = memo(
  ({ results, isLoading, searchQuery, searchTime }: SearchResultsProps) => {
    if (isLoading) {
      return <LoadingState />;
    }

    if (!searchQuery.trim()) {
      return null;
    }

    if (results.length === 0) {
      return <EmptyState searchQuery={searchQuery} />;
    }

    const shouldUseVirtualList = results.length > 20;

    return (
      <Box bg="transparent" p={0}>
        <SearchStats count={results.length} searchTime={searchTime} />

        {shouldUseVirtualList ? (
          <Box height="60vh" mt={4}>
            <SearchResultsVirtualList results={results} />
          </Box>
        ) : (
          <Box as="ul" role="list" aria-label="Search results">
            <VStack gap={4} align="stretch">
              {results.map((result, index) => (
                <Box
                  as="li"
                  key={result.link || `result-${index}`}
                  listStyleType="none"
                >
                  <SearchResultItem result={result} />
                </Box>
              ))}
            </VStack>
          </Box>
        )}
      </Box>
    );
  },
);

SearchResults.displayName = "SearchResults";

export default SearchResults;
