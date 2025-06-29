"use client";

import {
  Box,
  Button,
  Flex,
  IconButton,
  Portal,
  Text,
  VStack,
} from "@chakra-ui/react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState, useCallback, useEffect } from "react";
import { FaBars, FaTimes } from "react-icons/fa";

export const FloatingMenu = () => {
  const [isOpen, setIsOpen] = useState(false);
  const [isPrefetched, setIsPrefetched] = useState(false);
  const pathname = usePathname();

  const handleOpenMenu = useCallback(() => {
    setIsOpen(true);
  }, []);

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

  const menuItems = [
    {
      label: "View Feeds",
      href: "/mobile/feeds",
    },
    {
      label: "Read Feeds",
      href: "/mobile/feeds/read",
    },
    {
      label: "Register Feed",
      href: "/mobile/feeds/register",
    },
    {
      label: "Search Feeds",
      href: "/mobile/feeds/search",
    },
    {
      label: "Search Articles",
      href: "/mobile/articles/search",
    },
    {
      label: "View Stats",
      href: "/mobile/feeds/stats",
    },
    {
      label: "Home",
      href: "/",
    },
  ];

  // Helper that closes menu when a link is activated
  const handleNavigate = useCallback(() => {
    handleCloseMenu();
  }, [handleCloseMenu]);

  // Helper to check if a menu item is active
  const isActiveMenuItem = useCallback((href: string): boolean => {
    return pathname === href;
  }, [pathname]);

  return (
    <>
      {!isOpen && (
        <Box position="fixed" bottom={4} right={4} zIndex={1000}>
          <Button
            onClick={handleOpenMenu}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                handleOpenMenu();
              }
            }}
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
      )}

      {isOpen && (
        <Portal>
          <Box
            position="fixed"
            top="0"
            left="0"
            width="100vw"
            height="100vh"
            bg="rgba(0, 0, 0, 0.6)"
            backdropFilter="blur(16px)"
            zIndex={9999}
            display="flex"
            alignItems="center"
            justifyContent="center"
            onClick={handleCloseMenu}
            data-testid="modal-backdrop"
          >
            <Box
              width="90vw"
              maxWidth="320px"
              maxHeight="400px" // Limit height to prevent overflow
              background="linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)"
              borderRadius="xl"
              boxShadow="xl"
              border="1px solid rgba(255, 255, 255, 0.1)"
              p={3} // Reduced padding
              data-testid="menu-content"
              onClick={(e: React.MouseEvent<HTMLDivElement>) =>
                e.stopPropagation()
              }
              position="relative"
              overflow="hidden"
            >
              <Flex justify="space-between" align="center" mb={3}>
                <Text color="#ff006e" fontWeight="semibold" fontSize="md">
                  Menu
                </Text>
                <IconButton
                  onClick={handleCloseMenu}
                  data-testid="close-menu-button"
                  aria-label="Close menu"
                  variant="ghost"
                  size="sm"
                  color="white"
                  borderRadius="full"
                  _hover={{
                    bg: "rgba(255, 255, 255, 0.1)",
                  }}
                >
                  <FaTimes size={14} />
                </IconButton>
              </Flex>

              <VStack
                gap={1.5}
                align="stretch"
                maxHeight="300px"
                overflowY="auto"
              >
                {menuItems.map((item, index) => {
                  const isActive = isActiveMenuItem(item.href);
                  return (
                    <Link
                      key={index}
                      href={item.href}
                      style={{ textDecoration: "none" }}
                      onClick={handleNavigate}
                    >
                      <Box
                        width="full"
                        bg={isActive ? "rgba(255, 0, 110, 0.15)" : "rgba(255, 255, 255, 0.05)"}
                        borderRadius="md"
                        border={`1px solid ${isActive ? "#ff006e" : "rgba(255, 255, 255, 0.08)"}`}
                        textAlign="center"
                        transition="all 0.15s ease"
                        minH="44px"
                        display="flex"
                        alignItems="center"
                        justifyContent="center"
                        _hover={{
                          bg: isActive ? "rgba(255, 0, 110, 0.25)" : "rgba(255, 255, 255, 0.1)",
                          borderColor: "#ff006e",
                          transform: "translateY(-1px)",
                        }}
                        _active={{
                          transform: "translateY(0px)",
                        }}
                        position="relative"
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
            </Box>
          </Box>
        </Portal>
      )}

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
