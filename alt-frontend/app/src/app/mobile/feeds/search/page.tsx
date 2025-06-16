"use client";

import { Box, VStack, Text } from "@chakra-ui/react";
import { useState } from "react";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import SearchWindow from "@/components/mobile/search/SearchWindow";
import SearchResults from "@/components/mobile/search/SearchResults";
import { BackendFeedItem } from "@/schema/feed";
import { SearchQuery } from "@/schema/validation/searchQuery";

export default function SearchFeedsPage() {
  const [searchQuery, setSearchQuery] = useState<SearchQuery>({ query: "" });
  const [results, setResults] = useState<BackendFeedItem[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [searchTime, setSearchTime] = useState<number>();

  return (
    <Box
      minHeight="100vh"
      bg="linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)"
      color="white"
      p={4}
    >
      <VStack gap={6} align="stretch" maxWidth="600px" mx="auto">
        {/* Header Section */}
        <VStack gap={3} mt={8} mb={2}>
          <Text
            fontSize="3xl"
            fontWeight="bold"
            textAlign="center"
            color="#ff006e"
            bgGradient="linear(to-r, #ff006e, #8338ec)"
            bgClip="text"
          >
            Search Feeds
          </Text>
          <Text
            textAlign="center"
            color="rgba(255, 255, 255, 0.7)"
            fontSize="md"
            maxWidth="400px"
          >
            Discover content across your RSS feeds with intelligent search
          </Text>
        </VStack>

        {/* Search Input Section */}
        <Box
          bg="rgba(255, 255, 255, 0.05)"
          p={6}
          borderRadius="xl"
          border="1px solid rgba(255, 255, 255, 0.1)"
          boxShadow="0 8px 32px rgba(0, 0, 0, 0.3)"
        >
          <SearchWindow
            searchQuery={searchQuery}
            setSearchQuery={setSearchQuery}
            feedResults={results}
            setFeedResults={setResults}
            isLoading={isLoading}
            setIsLoading={setIsLoading}
            setSearchTime={setSearchTime}
          />
        </Box>

        {/* Search Results Section */}
        <SearchResults
          results={results}
          isLoading={isLoading}
          searchQuery={searchQuery.query || ""}
          searchTime={searchTime}
        />

        {/* Quick Tips */}
        {!searchQuery.query && !isLoading && results.length === 0 && (
          <Box
            bg="rgba(255, 255, 255, 0.03)"
            p={4}
            borderRadius="lg"
            border="1px solid rgba(255, 255, 255, 0.1)"
          >
            <Text
              color="rgba(255, 255, 255, 0.6)"
              fontSize="sm"
              textAlign="center"
            >
              💡 Try searching for topics like "AI", "technology", or "news"
            </Text>
          </Box>
        )}
      </VStack>

      <FloatingMenu />
    </Box>
  );
}