"use client";

import { Box, Button, Container, Heading, Input, Text, VStack } from "@chakra-ui/react";
import { useState } from "react";
import * as v from "valibot";
import { feedsApi } from "@/lib/api";
import { feedUrlSchema } from "@/schema/validation/feedUrlSchema";

export default function DesktopRegisterFeedsPage() {
  const [feedUrl, setFeedUrl] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [message, setMessage] = useState<{
    type: "success" | "error";
    text: string;
  } | null>(null);

  const validateUrl = (url: string): string | null => {
    if (!url.trim()) {
      return "Please enter a feed URL";
    }

    const result = v.safeParse(feedUrlSchema, { feed_url: url.trim() });
    if (!result.success) {
      return result.issues[0].message;
    }

    return null;
  };

  const handleUrlChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setFeedUrl(value);

    // Clear previous validation error
    setValidationError(null);

    // Real-time validation for better UX
    if (value.trim()) {
      const error = validateUrl(value);
      setValidationError(error);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    const error = validateUrl(feedUrl);
    if (error) {
      setValidationError(error);
      return;
    }

    setIsLoading(true);
    setMessage(null);
    setValidationError(null);

    try {
      const response = await feedsApi.registerRssFeed(feedUrl.trim());
      if (response.message) {
        setMessage({
          type: "success",
          text: "Feed registered successfully!",
        });
        setFeedUrl(""); // Clear form on success
      } else {
        setMessage({
          type: "error",
          text: "Failed to register feed. Please try again.",
        });
      }
    } catch {
      setMessage({
        type: "error",
        text: "Failed to register feed. Please check the URL and try again.",
      });
    }

    setIsLoading(false);
  };

  return (
    <Container maxW="container.md" py={12}>
      <VStack gap={8} align="stretch">
        <Box textAlign="center">
          <Heading size="xl" mb={4}>
            Register RSS Feed
          </Heading>
          <Text color="gray.600" fontSize="lg">
            Enter the URL of an RSS feed to add it to your collection
          </Text>
        </Box>

        <Box className="glass" p={8} borderRadius="18px" maxW="500px" mx="auto" w="full">
          <form onSubmit={handleSubmit}>
            <VStack gap={6}>
              <Box width="full">
                <Text color="gray.700" mb={3} fontSize="md" fontWeight="semibold">
                  Feed URL
                </Text>
                <Input
                  type="url"
                  value={feedUrl}
                  onChange={handleUrlChange}
                  placeholder="https://example.com/feed.xml"
                  size="lg"
                  bg="white"
                  border={`2px solid ${validationError ? "red.400" : "gray.200"}`}
                  _focus={{
                    borderColor: validationError ? "red.400" : "pink.400",
                    boxShadow: `0 0 0 1px ${validationError ? "red.400" : "pink.400"}`,
                  }}
                />
                {validationError && (
                  <Text color="red.500" fontSize="sm" mt={2}>
                    {validationError}
                  </Text>
                )}
              </Box>

              <Button
                type="submit"
                loading={isLoading}
                bg="pink.400"
                color="white"
                fontWeight="bold"
                size="lg"
                px={12}
                py={6}
                borderRadius="full"
                _hover={{
                  bg: "pink.500",
                  transform: "translateY(-2px)",
                }}
                _active={{
                  transform: "translateY(0px)",
                }}
                transition="all 0.2s ease"
                width="full"
                disabled={isLoading || !!validationError}
              >
                {isLoading ? "Registering..." : "Register Feed"}
              </Button>

              {message && (
                <Text
                  color={message.type === "success" ? "green.500" : "red.500"}
                  textAlign="center"
                  fontSize="md"
                  fontWeight="medium"
                  p={4}
                  bg={message.type === "success" ? "green.50" : "red.50"}
                  borderRadius="md"
                  border={`1px solid ${message.type === "success" ? "green.200" : "red.200"}`}
                >
                  {message.text}
                </Text>
              )}
            </VStack>
          </form>
        </Box>

        <Text textAlign="center" color="gray.500" fontSize="sm" mt={6}>
          Make sure the URL points to a valid RSS or Atom feed
        </Text>
      </VStack>
    </Container>
  );
}
