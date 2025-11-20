/**
 * DesktopRenderFeedDetails Component
 * Desktop専用のフィード詳細表示コンポーネント
 * mobileコンポーネントとの分離により、desktop固有のスタイルと動作を実現
 */
import { Box, HStack, Text } from "@chakra-ui/react";
import type { CSSObject } from "@emotion/react";
import { SmartContentRenderer } from "@/components/common/SmartContentRenderer";
import type {
  FeedContentOnTheFlyResponse,
  FetchArticleSummaryResponse,
} from "@/schema/feed";

// Desktop向けのスタイル設定（より大きなフォントサイズとパディング）
const desktopContentStyles: CSSObject = {
  "& table": {
    width: "100%",
    borderCollapse: "collapse",
    marginTop: "1em",
    marginBottom: "1em",
  },
  "& pre": {
    whiteSpace: "pre-wrap",
    overflowX: "auto",
    fontSize: "0.95em",
    padding: "1.2em",
    borderRadius: "8px",
  },
  "& code": {
    whiteSpace: "pre-wrap",
    fontSize: "0.95em",
  },
  "& blockquote": {
    borderLeft: "4px solid rgba(255, 255, 255, 0.3)",
    paddingLeft: "1.5em",
    marginLeft: 0,
    marginTop: "1em",
    marginBottom: "1em",
    color: "var(--text-secondary)",
    fontStyle: "italic",
    background: "rgba(255, 255, 255, 0.03)",
    padding: "1.2em",
    borderRadius: "0 8px 8px 0",
  },
  "& p": {
    marginBottom: "1.2em",
    lineHeight: "1.8",
  },
  "& h1, & h2, & h3, & h4, & h5, & h6": {
    marginTop: "1.5em",
    marginBottom: "0.8em",
    fontWeight: "bold",
  },
  "& img": {
    maxWidth: "100%",
    height: "auto",
    borderRadius: "12px",
    marginTop: "1.5em",
    marginBottom: "1.5em",
  },
};

// Union type for feedDetails
type DesktopRenderFeedDetailsProps =
  | FeedContentOnTheFlyResponse
  | FetchArticleSummaryResponse;

interface DesktopRenderFeedDetailsComponentProps {
  feedDetails?: DesktopRenderFeedDetailsProps | null;
  isLoading?: boolean;
  error?: string | null;
  articleUrl?: string;
}

export const DesktopRenderFeedDetails = ({
  feedDetails,
  isLoading,
  error,
  articleUrl,
}: DesktopRenderFeedDetailsComponentProps) => {
  // Loading state
  if (isLoading) {
    return (
      <Box
        display="flex"
        alignItems="center"
        justifyContent="center"
        minH="200px"
      >
        <Text
          fontStyle="italic"
          color="var(--text-secondary)"
          textAlign="center"
          fontSize="lg"
        >
          Loading article content...
        </Text>
      </Box>
    );
  }

  // Error state
  if (error) {
    return (
      <Box
        display="flex"
        alignItems="center"
        justifyContent="center"
        minH="200px"
        p={4}
        bg="rgba(255, 99, 71, 0.1)"
        borderRadius="12px"
        border="1px solid rgba(255, 99, 71, 0.3)"
      >
        <Text
          fontStyle="italic"
          color="var(--text-primary)"
          textAlign="center"
          fontSize="md"
        >
          {error}
        </Text>
      </Box>
    );
  }

  // No data available
  if (!feedDetails) {
    return (
      <Box
        display="flex"
        alignItems="center"
        justifyContent="center"
        minH="200px"
      >
        <Text
          fontStyle="italic"
          color="var(--text-secondary)"
          textAlign="center"
          fontSize="lg"
        >
          No content available for this article
        </Text>
      </Box>
    );
  }

  // FetchArticleSummaryResponse (has matched_articles property) - Rich article display
  // Check this first to prioritize Inoreader summary when available
  if (
    "matched_articles" in feedDetails &&
    feedDetails.matched_articles &&
    feedDetails.matched_articles.length > 0
  ) {
    const article = feedDetails.matched_articles[0];

    return (
      <Box>
        {/* Article Metadata - Desktop optimized */}
        <Box
          mb={6}
          p={6}
          bg="rgba(255, 255, 255, 0.05)"
          borderRadius="16px"
          border="1px solid rgba(255, 255, 255, 0.1)"
        >
          <Text
            fontSize="2xl"
            fontWeight="bold"
            color="var(--text-primary)"
            mb={3}
            lineHeight="1.4"
          >
            {article.title}
          </Text>

          <HStack gap={4} fontSize="md" color="var(--text-secondary)">
            {article.author && (
              <Text fontSize="md" fontWeight="medium">
                By {article.author}
              </Text>
            )}

            <Text fontSize="md">
              {new Date(article.published_at).toLocaleDateString("ja-JP", {
                year: "numeric",
                month: "short",
                day: "numeric",
              })}
            </Text>

            <Box
              px={3}
              py={1.5}
              bg="var(--accent-primary)"
              borderRadius="full"
              fontSize="xs"
              fontWeight="semibold"
            >
              {article.content_type}
            </Box>
          </HStack>
        </Box>

        {/* Smart Content Rendering - Desktop optimized */}
        <Box
          bg="rgba(255, 255, 255, 0.02)"
          borderRadius="12px"
          p={6}
          border="1px solid rgba(255, 255, 255, 0.05)"
          css={desktopContentStyles}
        >
          <SmartContentRenderer
            content={article.content}
            contentType={article.content_type}
            className="article-content-desktop"
            maxHeight="none"
            showMetadata={true}
            articleUrl={articleUrl}
            onError={(error) => {
              console.error("Content rendering error:", error);
            }}
          />
        </Box>
      </Box>
    );
  }

  // FeedContentOnTheFlyResponse (has content property) - Simple content display
  // This is the fallback when Inoreader summary is not available
  if ("content" in feedDetails) {
    return (
      <Box>
        <Box
          color="var(--text-primary)"
          lineHeight="1.8"
          fontSize="lg"
          maxW="100%"
          overflowWrap="anywhere"
          wordBreak="break-word"
          css={desktopContentStyles}
        >
          {/* content is already sanitized server-side as SafeHtmlString */}
          <Box
            as="div"
            display="block"
            width="100%"
            wordBreak="break-word"
            overflowWrap="anywhere"
            dangerouslySetInnerHTML={{
              __html: feedDetails.content, // SafeHtmlString - already sanitized
            }}
          />
        </Box>
      </Box>
    );
  }

  // No data available
  return (
    <Box
      display="flex"
      alignItems="center"
      justifyContent="center"
      minH="200px"
    >
      <Text
        fontStyle="italic"
        color="var(--text-secondary)"
        textAlign="center"
        fontSize="lg"
      >
        No content available for this article
      </Text>
    </Box>
  );
};

export default DesktopRenderFeedDetails;
