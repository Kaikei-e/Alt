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
            stableCallback();
          }
        });
      },
      {
        rootMargin: "10px",
      },
    );

    observer.observe(ref.current);

    return () => observer.disconnect();
  }, [stableCallback, ref]);
}
