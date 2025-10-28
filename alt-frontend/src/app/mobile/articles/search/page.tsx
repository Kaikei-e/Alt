"use client";

import { SearchArticles } from "@/components/mobile/search/SearchArticles";
import { ArticleSearchResults } from "@/components/mobile/search/ArticleSearchResults";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { Box, VStack, Text } from "@chakra-ui/react";
import { useState, Suspense } from "react";
import { Article } from "@/schema/article";
import { CircularProgress } from "@chakra-ui/progress";

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
        <VStack gap={3} mt={8} mb={2}>
          <Text
            fontSize="3xl"
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
          bg="var(--surface-bg)"
          p={6}
          borderRadius="0"
          border="2px solid var(--surface-border)"
          boxShadow="var(--shadow-sm)"
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
            bg="var(--surface-bg)"
            p={4}
            borderRadius="0"
            border="2px solid #dc2626"
            boxShadow="var(--shadow-sm)"
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
            bg="var(--surface-bg)"
            p={4}
            borderRadius="0"
            border="2px solid var(--surface-border)"
            boxShadow="var(--shadow-sm)"
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
