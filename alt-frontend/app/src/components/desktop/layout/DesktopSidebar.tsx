'use client';

import React from 'react';
import { 
  Box, 
  VStack, 
  Heading, 
  Button, 
  Text,
  Flex,
  IconButton
} from '@chakra-ui/react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { DesktopSidebarProps } from '@/types/desktop-feeds';

export const DesktopSidebar: React.FC<DesktopSidebarProps> = ({
  activeFilters,
  onFilterChange,
  feedSources,
  isCollapsed,
  onToggleCollapse
}) => {
  return (
    <Box className="glass" p={6} borderRadius="var(--radius-xl)">
      {/* ヘッダー */}
      <Flex justify="space-between" align="center" mb={6}>
        <Heading 
          size="md" 
          color="var(--text-primary)"
          fontFamily="var(--font-display)"
        >
          Filters
        </Heading>
        <IconButton
          aria-label="Collapse sidebar"
          size="sm"
          variant="ghost"
          color="var(--text-secondary)"
          onClick={onToggleCollapse}
        >
          {isCollapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
        </IconButton>
      </Flex>

      {!isCollapsed && (
        <VStack gap={6} align="stretch">
          {/* 読書状態フィルター */}
          <Box>
            <Text 
              fontSize="sm" 
              fontWeight="semibold" 
              color="var(--text-primary)"
              mb={3}
            >
              Read Status
            </Text>
            <VStack gap={2} align="stretch">
              {['all', 'unread', 'read'].map(status => (
                <Button
                  key={status}
                  size="sm"
                  variant={activeFilters.readStatus === status ? "solid" : "outline"}
                  colorScheme={activeFilters.readStatus === status ? "purple" : "gray"}
                  onClick={() => onFilterChange({ 
                    ...activeFilters, 
                    readStatus: status as 'all' | 'unread' | 'read' 
                  })}
                >
                  <Text 
                    fontSize="sm" 
                    textTransform="capitalize"
                  >
                    {status}
                  </Text>
                </Button>
              ))}
            </VStack>
          </Box>

          {/* 時間範囲フィルター */}
          <Box>
            <Text 
              fontSize="sm" 
              fontWeight="semibold" 
              color="var(--text-primary)"
              mb={3}
            >
              Time Range
            </Text>
            <VStack gap={2} align="stretch">
              {[
                { value: 'today', label: 'Today' },
                { value: 'week', label: 'This Week' },
                { value: 'month', label: 'This Month' },
                { value: 'all', label: 'All Time' }
              ].map(option => (
                <Button
                  key={option.value}
                  size="sm"
                  variant={activeFilters.timeRange === option.value ? "solid" : "outline"}
                  colorScheme={activeFilters.timeRange === option.value ? "purple" : "gray"}
                  onClick={() => onFilterChange({ 
                    ...activeFilters, 
                    timeRange: option.value as 'today' | 'week' | 'month' | 'all' 
                  })}
                >
                  <Text fontSize="sm">
                    {option.label}
                  </Text>
                </Button>
              ))}
            </VStack>
          </Box>

          {/* ソースフィルター */}
          <Box>
            <Text 
              fontSize="sm" 
              fontWeight="semibold" 
              color="var(--text-primary)"
              mb={3}
            >
              Sources
            </Text>
            <VStack gap={2} align="stretch">
              {feedSources.map(source => (
                <Button
                  key={source.id}
                  size="sm"
                  variant={activeFilters.sources.includes(source.id) ? "solid" : "outline"}
                  colorScheme={activeFilters.sources.includes(source.id) ? "purple" : "gray"}
                  onClick={() => {
                    const newSources = activeFilters.sources.includes(source.id)
                      ? activeFilters.sources.filter(id => id !== source.id)
                      : [...activeFilters.sources, source.id];
                    onFilterChange({ ...activeFilters, sources: newSources });
                  }}
                >
                  <Flex align="center" gap={2} w="100%">
                    <Text fontSize="lg">{source.icon}</Text>
                    <Text fontSize="sm" flex={1}>
                      {source.name}
                    </Text>
                    <Text 
                      fontSize="xs" 
                      color="var(--text-muted)"
                    >
                      ({source.unreadCount})
                    </Text>
                  </Flex>
                </Button>
              ))}
            </VStack>
          </Box>

          {/* クリアボタン */}
          <Button
            size="sm"
            variant="outline"
            borderColor="var(--surface-border)"
            color="var(--text-secondary)"
            _hover={{
              bg: "var(--surface-hover)",
              borderColor: "var(--accent-primary)"
            }}
            onClick={() => onFilterChange({
              sources: [],
              timeRange: 'all',
              readStatus: 'all',
              tags: [],
              priority: 'all'
            })}
          >
            Clear Filters
          </Button>
        </VStack>
      )}
    </Box>
  );
};