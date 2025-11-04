export function throttle<T extends (...args: never[]) => void>(
  func: T,
  delay: number
): (...args: Parameters<T>) => void {
  let timeoutId: NodeJS.Timeout | null = null;
  let lastExecTime = 0;

  return (...args: Parameters<T>) => {
    const currentTime = Date.now();
    const timeSinceLastExec = currentTime - lastExecTime;

    if (timeSinceLastExec > delay) {
      func(...args);
      lastExecTime = currentTime;
    } else {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }

      const remainingDelay = delay - timeSinceLastExec;
      timeoutId = setTimeout(() => {
        func(...args);
        lastExecTime = Date.now();
      }, remainingDelay);
    }
  };
}

export function debounce<T extends (...args: never[]) => void>(
  func: T,
  delay: number
): (...args: Parameters<T>) => void {
  let timeoutId: NodeJS.Timeout | null = null;

  return (...args: Parameters<T>) => {
    if (timeoutId) {
      clearTimeout(timeoutId);
    }
    timeoutId = setTimeout(() => func(...args), delay);
  };
}

export function createRateLimiter(maxCalls: number, windowMs: number) {
  const calls: number[] = [];

  return function rateLimiter(): boolean {
    const now = Date.now();

    // Remove calls outside the window
    while (calls.length > 0 && calls[0] <= now - windowMs) {
      calls.shift();
    }

    // Check if we can make another call
    if (calls.length < maxCalls) {
      calls.push(now);
      return true;
    }

    return false;
  };
}
