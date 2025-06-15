import { useEffect, useRef } from "react";

// Throttle function to limit callback execution frequency
function throttle<T extends (...args: never[]) => void>(
  func: T,
  delay: number,
): (...args: Parameters<T>) => void {
  let timeoutId: NodeJS.Timeout | null = null;
  let lastExecTime = 0;

  return (...args: Parameters<T>) => {
    const currentTime = Date.now();

    if (currentTime - lastExecTime > delay) {
      func(...args);
      lastExecTime = currentTime;
    } else {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
      timeoutId = setTimeout(
        () => {
          func(...args);
          lastExecTime = Date.now();
        },
        delay - (currentTime - lastExecTime),
      );
    }
  };
}

export function useInfiniteScroll(
  callback: () => void,
  ref: React.RefObject<HTMLDivElement | null>,
  resetKey?: number | string,
) {
  const callbackRef = useRef(callback);
  const throttledCallbackRef = useRef<(() => void) | null>(null);
  const retryCountRef = useRef(0);
  const maxRetries = 3;

  // Keep the callback ref updated
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  // Create throttled callback when resetKey changes
  useEffect(() => {
    throttledCallbackRef.current = throttle(() => {
      if (retryCountRef.current < maxRetries) {
        callbackRef.current();
      }
    }, 500); // Increased throttle time for better mobile performance
  }, [resetKey]);

  useEffect(() => {
    let observer: IntersectionObserver | null = null;
    let timeoutId: NodeJS.Timeout | null = null;

    const setupObserver = () => {
      const element = ref.current;

      if (!element || !throttledCallbackRef.current) {
        timeoutId = setTimeout(setupObserver, 100);
        return;
      }

      observer = new IntersectionObserver(
        (entries) => {
          entries.forEach((entry) => {
            if (entry.isIntersecting && throttledCallbackRef.current) {
              throttledCallbackRef.current();
              retryCountRef.current = 0; // Reset retry count on successful intersection
            }
          });
        },
        {
          rootMargin: "200px 0px", // Increased rootMargin for better mobile detection
          threshold: 0.1, // Added threshold for better detection
        },
      );

      observer.observe(element);
    };

    // Start the setup process
    setupObserver();

    return () => {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
      if (observer) {
        observer.disconnect();
      }
    };
  }, [ref, resetKey]);

  // Reset retry count when resetKey changes
  useEffect(() => {
    retryCountRef.current = 0;
  }, [resetKey]);
}
