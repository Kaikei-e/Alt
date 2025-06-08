import { Flex, Text, Button } from "@chakra-ui/react";

type ErrorStateProps = {
  error: string | null;
  onRetry: () => void;
  isLoading: boolean;
};

export default function ErrorState({
  error,
  onRetry,
  isLoading,
}: ErrorStateProps) {
  return (
    <Flex
      flexDirection="column"
      justifyContent="center"
      alignItems="center"
      height="100vh"
      width="100%"
      p={4}
    >
      <Text
        fontSize="lg"
        fontWeight="bold"
        color="red.500"
        mb={4}
        textAlign="center"
      >
        Failed to load feeds
      </Text>
      <Text color="gray.600" mb={6} textAlign="center" maxWidth="md">
        {error}
      </Text>
      <Button colorScheme="indigo" onClick={onRetry} disabled={isLoading}>
        {isLoading ? "Retrying..." : "Retry"}
      </Button>
    </Flex>
  );
}
