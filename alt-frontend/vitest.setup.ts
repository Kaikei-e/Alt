import "@testing-library/jest-dom";
import { vi } from "vitest";

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
