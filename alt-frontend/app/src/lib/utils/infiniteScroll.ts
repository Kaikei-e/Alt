import { useEffect, useRef } from "react";

// Throttle function to limit callback execution frequency
function throttle<T extends (...args: never[]) => void>(
  func: T,
  delay: number
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
      timeoutId = setTimeout(() => {
        func(...args);
        lastExecTime = Date.now();
      }, delay - (currentTime - lastExecTime));
    }
  };
}

export function useInfiniteScroll(
  callback: () => void,
  ref: React.RefObject<HTMLDivElement | null>,
  resetKey?: number | string
) {
  const callbackRef = useRef(callback);
  const throttledCallbackRef = useRef<(() => void) | null>(null);

  // Keep the callback ref updated
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  // Create throttled callback when resetKey changes
  useEffect(() => {
    throttledCallbackRef.current = throttle(() => {
      callbackRef.current();
    }, 300);
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
            }
          });
        },
        {
          rootMargin: "50px",
          threshold: 0,
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
  }, [ref, resetKey]); // Add resetKey as dependency to force observer reset
}
