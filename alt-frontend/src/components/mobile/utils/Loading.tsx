import { Box } from "@chakra-ui/react";
import { CircularProgress } from "@chakra-ui/progress";

interface LoadingProps {
  isLoading: boolean;
}

const Loading = ({ isLoading }: LoadingProps) => {
  if (!isLoading) return null;

  return (
    <Box
      display="flex"
      justifyContent="center"
      alignItems="center"
      w="stretch"
      h="stretch"
    >
      <CircularProgress isIndeterminate color="var(--alt-primary)" />
    </Box>
  );
};

export default Loading;
