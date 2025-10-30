/**
 * SmartContentRenderer Component
 * Intelligently renders content based on type detection with security and optimization
 */
import React, { memo, useMemo } from "react";
import { Box, Text } from "@chakra-ui/react";
import { renderingRegistry } from "@/utils/renderingStrategies";
import { analyzeContent, ContentAnalysis } from "@/utils/contentTypeDetector";

interface SmartContentRendererProps {
  content: string;
  contentType?: string;
  className?: string;
  maxHeight?: string;
  showMetadata?: boolean;
  onError?: (error: Error) => void;
  articleUrl?: string;
}

interface ContentMetadataProps {
  analysis: ContentAnalysis;
}

/**
 * Content metadata display component
 */
const ContentMetadata: React.FC<ContentMetadataProps> = memo(({ analysis }) => {
  if (!analysis) return null;

  return (
    <Box
      fontSize="xs"
      color="var(--alt-text-secondary)"
      mb={3}
      p={2}
      bg="rgba(255, 255, 255, 0.03)"
      borderRadius="8px"
      border="1px solid rgba(255, 255, 255, 0.08)"
      display="flex"
      flexWrap="wrap"
      gap={3}
    >
      <Text>
        <strong>Type:</strong> {analysis.type.toUpperCase()}
      </Text>
      <Text>
        <strong>Words:</strong> {analysis.wordCount.toLocaleString()}
      </Text>
      <Text>
        <strong>Read time:</strong> {analysis.estimatedReadingTime}min
      </Text>
      {analysis.hasImages && (
        <Text color="var(--accent-primary)">📷 Images</Text>
      )}
      {analysis.hasLinks && <Text color="var(--accent-primary)">🔗 Links</Text>}
      {analysis.needsSanitization && (
        <Text color="orange.400">⚠️ Sanitized</Text>
      )}
    </Box>
  );
});

ContentMetadata.displayName = "ContentMetadata";

/**
 * Error fallback component for content rendering errors
 */
const ContentErrorFallback: React.FC<{
  error: string;
  originalContent: string;
}> = memo(({ error, originalContent }) => (
  <Box
    p={4}
    bg="rgba(255, 0, 0, 0.1)"
    border="1px solid rgba(255, 0, 0, 0.3)"
    borderRadius="8px"
    color="var(--text-primary)"
  >
    <Text fontSize="sm" color="red.400" mb={2}>
      ⚠️ Content rendering error: {error}
    </Text>
    <Text fontSize="xs" color="var(--alt-text-secondary)" mb={2}>
      Displaying as plain text:
    </Text>
    <Text
      fontSize="sm"
      fontFamily="monospace"
      bg="rgba(0, 0, 0, 0.3)"
      p={2}
      borderRadius="4px"
      whiteSpace="pre-wrap"
      overflow="auto"
      maxHeight="200px"
    >
      {originalContent.length > 500
        ? originalContent.substring(0, 500) + "..."
        : originalContent}
    </Text>
  </Box>
));

ContentErrorFallback.displayName = "ContentErrorFallback";

/**
 * Main SmartContentRenderer component
 */
export const SmartContentRenderer: React.FC<SmartContentRendererProps> = memo(
  ({
    content,
    contentType,
    className,
    maxHeight = "none",
    showMetadata = false,
    onError,
    articleUrl,
  }) => {
    // Memoize content analysis for performance
    const analysis = useMemo(() => {
      if (!content || content.trim().length === 0) {
        return null;
      }

      try {
        return analyzeContent(content, contentType);
      } catch (error) {
        console.error("Content analysis failed:", error);
        onError?.(error as Error);
        return null;
      }
    }, [content, contentType, onError]);

    // Memoize rendered content for performance
    const renderedContent = useMemo(() => {
      if (!content || content.trim().length === 0) {
        return (
          <Text fontStyle="italic" color="var(--alt-text-secondary)">
            No content available
          </Text>
        );
      }

      try {
        return renderingRegistry.render(content, contentType, articleUrl);
      } catch (error) {
        console.error("Content rendering failed:", error);
        onError?.(error as Error);
        return (
          <ContentErrorFallback
            error={(error as Error).message}
            originalContent={content}
          />
        );
      }
    }, [content, contentType, onError]);

    // Empty content handling
    if (!content || content.trim().length === 0) {
      return (
        <Box className={className}>
          <Text fontStyle="italic" color="var(--alt-text-secondary)">
            No content available
          </Text>
        </Box>
      );
    }

    return (
      <Box className={className}>
        {/* Content metadata (optional) */}
        {showMetadata && analysis && <ContentMetadata analysis={analysis} />}

        {/* Rendered content */}
        <Box
          maxHeight={maxHeight}
          overflow={maxHeight !== "none" ? "auto" : "visible"}
          css={{
            // Custom scrollbar styling
            "&::-webkit-scrollbar": {
              width: "6px",
            },
            "&::-webkit-scrollbar-track": {
              background: "rgba(255, 255, 255, 0.1)",
              borderRadius: "3px",
            },
            "&::-webkit-scrollbar-thumb": {
              background: "rgba(255, 255, 255, 0.3)",
              borderRadius: "3px",
            },
            "&::-webkit-scrollbar-thumb:hover": {
              background: "rgba(255, 255, 255, 0.5)",
            },
            // Content styling
            "& p": {
              marginBottom: "1em",
              lineHeight: "1.7",
            },
            "& h1, & h2, & h3, & h4, & h5, & h6": {
              marginTop: "1.5em",
              marginBottom: "0.5em",
              fontWeight: "bold",
              color: "var(--text-primary)",
            },
            "& h1": { fontSize: "1.5em" },
            "& h2": { fontSize: "1.3em" },
            "& h3": { fontSize: "1.1em" },
            "& ul, & ol": {
              marginLeft: "1.5em",
              marginBottom: "1em",
            },
            "& li": {
              marginBottom: "0.3em",
            },
            "& a": {
              color: "var(--accent-primary)",
              textDecoration: "underline",
              "&:hover": {
                color: "var(--accent-secondary)",
              },
            },
            "& img": {
              maxWidth: "100%",
              height: "auto",
              borderRadius: "8px",
              marginTop: "1em",
              marginBottom: "1em",
            },
            "& blockquote": {
              borderLeft: "3px solid var(--accent-primary)",
              paddingLeft: "1em",
              marginLeft: "0",
              fontStyle: "italic",
              background: "rgba(255, 255, 255, 0.05)",
              padding: "1em",
              borderRadius: "0 8px 8px 0",
            },
            "& pre": {
              background: "rgba(0, 0, 0, 0.3)",
              padding: "1em",
              borderRadius: "8px",
              overflow: "auto",
              fontSize: "0.9em",
            },
            "& code": {
              background: "rgba(0, 0, 0, 0.2)",
              padding: "0.2em 0.4em",
              borderRadius: "3px",
              fontSize: "0.9em",
              fontFamily: "monospace",
            },
            "& table": {
              borderCollapse: "collapse",
              width: "100%",
              marginTop: "1em",
              marginBottom: "1em",
            },
            "& th, & td": {
              border: "1px solid rgba(255, 255, 255, 0.2)",
              padding: "0.5em",
              textAlign: "left",
            },
            "& th": {
              background: "rgba(255, 255, 255, 0.1)",
              fontWeight: "bold",
            },
          }}
        >
          {renderedContent}
        </Box>
      </Box>
    );
  },
);

SmartContentRenderer.displayName = "SmartContentRenderer";
