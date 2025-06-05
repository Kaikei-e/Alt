import { useCallback, useEffect, useRef } from "react";

export function useInfiniteScroll(
  callback: () => void,
  ref: React.RefObject<HTMLDivElement | null>,
) {
  const callbackRef = useRef(callback);

  // Keep the callback ref updated
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  // Create a stable callback that uses the ref
  const stableCallback = useCallback(() => {
    callbackRef.current();
  }, []);

  useEffect(() => {
    let observer: IntersectionObserver | null = null;
    let timeoutId: NodeJS.Timeout | null = null;

    const setupObserver = () => {
      const element = ref.current;

      if (!element) {
        timeoutId = setTimeout(setupObserver, 100);
        return;
      }

      observer = new IntersectionObserver(
        (entries) => {
          entries.forEach((entry) => {
            if (entry.isIntersecting) {
              stableCallback();
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
  }, [stableCallback, ref]);
}
