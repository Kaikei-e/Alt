"use client";

import { SearchArticles } from "@/components/mobile/search/SearchArticles";
import { ArticleCard } from "@/components/mobile/ArticleCard";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { Box, VStack, Text } from "@chakra-ui/react";
import { useState, Suspense } from "react";
import { Article } from "@/schema/article";
import { CircularProgress } from "@chakra-ui/progress";

export default function SearchPage() {
  const [articles, setArticles] = useState<Article[]>([]);
  const [query, setQuery] = useState<string>("");
  const [error, setError] = useState<string | null>(null);

  return (
    <Box minHeight="100vh" bg="var(--alt-gradient-bg)" color="white" p={4}>
      <VStack gap={6} align="stretch" maxWidth="600px" mx="auto">
        {/* Header Section */}
        <VStack gap={3} mt={8} mb={2}>
          <Text
            fontSize="3xl"
            fontWeight="bold"
            textAlign="center"
            color="var(--text-primary)"
            bgGradient="var(--accent-gradient)"
            bgClip="text"
          >
            Search Articles
          </Text>
          <Text
            textAlign="center"
            color="var(--alt-text-secondary)"
            fontSize="md"
            maxWidth="400px"
          >
            Explore articles from your subscribed feeds
          </Text>
        </VStack>

        {/* Search Input Section */}
        <Box
          bg="var(--alt-glass)"
          p={6}
          borderRadius="xl"
          border="1px solid var(--alt-glass-border)"
          boxShadow="0 8px 32px var(--alt-glass-shadow)"
        >
          <Suspense fallback={<CircularProgress isIndeterminate />}>
            <SearchArticles
              articles={articles}
              setArticles={setArticles}
              query={query}
              setQuery={setQuery}
              error={error}
              setError={setError}
            />
          </Suspense>
        </Box>

        {/* Search Results Section */}
        {articles.length > 0 ? (
          <VStack gap={4} align="stretch">
            {articles.map((article) => (
              <ArticleCard key={article.id} article={article} />
            ))}
          </VStack>
        ) : (
          !error && (
            <Box
              bg="var(--alt-glass)"
              p={4}
              borderRadius="lg"
              border="1px solid var(--alt-glass-border)"
            >
              <Text color="var(--alt-text-secondary)" fontSize="sm" textAlign="center">
                {query
                  ? `No articles match "${query}". Try different keywords.`
                  : '💡 Try searching for topics like "AI", "technology", or "news"'}
              </Text>
            </Box>
          )
        )}
      </VStack>

      <FloatingMenu />
    </Box>
  );
}
