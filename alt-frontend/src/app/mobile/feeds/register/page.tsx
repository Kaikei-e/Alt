"use client";

import { Box, Button, Input, Text, VStack } from "@chakra-ui/react";
import { useState } from "react";
import * as v from "valibot";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { feedApi } from "@/lib/api";
import { feedUrlSchema } from "@/schema/validation/feedUrlSchema";
import { ApiError } from "@/lib/api/core/ApiError";

export default function RegisterFeedsPage() {
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
      const response = await feedApi.registerRssFeed(feedUrl.trim());
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
    } catch (error) {
      let errorMessage = "Failed to register feed. Please check the URL and try again.";

      if (error instanceof ApiError) {
        if (error.code === "TLS_CERTIFICATE_ERROR") {
          // Use the message from backend which includes suggested URL
          errorMessage = error.message || "このURLの証明書に問題があります。別のURLを試してください";
        } else if (error.code === "VALIDATION_ERROR") {
          errorMessage = error.message || "URLの形式が正しくありません";
        } else if (error.code === "EXTERNAL_API_ERROR") {
          errorMessage = "RSSフィードにアクセスできませんでした。URLを確認してください";
        } else if (error.message) {
          errorMessage = error.message;
        }
      } else if (error instanceof Error) {
        errorMessage = error.message;
      }

      setMessage({
        type: "error",
        text: errorMessage,
      });
    }

    setIsLoading(false);
  };

  return (
    <Box minHeight="100dvh" bg="var(--alt-gradient-bg)" color="white" p={4}>
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
                <Text color="var(--text-primary)" mb={2} fontSize="sm" fontWeight="medium">
                  Feed URL
                </Text>
                <Input
                  type="url"
                  value={feedUrl}
                  onChange={handleUrlChange}
                  placeholder="https://example.com/feed.xml"
                  bg="var(--surface-bg)"
                  border={`1px solid ${validationError ? "var(--alt-error)" : "var(--alt-primary)"}`}
                  color="var(--text-primary)"
                  _placeholder={{ color: "var(--text-muted)" }}
                  _focus={{
                    borderColor: validationError ? "var(--alt-error)" : "var(--alt-primary)",
                    boxShadow: `0 0 0 1px ${validationError ? "var(--alt-error)" : "var(--alt-primary)"}`,
                  }}
                />
                {validationError && (
                  <Text color="var(--alt-error)" fontSize="sm" mt={1}>
                    {validationError}
                  </Text>
                )}
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
                disabled={isLoading || !!validationError}
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

        <Text textAlign="center" color="var(--alt-text-secondary)" fontSize="xs" mt={4}>
          Make sure the URL points to a valid RSS or Atom feed
        </Text>
      </VStack>

      <FloatingMenu />
    </Box>
  );
}
