"use client";

import { Box, Text, Flex } from "@chakra-ui/react";
import { useState } from "react";
import SearchWindow from "@/components/mobile/search/SearchWindow";
import { SearchQuery } from "@/schema/validation/searchQuery";
import { BackendFeedItem } from "@/schema/feed";

const SearchFeedsPage = () => {
  const [searchQuery, setSearchQuery] = useState<SearchQuery>({ query: "" });
  const [feedResults, setFeedResults] = useState<BackendFeedItem[]>([]);

  return (
    <Box
      width="100%"
      minHeight="100vh"
      minH="100dvh"
      position="relative"
      bg="#0f0f23"
      color="white"
    >
      <Flex
        flexDirection="column"
        alignItems="center"
        width="100%"
        px={4}
        pt={6}
        pb="calc(80px + env(safe-area-inset-bottom))"
      >
        <Box width="100%" maxWidth="500px" mb={6}>
          <Text fontSize="2xl" fontWeight="bold" textAlign="center" mb={4}>
            Search Feeds
          </Text>
          <SearchWindow
            searchQuery={searchQuery}
            setSearchQuery={setSearchQuery}
            feedResults={feedResults}
            setFeedResults={setFeedResults}
          />
        </Box>
      </Flex>
    </Box>
  );
};

export default SearchFeedsPage;
