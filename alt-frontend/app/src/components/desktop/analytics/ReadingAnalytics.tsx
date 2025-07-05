'use client';

import React from 'react';
import {
  Box,
  VStack,
  HStack,
  Text,
  SimpleGrid,
  Spinner,
  Flex
} from '@chakra-ui/react';
import { Progress } from '@chakra-ui/progress';
import { ReadingAnalytics as IReadingAnalytics } from '@/types/analytics';

interface ReadingAnalyticsProps {
  analytics: IReadingAnalytics | null;
  isLoading: boolean;
}

export const ReadingAnalytics: React.FC<ReadingAnalyticsProps> = ({
  analytics,
  isLoading
}) => {
  if (isLoading) {
    return (
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Flex justify="center" align="center" h="100px">
          <Spinner color="var(--accent-primary)" />
        </Flex>
      </Box>
    );
  }

  if (!analytics) {
    return (
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Text color="var(--text-secondary)" textAlign="center">
          データがありません
        </Text>
      </Box>
    );
  }

  const { today, week, streak } = analytics;

  return (
    <VStack gap={4} align="stretch">
      {/* 今日の統計 */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Text
          fontSize="sm"
          fontWeight="bold"
          color="var(--text-primary)"
          mb={3}
        >
          📈 Today&apos;s Reading
        </Text>
        
        <SimpleGrid columns={2} gap={3}>
          <VStack gap={1}>
            <Text
              fontSize="xl"
              fontWeight="bold"
              color="var(--accent-primary)"
            >
              {today.articlesRead}
            </Text>
            <Text fontSize="xs" color="var(--text-secondary)">
              Articles
            </Text>
          </VStack>
          
          <VStack gap={1}>
            <Text
              fontSize="xl"
              fontWeight="bold"
              color="var(--accent-primary)"
            >
              {today.timeSpent}m
            </Text>
            <Text fontSize="xs" color="var(--text-secondary)">
              Time
            </Text>
          </VStack>
          
          <VStack gap={1}>
            <Text
              fontSize="xl"
              fontWeight="bold"
              color="var(--accent-primary)"
            >
              {today.favoriteCount}
            </Text>
            <Text fontSize="xs" color="var(--text-secondary)">
              Favorites
            </Text>
          </VStack>
          
          <VStack gap={1}>
            <Box position="relative" w="40px" h="40px">
              <Progress
                value={today.completionRate}
                size="lg"
                colorScheme="purple"
                borderRadius="var(--radius-full)"
                bg="var(--surface-border)"
                sx={{
                  '& > div': {
                    bg: 'var(--accent-primary)'
                  }
                }}
              />
              <Text
                position="absolute"
                top="50%"
                left="50%"
                transform="translate(-50%, -50%)"
                fontSize="xs"
                fontWeight="bold"
                color="var(--text-primary)"
              >
                {today.completionRate}%
              </Text>
            </Box>
            <Text fontSize="xs" color="var(--text-secondary)">
              Completion
            </Text>
          </VStack>
        </SimpleGrid>
      </Box>

      {/* 週間トレンド */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <HStack justify="space-between" mb={3}>
          <Text
            fontSize="sm"
            fontWeight="bold"
            color="var(--text-primary)"
          >
            📊 Weekly Trend
          </Text>
          <HStack gap={1}>
            <Text fontSize="xs" color="var(--text-secondary)">
              {week.trendDirection === 'up' && '📈'}
              {week.trendDirection === 'down' && '📉'}
              {week.trendDirection === 'stable' && '➡️'}
            </Text>
            <Text
              fontSize="xs"
              color={
                week.trendDirection === 'up' 
                  ? 'var(--alt-success)' 
                  : week.trendDirection === 'down'
                  ? 'var(--alt-error)'
                  : 'var(--text-secondary)'
              }
              fontWeight="medium"
            >
              {Math.abs(week.weekOverWeek)}%
            </Text>
          </HStack>
        </HStack>
        
        <VStack gap={2} align="stretch">
          <HStack justify="space-between">
            <Text fontSize="lg" fontWeight="bold" color="var(--text-primary)">
              {week.totalArticles}
            </Text>
            <Text fontSize="xs" color="var(--text-secondary)">
              articles this week
            </Text>
          </HStack>
          
          {/* 簡易チャート */}
          <HStack gap={1} h="30px" align="end">
            {week.dailyBreakdown.map((day) => (
              <Box
                key={day.day}
                flex={1}
                bg="var(--accent-primary)"
                opacity={0.3 + (day.articles / Math.max(...week.dailyBreakdown.map(d => d.articles))) * 0.7}
                borderRadius="var(--radius-xs)"
                h={`${(day.articles / Math.max(...week.dailyBreakdown.map(d => d.articles))) * 100}%`}
                minH="4px"
              />
            ))}
          </HStack>
        </VStack>
      </Box>

      {/* 読書ストリーク */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Text
          fontSize="sm"
          fontWeight="bold"
          color="var(--text-primary)"
          mb={3}
        >
          🔥 Reading Streak
        </Text>
        
        <VStack gap={3}>
          <HStack gap={2}>
            <Text
              fontSize="2xl"
              fontWeight="bold"
              color="var(--accent-primary)"
            >
              {streak.current}
            </Text>
            <Text fontSize="sm" color="var(--text-secondary)">
              days
            </Text>
          </HStack>
          
          <VStack gap={1}>
            <HStack justify="space-between" w="full">
              <Text fontSize="xs" color="var(--text-secondary)">
                Best:
              </Text>
              <Text fontSize="xs" color="var(--text-primary)" fontWeight="medium">
                {streak.longest} days
              </Text>
            </HStack>
            <HStack justify="space-between" w="full">
              <Text fontSize="xs" color="var(--text-secondary)">
                Last read:
              </Text>
              <Text fontSize="xs" color="var(--text-primary)" fontWeight="medium">
                {new Date(streak.lastReadDate).toLocaleDateString()}
              </Text>
            </HStack>
          </VStack>
        </VStack>
      </Box>

      {/* カテゴリ分布 */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Text
          fontSize="sm"
          fontWeight="bold"
          color="var(--text-primary)"
          mb={3}
        >
          🎯 Categories
        </Text>
        
        <VStack gap={2}>
          {today.topCategories.slice(0, 4).map((category) => (
            <HStack key={category.category} justify="space-between" w="full">
              <HStack gap={2}>
                <Box
                  w="8px"
                  h="8px"
                  bg={category.color}
                  borderRadius="var(--radius-full)"
                />
                <Text fontSize="xs" color="var(--text-secondary)">
                  {category.category}
                </Text>
              </HStack>
              <HStack gap={2}>
                <Text fontSize="xs" color="var(--text-primary)" fontWeight="medium">
                  {category.count}
                </Text>
                <Text fontSize="xs" color="var(--text-muted)">
                  {category.percentage}%
                </Text>
              </HStack>
            </HStack>
          ))}
        </VStack>
      </Box>
    </VStack>
  );
};