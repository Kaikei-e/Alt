"use client";

import {
  Box,
  Container,
  Heading,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { articleApi } from "@/lib/api";
import type { Article } from "@/schema/article";

export default function ArticleDetailPage() {
  const params = useParams();
  const id = params?.id as string;
  const [article, setArticle] = useState<Article | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;

    const fetchArticle = async () => {
      try {
        setLoading(true);
        const data = await articleApi.getArticleById(id);
        setArticle(data);
      } catch (err) {
        console.error("Failed to fetch article:", err);
        setError("Article not found or error occurred.");
      } finally {
        setLoading(false);
      }
    };

    fetchArticle();
  }, [id]);

  if (loading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        h="50vh"
        data-testid="article-loading"
      >
        <Spinner size="xl" data-testid="article-spinner" />
      </Box>
    );
  }

  if (error || !article) {
    return (
      <Box p={8} textAlign="center" data-testid="article-error">
        <Text color="red.500" fontSize="lg" data-testid="article-error-message">
          {error || "Article not found"}
        </Text>
      </Box>
    );
  }

  return (
    <Container maxW="container.lg" py={8} data-testid="article-container">
      <VStack align="stretch" gap={6}>
        <Heading as="h1" size="2xl" data-testid="article-title">
          {article.title}
        </Heading>
        {article.published_at && (
          <Text color="gray.500" data-testid="article-published">
            Published: {new Date(article.published_at).toLocaleDateString()}
          </Text>
        )}
        <Box
          className="article-content"
          data-testid="article-content"
          dangerouslySetInnerHTML={{ __html: article.content }}
        />
      </VStack>
    </Container>
  );
}
