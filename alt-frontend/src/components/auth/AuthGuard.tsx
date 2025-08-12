"use client";

import { ReactNode } from 'react';
import {
  Box,
  VStack,
  Text,
  Spinner,
  Button,
  Flex,
} from '@chakra-ui/react';
import { RefreshCw, AlertCircle, Wifi } from 'lucide-react';
import { useAuth } from '@/contexts/auth-context';
import { AuthForm } from './AuthForm';
import { UserProfile } from './UserProfile';

interface AuthGuardProps {
  children?: ReactNode;
  showWhenAuthenticated?: ReactNode;
  showWhenUnauthenticated?: ReactNode;
  fallback?: ReactNode;
  requireAuth?: boolean;
}

export function AuthGuard({
  children,
  showWhenAuthenticated,
  showWhenUnauthenticated,
  fallback,
  requireAuth = false,
}: AuthGuardProps) {
  const { isAuthenticated, isLoading, error, user, refresh, retryLastAction, clearError } = useAuth();

  // Show loading state
  if (isLoading) {
    return (
      fallback || (
        <Box
          display="flex"
          alignItems="center"
          justifyContent="center"
          minH="200px"
          bg="var(--alt-glass)"
          border="1px solid"
          borderColor="var(--alt-glass-border)"
          backdropFilter="blur(12px)"
          borderRadius="lg"
          mx="auto"
          maxW="400px"
          w="full"
        >
          <VStack gap={3}>
            <Spinner
              size="lg"
              color="var(--alt-primary)"
            />
            <Text
              color="var(--text-primary)"
              fontSize="sm"
              fontFamily="body"
            >
              認証状態を確認中...
            </Text>
          </VStack>
        </Box>
      )
    );
  }

  // Show enhanced error state
  if (error && !isAuthenticated) {
    const getErrorIcon = () => {
      switch (error.type) {
        case 'NETWORK_ERROR':
          return <Wifi size={20} />;
        case 'TIMEOUT_ERROR':
          return <RefreshCw size={20} />;
        default:
          return <AlertCircle size={20} />;
      }
    };

    const getErrorColor = () => {
      switch (error.type) {
        case 'NETWORK_ERROR':
        case 'TIMEOUT_ERROR':
          return 'semantic.warning';
        default:
          return 'semantic.error';
      }
    };

    const handleRetryAction = async () => {
      try {
        if (error.isRetryable) {
          await retryLastAction();
        } else {
          await refresh();
        }
      } catch (err) {
        console.error('Retry failed:', err);
      }
    };

    return (
      <Box maxW="400px" w="full" mx="auto">
        <Box
          bg="var(--alt-glass)"
          border="1px solid"
          borderColor={getErrorColor()}
          borderRadius="lg"
          p={4}
          mb={4}
        >
          <VStack gap={3} align="stretch">
            <Flex align="center" gap={2}>
              <Box color={getErrorColor()}>
                {getErrorIcon()}
              </Box>
              <VStack gap={1} align="start" flex={1}>
                <Text color={getErrorColor()} fontSize="sm" fontWeight="semibold" fontFamily="heading">
                  認証エラー
                </Text>
                <Text color={getErrorColor()} fontSize="xs" fontFamily="body">
                  {error.message}
                </Text>
              </VStack>
            </Flex>
            
            <Flex gap={2} justify="stretch">
              <Button
                size="sm"
                variant="outline"
                bg="var(--alt-glass)"
                borderColor={getErrorColor()}
                color={getErrorColor()}
                onClick={handleRetryAction}
                disabled={isLoading}
                flex={1}
                _hover={{
                  bg: getErrorColor(),
                  color: "white",
                }}
              >
                <Flex align="center" gap={1}>
                  <RefreshCw size={12} />
                  <Text fontSize="xs">
                    {error.isRetryable ? '再試行' : '更新'}
                  </Text>
                </Flex>
              </Button>
              
              <Button
                size="sm"
                variant="ghost"
                color="var(--text-muted)"
                onClick={clearError}
                _hover={{
                  bg: "var(--alt-glass)",
                  color: "var(--text-primary)",
                }}
              >
                <Text fontSize="xs">閉じる</Text>
              </Button>
            </Flex>
            
            {error.retryCount !== undefined && error.retryCount > 0 && (
              <Text fontSize="xs" color="var(--text-muted)" textAlign="center" fontFamily="body">
                再試行回数: {error.retryCount}/3
              </Text>
            )}
          </VStack>
        </Box>
        
        {/* Show auth form as fallback when there's an error */}
        {showWhenUnauthenticated || <AuthForm />}
      </Box>
    );
  }

  // User is authenticated
  if (isAuthenticated && user) {
    return (
      <Box>
        {showWhenAuthenticated || <UserProfile />}
        {!requireAuth && children}
      </Box>
    );
  }

  // User is not authenticated
  if (!isAuthenticated) {
    if (requireAuth) {
      return (
        <Box maxW="400px" w="full" mx="auto">
          <VStack gap={4} textAlign="center" mb={6}>
            <Text
              fontSize="lg"
              fontWeight="semibold"
              color="var(--text-primary)"
              fontFamily="heading"
            >
              ログインが必要です
            </Text>
            <Text
              fontSize="sm"
              color="var(--text-muted)"
              fontFamily="body"
            >
              このコンテンツにアクセスするにはログインしてください
            </Text>
          </VStack>
          {showWhenUnauthenticated || <AuthForm />}
        </Box>
      );
    }

    return (
      <Box>
        {showWhenUnauthenticated || <AuthForm />}
        {children}
      </Box>
    );
  }

  // Default fallback
  return <Box>{children}</Box>;
}