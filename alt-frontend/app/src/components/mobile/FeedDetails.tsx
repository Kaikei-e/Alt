import { HStack, Text, Box, Portal, Button } from "@chakra-ui/react";
import { useState, useEffect, useCallback } from "react";
import { IoClose } from "react-icons/io5";
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
          minWidth="120px"
          fontSize="sm"
          _hover={{
            bg: "linear-gradient(45deg, #e6005c, #7129d4)",
            transform: "scale(1.05)",
            boxShadow: "0 4px 12px rgba(255, 0, 110, 0.4)",
          }}
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
            bg="rgba(0, 0, 0, 0.85)"
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
              bg: "rgba(0, 0, 0, 0.9)",
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
              boxShadow="0 25px 50px rgba(255, 0, 110, 0.3)"
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
                bg="linear-gradient(135deg, #1a1a2e 0%, #16213e 100%)"
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
                  color="#ff006e"
                  fontWeight="bold"
                  fontSize="md"
                  textShadow="0 2px 4px rgba(255, 0, 110, 0.3)"
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
                    background: "rgba(255, 255, 255, 0.1)",
                    borderRadius: "3px",
                  },
                  "&::-webkit-scrollbar-thumb": {
                    background: "linear-gradient(45deg, #ff006e, #8338ec)",
                    borderRadius: "3px",
                  },
                  "&::-webkit-scrollbar-thumb:hover": {
                    background: "linear-gradient(45deg, #e6005c, #7129d4)",
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
                <Text
                  data-testid={`summary-text-${uniqueId}`}
                  className="summary-text"
                  color={
                    error
                      ? "rgba(255, 255, 255, 0.7)"
                      : "rgba(255, 255, 255, 0.95)"
                  }
                  fontSize="md"
                  lineHeight="1.7"
                  fontStyle={
                    error || !feedDetails?.summary ? "italic" : "normal"
                  }
                  textAlign="justify"
                  letterSpacing="0.3px"
                  pb="16px" // Reduced padding since button is now in modal footer
                >
                  {getDisplayContent()}
                </Text>
              </Box>

              {/* Modal Footer with Hide Details button */}
              <Box
                position="sticky"
                bottom="0"
                zIndex="2"
                bg="linear-gradient(135deg, #1a1a2e 0%, #16213e 100%)"
                backdropFilter="blur(20px)"
                borderTop="1px solid rgba(255, 255, 255, 0.1)"
                px={6}
                py={3}
                borderBottomRadius="20px"
                display="flex"
                alignItems="center"
                justifyContent="flex-end"
                minHeight="50px"
              >
                <Button
                  onClick={handleHideDetails}
                  data-testid={`hide-details-button-${uniqueId}`}
                  className="hide-details-button"
                  size="sm"
                  borderRadius="full"
                  bg="linear-gradient(45deg, #ff006e, #8338ec)"
                  color="white"
                  fontWeight="bold"
                  p={2.5}
                  minHeight="36px"
                  minWidth="36px"
                  fontSize="md"
                  boxShadow="0 6px 20px rgba(255, 0, 110, 0.4)"
                  _hover={{
                    bg: "linear-gradient(45deg, #e6005c, #7129d4)",
                    transform: "scale(1.1)",
                    boxShadow: "0 8px 25px rgba(255, 0, 110, 0.6)",
                  }}
                  _active={{
                    transform: "scale(0.95)",
                  }}
                  transition="all 0.2s ease"
                  border="1.5px solid rgba(255, 255, 255, 0.3)"
                >
                  <IoClose />
                </Button>
              </Box>
            </Box>
          </Box>
        </Portal>
      )}
    </HStack>
  );
};
