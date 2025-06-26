"use client";

import { Box } from "@chakra-ui/react";
import { useEffect, useState } from "react";

interface VaporwaveBackgroundProps {
  /** Control visibility of the background */
  isVisible?: boolean;
  /** Animation speed multiplier (0-3) */
  animationSpeed?: number;
  /** Enable VHS scan lines and noise effects */
  enableVHS?: boolean;
  /** Enable floating geometric shapes */
  enableGeometry?: boolean;
  /** Enable animated gradient layers */
  enableGradients?: boolean;
  /** Z-index for layering */
  zIndex?: number;
}

export function VaporwaveBackground({
  isVisible = true,
  animationSpeed = 1,
  enableVHS = true,
  enableGeometry = true,
  enableGradients = true,
  zIndex = -1,
}: VaporwaveBackgroundProps) {
  const [reducedMotion, setReducedMotion] = useState(false);

  // Detect user's motion preference
  useEffect(() => {
    const mediaQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReducedMotion(mediaQuery.matches);

    const handleChange = (e: MediaQueryListEvent) => {
      setReducedMotion(e.matches);
    };

    mediaQuery.addEventListener("change", handleChange);
    return () => mediaQuery.removeEventListener("change", handleChange);
  }, []);

  // Clamp animation speed
  const clampedSpeed = Math.max(0, Math.min(3, animationSpeed));
  const animationDuration =
    clampedSpeed > 0 ? `${60 / clampedSpeed}s` : "infinite";

  // Disable effects if reduced motion is preferred
  const effectsEnabled = !reducedMotion;
  const showGradients = enableGradients && effectsEnabled;
  const showGeometry = enableGeometry && effectsEnabled;
  const showVHS = enableVHS && effectsEnabled;

  return (
    <Box
      position="fixed"
      top="0"
      left="0"
      right="0"
      bottom="0"
      overflow="hidden"
      pointerEvents="none"
      zIndex={zIndex}
      opacity={isVisible ? 1 : 0}
      transition="opacity 0.5s ease"
      style={{
        willChange: reducedMotion ? "auto" : "transform, opacity",
      }}
    >
      {/* Animated Gradient Layers */}
      {showGradients && (
        <>
          {/* Primary gradient layer */}
          <Box
            position="absolute"
            top="-50%"
            left="-50%"
            right="-50%"
            bottom="-50%"
            background="radial-gradient(circle at 20% 50%, rgba(131, 56, 236, 0.3) 0%, transparent 50%)"
            style={{
              animation: reducedMotion
                ? "none"
                : `vaporwaveFloat1 ${animationDuration} ease-in-out infinite`,
              transform: reducedMotion ? "none" : "translateZ(0)",
            }}
          />

          {/* Secondary gradient layer */}
          <Box
            position="absolute"
            top="-50%"
            left="-50%"
            right="-50%"
            bottom="-50%"
            background="radial-gradient(circle at 80% 20%, rgba(255, 0, 110, 0.2) 0%, transparent 50%)"
            style={{
              animation: reducedMotion
                ? "none"
                : `vaporwaveFloat2 ${animationDuration} ease-in-out infinite reverse`,
              transform: reducedMotion ? "none" : "translateZ(0)",
              animationDelay: "1s",
            }}
          />

          {/* Tertiary gradient layer */}
          <Box
            position="absolute"
            top="-50%"
            left="-50%"
            right="-50%"
            bottom="-50%"
            background="radial-gradient(circle at 60% 80%, rgba(58, 134, 255, 0.25) 0%, transparent 50%)"
            style={{
              animation: reducedMotion
                ? "none"
                : `vaporwaveFloat3 ${animationDuration} ease-in-out infinite`,
              transform: reducedMotion ? "none" : "translateZ(0)",
              animationDelay: "2s",
            }}
          />
        </>
      )}

      {/* Geometric Shapes */}
      {showGeometry && (
        <Box
          position="absolute"
          top="0"
          left="0"
          right="0"
          bottom="0"
          style={{
            background: `
              linear-gradient(45deg, transparent 40%, rgba(131, 56, 236, 0.03) 50%, transparent 60%),
              linear-gradient(-45deg, transparent 40%, rgba(255, 0, 110, 0.03) 50%, transparent 60%)
            `,
            backgroundSize: "60px 60px, 80px 80px",
            animation: reducedMotion
              ? "none"
              : `geometryMove ${animationDuration} linear infinite`,
          }}
        />
      )}

      {/* VHS Effects */}
      {showVHS && (
        <>
          {/* Scan lines */}
          <Box
            position="absolute"
            top="0"
            left="0"
            right="0"
            bottom="0"
            background="repeating-linear-gradient(0deg, transparent 0px, rgba(255, 255, 255, 0.02) 1px, transparent 2px, transparent 4px)"
            style={{
              animation: reducedMotion
                ? "none"
                : `scanLines ${animationDuration} linear infinite`,
            }}
          />

          {/* Noise texture */}
          <Box
            position="absolute"
            top="0"
            left="0"
            right="0"
            bottom="0"
            opacity="0.05"
            style={{
              background: `
                radial-gradient(circle at 25% 25%, #fff 1px, transparent 0),
                radial-gradient(circle at 75% 75%, #fff 1px, transparent 0)
              `,
              backgroundSize: "4px 4px, 6px 6px",
              animation: reducedMotion ? "none" : `noiseFlicker 0.5s infinite`,
            }}
          />
        </>
      )}

      {/* CSS Keyframes */}
      <style jsx>{`
        @keyframes vaporwaveFloat1 {
          0%,
          100% {
            transform: translate(0, 0) rotate(0deg);
          }
          33% {
            transform: translate(30px, -30px) rotate(1deg);
          }
          66% {
            transform: translate(-20px, 20px) rotate(-1deg);
          }
        }

        @keyframes vaporwaveFloat2 {
          0%,
          100% {
            transform: translate(0, 0) rotate(0deg);
          }
          50% {
            transform: translate(-40px, 30px) rotate(2deg);
          }
        }

        @keyframes vaporwaveFloat3 {
          0%,
          100% {
            transform: translate(0, 0) rotate(0deg);
          }
          25% {
            transform: translate(20px, 40px) rotate(-1deg);
          }
          75% {
            transform: translate(-30px, -20px) rotate(1deg);
          }
        }

        @keyframes geometryMove {
          0% {
            background-position:
              0px 0px,
              0px 0px;
          }
          100% {
            background-position:
              60px 60px,
              -80px 80px;
          }
        }

        @keyframes scanLines {
          0% {
            transform: translateY(0);
          }
          100% {
            transform: translateY(4px);
          }
        }

        @keyframes noiseFlicker {
          0%,
          100% {
            opacity: 0.05;
          }
          50% {
            opacity: 0.03;
          }
        }
      `}</style>
    </Box>
  );
}

export default VaporwaveBackground;
