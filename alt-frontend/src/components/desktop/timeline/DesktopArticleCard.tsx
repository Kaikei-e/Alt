"use client";

import { Badge, Box, HStack, Text, VStack } from "@chakra-ui/react";
import { ExternalLink } from "lucide-react";
import Link from "next/link";
import type React from "react";
import type { Article } from "@/schema/article";

interface DesktopArticleCardProps {
  article: Article;
}

const formatTimeAgo = (dateString: string) => {
  const now = new Date();
  const date = new Date(dateString);
  const diffInSeconds = Math.floor((now.getTime() - date.getTime()) / 1000);

  if (diffInSeconds < 60) return "たった今";
  if (diffInSeconds < 3600) return `${Math.floor(diffInSeconds / 60)}分前`;
  if (diffInSeconds < 86400) return `${Math.floor(diffInSeconds / 3600)}時間前`;
  return `${Math.floor(diffInSeconds / 86400)}日前`;
};

export const DesktopArticleCard: React.FC<DesktopArticleCardProps> = ({ article }) => {
  return (
    <Box
      className="glass"
      p={5}
      borderRadius="var(--radius-lg)"
      border="1px solid var(--surface-border)"
      cursor="pointer"
      transition="all var(--transition-smooth) ease"
      _hover={{
        transform: "translateY(-2px)",
        boxShadow: "0 8px 20px rgba(0, 0, 0, 0.1)",
        borderColor: "var(--accent-primary)",
      }}
      data-testid={`article-card-${article.id}`}
    >
      <VStack gap={3} align="stretch">
        {/* Header with title and link */}
        <HStack gap={2} align="flex-start">
          <VStack gap={1} align="stretch" flex={1}>
            <Text fontSize="lg" fontWeight="semibold" color="var(--text-primary)" lineHeight="1.4">
              {article.title}
            </Text>
            <Text fontSize="xs" color="var(--text-secondary)">
              {formatTimeAgo(article.published_at)}
            </Text>
          </VStack>

          <Link href={article.url} target="_blank" rel="noopener noreferrer">
            <Box
              p={2}
              borderRadius="var(--radius-md)"
              _hover={{ bg: "var(--surface-hover)" }}
              transition="background var(--transition-speed) ease"
            >
              <ExternalLink size={18} color="var(--accent-primary)" />
            </Box>
          </Link>
        </HStack>

        {/* Content preview */}
        <Text
          fontSize="sm"
          color="var(--text-secondary)"
          lineHeight="1.6"
          css={{
            display: "-webkit-box",
            WebkitLineClamp: 3,
            WebkitBoxOrient: "vertical",
            overflow: "hidden",
          }}
        >
          {article.content}
        </Text>

        {/* Tags */}
        {article.tags && article.tags.length > 0 && (
          <HStack gap={2} flexWrap="wrap">
            {article.tags.slice(0, 5).map((tag, index) => (
              <Badge
                key={index}
                bg="var(--accent-tertiary)"
                color="white"
                fontSize="xs"
                px={2}
                py={1}
                borderRadius="md"
              >
                {tag}
              </Badge>
            ))}
            {article.tags.length > 5 && (
              <Text fontSize="xs" color="var(--text-muted)">
                +{article.tags.length - 5}
              </Text>
            )}
          </HStack>
        )}
      </VStack>
    </Box>
  );
};
