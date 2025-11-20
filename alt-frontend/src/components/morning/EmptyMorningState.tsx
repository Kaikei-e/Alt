"use client";

import { Box, Button, Flex, Text, VStack } from "@chakra-ui/react";
import { motion } from "framer-motion";
import { Coffee, Moon, Sun } from "lucide-react";
import Link from "next/link";

/**
 * EmptyMorningState component displays a user-friendly empty state
 * when there are no morning updates available.
 */
export default function EmptyMorningState() {
  const currentHour = new Date().getHours();
  const isMorning = currentHour >= 5 && currentHour < 12;
  const greeting = isMorning ? "Good Morning" : "Hello";
  const icon = isMorning ? Sun : Moon;
  const IconComponent = icon;

  return (
    <Flex
      role="region"
      aria-label="Empty morning updates state"
      direction="column"
      justify="center"
      align="center"
      minH="70vh"
      p={6}
      textAlign="center"
    >
      <VStack gap={8} maxW="400px">
        {/* Animated Icon */}
        <motion.div
          data-testid="empty-morning-state-icon"
          style={{
            width: "120px",
            height: "120px",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            background: "var(--alt-glass)",
            borderRadius: "9999px",
            border: "2px solid var(--alt-glass-border)",
          }}
          animate={{
            scale: [1, 1.05, 1],
            opacity: [0.8, 1, 0.8],
          }}
          transition={{
            duration: 3,
            repeat: Infinity,
            ease: "easeInOut",
          }}
        >
          <IconComponent
            size={56}
            color="var(--alt-text-secondary)"
            strokeWidth={1.5}
          />
        </motion.div>

        {/* Heading */}
        <VStack gap={3}>
          <Text
            as="h2"
            fontSize={{ base: "2xl", md: "3xl" }}
            fontWeight="bold"
            color="var(--alt-text-primary)"
            bgGradient="var(--accent-gradient)"
            bgClip="text"
          >
            {greeting}! ☕
          </Text>

          <Text
            fontSize="md"
            color="var(--alt-text-secondary)"
            lineHeight="1.6"
            px={4}
          >
            No overnight updates yet. Check back later for the latest articles
            from your subscribed feeds.
          </Text>
        </VStack>

        {/* Call to Action Buttons */}
        <VStack gap={3} w="100%" align="center">
          <Link href="/mobile/feeds">
            <Button
              size="lg"
              borderRadius="16px"
              bgGradient="linear(to-r, #FF416C, #FF4B2B)"
              color="white"
              fontWeight="bold"
              px={8}
              _hover={{
                transform: "translateY(-2px)",
                boxShadow: "0 8px 25px rgba(255, 65, 108, 0.4)",
              }}
              _active={{
                transform: "translateY(0)",
              }}
              transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
            >
              <Flex align="center" gap={2}>
                <Coffee size={20} />
                <Text>Browse Current Feeds</Text>
              </Flex>
            </Button>
          </Link>

          <Link href="/mobile/recap/7days">
            <Button
              size="md"
              borderRadius="16px"
              variant="outline"
              borderColor="var(--surface-border)"
              color="var(--text-primary)"
              px={6}
              _hover={{
                bg: "var(--surface-hover)",
                borderColor: "var(--alt-primary)",
              }}
              transition="all 0.2s"
            >
              View 7-Day Recap Instead
            </Button>
          </Link>
        </VStack>

        {/* Helpful info box */}
        <Box
          mt={4}
          p={4}
          bg="var(--alt-glass)"
          borderRadius="12px"
          border="1px solid var(--alt-glass-border)"
          w="100%"
        >
          <Text fontSize="sm" color="var(--alt-text-secondary)">
            <strong>💡 Tip:</strong> Morning Letter shows you grouped overnight
            updates from your subscribed feeds. Articles are automatically
            grouped by similarity to reduce clutter.
          </Text>
        </Box>
      </VStack>
    </Flex>
  );
}

