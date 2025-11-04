/* eslint-disable @typescript-eslint/no-explicit-any */

import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeToggle } from "../../../src/components/ThemeToggle";
import { ThemeProvider } from "../../../src/providers/ThemeProvider";

// Mock ChakraUI components for testing - keep some mocks but use real ChakraProvider
vi.mock("@chakra-ui/react", async () => {
  const actual = (await vi.importActual("@chakra-ui/react")) as any;
  return {
    ...actual,
    Button: ({ children, onClick, onKeyDown, css, ...props }: any) => (
      <button onClick={onClick} onKeyDown={onKeyDown} style={css} {...props}>
        {children}
      </button>
    ),
    Text: ({ children, ...props }: any) => <span {...props}>{children}</span>,
    VStack: ({ children, ...props }: any) => <div {...props}>{children}</div>,
    Box: ({ children, ...props }: any) => <div {...props}>{children}</div>,
  };
});

// Mock Lucide icons
vi.mock("lucide-react", () => ({
  Sun: ({ style }: any) => <svg data-testid="sun-icon" style={style} />,
  Moon: ({ style }: any) => <svg data-testid="moon-icon" style={style} />,
}));

// Polyfill window.matchMedia for jsdom environment - always create it
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

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
};

Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  writable: true,
});

// Mock document.body.setAttribute to track DOM changes
const setAttributeMock = vi.fn();
Object.defineProperty(document.body, "setAttribute", {
  value: setAttributeMock,
  writable: true,
});

// Mock document.body.getAttribute to simulate theme state
const getAttributeMock = vi.fn();
Object.defineProperty(document.body, "getAttribute", {
  value: getAttributeMock,
  writable: true,
});

// Mock getComputedStyle for CSS variable testing
const originalGetComputedStyle = window.getComputedStyle;
const mockGetComputedStyle = vi.fn();
Object.defineProperty(window, "getComputedStyle", {
  value: mockGetComputedStyle,
  writable: true,
});

describe("Theme Integration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorageMock.getItem.mockReturnValue(null);
    getAttributeMock.mockReturnValue("alt-paper");

    // Setup default CSS variables for alt-paper theme
    mockGetComputedStyle.mockReturnValue({
      getPropertyValue: vi.fn((prop: string) => {
        const cssVars: Record<string, string> = {
          "--foreground": "#2c2c2c",
          "--background": "#f8f6f2",
          "--accent-primary": "#6b7280",
          "--accent-secondary": "#9ca3af",
          "--accent-tertiary": "#d1d5db",
        };
        return cssVars[prop] || "";
      }),
    });

    cleanup();
  });

  afterEach(() => {
    cleanup();
    // Restore original getComputedStyle
    window.getComputedStyle = originalGetComputedStyle;
  });

  it("デフォルトでalt-paperテーマが適用される", async () => {
    render(
      <ChakraProvider value={defaultSystem}>
        <ThemeProvider>
          <ThemeToggle />
        </ThemeProvider>
      </ChakraProvider>
    );

    // Wait for component to mount and theme to be resolved
    await waitFor(
      () => {
        const toggleButton = screen.queryByTestId("theme-toggle-button");
        expect(toggleButton).toBeInTheDocument();
        expect(toggleButton).toBeDefined();
      },
      { timeout: 3000 }
    );

    // Verify default theme is alt-paper
    const toggleButton = screen.getByTestId("theme-toggle-button");
    expect(toggleButton.getAttribute("aria-checked")).toBe("false"); // alt-paper = false (not vaporwave)

    // Verify that the sun icon is rendered (either mocked or real SVG)
    const iconContainer = toggleButton.querySelector("svg");
    expect(iconContainer).toBeTruthy();
  });
});
