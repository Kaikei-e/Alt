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
      bg="var(--surface-bg)"
      p={4}
      borderRadius="0"
      border="2px solid var(--surface-border)"
      _hover={{
        bg: "var(--surface-hover)",
        borderColor: "var(--alt-primary)",
        boxShadow: "var(--shadow-md)",
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
            color="var(--alt-primary)"
            fontWeight="700"
            _hover={{
              textDecoration: "underline",
              color: "var(--alt-secondary)",
              textDecorationThickness: "2px",
            }}
            lineHeight="1.3"
            letterSpacing="-0.025em"
          >
            {result.title}
          </Heading>
        </Link>

        {result.description && (
          <Text color="var(--text-secondary)" fontSize="sm" lineHeight="1.7">
            {result.description}
          </Text>
        )}

        <HStack gap={2} fontSize="xs" color="var(--text-muted)">
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
    bg="var(--surface-bg)"
    borderRadius="0"
    border="2px solid var(--surface-border)"
    p={8}
    textAlign="center"
    boxShadow="var(--shadow-sm)"
  >
    <VStack gap={4}>
      <Spinner size="lg" color="var(--alt-primary)" />
      <Text color="var(--text-secondary)">Searching feeds...</Text>
    </VStack>
  </Box>
);

const EmptyState = ({ searchQuery }: { searchQuery: string }) => (
  <Box
    bg="var(--surface-bg)"
    borderRadius="0"
    border="2px solid var(--surface-border)"
    p={8}
    textAlign="center"
    boxShadow="var(--shadow-sm)"
  >
    <VStack gap={3}>
      <Text fontSize="2xl" color="var(--text-muted)">
        🔍
      </Text>
      <Text color="var(--text-secondary)" fontWeight="medium">
        No results found
      </Text>
      {searchQuery && (
        <Text color="var(--text-muted)" fontSize="sm">
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
    <Text color="var(--alt-primary)" fontWeight="700" fontSize="lg">
      Search Results ({count})
    </Text>
    {searchTime && (
      <Text color="var(--text-muted)" fontSize="sm">
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
      bg="var(--surface-bg)"
      borderRadius="0"
      border="2px solid var(--surface-border)"
      p={4}
      boxShadow="var(--shadow-sm)"
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
