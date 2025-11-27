"use client";

import {
  Accordion,
  AccordionButton,
  AccordionIcon,
  AccordionItem,
  AccordionPanel,
} from "@chakra-ui/accordion";
import {
  Box,
  Button,
  Drawer,
  Flex,
  HStack,
  Portal,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  CalendarRange,
  ChartBar,
  Eye,
  Globe,
  Home,
  Infinity,
  Link as LinkIcon,
  Menu,
  Newspaper,
  Plus,
  Rss,
  Search,
  Star,
  X,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { ThemeToggle } from "../../ThemeToggle";

type MenuCategory = "feeds" | "recap" | "articles" | "other";

interface MenuItem {
  label: string;
  href: string;
  category: MenuCategory;
  icon: React.ReactNode;
  description?: string;
}

export const FloatingMenu = () => {
  const [isOpen, setIsOpen] = useState(false);
  const [isPrefetched, setIsPrefetched] = useState(false);
  const pathname = usePathname();

  const handleCloseMenu = useCallback(() => {
    setIsOpen(false);
  }, []);

  // Handle keyboard interactions
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && isOpen) {
        handleCloseMenu();
      }
    };

    if (isOpen) {
      document.addEventListener("keydown", handleKeyDown);
      // Prevent background scrolling when menu is open
      document.body.style.overflow = "hidden";

      // Focus management for accessibility
      setTimeout(() => {
        const firstAccordionButton = document.querySelector(
          '[data-testid="tab-feeds"]',
        ) as HTMLElement;
        if (firstAccordionButton) {
          firstAccordionButton.focus();
        }
      }, 100);
    } else {
      document.body.style.overflow = "unset";
    }

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.body.style.overflow = "unset";
    };
  }, [isOpen, handleCloseMenu]);

  useEffect(() => {
    if (isPrefetched) return;

    const prefetch = () => setIsPrefetched(true);
    if (typeof window !== "undefined" && "requestIdleCallback" in window) {
      (window as Window).requestIdleCallback(prefetch);
    } else {
      // Fallback for environments without requestIdleCallback
      setTimeout(prefetch, 0);
    }
  }, [isPrefetched]);

  const menuItems: MenuItem[] = [
    {
      label: "View Feeds",
      href: "/mobile/feeds",
      category: "feeds",
      icon: <Rss size={18} />,
      description: "Browse all RSS feeds",
    },
    {
      label: "Swipe Mode",
      href: "/mobile/feeds/swipe",
      category: "feeds",
      icon: <Infinity size={18} />,
      description: "Swipe through feeds",
    },
    {
      label: "Viewed Feeds",
      href: "/mobile/feeds/viewed",
      category: "feeds",
      icon: <Eye size={18} />,
      description: "Previously read feeds",
    },
    {
      label: "Favorite Feeds",
      href: "/mobile/feeds/favorites",
      category: "feeds",
      icon: <Star size={18} />,
      description: "Favorited articles",
    },
    {
      label: "Register Feed",
      href: "/mobile/feeds/register",
      category: "feeds",
      icon: <Plus size={18} />,
      description: "Add new RSS feed",
    },
    {
      label: "Manage Feeds Links",
      href: "/mobile/feeds/manage",
      category: "feeds",
      icon: <LinkIcon size={18} />,
      description: "Add or remove your registered RSS sources",
    },
    {
      label: "Search Feeds",
      href: "/mobile/feeds/search",
      category: "feeds",
      icon: <Search size={18} />,
      description: "Find specific feeds",
    },
    {
      label: "7-Day Recap",
      href: "/mobile/recap/7days",
      category: "recap",
      icon: <CalendarRange size={18} />,
      description: "Review the weekly highlights",
    },
    {
      label: "Morning Letter",
      href: "/mobile/recap/morning-letter/updates",
      category: "recap",
      icon: <Newspaper size={18} />,
      description: "Today's overnight updates",
    },
    {
      label: "View Articles",
      href: "/mobile/articles/view",
      category: "articles",
      icon: <Newspaper size={18} />,
      description: "Browse all articles",
    },
    {
      label: "Search Articles",
      href: "/mobile/articles/search",
      category: "articles",
      icon: <Search size={18} />,
      description: "Search through articles",
    },
    {
      label: "View Stats",
      href: "/mobile/feeds/stats",
      category: "other",
      icon: <ChartBar size={18} />,
      description: "Analytics & insights",
    },
    {
      label: "Home",
      href: "/",
      category: "other",
      icon: <Home size={18} />,
      description: "Return to dashboard",
    },
    {
      label: "Manage Domains",
      href: "/admin/scraping-domains",
      category: "other",
      icon: <Globe size={18} />,
      description: "Manage scraping domains",
    },
  ];

  // Helper that closes menu when a link is activated
  const handleNavigate = useCallback(() => {
    handleCloseMenu();
  }, [handleCloseMenu]);

  // Helper to check if a menu item is active
  const isActiveMenuItem = useCallback(
    (href: string): boolean => {
      return pathname === href;
    },
    [pathname],
  );

  // Group menu items into categories for accordion UI
  const categories = [
    {
      title: "Feeds",
      items: menuItems.filter((i) => i.category === "feeds"),
      icon: <Rss size={16} />,
      gradient: "var(--app-bg)",
    },
    {
      title: "Recap",
      items: menuItems.filter((i) => i.category === "recap"),
      icon: <CalendarRange size={16} />,
      gradient: "var(--accent-gradient)",
    },
    {
      title: "Articles",
      items: menuItems.filter((i) => i.category === "articles"),
      icon: <Newspaper size={16} />,
      gradient: "var(--accent-gradient)",
    },
    {
      title: "Other",
      items: menuItems.filter((i) => i.category === "other"),
      icon: <Star size={16} />,
      gradient: "var(--accent-gradient)",
    },
  ];

  return (
    <>
      <Drawer.Root
        open={isOpen}
        onOpenChange={(e) => setIsOpen(e.open)}
        placement="bottom"
      >
        <Drawer.Trigger asChild>
          <Box
            position="fixed"
            bottom={6}
            right={6}
            zIndex={1000}
            css={{
              paddingBottom: "calc(1.5rem + env(safe-area-inset-bottom, 0px))",
            }}
          >
            <Button
              data-testid="floating-menu-button"
              size="md"
              borderRadius="full"
              bg="var(--bg-surface)"
              backdropFilter="blur(12px)"
              color="var(--text-primary)"
              p={0}
              w="48px"
              h="48px"
              border="2px solid var(--text-primary)"
              boxShadow="var(--shadow-glass)"
              _hover={{
                transform: "scale(1.05) rotate(90deg)",
                bg: "var(--bg-surface-hover)",
                borderColor: "var(--accent-primary)",
              }}
              _active={{
                transform: "scale(0.95) rotate(90deg)",
              }}
              transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
              tabIndex={0}
              role="button"
              aria-label="Open floating menu"
              position="relative"
              overflow="hidden"
              onClick={() => setIsOpen(true)}
            >
              <Menu size={20} style={{ position: "relative", zIndex: 1 }} />
            </Button>
          </Box>
        </Drawer.Trigger>

        <Portal>
          <Drawer.Backdrop
            data-testid="modal-backdrop"
            bg="rgba(0, 0, 0, 0.4)"
            backdropFilter="blur(8px)"
          />
          <Drawer.Positioner>
            <Drawer.Content
              w="100vw"
              maxW="100vw"
              bg="var(--app-bg)"
              backdropFilter="blur(20px)"
              color="var(--text-primary)"
              borderTopRadius="32px"
              borderTop="1px solid var(--border-glass)"
              boxShadow="0 -10px 40px rgba(0,0,0,0.2)"
              pt={6}
              data-testid="bottom-sheet-menu"
              minH="70vh"
              maxH="90vh"
              overflowY="auto"
              position="relative"
              css={{
                paddingBottom:
                  "calc(1.5rem + env(safe-area-inset-bottom, 0px))",
              }}
            >
              {/* Drag handle */}
              <Box
                w="48px"
                h="4px"
                bg="var(--border-glass)"
                borderRadius="full"
                mx="auto"
                mb={6}
              />

              <Drawer.Header
                borderBottom="1px solid var(--border-glass)"
                pb={6}
                px={6}
              >
                <Flex align="center" justify="space-between">
                  <HStack gap={3}>
                    <Box
                      w="40px"
                      h="40px"
                      bg="var(--bg-surface)"
                      borderRadius="12px"
                      display="flex"
                      alignItems="center"
                      justifyContent="center"
                      border="1px solid var(--border-glass)"
                    >
                      <Star size={20} color="var(--accent-primary)" />
                    </Box>
                    <Box>
                      <Drawer.Title
                        fontSize="xl"
                        fontWeight="bold"
                        color="var(--text-primary)"
                      >
                        Navigation
                      </Drawer.Title>
                      <Text fontSize="sm" color="var(--text-secondary)">
                        Quick access to all features
                      </Text>
                    </Box>
                  </HStack>
                </Flex>
              </Drawer.Header>

              <Drawer.Body px={6} py={6}>
                <Accordion allowToggle defaultIndex={0} allowMultiple>
                  {categories.map((cat, idx) => (
                    <AccordionItem key={idx} border="none" mb={4}>
                      {({ isExpanded }) => (
                        <>
                          <h2>
                            <AccordionButton
                              data-testid={`tab-${cat.title.toLowerCase()}`}
                              px={4}
                              py={4}
                              justifyContent="space-between"
                              bg={isExpanded ? "var(--bg-surface)" : "transparent"}
                              borderRadius="16px"
                              _hover={{
                                bg: "var(--bg-surface-hover)",
                              }}
                              transition="all 0.2s ease"
                            >
                              <HStack gap={4} flex="1">
                                <Box
                                  color={isExpanded ? "var(--accent-primary)" : "var(--text-secondary)"}
                                  transition="color 0.2s"
                                >
                                  {cat.icon}
                                </Box>
                                <Box textAlign="left">
                                  <Text
                                    fontWeight="semibold"
                                    fontSize="lg"
                                    color="var(--text-primary)"
                                  >
                                    {cat.title}
                                  </Text>
                                </Box>
                              </HStack>
                              <AccordionIcon
                                color="var(--text-secondary)"
                                transition="transform 0.2s"
                                transform={
                                  isExpanded ? "rotate(180deg)" : "rotate(0deg)"
                                }
                              />
                            </AccordionButton>
                          </h2>
                          <AccordionPanel pb={2} pt={2} px={2}>
                            <VStack gap={1} align="stretch">
                              {cat.items.map((item, i) => {
                                const isActive = isActiveMenuItem(item.href);
                                return (
                                  <Link
                                    key={i}
                                    href={item.href}
                                    style={{ textDecoration: "none" }}
                                    onClick={handleNavigate}
                                  >
                                    <Box
                                      borderRadius="12px"
                                      p={3}
                                      bg={isActive ? "var(--bg-surface-active)" : "transparent"}
                                      _hover={{
                                        bg: "var(--bg-surface-hover)",
                                      }}
                                      transition="all 0.2s ease"
                                    >
                                      <HStack gap={3} align="center">
                                        <Box
                                          color={
                                            isActive
                                              ? "var(--accent-primary)"
                                              : "var(--text-secondary)"
                                          }
                                        >
                                          {item.icon}
                                        </Box>
                                        <Box flex="1">
                                          <Text
                                            color={
                                              isActive
                                                ? "var(--text-primary)"
                                                : "var(--text-primary)"
                                            }
                                            fontWeight={
                                              isActive ? "semibold" : "medium"
                                            }
                                            fontSize="sm"
                                          >
                                            {item.label}
                                          </Text>
                                          {item.description && (
                                            <Text
                                              fontSize="xs"
                                              color="var(--text-secondary)"
                                              mt={0.5}
                                            >
                                              {item.description}
                                            </Text>
                                          )}
                                        </Box>
                                      </HStack>
                                    </Box>
                                  </Link>
                                );
                              })}
                            </VStack>
                          </AccordionPanel>
                        </>
                      )}
                    </AccordionItem>
                  ))}
                </Accordion>

                {/* Theme Toggle */}
                <Box mt={6} pt={6} borderTop="1px solid var(--border-glass)">
                  <ThemeToggle size="md" showLabel />
                </Box>
              </Drawer.Body>

              <Drawer.CloseTrigger asChild>
                <Button
                  aria-label="Close menu"
                  data-testid="close-menu-button"
                  bg="var(--bg-glass)"
                  backdropFilter="blur(12px)"
                  color="var(--text-primary)"
                  borderRadius="full"
                  border="1px solid var(--border-glass)"
                  _hover={{
                    bg: "var(--bg-surface-hover)",
                    transform: "rotate(90deg)",
                    borderColor: "var(--accent-primary)",
                  }}
                  position="absolute"
                  top="24px"
                  right="24px"
                  w="40px"
                  h="40px"
                  transition="all 0.2s ease"
                >
                  <X size={18} />
                </Button>
              </Drawer.CloseTrigger>
            </Drawer.Content>
          </Drawer.Positioner>
        </Portal>
      </Drawer.Root>

      {/* Hidden pre-rendered menu to warm up rendering & reduce first-open cost */}
      {!isOpen && isPrefetched && (
        <Box display="none">
          {menuItems.map((item) => (
            <Link key={item.href} href={item.href}>
              {item.label}
            </Link>
          ))}
        </Box>
      )}
    </>
  );
};
