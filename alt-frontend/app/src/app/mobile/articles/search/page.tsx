"use client";

import { SearchArticles } from "@/components/mobile/search/SearchArticles";
import { ArticleCard } from "@/components/mobile/ArticleCard";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { Flex, Box } from "@chakra-ui/react";
import { useState, Suspense } from "react";
import { Article } from "@/schema/article";
import { CircularProgress } from "@chakra-ui/progress";

export default function SearchPage() {
  const [articles, setArticles] = useState<Article[]>([]);
  const [query, setQuery] = useState<string>("");
  const [error, setError] = useState<string | null>(null);

  return (
    <Box
      width="100%"
      className="feed-container"
      minHeight="100vh"
      minH="100dvh"
      position="relative"
    >
      <Flex
        flexDirection="column"
        alignItems="center"
        width="100%"
        px={4}
        pt={6}
        pb="calc(80px + env(safe-area-inset-bottom))"
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
        {articles.length > 0 && articles.map((article) => (
          <ArticleCard key={article.id} article={article} />
        ))}
      </Flex>
      <FloatingMenu />
    </Box>
  );
}