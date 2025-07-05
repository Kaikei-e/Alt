/**
 * Performance utilities for optimizing expensive operations
 */

// Debounce function to limit expensive operations
export function debounce<T extends (...args: any[]) => any>(
  func: T,
  wait: number
): (...args: Parameters<T>) => void {
  let timeout: NodeJS.Timeout | null = null;
  
  return function executedFunction(...args: Parameters<T>) {
    const later = () => {
      if (timeout) {
        clearTimeout(timeout);
        timeout = null;
      }
      func(...args);
    };
    
    if (timeout) {
      clearTimeout(timeout);
    }
    timeout = setTimeout(later, wait);
  };
}

// Throttle function to limit frequency of expensive operations
export function throttle<T extends (...args: any[]) => any>(
  func: T,
  limit: number
): (...args: Parameters<T>) => void {
  let inThrottle: boolean = false;
  
  return function executedFunction(...args: Parameters<T>) {
    if (!inThrottle) {
      func(...args);
      inThrottle = true;
      setTimeout(() => inThrottle = false, limit);
    }
  };
}

// Measure performance of operations
export function measurePerformance<T>(
  operation: () => T,
  label?: string
): T {
  const startTime = performance.now();
  const result = operation();
  const endTime = performance.now();
  
  if (label) {
    console.log(`${label} took ${endTime - startTime} milliseconds`);
  }
  
  return result;
}

// Batch operations for better performance
export function batchOperations<T, R>(
  items: T[],
  batchSize: number,
  operation: (batch: T[]) => R[]
): R[] {
  const results: R[] = [];
  
  for (let i = 0; i < items.length; i += batchSize) {
    const batch = items.slice(i, i + batchSize);
    const batchResults = operation(batch);
    results.push(...batchResults);
  }
  
  return results;
}

// Simple memoization for expensive computations
export function memoize<T extends (...args: any[]) => any>(
  fn: T,
  getKey?: (...args: Parameters<T>) => string
): T {
  const cache = new Map<string, ReturnType<T>>();
  
  return ((...args: Parameters<T>): ReturnType<T> => {
    const key = getKey ? getKey(...args) : JSON.stringify(args);
    
    if (cache.has(key)) {
      return cache.get(key)!;
    }
    
    const result = fn(...args);
    cache.set(key, result);
    return result;
  }) as T;
}

// Clear memoization cache when needed
export function clearMemoCache() {
  // This would clear any global caches if implemented
  console.log('Memoization caches cleared');
}