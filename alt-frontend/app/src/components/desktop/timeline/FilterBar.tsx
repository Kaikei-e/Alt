'use client';

import React from 'react';
import { Box, Text, Flex, Button, HStack, VStack } from '@chakra-ui/react';
import { FilterState } from '@/types/desktop-feed';

interface FilterBarProps {
  filters: FilterState;
  onFilterChange: (filters: FilterState) => void;
  availableTags?: string[];
  availableSources?: Array<{ id: string; name: string; icon: string }>;
}

export const FilterBar: React.FC<FilterBarProps> = ({
  filters,
  onFilterChange,
  availableTags = [],
  availableSources = [],
}) => {
  const handleReadStatusChange = () => {
    const nextValue = filters.readStatus === 'all' ? 'unread' : 'all';
    onFilterChange({
      ...filters,
      readStatus: nextValue,
    });
  };

  const handlePriorityChange = () => {
    const nextValue = filters.priority === 'all' ? 'high' : 'all';
    onFilterChange({
      ...filters,
      priority: nextValue,
    });
  };

  const handleTimeRangeChange = () => {
    const nextValue = filters.timeRange === 'all' ? 'today' : 'all';
    onFilterChange({
      ...filters,
      timeRange: nextValue,
    });
  };

  const handleSourceToggle = (sourceId: string) => {
    const currentSources = filters.sources;
    const isSelected = currentSources.includes(sourceId);
    const newSources = isSelected
      ? currentSources.filter(id => id !== sourceId)
      : [...currentSources, sourceId];

    onFilterChange({
      ...filters,
      sources: newSources,
    });
  };

  const handleTagToggle = (tag: string) => {
    const currentTags = filters.tags;
    const isSelected = currentTags.includes(tag);
    const newTags = isSelected
      ? currentTags.filter(t => t !== tag)
      : [...currentTags, tag];

    onFilterChange({
      ...filters,
      tags: newTags,
    });
  };

  const clearAllFilters = () => {
    onFilterChange({
      readStatus: 'all',
      sources: [],
      priority: 'all',
      tags: [],
      timeRange: 'all',
    });
  };

  const hasActiveFilters =
    filters.readStatus !== 'all' ||
    filters.sources.length > 0 ||
    filters.priority !== 'all' ||
    filters.tags.length > 0 ||
    filters.timeRange !== 'all';

  return (
    <Box
      data-testid="filter-bar"
      className="glass"
      p={4}
      borderRadius="var(--radius-lg)"
      mb={4}
    >
      <VStack align="stretch" gap={4}>
        {/* Filter Status Header */}
        <Flex justify="space-between" align="center">
          <Text fontSize="sm" color="var(--text-primary)" fontWeight="medium">
            Filters Active: {hasActiveFilters ? 'Yes' : 'None'}
          </Text>
          {hasActiveFilters && (
            <Button
              size="sm"
              variant="ghost"
              color="var(--text-muted)"
              onClick={clearAllFilters}
              _hover={{ color: 'var(--alt-primary)' }}
            >
              Clear All
            </Button>
          )}
        </Flex>

        {/* Main Filter Controls */}
        <HStack gap={4} wrap="wrap">
          {/* Read Status Filter */}
          <Box>
            <Text fontSize="xs" color="var(--text-muted)" mb={2}>
              Read Status
            </Text>
            <Button
              data-testid="read-status-filter"
              size="sm"
              variant={filters.readStatus !== 'all' ? 'solid' : 'outline'}
              colorScheme={filters.readStatus !== 'all' ? 'blue' : 'gray'}
              onClick={handleReadStatusChange}
              bg={filters.readStatus !== 'all' ? 'var(--alt-primary)' : 'transparent'}
              color={filters.readStatus !== 'all' ? 'white' : 'var(--text-secondary)'}
              borderColor="var(--surface-border)"
              _hover={
                filters.readStatus !== 'all'
                  ? { bg: 'var(--alt-primary)', opacity: 0.8 }
                  : { borderColor: 'var(--alt-primary)', color: 'var(--alt-primary)' }
              }
            >
              {filters.readStatus}
            </Button>
          </Box>

          {/* Priority Filter */}
          <Box>
            <Text fontSize="xs" color="var(--text-muted)" mb={2}>
              Priority
            </Text>
            <Button
              data-testid="priority-filter"
              size="sm"
              variant={filters.priority !== 'all' ? 'solid' : 'outline'}
              colorScheme={filters.priority !== 'all' ? 'orange' : 'gray'}
              onClick={handlePriorityChange}
              bg={filters.priority !== 'all' ? 'var(--alt-secondary)' : 'transparent'}
              color={filters.priority !== 'all' ? 'white' : 'var(--text-secondary)'}
              borderColor="var(--surface-border)"
              _hover={
                filters.priority !== 'all'
                  ? { bg: 'var(--alt-secondary)', opacity: 0.8 }
                  : { borderColor: 'var(--alt-secondary)', color: 'var(--alt-secondary)' }
              }
            >
              {filters.priority}
            </Button>
          </Box>

          {/* Time Range Filter */}
          <Box>
            <Text fontSize="xs" color="var(--text-muted)" mb={2}>
              Time Range
            </Text>
            <Button
              data-testid="time-range-filter"
              size="sm"
              variant={filters.timeRange !== 'all' ? 'solid' : 'outline'}
              colorScheme={filters.timeRange !== 'all' ? 'green' : 'gray'}
              onClick={handleTimeRangeChange}
              bg={filters.timeRange !== 'all' ? 'var(--accent-primary)' : 'transparent'}
              color={filters.timeRange !== 'all' ? 'white' : 'var(--text-secondary)'}
              borderColor="var(--surface-border)"
              _hover={
                filters.timeRange !== 'all'
                  ? { bg: 'var(--accent-primary)', opacity: 0.8 }
                  : { borderColor: 'var(--accent-primary)', color: 'var(--accent-primary)' }
              }
            >
              {filters.timeRange}
            </Button>
          </Box>
        </HStack>

        {/* Sources Filter */}
        {availableSources.length > 0 && (
          <Box data-testid="sources-filter">
            <Text fontSize="xs" color="var(--text-muted)" mb={2}>
              Sources ({filters.sources.length} selected)
            </Text>
            <HStack gap={2} wrap="wrap">
              {availableSources.map(source => (
                <Button
                  key={source.id}
                  size="sm"
                  variant={filters.sources.includes(source.id) ? 'solid' : 'outline'}
                  onClick={() => handleSourceToggle(source.id)}
                  bg={filters.sources.includes(source.id) ? 'var(--alt-primary)' : 'transparent'}
                  color={filters.sources.includes(source.id) ? 'white' : 'var(--text-secondary)'}
                  borderColor="var(--surface-border)"
                  _hover={
                    filters.sources.includes(source.id)
                      ? { bg: 'var(--alt-primary)', opacity: 0.8 }
                      : { borderColor: 'var(--alt-primary)', color: 'var(--alt-primary)' }
                  }
                >
                  {source.icon} {source.name}
                </Button>
              ))}
            </HStack>
          </Box>
        )}

        {/* Tags Filter */}
        {availableTags.length > 0 && (
          <Box data-testid="tags-filter">
            <Text fontSize="xs" color="var(--text-muted)" mb={2}>
              Tags ({filters.tags.length} selected)
            </Text>
            <HStack gap={2} wrap="wrap">
              {availableTags.map(tag => (
                <Button
                  key={tag}
                  size="sm"
                  variant={filters.tags.includes(tag) ? 'solid' : 'outline'}
                  onClick={() => handleTagToggle(tag)}
                  bg={filters.tags.includes(tag) ? 'var(--accent-secondary)' : 'transparent'}
                  color={filters.tags.includes(tag) ? 'white' : 'var(--text-secondary)'}
                  borderColor="var(--surface-border)"
                  _hover={
                    filters.tags.includes(tag)
                      ? { bg: 'var(--accent-secondary)', opacity: 0.8 }
                      : { borderColor: 'var(--accent-secondary)', color: 'var(--accent-secondary)' }
                  }
                >
                  #{tag}
                </Button>
              ))}
            </HStack>
          </Box>
        )}
      </VStack>
    </Box>
  );
};