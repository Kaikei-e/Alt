import { useCallback, useEffect } from "react";

export function useInfiniteScroll(
  callback: () => void,
  ref: React.RefObject<HTMLDivElement | null>,
) {
  const stableCallback = useCallback(callback, [callback]);

  useEffect(() => {
    if (!ref.current) return;

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            console.log("Reached bottom of the page");
            stableCallback();
          }
        });
      },
      {
        rootMargin: '20px', // Trigger slightly before the element is fully visible
      }
    );

    observer.observe(ref.current);

    return () => observer.disconnect();
  }, [stableCallback, ref]);
}
