'use client'

import { useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Box, VStack, Text, Flex } from '@chakra-ui/react'
import { useAuth } from '@/contexts/auth-context'
import { AuthForm } from '@/components/auth/AuthForm'

export default function LoginPage() {
  const { isAuthenticated } = useAuth()
  const router = useRouter()
  const searchParams = useSearchParams()
  
  // Get return URL from query parameters
  const returnUrl = searchParams.get('returnUrl') || '/'

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated) {
      router.push(returnUrl)
    }
  }, [isAuthenticated, router, returnUrl])

  const handleLoginSuccess = () => {
    // On successful login, redirect to return URL
    router.push(returnUrl)
  }

  if (isAuthenticated) {
    return (
      <Flex
        minH="100vh"
        align="center"
        justify="center"
        bg="var(--alt-glass-bg)"
      >
        <Text color="var(--text-primary)" fontFamily="body">
          リダイレクト中...
        </Text>
      </Flex>
    )
  }

  return (
    <Box
      minH="100vh"
      bg="var(--alt-glass-bg)"
      bgImage="radial-gradient(circle at 25% 25%, var(--alt-glass) 0%, transparent 70%), radial-gradient(circle at 75% 75%, var(--alt-primary-alpha) 0%, transparent 70%)"
      position="relative"
      overflow="hidden"
    >
      {/* Background Pattern */}
      <Box
        position="absolute"
        top="0"
        left="0"
        right="0"
        bottom="0"
        bgImage="url('data:image/svg+xml;charset=utf-8,%3Csvg width=%2760%27 height=%2760%27 viewBox=%270 0 60 60%27 xmlns=%27http://www.w3.org/2000/svg%27%3E%3Cg fill=%27none%27 fill-rule=%27evenodd%27%3E%3Cg fill=%27%23ffffff%27 fill-opacity=%270.03%27%3E%3Ccircle cx=%2730%27 cy=%2730%27 r=%271%27/%3E%3C/g%3E%3C/svg%3E')"
        pointerEvents="none"
      />

      <Flex
        minH="100vh"
        align="center"
        justify="center"
        p={4}
        position="relative"
        zIndex={1}
      >
        <VStack gap={8} w="full" maxW="400px">
          {/* Header */}
          <VStack gap={4} textAlign="center">
            <Text
              fontSize="2xl"
              fontWeight="bold"
              fontFamily="heading"
              color="var(--alt-primary)"
              textShadow="0 2px 4px rgba(0,0,0,0.1)"
            >
              Alt Reader
            </Text>
            
            <VStack gap={2}>
              <Text
                fontSize="lg"
                fontWeight="semibold"
                fontFamily="heading"
                color="var(--text-primary)"
              >
                ログイン
              </Text>
              
              {returnUrl !== '/' && (
                <Box
                  bg="var(--alt-glass)"
                  border="1px solid"
                  borderColor="var(--alt-glass-border)"
                  borderRadius="md"
                  px={3}
                  py={2}
                  backdropFilter="blur(8px)"
                >
                  <Text
                    fontSize="sm"
                    color="var(--text-muted)"
                    fontFamily="body"
                    textAlign="center"
                  >
                    続行するにはログインが必要です
                  </Text>
                </Box>
              )}
            </VStack>
          </VStack>

          {/* Login Form */}
          <AuthForm
            initialMode="login"
            onSuccess={handleLoginSuccess}
          />

          {/* Footer */}
          <Box textAlign="center">
            <Text
              fontSize="xs"
              color="var(--text-muted)"
              fontFamily="body"
              opacity={0.8}
            >
              © 2025 Alt Reader. All rights reserved.
            </Text>
          </Box>
        </VStack>
      </Flex>
    </Box>
  )
}