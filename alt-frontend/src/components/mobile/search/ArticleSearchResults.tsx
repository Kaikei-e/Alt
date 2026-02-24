"use client";

import { CircularProgress } from "@chakra-ui/progress";
import { Box, Text, VStack } from "@chakra-ui/react";
import { ArticleCard } from "@/components/mobile/ArticleCard";
import type { Article } from "@/schema/article";

interface ArticleSearchResultsProps {
  results: Article[];
  isLoading: boolean;
  searchQuery: string;
  searchTime?: number;
}

export const ArticleSearchResults = ({
  results,
  isLoading,
  searchQuery,
  searchTime,
}: ArticleSearchResultsProps) => {
  if (isLoading) {
    return (
      <Box
        bg="var(--surface-bg)"
        p={6}
        borderRadius="0"
        border="2px solid var(--surface-border)"
        boxShadow="var(--shadow-sm)"
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="200px"
      >
        <VStack gap={3}>
          <CircularProgress isIndeterminate color="var(--alt-primary)" />
          <Text color="var(--text-secondary)" fontSize="sm">
            Searching articles...
          </Text>
        </VStack>
      </Box>
    );
  }

  if (!searchQuery) {
    return null;
  }

  if (results.length === 0) {
    return (
      <Box
        bg="var(--surface-bg)"
        p={6}
        borderRadius="0"
        border="2px solid var(--surface-border)"
        boxShadow="var(--shadow-sm)"
        data-testid="search-empty-state"
      >
        <VStack gap={3}>
          <Text
            color="var(--text-primary)"
            fontSize="lg"
            fontWeight="600"
            textAlign="center"
            data-testid="search-empty-heading"
          >
            No articles found
          </Text>
          <Text
            color="var(--text-secondary)"
            fontSize="sm"
            textAlign="center"
            lineHeight="1.7"
          >
            No articles match &quot;{searchQuery}&quot;. Try different keywords
            or check your spelling.
          </Text>
        </VStack>
      </Box>
    );
  }

  return (
    <VStack gap={4} align="stretch" data-testid="search-results">
      {/* Search metadata */}
      <Box
        bg="var(--surface-bg)"
        p={4}
        borderRadius="0"
        border="2px solid var(--surface-border)"
        boxShadow="var(--shadow-sm)"
        data-testid="search-metadata"
      >
        <VStack gap={2} align="start">
          <Text
            color="var(--text-primary)"
            fontSize="sm"
            fontWeight="600"
            data-testid="search-count"
          >
            Found {results.length} article{results.length !== 1 ? "s" : ""}
          </Text>
          {searchTime !== undefined && (
            <Text color="var(--text-secondary)" fontSize="xs">
              Search completed in {searchTime}ms
            </Text>
          )}
        </VStack>
      </Box>

      {/* Article cards */}
      {results.map((article) => (
        <ArticleCard key={article.id} article={article} />
      ))}
    </VStack>
  );
};
