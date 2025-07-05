'use client';

import React from 'react';
import {
  Box,
  Stack,
  Tabs
} from '@chakra-ui/react';
import { ReadingAnalytics } from './ReadingAnalytics';
import { TrendingTopics } from './TrendingTopics';
import { QuickActions } from './QuickActions';
import { SourceAnalytics } from './SourceAnalytics';
import { BookmarksList } from './BookmarksList';
import { ReadingQueue } from './ReadingQueue';
import { useReadingAnalytics } from '@/hooks/useReadingAnalytics';
import { useTrendingTopics } from '@/hooks/useTrendingTopics';
import { useSourceAnalytics } from '@/hooks/useSourceAnalytics';

export const RightPanel: React.FC = () => {
  const { analytics, isLoading: analyticsLoading } = useReadingAnalytics();
  const { topics, isLoading: topicsLoading } = useTrendingTopics();
  const { sources, isLoading: sourcesLoading } = useSourceAnalytics();

  return (
    <Box className="glass" borderRadius="var(--radius-xl)" overflow="hidden">
      <Tabs.Root defaultValue="analytics" size="sm">
        <Tabs.List
          bg="var(--surface-bg)"
          borderBottom="1px solid var(--surface-border)"
        >
          <Tabs.Trigger
            value="analytics"
            flex={1}
            color="var(--text-secondary)"
            fontSize="sm"
            fontWeight="medium"
            _selected={{
              color: 'var(--text-primary)',
              bg: 'var(--accent-primary)',
              borderColor: 'var(--accent-primary)'
            }}
          >
            📊 Analytics
          </Tabs.Trigger>
          <Tabs.Trigger
            value="actions"
            flex={1}
            color="var(--text-secondary)"
            fontSize="sm"
            fontWeight="medium"
            _selected={{
              color: 'var(--text-primary)',
              bg: 'var(--accent-primary)',
              borderColor: 'var(--accent-primary)'
            }}
          >
            ⚡ Actions
          </Tabs.Trigger>
        </Tabs.List>

        <Tabs.ContentGroup>
          {/* Analytics Tab */}
          <Tabs.Content value="analytics" p={0}>
            <Stack gap={4} p={4} align="stretch">
              <ReadingAnalytics 
                analytics={analytics} 
                isLoading={analyticsLoading} 
              />
              
              <TrendingTopics 
                topics={topics} 
                isLoading={topicsLoading} 
              />
              
              <SourceAnalytics 
                sources={sources} 
                isLoading={sourcesLoading} 
              />
            </Stack>
          </Tabs.Content>

          {/* Actions Tab */}
          <Tabs.Content value="actions" p={0}>
            <Stack gap={4} p={4} align="stretch">
              <QuickActions />
              <BookmarksList />
              <ReadingQueue />
            </Stack>
          </Tabs.Content>
        </Tabs.ContentGroup>
      </Tabs.Root>
    </Box>
  );
};