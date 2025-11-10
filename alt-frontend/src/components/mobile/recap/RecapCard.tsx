"use client";

import { Box, Button, Flex, Text, VStack } from "@chakra-ui/react";
import { ChevronDown, ChevronUp, Link as LinkIcon } from "lucide-react";
import Link from "next/link";
import { useCallback, useState } from "react";
import type { RecapGenre } from "@/schema/recap";

type RecapCardProps = {
  genre: RecapGenre;
};

const RecapCard = ({ genre }: RecapCardProps) => {
  const [isExpanded, setIsExpanded] = useState(false);

  const handleToggle = useCallback(() => {
    setIsExpanded((prev) => !prev);
  }, []);

  return (
    <Box
      p="2px"
      borderRadius="18px"
      border="2px solid var(--surface-border)"
      transition="transform 0.3s ease-in-out, box-shadow 0.3s ease-in-out"
      data-testid={`recap-card-${genre.genre}`}
    >
      <Box
        className="glass"
        w="full"
        p={4}
        borderRadius="16px"
        bg="var(--surface-bg)"
        data-testid="recap-card-container"
      >
        <VStack align="stretch" gap={3}>
          {/* ヘッダー: ジャンル名・メトリクス */}
          <Flex justify="space-between" align="center">
            <Text
              fontSize="lg"
              fontWeight="bold"
              color="var(--accent-primary)"
              textTransform="uppercase"
              letterSpacing="0.08em"
            >
              {genre.genre}
            </Text>
            <Flex gap={3} fontSize="xs" color="var(--text-secondary)">
              <Text>Clusters: {genre.clusterCount}</Text>
              <Text>Articles: {genre.articleCount}</Text>
            </Flex>
          </Flex>

          {/* トピックChips */}
          {genre.topTerms.length > 0 && (
            <Flex gap={2} wrap="wrap">
              {genre.topTerms.slice(0, 5).map((term, idx) => (
                <Box
                  key={idx}
                  px={3}
                  py={1}
                  bg="rgba(255, 255, 255, 0.1)"
                  borderRadius="full"
                  fontSize="xs"
                  color="var(--text-primary)"
                  border="1px solid var(--surface-border)"
                >
                  {term}
                </Box>
              ))}
            </Flex>
          )}

          {/* 要約プレビュー。改行された文章のそれぞれ先頭100文字を表示する（2行） */}
          <Text
            fontSize="sm"
            color="var(--text-primary)"
            lineHeight="1.6"
            {...(!isExpanded && { noOfLines: 2 })}
          >
            {genre.summary.split("\n").map((line) => line.slice(0, 100)).join("\n")}
          </Text>

          {/* 展開ボタン */}
          <Button
            size="sm"
            onClick={handleToggle}
            borderRadius="full"
            bg="var(--alt-primary)"
            color="var(--text-primary)"
            fontWeight="bold"
            _hover={{
              transform: "scale(1.02)",
            }}
            _active={{
              transform: "scale(0.98)",
            }}
            transition="all 0.2s ease"
          >
            <Flex align="center" gap={2}>
              {isExpanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
              <Text>{isExpanded ? "Collapse" : "View details"}</Text>
            </Flex>
          </Button>

          {/* 展開時: 全文＋Evidence */}
          {isExpanded && (
            <VStack align="stretch" gap={3} pt={2}>
              {/* 全文表示 */}
              <Box
                p={3}
                bg="rgba(255, 255, 255, 0.03)"
                borderRadius="12px"
                border="1px solid var(--surface-border)"
              >
                <Text
                  fontSize="xs"
                  color="var(--text-secondary)"
                  fontWeight="bold"
                  mb={2}
                  textTransform="uppercase"
                  letterSpacing="1px"
                >
                  Full Summary
                </Text>
                <Text fontSize="sm" color="var(--text-primary)" lineHeight="1.7" whiteSpace="pre-wrap">
                  {genre.summary}
                </Text>
              </Box>

              {/* Evidence Links */}
              {genre.evidenceLinks.length > 0 && (
                <Box>
                  <Text
                    fontSize="xs"
                    color="var(--text-secondary)"
                    fontWeight="bold"
                    mb={2}
                    textTransform="uppercase"
                    letterSpacing="1px"
                  >
                    Evidence ({genre.evidenceLinks.length} articles)
                  </Text>
                  <VStack align="stretch" gap={2}>
                    {genre.evidenceLinks.map((evidence) => (
                      <Link
                        key={evidence.articleId}
                        href={evidence.sourceUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        <Flex
                          p={2}
                          bg="rgba(255, 255, 255, 0.05)"
                          borderRadius="8px"
                          border="1px solid var(--surface-border)"
                          align="center"
                          gap={2}
                          _hover={{
                            bg: "rgba(255, 255, 255, 0.1)",
                            borderColor: "var(--alt-primary)",
                          }}
                          transition="all 0.2s ease"
                        >
                          <LinkIcon size={14} color="var(--alt-primary)" />
                          <Text
                            fontSize="xs"
                            color="var(--text-primary)"
                            flex={1}
                            overflow="hidden"
                            textOverflow="ellipsis"
                            whiteSpace="nowrap"
                          >
                            {evidence.title}
                          </Text>
                        </Flex>
                      </Link>
                    ))}
                  </VStack>
                </Box>
              )}
            </VStack>
          )}
        </VStack>
      </Box>
    </Box>
  );
};

export default RecapCard;

