'use client';

import React from 'react';
import {
  Box,
  Stack,
  HStack,
  Text,
  Button
} from '@chakra-ui/react';

export const BookmarksList: React.FC = () => {
  // Mock data for bookmarks
  const bookmarks = [
    {
      id: '1',
      title: 'Understanding React Server Components',
      source: 'React Blog',
      readTime: '8 min read',
      addedDate: '2024-01-15'
    },
    {
      id: '2',
      title: 'Modern CSS Techniques for 2024',
      source: 'CSS Tricks',
      readTime: '12 min read',
      addedDate: '2024-01-14'
    },
    {
      id: '3',
      title: 'Building Scalable APIs with Node.js',
      source: 'Node.js Weekly',
      readTime: '15 min read',
      addedDate: '2024-01-13'
    }
  ];

  return (
    <Box className="glass" p={4} borderRadius="var(--radius-lg)">
      <Text
        fontSize="sm"
        fontWeight="bold"
        color="var(--text-primary)"
        mb={3}
      >
        🔖 Recent Bookmarks
      </Text>
      
      <Stack gap={3} align="stretch">
        {bookmarks.map((bookmark) => (
          <Box
            key={bookmark.id}
            p={3}
            bg="var(--surface-bg)"
            borderRadius="var(--radius-md)"
            border="1px solid var(--surface-border)"
            _hover={{
              bg: 'var(--surface-hover)',
              transform: 'translateX(2px)'
            }}
            transition="all var(--transition-speed) ease"
            cursor="pointer"
          >
            <Stack gap={2} align="start">
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
                  WebkitBoxOrient: 'vertical'
                }}
              >
                {bookmark.title}
              </Text>
              
              <HStack justify="space-between" w="full">
                <Text fontSize="xs" color="var(--text-muted)">
                  {bookmark.source}
                </Text>
                <Text fontSize="xs" color="var(--text-muted)">
                  {bookmark.readTime}
                </Text>
              </HStack>
              
              <Text fontSize="xs" color="var(--text-secondary)">
                Added {new Date(bookmark.addedDate).toLocaleDateString()}
              </Text>
            </Stack>
          </Box>
        ))}

        <Button
          size="xs"
          variant="ghost"
          color="var(--accent-primary)"
          _hover={{
            bg: 'var(--surface-hover)'
          }}
        >
          View All Bookmarks
        </Button>
      </Stack>
    </Box>
  );
};