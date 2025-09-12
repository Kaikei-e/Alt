import { HStack, Text, Box, Portal, Button } from "@chakra-ui/react";
import { useState, useEffect, useCallback } from "react";
import { X, Star } from "lucide-react";
import { FetchArticleSummaryResponse } from "@/schema/feed";
import { feedsApi } from "@/lib/api";
import { SmartContentRenderer } from "@/components/common/SmartContentRenderer";

export const FeedDetails = ({ feedURL }: { feedURL: string }) => {
  const [articleSummary, setArticleSummary] =
    useState<FetchArticleSummaryResponse | null>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isFavoriting, setIsFavoriting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleHideDetails = useCallback(() => {
    setIsOpen(false);
  }, []);

  // Handle escape key to close modal
  useEffect(() => {
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && isOpen) {
        handleHideDetails();
      }
    };

    if (isOpen) {
      document.addEventListener("keydown", handleEscape);
    }

    return () => {
      document.removeEventListener("keydown", handleEscape);
    };
  }, [isOpen, handleHideDetails]);

  const handleShowDetails = async () => {
    if (!feedURL) {
      setError("No feed URL available");
      setIsOpen(true);
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const summary = await feedsApi.getArticleSummary(feedURL);
      setArticleSummary(summary);
    } catch (err) {
      console.error("Error fetching article summary:", err);
      setError("Summary not available for this article");
      setArticleSummary(null);
    } finally {
      setIsLoading(false);
      setIsOpen(true);
    }
  };

  // Create unique test IDs based on feedURL to avoid conflicts
  const uniqueId = feedURL ? btoa(feedURL).slice(0, 8) : "default";

  return (
    <HStack justify="space-between">
      {!isOpen && (
        <Button
          onClick={handleShowDetails}
          data-testid={`show-details-button-${uniqueId}`}
          className="show-details-button"
          size="sm"
          borderRadius="full"
          bg="var(--alt-secondary)"
          color="var(--text-primary)"
          fontWeight="bold"
          px={4}
          minHeight="44px"
          minWidth="120px"
          fontSize="sm"
          _active={{
            transform: "scale(0.98)",
          }}
          transition="all 0.2s ease"
          border="1px solid rgba(255, 255, 255, 0.2)"
        >
          Show Details
        </Button>
      )}

      {isOpen && (
        <Portal>
          <Box
            position="fixed"
            top="0"
            left="0"
            width="100vw"
            height="100vh"
            bg="var(--alt-glass)"
            backdropFilter="blur(12px)"
            zIndex={9999}
            display="flex"
            alignItems="center"
            justifyContent="center"
            onClick={(e) => {
              // Ensure we're clicking on the backdrop itself, not any child elements
              if (e.target === e.currentTarget) {
                handleHideDetails();
              }
            }}
            onTouchEnd={(e) => {
              // Handle touch events for mobile
              if (e.target === e.currentTarget) {
                e.preventDefault();
                handleHideDetails();
              }
            }}
            _active={{
              bg: "var(--surface-bg)",
            }}
            data-testid="modal-backdrop"
            role="dialog"
            aria-modal="true"
            aria-labelledby="summary-header"
            aria-describedby="summary-content"
            p={4}
            style={{ touchAction: "manipulation" }}
          >
            <Box
              onClick={(e) => e.stopPropagation()}
              width="90vw"
              maxWidth="420px"
              height="75vh"
              maxHeight="600px"
              minHeight="350px"
              background="linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)"
              borderRadius="20px"
              boxShadow="0 25px 50px var(--accent-primary)"
              border="1px solid rgba(255, 255, 255, 0.15)"
              display="flex"
              flexDirection="column"
              data-testid="modal-content"
              tabIndex={-1}
              overflow="hidden"
            >
              {/* Header with title only */}
              <Box
                position="sticky"
                top="0"
                zIndex="2"
                bg="var(--app-bg)"
                height="50px"
                minHeight="50px"
                backdropFilter="blur(20px)"
                borderBottom="1px solid rgba(255, 255, 255, 0.1)"
                px={6}
                py={3}
                data-testid="summary-header"
                id="summary-header"
                borderTopRadius="20px"
                display="flex"
                alignItems="center"
                justifyContent="center"
              >
                <Box
                  data-testid="header-area"
                  position="absolute"
                  top="0"
                  left="0"
                  right="0"
                  bottom="0"
                  zIndex="-1"
                />
                <Text
                  color="var(--text-primary)"
                  fontWeight="bold"
                  fontSize="md"
                  textShadow="0 2px 4px var(--alt-glass-shadow)"
                >
                  Article Summary
                </Text>
              </Box>

              {/* Content */}
              <Box
                flex="1"
                overflow="auto"
                px={6}
                py={5}
                bg="var(--app-bg)"
                scrollBehavior="smooth"
                overscrollBehavior="contain"
                willChange="scroll-position"
                data-testid="scrollable-content"
                id="summary-content"
                position="relative"
                css={{
                  "&::-webkit-scrollbar": {
                    width: "6px",
                  },
                  "&::-webkit-scrollbar-track": {
                    background: "var(--app-bg)",
                    borderRadius: "3px",
                  },
                  "&::-webkit-scrollbar-thumb": {
                    background: "var(--app-bg)",
                    borderRadius: "3px",
                  },
                  "&::-webkit-scrollbar-thumb:hover": {
                    background: "var(--app-bg)",
                  },
                }}
              >
                <Box
                  data-testid="content-area"
                  position="absolute"
                  top="0"
                  left="0"
                  right="0"
                  bottom="0"
                  zIndex="-1"
                />

                {/* Article Metadata - only show if we have article data */}
                {articleSummary?.matched_articles &&
                  articleSummary.matched_articles.length > 0 &&
                  !error && (
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
                        {articleSummary.matched_articles[0].title}
                      </Text>

                      <HStack
                        gap={3}
                        fontSize="sm"
                        color="var(--alt-text-secondary)"
                      >
                        {articleSummary.matched_articles[0].author && (
                          <Text>
                            By {articleSummary.matched_articles[0].author}
                          </Text>
                        )}

                        <Text>
                          {new Date(
                            articleSummary.matched_articles[0].published_at,
                          ).toLocaleDateString("ja-JP", {
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
                          {articleSummary.matched_articles[0].content_type}
                        </Text>
                      </HStack>
                    </Box>
                  )}

                {/* Smart Content Rendering */}
                {isLoading ? (
                  <Text
                    fontStyle="italic"
                    color="var(--alt-text-secondary)"
                    textAlign="center"
                    py={8}
                  >
                    Loading summary...
                  </Text>
                ) : error ? (
                  <Text
                    fontStyle="italic"
                    color="var(--alt-text-secondary)"
                    textAlign="center"
                    py={8}
                  >
                    {error}
                  </Text>
                ) : articleSummary?.matched_articles &&
                  articleSummary.matched_articles.length > 0 ? (
                  <SmartContentRenderer
                    content={articleSummary.matched_articles[0].content}
                    contentType={
                      articleSummary.matched_articles[0].content_type
                    }
                    className="article-content"
                    maxHeight="60vh"
                    showMetadata={true}
                    onError={(error) => {
                      console.error("Content rendering error:", error);
                    }}
                  />
                ) : (
                  <Text
                    fontStyle="italic"
                    color="var(--alt-text-secondary)"
                    textAlign="center"
                    py={8}
                  >
                    No summary available for this article
                  </Text>
                )}
              </Box>

              {/* Modal Footer with Fave and Hide Details buttons */}
              <Box
                position="sticky"
                bottom="0"
                zIndex="2"
                bg="var(--app-bg)"
                backdropFilter="blur(20px)"
                borderTop="1px solid rgba(255, 255, 255, 0.1)"
                px={6}
                py={3}
                borderBottomRadius="20px"
                display="flex"
                alignItems="center"
                justifyContent="space-between"
                minHeight="50px"
              >
                <Button
                  onClick={async () => {
                    try {
                      setIsFavoriting(true);
                      await feedsApi.registerFavoriteFeed(feedURL);
                    } catch (e) {
                      console.error("Failed to favorite feed", e);
                    } finally {
                      setIsFavoriting(false);
                    }
                  }}
                  size="sm"
                  borderRadius="full"
                  bg="var(--alt-primary)"
                  color="var(--text-primary)"
                  fontWeight="bold"
                  px={4}
                  minHeight="36px"
                  minWidth="80px"
                  fontSize="sm"
                  border="1px solid rgba(255, 255, 255, 0.2)"
                  disabled={isFavoriting}
                >
                  <Star size={16} style={{ marginRight: 4 }} />
                  {isFavoriting ? "Saving" : "Fave"}
                </Button>
                <Button
                  onClick={handleHideDetails}
                  data-testid={`hide-details-button-${uniqueId}`}
                  className="hide-details-button"
                  size="sm"
                  borderRadius="full"
                  bg="var(--accent-gradient)"
                  color="var(--text-primary)"
                  fontWeight="bold"
                  p={2.5}
                  minHeight="36px"
                  minWidth="36px"
                  fontSize="md"
                  boxShadow="var(--btn-shadow)"
                  transition="all 0.2s ease"
                  border="1.5px solid var(--alt-glass-border)"
                >
                  <X size={16} />
                </Button>
              </Box>
            </Box>
          </Box>
        </Portal>
      )}
    </HStack>
  );
};
