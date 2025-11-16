// src/test-utils.tsx

import { ChakraProvider, createSystem, defaultConfig } from "@chakra-ui/react";
import { cleanup, type RenderOptions, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type React from "react";
import { vi } from "vitest";

// Create a minimal system for testing
const testSystem = createSystem(defaultConfig);

// Create a wrapper that ensures clean state
const TestWrapper: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  return <ChakraProvider value={testSystem}>{children}</ChakraProvider>;
};

// Enhanced render function with automatic cleanup
const customRender = (
  ui: React.ReactElement,
  options?: Omit<RenderOptions, "wrapper">,
) => {
  // Force cleanup before rendering
  cleanup();

  // Clear document body to ensure clean slate
  if (typeof document !== "undefined") {
    document.body.innerHTML = "";
  }

  const result = render(ui, { wrapper: TestWrapper, ...options });

  return {
    ...result,
    user: userEvent.setup(), // Include user event setup
  };
};

// Specific render function for components
export const renderWithProviders = customRender;

// Helper to safely get unique elements
export const getSafeElement = (
  getByTestId: (id: string) => HTMLElement,
  testId: string,
) => {
  try {
    return getByTestId(testId);
  } catch (error) {
    const elements = document.querySelectorAll(`[data-testid="${testId}"]`);
    if (elements.length > 1) {
      return elements[0] as HTMLElement;
    }
    throw error;
  }
};

// Helper to wait for unique element
export const waitForUniqueElement = async (
  testId: string,
  timeout: number = 3000,
): Promise<HTMLElement> => {
  return new Promise((resolve, reject) => {
    const startTime = Date.now();

    const checkElement = () => {
      const elements = document.querySelectorAll(`[data-testid="${testId}"]`);

      if (elements.length === 1) {
        resolve(elements[0] as HTMLElement);
        return;
      }

      if (Date.now() - startTime > timeout) {
        reject(
          new Error(
            `Timeout waiting for unique element with testId "${testId}". Found ${elements.length} elements.`,
          ),
        );
        return;
      }

      setTimeout(checkElement, 50);
    };

    checkElement();
  });
};

// Mock data generators
export const createMockDesktopFeeds = (count: number = 3) => {
  return Array.from({ length: count }, (_, index) => ({
    id: `desktop-feed-${index}`,
    title: `Desktop Feed ${index}`,
    description: `Detailed description for desktop feed ${index}...`,
    isRead: false,
    isFavorite: false,
    isBookmarked: false,
    readLater: false,
    publishedAt: new Date().toISOString(),
    url: `https://example.com/article-${index}`,
  }));
};

// Helper to create fresh mock functions
export const createMockHandlers = () => ({
  onMarkAsRead: vi.fn(),
  onToggleFavorite: vi.fn(),
  onToggleBookmark: vi.fn(),
  onToggleReadLater: vi.fn(),
  onViewArticle: vi.fn(),
  onLoadMore: vi.fn(),
});

// Helper to ensure clean test environment
export const ensureCleanEnvironment = () => {
  if (typeof document !== "undefined") {
    // Remove all existing elements with test IDs
    const testElements = document.querySelectorAll("[data-testid]");
    testElements.forEach((el) => el.remove());

    // Clear body
    document.body.innerHTML = "";
  }

  // Clear all mocks
  vi.clearAllMocks();
  vi.clearAllTimers();
};

// Export all testing library utilities
export * from "@testing-library/react";
export { customRender as render };
