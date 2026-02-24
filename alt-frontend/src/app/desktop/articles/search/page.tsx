"use client";

import {
  Box,
  Button,
  Heading,
  HStack,
  Input,
  Text,
  VStack,
} from "@chakra-ui/react";
import { useState } from "react";
import { articleApi } from "@/lib/api";
import type { Article } from "@/schema/article";

export default function DesktopArticlesSearchPage() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Article[]>([]);
  const [searched, setSearched] = useState(false);

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!query.trim()) return;

    try {
      // Use articleApi to search
      const data = await articleApi.searchArticles(query);
      setResults(data);
      setSearched(true);
    } catch (error) {
      console.error("Search failed:", error);
      setResults([]);
      setSearched(true);
    }
  };

  return (
    <Box p={8} data-testid="search-page">
      <Heading size="lg" mb={6} data-testid="search-heading">
        Article Search
      </Heading>
      <form onSubmit={handleSearch}>
        <HStack mb={8}>
          <Input
            placeholder="Search articles..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            data-testid="search-input"
          />
          <Button type="submit" colorPalette="blue" data-testid="search-button">
            Search
          </Button>
        </HStack>
      </form>

      <VStack align="stretch" gap={4} data-testid="search-results">
        {results.map((article) => (
          <Box
            key={article.id}
            p={4}
            borderWidth="1px"
            borderRadius="lg"
            data-testid="search-result"
          >
            <Heading size="md">{article.title}</Heading>
            <Text mt={2}>{article.content?.substring(0, 100)}...</Text>
          </Box>
        ))}
        {searched && results.length === 0 && (
          <Text data-testid="search-empty-state">No results found.</Text>
        )}
      </VStack>
    </Box>
  );
}
