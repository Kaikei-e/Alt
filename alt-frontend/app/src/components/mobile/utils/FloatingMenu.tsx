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
import {
  FaBars,
  FaTimes,
  FaRss,
  FaSearch,
  FaPlus,
  FaEye,
  FaChartBar,
  FaHome,
  FaNewspaper,
  FaStar
} from "react-icons/fa";

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
        const firstAccordionButton = document.querySelector('[data-testid="tab-feeds"]') as HTMLElement;
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
      icon: <FaRss size={18} />,
      description: "Browse all RSS feeds",
    },
    {
      label: "Viewed Feeds",
      href: "/mobile/feeds/viewed",
      category: "feeds",
      icon: <FaEye size={18} />,
      description: "Previously read feeds",
    },
    {
      label: "Register Feed",
      href: "/mobile/feeds/register",
      category: "feeds",
      icon: <FaPlus size={18} />,
      description: "Add new RSS feed",
    },
    {
      label: "Search Feeds",
      href: "/mobile/feeds/search",
      category: "feeds",
      icon: <FaSearch size={18} />,
      description: "Find specific feeds",
    },
    {
      label: "Search Articles",
      href: "/mobile/articles/search",
      category: "articles",
      icon: <FaNewspaper size={18} />,
      description: "Search through articles",
    },
    {
      label: "View Stats",
      href: "/mobile/feeds/stats",
      category: "other",
      icon: <FaChartBar size={18} />,
      description: "Analytics & insights",
    },
    {
      label: "Home",
      href: "/",
      category: "other",
      icon: <FaHome size={18} />,
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
      icon: <FaRss size={16} />,
      gradient: "linear-gradient(135deg, #ff006e 0%, #ff4081 100%)",
    },
    {
      title: "Articles",
      items: menuItems.filter((i) => i.category === "articles"),
      icon: <FaNewspaper size={16} />,
      gradient: "linear-gradient(135deg, #8338ec 0%, #9c27b0 100%)",
    },
    {
      title: "Other",
      items: menuItems.filter((i) => i.category === "other"),
      icon: <FaStar size={16} />,
      gradient: "linear-gradient(135deg, #3a86ff 0%, #2196f3 100%)",
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
              bg="linear-gradient(135deg, #ff006e 0%, #8338ec 50%, #3a86ff 100%)"
              color="white"
              p={0}
              w="48px"
              h="48px"
              shadow="0 4px 16px rgba(255, 0, 110, 0.3)"
              border="2px solid rgba(255, 255, 255, 0.2)"
              _hover={{
                transform: "scale(1.05) rotate(90deg)",
                shadow: "0 6px 20px rgba(255, 0, 110, 0.4)",
                bg: "linear-gradient(135deg, #e6005c 0%, #7129d4 50%, #2979ff 100%)",
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
            >
              {/* Animated background pulse */}
              <Box
                position="absolute"
                top="50%"
                left="50%"
                transform="translate(-50%, -50%)"
                w="120%"
                h="120%"
                bg="linear-gradient(135deg, rgba(255, 255, 255, 0.2) 0%, transparent 100%)"
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
              <FaBars size={16} style={{ position: "relative", zIndex: 1 }} />
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
              bg="linear-gradient(135deg, #0a0a0f 0%, #1a1a2e 30%, #16213e 70%, #0f0f23 100%)"
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
                bgGradient: "linear(to-r, #ff006e, #8338ec, #3a86ff)",
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
                      bg="linear-gradient(135deg, #ff006e, #8338ec)"
                      borderRadius="full"
                      display="flex"
                      alignItems="center"
                      justifyContent="center"
                    >
                      <FaStar size={18} color="white" />
                    </Box>
                    <Box>
                      <Drawer.Title
                        fontSize="xl"
                        fontWeight="bold"
                        bgGradient="linear(to-r, #ff006e, #8338ec)"
                        bgClip="text"
                      >
                        Navigation
                      </Drawer.Title>
                      <Text fontSize="sm" color="rgba(255, 255, 255, 0.6)">
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
                      <h2>
                        <AccordionButton
                          data-testid={`tab-${cat.title.toLowerCase()}`}
                          px={6}
                          py={5}
                          justifyContent="space-between"
                          bg="rgba(255, 255, 255, 0.04)"
                          backdropFilter="blur(20px)"
                          border="1px solid rgba(255, 255, 255, 0.1)"
                          borderRadius="16px"
                          _hover={{
                            bg: "rgba(255, 255, 255, 0.08)",
                            transform: "translateY(-2px)",
                            boxShadow: "0 8px 25px rgba(0, 0, 0, 0.2)",
                          }}
                          _expanded={{
                            bg: "rgba(255, 255, 255, 0.08)",
                            borderColor: "rgba(255, 0, 110, 0.4)",
                            boxShadow: "0 0 30px rgba(255, 0, 110, 0.2)",
                          }}
                          transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
                        >
                          <HStack gap={4} flex="1">
                            <Box
                              w="32px"
                              h="32px"
                              bg={cat.gradient}
                              borderRadius="8px"
                              display="flex"
                              alignItems="center"
                              justifyContent="center"
                              color="white"
                            >
                              {cat.icon}
                            </Box>
                            <Box textAlign="left">
                              <Text fontWeight="bold" fontSize="lg" color="white">
                                {cat.title}
                              </Text>
                              <Text fontSize="sm" color="rgba(255, 255, 255, 0.6)">
                                {cat.items.length} items
                              </Text>
                            </Box>
                          </HStack>
                          <AccordionIcon color="rgba(255, 255, 255, 0.7)" />
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
                                      ? "rgba(255, 0, 110, 0.08)"
                                      : "rgba(255, 255, 255, 0.02)"
                                  }
                                  borderRadius="10px"
                                  border={`1px solid ${isActive
                                      ? "rgba(255, 0, 110, 0.25)"
                                      : "rgba(255, 255, 255, 0.04)"
                                    }`}
                                  p={3}
                                  _hover={{
                                    bg: isActive
                                      ? "rgba(255, 0, 110, 0.12)"
                                      : "rgba(255, 255, 255, 0.04)",
                                    borderColor: "rgba(255, 0, 110, 0.3)",
                                    transform: "translateY(-1px)",
                                    boxShadow: "0 2px 8px rgba(0, 0, 0, 0.15)",
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
                                      bg="linear-gradient(to bottom, #ff006e, #8338ec)"
                                      borderRadius="0 2px 2px 0"
                                    />
                                  )}

                                  <HStack gap={3} align="center">
                                    <Box
                                      color={isActive ? "#ff006e" : "rgba(255, 255, 255, 0.6)"}
                                      transition="color 0.2s ease"
                                      fontSize="16px"
                                    >
                                      {item.icon}
                                    </Box>
                                    <Box flex="1">
                                      <HStack justify="space-between" align="center">
                                        <Text
                                          color={isActive ? "#ff006e" : "white"}
                                          fontWeight={isActive ? "semibold" : "medium"}
                                          fontSize="sm"
                                          lineHeight="1.2"
                                        >
                                          {item.label}
                                        </Text>
                                      </HStack>
                                      {item.description && (
                                        <Text
                                          fontSize="xs"
                                          color="rgba(255, 255, 255, 0.4)"
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
                    </AccordionItem>
                  ))}
                </Accordion>
              </Drawer.Body>

              <Drawer.CloseTrigger asChild>
                <Button
                  aria-label="Close menu"
                  data-testid="close-menu-button"
                  variant="ghost"
                  color="white"
                  borderRadius="full"
                  _hover={{
                    bg: "rgba(255, 255, 255, 0.1)",
                    transform: "rotate(90deg)",
                  }}
                  position="absolute"
                  top="20px"
                  right="20px"
                  w="40px"
                  h="40px"
                  transition="all 0.2s ease"
                >
                  <FaTimes size={18} />
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
