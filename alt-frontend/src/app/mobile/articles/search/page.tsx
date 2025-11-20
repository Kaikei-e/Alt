"use client";

import { CircularProgress } from "@chakra-ui/progress";
import { Box, Text, VStack } from "@chakra-ui/react";
import { Suspense, useState } from "react";
import { ArticleSearchResults } from "@/components/mobile/search/ArticleSearchResults";
import { SearchArticles } from "@/components/mobile/search/SearchArticles";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import type { Article } from "@/schema/article";

export default function SearchPage() {
  const [articles, setArticles] = useState<Article[]>([]);
  const [query, setQuery] = useState<string>("");
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [searchTime, setSearchTime] = useState<number>();

  return (
    <Box minHeight="100dvh" bg="var(--app-bg)" color="var(--foreground)" p={4}>
      <VStack gap={6} align="stretch" maxWidth="600px" mx="auto">
        {/* Header Section */}
        <VStack gap={3} mt={4} mb={2}>
          <Text
            fontSize="2xl"
            fontWeight="700"
            textAlign="center"
            color="var(--text-primary)"
            letterSpacing="-0.025em"
          >
            Search Articles
          </Text>
          <Text
            textAlign="center"
            color="var(--text-secondary)"
            fontSize="md"
            maxWidth="400px"
            lineHeight="1.7"
          >
            Explore articles from your subscribed feeds with powerful full-text
            search
          </Text>
        </VStack>

        {/* Search Input Section */}
        <Box
          bg="var(--bg-glass)"
          backdropFilter="blur(12px)"
          p={6}
          borderRadius="24px"
          border="1px solid var(--border-glass)"
          boxShadow="var(--shadow-glass)"
        >
          <Suspense fallback={<CircularProgress isIndeterminate />}>
            <SearchArticles
              articles={articles}
              setArticles={setArticles}
              query={query}
              setQuery={setQuery}
              error={error}
              setError={setError}
              isLoading={isLoading}
              setIsLoading={setIsLoading}
              setSearchTime={setSearchTime}
            />
          </Suspense>
        </Box>

        {/* Error Message */}
        {error && (
          <Box
            bg="var(--bg-glass)"
            backdropFilter="blur(12px)"
            p={4}
            borderRadius="24px"
            border="1px solid #dc2626"
            boxShadow="var(--shadow-glass)"
          >
            <Text
              color="#dc2626"
              fontSize="sm"
              textAlign="center"
              fontWeight="medium"
            >
              {error}
            </Text>
          </Box>
        )}

        {/* Search Results Section */}
        <ArticleSearchResults
          results={articles}
          isLoading={isLoading}
          searchQuery={query}
          searchTime={searchTime}
        />

        {/* Quick Tips */}
        {!query && !isLoading && articles.length === 0 && !error && (
          <Box
            bg="var(--bg-glass)"
            backdropFilter="blur(12px)"
            p={4}
            borderRadius="24px"
            border="1px solid var(--border-glass)"
            boxShadow="var(--shadow-glass)"
          >
            <Text
              color="var(--text-secondary)"
              fontSize="sm"
              textAlign="center"
              lineHeight="1.7"
            >
              💡 Try searching for topics like &quot;AI&quot;,
              &quot;technology&quot;, or &quot;news&quot;
            </Text>
          </Box>
        )}
      </VStack>

      <FloatingMenu />
    </Box>
  );
}
