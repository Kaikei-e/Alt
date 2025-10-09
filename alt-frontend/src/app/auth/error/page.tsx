'use client';

import { Box, Button, Container, Heading, Link, Text, VStack } from '@chakra-ui/react';
import NextLink from 'next/link';
import { useSearchParams } from 'next/navigation';
import { Suspense } from 'react';

function AuthErrorContent() {
  const searchParams = useSearchParams();
  const error = searchParams.get('error') || 'unknown';
  const message = searchParams.get('message') || 'ログインに失敗しました。もう一度お試しください。';

  const getErrorTitle = () => {
    switch (error) {
      case 'auth_failed':
        return '認証に失敗しました';
      case 'session_expired':
        return 'セッションの有効期限が切れました';
      case 'invalid_token':
        return '無効なトークンです';
      default:
        return '認証エラー';
    }
  };

  return (
    <Container
      as="main"
      role="main"
      aria-labelledby="auth-error-heading"
      maxW="container.md"
      py={12}
    >
      <VStack gap={6} align="stretch">
        <Box>
          <Heading id="auth-error-heading" as="h1" size="lg" mb={4}>
            {getErrorTitle()}
          </Heading>

          <Box
            role="alert"
            data-testid="error-message"
            bg="red.50"
            border="1px solid"
            borderColor="red.200"
            borderRadius="md"
            p={4}
            mb={4}
          >
            <Text color="red.800">{message}</Text>
          </Box>

          {error && (
            <Text fontSize="sm" color="gray.600" data-testid="error-details">
              エラーコード: {error}
            </Text>
          )}
        </Box>

        <VStack gap={3} align="stretch">
          <Link
            as={NextLink}
            href="/auth/login"
            data-testid="back-to-login-button"
            textDecoration="none"
            _hover={{ textDecoration: 'none' }}
          >
            <Button colorScheme="blue" size="lg" width="full">
              ログインし直す
            </Button>
          </Link>

          <Link
            as={NextLink}
            href="/public/landing"
            textAlign="center"
            color="blue.600"
            fontSize="sm"
          >
            ホームに戻る
          </Link>
        </VStack>
      </VStack>
    </Container>
  );
}

export default function Page() {
  return (
    <Suspense fallback={<Box p={6}>読み込み中...</Box>}>
      <AuthErrorContent />
    </Suspense>
  );
}
