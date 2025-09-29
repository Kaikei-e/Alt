"use client";

import {
  Box,
  Flex,
  Stack,
  Text,
} from "@chakra-ui/react";
import { useOryFlow } from "@/lib/ory/use-flow";
import { OryFlowForm } from "@/components/ory/OryFlowForm";

interface RegisterClientProps {
  flowId?: string;
  returnUrl?: string;
}

export default function RegisterClient({ flowId, returnUrl }: RegisterClientProps) {
  const { flow, isLoading, isSubmitting, error, handleSubmit } = useOryFlow({
    type: "registration",
    flowId,
    returnTo: returnUrl,
  });

  return (
    <Box
      minH="100vh"
      bg="var(--alt-glass-bg)"
      bgImage="radial-gradient(circle at 25% 25%, var(--alt-glass) 0%, transparent 70%), radial-gradient(circle at 75% 75%, var(--alt-primary-alpha) 0%, transparent 70%)"
      position="relative"
      overflow="hidden"
    >
      <Box
        position="absolute"
        inset={0}
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
        <Stack gap={8} w="full" maxW="420px">
          <Stack gap={4} textAlign="center">
            <Text
              fontSize="2xl"
              fontWeight="bold"
              fontFamily="heading"
              color="var(--alt-primary)"
              textShadow="0 2px 4px rgba(0,0,0,0.1)"
            >
              Alt
            </Text>
            <Text fontSize="lg" fontWeight="semibold" color="var(--text-primary)">
              新規登録
            </Text>
          </Stack>

          <Box
            w="full"
            p={6}
            bg="var(--alt-glass)"
            border="1px solid"
            borderColor="var(--alt-glass-border)"
            borderRadius="lg"
            backdropFilter="blur(12px)"
          >
            <OryFlowForm
              flow={flow}
              isLoading={isLoading}
              isSubmitting={isSubmitting}
              error={error}
              onSubmit={handleSubmit}
            />
          </Box>
        </Stack>
      </Flex>
    </Box>
  );
}
