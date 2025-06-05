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
    console.log('Infinite scroll triggered');
    callbackRef.current();
  }, []);

  useEffect(() => {
    let observer: IntersectionObserver | null = null;
    let timeoutId: NodeJS.Timeout | null = null;

    const setupObserver = () => {
      const element = ref.current;
      console.log('Checking for element:', element);
      
      if (!element) {
        console.log('Element not found, retrying in 100ms...');
        timeoutId = setTimeout(setupObserver, 100);
        return;
      }

      console.log('Element found, setting up intersection observer');
      observer = new IntersectionObserver(
        (entries) => {
          console.log('Intersection observer triggered with', entries.length, 'entries');
          entries.forEach((entry) => {
            console.log('Entry details:', {
              isIntersecting: entry.isIntersecting,
              intersectionRatio: entry.intersectionRatio,
              boundingClientRect: entry.boundingClientRect,
              target: entry.target
            });
            if (entry.isIntersecting) {
              console.log('Element is intersecting, triggering callback');
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
      console.log('Successfully started observing element');
    };

    // Start the setup process
    setupObserver();

    return () => {
      console.log('Cleaning up infinite scroll');
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
      if (observer) {
        observer.disconnect();
      }
    };
  }, [stableCallback, ref]);
}
