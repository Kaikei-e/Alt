"use client";
import {
  Badge,
  Box,
  Button,
  Link as ChakraLink,
  Flex,
  Icon,
  IconButton,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  ChevronLeft,
  ChevronRight,
  FileText,
  Home,
  Link as LinkIcon,
  Rss,
  Search,
  Settings,
} from "lucide-react";
import NextLink from "next/link";
import type React from "react";

// Icon resolver function to avoid Server/Client boundary issues
const getIconComponent = (
  iconName?: string,
  defaultIcon?: React.ComponentType<{ size?: number }>,
) => {
  if (defaultIcon) return defaultIcon;

  switch (iconName) {
    case "Home":
      return Home;
    case "Rss":
      return Rss;
    case "FileText":
      return FileText;
    case "Search":
      return Search;
    case "Link":
      return LinkIcon;
    case "Settings":
      return Settings;
    default:
      return Home; // fallback
  }
};

interface NavItem {
  id: number;
  label: string;
  icon?: React.ComponentType<{ size?: number }>;
  iconName?: string; // Support icon names for Server/Client boundary
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
  readStatus: "all" | "read" | "unread";
  sources: string[];
  priority: "all" | "high" | "medium" | "low";
  tags: string[];
  timeRange: "all" | "today" | "week" | "month";
}

type mode = "navigation" | "feeds-filter";

interface DesktopSidebarProps {
  // Navigation props
  navItems?: NavItem[];
  logoText?: string;
  logoSubtext?: string;

  // Feeds filtering props
  activeFilters?: FilterState;
  onFilterChange?: (filters: FilterState) => void;
  onClearAll?: () => void;
  feedSources?: FeedSource[];
  isCollapsed?: boolean;
  onToggleCollapse?: () => void;

  // Mode prop to determine which interface to use
  mode?: mode;
}

const defaultSidebarNavItems = [
  {
    id: 1,
    label: "Dashboard",
    icon: Home,
    href: "/desktop/home",
    active: true,
  },
  { id: 2, label: "Feeds", icon: Rss, href: "/desktop/feeds" },
  {
    id: 6,
    label: "Manage Feeds Links",
    iconName: "Link",
    href: "/feeds/manage",
  },
  { id: 3, label: "Articles", icon: FileText, href: "/desktop/articles" },
  { id: 4, label: "Search", icon: Search, href: "/desktop/articles/search" },
  { id: 5, label: "Settings", icon: Settings, href: "/desktop/settings" },
];

export const DefaultSidebarProps: DesktopSidebarProps = {
  navItems: defaultSidebarNavItems,
  logoText: "Alt RSS",
  logoSubtext: "Feed Reader",
  isCollapsed: false,
  onToggleCollapse: () => {},
  mode: "navigation",
};

