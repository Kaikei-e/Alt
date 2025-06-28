import { HStack, Text, Box, Portal, Button, Flex } from "@chakra-ui/react";
import { useState, useEffect, useCallback } from "react";
import { FeedDetails as FeedDetailsType, FeedURLPayload } from "@/schema/feed";
import { feedsApi } from "@/lib/api";

export const FeedDetails = ({ feedURL }: { feedURL: string }) => {
  const [feedDetails, setFeedDetails] = useState<FeedDetailsType | null>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
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
      const details = await feedsApi.getFeedDetails({
        feed_url: feedURL,
      } as FeedURLPayload);
      setFeedDetails(details);
    } catch (err) {
      console.error("Error fetching feed details:", err);
      setError("Summary not available for this article");
      setFeedDetails(null);
    } finally {
      setIsLoading(false);
      setIsOpen(true);
    }
  };

  const getDisplayContent = () => {
    if (isLoading) {
      return "Loading summary...";
    }

    if (error) {
      return error;
    }

    if (feedDetails?.summary) {
      return feedDetails.summary;
    }

    return "No summary available for this article";
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
          bg="linear-gradient(45deg, #ff006e, #8338ec)"
          color="white"
          fontWeight="bold"
          px={4}
          minHeight="44px"
          minWidth="44px"
          _hover={{
            bg: "linear-gradient(45deg, #e6005c, #7129d4)",
            transform: "translateY(-1px)",
          }}
          _active={{
            transform: "translateY(0px)",
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
            bg="rgba(0, 0, 0, 0.8)"
            backdropFilter="blur(8px)"
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
            _active={{
              bg: "rgba(0, 0, 0, 0.85)",
            }}
            data-testid="modal-backdrop"
            role="dialog"
            aria-modal="true"
            aria-labelledby="summary-header"
            aria-describedby="summary-content"
          >
            <Box
              onClick={(e) => e.stopPropagation()}
              width="85vw"
              maxWidth="450px"
              height="80vh"
              maxHeight="80vh"
              minHeight="400px"
              background="#1a1a2e"
              borderRadius="lg"
              boxShadow="xl"
              border="1px solid rgba(255, 255, 255, 0.1)"
              display="flex"
              flexDirection="column"
              data-testid="modal-content"
              tabIndex={-1}
            >
              <Box
                position="sticky"
                top="0"
                zIndex="1"
                bg="#1a1a2e"
                height="60px"
                minHeight="60px"
                backdropFilter="blur(16px)"
                borderBottom="1px solid rgba(255,255,255,0.15)"
                p={6}
                data-testid="summary-header"
                id="summary-header"
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
                <Flex
                  justify="space-between"
                  align="center"
                  flexDirection="row"
                >
                  <Text color="#ff006e" fontWeight="bold" fontSize="md">
                    Article Summary
                  </Text>
                  <Button
                    onClick={handleHideDetails}
                    data-testid={`hide-details-button-${uniqueId}`}
                    className="hide-details-button"
                    size="sm"
                    borderRadius="full"
                    bg="linear-gradient(45deg, #ff006e, #8338ec)"
                    color="white"
                    fontWeight="bold"
                    px={4}
                    minHeight="44px"
                    minWidth="44px"
                    _hover={{
                      bg: "linear-gradient(45deg, #e6005c, #7129d4)",
                      transform: "translateY(-1px)",
                    }}
                    _active={{
                      transform: "translateY(0px)",
                    }}
                    transition="all 0.2s ease"
                    border="1px solid rgba(255, 255, 255, 0.2)"
                  >
                    <Text>Hide Details</Text>
                  </Button>
                </Flex>
              </Box>
              <Box
                flex="1"
                overflow="auto"
                paddingX="16px"
                paddingY="12px"
                scrollBehavior="smooth"
                overscrollBehavior="contain"
                willChange="scroll-position"
                data-testid="scrollable-content"
                id="summary-content"
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
                <Text
                  data-testid={`summary-text-${uniqueId}`}
                  className="summary-text"
                  color={
                    error
                      ? "rgba(255, 255, 255, 0.6)"
                      : "rgba(255, 255, 255, 0.9)"
                  }
                  fontSize="sm"
                  lineHeight="1.6"
                  fontStyle={
                    error || !feedDetails?.summary ? "italic" : "normal"
                  }
                >
                  {getDisplayContent()}
                </Text>
              </Box>
            </Box>
          </Box>
        </Portal>
      )}
    </HStack>
  );
};
