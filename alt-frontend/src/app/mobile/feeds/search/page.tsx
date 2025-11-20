"use client";

import { Box, Text, VStack } from "@chakra-ui/react";
import { useState } from "react";
import SearchResults from "@/components/mobile/search/SearchResults";
import SearchWindow from "@/components/mobile/search/SearchWindow";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import type { BackendFeedItem } from "@/schema/feed";
import type { SearchQuery } from "@/schema/validation/searchQuery";

export default function SearchFeedsPage() {
  const [searchQuery, setSearchQuery] = useState<SearchQuery>({ query: "" });
  const [results, setResults] = useState<BackendFeedItem[]>([]);
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
            Search Feeds
          </Text>
          <Text
            textAlign="center"
            color="var(--text-secondary)"
            fontSize="md"
            maxWidth="400px"
            lineHeight="1.7"
          >
            Discover content across your RSS feeds with intelligent search
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
