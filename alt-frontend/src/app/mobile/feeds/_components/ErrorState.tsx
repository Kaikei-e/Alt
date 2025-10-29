import { Flex, Text, Button, Box, Icon } from "@chakra-ui/react";
import { useState } from "react";
import { AlertTriangle, WifiOff, Server, Clock } from "lucide-react";

type ErrorStateProps = {
  error: Error | null;
  onRetry: () => Promise<void>;
  isLoading: boolean;
};

const getErrorDetails = (
  error: Error | null,
): { title: string; message: string; icon: React.ElementType } => {
  if (!error) {
    return {
      title: "An Unknown Error Occurred",
      message: "Something went wrong. Please try again.",
      icon: AlertTriangle,
    };
  }

  const errorMessage = (error.message || String(error)).toLowerCase();

  if (
    errorMessage.includes("network") ||
    errorMessage.includes("fetch") ||
    errorMessage.includes("connection")
  ) {
    return {
      title: "Network Error",
      message: "Please check your internet connection and try again.",
      icon: WifiOff,
    };
  }
  if (
    errorMessage.includes("timeout") ||
    error.name === "AbortError" ||
    errorMessage.includes("408")
  ) {
    return {
      title: "Request Timeout",
      message: "The request took too long to complete. Please try again.",
      icon: Clock,
    };
  }
  if (
    errorMessage.includes("429") ||
    errorMessage.includes("rate limit") ||
    errorMessage.includes("too many requests")
  ) {
    return {
      title: "Rate Limit Exceeded",
      message:
        "You're making requests too quickly. Please wait a moment and try again.",
      icon: Clock,
    };
  }
  if (
    errorMessage.includes("server error") ||
    errorMessage.includes("500") ||
    errorMessage.includes("503") ||
    errorMessage.includes("502") ||
    errorMessage.includes("bad gateway")
  ) {
    return {
      title: "Server Error",
      message: "We're having some trouble on our end. Please try again later.",
      icon: Server,
    };
  }
  if (
    error instanceof TypeError ||
    errorMessage.includes("null is not an object")
  ) {
    return {
      title: "Application Error",
      message:
        "An unexpected application error occurred. We've been notified and are looking into it.",
      icon: AlertTriangle,
    };
  }

  return {
    title: "Unable to Load Feeds",
    message: error.message,
    icon: AlertTriangle,
  };
};

export default function ErrorState({
  error,
  onRetry,
  isLoading,
}: ErrorStateProps) {
  const [isRetrying, setIsRetrying] = useState(false);
  const { title, message, icon } = getErrorDetails(error);

  const handleRetry = async () => {
    if (isRetrying || isLoading) return;
    setIsRetrying(true);
    try {
      await onRetry();
    } catch (e) {
      console.error("Retry attempt failed", e);
    } finally {
      setIsRetrying(false);
    }
  };

  return (
    <Flex
      direction="column"
      justify="center"
      align="center"
      minH="100dvh"
      bg="var(--app-bg)"
      p={4}
      textAlign="center"
    >
      <Box
        p={{ base: 6, md: 8 }}
        borderRadius="24px"
        maxW="420px"
        w="100%"
        boxShadow="0 10px 30px -15px rgba(0, 0, 0, 0.3)"
        border="1px solid"
        borderColor="whiteAlpha.200"
        bg="whiteAlpha.50"
        backdropFilter="blur(10px)"
      >
        <Flex justify="center" align="center" mb={5}>
          <Icon as={icon} boxSize="40px" color="red.500" />
        </Flex>

        <Text
          fontSize={{ base: "xl", md: "2xl" }}
          fontWeight="bold"
          color="white"
          mb={3}
        >
          {title}
        </Text>

        <Text
          color="whiteAlpha.800"
          mb={8}
          fontSize="md"
          lineHeight="1.6"
        >
          {message}
        </Text>

        <Button
          onClick={handleRetry}
          loading={isRetrying || isLoading}
          loadingText="再試行中..."
          size="lg"
          w="100%"
          borderRadius="16px"
          bgGradient="linear(to-r, #FF416C, #FF4B2B)"
          color="white"
          fontWeight="bold"
          _hover={{
            transform: "translateY(-2px)",
            boxShadow: "lg",
          }}
          _active={{
            transform: "translateY(0)",
          }}
          _disabled={{
            bg: "gray.600",
            opacity: 0.7,
            cursor: "not-allowed",
          }}
          transition="all 0.2s"
        >
          再試行
        </Button>

        {process.env.NODE_ENV === "development" && error && (
          <Box mt={4} p={3} bg="blackAlpha.300" borderRadius="8px">
            <Text fontSize="xs" color="whiteAlpha.700" textAlign="left">
              デバッグ情報:
            </Text>
            <Text
              fontSize="xs"
              color="whiteAlpha.700"
              textAlign="left"
              mt={2}
              fontFamily="monospace"
            >
              {error.message}
            </Text>
          </Box>
        )}
      </Box>
    </Flex>
  );
}
