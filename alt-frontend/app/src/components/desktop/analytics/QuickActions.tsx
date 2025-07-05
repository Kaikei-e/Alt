'use client';

import React from 'react';
import {
  Box,
  Stack,
  HStack,
  Text,
  Button,
  SimpleGrid,
  Badge
} from '@chakra-ui/react';
import { useQuickActions } from '@/hooks/useQuickActions';

export const QuickActions: React.FC = () => {
  const { actions, counters } = useQuickActions();

  return (
    <Stack gap={4} align="stretch">
      {/* 統計サマリー */}
      <Box className="glass" p={4} borderRadius="var(--radius-lg)">
        <Text
          fontSize="sm"
          fontWeight="bold"
          color="var(--text-primary)"
          mb={3}
        >
          ⚡ Quick Actions
        </Text>
        
        <SimpleGrid columns={3} gap={3} mb={4}>
          <Stack gap={1}>
            <Text
              fontSize="lg"
              fontWeight="bold"
              color="var(--accent-primary)"
            >
              {counters.unread}
            </Text>
            <Text fontSize="xs" color="var(--text-secondary)">
              Unread
            </Text>
          </Stack>
          
          <Stack gap={1}>
            <Text
              fontSize="lg"
              fontWeight="bold"
              color="var(--accent-secondary)"
            >
              {counters.bookmarks}
            </Text>
            <Text fontSize="xs" color="var(--text-secondary)">
              Bookmarks
            </Text>
          </Stack>
          
          <Stack gap={1}>
            <Text
              fontSize="lg"
              fontWeight="bold"
              color="var(--accent-tertiary)"
            >
              {counters.queue}
            </Text>
            <Text fontSize="xs" color="var(--text-secondary)">
              Queue
            </Text>
          </Stack>
        </SimpleGrid>

        <Stack gap={2}>
          {actions.map((action) => (
            <Button
              key={action.id}
              size="sm"
              variant="outline"
              borderColor="var(--surface-border)"
              color="var(--text-secondary)"
              borderLeftWidth="3px"
              borderLeftColor={action.color || 'var(--accent-primary)'}
              borderRadius="var(--radius-md)"
              justifyContent="flex-start"
              w="full"
              h="auto"
              p={3}
              onClick={action.action}
              _hover={{
                bg: 'var(--surface-hover)',
                borderColor: action.color || 'var(--accent-primary)',
                transform: 'translateX(2px)'
              }}
              transition="all var(--transition-speed) ease"
            >
              <HStack justify="space-between" w="full">
                <HStack gap={2}>
                  <Text fontSize="sm">{action.icon}</Text>
                  <Text fontSize="xs" fontWeight="medium">
                    {action.title}
                  </Text>
                </HStack>
                {action.count !== undefined && (
                  <Badge
                    size="sm"
                    bg="var(--surface-bg)"
                    color="var(--text-primary)"
                    fontSize="xs"
                  >
                    {action.count}
                  </Badge>
                )}
              </HStack>
            </Button>
          ))}
        </Stack>
      </Box>
    </Stack>
  );
};