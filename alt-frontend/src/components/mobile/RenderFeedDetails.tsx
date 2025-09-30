import { HStack, Text, Box } from "@chakra-ui/react";
import type { CSSObject } from "@emotion/react";
import type {
  FeedContentOnTheFlyResponse,
  FetchArticleSummaryResponse,
} from "@/schema/feed";
import { SmartContentRenderer } from "@/components/common/SmartContentRenderer";
import DOMPurify from "isomorphic-dompurify";

// Harden DOMPurify: enforce safe link/img handling and URL schemes
// Guard to avoid duplicate hook registration on HMR/SSR
if (!(globalThis as any).__ALT_DOMPURIFY_HOOKS__) {
  DOMPurify.addHook("afterSanitizeAttributes", (node: Element) => {
    const nodeName = node.nodeName ? node.nodeName.toLowerCase() : "";

    if (nodeName === "a") {
      const href = node.getAttribute("href") || "";
      const isAllowedHref = /^(?:(?:https?|mailto|tel):|\/(?!\/)|#)/i.test(
        href,
      );
      if (!isAllowedHref) {
        node.removeAttribute("href");
      }
      // Force safe target and rel on external links
      const target = node.getAttribute("target");
      if (target && target.toLowerCase() === "_blank") {
        node.setAttribute("target", "_blank");
      } else {
        node.removeAttribute("target");
      }
      node.setAttribute("rel", "noopener noreferrer nofollow ugc");
    }

    if (nodeName === "img") {
      const src = node.getAttribute("src") || "";
      // Allow only http(s), relative, or specific data:image formats (exclude SVG for safety)
      const isAllowedImgSrc =
        /^(?:(?:https?):|\/(?!\/)|data:image\/(?:png|jpeg|jpg|gif|webp);base64,)/i.test(
          src,
        );
      if (!isAllowedImgSrc) {
        node.removeAttribute("src");
      }
      // Extra hardening even though not allowed in config
      node.removeAttribute("srcset");
      node.removeAttribute("onerror");
      node.removeAttribute("onload");
    }
  });

  (globalThis as any).__ALT_DOMPURIFY_HOOKS__ = true;
}

const fallbackContentStyles: CSSObject = {
  "& img": {
    maxWidth: "100%",
    height: "auto",
    borderRadius: "8px",
  },
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
            fontSize="lg"
            fontWeight="bold"
            color="var(--text-primary)"
            mb={2}
            lineHeight="1.4"
          >
            {article.title}
          </Text>

          <HStack gap={3} fontSize="sm" color="var(--alt-text-secondary)">
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
          {/* content wrapped in quotes to transform it into a html string by secure DOMPurify*/}
          <Box
            as="div"
            display="block"
            width="100%"
            wordBreak="break-word"
            overflowWrap="anywhere"
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(feedDetails.content, {
                ALLOWED_TAGS: [
                  "p",
                  "br",
                  "strong",
                  "b",
                  "em",
                  "i",
                  "u",
                  "a",
                  "img",
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
                ALLOWED_ATTR: [
                  "href",
                  "src",
                  "alt",
                  "title",
                  "target",
                  "rel",
                ],
                FORBID_ATTR: [
                  "onclick",
                  "onload",
                  "onerror",
                  "onmouseover",
                  "onmouseout",
                  "onfocus",
                  "onblur",
                  "style",
                ],
                FORBID_TAGS: [
                  "script",
                  "object",
                  "embed",
                  "form",
                  "input",
                  "textarea",
                  "button",
                  "select",
                  "option",
                  "iframe",
                  "svg",
                  "math",
                ],
                // Disallow dangerous URL schemes; allow https, http, relative, mailto, tel, anchors
                ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|tel):|\/(?!\/)|#)/i,
                KEEP_CONTENT: true,
                SAFE_FOR_TEMPLATES: true,
                RETURN_TRUSTED_TYPE: false,
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
