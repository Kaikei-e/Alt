"use client";

import {
  Box,
  Button,
  Flex,
  Progress,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import { Activity, Clock, LogOut, Settings } from "lucide-react";
import { useEffect, useState } from "react";
import { useAuth } from "@/contexts/auth-context";

interface UserProfileProps {
  onLogout?: () => void;
}

export function UserProfile({ onLogout }: UserProfileProps) {
  const { user, logout, isLoading, lastActivity, sessionTimeout, refresh } =
    useAuth();
  const [timeRemaining, setTimeRemaining] = useState<number>(0);
  const [showTimeoutWarning, setShowTimeoutWarning] = useState(false);

  // Calculate session timeout progress
  useEffect(() => {
    if (lastActivity && sessionTimeout) {
      const updateTimeRemaining = () => {
        const now = new Date();
        const minutesSinceLastActivity = Math.floor(
          (now.getTime() - lastActivity.getTime()) / (1000 * 60),
        );
        const remaining = sessionTimeout - minutesSinceLastActivity;

        setTimeRemaining(Math.max(0, remaining));
        setShowTimeoutWarning(remaining <= 5 && remaining > 0); // Show warning when 5 minutes or less remain
      };

      // Update immediately
      updateTimeRemaining();

      // Update every minute
      const interval = setInterval(updateTimeRemaining, 60000);
      return () => clearInterval(interval);
    }
  }, [lastActivity, sessionTimeout]);

  if (!user) {
    return null;
  }

  const handleLogout = async () => {
    try {
      await logout();
      onLogout?.();
    } catch (error) {
      console.error("Logout error:", error);
    }
  };

  const handleRefreshSession = async () => {
    try {
      await refresh();
    } catch (error) {
      console.error("Session refresh error:", error);
    }
  };

  const getUserInitials = (name?: string, email?: string) => {
    if (name) {
      return name
        .split(" ")
        .map((word) => word[0])
        .join("")
        .toUpperCase()
        .slice(0, 2);
    }
    if (email) {
      return email[0].toUpperCase();
    }
    return "U";
  };

  return (
    <Box
      bg="var(--alt-glass)"
      border="1px solid"
      borderColor="var(--alt-glass-border)"
      backdropFilter="blur(12px) saturate(1.2)"
      p={4}
      borderRadius="lg"
      w="full"
      maxW="400px"
      mx="auto"
      position="relative"
      overflow="hidden"
    >
      {/* Subtle gradient overlay */}
      <Box
        position="absolute"
        top={0}
        left={0}
        right={0}
        bottom={0}
        bgGradient="linear(135deg, transparent 0%, var(--alt-glass) 50%, transparent 100%)"
        opacity={0.3}
        pointerEvents="none"
      />

      <Box position="relative" zIndex={1}>
        <Flex align="center" justify="space-between" mb={4}>
          <Flex align="center" gap={3}>
            <Box
              w="3rem"
              h="3rem"
              bg="var(--alt-primary)"
              color="white"
              borderRadius="full"
              display="flex"
              alignItems="center"
              justifyContent="center"
              fontFamily="heading"
              fontWeight="semibold"
              fontSize="lg"
            >
              {getUserInitials(user.name, user.email)}
            </Box>
            <VStack align="start" gap={0}>
              <Text
                fontSize="md"
                fontWeight="semibold"
                color="var(--text-primary)"
                fontFamily="heading"
              >
                {user.name || "ユーザー"}
              </Text>
              <Text fontSize="sm" color="var(--text-muted)" fontFamily="body">
                {user.email}
              </Text>
            </VStack>
          </Flex>

          <VStack gap={1} align="end">
            <Box
              bg="var(--alt-glass)"
              color="var(--alt-primary)"
              border="1px solid"
              borderColor="var(--alt-primary)"
              fontSize="xs"
              px={2}
              py={1}
              borderRadius="full"
              display="flex"
              alignItems="center"
              gap={1}
            >
              <Activity size={10} />
              ログイン中
            </Box>

            {sessionTimeout && timeRemaining > 0 && (
              <Box
                bg={
                  showTimeoutWarning ? "semantic.warning" : "var(--alt-glass)"
                }
                color={showTimeoutWarning ? "white" : "var(--text-muted)"}
                border="1px solid"
                borderColor={
                  showTimeoutWarning
                    ? "semantic.warning"
                    : "var(--alt-glass-border)"
                }
                fontSize="xs"
                px={2}
                py={1}
                borderRadius="md"
                display="flex"
                alignItems="center"
                gap={1}
              >
                <Clock size={8} />
                {timeRemaining}分
              </Box>
            )}
          </VStack>
        </Flex>

        {/* Session timeout warning */}
        {showTimeoutWarning && (
          <Box
            bg="semantic.warning"
            color="white"
            border="1px solid"
            borderColor="semantic.warning"
            borderRadius="md"
            p={3}
            mb={2}
          >
            <VStack gap={2} align="stretch">
              <Flex align="center" gap={2}>
                <Clock size={16} />
                <Text fontSize="sm" fontWeight="semibold" fontFamily="heading">
                  セッション期限警告
                </Text>
              </Flex>
              <Text fontSize="xs" fontFamily="body">
                あと{timeRemaining}分でセッションが期限切れになります
              </Text>
              <Button
                size="sm"
                bg="white"
                color="semantic.warning"
                onClick={handleRefreshSession}
                disabled={isLoading}
                _hover={{
                  bg: "var(--alt-glass)",
                }}
              >
                セッション延長
              </Button>
            </VStack>
          </Box>
        )}

        {/* Session progress indicator */}
        {sessionTimeout && timeRemaining > 0 && (
          <Box mb={4}>
            <Flex justify="space-between" align="center" mb={1}>
              <Text fontSize="xs" color="var(--text-muted)" fontFamily="body">
                セッション残り時間
              </Text>
              <Text fontSize="xs" color="var(--text-muted)" fontFamily="body">
                {Math.round((timeRemaining / sessionTimeout) * 100)}%
              </Text>
            </Flex>
            <Progress.Root
              value={(timeRemaining / sessionTimeout) * 100}
              colorPalette={showTimeoutWarning ? "orange" : "blue"}
              size="sm"
            >
              <Progress.Track bg="var(--alt-glass)" borderRadius="full">
                <Progress.Range />
              </Progress.Track>
            </Progress.Root>
          </Box>
        )}

        <VStack gap={2} align="stretch">
          <Button
            variant="outline"
            w="full"
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-glass-border)"
            color="var(--text-primary)"
            fontFamily="body"
            _hover={{
              bg: "var(--alt-glass)",
              borderColor: "var(--alt-primary)",
              transform: "translateY(-1px)",
            }}
            _active={{
              transform: "translateY(0)",
            }}
          >
            <Flex align="center" gap={2}>
              <Settings size={16} />
              設定
            </Flex>
          </Button>

          <Button
            onClick={handleLogout}
            variant="outline"
            w="full"
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-glass-border)"
            color="var(--text-primary)"
            fontFamily="body"
            loading={isLoading}
            _hover={{
              bg: "var(--alt-glass)",
              borderColor: "semantic.error",
              color: "semantic.error",
              transform: "translateY(-1px)",
            }}
            _active={{
              transform: "translateY(0)",
            }}
          >
            {isLoading ? (
              <Flex align="center" gap={2}>
                <Spinner size="sm" />
                ログアウト中...
              </Flex>
            ) : (
              <Flex align="center" gap={2}>
                <LogOut size={16} />
                ログアウト
              </Flex>
            )}
          </Button>
        </VStack>
      </Box>
    </Box>
  );
}