export const DesktopSidebar: React.FC<DesktopSidebarProps> = ({
  navItems = [],
  logoText = "Alt RSS",
  logoSubtext = "Feed Reader",
  activeFilters,
  onFilterChange,
  onClearAll,
  feedSources = [],
  isCollapsed = false,
  onToggleCollapse,
  mode = "navigation",
}) => {
  if (mode === "feeds-filter") {
    return (
      <Box
        className="glass"
        h="full"
        p="var(--space-4)"
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
                bg: "var(--surface-hover)",
                transform: "translateY(-2px)",
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
                {["all", "unread", "read"].map((status) => (
                  <label
                    key={status}
                    htmlFor={`read-status-${status}`}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: "8px",
                      cursor: "pointer",
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
                          background: "var(--alt-primary)",
                          borderColor: "var(--alt-primary)",
                        }),
                      }}
                    >
                      <input
                        type="radio"
                        name="readStatus"
                        value={status}
                        id={`read-status-${status}`}
                        checked={activeFilters?.readStatus === status}
                        onChange={() =>
                          onFilterChange?.({
                            ...activeFilters!,
                            readStatus: status as "all" | "read" | "unread",
                          })
                        }
                        style={{
                          opacity: 0,
                          position: "absolute",
                          width: "100%",
                          height: "100%",
                          cursor: "pointer",
                        }}
                        data-testid={`sidebar-filter-read-status-${status}`}
                      />
                    </Box>
                    <Text
                      fontSize="sm"
                      color="var(--text-secondary)"
                      cursor="pointer"
                      textTransform="capitalize"
                    >
                      {status}
                    </Text>
                  </label>
                ))}
              </VStack>
            </Box>

            {/* Feed Sources Filter */}
            <Box>
              <Text
                fontSize="sm"
                fontWeight="medium"
                color="var(--text-primary)"
                mb={3}
                data-testid="filter-sources-label"
              >
                Sources
              </Text>
              <VStack gap={2} align="start" maxH="200px" overflowY="auto">
                {feedSources.map((source) => (
                  <label
                    key={source.id}
                    htmlFor={`source-${source.id}`}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: "8px",
                      cursor: "pointer",
                    }}
                  >
                    <Box
                      className="checkbox-custom"
                      position="relative"
                      w="16px"
                      h="16px"
                      borderRadius="4px"
                      border="2px solid var(--surface-border)"
                      bg="var(--surface-bg)"
                      transition="all var(--transition-speed) ease"
                      data-testid="filter-source-checkbox"
                      css={{
                        ...(activeFilters?.sources.includes(source.id) && {
                          background: "var(--alt-primary)",
                          borderColor: "var(--alt-primary)",
                        }),
                      }}
                    >
                      <input
                        type="checkbox"
                        id={`source-${source.id}`}
                        checked={activeFilters?.sources.includes(source.id)}
                        onChange={() => {
                          const newSources = activeFilters?.sources.includes(
                            source.id,
                          )
                            ? activeFilters.sources.filter(
                                (id) => id !== source.id,
                              )
                            : [...(activeFilters?.sources || []), source.id];
                          onFilterChange?.({
                            ...activeFilters!,
                            sources: newSources,
                          });
                        }}
                        style={{
                          opacity: 0,
                          position: "absolute",
                          width: "100%",
                          height: "100%",
                          cursor: "pointer",
                        }}
                        data-testid={`filter-source-${source.id}`}
                        className="filter-source-checkbox"
                      />
                    </Box>
                    <Flex align="center" gap={1} flex={1}>
                      <Text fontSize="sm" color="var(--text-secondary)">
                        {source.icon}
                      </Text>
                      <Text
                        fontSize="sm"
                        color="var(--text-secondary)"
                        flex={1}
                      >
                        {source.name}
                      </Text>
                      <Badge
                        bg="var(--alt-primary)"
                        color="white"
                        fontSize="xs"
                        borderRadius="full"
                        px={2}
                        py={1}
                      >
                        {source.unreadCount}
                      </Badge>
                    </Flex>
                  </label>
                ))}
              </VStack>
            </Box>

            {/* Time Range Filter */}
            <Box>
              <Text
                fontSize="sm"
                fontWeight="medium"
                color="var(--text-primary)"
                mb={3}
                data-testid="filter-time-range-label"
              >
                Time Range
              </Text>
              <VStack gap={2} align="start">
                {["all", "today", "week", "month"].map((range) => (
                  <label
                    key={range}
                    htmlFor={`time-range-${range}`}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: "8px",
                      cursor: "pointer",
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
                          background: "var(--alt-primary)",
                          borderColor: "var(--alt-primary)",
                        }),
                      }}
                    >
                      <input
                        type="radio"
                        name="timeRange"
                        value={range}
                        id={`time-range-${range}`}
                        checked={activeFilters?.timeRange === range}
                        onChange={() =>
                          onFilterChange?.({
                            ...activeFilters!,
                            timeRange: range as
                              | "all"
                              | "today"
                              | "week"
                              | "month",
                          })
                        }
                        style={{
                          opacity: 0,
                          position: "absolute",
                          width: "100%",
                          height: "100%",
                          cursor: "pointer",
                        }}
                        data-testid={`sidebar-filter-time-range-${range}`}
                      />
                    </Box>
                    <Text
                      fontSize="sm"
                      color="var(--text-secondary)"
                      cursor="pointer"
                      textTransform="capitalize"
                    >
                      {range}
                    </Text>
                  </label>
                ))}
              </VStack>
            </Box>

            {/* Clear Filters Button */}
            <Button
              onClick={
                onClearAll ||
                (() =>
                  onFilterChange?.({
                    sources: [],
                    timeRange: "all",
                    readStatus: "all",
                    tags: [],
                    priority: "all",
                  }))
              }
              variant="outline"
              size="sm"
              colorScheme="red"
              data-testid="sidebar-filter-clear-button"
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
    <VStack align="stretch" gap={8} h="full">
      {/* Logo Section */}
      <Box py={4}>
        <Text
          fontSize="xl"
          fontWeight="bold"
          bgGradient="var(--accent-gradient)"
          bgClip="text"
          className="gradient-text"
        >
          {logoText}
        </Text>
        <Text fontSize="sm" color="var(--text-muted)" mt={1}>
          {logoSubtext}
        </Text>
      </Box>

      {/* Navigation */}
      <VStack
        gap={2}
        align="stretch"
        flex="1"
        aria-label="Main navigation"
        data-testid="desktop-navigation-list"
      >
        {navItems.map((item) => (
          <ChakraLink
            key={item.id}
            as={NextLink}
            href={item.href}
            textDecoration="none"
            _hover={{ textDecoration: "none" }}
            data-testid={`desktop-nav-link-${item.label.toLowerCase()}`}
          >
            <Flex
              align="center"
              gap={3}
              p={4}
              h="52px"
              w="full"
              borderRadius="var(--radius-lg)"
              bg={item.active ? "var(--surface-hover)" : "transparent"}
              border="1px solid"
              borderColor={item.active ? "var(--alt-primary)" : "transparent"}
              color={item.active ? "var(--alt-primary)" : "var(--text-primary)"}
              transition="all var(--transition-speed) ease"
              _hover={{
                bg: "var(--surface-hover)",
                transform: "translateX(4px)",
                borderColor: "var(--alt-primary)",
              }}
            >
              <Icon
                as={getIconComponent(item.iconName, item.icon)}
                boxSize={5}
              />
              <Text fontSize="sm" fontWeight="medium">
                {item.label}
              </Text>
            </Flex>
          </ChakraLink>
        ))}
      </VStack>
    </VStack>
  );
};
