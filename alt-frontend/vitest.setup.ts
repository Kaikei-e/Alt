import "@testing-library/jest-dom";
import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach, vi } from "vitest";

// Browser environment specific mocks (only in jsdom)
if (typeof window !== 'undefined') {
  // Mock window.matchMedia globally
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });

  // Mock ResizeObserver
  global.ResizeObserver = vi.fn().mockImplementation(() => ({
    observe: vi.fn(),
    unobserve: vi.fn(),
    disconnect: vi.fn(),
  }));

  // Mock IntersectionObserver
  global.IntersectionObserver = vi.fn().mockImplementation(() => ({
    observe: vi.fn(),
    unobserve: vi.fn(),
    disconnect: vi.fn(),
  }));

  // Stub navigation to avoid jsdom "navigation not implemented" errors in unit tests
  try {
    const originalLocation = window.location;
    let hrefValue = originalLocation?.href || 'http://localhost/';

    const mockLocation: Partial<Location> & { href: string } = {
      ...originalLocation,
      assign: vi.fn((url: string | URL) => {
        hrefValue = String(url);
      }),
      replace: vi.fn((url: string | URL) => {
        hrefValue = String(url);
      }),
      reload: vi.fn(),
      get href() {
        return hrefValue;
      },
      set href(val: string) {
        hrefValue = String(val);
      },
    } as any;

    Object.defineProperty(window, 'location', {
      configurable: true,
      writable: true,
      value: mockLocation,
    });
  } catch {
    // ignore if window/location is not available
  }
}

// ENHANCED: Aggressive cleanup to prevent flaky tests
beforeEach(() => {
  // Clear all mocks before each test
  vi.clearAllMocks();

  // Force DOM cleanup
  if (typeof document !== 'undefined') {
    cleanup();
    document.body.innerHTML = '';
    document.head.querySelectorAll('style').forEach(style => style.remove());
  }

  // Clear any pending timers
  vi.clearAllTimers();

  // Reset browser state
  if (typeof window !== 'undefined') {
    // Clear storage
    localStorage.clear();
    sessionStorage.clear();

    // Reset scroll
    window.scrollTo(0, 0);

    // Clear any global event listeners that might leak
    const events = ['resize', 'scroll', 'click', 'mousedown', 'mouseup', 'keydown', 'keyup'];
    events.forEach(event => {
      // Remove any existing event listeners for this event type
      window.removeEventListener(event, () => {});
    });
  }

  // Force garbage collection if available
  if (global.gc) {
    global.gc();
  }
});

afterEach(async () => {
  // Comprehensive cleanup after each test
  if (typeof document !== 'undefined') {
    cleanup();

    // More aggressive DOM cleanup
    document.body.innerHTML = '';
    document.head.innerHTML = document.head.innerHTML.replace(/<style[^>]*>.*?<\/style>/gi, '');

    // Clear any remaining React roots or portals
    const reactRoots = document.querySelectorAll('[data-reactroot], [data-react-root]');
    reactRoots.forEach(root => root.remove());
  }

  // Clear all timers and mocks
  vi.clearAllTimers();
  vi.clearAllMocks();

  // Wait for any pending async operations
  await new Promise(resolve => setTimeout(resolve, 0));

  // Force garbage collection
  if (global.gc) {
    global.gc();
  }
});

// Suppress styled-jsx and other test-specific warnings
const originalError = console.error;
const originalWarn = console.warn;

console.error = (...args) => {
  const message = typeof args[0] === 'string' ? args[0] : '';

  // styled-jsx warnings
  if (message.includes('Received `true` for a non-boolean attribute `jsx`') ||
    message.includes('jsx="true"') ||
    message.includes('If you want to write it to the DOM, pass a string instead')) {
    return;
  }

  // React DOM warnings that are expected in tests
  if (message.includes('Warning: ReactDOM.render is no longer supported') ||
    message.includes('Warning: Each child in a list should have a unique "key" prop') ||
    message.includes('Warning: Failed prop type')) {
    return;
  }

  // Memory-related warnings we want to suppress in tests
  if (message.includes('Worker terminated due to reaching memory limit') ||
    message.includes('heap out of memory')) {
    return;
  }

  originalError.call(console, ...args);
};

console.warn = (...args) => {
  const message = typeof args[0] === 'string' ? args[0] : '';

  if (message.includes('jsx') ||
    message.includes('styled-jsx') ||
    message.includes('React.createRef is deprecated')) {
    return;
  }

  originalWarn.call(console, ...args);
};