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
      bg="var(--alt-gradient-bg)"
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
            color="var(--alt-text-primary)"
            bgGradient="var(--accent-gradient)"
            bgClip="text"
          >
            Search Feeds
          </Text>
          <Text
            textAlign="center"
            color="var(--alt-text-secondary)"
            fontSize="md"
            maxWidth="400px"
          >
            Discover content across your RSS feeds with intelligent search
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
            bg="var(--alt-glass)"
            p={4}
            borderRadius="lg"
            border="1px solid var(--alt-glass-border)"
          >
            <Text
              color="var(--alt-text-secondary)"
              fontSize="sm"
              textAlign="center"
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
