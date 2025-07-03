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
      bg="var(--alt-gradient-bg)"
      color="white"
      p={4}
    >
      <VStack gap={6} align="stretch" maxWidth="500px" mx="auto">
        <Text
          fontSize="2xl"
          fontWeight="bold"
          textAlign="center"
          color="var(--text-primary)"
          mt={8}
        >
          Register RSS Feed
        </Text>

        <Text textAlign="center" color="var(--alt-text-secondary)" fontSize="sm">
          Enter the URL of an RSS feed to add it to your collection
        </Text>

        <Box
          bg="var(--alt-glass)"
          p={6}
          borderRadius="lg"
          border="1px solid var(--alt-glass-border)"
        >
          <form onSubmit={handleSubmit}>
            <VStack gap={4}>
              <Box width="full">
                <Text
                  color="var(--text-primary)"
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
                  bg="var(--surface-bg)"
                  border="1px solid var(--alt-primary)"
                  color="var(--text-primary)"
                  _placeholder={{ color: "var(--text-muted)" }}
                  _focus={{
                    borderColor: "var(--alt-primary)",
                    boxShadow: "0 0 0 1px var(--alt-primary)",
                  }}
                />
              </Box>

              <Button
                type="submit"
                loading={isLoading}
                bg="var(--accent-gradient)"
                color="white"
                fontWeight="bold"
                px={8}
                py={6}
                borderRadius="full"
                _hover={{
                  bg: "var(--accent-gradient)",
                  transform: "translateY(-2px)",
                }}
                _active={{
                  transform: "translateY(0px)",
                }}
                transition="all 0.2s ease"
                border="1px solid var(--alt-glass-border)"
                width="full"
                disabled={isLoading}
              >
                {isLoading ? "Registering..." : "Register Feed"}
              </Button>

              {message && (
                <Text
                  color={message.type === "success" ? "var(--alt-success)" : "var(--alt-error)"}
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
          color="var(--alt-text-secondary)"
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
