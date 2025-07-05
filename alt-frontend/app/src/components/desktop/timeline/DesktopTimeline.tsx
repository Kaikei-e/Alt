'use client';

import React from 'react';
import { VStack, Text, Spinner, Flex } from '@chakra-ui/react';
import { FilterState } from '@/types/desktop-feeds';

interface DesktopTimelineProps {
  searchQuery: string;
  filters: FilterState;
}

export const DesktopTimeline: React.FC<DesktopTimelineProps> = ({
  searchQuery
}) => {
  // TASK2で詳細実装（filtersは将来使用予定）
  return (
    <VStack gap={6} align="stretch">
      {searchQuery && (
        <Flex 
          className="glass" 
          p={4} 
          borderRadius="var(--radius-lg)"
          align="center"
          gap={2}
        >
          <Text color="var(--text-primary)" fontWeight="medium">
            検索: &quot;{searchQuery}&quot;
          </Text>
        </Flex>
      )}
      
      {/* プレースホルダー - TASK2で実装 */}
      <Flex 
        className="glass" 
        p={8} 
        borderRadius="var(--radius-xl)"
        direction="column"
        align="center"
        gap={4}
      >
        <Spinner 
          size="lg" 
          color="var(--accent-primary)"
        />
        <Text color="var(--text-secondary)">
          フィードカードはTASK2で実装されます
        </Text>
      </Flex>
    </VStack>
  );
};