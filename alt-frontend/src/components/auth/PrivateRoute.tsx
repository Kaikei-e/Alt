"use client";

import { ReactNode } from 'react';
import {
  Box,
  VStack,
  Text,
  Button,
  Flex,
  Spinner,
} from '@chakra-ui/react';
import { Lock, ArrowLeft, Home } from 'lucide-react';
import { useAuth } from '@/contexts/auth-context';
import { AuthForm } from './AuthForm';

interface PrivateRouteProps {
  children: ReactNode;
  fallback?: ReactNode;
  redirectTo?: string;
  requiredRole?: string;
  showLoginPrompt?: boolean;
  allowPartialAccess?: boolean;
}

export function PrivateRoute({
  children,
  fallback,
  redirectTo,
  requiredRole,
  showLoginPrompt = true,
  allowPartialAccess = false,
}: PrivateRouteProps) {
  const { user, isAuthenticated, isLoading, error } = useAuth();

  // Show loading state
  if (isLoading) {
    return (
      fallback || (
        <Box
          display="flex"
          alignItems="center"
          justifyContent="center"
          minH="400px"
          bg="var(--alt-glass)"
          border="1px solid"
          borderColor="var(--alt-glass-border)"
          backdropFilter="blur(12px)"
          borderRadius="lg"
          mx="auto"
          maxW="600px"
          w="full"
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
            <VStack gap={4}>
              <Spinner
                size="xl"
                color="var(--alt-primary)"
              />
              <Text
                color="var(--text-primary)"
                fontSize="md"
                fontFamily="heading"
                fontWeight="semibold"
              >
                認証状態を確認中...
              </Text>
              <Text
                color="var(--text-muted)"
                fontSize="sm"
                fontFamily="body"
                textAlign="center"
              >
                アクセス権限を確認しています
              </Text>
            </VStack>
          </Box>
        </Box>
      )
    );
  }

  // Check if user is authenticated
  if (!isAuthenticated || !user) {
    if (showLoginPrompt) {
      return (
        <Box maxW="600px" w="full" mx="auto" p={4}>
          <Box
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-glass-border)"
            backdropFilter="blur(12px)"
            borderRadius="lg"
            p={6}
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
              <VStack gap={6} textAlign="center">
                <Box
                  w="4rem"
                  h="4rem"
                  bg="var(--alt-primary)"
                  color="white"
                  borderRadius="full"
                  display="flex"
                  alignItems="center"
                  justifyContent="center"
                  mx="auto"
                >
                  <Lock size={24} />
                </Box>
                
                <VStack gap={2}>
                  <Text
                    fontSize="xl"
                    fontWeight="bold"
                    color="var(--text-primary)"
                    fontFamily="heading"
                  >
                    ログインが必要です
                  </Text>
                  <Text
                    fontSize="sm"
                    color="var(--text-muted)"
                    fontFamily="body"
                    maxW="400px"
                  >
                    このコンテンツにアクセスするには、ログインまたはアカウント作成が必要です
                  </Text>
                </VStack>
                
                {error && (
                  <Box
                    bg="var(--alt-glass)"
                    border="1px solid"
                    borderColor="semantic.error"
                    borderRadius="md"
                    p={3}
                    w="full"
                    maxW="400px"
                  >
                    <Text color="semantic.error" fontSize="sm" fontFamily="body" textAlign="center">
                      {error.message}
                    </Text>
                  </Box>
                )}
              </VStack>
              
              <Box mt={6} maxW="400px" mx="auto">
                <AuthForm onSuccess={() => window.location.reload()} />
              </Box>
              
              <VStack gap={3} mt={6}>
                <Button
                  variant="ghost"
                  w="full"
                  maxW="400px"
                  bg="var(--alt-glass)"
                  border="1px solid"
                  borderColor="var(--alt-glass-border)"
                  color="var(--text-primary)"
                  fontFamily="body"
                  onClick={() => window.history.back()}
                  _hover={{
                    bg: "var(--alt-glass)",
                    borderColor: "var(--alt-primary)",
                    transform: "translateY(-1px)",
                  }}
                >
                  <Flex align="center" gap={2}>
                    <ArrowLeft size={16} />
                    戻る
                  </Flex>
                </Button>
                
                <Button
                  variant="ghost"
                  w="full"
                  maxW="400px"
                  bg="var(--alt-glass)"
                  border="1px solid"
                  borderColor="var(--alt-glass-border)"
                  color="var(--text-primary)"
                  fontFamily="body"
                  onClick={() => window.location.href = '/'}
                  _hover={{
                    bg: "var(--alt-glass)",
                    borderColor: "var(--alt-primary)",
                    transform: "translateY(-1px)",
                  }}
                >
                  <Flex align="center" gap={2}>
                    <Home size={16} />
                    ホームへ
                  </Flex>
                </Button>
              </VStack>
            </Box>
          </Box>
        </Box>
      );
    }
    
    return fallback || null;
  }

  // Check role-based access
  if (requiredRole && user.role !== requiredRole) {
    return (
      <Box maxW="600px" w="full" mx="auto" p={4}>
        <Box
          bg="var(--alt-glass)"
          border="1px solid"
          borderColor="semantic.warning"
          backdropFilter="blur(12px)"
          borderRadius="lg"
          p={6}
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
            <VStack gap={6} textAlign="center">
              <Box
                w="4rem"
                h="4rem"
                bg="semantic.warning"
                color="white"
                borderRadius="full"
                display="flex"
                alignItems="center"
                justifyContent="center"
                mx="auto"
              >
                <Lock size={24} />
              </Box>
              
              <VStack gap={2}>
                <Text
                  fontSize="xl"
                  fontWeight="bold"
                  color="var(--text-primary)"
                  fontFamily="heading"
                >
                  アクセス権限がありません
                </Text>
                <Text
                  fontSize="sm"
                  color="var(--text-muted)"
                  fontFamily="body"
                  maxW="400px"
                >
                  このコンテンツにアクセスするには「{requiredRole}」権限が必要です
                </Text>
                <Text
                  fontSize="xs"
                  color="var(--text-muted)"
                  fontFamily="body"
                >
                  現在の権限: {user.role || '未設定'}
                </Text>
              </VStack>
              
              <VStack gap={3} w="full" maxW="400px">
                <Button
                  variant="outline"
                  w="full"
                  bg="var(--alt-glass)"
                  border="1px solid"
                  borderColor="var(--alt-glass-border)"
                  color="var(--text-primary)"
                  fontFamily="body"
                  onClick={() => window.history.back()}
                  _hover={{
                    bg: "var(--alt-glass)",
                    borderColor: "var(--alt-primary)",
                    transform: "translateY(-1px)",
                  }}
                >
                  <Flex align="center" gap={2}>
                    <ArrowLeft size={16} />
                    戻る
                  </Flex>
                </Button>
                
                <Button
                  variant="outline"
                  w="full"
                  bg="var(--alt-glass)"
                  border="1px solid"
                  borderColor="var(--alt-glass-border)"
                  color="var(--text-primary)"
                  fontFamily="body"
                  onClick={() => window.location.href = '/'}
                  _hover={{
                    bg: "var(--alt-glass)",
                    borderColor: "var(--alt-primary)",
                    transform: "translateY(-1px)",
                  }}
                >
                  <Flex align="center" gap={2}>
                    <Home size={16} />
                    ホームへ
                  </Flex>
                </Button>
              </VStack>
            </VStack>
          </Box>
        </Box>
      </Box>
    );
  }

  // User is authenticated and has required permissions
  return <>{children}</>;
}

// Higher-order component for wrapping pages
export function withPrivateRoute<P extends object>(
  WrappedComponent: React.ComponentType<P>,
  options?: Omit<PrivateRouteProps, 'children'>
) {
  return function PrivateRouteWrapper(props: P) {
    return (
      <PrivateRoute {...options}>
        <WrappedComponent {...props} />
      </PrivateRoute>
    );
  };
}

// Hook for checking authentication status in components
export function usePrivateRoute(requiredRole?: string) {
  const { user, isAuthenticated, isLoading } = useAuth();
  
  const hasAccess = isAuthenticated && 
    user && 
    (!requiredRole || user.role === requiredRole);
  
  return {
    hasAccess,
    isAuthenticated,
    isLoading,
    user,
    userRole: user?.role,
    isAuthorized: hasAccess,
    canAccess: (role?: string) => {
      if (!isAuthenticated || !user) return false;
      if (!role) return true;
      return user.role === role;
    }
  };
}