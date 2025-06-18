"use client";

import { Box, Button, Flex, IconButton, Portal, Text, VStack } from "@chakra-ui/react";
import Link from "next/link";
import { useState, useCallback, useEffect } from "react";
import { FaBars, FaTimes } from "react-icons/fa";

export const FloatingMenu = () => {
  const [isOpen, setIsOpen] = useState(false);

  const handleOpenMenu = useCallback(() => {
    setIsOpen(true);
  }, []);

  const handleCloseMenu = useCallback(() => {
    setIsOpen(false);
  }, []);

  const handleToggleMenu = useCallback(() => {
    setIsOpen(prev => !prev);
  }, []);

  // Close menu on escape key
  useEffect(() => {
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && isOpen) {
        handleCloseMenu();
      }
    };

    if (isOpen) {
      document.addEventListener('keydown', handleEscape);
      // Prevent background scrolling when menu is open
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = 'unset';
    }

    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.body.style.overflow = 'unset';
    };
  }, [isOpen, handleCloseMenu]);

  const menuItems = [
    {
      label: "View Feeds",
      href: "/mobile/feeds",
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
  ];

  return (
    <>
      {!isOpen && (
        <Box
          position="fixed"
          bottom={4}
          right={4}
          zIndex={1000}
        >
          <Button
            onClick={handleOpenMenu}
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
            backdropFilter="blur(4px)"
            zIndex={9999}
            display="flex"
            alignItems="center"
            justifyContent="center"
            onClick={handleCloseMenu}
            data-testid="modal-backdrop"
          >
            <Box
              width="320px"
              maxWidth="90vw"
              background="linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)"
              borderRadius="xl"
              boxShadow="xl"
              border="1px solid rgba(255, 255, 255, 0.1)"
              p={4}
              data-testid="menu-content"
              onClick={(e) => e.stopPropagation()}
              position="relative"
            >
              <Flex justify="space-between" align="center" mb={4}>
                <Text color="#ff006e" fontWeight="semibold" fontSize="lg">
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

              <VStack gap={2} align="stretch">
                {menuItems.map((item, index) => (
                  <Link key={index} href={item.href} style={{ textDecoration: 'none' }}>
                    <Box
                      width="full"
                      p={3}
                      bg="rgba(255, 255, 255, 0.05)"
                      borderRadius="lg"
                      border="1px solid rgba(255, 255, 255, 0.08)"
                      textAlign="center"
                      transition="all 0.2s ease"
                      _hover={{
                        bg: "rgba(255, 255, 255, 0.1)",
                        borderColor: "#ff006e",
                        transform: "translateY(-1px)",
                      }}
                      _active={{
                        transform: "translateY(0px)",
                      }}
                    >
                      <Text
                        color="white"
                        fontWeight="medium"
                        fontSize="sm"
                      >
                        {item.label}
                      </Text>
                    </Box>
                  </Link>
                ))}
              </VStack>
            </Box>
          </Box>
        </Portal>
      )}
    </>
  );
};