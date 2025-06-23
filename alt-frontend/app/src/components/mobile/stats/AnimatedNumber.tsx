"use client";

import { Text, Box, TextProps } from "@chakra-ui/react";
import { useEffect, useState } from "react";

interface AnimatedNumberProps {
  /** The target number to display */
  value: number;
  /** Animation duration in milliseconds */
  duration?: number;
  /** Number format options */
  formatOptions?: Intl.NumberFormatOptions;
  /** Custom styles for the text */
  textProps?: Omit<TextProps, 'children'>;
  /** Callback when animation completes */
  onComplete?: () => void;
}

export function AnimatedNumber({
  value,
  duration = 300,
  formatOptions,
  textProps = {},
  onComplete,
}: AnimatedNumberProps) {
  const [displayValue, setDisplayValue] = useState(value);
  const [isAnimating, setIsAnimating] = useState(false);

  useEffect(() => {
    if (displayValue === value) return;

    // Check for reduced motion preference
    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    if (reducedMotion) {
      // Skip animation for accessibility
      setDisplayValue(value);
      onComplete?.();
      return;
    }

    setIsAnimating(true);
    const startValue = displayValue;
    const difference = value - startValue;
    const startTime = Date.now();

    const animate = () => {
      const elapsed = Date.now() - startTime;
      const progress = Math.min(elapsed / duration, 1);

      // Easing function (ease-out cubic)
      const easedProgress = 1 - Math.pow(1 - progress, 3);

      const currentValue = startValue + (difference * easedProgress);
      setDisplayValue(Math.round(currentValue));

      if (progress < 1) {
        requestAnimationFrame(animate);
      } else {
        setDisplayValue(value);
        setIsAnimating(false);
        onComplete?.();
      }
    };

    const animationFrame = requestAnimationFrame(animate);

    return () => {
      cancelAnimationFrame(animationFrame);
      setIsAnimating(false);
    };
  }, [value, duration, onComplete, displayValue]);

  const formattedValue = formatOptions
    ? new Intl.NumberFormat('en-US', formatOptions).format(displayValue)
    : displayValue.toString();

  return (
    <>
      <Text
        fontFamily="monospace"
        fontWeight="bold"
        textShadow="0 0 10px var(--vaporwave-pink)"
        color="white"
        transition={isAnimating ? "none" : "color 0.2s ease"}
        style={{
          willChange: isAnimating ? "transform" : "auto",
        }}
        aria-live="polite"
        aria-atomic="true"
        {...textProps}
      >
        {formattedValue}
      </Text>

      {/* Hidden element for screen readers to announce changes */}
      <Box
        position="absolute"
        left="-10000px"
        width="1px"
        height="1px"
        overflow="hidden"
        aria-live="assertive"
        aria-atomic="true"
      >
        {isAnimating ? `Updating value to ${formattedValue}` : `Current value: ${formattedValue}`}
      </Box>
    </>
  );
}

export default AnimatedNumber;