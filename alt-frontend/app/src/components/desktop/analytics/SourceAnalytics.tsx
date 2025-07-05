'use client';

import React, { useState } from 'react';
import {
  Box,
  Stack,
  HStack,
  Text,
  Progress,
  Spinner,
  Flex,
  Button,
  Select,
  createListCollection
} from '@chakra-ui/react';
import { SourceAnalytic } from '@/types/analytics';

interface SourceAnalyticsProps {
  sources: SourceAnalytic[];
  isLoading: boolean;
}

export const SourceAnalytics: React.FC<SourceAnalyticsProps> = ({
  sources,
  isLoading
}) => {
  const [sortBy, setSortBy] = useState<'articles' | 'reliability' | 'engagement'>('articles');
  const [showAll, setShowAll] = useState(false);

  const sortOptions = createListCollection({
    items: [
      { label: 'Articles', value: 'articles' },
      { label: 'Reliability', value: 'reliability' },
      { label: 'Engagement', value: 'engagement' }
    ]
  });

  if (isLoading) {
    return (
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Text fontSize="sm" fontWeight="bold" color="var(--text-primary)" mb={3}>
          📰 Source Analytics
        </Text>
        <Flex justify="center" align="center" h="80px">
          <Spinner color="var(--accent-primary)" size="sm" />
        </Flex>
      </Box>
    );
  }

  const sortedSources = [...sources].sort((a, b) => {
    switch (sortBy) {
      case 'articles':
        return b.totalArticles - a.totalArticles;
      case 'reliability':
        return b.reliability - a.reliability;
      case 'engagement':
        return b.engagement - a.engagement;
      default:
        return 0;
    }
  });

  const displaySources = showAll ? sortedSources : sortedSources.slice(0, 4);

  return (
    <Box className="glass" p={4} borderRadius="var(--radius-lg)">
      <HStack justify="space-between" mb={3}>
        <Text
          fontSize="sm"
          fontWeight="bold"
          color="var(--text-primary)"
        >
          📰 Source Analytics
        </Text>
        <Select.Root
          collection={sortOptions}
          value={[sortBy]}
          onValueChange={(details) => setSortBy(details.value[0] as 'articles' | 'reliability' | 'engagement')}
          size="xs"
          w="auto"
        >
          <Select.Trigger
            bg="var(--surface-bg)"
            borderColor="var(--surface-border)"
            color="var(--text-primary)"
            fontSize="xs"
          >
            <Select.ValueText placeholder="Sort by" />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Positioner>
            <Select.Content>
              {sortOptions.items.map((option) => (
                <Select.Item key={option.value} item={option}>
                  <Select.ItemText>{option.label}</Select.ItemText>
                  <Select.ItemIndicator />
                </Select.Item>
              ))}
            </Select.Content>
          </Select.Positioner>
        </Select.Root>
      </HStack>

      <Stack gap={3} align="stretch">
        {displaySources.map((source, index) => (
          <Box
            key={source.id}
            p={3}
            bg="var(--surface-bg)"
            borderRadius="var(--radius-md)"
            border="1px solid var(--surface-border)"
          >
            <HStack justify="space-between" mb={2}>
              <HStack gap={2}>
                <Text fontSize="xs" color="var(--text-muted)" fontWeight="bold">
                  #{index + 1}
                </Text>
                <Text fontSize="sm">{source.icon}</Text>
                <Stack gap={0} align="start">
                  <Text
                    fontSize="xs"
                    color="var(--text-primary)"
                    fontWeight="medium"
                  >
                    {source.name}
                  </Text>
                  <Text fontSize="xs" color="var(--text-muted)">
                    {source.category}
                  </Text>
                </Stack>
              </HStack>
            </HStack>

            <Stack gap={2} align="stretch">
              <HStack justify="space-between">
                <Text fontSize="xs" color="var(--text-secondary)">
                  Articles:
                </Text>
                <Text fontSize="xs" color="var(--text-primary)" fontWeight="medium">
                  {source.totalArticles}
                </Text>
              </HStack>
              
              <HStack justify="space-between">
                <Text fontSize="xs" color="var(--text-secondary)">
                  Reliability:
                </Text>
                <HStack gap={1}>
                  <Progress.Root
                    value={(source.reliability / 10) * 100}
                    size="sm"
                    w="40px"
                    bg="var(--surface-border)"
                    borderRadius="var(--radius-full)"
                  >
                    <Progress.Track>
                      <Progress.Range bg="var(--alt-success)" />
                    </Progress.Track>
                  </Progress.Root>
                  <Text fontSize="xs" color="var(--text-primary)" fontWeight="medium">
                    {source.reliability}/10
                  </Text>
                </HStack>
              </HStack>
              
              <HStack justify="space-between">
                <Text fontSize="xs" color="var(--text-secondary)">
                  Engagement:
                </Text>
                <HStack gap={1}>
                  <Progress.Root
                    value={source.engagement}
                    size="sm"
                    w="40px"
                    bg="var(--surface-border)"
                    borderRadius="var(--radius-full)"
                  >
                    <Progress.Track>
                      <Progress.Range bg="var(--accent-primary)" />
                    </Progress.Track>
                  </Progress.Root>
                  <Text fontSize="xs" color="var(--text-primary)" fontWeight="medium">
                    {source.engagement}%
                  </Text>
                </HStack>
              </HStack>
            </Stack>
          </Box>
        ))}

        {sources.length > 4 && (
          <Button
            size="xs"
            variant="ghost"
            color="var(--accent-primary)"
            onClick={() => setShowAll(!showAll)}
            _hover={{
              bg: 'var(--surface-hover)'
            }}
          >
            {showAll ? 'Show Less' : `Show All (${sources.length})`}
          </Button>
        )}
      </Stack>
    </Box>
  );
};