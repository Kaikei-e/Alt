import { HStack, Text, Box, Button } from "@chakra-ui/react";
import type { CSSObject } from "@emotion/react";
import { useState, useEffect, useCallback } from "react";
import { X, Star, Archive } from "lucide-react";
import {
  FetchArticleSummaryResponse,
  FeedContentOnTheFlyResponse,
} from "@/schema/feed";
import { feedsApi } from "@/lib/api";
import RenderFeedDetails from "./RenderFeedDetails";

const scrollAreaStyles: CSSObject = {
  "&::-webkit-scrollbar": {
    width: "4px",
  },
  "&::-webkit-scrollbar-track": {
    background: "transparent",
    borderRadius: "2px",
  },
  "&::-webkit-scrollbar-thumb": {
    background: "rgba(255, 255, 255, 0.2)",
    borderRadius: "2px",
  },
  "&::-webkit-scrollbar-thumb:hover": {
    background: "rgba(255, 255, 255, 0.3)",
  },
};

interface FeedDetailsProps {
  feedURL?: string;
  feedTitle?: string;
  initialData?: FetchArticleSummaryResponse | FeedContentOnTheFlyResponse;
}

export const FeedDetails = ({
  feedURL,
  feedTitle,
  initialData,
}: FeedDetailsProps) => {
  const [articleSummary, setArticleSummary] =
    useState<FetchArticleSummaryResponse | null>(
      initialData && "matched_articles" in initialData ? initialData : null,
    );
  const [feedDetails, setFeedDetails] =
    useState<FeedContentOnTheFlyResponse | null>(
      initialData && "content" in initialData ? initialData : null,
    );
  const [isOpen, setIsOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isFavoriting, setIsFavoriting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isBookmarked, setIsBookmarked] = useState(false);
  const [isArchiving, setIsArchiving] = useState(false);
  const [isArchived, setIsArchived] = useState(false);
  const [summary, setSummary] = useState<string | null>(null);
  const [isSummarizing, setIsSummarizing] = useState(false);

  const handleHideDetails = useCallback(() => {
    setIsOpen(false);
    setIsArchived(false);
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
    setIsArchived(false);

    // If we already have initial data, just open the modal
    if (initialData) {
      setIsOpen(true);
      return;
    }

    if (!feedURL) {
      setError("No feed URL available");
      setIsOpen(true);
      return;
    }

    setIsLoading(true);
    setError(null);

    // Fetch both summary and content independently
    const summaryPromise = feedsApi.getArticleSummary(feedURL).catch((err) => {
      console.error("Error fetching article summary:", err);
      return null;
    });

    const detailsPromise = feedsApi
      .getFeedContentOnTheFly({
        feed_url: feedURL,
      })
      .catch((err) => {
        console.error("Error fetching article content:", err);
        return null;
      });

    try {
      const [summary, details] = await Promise.all([
        summaryPromise,
        detailsPromise,
      ]);

      console.log("Summary response:", summary);
      console.log("Details response:", details);

      // Check if summary has valid content
      const hasValidSummary =
        summary &&
        summary.matched_articles &&
        summary.matched_articles.length > 0;
      // Check if details has valid content
      const hasValidDetails =
        details && details.content && details.content.trim() !== "";

      if (hasValidSummary) {
        setArticleSummary(summary);
        console.log("Setting article summary:", summary);
      }

      if (hasValidDetails) {
        setFeedDetails(details);
        console.log("Setting feed details:", details);
      }

      // If neither API call succeeded with valid content, show error
      if (!hasValidSummary && !hasValidDetails) {
        setError("Unable to fetch article content");
        console.log("No valid content from either API");
      }

    } catch (err) {
      console.error("Unexpected error:", err);
      setError("Unexpected error occurred");
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
          _hover={{
            filter: "brightness(1.06)",
            transform: "translateY(-1px)",
          }}
          _active={{
            transform: "scale(0.98)",
          }}
          transition="all 0.2s ease"
          border="1px solid rgba(255, 255, 255, 0.2)"
          disabled={isLoading}
        >
          {isLoading ? (
            <HStack gap={2} alignItems="center">
              {/* Spinner imported lazily to avoid bundle bloat */}
              {/* Using a simple dot instead to avoid additional import */}
              <Text as="span">Loading</Text>
            </HStack>
          ) : (
            "Show Details"
          )}
        </Button>
      )}

      {isOpen && (
        <div>
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
              width="95vw"
              maxWidth="450px"
              height="85vh"
              maxHeight="700px"
              minHeight="400px"
              background="var(--app-bg)"
              borderRadius="16px"
              boxShadow="0 20px 40px rgba(0, 0, 0, 0.3)"
              border="1px solid rgba(255, 255, 255, 0.1)"
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
                bg="rgba(255, 255, 255, 0.05)"
                height="60px"
                minHeight="60px"
                backdropFilter="blur(20px)"
                borderBottom="1px solid rgba(255, 255, 255, 0.1)"
                px={4}
                py={3}
                data-testid="summary-header"
                id="summary-header"
                borderTopRadius="16px"
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
                px={0}
                py={0}
                bg="transparent"
                scrollBehavior="smooth"
                overscrollBehavior="contain"
                willChange="scroll-position"
                data-testid="scrollable-content"
                id="summary-content"
                position="relative"
                css={scrollAreaStyles}
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

                {/* Render content based on data type */}
                <RenderFeedDetails
                  feedDetails={articleSummary || feedDetails}
                  isLoading={isLoading}
                  error={error}
                />

                {/* Display Japanese Summary */}
                {summary && (
                  <Box
                    mt={4}
                    px={4}
                    py={4}
                    bg="rgba(255, 255, 255, 0.03)"
                    borderRadius="12px"
                    border="1px solid rgba(255, 255, 255, 0.1)"
                    mx={4}
                    mb={4}
                  >
                    <Text
                      fontSize="xs"
                      color="var(--text-secondary)"
                      fontWeight="bold"
                      mb={2}
                      textTransform="uppercase"
                      letterSpacing="1px"
                    >
                      日本語要約 / Japanese Summary
                    </Text>
                    <Text
                      fontSize="sm"
                      color="var(--text-primary)"
                      lineHeight="1.7"
                      whiteSpace="pre-wrap"
                    >
                      {summary}
                    </Text>
                  </Box>
                )}
              </Box>

              {/* Modal Footer with action buttons */}
              <Box
                position="sticky"
                bottom="0"
                zIndex="2"
                bg="rgba(255, 255, 255, 0.05)"
                backdropFilter="blur(20px)"
                borderTop="1px solid rgba(255, 255, 255, 0.1)"
                px={3}
                py={3}
                borderBottomRadius="16px"
                display="flex"
                alignItems="center"
                justifyContent="space-between"
                minHeight="60px"
                gap={2}
              >
                <Button
                  onClick={async () => {
                    if (!feedURL) return;
                    try {
                      setIsFavoriting(true);
                      await feedsApi.registerFavoriteFeed(feedURL);
                      setIsBookmarked(true);
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
                  p={2}
                  minHeight="36px"
                  minWidth="36px"
                  fontSize="sm"
                  border="1px solid rgba(255, 255, 255, 0.2)"
                  disabled={isFavoriting || isBookmarked}
                  title="Favorite"
                >
                  <Star size={16} />
                </Button>
              <Button
                onClick={async () => {
                  if (!feedURL) return;
                  try {
                    setIsArchiving(true);
                    await feedsApi.archiveContent(feedURL, feedTitle);
                    setIsArchived(true);
                  } catch (e) {
                    console.error("Error archiving feed:", e);
                  } finally {
                      setIsArchiving(false);
                    }
                  }}
                  size="sm"
                  borderRadius="full"
                  bg="var(--alt-primary)"
                  color="var(--text-primary)"
                  fontWeight="bold"
                  px={3}
                  minHeight="36px"
                  minWidth="auto"
                  fontSize="xs"
                  border="1px solid rgba(255, 255, 255, 0.2)"
                  disabled={isArchiving || isArchived}
                  title="Archive"
                >
                  <Archive size={14} style={{ marginRight: 4 }} />
                  {isArchiving
                    ? "..."
                    : isArchived
                      ? "✓"
                      : "Archive"}
                </Button>
                <Button
                  onClick={async () => {
                    if (!feedURL) return;
                    try {
                      setIsSummarizing(true);
                      const result = await feedsApi.summarizeArticle(feedURL);
                      setSummary(result.summary);
                    } catch (e) {
                      console.error("Failed to summarize article", e);
                      setSummary("要約の生成に失敗しました。");
                    } finally {
                      setIsSummarizing(false);
                    }
                  }}
                  size="sm"
                  borderRadius="full"
                  bg="var(--alt-secondary)"
                  color="var(--text-primary)"
                  fontWeight="bold"
                  px={3}
                  minHeight="36px"
                  minWidth="auto"
                  fontSize="xs"
                  border="1px solid rgba(255, 255, 255, 0.2)"
                  disabled={isSummarizing || !!summary}
                  title="Summarize to Japanese"
                  _hover={{
                    filter: "brightness(1.1)",
                  }}
                >
                  {isSummarizing ? "要約中..." : summary ? "✓ 要約済" : "要約"}
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
        </div>
      )}
    </HStack>
  );
};
