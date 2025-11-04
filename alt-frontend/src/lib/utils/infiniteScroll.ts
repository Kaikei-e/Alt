import { useEffect, useRef } from "react";
import { throttle } from "@/lib/utils/throttle";

const DEFAULT_THROTTLE_DELAY = 100; // Reduced for better responsiveness
const DEFAULT_ROOT_MARGIN = "100px 0px"; // Reduced for faster triggering
const DEFAULT_THRESHOLD = 0.1;
const SETUP_RETRY_DELAY = 50; // Faster retry

export function useInfiniteScroll(
  callback: () => void,
  ref: React.RefObject<HTMLDivElement | null>,
  resetKey?: number | string,
  options?: {
    throttleDelay?: number;
    rootMargin?: string;
    threshold?: number;
  }
) {
  const callbackRef = useRef(callback);
  const throttledCallbackRef = useRef<(() => void) | null>(null);
  const retryCountRef = useRef(0);

  const {
    throttleDelay = DEFAULT_THROTTLE_DELAY,
    rootMargin = DEFAULT_ROOT_MARGIN,
    threshold = DEFAULT_THRESHOLD,
  } = options || {};

  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    throttledCallbackRef.current = throttle(() => {
      callbackRef.current();
    }, throttleDelay);
  }, [resetKey, throttleDelay]);

  useEffect(() => {
    let observer: IntersectionObserver | null = null;
    let timeoutId: NodeJS.Timeout | null = null;

    const setupObserver = () => {
      const element = ref.current;

      if (!element || !throttledCallbackRef.current) {
        timeoutId = setTimeout(setupObserver, SETUP_RETRY_DELAY);
        return;
      }

      observer = new IntersectionObserver(
        (entries) => {
          entries.forEach((entry) => {
            if (entry.isIntersecting && throttledCallbackRef.current) {
              // Add small delay to ensure DOM is ready
              setTimeout(() => {
                if (throttledCallbackRef.current) {
                  throttledCallbackRef.current();
                  retryCountRef.current = 0;
                }
              }, 10);
            }
          });
        },
        {
          rootMargin,
          threshold,
        }
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
  }, [ref, resetKey, rootMargin, threshold]);

  useEffect(() => {
    retryCountRef.current = 0;
  }, [resetKey]);
}
