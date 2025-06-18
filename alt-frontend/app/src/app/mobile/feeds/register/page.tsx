"use client";

import { Box, Button, Input, VStack, Text } from "@chakra-ui/react";
import { useState } from "react";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { feedsApi } from "@/lib/api";

export default function RegisterFeedsPage() {
  const [feedUrl, setFeedUrl] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{
    type: "success" | "error";
    text: string;
  } | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!feedUrl.trim()) {
      setMessage({
        type: "error",
        text: "Please enter a feed URL",
      });
      return;
    }

    setIsLoading(true);
    setMessage(null);

    const response = await feedsApi.registerRssFeed(feedUrl);
    if (response.message) {
      setMessage({
        type: "success",
        text: "Feed registered successfully!",
      });
    } else {
      setMessage({
        type: "error",
        text: "Failed to register feed. Please try again.",
      });
    }

    setIsLoading(false);
  };

  return (
    <Box
      minHeight="100vh"
      bg="linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)"
      color="white"
      p={4}
    >
      <VStack gap={6} align="stretch" maxWidth="500px" mx="auto">
        <Text
          fontSize="2xl"
          fontWeight="bold"
          textAlign="center"
          color="#ff006e"
          mt={8}
        >
          Register RSS Feed
        </Text>

        <Text textAlign="center" color="rgba(255, 255, 255, 0.7)" fontSize="sm">
          Enter the URL of an RSS feed to add it to your collection
        </Text>

        <Box
          bg="rgba(255, 255, 255, 0.05)"
          p={6}
          borderRadius="lg"
          border="1px solid rgba(255, 255, 255, 0.1)"
        >
          <form onSubmit={handleSubmit}>
            <VStack gap={4}>
              <Box width="full">
                <Text
                  color="rgba(255, 255, 255, 0.9)"
                  mb={2}
                  fontSize="sm"
                  fontWeight="medium"
                >
                  Feed URL
                </Text>
                <Input
                  type="url"
                  value={feedUrl}
                  onChange={(e) => setFeedUrl(e.target.value)}
                  placeholder="https://example.com/feed.xml"
                  bg="rgba(255, 255, 255, 0.1)"
                  border="1px solid rgba(255, 255, 255, 0.2)"
                  color="white"
                  _placeholder={{ color: "rgba(255, 255, 255, 0.5)" }}
                  _focus={{
                    borderColor: "#ff006e",
                    boxShadow: "0 0 0 1px #ff006e",
                  }}
                />
              </Box>

              <Button
                type="submit"
                loading={isLoading}
                bg="linear-gradient(45deg, #ff006e, #8338ec)"
                color="white"
                fontWeight="bold"
                px={8}
                py={6}
                borderRadius="full"
                _hover={{
                  bg: "linear-gradient(45deg, #e6005c, #7129d4)",
                  transform: "translateY(-2px)",
                }}
                _active={{
                  transform: "translateY(0px)",
                }}
                transition="all 0.2s ease"
                border="1px solid rgba(255, 255, 255, 0.2)"
                width="full"
                disabled={isLoading}
              >
                {isLoading ? "Registering..." : "Register Feed"}
              </Button>

              {message && (
                <Text
                  color={message.type === "success" ? "#4ade80" : "#f87171"}
                  textAlign="center"
                  fontSize="sm"
                  fontWeight="medium"
                >
                  {message.text}
                </Text>
              )}
            </VStack>
          </form>
        </Box>

        <Text
          textAlign="center"
          color="rgba(255, 255, 255, 0.6)"
          fontSize="xs"
          mt={4}
        >
          Make sure the URL points to a valid RSS or Atom feed
        </Text>
      </VStack>

      <FloatingMenu />
    </Box>
  );
}
