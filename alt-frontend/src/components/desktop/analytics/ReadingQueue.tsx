"use client";

import { Progress } from "@chakra-ui/progress";
import { Badge, Box, Button, HStack, Text, VStack } from "@chakra-ui/react";
import type React from "react";

export const ReadingQueue: React.FC = () => {
  // Mock data for reading queue
  const queueItems = [
    {
      id: "1",
      title: "Advanced TypeScript Patterns",
      source: "TypeScript Weekly",
      readTime: "10 min read",
      progress: 60,
      priority: "high" as const,
    },
    {
      id: "2",
      title: "Optimizing React Performance",
      source: "React Newsletter",
      readTime: "7 min read",
      progress: 0,
      priority: "medium" as const,
    },
    {
      id: "3",
      title: "Database Design Best Practices",
      source: "Dev Database",
      readTime: "20 min read",
      progress: 25,
      priority: "low" as const,
    },
  ];

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case "high":
        return "var(--alt-error)";
      case "medium":
        return "var(--alt-warning)";
      case "low":
        return "var(--alt-success)";
      default:
        return "var(--text-secondary)";
    }
  };

  const getPriorityLabel = (priority: string) => {
    switch (priority) {
      case "high":
        return "🔴";
      case "medium":
        return "🟡";
      case "low":
        return "🟢";
      default:
        return "⚪";
    }
  };

  return (
    <Box className="glass" p={4} borderRadius="var(--radius-lg)">
      <HStack justify="space-between" mb={3}>
        <Text fontSize="sm" fontWeight="bold" color="var(--text-primary)">
          📚 Reading Queue
        </Text>
        <Badge size="sm" bg="var(--accent-primary)" color="white" fontSize="xs">
          {queueItems.length} items
        </Badge>
      </HStack>

      <VStack gap={3} align="stretch">
        {queueItems.map((item) => (
          <Box
            key={item.id}
            p={3}
            bg="var(--surface-bg)"
            borderRadius="var(--radius-md)"
            border="1px solid var(--surface-border)"
            _hover={{
              bg: "var(--surface-hover)",
              transform: "translateX(2px)",
            }}
            transition="all var(--transition-speed) ease"
            cursor="pointer"
          >
            <VStack gap={2} align="start">
              <HStack justify="space-between" w="full">
                <Text
                  fontSize="xs"
                  color="var(--text-primary)"
                  fontWeight="medium"
                  lineHeight="1.2"
                  overflow="hidden"
                  textOverflow="ellipsis"
                  display="-webkit-box"
                  css={{
                    WebkitLineClamp: 2,
                    WebkitBoxOrient: "vertical",
                  }}
                  flex={1}
                >
                  {item.title}
                </Text>
                <Text fontSize="xs" color={getPriorityColor(item.priority)}>
                  {getPriorityLabel(item.priority)}
                </Text>
              </HStack>

              <HStack justify="space-between" w="full">
                <Text fontSize="xs" color="var(--text-muted)">
                  {item.source}
                </Text>
                <Text fontSize="xs" color="var(--text-muted)">
                  {item.readTime}
                </Text>
              </HStack>

              {item.progress > 0 && (
                <VStack gap={1} w="full">
                  <HStack justify="space-between" w="full">
                    <Text fontSize="xs" color="var(--text-secondary)">
                      Progress:
                    </Text>
                    <Text
                      fontSize="xs"
                      color="var(--text-primary)"
                      fontWeight="medium"
                    >
                      {item.progress}%
                    </Text>
                  </HStack>
                  <Progress
                    value={item.progress}
                    size="sm"
                    w="full"
                    bg="var(--surface-border)"
                    sx={{
                      "& > div": {
                        bg: "var(--accent-primary)",
                      },
                    }}
                    borderRadius="var(--radius-full)"
                  />
                </VStack>
              )}
            </VStack>
          </Box>
        ))}

        <Button
          size="xs"
          variant="ghost"
          color="var(--accent-primary)"
          _hover={{
            bg: "var(--surface-hover)",
          }}
        >
          Manage Queue
        </Button>
      </VStack>
    </Box>
  );
};
