"use client";

import {
  Box,
  Button,
  Text,
  VStack,
  HStack,
  Drawer,
  Portal,
  Flex,
} from "@chakra-ui/react";
import {
  Accordion,
  AccordionItem,
  AccordionButton,
  AccordionPanel,
  AccordionIcon,
} from "@chakra-ui/accordion";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState, useCallback, useEffect } from "react";
import { Rss, Search, Plus, Eye, ChartBar, Home, Newspaper, Star, Menu, X } from "lucide-react";
import { ThemeToggle } from "../../ThemeToggle";

type MenuCategory = "feeds" | "other" | "articles";

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
      label: "Viewed Feeds",
      href: "/mobile/feeds/viewed",
      category: "feeds",
      icon: <Eye size={18} />,
      description: "Previously read feeds",
    },
    {
      label: "Register Feed",
      href: "/mobile/feeds/register",
      category: "feeds",
      icon: <Plus size={18} />,
      description: "Add new RSS feed",
    },
    {
      label: "Search Feeds",
      href: "/mobile/feeds/search",
      category: "feeds",
      icon: <Search size={18} />,
      description: "Find specific feeds",
    },
    {
      label: "Search Articles",
      href: "/mobile/articles/search",
      category: "articles",
      icon: <Newspaper size={18} />,
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
          <Box position="fixed" bottom={6} right={6} zIndex={1000}>
            <Button
              data-testid="floating-menu-button"
              size="md"
              borderRadius="full"
              bg="var(--alt-primary)"
              color="var(--alt-text-primary)"
              p={0}
              w="48px"
              h="48px"
              shadow="0 4px 16px var(--accent-primary)"
              border="2px solid rgba(255, 255, 255, 0.2)"
              _hover={{
                transform: "scale(1.05) rotate(90deg)",
                shadow: "0 6px 20px var(--accent-primary)",
                bg: "var(--alt-primary)",
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
              {/* Animated background pulse */}
              <Box
                position="absolute"
                top="50%"
                left="50%"
                transform="translate(-50%, -50%)"
                w="120%"
                h="120%"
                bg="var(--accent-gradient)"
                borderRadius="full"
                opacity="0.6"
                css={{
                  "@keyframes pulse": {
                    "0%, 100%": {
                      opacity: 0.6,
                      transform: "translate(-50%, -50%) scale(1)",
                    },
                    "50%": {
                      opacity: 0.8,
                      transform: "translate(-50%, -50%) scale(1.1)",
                    },
                  },
                  animation: "pulse 2s ease-in-out infinite",
                }}
              />
              <Menu size={16} style={{ position: "relative", zIndex: 1 }} />
            </Button>
          </Box>
        </Drawer.Trigger>

        <Portal>
          <Drawer.Backdrop
            data-testid="modal-backdrop"
            bg="rgba(0, 0, 0, 0.7)"
            backdropFilter="blur(12px)"
          />
          <Drawer.Positioner>
            <Drawer.Content
              w="100vw"
              maxW="100vw"
              bg="var(--app-bg)"
              color="white"
              borderTopRadius="32px"
              pt={6}
              data-testid="bottom-sheet-menu"
              minH="70vh"
              maxH="90vh"
              overflowY="auto"
              position="relative"
              _before={{
                content: '""',
                position: "absolute",
                top: 0,
                left: 0,
                right: 0,
                height: "4px",
                bgGradient: "var(--accent-gradient)",
                borderTopRadius: "32px",
              }}
            >
              {/* Drag handle */}
              <Box
                w="48px"
                h="4px"
                bg="rgba(255, 255, 255, 0.3)"
                borderRadius="full"
                mx="auto"
                mb={4}
              />

              <Drawer.Header
                borderBottomWidth="1px"
                borderColor="rgba(255, 255, 255, 0.1)"
                pb={4}
                px={6}
              >
                <Flex align="center" justify="space-between">
                  <HStack gap={3}>
                    <Box
                      w="40px"
                      h="40px"
                      bg="var(--alt-primary)"
                      borderRadius="full"
                      display="flex"
                      alignItems="center"
                      justifyContent="center"
                    >
                      <Star size={18} color="white" />
                    </Box>
                    <Box>
                      <Drawer.Title
                        fontSize="xl"
                        fontWeight="bold"
                        bgGradient="var(--accent-primary)"
                        bgClip="text"
                      >
                        Navigation
                      </Drawer.Title>
                      <Text fontSize="sm" color="var(--text-primary)">
                        Quick access to all features
                      </Text>
                    </Box>
                  </HStack>
                </Flex>
              </Drawer.Header>

              <Drawer.Body px={6} py={4}>
                <Accordion allowToggle defaultIndex={0}>
                  {categories.map((cat, idx) => (
                    <AccordionItem key={idx} border="none" mb={4}>
                      {({ isExpanded }) => (
                        <>
                          <h2>
                            <AccordionButton
                              data-testid={`tab-${cat.title.toLowerCase()}`}
                              px={6}
                              py={5}
                              justifyContent="space-between"
                              bg="var(--alt-glass)"
                              backdropFilter="blur(20px)"
                              border="1px solid var(--alt-glass-border)"
                              borderRadius="16px"
                              _hover={{
                                bg: "var(--alt-glass)",
                                transform: "translateY(-2px)",
                                boxShadow: "0 8px 25px rgba(0, 0, 0, 0.2)",
                              }}
                              _expanded={{
                                bg: "var(--alt-glass)",
                                borderColor: "var(--alt-glass-border)",
                                boxShadow: "0 0 30px var(--alt-glass-shadow)",
                              }}
                              transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
                            >
                              <HStack gap={4} flex="1">
                                <Box
                                  w="32px"
                                  h="32px"
                                  bg="var(--alt-secondary)"
                                  borderRadius="8px"
                                  display="flex"
                                  alignItems="center"
                                  justifyContent="center"
                                  color="white"
                                >
                                  {cat.icon}
                                </Box>
                                <Box textAlign="left">
                                  <Text
                                    fontWeight="bold"
                                    fontSize="lg"
                                    color="var(--text-primary)"
                                  >
                                    {cat.title}
                                  </Text>
                                  <Text
                                    fontSize="sm"
                                    color="var(--text-primary)"
                                  >
                                    {cat.items.length} items
                                  </Text>
                                </Box>
                              </HStack>
                              <AccordionIcon
                                color="var(--alt-text-primary)"
                                transition="transform 0.2s"
                                transform={isExpanded ? "rotate(180deg)" : "rotate(0deg)"}
                              />
                            </AccordionButton>
                          </h2>
                          <AccordionPanel pb={4} pt={4} px={0}>
                            <VStack gap={2} align="stretch">
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
                                      mx="auto"
                                      maxW="320px"
                                      w="85%"
                                      bg={
                                        isActive
                                          ? "var(--alt-glass)"
                                          : "var(--alt-glass)"
                                      }
                                      borderRadius="10px"
                                      border={`1px solid ${isActive
                                        ? "var(--alt-glass-border)"
                                        : "var(--alt-glass-border)"
                                        }`}
                                      p={3}
                                      _hover={{
                                        bg: isActive
                                          ? "var(--alt-glass)"
                                          : "var(--alt-glass)",
                                        borderColor: "var(--alt-glass-border)",
                                        transform: "translateY(-1px)",
                                        boxShadow:
                                          "0 2px 8px var(--alt-glass-shadow)",
                                      }}
                                      _active={{ transform: "translateY(0px)" }}
                                      transition="all 0.2s ease"
                                      position="relative"
                                      overflow="hidden"
                                    >
                                      {/* Active indicator */}
                                      {isActive && (
                                        <Box
                                          position="absolute"
                                          left="0"
                                          top="0"
                                          bottom="0"
                                          w="3px"
                                          bg="var(--accent-gradient)"
                                          borderRadius="0 2px 2px 0"
                                        />
                                      )}

                                      <HStack gap={3} align="center">
                                        <Box
                                          color={
                                            isActive
                                              ? "var(--accent-primary)"
                                              : "var(--alt-text-secondary)"
                                          }
                                          transition="color 0.2s ease"
                                          fontSize="16px"
                                        >
                                          {item.icon}
                                        </Box>
                                        <Box flex="1">
                                          <HStack
                                            justify="space-between"
                                            align="center"
                                          >
                                            <Text
                                              color={
                                                isActive ? "var(--text-primary)" : "white"
                                              }
                                              fontWeight={
                                                isActive ? "semibold" : "medium"
                                              }
                                              fontSize="sm"
                                              lineHeight="1.2"
                                            >
                                              {item.label}
                                            </Text>
                                          </HStack>
                                          {item.description && (
                                            <Text
                                              fontSize="xs"
                                              color="var(--alt-text-primary)"
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
                <Box
                  mt={6}
                  pt={4}
                  borderTop="1px solid var(--accent-primary)"
                >
                  <ThemeToggle size="md" showLabel />
                </Box>
              </Drawer.Body>

              <Drawer.CloseTrigger asChild>
                <Button
                  aria-label="Close menu"
                  data-testid="close-menu-button"
                  bg="var(--alt-primary)"
                  color="var(--text-primary)"
                  borderRadius="full"
                  _hover={{
                    bg: "var(--accent-primary)",
                    transform: "rotate(90deg)",
                  }}
                  position="absolute"
                  top="20px"
                  right="20px"
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
