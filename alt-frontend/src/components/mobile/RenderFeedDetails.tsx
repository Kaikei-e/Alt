import { Box, HStack, Text } from "@chakra-ui/react";
import type { CSSObject } from "@emotion/react";
import sanitizeHtml from "sanitize-html";
import { SmartContentRenderer } from "@/components/common/SmartContentRenderer";
import type {
  FeedContentOnTheFlyResponse,
  FetchArticleSummaryResponse,
} from "@/schema/feed";

const fallbackContentStyles: CSSObject = {
  "& table": {
    width: "100%",
    borderCollapse: "collapse",
  },
  "& pre": {
    whiteSpace: "pre-wrap",
    overflowX: "auto",
  },
  "& code": {
    whiteSpace: "pre-wrap",
  },
  "& blockquote": {
    borderLeft: "3px solid rgba(255, 255, 255, 0.2)",
    paddingLeft: "12px",
    marginLeft: 0,
    color: "var(--alt-text-secondary)",
  },
};

// Union type for feedDetails
type RenderFeedDetailsProps =
  | FeedContentOnTheFlyResponse
  | FetchArticleSummaryResponse;

interface RenderFeedDetailsComponentProps {
  feedDetails?: RenderFeedDetailsProps | null;
  isLoading?: boolean;
  error?: string | null;
}

const RenderFeedDetails = ({
  feedDetails,
  isLoading,
  error,
}: RenderFeedDetailsComponentProps) => {
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
  if (
    "matched_articles" in feedDetails &&
    feedDetails.matched_articles &&
    feedDetails.matched_articles.length > 0
  ) {
    const article = feedDetails.matched_articles[0];

    return (
      <Box px={4} py={4}>
        {/* Article Metadata */}
        <Box
          mb={4}
          p={4}
          bg="rgba(255, 255, 255, 0.05)"
          borderRadius="12px"
          border="1px solid rgba(255, 255, 255, 0.1)"
        >
          <Text
            fontSize="xl"
            fontWeight="bold"
            color="var(--text-primary)"
            mb={2}
            lineHeight="1.4"
          >
            {article.title}
          </Text>

          <HStack gap={3} fontSize="md" color="var(--alt-text-secondary)">
            {article.author && <Text>By {article.author}</Text>}

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
        <Box
          bg="rgba(255, 255, 255, 0.02)"
          borderRadius="8px"
          p={3}
          border="1px solid rgba(255, 255, 255, 0.05)"
        >
          <SmartContentRenderer
            content={article.content}
            contentType={article.content_type}
            className="article-content"
            maxHeight="50vh"
            showMetadata={true}
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
      <Box px={4} py={4}>
        <Box
          color="var(--text-primary)"
          lineHeight="1.6"
          fontSize="md"
          maxW="100%"
          overflowWrap="anywhere"
          wordBreak="break-word"
          css={fallbackContentStyles}
        >
          {/* content sanitized with sanitize-html */}
          <Box
            as="div"
            display="block"
            width="100%"
            wordBreak="break-word"
            overflowWrap="anywhere"
            dangerouslySetInnerHTML={{
              __html: sanitizeHtml(feedDetails.content, {
                allowedTags: [
                  "p",
                  "br",
                  "strong",
                  "b",
                  "em",
                  "i",
                  "u",
                  "a",
                  "h1",
                  "h2",
                  "h3",
                  "h4",
                  "h5",
                  "h6",
                  "ul",
                  "ol",
                  "li",
                  "blockquote",
                  "pre",
                  "code",
                  "table",
                  "thead",
                  "tbody",
                  "tr",
                  "td",
                  "th",
                  "div",
                  "span",
                ],
                allowedAttributes: {
                  a: ["href", "title", "target", "rel"],
                },
                allowedSchemes: ["http", "https", "mailto", "tel"],
                allowProtocolRelative: false,
                disallowedTagsMode: "discard",
                transformTags: {
                  a: (tagName, attribs) => {
                    const href = attribs.href || "";
                    // Only allow safe URL schemes
                    if (!/^(?:(?:https?|mailto|tel):|\/(?!\/)|#)/i.test(href)) {
                      // Remove href attribute for unsafe URLs
                      const safeAttribs = { ...attribs };
                      delete safeAttribs.href;
                      return { tagName, attribs: safeAttribs };
                    }
                    // Force safe attributes on links
                    return {
                      tagName,
                      attribs: {
                        ...attribs,
                        rel: "noopener noreferrer nofollow ugc",
                      },
                    };
                  },
                },
              }),
            }}
          />
        </Box>
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
