import React from 'react';
import Link from 'next/link';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import {
  Box,
  VStack,
  Text,
  Button,
  Flex,
  IconButton,
  Badge
} from '@chakra-ui/react';

interface NavItem {
  id: number;
  label: string;
  icon: React.ComponentType<{ size?: number }>;
  href: string;
  active?: boolean;
}

interface FeedSource {
  id: string;
  name: string;
  icon: string;
  unreadCount: number;
  category: string;
}

interface FilterState {
  readStatus: 'all' | 'read' | 'unread';
  sources: string[];
  priority: 'all' | 'high' | 'medium' | 'low';
  tags: string[];
  timeRange: 'all' | 'today' | 'week' | 'month';
}

interface DesktopSidebarProps {
  // Navigation props
  navItems?: NavItem[];
  logoText?: string;
  logoSubtext?: string;

  // Feeds filtering props
  activeFilters?: FilterState;
  onFilterChange?: (filters: FilterState) => void;
  feedSources?: FeedSource[];
  isCollapsed?: boolean;
  onToggleCollapse?: () => void;

  // Mode prop to determine which interface to use
  mode?: 'navigation' | 'feeds-filter';
}

export const DesktopSidebar: React.FC<DesktopSidebarProps> = ({
  navItems = [],
  logoText = 'Alt RSS',
  logoSubtext = 'Feed Reader',
  activeFilters,
  onFilterChange,
  feedSources = [],
  isCollapsed = false,
  onToggleCollapse,
  mode = 'navigation'
}) => {
  if (mode === 'feeds-filter') {
    return (
      <Box 
        className="glass" 
        h="full" 
        p="var(--space-6)"
        data-testid="desktop-sidebar-filters"
      >
        {/* Header with collapse toggle */}
        <Flex justify="space-between" align="center" mb={6}>
          <Text 
            fontSize="lg" 
            fontWeight="bold" 
            color="var(--text-primary)"
            data-testid="filter-header-title"
          >
            Filters
          </Text>
          {onToggleCollapse && (
            <IconButton
              onClick={onToggleCollapse}
              bg="var(--surface-bg)"
              color="var(--text-primary)"
              borderRadius="var(--radius-md)"
              size="sm"
              aria-label="Collapse sidebar"
              _hover={{
                bg: 'var(--surface-hover)',
                transform: 'translateY(-2px)'
              }}
              transition="all var(--transition-speed) ease"
              data-testid="sidebar-collapse-toggle"
            >
              {isCollapsed ? (
                <ChevronRight size={16} />
              ) : (
                <ChevronLeft size={16} />
              )}
            </IconButton>
          )}
        </Flex>

        {!isCollapsed && (
          <VStack gap={6} align="stretch" flex={1}>
            {/* Read Status Filter */}
            <Box>
              <Text 
                fontSize="sm" 
                fontWeight="medium" 
                color="var(--text-primary)" 
                mb={3}
                data-testid="filter-read-status-label"
              >
                Read Status
              </Text>
              <VStack gap={2} align="start">
                {['all', 'unread', 'read'].map((status) => (
                  <Box
                    key={status}
                    as="label"
                    display="flex"
                    alignItems="center"
                    gap={2}
                    cursor="pointer"
                    css={{
                      '&:hover .radio-custom': {
                        borderColor: 'var(--alt-primary)',
                      }
                    }}
                  >
                    <Box
                      className="radio-custom"
                      position="relative"
                      w="16px"
                      h="16px"
                      borderRadius="50%"
                      border="2px solid var(--surface-border)"
                      bg="var(--surface-bg)"
                      transition="all var(--transition-speed) ease"
                      css={{
                        ...(activeFilters?.readStatus === status && {
                          background: 'var(--alt-primary)',
                          borderColor: 'var(--alt-primary)',
                        })
                      }}
                    >
                      <input
                        type="radio"
                        name="readStatus"
                        value={status}
                        checked={activeFilters?.readStatus === status}
                        onChange={() => onFilterChange?.({
                          ...activeFilters!,
                          readStatus: status as 'all' | 'read' | 'unread'
                        })}
                        style={{
                          opacity: 0,
                          position: 'absolute',
                          width: '100%',
                          height: '100%',
                          cursor: 'pointer'
                        }}
                        data-testid={`filter-read-status-${status}`}
                      />
                    </Box>
                    <Text 
                      fontSize="sm" 
                      color="var(--text-secondary)" 
                      textTransform="capitalize"
                    >
                      {status}
                    </Text>
                  </Box>
                ))}
              </VStack>
            </Box>

            {/* Feed Sources */}
            <Box>
              <Text 
                fontSize="sm" 
                fontWeight="medium" 
                color="var(--text-primary)" 
                mb={3}
              >
                Sources
              </Text>
              <Box 
                maxH="160px" 
                overflowY="auto"
                css={{
                  '&::-webkit-scrollbar': {
                    width: '4px',
                  },
                  '&::-webkit-scrollbar-track': {
                    background: 'var(--surface-bg)',
                  },
                  '&::-webkit-scrollbar-thumb': {
                    background: 'var(--surface-border)',
                    borderRadius: 'var(--radius-full)',
                  },
                }}
              >
                <VStack gap={2} align="stretch">
                  {feedSources.map((source) => (
                    <Flex key={source.id} justify="space-between" align="center">
                      <Box
                        as="label"
                        display="flex"
                        alignItems="center"
                        gap={2}
                        cursor="pointer"
                        css={{
                          '&:hover .checkbox-custom': {
                            borderColor: 'var(--alt-primary)',
                          }
                        }}
                      >
                        <Box
                          className="checkbox-custom"
                          position="relative"
                          w="16px"
                          h="16px"
                          borderRadius="var(--radius-xs)"
                          border="2px solid var(--surface-border)"
                          bg="var(--surface-bg)"
                          transition="all var(--transition-speed) ease"
                          css={{
                            ...(activeFilters?.sources.includes(source.id) && {
                              background: 'var(--alt-primary)',
                              borderColor: 'var(--alt-primary)',
                            })
                          }}
                        >
                          <input
                            type="checkbox"
                            checked={activeFilters?.sources.includes(source.id)}
                            onChange={(e) => {
                              const newSources = e.target.checked
                                ? [...(activeFilters?.sources || []), source.id]
                                : (activeFilters?.sources || []).filter(id => id !== source.id);
                              onFilterChange?.({
                                ...activeFilters!,
                                sources: newSources
                              });
                            }}
                            style={{
                              opacity: 0,
                              position: 'absolute',
                              width: '100%',
                              height: '100%',
                              cursor: 'pointer'
                            }}
                            data-testid="filter-source-checkbox"
                          />
                        </Box>
                        <Text fontSize="sm" color="var(--text-secondary)">
                          {source.name}
                        </Text>
                      </Box>
                      <Badge
                        bg="var(--surface-bg)"
                        color="var(--text-muted)"
                        fontSize="xs"
                        px={2}
                        py={1}
                        borderRadius="var(--radius-sm)"
                        border="1px solid var(--surface-border)"
                      >
                        {source.unreadCount}
                      </Badge>
                    </Flex>
                  ))}
                </VStack>
              </Box>
            </Box>

            {/* Time Range Filter */}
            <Box>
              <Text 
                fontSize="sm" 
                fontWeight="medium" 
                color="var(--text-primary)" 
                mb={3}
              >
                Time Range
              </Text>
              <VStack gap={2} align="start">
                {['all', 'today', 'week', 'month'].map((range) => (
                  <Box
                    key={range}
                    as="label"
                    display="flex"
                    alignItems="center"
                    gap={2}
                    cursor="pointer"
                    css={{
                      '&:hover .radio-custom': {
                        borderColor: 'var(--alt-primary)',
                      }
                    }}
                  >
                    <Box
                      className="radio-custom"
                      position="relative"
                      w="16px"
                      h="16px"
                      borderRadius="50%"
                      border="2px solid var(--surface-border)"
                      bg="var(--surface-bg)"
                      transition="all var(--transition-speed) ease"
                      css={{
                        ...(activeFilters?.timeRange === range && {
                          background: 'var(--alt-primary)',
                          borderColor: 'var(--alt-primary)',
                        })
                      }}
                    >
                      <input
                        type="radio"
                        name="timeRange"
                        value={range}
                        checked={activeFilters?.timeRange === range}
                        onChange={() => onFilterChange?.({
                          ...activeFilters!,
                          timeRange: range as 'all' | 'today' | 'week' | 'month'
                        })}
                        style={{
                          opacity: 0,
                          position: 'absolute',
                          width: '100%',
                          height: '100%',
                          cursor: 'pointer'
                        }}
                        data-testid={`filter-time-range-${range}`}
                      />
                    </Box>
                    <Text 
                      fontSize="sm" 
                      color="var(--text-secondary)" 
                      textTransform="capitalize"
                    >
                      {range}
                    </Text>
                  </Box>
                ))}
              </VStack>
            </Box>

            {/* Clear Filters Button */}
            <Button
              onClick={() => onFilterChange?.({
                sources: [],
                timeRange: 'all',
                readStatus: 'all',
                tags: [],
                priority: 'all'
              })}
              w="full"
              bg="var(--surface-bg)"
              color="var(--text-primary)"
              borderColor="var(--surface-border)"
              borderWidth="1px"
              borderRadius="var(--radius-md)"
              fontSize="sm"
              fontWeight="medium"
              _hover={{
                bg: 'var(--surface-hover)',
                borderColor: 'var(--alt-primary)',
                transform: 'translateY(-2px)'
              }}
              transition="all var(--transition-speed) ease"
              data-testid="filter-clear-button"
            >
              Clear Filters
            </Button>
          </VStack>
        )}
      </Box>
    );
  }

  // Default navigation mode
  return (
    <Box className="glass" h="full" p="var(--space-6)">
      {/* Logo Section */}
      <Box mb={8}>
        <Text 
          fontSize="2xl" 
          fontWeight="bold" 
          color="var(--text-primary)" 
          mb={1}
        >
          {logoText}
        </Text>
        <Text 
          fontSize="sm" 
          color="var(--text-secondary)"
        >
          {logoSubtext}
        </Text>
      </Box>

      {/* Navigation */}
      <Box as="nav" flex={1} aria-label="Main navigation">
        <VStack gap={2} align="stretch">
          {navItems.map((item) => (
            <Box key={item.id}>
              <Link href={item.href} style={{ textDecoration: 'none' }}>
                <Flex
                  align="center"
                  gap={3}
                  px={4}
                  py={3}
                  borderRadius="var(--radius-md)"
                  bg={item.active ? 'var(--surface-hover)' : 'transparent'}
                  color={item.active ? 'var(--alt-primary)' : 'var(--text-secondary)'}
                  _hover={{
                    bg: 'var(--surface-hover)',
                    color: 'var(--alt-primary)',
                    transform: 'translateY(-2px)'
                  }}
                  transition="all var(--transition-speed) ease"
                >
                  <item.icon size={20} />
                  <Text fontWeight="medium">{item.label}</Text>
                </Flex>
              </Link>
            </Box>
          ))}
        </VStack>
      </Box>
    </Box>
  );
};