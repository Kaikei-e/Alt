"use client";

import { Box, Button, Flex, Text } from "@chakra-ui/react";
import { memo, useMemo } from "react";
import type { Article } from "@/schema/article";

interface ArticleViewCardProps {
  article: Article;
  onDetailsClick: () => void;
}

export const ArticleViewCard = memo(function ArticleViewCard({
  article,
  onDetailsClick,
}: ArticleViewCardProps) {
  const publishedLabel = useMemo(() => {
    if (!article.published_at) {
      return null;
    }
    try {
      return new Date(article.published_at).toLocaleString();
    } catch {
      return article.published_at;
    }
  }, [article.published_at]);

  // Truncate content for preview (first 150 characters)
  const contentPreview = useMemo(() => {
    if (!article.content) return "";
    const plainText = article.content.replace(/<[^>]*>/g, "");
    return plainText.length > 150 ? plainText.substring(0, 150) + "..." : plainText;
  }, [article.content]);

  return (
    <Box
      bg="var(--alt-glass)"
      backdropFilter="blur(20px)"
      border="2px solid var(--alt-glass-border)"
      borderRadius="18px"
      p={6}
      mb={4}
      boxShadow="0 4px 12px rgba(0, 0, 0, 0.15), 0 0 0 1px rgba(255, 255, 255, 0.05) inset"
      transition="all 0.2s ease"
      _hover={{
        transform: "translateY(-2px)",
        boxShadow: "0 8px 24px rgba(0, 0, 0, 0.25), 0 0 0 1px rgba(255, 255, 255, 0.1) inset",
        borderColor: "var(--alt-primary)",
      }}
      data-testid={`article-card-${article.id}`}
    >
      {/* Title */}
      <Text
        fontSize="lg"
        fontWeight="bold"
        color="var(--alt-text-primary)"
        mb={2}
        lineHeight="1.4"
      >
        {article.title}
      </Text>

      {/* Published Date */}
      {publishedLabel && (
        <Text
          fontSize="xs"
          color="var(--alt-text-secondary)"
          mb={3}
          textTransform="uppercase"
          letterSpacing="0.05em"
        >
          {publishedLabel}
        </Text>
      )}

      {/* Content Preview */}
      {contentPreview && (
        <Text fontSize="sm" color="var(--alt-text-primary)" lineHeight="1.6" mb={4} opacity={0.9}>
          {contentPreview}
        </Text>
      )}

      {/* Action Area */}
      <Flex justify="space-between" align="center" mt={4}>
        {/* Left side - empty as specified */}
        <Box />

        {/* Right side - Details button */}
        <Button
          onClick={onDetailsClick}
          size="sm"
          bg="var(--alt-primary)"
          color="white"
          fontWeight="bold"
          borderRadius="12px"
          px={6}
          _hover={{
            bg: "var(--alt-secondary)",
            transform: "translateY(-1px)",
          }}
          _active={{
            transform: "translateY(0)",
          }}
          transition="all 0.2s ease"
          data-testid={`details-button-${article.id}`}
        >
          Details
        </Button>
      </Flex>
    </Box>
  );
});
