import { HStack, Text, Box, Portal, Button, Flex } from "@chakra-ui/react";
import { useState } from "react";
import { FeedDetails as FeedDetailsType, FeedURLPayload } from "@/schema/feed";
import { feedsApi } from "@/lib/api";

export const FeedDetails = ({ feedURL }: { feedURL: string }) => {
  const [feedDetails, setFeedDetails] = useState<FeedDetailsType | null>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

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

  const handleHideDetails = () => {
    setIsOpen(false);
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

  return (
    <HStack justify="space-between">
      {!isOpen && (
        <Button
          onClick={handleShowDetails}
          data-testid="show-details-button"
          size="sm"
          borderRadius="full"
          bg="linear-gradient(45deg, #ff006e, #8338ec)"
          color="white"
          fontWeight="bold"
          px={4}
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
            onClick={handleHideDetails}
          >
            <Box
              onClick={(e) => e.stopPropagation()}
              width="90vw"
              maxWidth="500px"
              maxHeight="80vh"
              background="#1a1a2e"
              borderRadius="lg"
              boxShadow="xl"
              border="1px solid rgba(255, 255, 255, 0.1)"
              p={6}
              overflow="auto"
            >
              <Flex
                justify="space-between"
                align="center"
                flexDirection="row"
                mb={4}
              >
                <Text color="#ff006e" fontWeight="bold" fontSize="md" mb={4}>
                  Article Summary
                </Text>
                <Button
                  onClick={handleHideDetails}
                  data-testid="hide-details-button"
                  size="sm"
                  borderRadius="full"
                  bg="linear-gradient(45deg, #ff006e, #8338ec)"
                  color="white"
                  fontWeight="bold"
                  px={4}
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
              <Text
                data-testid="summary-text"
                color={
                  error
                    ? "rgba(255, 255, 255, 0.6)"
                    : "rgba(255, 255, 255, 0.9)"
                }
                fontSize="sm"
                lineHeight="1.5"
                fontStyle={error || !feedDetails?.summary ? "italic" : "normal"}
              >
                {getDisplayContent()}
              </Text>
            </Box>
          </Box>
        </Portal>
      )}
    </HStack>
  );
};
