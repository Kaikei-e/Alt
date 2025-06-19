import { useEffect, useRef } from "react";
import { throttle } from "@/lib/utils/throttle";

const DEFAULT_THROTTLE_DELAY = 500;
const DEFAULT_MAX_RETRIES = 3;
const DEFAULT_ROOT_MARGIN = "200px 0px";
const DEFAULT_THRESHOLD = 0.1;
const SETUP_RETRY_DELAY = 100;

export function useInfiniteScroll(
  callback: () => void,
  ref: React.RefObject<HTMLDivElement | null>,
  resetKey?: number | string,
  options?: {
    throttleDelay?: number;
    maxRetries?: number;
    rootMargin?: string;
    threshold?: number;
  }
) {
  const callbackRef = useRef(callback);
  const throttledCallbackRef = useRef<(() => void) | null>(null);
  const retryCountRef = useRef(0);
  
  const {
    throttleDelay = DEFAULT_THROTTLE_DELAY,
    maxRetries = DEFAULT_MAX_RETRIES,
    rootMargin = DEFAULT_ROOT_MARGIN,
    threshold = DEFAULT_THRESHOLD,
  } = options || {};

  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    throttledCallbackRef.current = throttle(() => {
      if (retryCountRef.current < maxRetries) {
        callbackRef.current();
      }
    }, throttleDelay);
  }, [resetKey, throttleDelay, maxRetries]);

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
              throttledCallbackRef.current();
              retryCountRef.current = 0;
            }
          });
        },
        {
          rootMargin,
          threshold,
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
  }, [ref, resetKey, rootMargin, threshold]);

  useEffect(() => {
    retryCountRef.current = 0;
  }, [resetKey]);
}
