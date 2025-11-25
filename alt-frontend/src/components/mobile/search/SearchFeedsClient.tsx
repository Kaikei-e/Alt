"use client";

import { Box, Text, VStack } from "@chakra-ui/react";
import { Suspense, useState, useTransition, useDeferredValue } from "react";
import dynamic from "next/dynamic";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import type { SearchFeedItem } from "@/schema/search";
import type { SearchQuery } from "@/schema/validation/searchQuery";

// Dynamic imports with loading states
const SearchWindow = dynamic(
  () => import("@/components/mobile/search/SearchWindow"),
  {
    loading: () => (
      <Box
        bg="var(--bg-glass)"
        backdropFilter="blur(12px)"
        p={6}
        borderRadius="24px"
        border="1px solid var(--border-glass)"
        boxShadow="var(--shadow-glass)"
      >
        <Box
          height="48px"
          bg="var(--bg-surface)"
          borderRadius="12px"
          opacity={0.6}
        />
      </Box>
    ),
    ssr: false,
  }
);

const SearchResults = dynamic(
  () => import("@/components/mobile/search/SearchResults"),
  {
    loading: () => null,
    ssr: false,
  }
);

export default function SearchFeedsClient() {
  const [searchQuery, setSearchQuery] = useState<SearchQuery>({ query: "" });
  const [results, setResults] = useState<SearchFeedItem[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [searchTime, setSearchTime] = useState<number>();
  const [isPending, startTransition] = useTransition();

  // Defer search query updates to reduce validation overhead
  const deferredQuery = useDeferredValue(searchQuery);

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
          <Suspense
            fallback={
              <Box
                height="48px"
                bg="var(--bg-surface)"
                borderRadius="12px"
                opacity={0.6}
              />
            }
          >
            <SearchWindow
              searchQuery={searchQuery}
              setSearchQuery={setSearchQuery}
              feedResults={results}
              setFeedResults={setResults}
              isLoading={isLoading || isPending}
              setIsLoading={setIsLoading}
              setSearchTime={setSearchTime}
            />
          </Suspense>
        </Box>

        {/* Search Results Section */}
        <Suspense fallback={null}>
          <SearchResults
            results={results}
            isLoading={isLoading || isPending}
            searchQuery={deferredQuery.query || ""}
            searchTime={searchTime}
          />
        </Suspense>

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

