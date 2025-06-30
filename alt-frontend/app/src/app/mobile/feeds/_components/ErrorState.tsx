import { Flex, Text, Button, Box } from "@chakra-ui/react";
import { useCallback, useState, useRef } from "react";

type ErrorStateProps = {
  error: Error | null;
  onRetry: () => void;
  isLoading: boolean;
};

export default function ErrorState({
  error,
  onRetry,
  isLoading,
}: ErrorStateProps) {
  const [retryCount, setRetryCount] = useState(0);
  const [isRetrying, setIsRetrying] = useState(false);
  const retryTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  // Enhanced error message mapping
  const getErrorMessage = (error: Error | null) => {
    if (!error) return "An unexpected error occurred";

    // Check for status codes in error messages (format: "API request failed: 429 Too Many Requests")
    const errorMessage = error.message || String(error);
    if (
      errorMessage.includes("429") ||
      errorMessage.toLowerCase().includes("rate limit") ||
      errorMessage.toLowerCase().includes("too many requests")
    ) {
      return "Rate limit exceeded";
    }
    if (
      errorMessage.includes("500") ||
      errorMessage.toLowerCase().includes("server error") ||
      errorMessage.toLowerCase().includes("internal server error")
    ) {
      return "Server error - please try again later";
    }
    if (
      errorMessage.includes("503") ||
      errorMessage.toLowerCase().includes("service unavailable")
    ) {
      return "Service temporarily unavailable";
    }
    if (
      errorMessage.includes("502") ||
      errorMessage.toLowerCase().includes("bad gateway")
    ) {
      return "Service temporarily unavailable";
    }
    if (
      errorMessage.toLowerCase().includes("network") ||
      errorMessage.toLowerCase().includes("fetch") ||
      errorMessage.toLowerCase().includes("connection")
    ) {
      return "Network connection error";
    }
    if (
      errorMessage.toLowerCase().includes("timeout") ||
      errorMessage.includes("408")
    ) {
      return "Request timeout - please try again";
    }

    return errorMessage;
  };

  // Exponential backoff retry logic with automatic retries
  const handleRetryWithBackoff = useCallback(async () => {
    if (retryTimeoutRef.current) {
      clearTimeout(retryTimeoutRef.current);
    }

    setIsRetrying(true);

    const maxRetries = 3;
    let currentRetry = 0;

    const attemptRetry = async (): Promise<void> => {
      currentRetry++;
      setRetryCount(currentRetry);

      try {
        await onRetry();
        // If successful, reset and return
        setIsRetrying(false);
        setRetryCount(0);
        return;
      } catch (err) {
        // If failed and we haven't exceeded max retries, try again
        if (currentRetry < maxRetries) {
          const delay =
            process.env.NODE_ENV === "test"
              ? 100
              : Math.min(1000 * Math.pow(2, currentRetry - 1), 8000);

          await new Promise((resolve) => setTimeout(resolve, delay));
          return attemptRetry();
        } else {
          // All retries exhausted
          setIsRetrying(false);
          throw err;
        }
      }
    };

    try {
      await attemptRetry();
    } catch (err) {
      console.error("All retry attempts failed:", err);
    }
  }, [onRetry]);

  const errorMessage = getErrorMessage(error);
  const showDetailedError = errorMessage !== "An unexpected error occurred";

  return (
    <Flex
      flexDirection="column"
      justifyContent="center"
      alignItems="center"
      height="100vh"
      width="100%"
      p={6}
    >
      <Box
        className="glass"
        p={8}
        borderRadius="18px"
        textAlign="center"
        maxWidth="400px"
        width="100%"
      >
        <Text fontSize="xl" fontWeight="bold" color="#e53935" mb={4}>
          Unable to Load Feeds
        </Text>

        {showDetailedError && (
          <Text
            color="rgba(255, 255, 255, 0.8)"
            mb={6}
            fontSize="md"
            lineHeight="1.5"
          >
            {errorMessage}
          </Text>
        )}

        <Button
          onClick={handleRetryWithBackoff}
          disabled={isLoading || isRetrying}
          size="md"
          borderRadius="full"
          bg="linear-gradient(45deg, #ff006e, #8338ec)"
          color="white"
          fontWeight="bold"
          px={6}
          _hover={{
            bg: "linear-gradient(45deg, #e6005c, #7129d4)",
            transform: "translateY(-1px)",
          }}
          _active={{
            transform: "translateY(0px)",
          }}
          _disabled={{
            opacity: 0.6,
          }}
          transition="all 0.2s ease"
          border="1px solid rgba(255, 255, 255, 0.2)"
        >
          {isRetrying
            ? "Retrying..."
            : isLoading
              ? "Retrying..."
              : retryCount > 0
                ? `Retry (${retryCount + 1})`
                : "Retry"}
        </Button>

        {retryCount > 0 && (
          <Text color="rgba(255, 255, 255, 0.6)" fontSize="sm" mt={4}>
            Using exponential backoff (attempt {retryCount + 1})
          </Text>
        )}
      </Box>
    </Flex>
  );
}
