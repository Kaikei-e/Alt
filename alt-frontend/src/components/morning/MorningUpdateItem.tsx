"use client";

import { Box, Text, Button, Flex } from "@chakra-ui/react";
import { ChevronDown } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import type { MorningUpdate } from "@/schema/morning";

type MorningUpdateItemProps = {
  update: MorningUpdate;
};

export const MorningUpdateItem = ({ update }: MorningUpdateItemProps) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const hasDuplicates = update.duplicates.length > 0;

  if (!update.primary_article) {
    return null;
  }

  return (
    <Box
      borderWidth="1px"
      borderRadius="md"
      p={4}
      mb={3}
      bg="var(--card-bg)"
      data-testid="morning-update-item"
    >
      {/* Primary Article */}
      <Box mb={hasDuplicates ? 3 : 0}>
        <Link
          href={update.primary_article.url}
          target="_blank"
          rel="noopener noreferrer"
        >
          <Text
            color="var(--accent-primary)"
            fontWeight="medium"
            fontSize="sm"
            _hover={{ textDecoration: "underline" }}
          >
            {update.primary_article.title}
          </Text>
        </Link>
        {update.primary_article.published_at && (
          <Text fontSize="xs" color="var(--text-secondary)" mt={1}>
            {new Date(update.primary_article.published_at).toLocaleDateString(
              "en-US",
              {
                month: "short",
                day: "numeric",
                hour: "2-digit",
                minute: "2-digit",
              },
            )}
          </Text>
        )}
      </Box>

      {/* Duplicates Toggle */}
      {hasDuplicates && (
        <Box>
          <Button
            size="xs"
            variant="ghost"
            onClick={() => setIsExpanded(!isExpanded)}
            color="var(--text-secondary)"
            _hover={{ bg: "var(--hover-bg)" }}
          >
            <Flex align="center" gap={1}>
              <Text>
                and {update.duplicates.length} other
                {update.duplicates.length > 1 ? "s" : ""}
              </Text>
              <ChevronDown
                size={16}
                style={{
                  transform: isExpanded ? "rotate(180deg)" : "rotate(0deg)",
                  transition: "transform 0.2s",
                }}
              />
            </Flex>
          </Button>

          {isExpanded && (
            <Box
              mt={2}
              pl={4}
              borderLeftWidth="2px"
              borderColor="var(--border-color)"
            >
              {update.duplicates.map((duplicate) => (
                <Box key={duplicate.id} mb={2}>
                  <Link
                    href={duplicate.url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Text
                      color="var(--text-primary)"
                      fontSize="xs"
                      _hover={{ textDecoration: "underline" }}
                    >
                      {duplicate.title}
                    </Text>
                  </Link>
                  {duplicate.published_at && (
                    <Text fontSize="xs" color="var(--text-secondary)" mt={0.5}>
                      {new Date(duplicate.published_at).toLocaleDateString(
                        "en-US",
                        {
                          month: "short",
                          day: "numeric",
                        },
                      )}
                    </Text>
                  )}
                </Box>
              ))}
            </Box>
          )}
        </Box>
      )}
    </Box>
  );
};
