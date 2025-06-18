import { Box, VStack, Text, HStack, Heading, Spinner } from "@chakra-ui/react";
import Link from "next/link";
import { BackendFeedItem } from "@/schema/feed";

interface SearchResultsProps {
  results: BackendFeedItem[];
  isLoading: boolean;
  searchQuery: string;
  searchTime?: number;
}

interface SearchResultItemProps {
  result: BackendFeedItem;
}

const SearchResultItem = ({ result }: SearchResultItemProps) => {
  return (
    <Box
      bg="rgba(255, 255, 255, 0.03)"
      p={4}
      borderRadius="md"
      border="1px solid rgba(255, 255, 255, 0.1)"
      _hover={{
        bg: "rgba(255, 255, 255, 0.08)",
        transform: "translateY(-1px)",
      }}
      transition="all 0.2s ease"
      role="article"
      aria-label={`Search result: ${result.title}`}
    >
      <VStack align="start" gap={2}>
        <Link
          href={result.link || "#"}
          target="_blank"
          rel="noopener noreferrer"
        >
          <Heading
            as="h3"
            size="md"
            color="#ff006e"
            fontWeight="bold"
            _hover={{
              textDecoration: "underline",
              color: "#e6005c",
            }}
            lineHeight="1.3"
          >
            {result.title}
          </Heading>
        </Link>

        {result.description && (
          <Text color="rgba(255, 255, 255, 0.8)" fontSize="sm" lineHeight="1.4">
            {result.description}
          </Text>
        )}

        <HStack gap={2} fontSize="xs" color="rgba(255, 255, 255, 0.6)">
          {result.published && (
            <Text>
              {new Date(result.published).toLocaleDateString("en-US", {
                year: "numeric",
                month: "short",
                day: "numeric",
              })}
            </Text>
          )}
          {result.authors && result.authors.length > 0 && (
            <>
              {result.published && <Text>•</Text>}
              <Text>{result.authors[0].name}</Text>
            </>
          )}
        </HStack>
      </VStack>
    </Box>
  );
};

const LoadingState = () => (
  <Box
    bg="rgba(255, 255, 255, 0.05)"
    borderRadius="lg"
    border="1px solid rgba(255, 255, 255, 0.1)"
    p={8}
    textAlign="center"
  >
    <VStack gap={4}>
      <Spinner size="lg" color="#ff006e" />
      <Text color="rgba(255, 255, 255, 0.8)">Searching feeds...</Text>
    </VStack>
  </Box>
);

const EmptyState = ({ searchQuery }: { searchQuery: string }) => (
  <Box
    bg="rgba(255, 255, 255, 0.05)"
    borderRadius="lg"
    border="1px solid rgba(255, 255, 255, 0.1)"
    p={8}
    textAlign="center"
  >
    <VStack gap={3}>
      <Text fontSize="2xl" color="rgba(255, 255, 255, 0.5)">
        🔍
      </Text>
      <Text color="rgba(255, 255, 255, 0.8)" fontWeight="medium">
        No results found
      </Text>
      {searchQuery && (
        <Text color="rgba(255, 255, 255, 0.6)" fontSize="sm">
          No feeds match &quot;{searchQuery}&quot;. Try different keywords.
        </Text>
      )}
    </VStack>
  </Box>
);

const SearchStats = ({
  count,
  searchTime,
}: {
  count: number;
  searchTime?: number;
}) => (
  <HStack justify="space-between" align="center" mb={4}>
    <Text color="#ff006e" fontWeight="bold" fontSize="lg">
      Search Results ({count})
    </Text>
    {searchTime && (
      <Text color="rgba(255, 255, 255, 0.6)" fontSize="sm">
        Found in {searchTime}ms
      </Text>
    )}
  </HStack>
);

export const SearchResults = ({
  results,
  isLoading,
  searchQuery,
  searchTime,
}: SearchResultsProps) => {
  if (isLoading) {
    return <LoadingState />;
  }

  if (!searchQuery.trim()) {
    return null;
  }

  if (results.length === 0) {
    return <EmptyState searchQuery={searchQuery} />;
  }

  return (
    <Box
      bg="rgba(255, 255, 255, 0.05)"
      borderRadius="lg"
      border="1px solid rgba(255, 255, 255, 0.1)"
      p={4}
    >
      <SearchStats count={results.length} searchTime={searchTime} />

      <Box as="ul" role="list" aria-label="Search results">
        <VStack gap={4} align="stretch">
          {results.map((result, index) => (
            <Box
              as="li"
              key={result.link || `result-${index}`}
              listStyleType="none"
            >
              <SearchResultItem result={result} />
            </Box>
          ))}
        </VStack>
      </Box>
    </Box>
  );
};

export default SearchResults;
