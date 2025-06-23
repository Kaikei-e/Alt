"use client";

import { Box } from "@chakra-ui/react";
import { useEffect } from "react";

interface SSEProgressBarProps {
  /** Progress value between 0 and 100 */
  progress: number;
  /** Whether the progress bar should be visible */
  isVisible: boolean;
  /** Callback when progress reaches 100% */
  onComplete?: () => void;
}

export const SSEProgressBar = ({
  progress,
  isVisible,
  onComplete,
}: SSEProgressBarProps) => {
  useEffect(() => {
    if (progress >= 100 && onComplete) {
      onComplete();
    }
  }, [progress, onComplete]);

  if (!isVisible) return null;

  return (
    <Box
      position="fixed"
      top={0}
      left={0}
      right={0}
      height="2px"
      bg="blackAlpha.300"
      zIndex={1000}
    >
      <Box
        height="100%"
        bg="linear-gradient(90deg, var(--vaporwave-pink), var(--vaporwave-purple))"
        width={`${progress}%`}
        transition="width 100ms linear"
      />
    </Box>
  );
};

export default SSEProgressBar;