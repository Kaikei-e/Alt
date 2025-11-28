"use client";

import { Box, Heading, Text, Spinner, VStack, Container } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
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
      <Box display="flex" justifyContent="center" alignItems="center" h="50vh">
        <Spinner size="xl" />
      </Box>
    );
  }

  if (error || !article) {
    return (
      <Box p={8} textAlign="center">
        <Text color="red.500" fontSize="lg">{error || "Article not found"}</Text>
      </Box>
    );
  }

  return (
    <Container maxW="container.lg" py={8}>
      <VStack align="stretch" gap={6}>
        <Heading as="h1" size="2xl">{article.title}</Heading>
        {article.published_at && (
          <Text color="gray.500">
            Published: {new Date(article.published_at).toLocaleDateString()}
          </Text>
        )}
        <Box
          className="article-content"
          dangerouslySetInnerHTML={{ __html: article.content }}
        />
        {/* Author is not in Article interface, so omitting */}
      </VStack>
    </Container>
  );
}
