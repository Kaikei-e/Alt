import "@testing-library/jest-dom";
import { vi } from "vitest";

// Global test setup for Vitest

// Mock window.matchMedia globally
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(), // deprecated
    removeListener: vi.fn(), // deprecated
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

// Mock window.ResizeObserver
global.ResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));

// Mock window.IntersectionObserver
global.IntersectionObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));

// Set up environment variables
process.env.NEXT_PUBLIC_IDP_ORIGIN =
  process.env.NEXT_PUBLIC_IDP_ORIGIN || "https://id.test.local";
process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL =
  process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL || "https://id.test.local";
process.env.NEXT_PUBLIC_APP_ORIGIN =
  process.env.NEXT_PUBLIC_APP_ORIGIN || "https://app.test.local";
process.env.NEXT_PUBLIC_AUTH_SERVICE_URL =
  process.env.NEXT_PUBLIC_AUTH_SERVICE_URL || "https://auth.test.local";
