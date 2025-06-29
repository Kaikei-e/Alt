"use client";

import {
  Box,
  Button,
  Text,
  VStack,
  Drawer,
  Portal,
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
import { FaBars, FaTimes } from "react-icons/fa";

type MenuCategory = "feeds" | "other" | "articles";

interface MenuItem {
  label: string;
  href: string;
  category: MenuCategory;
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
    },
    {
      label: "Read Feeds",
      href: "/mobile/feeds/read",
      category: "feeds",
    },
    {
      label: "Register Feed",
      href: "/mobile/feeds/register",
      category: "feeds",
    },
    {
      label: "Search Feeds",
      href: "/mobile/feeds/search",
      category: "feeds",
    },
    {
      label: "Search Articles",
      href: "/mobile/articles/search",
      category: "articles",
    },
    {
      label: "View Stats",
      href: "/mobile/feeds/stats",
      category: "other",
    },
    {
      label: "Home",
      href: "/",
      category: "other",
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
    },
    {
      title: "Articles",
      items: menuItems.filter((i) => i.category === "articles"),
    },
    {
      title: "Other",
      items: menuItems.filter((i) => i.category === "other"),
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
          <Box position="fixed" bottom={4} right={4} zIndex={1000}>
            <Button
              data-testid="floating-menu-button"
              size="md"
              borderRadius="full"
              bg="linear-gradient(45deg, #ff006e, #8338ec)"
              color="white"
              p={2}
              minW="48px"
              h="48px"
              shadow="lg"
              border="1px solid rgba(255, 255, 255, 0.2)"
              _hover={{
                transform: "scale(1.05)",
                shadow: "xl",
                bg: "linear-gradient(45deg, #e60062, #7329d3)",
              }}
              _active={{
                transform: "scale(0.98)",
              }}
              transition="all 0.2s ease"
              tabIndex={0}
              role="button"
              aria-label="Open floating menu"
            >
              <FaBars size={16} />
            </Button>
          </Box>
        </Drawer.Trigger>

        <Portal>
          <Drawer.Backdrop data-testid="modal-backdrop" />
          <Drawer.Positioner>
            <Drawer.Content
              bg="linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)"
              color="white"
              borderTopRadius="xl"
              data-testid="bottom-sheet-menu"
            >
              <Drawer.Header
                borderBottomWidth="1px"
                borderColor="rgba(255, 255, 255, 0.1)"
              >
                <Drawer.Title>Menu</Drawer.Title>
              </Drawer.Header>

              <Drawer.Body>
                <Accordion allowToggle defaultIndex={0}>
                  {categories.map((cat, idx) => (
                    <AccordionItem key={idx} border="none">
                      <h2>
                        <AccordionButton
                          data-testid={`tab-${cat.title.toLowerCase()}`}
                          _expanded={{
                            bg: "rgba(255, 0, 110, 0.15)",
                            color: "#ff006e",
                          }}
                          borderRadius="md"
                        >
                          <Box
                            flex="1"
                            textAlign="left"
                            fontWeight="semibold"
                          >
                            {cat.title}
                          </Box>
                          <AccordionIcon />
                        </AccordionButton>
                      </h2>
                      <AccordionPanel pb={4}>
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
                                  width="full"
                                  bg={
                                    isActive
                                      ? "rgba(255, 0, 110, 0.15)"
                                      : "rgba(255, 255, 255, 0.05)"
                                  }
                                  borderRadius="md"
                                  border={`1px solid ${isActive
                                      ? "#ff006e"
                                      : "rgba(255, 255, 255, 0.08)"
                                    }`}
                                  textAlign="center"
                                  transition="all 0.15s ease"
                                  minH="44px"
                                  display="flex"
                                  alignItems="center"
                                  justifyContent="center"
                                  _hover={{
                                    bg: isActive
                                      ? "rgba(255, 0, 110, 0.25)"
                                      : "rgba(255, 255, 255, 0.1)",
                                    borderColor: "#ff006e",
                                    transform: "translateY(-1px)",
                                  }}
                                  _active={{ transform: "translateY(0px)" }}
                                >
                                  <Text
                                    color={isActive ? "#ff006e" : "white"}
                                    fontWeight={isActive ? "bold" : "medium"}
                                    fontSize="sm"
                                    lineHeight="1.2"
                                  >
                                    {item.label}
                                  </Text>
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
                  _hover={{ bg: "rgba(255, 255, 255, 0.1)" }}
                  position="absolute"
                  top="12px"
                  right="12px"
                  w="32px"
                  h="32px"
                >
                  <FaTimes size={16} />
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
