import { HStack, Text, Box } from "@chakra-ui/react";
import type { FeedContentOnTheFlyResponse, FetchArticleSummaryResponse } from "@/schema/feed";
import { SmartContentRenderer } from "@/components/common/SmartContentRenderer";

// Union type for feedDetails
type RenderFeedDetailsProps = FeedContentOnTheFlyResponse | FetchArticleSummaryResponse;

interface RenderFeedDetailsComponentProps {
  feedDetails?: RenderFeedDetailsProps | null;
  isLoading?: boolean;
  error?: string | null;
}

const RenderFeedDetails = ({ feedDetails, isLoading, error }: RenderFeedDetailsComponentProps) => {
  // Loading state
  if (isLoading) {
    return (
      <Text
        fontStyle="italic"
        color="var(--alt-text-secondary)"
        textAlign="center"
        py={8}
      >
        Loading summary...
      </Text>
    );
  }

  // Error state
  if (error) {
    return (
      <Text
        fontStyle="italic"
        color="var(--alt-text-secondary)"
        textAlign="center"
        py={8}
      >
        {error}
      </Text>
    );
  }

  // No data available
  if (!feedDetails) {
    return (
      <Text
        fontStyle="italic"
        color="var(--alt-text-secondary)"
        textAlign="center"
        py={8}
      >
        No summary available for this article
      </Text>
    );
  }

  // FetchArticleSummaryResponse (has matched_articles property) - Rich article display
  // Check this first to prioritize Inoreader summary when available
  if ('matched_articles' in feedDetails && feedDetails.matched_articles && feedDetails.matched_articles.length > 0) {
    const article = feedDetails.matched_articles[0];

    return (
      <Box px={6} py={5}>
        {/* Article Metadata */}
        <Box
          mb={4}
          p={3}
          bg="rgba(255, 255, 255, 0.05)"
          borderRadius="12px"
          border="1px solid rgba(255, 255, 255, 0.1)"
        >
          <Text
            fontSize="lg"
            fontWeight="bold"
            color="var(--text-primary)"
            mb={2}
            lineHeight="1.4"
          >
            {article.title}
          </Text>

          <HStack
            gap={3}
            fontSize="sm"
            color="var(--alt-text-secondary)"
          >
            {article.author && (
              <Text>By {article.author}</Text>
            )}

            <Text>
              {new Date(article.published_at).toLocaleDateString("ja-JP", {
                year: "numeric",
                month: "short",
                day: "numeric",
              })}
            </Text>

            <Text
              px={2}
              py={1}
              bg="var(--accent-primary)"
              borderRadius="full"
              fontSize="xs"
              fontWeight="semibold"
            >
              {article.content_type}
            </Text>
          </HStack>
        </Box>

        {/* Smart Content Rendering */}
        <SmartContentRenderer
          content={article.content}
          contentType={article.content_type}
          className="article-content"
          maxHeight="60vh"
          showMetadata={true}
          onError={(error) => {
            console.error("Content rendering error:", error);
          }}
        />
      </Box>
    );
  }

  // FeedContentOnTheFlyResponse (has content property) - Simple content display
  // This is the fallback when Inoreader summary is not available
  if ('content' in feedDetails) {
    return (
      <Box px={6} py={5}>
        <Text
          color="var(--text-primary)"
          lineHeight="1.6"
          fontSize="md"
        >
          {feedDetails.content}
        </Text>
      </Box>
    );
  }

  // No data available
  return (
    <Text
      fontStyle="italic"
      color="var(--alt-text-secondary)"
      textAlign="center"
      py={8}
    >
      No summary available for this article
    </Text>
  );
};

export default RenderFeedDetails;